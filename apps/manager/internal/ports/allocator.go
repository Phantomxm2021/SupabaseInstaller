package ports

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"supabase-manager/apps/manager/internal/store"
)

type Kind string

const (
	KindAPI       Kind = "API"
	KindStudio    Kind = "STUDIO"
	KindDirectDB  Kind = "DATABASE"
	KindPoolerTxn Kind = "POOLER_TRANSACTION"
	KindPoolerSes Kind = "POOLER_SESSION"
)

var ErrExhausted = errors.New("no available port in allocation range")

type Probe interface {
	Available(port int) bool
}

// ContextProbe can inspect resources outside the Manager container. The
// provisioner implementation uses Docker's host-side port bindings, which a
// local net.Listen probe cannot see.
type ContextProbe interface {
	AvailableContext(context.Context, int) (bool, error)
}

type NetworkProbe struct{}

func (NetworkProbe) Available(port int) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

type Allocator struct {
	store  *store.Store
	min    int
	max    int
	probe  Probe
	remote ContextProbe
	now    func() time.Time
}

func NewAllocator(store *store.Store, minPort, maxPort int, probe Probe) *Allocator {
	return &Allocator{store: store, min: minPort, max: maxPort, probe: probe, now: time.Now}
}

// NewAllocatorWithContextProbe retains the local probe as a fast fallback for
// native listeners and adds a provisioner-backed check for Docker-published
// host ports.
func NewAllocatorWithContextProbe(store *store.Store, minPort, maxPort int, probe Probe, remote ContextProbe) *Allocator {
	return &Allocator{store: store, min: minPort, max: maxPort, probe: probe, remote: remote, now: time.Now}
}

func (a *Allocator) available(ctx context.Context, port int) (bool, error) {
	if !a.probe.Available(port) {
		return false, nil
	}
	if a.remote == nil {
		return true, nil
	}
	return a.remote.AvailableContext(ctx, port)
}

// ReserveMany allocates all requested server-owned ports as one atomic set.
// Existing reservations are reused; newly selected candidates are committed
// together, so a failed multi-service allocation cannot leak a partial set.
func (a *Allocator) ReserveMany(ctx context.Context, projectID string, kinds []Kind) (map[Kind]int, error) {
	allocated := make(map[Kind]int, len(kinds))
	missing := make([]Kind, 0, len(kinds))
	for _, kind := range kinds {
		if _, seen := allocated[kind]; seen {
			continue
		}
		port, err := a.store.ReservedPort(ctx, projectID, string(kind))
		if err == nil {
			allocated[kind] = port
			continue
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		missing = append(missing, kind)
	}
	if len(missing) == 0 {
		return allocated, nil
	}
	rangeSize := a.max - a.min + 1
	for cursor := 0; cursor < rangeSize; cursor++ {
		selected := make(map[string]int, len(missing))
		used := make(map[int]bool, len(allocated))
		for _, port := range allocated {
			used[port] = true
		}
		for _, kind := range missing {
			for offset := 0; offset < rangeSize; offset++ {
				port := a.min + (cursor+offset)%rangeSize
				if used[port] {
					continue
				}
				available, probeErr := a.available(ctx, port)
				if probeErr != nil {
					return nil, fmt.Errorf("check host port %d: %w", port, probeErr)
				}
				if !available {
					continue
				}
				selected[string(kind)] = port
				used[port] = true
				break
			}
		}
		if len(selected) != len(missing) {
			return nil, ErrExhausted
		}
		reserved, err := a.store.TryReservePorts(ctx, projectID, selected, a.now())
		if err != nil {
			return nil, fmt.Errorf("reserve server-owned ports: %w", err)
		}
		if reserved {
			for kind, port := range selected {
				allocated[Kind(kind)] = port
			}
			return allocated, nil
		}
		for index := 0; index < len(missing); index++ {
			port, readErr := a.store.ReservedPort(ctx, projectID, string(missing[index]))
			if readErr == nil {
				allocated[missing[index]] = port
				missing = append(missing[:index], missing[index+1:]...)
				index--
			}
		}
		if len(missing) == 0 {
			return allocated, nil
		}
	}
	return nil, ErrExhausted
}

// CandidateMany chooses the ports an update would need without changing the
// canonical allocation table. The selected values are protected by the
// configuration admission transaction; this method is deliberately only a
// read/allocate hint so a render failure cannot steal a last-good port.
func (a *Allocator) CandidateMany(ctx context.Context, projectID string, kinds []Kind) (map[Kind]int, error) {
	allocated := make(map[Kind]int, len(kinds))
	missing := make([]Kind, 0, len(kinds))
	used := make(map[int]bool, len(kinds))
	for _, kind := range kinds {
		if _, seen := allocated[kind]; seen {
			continue
		}
		port, err := a.store.ReservedPort(ctx, projectID, string(kind))
		if err == nil {
			allocated[kind] = port
			used[port] = true
			continue
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		missing = append(missing, kind)
	}
	if len(missing) == 0 {
		return allocated, nil
	}
	rangeSize := a.max - a.min + 1
	for offset := 0; offset < rangeSize; offset++ {
		port := a.min + offset
		if used[port] {
			continue
		}
		available, probeErr := a.available(ctx, port)
		if probeErr != nil {
			return nil, fmt.Errorf("check host port %d: %w", port, probeErr)
		}
		if !available {
			continue
		}
		inUse, err := a.store.PortInUse(ctx, port)
		if err != nil {
			return nil, err
		}
		if inUse {
			continue
		}
		kind := missing[0]
		allocated[kind] = port
		used[port] = true
		missing = missing[1:]
		if len(missing) == 0 {
			return allocated, nil
		}
	}
	return nil, ErrExhausted
}

func (a *Allocator) ReleaseProject(ctx context.Context, projectID string) error {
	return a.store.ReleaseProjectPorts(ctx, projectID)
}

func (a *Allocator) Release(ctx context.Context, projectID string, kind Kind) error {
	return a.store.ReleasePort(ctx, projectID, string(kind))
}

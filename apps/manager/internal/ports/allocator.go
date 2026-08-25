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
	KindDirectDB  Kind = "DIRECT_DB"
	KindPoolerTxn Kind = "POOLER_TRANSACTION"
	KindPoolerSes Kind = "POOLER_SESSION"
)

var ErrExhausted = errors.New("no available port in allocation range")

type Probe interface {
	Available(port int) bool
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
	store *store.Store
	min   int
	max   int
	probe Probe
	now   func() time.Time
}

func NewAllocator(store *store.Store, minPort, maxPort int, probe Probe) *Allocator {
	return &Allocator{store: store, min: minPort, max: maxPort, probe: probe, now: time.Now}
}

func (a *Allocator) Reserve(ctx context.Context, projectID string, kind Kind) (int, error) {
	if port, err := a.store.ReservedPort(ctx, projectID, string(kind)); err == nil {
		return port, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return 0, err
	}
	for port := a.min; port <= a.max; port++ {
		if !a.probe.Available(port) {
			continue
		}
		reserved, err := a.store.TryReservePort(ctx, projectID, string(kind), port, a.now())
		if err != nil {
			return 0, fmt.Errorf("reserve %s port: %w", kind, err)
		}
		if reserved {
			return port, nil
		}
	}
	return 0, ErrExhausted
}

func (a *Allocator) ReleaseProject(ctx context.Context, projectID string) error {
	return a.store.ReleaseProjectPorts(ctx, projectID)
}

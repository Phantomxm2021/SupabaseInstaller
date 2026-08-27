package health

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"supabase-manager/internal/contracts"
)

const cpuSampleInterval = 100 * time.Millisecond

// HostResources returns a best-effort host snapshot from the provisioner's
// Linux namespace. The project root is used for disk capacity because it is
// the volume that stores all managed runtimes.
func (source *DockerSource) HostResources(ctx context.Context, projectRoot string) (contracts.HostResources, error) {
	_ = source
	if err := ctx.Err(); err != nil {
		return contracts.HostResources{}, err
	}
	cpuPercent := 0.0
	firstIdle, firstTotal, err := readCPUStat()
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return contracts.HostResources{}, err
	}
	if err == nil {
		timer := time.NewTimer(cpuSampleInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return contracts.HostResources{}, ctx.Err()
		case <-timer.C:
		}
		secondIdle, secondTotal, secondErr := readCPUStat()
		if secondErr != nil {
			if !errors.Is(secondErr, fs.ErrNotExist) {
				return contracts.HostResources{}, secondErr
			}
		} else if totalDelta := secondTotal - firstTotal; totalDelta > 0 {
			idleDelta := secondIdle - firstIdle
			if idleDelta < 0 {
				idleDelta = 0
			}
			cpuPercent = (1 - float64(idleDelta)/float64(totalDelta)) * 100
			if cpuPercent < 0 {
				cpuPercent = 0
			} else if cpuPercent > 100 {
				cpuPercent = 100
			}
		}
	}

	memoryUsed, memoryTotal, memoryErr := readMemory()
	if memoryErr != nil && !errors.Is(memoryErr, fs.ErrNotExist) {
		return contracts.HostResources{}, memoryErr
	}
	diskUsed, diskTotal, err := readDisk(projectRoot)
	if err != nil {
		return contracts.HostResources{}, err
	}
	return contracts.HostResources{
		CPUPercent:       cpuPercent,
		CPUCores:         runtime.NumCPU(),
		MemoryUsedBytes:  memoryUsed,
		MemoryTotalBytes: memoryTotal,
		DiskUsedBytes:    diskUsed,
		DiskTotalBytes:   diskTotal,
		CollectedAt:      time.Now(),
	}, nil
}

func readCPUStat() (idle, total uint64, err error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, fmt.Errorf("open CPU stats: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		if scanErr := scanner.Err(); scanErr != nil {
			return 0, 0, fmt.Errorf("read CPU stats: %w", scanErr)
		}
		return 0, 0, fmt.Errorf("CPU stats are unavailable")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, fmt.Errorf("CPU stats are malformed")
	}
	for index, value := range fields[1:] {
		parsed, parseErr := strconv.ParseUint(value, 10, 64)
		if parseErr != nil {
			return 0, 0, fmt.Errorf("parse CPU stats: %w", parseErr)
		}
		total += parsed
		if index == 3 || index == 4 { // idle and iowait
			idle += parsed
		}
	}
	return idle, total, nil
}

func readMemory() (used, total uint64, err error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, fmt.Errorf("open memory stats: %w", err)
	}
	defer file.Close()
	values := map[string]uint64{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil {
			return 0, 0, fmt.Errorf("parse memory stats: %w", parseErr)
		}
		// /proc/meminfo values are expressed in KiB.
		values[strings.TrimSuffix(fields[0], ":")] = value * 1024
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("read memory stats: %w", err)
	}
	total, ok := values["MemTotal"]
	if !ok || total == 0 {
		return 0, 0, fmt.Errorf("memory total is unavailable")
	}
	available := values["MemAvailable"]
	if available > total {
		available = total
	}
	return total - available, total, nil
}

func readDisk(path string) (used, total uint64, err error) {
	if path == "" {
		return 0, 0, fmt.Errorf("project root is required for disk stats")
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("read disk stats: %w", err)
	}
	blockSize := uint64(stat.Bsize)
	total = stat.Blocks * blockSize
	available := stat.Bavail * blockSize
	if available > total {
		available = total
	}
	return total - available, total, nil
}

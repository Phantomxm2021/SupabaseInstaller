package health

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestHostResourcesReadsCapacityAndUsage(t *testing.T) {
	resources, err := (&DockerSource{}).HostResources(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("HostResources() error = %v", err)
	}
	if resources.CPUCores < 1 || resources.CPUPercent < 0 || resources.CPUPercent > 100 {
		t.Fatalf("unexpected CPU metrics: %#v", resources)
	}
	if _, procAvailable := os.Stat("/proc/meminfo"); procAvailable == nil && resources.MemoryTotalBytes == 0 {
		t.Fatalf("memory metrics are unavailable on a procfs host: %#v", resources)
	}
	if resources.MemoryUsedBytes > resources.MemoryTotalBytes {
		t.Fatalf("unexpected memory metrics: %#v", resources)
	}
	if resources.DiskTotalBytes == 0 || resources.DiskUsedBytes > resources.DiskTotalBytes {
		t.Fatalf("unexpected disk metrics: %#v", resources)
	}
	if resources.CollectedAt.IsZero() || time.Since(resources.CollectedAt) < 0 {
		t.Fatalf("unexpected collection time: %s", resources.CollectedAt)
	}
}

func TestHostResourcesHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&DockerSource{}).HostResources(ctx, t.TempDir()); err == nil {
		t.Fatal("HostResources() succeeded with canceled context")
	}
}

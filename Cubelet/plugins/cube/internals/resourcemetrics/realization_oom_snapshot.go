package resourcemetrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	cubeboxstore "github.com/tencentcloud/CubeSandbox/Cubelet/pkg/store/cubebox"
)

type realizationOOMCubeboxStore interface {
	Get(context.Context, string) (*cubeboxstore.CubeBox, error)
}

type realizationOOMSnapshotBinder interface {
	SetRealizationOOMSnapshotReader(func(context.Context, string) (string, bool, uint64, time.Time, error))
}

func newRealizationOOMSnapshotReader(store realizationOOMCubeboxStore, reader hostSandboxUsageReader, now func() time.Time) func(context.Context, string) (string, bool, uint64, time.Time, error) {
	return func(ctx context.Context, sandboxID string) (string, bool, uint64, time.Time, error) {
		if store == nil || reader == nil || now == nil {
			return "", false, 0, time.Time{}, fmt.Errorf("realization OOM snapshot authority is unavailable")
		}
		if strings.TrimSpace(sandboxID) == "" {
			return "", false, 0, time.Time{}, fmt.Errorf("realization OOM sandbox ID is required")
		}
		box, err := store.Get(ctx, sandboxID)
		if err != nil {
			return "", false, 0, time.Time{}, fmt.Errorf("load cubebox %s for realization OOM snapshot: %w", sandboxID, err)
		}
		if box == nil {
			return "", false, 0, time.Time{}, fmt.Errorf("cubebox %s is unavailable for realization OOM snapshot", sandboxID)
		}
		path := box.CGroupPath
		if strings.TrimSpace(path) == "" || strings.TrimSpace(path) != path {
			return "", false, 0, time.Time{}, fmt.Errorf("cubebox %s has no canonical cgroup path", sandboxID)
		}
		usage, err := reader.UsageSnapshot(ctx, path)
		if err != nil {
			return "", false, 0, time.Time{}, fmt.Errorf("read realization OOM snapshot for %s: %w", sandboxID, err)
		}
		capturedAt := now().UTC()
		if capturedAt.IsZero() {
			return "", false, 0, time.Time{}, fmt.Errorf("realization OOM snapshot timestamp is unavailable")
		}
		if !usage.MemoryOOMKillsKnown {
			return path, false, 0, capturedAt, nil
		}
		return path, true, usage.MemoryOOMKillsTotal, capturedAt, nil
	}
}

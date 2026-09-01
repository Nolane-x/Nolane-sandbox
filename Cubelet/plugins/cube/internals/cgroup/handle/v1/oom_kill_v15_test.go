package v1

import (
	"testing"

	cgroup1stats "github.com/containerd/cgroups/v3/cgroup1/stats"
	"github.com/stretchr/testify/require"

	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/cgroup/handle"
)

func TestV15UsageSnapshotPreservesV1OOMKillEvidence(t *testing.T) {
	got, err := usageSnapshotFromMetrics(&cgroup1stats.Metrics{
		CPU: &cgroup1stats.CPUStat{
			Usage:      &cgroup1stats.CPUUsage{},
			Throttling: &cgroup1stats.Throttle{},
		},
		Memory:           &cgroup1stats.MemoryStat{Usage: &cgroup1stats.MemoryEntry{}},
		MemoryOomControl: &cgroup1stats.MemoryOomControl{OomKill: 4},
	}, handle.CPULimit{})
	require.NoError(t, err)
	require.True(t, got.MemoryOOMKillsKnown)
	require.Equal(t, uint64(4), got.MemoryOOMKillsTotal)
}

func TestV15UsageSnapshotKeepsV1OOMKillUnknownWhenKernelEvidenceMissing(t *testing.T) {
	got, err := usageSnapshotFromMetrics(&cgroup1stats.Metrics{
		CPU: &cgroup1stats.CPUStat{
			Usage:      &cgroup1stats.CPUUsage{},
			Throttling: &cgroup1stats.Throttle{},
		},
		Memory: &cgroup1stats.MemoryStat{Usage: &cgroup1stats.MemoryEntry{}},
	}, handle.CPULimit{})
	require.NoError(t, err)
	require.False(t, got.MemoryOOMKillsKnown)
	require.Zero(t, got.MemoryOOMKillsTotal)
}

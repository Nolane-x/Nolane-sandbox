package v2

import (
	"testing"

	cgroup2stats "github.com/containerd/cgroups/v3/cgroup2/stats"
	"github.com/stretchr/testify/require"

	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/cgroup/handle"
)

func TestV15UsageSnapshotPreservesV2OOMKillEvidence(t *testing.T) {
	got, err := usageSnapshotFromMetrics(&cgroup2stats.Metrics{
		CPU:          &cgroup2stats.CPUStat{},
		Memory:       &cgroup2stats.MemoryStat{},
		MemoryEvents: &cgroup2stats.MemoryEvents{OomKill: 4},
	}, handle.CPULimit{})
	require.NoError(t, err)
	require.True(t, got.MemoryOOMKillsKnown)
	require.Equal(t, uint64(4), got.MemoryOOMKillsTotal)
}

func TestV15UsageSnapshotKeepsV2OOMKillUnknownWhenKernelEvidenceMissing(t *testing.T) {
	got, err := usageSnapshotFromMetrics(&cgroup2stats.Metrics{
		CPU:    &cgroup2stats.CPUStat{},
		Memory: &cgroup2stats.MemoryStat{},
	}, handle.CPULimit{})
	require.NoError(t, err)
	require.False(t, got.MemoryOOMKillsKnown)
	require.Zero(t, got.MemoryOOMKillsTotal)
}

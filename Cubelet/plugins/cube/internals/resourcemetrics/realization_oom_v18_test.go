package resourcemetrics

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	cubeboxstore "github.com/tencentcloud/CubeSandbox/Cubelet/pkg/store/cubebox"
	"github.com/tencentcloud/CubeSandbox/Cubelet/plugins/cube/internals/cgroup/handle"
)

type fakeRealizationOOMStore struct {
	box *cubeboxstore.CubeBox
	err error
}

func (f fakeRealizationOOMStore) Get(context.Context, string) (*cubeboxstore.CubeBox, error) {
	return f.box, f.err
}

type fakeRealizationOOMReader struct {
	snapshot handle.UsageSnapshot
	err      error
	group    string
}

func (f *fakeRealizationOOMReader) UsageSnapshot(_ context.Context, group string) (handle.UsageSnapshot, error) {
	f.group = group
	return f.snapshot, f.err
}

func TestRealizationOOMSnapshotReaderBindsSandboxToExactCgroup(t *testing.T) {
	capturedAt := time.Date(2026, 9, 5, 5, 50, 0, 123456789, time.UTC)
	reader := &fakeRealizationOOMReader{snapshot: handle.UsageSnapshot{MemoryOOMKillsKnown: true, MemoryOOMKillsTotal: 17}}
	read := newRealizationOOMSnapshotReader(fakeRealizationOOMStore{box: &cubeboxstore.CubeBox{Metadata: cubeboxstore.Metadata{ID: "sandbox-a"}, CGroupPath: "/cube/path"}}, reader, func() time.Time { return capturedAt })

	path, known, total, gotAt, err := read(context.Background(), "sandbox-a")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if path != "/cube/path" || reader.group != "/cube/path" || !known || total != 17 || !gotAt.Equal(capturedAt) {
		t.Fatalf("snapshot = path=%q known=%v total=%d at=%v reader=%q", path, known, total, gotAt, reader.group)
	}
}

func TestRealizationOOMSnapshotReaderPreservesUnknownSignal(t *testing.T) {
	capturedAt := time.Date(2026, 9, 5, 5, 51, 0, 0, time.UTC)
	reader := &fakeRealizationOOMReader{snapshot: handle.UsageSnapshot{MemoryOOMKillsKnown: false, MemoryOOMKillsTotal: 99}}
	read := newRealizationOOMSnapshotReader(fakeRealizationOOMStore{box: &cubeboxstore.CubeBox{Metadata: cubeboxstore.Metadata{ID: "sandbox-a"}, CGroupPath: "/cube/path"}}, reader, func() time.Time { return capturedAt })

	path, known, total, gotAt, err := read(context.Background(), "sandbox-a")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if path != "/cube/path" || known || total != 0 || !gotAt.Equal(capturedAt) {
		t.Fatalf("unknown signal = path=%q known=%v total=%d at=%v", path, known, total, gotAt)
	}
}

type fakeRealizationOOMProofVisitor struct {
	visit bool
}

func (f fakeRealizationOOMProofVisitor) VisitRealizationOOMProofs(visit func(string, uint64, string, uint64, uint64, uint64, time.Time, time.Time, time.Time, string)) {
	if !f.visit {
		return
	}
	visit(
		"sandbox-a",
		math.MaxUint64,
		"/cube/path",
		math.MaxUint64-1,
		math.MaxUint64,
		1,
		time.Date(2026, 9, 5, 5, 0, 0, 123456789, time.UTC),
		time.Date(2026, 9, 5, 5, 1, 0, 987654321, time.UTC),
		time.Date(2026, 9, 5, 5, 0, 59, 555555555, time.UTC),
		"containerd.task.wait",
	)
}

func TestRealizationOOMTransportPreservesExactUint64AndNanoseconds(t *testing.T) {
	handler := newPrometheusHandlerWithTaskEvidence(nil, nil, fakeRealizationOOMProofVisitor{visit: true}, time.Now)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/metrics/resource", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := recorder.Body.String()
	for _, want := range []string{
		"cubesandbox_task_realization_oom_info",
		`sandbox_id="sandbox-a"`,
		`generation="` + strconv.FormatUint(math.MaxUint64, 10) + `"`,
		`cgroup_path="/cube/path"`,
		`signal="kernel.cgroup.memory.oom_kill"`,
		`baseline_oom_kills="` + strconv.FormatUint(math.MaxUint64-1, 10) + `"`,
		`final_oom_kills="` + strconv.FormatUint(math.MaxUint64, 10) + `"`,
		`oom_kills="1"`,
		`baseline_at="2026-09-05T05:00:00.123456789Z"`,
		`observed_at="2026-09-05T05:01:00.987654321Z"`,
		`exited_at="2026-09-05T05:00:59.555555555Z"`,
		`outcome_source="containerd.task.wait"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Wave 18 exposition missing %q:\n%s", want, body)
		}
	}
}

func TestRealizationOOMTransportOmitsUnknownProof(t *testing.T) {
	handler := newPrometheusHandlerWithTaskEvidence(nil, nil, fakeRealizationOOMProofVisitor{}, time.Now)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/metrics/resource", nil))
	if strings.Contains(recorder.Body.String(), "cubesandbox_task_realization_oom_info") {
		t.Fatalf("unknown Wave 18 proof was exported:\n%s", recorder.Body.String())
	}
}

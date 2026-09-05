//go:build linux

package kernelvictim

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

const kernelVictimRingBufferBytes = 1 << 20

func StartBestEffortCollector(ctx context.Context) (Source, error) {
	if ctx == nil {
		return nil, fmt.Errorf("kernel victim collector context is required")
	}
	bootID, offsetSeconds, offsetNanoseconds, err := collectorIdentity()
	if err != nil {
		return nil, err
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return nil, fmt.Errorf("kernel victim starttime bridge is unsupported on %s", runtime.GOARCH)
	}

	kernelBTF, err := btf.LoadKernelSpec()
	if err != nil {
		return nil, fmt.Errorf("load running kernel BTF: %w", err)
	}
	layout, err := ResolveLayout(kernelBTF)
	if err != nil {
		return nil, err
	}

	events, err := ebpf.NewMap(&ebpf.MapSpec{Type: ebpf.RingBuf, MaxEntries: kernelVictimRingBufferBytes})
	if err != nil {
		return nil, fmt.Errorf("create kernel victim ring buffer: %w", err)
	}
	programSpec, err := BuildProgramSpec(layout, events)
	if err != nil {
		events.Close()
		return nil, err
	}
	program, err := ebpf.NewProgram(programSpec)
	if err != nil {
		events.Close()
		return nil, fmt.Errorf("load kernel victim raw-tracepoint program: %w", err)
	}
	attached, err := link.AttachRawTracepoint(link.RawTracepointOptions{Name: "mark_victim", Program: program})
	if err != nil {
		program.Close()
		events.Close()
		return nil, fmt.Errorf("attach oom mark_victim raw tracepoint: %w", err)
	}
	reader, err := ringbuf.NewReader(events)
	if err != nil {
		attached.Close()
		program.Close()
		events.Close()
		return nil, fmt.Errorf("open kernel victim ring buffer: %w", err)
	}

	store := NewStore()
	go runCollector(ctx, reader, attached, program, events, store, bootID, offsetSeconds, offsetNanoseconds)
	return store, nil
}

func runCollector(
	ctx context.Context,
	reader *ringbuf.Reader,
	attached link.Link,
	program *ebpf.Program,
	events *ebpf.Map,
	store *Store,
	bootID string,
	offsetSeconds int64,
	offsetNanoseconds int64,
) {
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = reader.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	defer reader.Close()
	defer attached.Close()
	defer program.Close()
	defer events.Close()

	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) || ctx.Err() != nil {
				return
			}
			return
		}
		event, err := normalizeCollectorRecord(record.RawSample, bootID, offsetSeconds, offsetNanoseconds, collectorGOARCH())
		if err != nil {
			continue
		}
		_ = store.Add(event)
	}
}

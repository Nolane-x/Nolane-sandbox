package cube

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type v29BlockingWaitChild struct {
	mu        sync.Mutex
	pid       int
	alive     bool
	killCount int
	waitCount int
	killed    chan struct{}
	cancel    context.CancelFunc
}

func newV29BlockingWaitChild(pid int, cancel context.CancelFunc) *v29BlockingWaitChild {
	return &v29BlockingWaitChild{pid: pid, alive: true, killed: make(chan struct{}), cancel: cancel}
}

func (c *v29BlockingWaitChild) PID() int { return c.pid }
func (c *v29BlockingWaitChild) AwaitReady(context.Context, string) error { return nil }
func (c *v29BlockingWaitChild) Release(string) error { return nil }
func (c *v29BlockingWaitChild) AwaitDone(context.Context, string) error {
	c.cancel()
	return nil
}
func (c *v29BlockingWaitChild) Wait() (int, error) {
	c.mu.Lock()
	c.waitCount++
	c.mu.Unlock()
	<-c.killed
	c.mu.Lock()
	c.alive = false
	c.mu.Unlock()
	return -1, nil
}
func (c *v29BlockingWaitChild) Kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.killCount++
	if c.alive {
		c.alive = false
		select {
		case <-c.killed:
		default:
			close(c.killed)
		}
	}
	return nil
}
func (c *v29BlockingWaitChild) Alive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.alive
}
func (c *v29BlockingWaitChild) counts() (kill, wait int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.killCount, c.waitCount
}

func TestV29ContextCancellationAfterDoneKillsAndReapsExactChild(t *testing.T) {
	f := newV29Fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	child := newV29BlockingWaitChild(v29HelperPID, cancel)
	f.executor.launch = func(context.Context, string, string) (runtimeCgroupHelperChild, error) {
		return child, nil
	}

	done := make(chan error, 1)
	go func() {
		_, err := ValidateRuntimeCgroupHelperPlacementAuthority(
			ctx,
			f.v28.bridge.controller,
			f.v28.bridge.realization,
			f.v28.runtimeAuth,
			f.v28.bridge.epochObserver,
			f.v28.bridge.client,
			f.v28.runtimeObserver,
			f.v28.cgroupObserver,
			f.executor,
		)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v want context canceled", err)
		}
	case <-time.After(250 * time.Millisecond):
		// Release the intentionally blocked test child so this RED test cannot
		// leak a goroutine in the failing implementation.
		_ = child.Kill()
		<-done
		t.Fatal("Wave29 blocked in child Wait after context cancellation")
	}
	killCount, waitCount := child.counts()
	if killCount != 1 || waitCount != 1 {
		t.Fatalf("kill=%d wait=%d want exact kill/reap once", killCount, waitCount)
	}
}

func TestV29HelperExecutableDigestMustMatchExactStartedChild(t *testing.T) {
	f := newV29Fixture(t)
	f.executor.readHelperExecutableDigest = func(pid int) (string, error) {
		if pid != v29HelperPID {
			t.Fatalf("pid=%d want=%d", pid, v29HelperPID)
		}
		return strings.Repeat("4b", 32), nil
	}
	_, err := validateV29(t, f)
	if !errors.Is(err, ErrInvalidRuntimeCgroupHelperPlacementAuthority) {
		t.Fatalf("error=%v want ErrInvalidRuntimeCgroupHelperPlacementAuthority", err)
	}
	if f.child.waitCount != 1 {
		t.Fatalf("mismatched child executable was not reaped: wait=%d", f.child.waitCount)
	}
}

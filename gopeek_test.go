package gopeek_test

import (
	"sync"
	"testing"
	"time"

	"github.com/cat2neat/gopeek"
	"github.com/davecgh/go-spew/spew"
	"github.com/maruel/panicparse/stack"
)

func TestGoPeek(t *testing.T) {
	tests := []struct {
		do       func() ([]stack.Goroutine, error)
		expected int
		err      error
	}{
		{
			do: func() ([]stack.Goroutine, error) {
				return gopeek.NewCondition().
					FilterByGo(
						func(g *stack.Goroutine) bool {
							return true
						}).
					FilterByGoes(
						func(gs []stack.Goroutine) bool {
							// There should be only 2 goroutines
							// - main.main (StateWaitingChannel)
							// - running this test (StateRunning)
							return len(gs) == 2
						}).
					In(gopeek.StateRunning, gopeek.StateWaitingChannel).
					EQ(2).Eval()
			},
			expected: 2,
		},
		{
			do: func() ([]stack.Goroutine, error) {
				// Never happen
				return gopeek.NewCondition().
					In(gopeek.StateSysCall, gopeek.StateWaitingIO).
					GT(1).Wait(time.Millisecond)
			},
			err: gopeek.ErrTimeout,
		},
		{
			do: func() ([]stack.Goroutine, error) {
				cond := sync.NewCond(&sync.Mutex{})
				for i := 0; i < 3; i++ {
					go func() {
						cond.L.Lock()
						cond.Wait()
						cond.L.Unlock()
					}()
				}
				// Wait until all spawned goroutines blocked due to lock(cond)
				gs, err := gopeek.NewCondition().
					CreatedBy("gopeek_test.TestGoPeek.*").
					Is(gopeek.StateWaitingLock).
					EQ(3).Wait(time.Second)
				cond.Broadcast()
				return gs, err
			},
			expected: 3,
		},
		{
			do: func() ([]stack.Goroutine, error) {
				go func() {
					time.Sleep(time.Second)
				}()
				// Wait until the spawned goroutine blocked due to sleeping
				return gopeek.NewCondition().
					Not(gopeek.StateWaitingIO).
					Not(gopeek.StateWaitingSelect).
					Not(gopeek.StateWaitingLock).
					LT(5).
					GT(2).
					Is(gopeek.StateSleeping).
					EQ(1).
					Wait(time.Millisecond)
			},
			expected: 1,
		},
	}
	for _, ts := range tests {
		gs, err := ts.do()
		if ts.err == nil {
			if err != nil {
				t.Fatal(err)
			}
			if ts.expected != len(gs) {
				t.Errorf("# of goroutines expected: %d, actual: %d\n", ts.expected, len(gs))
			}
		} else if ts.err != err {
			t.Errorf("error expected: %+v, actual: %+v\n", ts.err, err)
		}
		spew.Dump(gs)
	}
}

func TestState(t *testing.T) {
	tests := []struct {
		input    string
		expected gopeek.State
	}{
		{input: "idle", expected: gopeek.StateIdle},
		{input: "runnable", expected: gopeek.StateRunnable},
		{input: "running", expected: gopeek.StateRunning},
		{input: "syscall", expected: gopeek.StateSysCall},
		{input: "waiting", expected: gopeek.StateWaiting},
		{input: "dead", expected: gopeek.StateDead},
		{input: "enqueue", expected: gopeek.StateEnqueue},
		{input: "copystack", expected: gopeek.StateCopyStack},
		{input: "sleep", expected: gopeek.StateSleeping},
		{input: "chan send", expected: gopeek.StateWaitingChannel},
		{input: "chan receive", expected: gopeek.StateWaitingChannel},
		{input: "select", expected: gopeek.StateWaitingSelect},
		{input: "select (no cases)", expected: gopeek.StateWaitingSelect},
		{input: "IO wait", expected: gopeek.StateWaitingIO},
		{input: "semacquire", expected: gopeek.StateWaitingLock},
		{input: "semarelease", expected: gopeek.StateWaitingLock},
		{input: "GC sweep wait", expected: gopeek.StateWaitingGCActivity},
		{input: "GC assist wait", expected: gopeek.StateWaitingGCActivity},
		{input: "force gc (idle)", expected: gopeek.StateWaitingGCActivity},
		{input: "GC assist marking", expected: gopeek.StateWaitingGCActivity},
		{input: "garbage collection scan", expected: gopeek.StateWaitingGCActivity},
		{input: "garbage collection", expected: gopeek.StateWaitingGCActivity},
		{input: "panicwait", expected: gopeek.StateOther},
		{input: "stack growth", expected: gopeek.StateOther},
		{input: "dumping heap", expected: gopeek.StateOther},
		{input: "trace reader (blocked)", expected: gopeek.StateOther},
		{input: "finalizer wait", expected: gopeek.StateOther},
		{input: "timer goroutine (idle)", expected: gopeek.StateOther},
	}
	for _, ts := range tests {
		if s := gopeek.NewState(ts.input); s != ts.expected {
			t.Errorf("expected: %d, actual: %d", int(ts.expected), int(s))
		}
	}
}

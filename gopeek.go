// Package gopeek peeks goroutines and helps gophers cope with those
// in intuitive ways such as waiting for some goroutines
// to change its state into waiting a lock primitive and so on.
//
// It is mostly useful on test code now using time.Sleep to yield
// other goroutines a moment to make something happen.
// Such tests tend to take long if time.Sleep invoked many times.
// With gopeek you can do within the order of magnitude less time.
package gopeek

import (
	"bytes"
	"errors"
	"io/ioutil"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/maruel/panicparse/stack"
)

type (
	// FilterByGo returns true if a goroutine passed satisfies a condition
	// implemented in this func or false otherwise.
	FilterByGo func(*stack.Goroutine) bool

	// FilterByGoes returns true if goroutines passed satisfies a condition
	// implemented in this func or false otherwise.
	FilterByGoes func([]stack.Goroutine) bool

	// Condition provides the way to describe what/how many goroutines exist and
	// what state they are by using built-in|user-defined filters and
	// retrieve goroutines that satisfy all filters or wait until there is
	// at least one goroutine that satisfy all filters.
	Condition struct {
		filters []interface{}
		buf     []byte
	}

	// State represents a state of a goroutine based on G's waitreason.
	State int

	// Option configures a Condition.
	Option func(*Condition)
)

const (
	// StateIdle means a goroutine in idle.
	StateIdle State = iota
	// StateRunnable means a goroutine in runnable.
	StateRunnable
	// StateRunning means a goroutine in running.
	StateRunning
	// StateSysCall means a goroutine in calling a syscall.
	StateSysCall
	// StateWaiting means a goroutine in waiting.
	StateWaiting
	// StateDead means a goroutine in dead.
	StateDead
	// StateEnqueue means a goroutine in enqueue.
	StateEnqueue
	// StateCopyStack means a goroutine in copystack.
	StateCopyStack
	// StateSleeping means a goroutine blocked due to sleeping (time.Sleep).
	StateSleeping
	// StateWaitingChannel means a goroutine blocked due to waiting a channel.
	StateWaitingChannel
	// StateWaitingSelect means a goroutine blocked in a select clause.
	StateWaitingSelect
	// StateWaitingGCActivity means a goroutine blocked due to some GC activity.
	StateWaitingGCActivity
	// StateWaitingIO means a goroutine blocked due to network I/O.
	StateWaitingIO
	// StateWaitingLock means a goroutine blocked due to a lock primitive.
	StateWaitingLock
	// StateOther means a goroutine blocked due to some other reason.
	StateOther
)

// Strings can be set to G's waitreason found by
// - listed in runtime/traceback.go
// - grep by gopark(|unlock)\( and waitreason
// on release-branch.go1.[6-7] and master (af67f7de3f7b0d26f95d813022f876eef1fa3889)
// to be used for identifying a state of a goroutine.
//
// strWaiting(Lock|Channel|Select|GCActivity1) used as prefix match.
// strWaitingGCActivity2 used as sub-string match.
// others used as perfect match.
const (
	strIdle               string = "idle"
	strRunnable           string = "runnable"
	strRunning            string = "running"
	strSysCall            string = "syscall"
	strWaiting            string = "waiting"
	strDead               string = "dead"
	strEnqueue            string = "enqueue"
	strCopyStack          string = "copystack"
	strSleeping           string = "sleep"
	strWaitingChannel     string = "chan"
	strWaitingSelect      string = "select"
	strWaitingGCActivity1 string = "gc "
	strWaitingGCActivity2 string = "garbage"
	strWaitingIO          string = "IO wait"
	strWaitingLock        string = "sem"
)

const (
	defaultFilterSize = 10
	defaultBufSize    = 1 << 20
)

var (
	// ErrTimeout indicates timeout happened while calling Wait.
	ErrTimeout = errors.New("Timeout occurred while waiting")
)

// WithFilterSize returns an Option for gopeek.NewCondition
// that provides a initial filter size. Setting this value is meory efficient
// if how many filters you will use is determined beforehand.
func WithFilterSize(fs int) Option {
	return func(c *Condition) {
		c.filters = make([]interface{}, 0, fs)
	}
}

// WithBufSize returns an Option for gopeek.NewCondition
// that provides a initial buffer size used for storing data
// returned from runtime.Stack. In most cases the default(1M) is sufficient.
func WithBufSize(bs int) Option {
	return func(c *Condition) {
		c.buf = make([]byte, bs)
	}
}

// NewCondition returns a new Condition that filters goroutines
// based on built-in|used-defined filters added later.
func NewCondition(opts ...Option) *Condition {
	c := &Condition{}
	for _, opt := range opts {
		opt(c)
	}
	if c.filters == nil {
		c.filters = make([]interface{}, 0, defaultFilterSize)
	}
	if c.buf == nil {
		c.buf = make([]byte, defaultBufSize)
	}

	return c
}

// FilterByGo adds a user-defined FilterByGo filter.
// It returns Condition itself for method chaining.
func (c *Condition) FilterByGo(f FilterByGo) *Condition {
	c.filters = append(c.filters, f)
	return c
}

// FilterByGoes adds a user-defined FilterByGoes filter.
// It returns Condition itself for method chaining.
func (c *Condition) FilterByGoes(f FilterByGoes) *Condition {
	c.filters = append(c.filters, f)
	return c
}

// GT adds a FilterByGoes filter that return true if len(goroutines) > v
// or false otherwise.
// It returns Condition itself for method chaining.
func (c *Condition) GT(v int) *Condition {
	f := func(gs []stack.Goroutine) bool {
		return len(gs) > v
	}
	c.filters = append(c.filters, FilterByGoes(f))
	return c
}

// LT adds a FilterByGoes filter that return true if len(goroutines) < v
// or false otherwise.
// It returns Condition itself for method chaining.
func (c *Condition) LT(v int) *Condition {
	f := func(gs []stack.Goroutine) bool {
		return len(gs) < v
	}
	c.filters = append(c.filters, FilterByGoes(f))
	return c
}

// EQ adds a FilterByGoes filter that return true if len(goroutines) == v
// or false otherwise.
// It returns Condition itself for method chaining.
func (c *Condition) EQ(v int) *Condition {
	f := func(gs []stack.Goroutine) bool {
		return len(gs) == v
	}
	c.filters = append(c.filters, FilterByGoes(f))
	return c
}

// CreatedBy adds a FilterByGo filter that return true
// if a goroutine's Signature.CreatedBy.Func.PkgDotName() matches fun regexp
// or false otherwise.
// It returns Condition itself for method chaining.
// Panic happens if regexp failed to compile fun.
func (c *Condition) CreatedBy(fun string) *Condition {
	re := regexp.MustCompile(fun)
	f := func(g *stack.Goroutine) bool {
		return re.Match([]byte(g.Signature.CreatedBy.Func.PkgDotName()))
	}
	c.filters = append(c.filters, FilterByGo(f))
	return c
}

// Is adds a FilterByGo filter that return true if a goroutine's state == state
// or false otherwise.
// It returns Condition itself for method chaining.
func (c *Condition) Is(state State) *Condition {
	f := func(g *stack.Goroutine) bool {
		cur := NewState(g.State)
		return state == cur
	}
	c.filters = append(c.filters, FilterByGo(f))
	return c
}

// Not adds a FilterByGo filter that return true if a goroutine's state != state
// or false otherwise.
// It returns Condition itself for method chaining.
func (c *Condition) Not(state State) *Condition {
	f := func(g *stack.Goroutine) bool {
		cur := NewState(g.State)
		return state != cur
	}
	c.filters = append(c.filters, FilterByGo(f))
	return c
}

// In adds a FilterByGo filter that return true
// if a goroutine's state is included in states or false otherwise.
// It returns Condition itself for method chaining.
func (c *Condition) In(states ...State) *Condition {
	f := func(g *stack.Goroutine) bool {
		cur := NewState(g.State)
		for _, s := range states {
			if cur == s {
				return true
			}
		}
		return false
	}
	c.filters = append(c.filters, FilterByGo(f))
	return c
}

// Eval retrieves all goroutines that currently exist and apply all filters.
// It returns goroutines that satisfy all filter's conditions
// or nil otherwise and error when stack.ParseDump failed.
func (c *Condition) Eval() ([]stack.Goroutine, error) {
	var n int
	for {
		n = runtime.Stack(c.buf, true)
		if n == len(c.buf) {
			// may need more buf
			c.buf = make([]byte, n*2)
			continue
		}
		break
	}
	buf := c.buf[:n]
	gs, err := stack.ParseDump(bytes.NewReader(buf), ioutil.Discard)
	if err != nil {
		return nil, err
	}
	// goroutines applied a FilterByGo
	ngs := make([]stack.Goroutine, 0, len(gs))
	for _, f := range c.filters {
		switch f.(type) {
		case FilterByGo:
			// reset ngs for reuse
			ngs := ngs[:0]
			for _, g := range gs {
				if f.(FilterByGo)(&g) {
					ngs = append(ngs, g)
				}
			}
			if len(ngs) == 0 {
				// no chance to satisfy the condition
				return nil, nil
			}
			// update gs to the filtered ngs
			gs = ngs
		case FilterByGoes:
			if !f.(FilterByGoes)(gs) {
				// no chance to satisfy the condition
				return nil, nil
			}
		}
	}
	return gs, nil
}

// Wait calls Eval repeatedly until Eval returns at least one goroutine
// or error or timeout passed.
// It returns goroutines that satisfy all filter's conditions or nil otherwise
// and error when stack.ParseDump failed or timeout happened.
func (c *Condition) Wait(timeout time.Duration) ([]stack.Goroutine, error) {
	start := time.Now()
	for {
		gs, err := c.Eval()
		if err != nil {
			return nil, err
		}
		if len(gs) > 0 {
			return gs, nil
		}
		if timeout > 0 && time.Now().Sub(start) > timeout {
			return nil, ErrTimeout
		}
		runtime.Gosched()
	}
}

// NewState returns a new State based on state.
func NewState(state string) State {
	switch state {
	case strIdle:
		return StateIdle
	case strRunnable:
		return StateRunnable
	case strRunning:
		return StateRunning
	case strSysCall:
		return StateSysCall
	case strWaiting:
		return StateWaiting
	case strDead:
		return StateDead
	case strEnqueue:
		return StateEnqueue
	case strCopyStack:
		return StateCopyStack
	case strSleeping:
		return StateSleeping
	case strWaitingIO:
		return StateWaitingIO
	default:
		if strings.HasPrefix(state, strWaitingLock) {
			return StateWaitingLock
		} else if strings.HasPrefix(state, strWaitingChannel) {
			return StateWaitingChannel
		} else if strings.HasPrefix(state, strWaitingSelect) {
			return StateWaitingSelect
		} else if str := strings.ToLower(state); strings.HasPrefix(str, strWaitingGCActivity2) ||
			strings.Contains(str, strWaitingGCActivity1) {
			return StateWaitingGCActivity
		}
	}
	return StateOther
}

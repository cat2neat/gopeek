gopeek
======

[![Go Report Card](https://goreportcard.com/badge/cat2neat/gopeek)](https://goreportcard.com/report/cat2neat/gopeek) [![Build Status](https://travis-ci.org/cat2neat/gopeek.svg?branch=master)](https://travis-ci.org/cat2neat/gopeek) [![Coverage Status](https://coveralls.io/repos/github/cat2neat/gopeek/badge.svg?branch=master)](https://coveralls.io/github/cat2neat/gopeek?branch=master)

Peeks goroutines and helps gophers cope with those in intuitive ways
such as waiting for some goroutines
to change its state into waiting a lock primitive and so on.

It is mostly useful on test code now using time.Sleep to yield
other goroutines a moment to make something happen.
Such tests tend to take long if time.Sleep invoked many times.
With gopeek you can do within the order of magnitude less time.

https://github.com/uber-go/ratelimit/pull/1#discussion-diff-78767601
motivated me.

Example
-------

```go
import "github.com/cat2neat/gopeek"
import "github.com/maruel/panicparse/stack"

// Wait for goroutines
// - created by the func in "github.com/cat2neat/gopeek/.*" (Regex can be used)
// - locked at a M (Any fields in stack.Gorotine can be used at user-defined)
// - blocked by channel primitives
// - the number of goroutines which satisfy the above conditions == 3
// Return goroutines that satisfy the above all conditions or
// Timeout after time.Millisecond passed if no goroutines satisfy such.
gopeek.NewCondition().
       CreatedBy("github.com/cat2neat/gopeek/.*").
       FilterByGo(func(g stack.Gorotine) bool {
          return g.Signature.Locked
       }).
       Is(gopeek.StateWaitingChannel).
       EQ(3).
       Wait(time.Millisecond)

// Wait for goroutines
// - created by the func in "github.com/cat2neat/gopeek/.*"
// - blocked by I/O (net poller) or Lock caused by sync primitives or time.Sleep
// - the number of goroutines which satisfy the above conditions == 3 or >= 6
// Return goroutines that satisfy the above all conditions or
// Timeout after time.Millisecond passed if no goroutines satisfy such.
gopeek.NewCondition().
       CreatedBy("github.com/cat2neat/gopeek/.*").
       In(gopeek.StateWaitingIO, gopeek.StateWaitingLock, gopeek.StateSleeping).
       FilterByGoes(func(gs []stack.Gorotine) bool {
          return len(gs) == 3 || len(gs) >= 6
       }).
       Wait(time.Millisecond)
```

Install
-------

```shell
go get -u github.com/cat2neat/gopeek
```

Features
--------
- Wait for goroutines to satisfy specified conditions
- Retrieve goroutines that satisfy specified conditions

with built-in filters and|or user-defined ones.

All filters are lazy evaluated when (Wait|Eval) called.

Usage
-----
See the above example code or gopeek_test.go for the actual use.

Cautions
--------
Don't use gopeek except in test code.
You should be wrong if you will try to solve a real concurrency problem with gopeek.

package main

import (
	"errors"
	"fmt"
	"log"
	"sync"
)

type CoState uint32

const (
	Suspended CoState = iota // yield or not start
	Running
	Dead // finished or stopped with an error
)

func (s CoState) String() string {
	switch s {
	case Suspended:
		return "Suspended"
	case Running:
		return "Running"
	case Dead:
		return "Dead"
	default:
		return "Unknown"
	}
}

type Coroutine struct {
	pool   *CoPool
	waitIn chan func()
	runner func()
	state  CoState
}

// 1. 默认参数nil，唤醒co.runner中上次挂起的f
// 2. 将f传入co.runner中并运行，直到f执行完或挂起
func (co *Coroutine) run(f func()) error {
	co.pool.runCo = co
	defer func() {
		co.pool.runCo = nil
	}()
	if co.state == Dead {
		return errors.New("cannot resume dead coroutine")
	}
	if co.state != Suspended {
		return fmt.Errorf("resume not suspended coroutine %s", co.state)
	}
	co.state = Running
	if co.runner == nil {
		co.runner = func() {
			defer func() {
				if e := recover(); e != nil {
					log.Printf("co running err, %v\n", e)
					if co.state == Running {
						co.pool.notifyOut <- fmt.Errorf("%v", e)
					}
				}
				co.state = Dead
			}()

			f()
			co.state = Suspended
			co.pool.notifyOut <- nil
			for {
				f = nil
				if !co.pool.put(co) {
					return
				}
				f = <-co.waitIn
				f()
				co.state = Suspended
				co.pool.notifyOut <- nil
			}
		}
		go co.runner()
	} else {
		co.waitIn <- f
	}

	return <-co.pool.notifyOut
}

// 唤醒co.runner中上次挂起的f
func (co *Coroutine) Resume() error {
	return co.run(nil)
}

// 将当前co设为挂起状态，唤醒外部Resume调用处，并执行f，完成后挂起，等待外部再次Resume才能唤醒
// Notice: f中不应该再次调用Yield，会触发panic
func (co *Coroutine) Yield(f func()) {
	if co.state != Running {
		panic("cannot yield Suspended coroutine")
	}
	co.state = Suspended
	co.pool.notifyOut <- nil
	f()
	<-co.waitIn
}

type CoPool struct {
	stack     []*Coroutine
	size      int
	cap       int
	mu        sync.RWMutex
	notifyOut chan error
	runCo     *Coroutine
}

func (p *CoPool) get() (co *Coroutine) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.size > 0 {
		top := p.size - 1
		co = p.stack[top]
		p.stack[top] = nil
		p.size--
	} else {
		co = &Coroutine{
			pool:   p,
			waitIn: make(chan func()),
			state:  Suspended,
		}
	}
	return
}

func (p *CoPool) put(co *Coroutine) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.size >= p.cap {
		return false
	}
	p.stack[p.size] = co
	p.size++
	return true
}

func (p *CoPool) RunningCo() *Coroutine {
	return p.runCo
}

func (p *CoPool) Run(f func()) error {
	co := p.get()
	return co.run(f)
}

func NewPool(cap int) *CoPool {
	p := &CoPool{
		cap:       cap,
		stack:     make([]*Coroutine, cap),
		size:      0,
		notifyOut: make(chan error, 1),
	}
	return p
}

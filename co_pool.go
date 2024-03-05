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

func (co *Coroutine) Resume(f func(), notifyOut chan error) error {
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
						notifyOut <- fmt.Errorf("%v", e)
					}
				}
				co.state = Dead
			}()

			f()
			notifyOut <- nil
			for {
				f = nil
				co.state = Suspended
				if !co.pool.Put(co) {
					return
				}
				f = <-co.waitIn
				f()
				notifyOut <- nil
			}
		}
		go co.runner()
	} else {
		co.waitIn <- f
	}

	return <-notifyOut
}

func (co *Coroutine) Yield() {

}

type CoPool struct {
	stack []*Coroutine
	size  int
	cap   int
	mu    sync.RWMutex
}

func (p *CoPool) Get() (co *Coroutine) {
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

func (p *CoPool) Put(co *Coroutine) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.size >= p.cap {
		return false
	}
	p.stack[p.size] = co
	p.size++
	return true
}

func NewCoPool(cap int) *CoPool {
	p := &CoPool{
		cap:   cap,
		stack: make([]*Coroutine, cap),
		size:  0,
	}
	return p
}

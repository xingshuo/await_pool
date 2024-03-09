package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
)

type CoState uint32

const (
	Idle CoState = iota
	Running
	Suspended // yield
	Dead      // finished or stopped with an error
)

func (s CoState) String() string {
	switch s {
	case Idle:
		return "Idle"
	case Running:
		return "Running"
	case Suspended:
		return "Suspended"
	case Dead:
		return "Dead"
	default:
		return "Unknown"
	}
}

type Coroutine struct {
	pool             *CoPool
	waitIn           chan func()
	runner           func()
	state            CoState
	completedTaskNum int
}

// 1. 默认参数nil，唤醒co.runner中上次挂起的f
// 2. 将f传入co.runner中并运行，直到f执行完或挂起
func (co *Coroutine) run(f func()) error {
	co.pool.runCo = co
	defer func() {
		co.pool.runCo = nil
	}()
	if f != nil { // Run
		if co.state != Idle {
			return fmt.Errorf("run not idle coroutine %s", co.state)
		}
	} else { // Resume
		if co.state != Suspended {
			return fmt.Errorf("resume not suspended coroutine %s", co.state)
		}
	}
	co.state = Running
	pstat := co.pool.stat
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
				if pstat != nil {
					atomic.AddInt64(&pstat.alive, -1)
				}
			}()

			if pstat != nil {
				atomic.AddInt64(&pstat.total, 1)
				atomic.AddInt64(&pstat.alive, 1)
			}
			f()
			co.state = Idle
			co.completedTaskNum++
			co.pool.notifyOut <- nil
			for {
				f = nil
				if !co.pool.put(co) {
					return
				}
				f = <-co.waitIn
				f()
				co.state = Idle
				co.completedTaskNum++
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
	pstat := co.pool.stat
	if pstat != nil {
		defer atomic.AddInt64(&pstat.suspend, -1)
		atomic.AddInt64(&pstat.suspend, 1)
	}
	co.state = Suspended
	co.pool.notifyOut <- nil
	f()
	<-co.waitIn
}

type CoStat struct {
	total   int64
	alive   int64
	suspend int64
}

type CoPool struct {
	stack            []*Coroutine
	size             int
	cap              int
	mu               sync.RWMutex
	notifyOut        chan error
	runCo            *Coroutine
	stat             *CoStat
	coResetThreshold int
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
			pool:             p,
			waitIn:           make(chan func()),
			state:            Idle,
			completedTaskNum: 0,
		}
	}
	return
}

func (p *CoPool) put(co *Coroutine) bool {
	if co.completedTaskNum >= p.coResetThreshold {
		return false
	}
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

func (p *CoPool) Stat() string {
	pstat := p.stat
	if pstat == nil {
		return ""
	}
	total := atomic.LoadInt64(&pstat.total)
	alive := atomic.LoadInt64(&pstat.alive)
	suspend := atomic.LoadInt64(&pstat.suspend)
	p.mu.RLock()
	idle := int64(p.size)
	p.mu.RUnlock()
	running := alive - idle - suspend
	var dot strings.Builder
	dot.WriteString(fmt.Sprintf("===CoPool Stat, Cap[%d]:===\n", p.cap))
	dot.WriteString(fmt.Sprintf("Total Count: %d\n", total))
	dot.WriteString(fmt.Sprintf("Alive Count: %d\n", alive))
	dot.WriteString(fmt.Sprintf("Running Count: %d\n", running))
	dot.WriteString(fmt.Sprintf("Suspend Count: %d\n", suspend))
	dot.WriteString(fmt.Sprintf("Idle Count: %d\n", idle))
	return dot.String()
}

// [cap]协程池容量：每次Coroutine执行完f，若当前协程池协程数量 < cap，会放回协程池，否则自动释放
// [coResetThreshold]每个Coroutine执行任务数量上限：参考: https://github.com/grpc/grpc-go/blob/master/server.go#L614
// [openStat]是否开启协程状态统计，默认关闭
func NewPool(cap, coResetThreshold int, openStat ...bool) *CoPool {
	p := &CoPool{
		cap:              cap,
		stack:            make([]*Coroutine, cap),
		size:             0,
		notifyOut:        make(chan error, 1),
		coResetThreshold: coResetThreshold,
	}
	if len(openStat) > 0 && openStat[0] {
		p.stat = &CoStat{}
	}
	return p
}

package main

import (
	"fmt"
	"log"
	"sync/atomic"
)

type QueueMsg struct {
	wakeup func()
	call   func()
}

type Service interface {
	Enqueue(func())
	Run()
	Await(func())
	Quit()
}

type BaseService struct {
	queue    chan *QueueMsg
	suspend  chan error
	quit     chan struct{}
	quitFlag bool
}

func (s BaseService) Enqueue(f func()) {
	s.queue <- &QueueMsg{call: f}
}

func (s *BaseService) Quit() {
	s.quitFlag = true
	close(s.quit)
}

const NilGoSeq uint64 = 0

type GoService struct {
	BaseService
	allocSeq uint64
	runSeq   uint64
	flag     *int
}

func (s *GoService) Await(call func()) {
	curSeq := atomic.LoadUint64(&s.runSeq)
	s.suspend <- nil
	call()
	//TODO: use sync.Pool??
	c := make(chan struct{})
	s.queue <- &QueueMsg{
		wakeup: func() {
			atomic.StoreUint64(&s.runSeq, curSeq)
			c <- struct{}{}
		},
	}
	<-c
}

func (s *GoService) Run() {
	for !s.quitFlag {
		select {
		case msg := <-s.queue:
			if msg.wakeup != nil {
				msg.wakeup()
			} else {
				go s.handleMsg(msg.call)
			}
			<-s.suspend
			atomic.StoreUint64(&s.runSeq, NilGoSeq)
		case <-s.quit:
			break
		}
	}

	for {
		select {
		case msg := <-s.queue:
			if msg.wakeup != nil {
				msg.wakeup()
			} else {
				go s.handleMsg(msg.call)
			}
			<-s.suspend
			atomic.StoreUint64(&s.runSeq, NilGoSeq)
		default:
			return
		}
	}
}

func (s *GoService) handleMsg(call func()) {
	curSeq := atomic.AddUint64(&s.allocSeq, 1)
	if curSeq == NilGoSeq {
		curSeq = atomic.AddUint64(&s.allocSeq, 1)
	}
	atomic.StoreUint64(&s.runSeq, curSeq)
	s.flag = new(int)
	defer func() {
		if e := recover(); e != nil {
			log.Printf("go running err, %v\n", e)
			if curSeq == atomic.LoadUint64(&s.runSeq) {
				s.suspend <- fmt.Errorf("%v", e)
			}
		} else {
			s.suspend <- nil
		}
	}()
	call()
}

type CoService struct {
	BaseService
	pool  *CoPool
	runCo *Coroutine
}

func (s *CoService) Await(call func()) {
	co := s.runCo
	s.suspend <- nil
	co.state = Suspended
	call()
	s.queue <- &QueueMsg{
		wakeup: func() {
			s.runCo = co
			err := co.Resume(nil, s.suspend)
			s.runCo = nil
			if err != nil {
				log.Printf("handle msg call err, %v\n", err)
			}
		},
	}
	<-co.waitIn
}

func (s *CoService) Run() {
	for !s.quitFlag {
		select {
		case msg := <-s.queue:
			if msg.wakeup != nil {
				msg.wakeup()
			} else {
				co := s.pool.Get()
				s.runCo = co
				err := co.Resume(msg.call, s.suspend)
				s.runCo = nil
				if err != nil {
					log.Printf("handle msg call err, %v\n", err)
				}
			}
		case <-s.quit:
			break
		}
	}

	for {
		select {
		case msg := <-s.queue:
			if msg.wakeup != nil {
				msg.wakeup()
			} else {
				co := s.pool.Get()
				s.runCo = co
				err := co.Resume(msg.call, s.suspend)
				s.runCo = nil
				if err != nil {
					log.Printf("handle msg call err, %v\n", err)
				}
			}
		default:
			return
		}
	}
}

func NewService(qsize, psize int) (s Service) {
	if psize > 0 {
		s = &CoService{
			BaseService: BaseService{
				queue:   make(chan *QueueMsg, qsize),
				suspend: make(chan error, 1),
				quit:    make(chan struct{}),
			},
			pool: NewCoPool(psize),
		}
	} else {
		s = &GoService{
			BaseService: BaseService{
				queue:   make(chan *QueueMsg, qsize),
				suspend: make(chan error, 1),
				quit:    make(chan struct{}),
			},
		}
	}
	return
}

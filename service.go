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

type GoService struct {
	BaseService
	goSeq uint64
}

func (s *GoService) Await(call func()) {
	curSeq := atomic.LoadUint64(&s.goSeq)
	s.suspend <- nil
	call()
	//TODO: use sync.Pool??
	c := make(chan struct{})
	s.queue <- &QueueMsg{
		wakeup: func() {
			atomic.StoreUint64(&s.goSeq, curSeq)
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
		default:
			return
		}
	}
}

func (s *GoService) handleMsg(call func()) {
	curSeq := atomic.AddUint64(&s.goSeq, 1)
	defer func() {
		if e := recover(); e != nil {
			log.Printf("go running err, %v\n", e)
			if curSeq != atomic.LoadUint64(&s.goSeq) {
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
		cs := &CoService{
			BaseService: BaseService{
				queue:   make(chan *QueueMsg, qsize),
				suspend: make(chan error, 1),
				quit:    make(chan struct{}),
			},
		}
		cs.pool = NewCoPool(psize)
		s = cs
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

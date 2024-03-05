package main

import (
	"fmt"
	"log"
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

type GoCtx struct {
	Running bool
}
type GoService struct {
	BaseService
	runCtx  *GoCtx
	suspend chan error
}

func (s *GoService) Await(call func()) {
	s.runCtx.Running = false
	ctx := s.runCtx
	s.suspend <- nil
	call()
	//TODO: use sync.Pool??
	c := make(chan struct{})
	s.queue <- &QueueMsg{
		wakeup: func() {
			c <- struct{}{}
		},
	}
	<-c
	ctx.Running = true
	s.runCtx = ctx
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
			s.runCtx = nil
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
			s.runCtx = nil
		default:
			return
		}
	}
}

func (s *GoService) handleMsg(call func()) {
	s.runCtx = &GoCtx{
		Running: true,
	}
	ctx := s.runCtx
	defer func() {
		if e := recover(); e != nil {
			log.Printf("go running err, %v\n", e)
			if ctx.Running {
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
	co.Yield(func() {
		call()
		s.queue <- &QueueMsg{
			wakeup: func() {
				s.runCo = co
				err := co.Resume(nil)
				s.runCo = nil
				if err != nil {
					log.Printf("handle msg call err, %v\n", err)
				}
			},
		}
	})
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
				err := co.Resume(msg.call)
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
				err := co.Resume(msg.call)
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
				queue: make(chan *QueueMsg, qsize),
				quit:  make(chan struct{}),
			},
			pool: NewCoPool(psize),
		}
	} else {
		s = &GoService{
			BaseService: BaseService{
				queue: make(chan *QueueMsg, qsize),
				quit:  make(chan struct{}),
			},
			suspend: make(chan error, 1),
		}
	}
	return
}

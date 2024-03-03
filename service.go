package main

import (
	"fmt"
	"log"
)

type QueueMsg struct {
	wakeup func()
	call   func()
}

type Service struct {
	queue    chan *QueueMsg
	suspend  chan error
	quit     chan struct{}
	quitFlag bool
	pool     *CoPool
	runCo    *Coroutine
}

func (s *Service) Await(call func()) {
	if s.pool != nil {
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
	} else {
		s.suspend <- nil
		func() {
			defer func() {
				if e := recover(); e != nil {
					log.Printf("go running err: %v\n", e)
				}
			}()
			call()
		}()
		c := make(chan struct{})
		s.queue <- &QueueMsg{
			wakeup: func() {
				c <- struct{}{}
			},
		}
		<-c
	}
}

func (s *Service) handleMsg(call func()) {
	defer func() {
		if e := recover(); e != nil {
			log.Printf("go running err, %v\n", e)
			//TODO: 只有框架正在处理的go发生异常需要唤醒suspend
			s.suspend <- fmt.Errorf("%v", e)
		} else {
			s.suspend <- nil
		}
	}()
	call()
}

func (s *Service) Enqueue(call func()) {
	s.queue <- &QueueMsg{
		call: call,
	}
}

func (s *Service) Run() {
	for !s.quitFlag {
		select {
		case msg := <-s.queue:
			if s.pool != nil {
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
			} else {
				if msg.wakeup != nil {
					msg.wakeup()
				} else {
					go s.handleMsg(msg.call)
				}
				<-s.suspend
			}
		case <-s.quit:
			break
		}
	}

	for {
		select {
		case msg := <-s.queue:
			if s.pool != nil {
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
			} else {
				if msg.wakeup != nil {
					msg.wakeup()
				} else {
					go s.handleMsg(msg.call)
				}
				<-s.suspend
			}
		default:
			return
		}
	}
}

func (s *Service) Quit() {
	s.quitFlag = true
	close(s.quit)
}

func NewService(qsize, psize int) *Service {
	s := &Service{
		queue:   make(chan *QueueMsg, qsize),
		suspend: make(chan error, 1),
		quit:    make(chan struct{}),
	}
	if psize > 0 {
		s.pool = NewCoPool(psize)
	}
	return s
}

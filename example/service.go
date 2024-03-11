package main

import (
	"log"

	"github.com/xingshuo/await_pool"
)

type QueueMsg struct {
	wakeup func()
	call   func()
}

type CoService struct {
	queue    chan *QueueMsg
	quit     chan struct{}
	quitFlag bool
	coPool   *await_pool.CoPool
}

func (s CoService) Enqueue(f func()) {
	s.queue <- &QueueMsg{call: f}
}

func (s *CoService) Await(call func()) {
	co := s.coPool.RunningCo()
	co.Yield(func() {
		call()
		s.queue <- &QueueMsg{
			wakeup: func() {
				err := co.Resume()
				if err != nil {
					log.Printf("resume msg call err, %v\n", err)
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
				err := s.coPool.Run(msg.call)
				if err != nil {
					log.Printf("run msg call err, %v\n", err)
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
				err := s.coPool.Run(msg.call)
				if err != nil {
					log.Printf("handle msg call err, %v\n", err)
				}
			}
		default:
			return
		}
	}
}

func (s *CoService) Quit() {
	s.quitFlag = true
	close(s.quit)
}

func (s *CoService) Stat() {
	stat := s.coPool.Stat()
	if len(stat) > 0 {
		log.Println(stat)
	}
}

type Service interface {
	Enqueue(func())
	Run()
	Await(func())
	Quit()
	Stat()
}

func NewService(qsize, psize, coThreshold int, openStat bool) Service {
	s := &CoService{
		queue:  make(chan *QueueMsg, qsize),
		quit:   make(chan struct{}),
		coPool: await_pool.NewPool(psize, coThreshold, openStat),
	}
	return s
}

package main

import (
	"flag"
	"log"
	"time"
)

var (
	poolSize         int
	coResetThreshold int
	queueSize        int
	poolStat         bool
)

func main() {
	flag.IntVar(&queueSize, "queue", 10, "queue size, default 10")
	flag.IntVar(&poolSize, "pool", 3, "pool size, default 3")
	flag.IntVar(&coResetThreshold, "threshold", 2, "co reset threshold, default 2")
	flag.BoolVar(&poolStat, "stat", false, "open pool stat, default false")
	flag.Parse()

	s := NewService(queueSize, poolSize, coResetThreshold, poolStat)
	f1 := func() {
		log.Println("begin run f1")
		s.Stat()
		s.Await(func() {
			log.Println("before f1 rpc")
			s.Stat()
			time.Sleep(time.Second * 2)
			log.Println("after f1 rpc")
			s.Stat()
		})
		log.Println("end run f1")
		s.Stat()
	}
	f2 := func() {
		log.Println("begin run f2")
		s.Stat()
		log.Println("end run f2")
	}
	f3 := func() {
		log.Println("begin run f3")
		s.Stat()
		s.Await(func() {
			log.Println("before f3 rpc")
			s.Stat()
			time.Sleep(time.Second * 4)
			log.Println("after f3 rpc")
			s.Stat()
		})
		log.Println("end run f3 && call Quit")
		s.Stat()
		s.Quit()
	}
	f4 := func() {
		log.Println("begin run f4")
		s.Stat()
		log.Println("end run f4")
	}

	var ptr *int
	f5 := func() {
		log.Println("begin run f5")
		s.Stat()
		*ptr = 100
		log.Println("end run f5")
	}

	f6 := func() {
		log.Println("begin run f6")
		s.Stat()
		s.Await(func() {
			log.Println("before f6 rpc")
			s.Stat()
			time.Sleep(time.Second * 3)
			log.Println("after f6 rpc")
			s.Stat()
			*ptr = 200
		})
		log.Println("end run f6")
	}

	s.Enqueue(f1)
	s.Enqueue(f2)
	s.Enqueue(f3)
	s.Enqueue(f4)
	s.Enqueue(f5)
	s.Enqueue(f6)

	log.Printf("----enter service dispatch pool size:%d----\n", poolSize)
	s.Run()
	s.Stat()
	log.Println("----quit service dispatch----")
}

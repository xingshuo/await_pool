package main

import (
	"flag"
	"log"
	"time"
)

func main() {
	var poolSize int
	flag.IntVar(&poolSize, "pool", 0, "pool size, default 0")
	flag.Parse()
	s := NewService(10, poolSize)
	f1 := func() {
		log.Println("begin run f1")
		s.Await(func() {
			log.Println("before f1 rpc")
			time.Sleep(time.Second * 2)
			log.Println("after f1 rpc")
		})
		log.Println("end run f1")
	}
	f2 := func() {
		log.Println("begin run f2")
		log.Println("end run f2")
	}
	f3 := func() {
		log.Println("begin run f3")
		s.Await(func() {
			log.Println("before f3 rpc")
			time.Sleep(time.Second * 4)
			log.Println("after f3 rpc")
		})
		log.Println("end run f3 && call Quit")
		s.Quit()
	}
	f4 := func() {
		log.Println("begin run f4")
		log.Println("end run f4")
	}

	var ptr *int
	f5 := func() {
		log.Println("begin run f5")
		*ptr = 100
		log.Println("end run f5")
	}

	f6 := func() {
		log.Println("begin run f6")
		s.Await(func() {
			log.Println("before f6 rpc")
			time.Sleep(time.Second * 3)
			log.Println("after f6 rpc")
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
	log.Println("----quit service dispatch----")
}

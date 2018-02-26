package main

import (
	"fmt"
	"sync"
)

func main() {
	people := []string{"Anna", "Bob", "Cody", "Dave", "Eva"}
	match := make(chan string, 1) // 为一个未匹配的发送操作提供空间
	wg := new(sync.WaitGroup)
	wg.Add(len(people))
	for _, name := range people {
		go Seek(name, match, wg)
	}
	wg.Wait()
	select {
	case name := <-match:
		fmt.Printf("No one received %s’s message.\n", name)
	default:
		// 没有待处理的发送操作
	}
}

// Seek 发送一个name到match管道或从match管道接收一个peer，结束时通知wait group
func Seek(name string, match chan string, wg *sync.WaitGroup) {
	select {
	case peer := <-match:
		fmt.Printf("%s sent a message to %s.\n", peer, name)
	case match <- name:
		// 等待某个goroutine接收我的消息
	}
	wg.Done()
}

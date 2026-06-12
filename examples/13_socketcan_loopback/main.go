// 运行: go run ./examples/13_socketcan_loopback
// 前置: Linux + scripts/setup-vcan.sh up vcan0
//
// 演示 WithLoopback(true) + WithRecvOwnMsgs(true) 的组合：
// 单 vcan 接口上自发自收，验证收发链路无需外部硬件即可做回归测试。

//go:build linux

package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Crush251/gocan"
)

const totalFrames = 100

func main() {
	bus, err := gocan.Open(
		gocan.SocketCAN("vcan0"),
		gocan.WithLoopback(true),    // 默认就是 true，这里显式声明
		gocan.WithRecvOwnMsgs(true), // 关键：让本 socket 收到自己发的帧
		gocan.WithRxBufferSize(totalFrames*2),
	)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	got := make(map[uint32]int)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for i := 0; i < totalFrames; i++ {
			f, err := bus.ReadOne(ctx)
			if err != nil {
				log.Printf("read[%d]: %v", i, err)
				return
			}
			got[f.ID]++
		}
	}()

	for i := 0; i < totalFrames; i++ {
		f, _ := gocan.NewFrame(uint32(0x100+i), []byte{byte(i)})
		if err := bus.Send(context.Background(), f); err != nil {
			log.Fatalf("send[%d]: %v", i, err)
		}
	}
	wg.Wait()

	pass := true
	for i := 0; i < totalFrames; i++ {
		if got[uint32(0x100+i)] != 1 {
			log.Printf("missing 0x%X (count=%d)", 0x100+i, got[uint32(0x100+i)])
			pass = false
		}
	}
	if pass {
		log.Printf("PASS: %d frames sent and received", totalFrames)
	} else {
		log.Printf("FAIL: see missing frames above")
	}

	// 试试看：把 WithRecvOwnMsgs(true) 注释掉重跑，
	// 你会看到 ReadOne 在 5 秒后超时——本 socket 不会收到自己发的帧。
}

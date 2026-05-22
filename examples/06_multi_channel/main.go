// 运行: go run ./examples/06_multi_channel
// 前置: 同时连接两路 PCAN-USB（USBBus1 + USBBus2）
//
// 演示在同一进程里独立操作多个 CAN 通道：
// 每个 Bus 各自拥有 reader goroutine，互不影响。

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"

	"github.com/Crush251/pcanbasic_go"
)

func main() {
	bus1, err := pcanbasic.Open(pcanbasic.USBBus1, pcanbasic.WithBitrate(pcanbasic.Baud500K))
	if err != nil {
		log.Fatalf("open USBBus1: %v", err)
	}
	defer bus1.Close()

	bus2, err := pcanbasic.Open(pcanbasic.USBBus2, pcanbasic.WithBitrate(pcanbasic.Baud500K))
	if err != nil {
		log.Fatalf("open USBBus2: %v", err)
	}
	defer bus2.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go pump(ctx, &wg, "bus1", bus1)
	go pump(ctx, &wg, "bus2", bus2)
	wg.Wait()
	log.Println("both pumps exited")
}

func pump(ctx context.Context, wg *sync.WaitGroup, tag string, bus *pcanbasic.Bus) {
	defer wg.Done()
	for {
		fr, err := bus.ReadOne(ctx)
		if err != nil {
			log.Printf("[%s] stop: %v", tag, err)
			return
		}
		log.Printf("[%s] rx id=0x%X data=%X", tag, fr.ID, fr.Data)
	}
}

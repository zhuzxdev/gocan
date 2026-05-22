// 运行: go run ./examples/07_filter -channel=USBBus1
// 前置: Windows + PCAN-USB
//
// 演示用 SetFilter 收窄接收 ID 范围、用 ResetFilter 恢复"接收全部"。
// PCAN 默认是开放的（所有 ID 都接收），SetFilter 是"收窄"，不是"添加"。

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/Crush251/pcanbasic_go"
)

func main() {
	chName := flag.String("channel", "USBBus1", "channel name")
	flag.Parse()

	ch, ok := lookupChannel(*chName)
	if !ok {
		log.Fatalf("unknown channel: %s", *chName)
	}

	bus, err := pcanbasic.Open(ch, pcanbasic.WithBitrate(pcanbasic.Baud500K))
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer bus.Close()

	// 只接收 ID 范围 [0x100, 0x1FF] 的标准帧。
	if err := bus.SetFilter(0x100, 0x1FF, pcanbasic.FilterStandard); err != nil {
		log.Fatalf("set filter: %v", err)
	}
	log.Println("filter set to [0x100, 0x1FF] standard")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// 收 5 秒后恢复全开。
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case fr := <-bus.Receive():
			log.Printf("rx id=0x%X data=%X", fr.ID, fr.Data)
		case <-timer.C:
			if err := bus.ResetFilter(); err != nil {
				log.Fatalf("reset filter: %v", err)
			}
			log.Println("filter reset — now accepting all IDs")
			timer.Reset(time.Hour) // 关掉计时器
		case <-ctx.Done():
			return
		}
	}
}

func lookupChannel(name string) (pcanbasic.Channel, bool) {
	switch name {
	case "USBBus1":
		return pcanbasic.USBBus1, true
	case "USBBus2":
		return pcanbasic.USBBus2, true
	}
	return 0, false
}

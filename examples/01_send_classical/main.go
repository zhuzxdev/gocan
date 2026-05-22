// 运行: go run ./examples/01_send_classical -channel=USBBus1
// 前置: Windows + PCAN-USB + 已放置 PCANBasic.dll（或设 PCANBASIC_DLL_PATH）
//
// 本示例演示如何打开 Classical CAN 通道并发送一帧标准帧。
// 生产代码中应根据 errors.Is(err, ...) 区分 BUSOFF / 队列满等具体错误。

package main

import (
	"context"
	"flag"
	"log"

	"github.com/Crush251/pcanbasic_go"
)

func main() {
	chName := flag.String("channel", "USBBus1", "channel name (USBBus1..USBBus16)")
	flag.Parse()

	ch, ok := lookupChannel(*chName)
	if !ok {
		log.Fatalf("unknown channel: %s", *chName)
	}

	bus, err := pcanbasic.Open(ch, pcanbasic.WithBitrate(pcanbasic.Baud1M))
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer bus.Close()

	frame, err := pcanbasic.NewFrame(0x123, []byte{0x01, 0x02, 0x03, 0x04})
	if err != nil {
		log.Fatal(err)
	}
	if err := bus.Send(context.Background(), frame); err != nil {
		log.Fatalf("send: %v", err)
	}
	log.Printf("sent: id=0x%X data=%X", frame.ID, frame.Data)
}

func lookupChannel(name string) (pcanbasic.Channel, bool) {
	switch name {
	case "USBBus1":
		return pcanbasic.USBBus1, true
	case "USBBus2":
		return pcanbasic.USBBus2, true
	case "USBBus3":
		return pcanbasic.USBBus3, true
	case "USBBus4":
		return pcanbasic.USBBus4, true
	case "USBBus5":
		return pcanbasic.USBBus5, true
	case "USBBus6":
		return pcanbasic.USBBus6, true
	case "USBBus7":
		return pcanbasic.USBBus7, true
	case "USBBus8":
		return pcanbasic.USBBus8, true
	}
	return 0, false
}

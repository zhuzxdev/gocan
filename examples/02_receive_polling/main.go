// 运行: go run ./examples/02_receive_polling -channel=USBBus1
// 前置: Windows + PCAN-USB + 总线上有其他节点在发帧
//
// 演示用 Polling 模式接收 Classical CAN 帧。
// 适合对 CPU 占用敏感、可接受 1ms 级抖动的场景。

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

	bus, err := pcanbasic.Open(ch,
		pcanbasic.WithBitrate(pcanbasic.Baud500K),
		pcanbasic.WithReceiveMode(pcanbasic.ModePolling),
		pcanbasic.WithPollInterval(time.Millisecond),
	)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer bus.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Println("polling mode receive started, Ctrl-C to stop")
	for {
		fr, err := bus.ReadOne(ctx)
		if err != nil {
			log.Printf("stop: %v", err)
			return
		}
		log.Printf("rx id=0x%X ext=%v rtr=%v data=%X ts=%dµs",
			fr.ID,
			fr.Has(pcanbasic.FlagExtended),
			fr.Has(pcanbasic.FlagRemote),
			fr.Data, fr.TimestampMicros)
	}
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
	}
	return 0, false
}

// 运行: go run ./examples/03_receive_event -channel=USBBus1
// 前置: Windows + PCAN-USB（Event 模式仅在 Windows 真实生效）
//
// 演示 Event 模式接收：reader 阻塞在 WaitForMultipleObjects，CPU 占用极低，
// 接收延迟取决于驱动而不是轮询周期。

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"

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
		pcanbasic.WithReceiveMode(pcanbasic.ModeEvent),
	)
	if err != nil {
		log.Fatalf("open (event): %v — 非 Windows 平台请改用 ModePolling", err)
	}
	defer bus.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Println("event mode receive started, Ctrl-C to stop")
	for {
		select {
		case fr, ok := <-bus.Receive():
			if !ok {
				log.Println("bus closed")
				return
			}
			log.Printf("rx id=0x%X data=%X ts=%dµs", fr.ID, fr.Data, fr.TimestampMicros)
		case e := <-bus.Errors():
			log.Printf("rx-err: %v", e)
		case <-ctx.Done():
			log.Println("interrupted")
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
	case "USBBus3":
		return pcanbasic.USBBus3, true
	case "USBBus4":
		return pcanbasic.USBBus4, true
	}
	return 0, false
}

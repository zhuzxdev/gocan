// 运行: go run ./examples/05_receive_fd -channel=USBBus1
// 前置: 支持 CAN FD 的 PCAN 硬件
//
// 演示 OpenFD 接收 FD 帧。区分 Classical（FlagFD=0）和真正 FD 帧
// 主要看 fr.Has(FlagFD) 与 len(fr.Data)。

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/Crush251/pcanbasic_go"
)

const fdBitrate = "f_clock=80000000, " +
	"nom_brp=2, nom_tseg1=63, nom_tseg2=16, nom_sjw=16, " +
	"data_brp=2, data_tseg1=15, data_tseg2=4, data_sjw=4"

func main() {
	chName := flag.String("channel", "USBBus1", "channel name")
	flag.Parse()

	ch, ok := lookupChannel(*chName)
	if !ok {
		log.Fatalf("unknown channel: %s", *chName)
	}

	bus, err := pcanbasic.OpenFD(ch, fdBitrate)
	if err != nil {
		log.Fatalf("openFD: %v", err)
	}
	defer bus.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Println("FD receive started, Ctrl-C to stop")
	for {
		fr, err := bus.ReadOne(ctx)
		if err != nil {
			log.Printf("stop: %v", err)
			return
		}
		kind := "classical"
		if fr.Has(pcanbasic.FlagFD) {
			kind = "FD"
		}
		log.Printf("rx[%s] id=0x%X brs=%v esi=%v len=%d",
			kind, fr.ID,
			fr.Has(pcanbasic.FlagBRS),
			fr.Has(pcanbasic.FlagESI),
			len(fr.Data))
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

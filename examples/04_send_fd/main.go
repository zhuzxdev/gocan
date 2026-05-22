// 运行: go run ./examples/04_send_fd -channel=USBBus1
// 前置: Windows + 支持 CAN FD 的硬件（如 PCAN-USB Pro FD）
//
// 演示用 OpenFD 打开 FD 通道并发送一帧 64 字节 BRS 帧。
// fdBitrate 字符串中 nom_* 是仲裁段，data_* 是数据段（高速）参数。

package main

import (
	"context"
	"flag"
	"log"

	"github.com/Crush251/pcanbasic_go"
)

// 80 MHz 时钟下的 1Mbit 仲裁 + 2Mbit 数据段示例参数。
// 实际项目应根据硬件手册推荐值调整。
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

	// 64 字节 FD 帧，启用 BRS（数据段高速）。
	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}
	fr, err := pcanbasic.NewFDFrame(0x456, data, true, false)
	if err != nil {
		log.Fatal(err)
	}
	if err := bus.Send(context.Background(), fr); err != nil {
		log.Fatalf("send fd: %v", err)
	}
	log.Printf("sent FD id=0x%X len=%d brs=true", fr.ID, len(fr.Data))
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

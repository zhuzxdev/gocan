// 运行: go run ./examples/11_busgroup_socketcan
// 前置: Linux + 已用 scripts/setup-vcan.sh up vcan0 vcan1 创建虚拟 CAN 接口
//
// 演示用 BusGroup 同时管理两个 SocketCAN 通道，合并接收循环 + 一行收尾。
// 替代手写 sync.WaitGroup + 双份 reader goroutine 的旧式写法。

package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"

	"github.com/Crush251/gocan"
)

func main() {
	g := gocan.NewBusGroup(0)
	defer func() {
		if err := g.Close(); err != nil {
			var gce *gocan.GroupCloseError
			if errors.As(err, &gce) {
				for name, e := range gce.Causes {
					log.Printf("close %s: %v", name, e)
				}
			} else {
				log.Printf("close: %v", err)
			}
		}
	}()

	if _, err := g.Add("front", gocan.SocketCAN("vcan0")); err != nil {
		log.Fatalf("add front: %v", err)
	}
	if _, err := g.Add("chassis", gocan.SocketCAN("vcan1")); err != nil {
		log.Fatalf("add chassis: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case sf := <-g.Receive():
			log.Printf("[%s] id=0x%X dlc=%d data=%X", sf.Source, sf.Frame.ID, len(sf.Frame.Data), sf.Frame.Data)
		}
	}
}

// 运行: go run ./examples/12_busgroup_fan_in
// 前置: Linux + scripts/setup-vcan.sh up vcan0 vcan1 vcan2
//
// 演示用 SourcedFrame 的 Source 字段做业务路由：合并接收 + switch 分发。
// 退出前用 BusGroup.Each 给每个 Bus 打一次 Status 做诊断。

package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/Crush251/gocan"
)

func handleEngine(f gocan.Frame)    { log.Printf("engine: 0x%X %X", f.ID, f.Data) }
func handleBody(f gocan.Frame)      { log.Printf("body: 0x%X %X", f.ID, f.Data) }
func handleTelemetry(f gocan.Frame) { log.Printf("telemetry: 0x%X %X", f.ID, f.Data) }

func main() {
	g := gocan.NewBusGroup(0)
	defer func() {
		g.Each(func(name string, bus *gocan.Bus) {
			st, err := bus.Status()
			if err != nil {
				log.Printf("[%s] status err: %v", name, err)
				return
			}
			log.Printf("[%s] final status=0x%X", name, uint32(st))
		})
		_ = g.Close()
	}()

	for _, p := range []struct{ name, iface string }{
		{"engine", "vcan0"},
		{"body", "vcan1"},
		{"telemetry", "vcan2"},
	} {
		if _, err := g.Add(p.name, gocan.SocketCAN(p.iface)); err != nil {
			log.Fatalf("add %s: %v", p.name, err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case sf := <-g.Receive():
			switch sf.Source {
			case "engine":
				handleEngine(sf.Frame)
			case "body":
				handleBody(sf.Frame)
			case "telemetry":
				handleTelemetry(sf.Frame)
			default:
				log.Printf("unknown source %q", sf.Source)
			}
		}
	}
}

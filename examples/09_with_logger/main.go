// 运行: go run ./examples/09_with_logger -channel=USBBus1
// 前置: Windows + PCAN-USB（不依赖也能编译通过）
//
// 演示如何把标准库的 log/slog 适配为 pcanbasic.Logger 接口，
// 让库内部的提示（如 Event 模式降级）走结构化日志。

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/Crush251/pcanbasic_go"
)

// slogAdapter 把 *slog.Logger 适配为 pcanbasic.Logger 接口。
type slogAdapter struct{ l *slog.Logger }

func (a slogAdapter) Debugf(format string, args ...any) {
	a.l.Debug("pcanbasic", "msg", fmt.Sprintf(format, args...))
}
func (a slogAdapter) Infof(format string, args ...any) {
	a.l.Info("pcanbasic", "msg", fmt.Sprintf(format, args...))
}
func (a slogAdapter) Warnf(format string, args ...any) {
	a.l.Warn("pcanbasic", "msg", fmt.Sprintf(format, args...))
}

func main() {
	chName := flag.String("channel", "USBBus1", "channel name")
	flag.Parse()

	ch, ok := lookupChannel(*chName)
	if !ok {
		log.Fatalf("unknown channel: %s", *chName)
	}

	jsonLogger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	bus, err := pcanbasic.Open(ch,
		pcanbasic.WithBitrate(pcanbasic.Baud500K),
		pcanbasic.WithLogger(slogAdapter{l: jsonLogger}),
	)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer bus.Close()

	fr, _ := pcanbasic.NewFrame(0x100, []byte{0xAA, 0xBB})
	if err := bus.Send(context.Background(), fr); err != nil {
		log.Fatalf("send: %v", err)
	}
	log.Println("sent — check stderr for structured logs from pcanbasic")
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

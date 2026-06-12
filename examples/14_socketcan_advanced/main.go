// 运行: go run ./examples/14_socketcan_advanced
// 前置: Linux + scripts/setup-vcan.sh up vcan0
//
// 演示完整的 SocketCAN 调试 + 调优组合：
//   1. WithErrFilter         订阅总线错误帧用于诊断
//   2. WithJoinFilters       多 SetFilter 范围用 AND 语义
//   3. WithRecvTimestamp     内核纳秒时间戳直接写入 Frame.TimestampMicros
//   4. WithSocketBuffers     为高吞吐场景扩 socket 缓冲区
//   5. WithRWTimeout         读超时防止 reader 永远阻塞
//
// 末尾演示运行期通过 SetErrFilter(0) 关闭错误帧订阅。

//go:build linux

package main

import (
	"context"
	"log"
	"time"

	"github.com/Crush251/gocan"
)

func main() {
	bus, err := gocan.Open(
		gocan.SocketCAN("vcan0"),
		gocan.WithLoopback(true),
		gocan.WithRecvOwnMsgs(true),

		// (1) 错误帧诊断：BUSOFF 等关键事件会被解码成 raw 错误帧投递。
		gocan.WithErrFilter(gocan.CANErrBusOff|gocan.CANErrTxTimeout|gocan.CANErrCrtl),

		// (2) AND 语义：后续 SetFilter 必须同时匹配才接收（默认 OR）。
		gocan.WithJoinFilters(true),

		// (3) 内核纳秒时间戳：精度高于 time.Now() 合成方案。
		gocan.WithRecvTimestamp(gocan.RxTimestampNano),

		// (4) 高吞吐：rcv 2 MiB / snd 1 MiB（受 net.core.rmem_max 限制）。
		gocan.WithSocketBuffers(2*1024*1024, 1*1024*1024),

		// (5) 读超时 500 ms；不限制写。
		gocan.WithRWTimeout(500*time.Millisecond, 0),
	)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer bus.Close()

	// 手动设两条不相交范围的滤波器；JOIN_FILTERS=true 下 AND 语义会让两条
	// 都必须命中——在不相交的两条上没有交集，等于"什么都不收"。
	if err := bus.SetFilter(0x100, 0x10F, gocan.FilterStandard); err != nil {
		log.Fatalf("SetFilter A: %v", err)
	}
	if err := bus.SetFilter(0x200, 0x20F, gocan.FilterStandard); err != nil {
		log.Fatalf("SetFilter B: %v", err)
	}

	// 发一帧 0x100：因为 AND 语义没法同时落在 0x200..0x20F，所以收不到。
	frame, _ := gocan.NewFrame(0x100, []byte{0x42})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := bus.Send(ctx, frame); err != nil {
		log.Printf("send: %v", err)
	}
	if _, err := bus.ReadOne(ctx); err != nil {
		log.Printf("(预期) 在 AND 滤波下读不到帧: %v", err)
	}

	// 演示运行期改变错误帧订阅。
	if err := bus.SetErrFilter(0); err != nil {
		log.Fatalf("SetErrFilter(0): %v", err)
	}
	log.Printf("错误帧订阅已关闭")
}

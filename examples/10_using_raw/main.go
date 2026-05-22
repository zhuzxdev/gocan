// 运行: go run ./examples/10_using_raw
// 前置: Windows + 已放置 PCANBasic.dll
//
// 演示绕过高层 Bus，直接调用 raw 子包：
//   - raw.EnsureLoaded() 触发 DLL 加载
//   - raw.GetValue(PCAN_API_VERSION, ...) 读取驱动版本字符串
//
// 适合需要 PCAN 罕见参数、且不便走上层 API 的场景。

package main

import (
	"log"
	"unsafe"

	"github.com/Crush251/pcanbasic_go/raw"
)

func main() {
	if err := raw.EnsureLoaded(); err != nil {
		log.Fatalf("load PCANBasic.dll: %v", err)
	}

	// PCAN_API_VERSION 返回一个 NUL 结尾的 ASCII 字符串。
	var buf [256]byte
	s := raw.GetValue(raw.PCAN_NONEBUS, raw.PCAN_API_VERSION,
		unsafe.Pointer(&buf[0]), uint32(len(buf)))
	if s != raw.PCAN_ERROR_OK {
		log.Fatalf("GetValue(API_VERSION) failed: 0x%X", uint32(s))
	}

	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	log.Printf("PCANBasic API version: %s", string(buf[:n]))
}

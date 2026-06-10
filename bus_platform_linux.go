//go:build linux

package gocan

import (
	"golang.org/x/sys/unix"

	"github.com/Crush251/gocan/raw"
)

// linuxConfig 聚合 Linux 专属 Option 的可配置项。
// 所有字段使用 pointer / 0 哨兵：未传 Option 时不调对应 setsockopt。
type linuxConfig struct {
	loopback    *bool
	recvOwnMsgs *bool
}

// applyPlatformOptions 在 Initialize 成功后、startReader 之前调用，
// 把 cfg.linux 的字段写到底层 socket 上。任意一项失败 → Uninitialize 回滚 → 返回错误。
func applyPlatformOptions(b *Bus, cfg *config) error {
	lc := &cfg.linux
	if lc.loopback != nil {
		v := 0
		if *lc.loopback {
			v = 1
		}
		if s := raw.SetCANRawSockoptInt(b.ch, unix.CAN_RAW_LOOPBACK, v); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(CAN_RAW_LOOPBACK)", s)
		}
	}
	if lc.recvOwnMsgs != nil {
		v := 0
		if *lc.recvOwnMsgs {
			v = 1
		}
		if s := raw.SetCANRawSockoptInt(b.ch, unix.CAN_RAW_RECV_OWN_MSGS, v); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(CAN_RAW_RECV_OWN_MSGS)", s)
		}
	}
	return nil
}

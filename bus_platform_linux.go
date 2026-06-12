//go:build linux

package gocan

import (
	"time"

	"golang.org/x/sys/unix"

	"github.com/Crush251/gocan/raw"
)

// linuxConfig 聚合 Linux 专属 Option 的可配置项。
// 所有字段使用 pointer / 0 哨兵：未传 Option 时不调对应 setsockopt。
type linuxConfig struct {
	loopback     *bool
	recvOwnMsgs  *bool
	errFilter    *uint32
	joinFilters  *bool
	rxTimestamp  RxTimestamp
	soRcvBuf     int
	soSndBuf     int
	readTimeout  time.Duration
	writeTimeout time.Duration
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
	if lc.errFilter != nil {
		if s := raw.SetCANRawErrFilter(b.ch, *lc.errFilter); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(CAN_RAW_ERR_FILTER)", s)
		}
	}
	if lc.joinFilters != nil {
		if s := raw.SetCANRawJoinFilters(b.ch, *lc.joinFilters); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(CAN_RAW_JOIN_FILTERS)", s)
		}
	}
	if lc.rxTimestamp != RxTimestampNone {
		if s := raw.EnableRxTimestamp(b.ch, uint8(lc.rxTimestamp)); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(SO_TIMESTAMP*)", s)
		}
	}
	if lc.soRcvBuf > 0 || lc.soSndBuf > 0 {
		if s := raw.SetSocketBuffers(b.ch, lc.soRcvBuf, lc.soSndBuf); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(SO_RCVBUF/SO_SNDBUF)", s)
		}
	}
	if lc.readTimeout > 0 || lc.writeTimeout > 0 {
		if s := raw.SetReadWriteTimeout(b.ch, lc.readTimeout, lc.writeTimeout); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(SO_RCVTIMEO/SO_SNDTIMEO)", s)
		}
	}
	return nil
}

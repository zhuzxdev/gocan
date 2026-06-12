//go:build linux

package gocan

import (
	"time"

	"github.com/Crush251/gocan/raw"
)

// CAN 错误帧位掩码（与 raw 包对应常量等价）。
const (
	CANErrTxTimeout = raw.CANErrTxTimeout
	CANErrLostArb   = raw.CANErrLostArb
	CANErrCrtl      = raw.CANErrCrtl
	CANErrProt      = raw.CANErrProt
	CANErrTrx       = raw.CANErrTrx
	CANErrAck       = raw.CANErrAck
	CANErrBusOff    = raw.CANErrBusOff
	CANErrBusError  = raw.CANErrBusError
	CANErrRestarted = raw.CANErrRestarted
	CANErrMaskAll   = raw.CANErrMaskAll
)

// WithLoopback 设置 CAN_RAW_LOOPBACK（默认内核为 true，即本地回环开启）。
// 关闭后，本进程发出的帧不会被同主机其他 socket 看到。
func WithLoopback(enabled bool) Option {
	return func(c *config) {
		v := enabled
		c.linux.loopback = &v
	}
}

// WithRecvOwnMsgs 设置 CAN_RAW_RECV_OWN_MSGS。开启后会收到本 socket 自己发出的帧
// （需配合 WithLoopback(true)；通常用于自发自收的回归测试）。
func WithRecvOwnMsgs(enabled bool) Option {
	return func(c *config) {
		v := enabled
		c.linux.recvOwnMsgs = &v
	}
}

// WithErrFilter 启用 CAN_RAW_ERR_FILTER，只接收 mask 中位标记的错误帧类型。
// 参见 raw/can_err_linux.go 中的 CANErr* 常量。
func WithErrFilter(mask uint32) Option {
	return func(c *config) {
		v := mask
		c.linux.errFilter = &v
	}
}

// WithJoinFilters 设置 CAN_RAW_JOIN_FILTERS 语义：
// true 表示多个 SetFilter 范围必须**全部**匹配（AND）；false / 默认是任一匹配（OR）。
// 内核 < 4.1 不支持，setsockopt 会返回 ENOPROTOOPT；调用 Open 时会返回包含
// PCAN_ERROR_ILLPARAMVAL 的 *Error，提示需要 ≥ 4.1。
func WithJoinFilters(and bool) Option {
	return func(c *config) {
		v := and
		c.linux.joinFilters = &v
	}
}

// RxTimestamp 选择内核给入帧打时间戳的机制。
// 默认 RxTimestampNone 不启用；启用时 Frame.TimestampMicros 由内核提供。
type RxTimestamp uint8

const (
	RxTimestampNone     RxTimestamp = 0
	RxTimestampSecond   RxTimestamp = 1 // SO_TIMESTAMP（μs 精度）
	RxTimestampNano     RxTimestamp = 2 // SO_TIMESTAMPNS（ns 精度）
	RxTimestampHardware RxTimestamp = 3 // SO_TIMESTAMPING + RX_HARDWARE，不支持时降级到 NS
)

// WithRecvTimestamp 启用内核接收时间戳，结果写入 Frame.TimestampMicros。
// 不传该 Option 时保持现有行为：SocketCAN 后端用 time.Now() 合成时间戳。
func WithRecvTimestamp(mode RxTimestamp) Option {
	return func(c *config) {
		c.linux.rxTimestamp = mode
	}
}

// WithSocketBuffers 设置 SO_RCVBUF / SO_SNDBUF。任一非正值则跳过对应方向。
// 实际生效值受内核 net.core.rmem_max / wmem_max 上限限制。
func WithSocketBuffers(rcvBytes, sndBytes int) Option {
	return func(c *config) {
		if rcvBytes > 0 {
			c.linux.soRcvBuf = rcvBytes
		}
		if sndBytes > 0 {
			c.linux.soSndBuf = sndBytes
		}
	}
}

// WithRWTimeout 设置 SO_RCVTIMEO / SO_SNDTIMEO。零值表示该方向不设超时。
// 注意：当前 reader goroutine 用 polling 循环 + 短读，超时通常无显著影响；
// 主要用于 SocketCAN 在某些场景下避免 read() 永远阻塞。
func WithRWTimeout(read, write time.Duration) Option {
	return func(c *config) {
		if read > 0 {
			c.linux.readTimeout = read
		}
		if write > 0 {
			c.linux.writeTimeout = write
		}
	}
}

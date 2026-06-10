//go:build linux

package gocan

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

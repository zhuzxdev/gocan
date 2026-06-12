package gocan

import (
	"time"

	"github.com/Crush251/gocan/raw"
)

// Channel 是 PCAN 通道句柄的别名，方便 Open 调用。
type Channel = raw.TPCANHandle

// Bitrate 是 Classical CAN 波特率的别名。
type Bitrate = raw.TPCANBaudrate

// PCAN-USB 通道常量。重新导出 raw 包的对应值，避免使用者引入 raw 包。
const (
	USBBus1  Channel = raw.PCAN_USBBUS1
	USBBus2  Channel = raw.PCAN_USBBUS2
	USBBus3  Channel = raw.PCAN_USBBUS3
	USBBus4  Channel = raw.PCAN_USBBUS4
	USBBus5  Channel = raw.PCAN_USBBUS5
	USBBus6  Channel = raw.PCAN_USBBUS6
	USBBus7  Channel = raw.PCAN_USBBUS7
	USBBus8  Channel = raw.PCAN_USBBUS8
	USBBus9  Channel = raw.PCAN_USBBUS9
	USBBus10 Channel = raw.PCAN_USBBUS10
	USBBus11 Channel = raw.PCAN_USBBUS11
	USBBus12 Channel = raw.PCAN_USBBUS12
	USBBus13 Channel = raw.PCAN_USBBUS13
	USBBus14 Channel = raw.PCAN_USBBUS14
	USBBus15 Channel = raw.PCAN_USBBUS15
	USBBus16 Channel = raw.PCAN_USBBUS16
)

// 常用 Classical CAN 波特率。
const (
	Baud1M   Bitrate = raw.PCAN_BAUD_1M
	Baud500K Bitrate = raw.PCAN_BAUD_500K
	Baud250K Bitrate = raw.PCAN_BAUD_250K
	Baud125K Bitrate = raw.PCAN_BAUD_125K
	Baud100K Bitrate = raw.PCAN_BAUD_100K
	Baud50K  Bitrate = raw.PCAN_BAUD_50K
	Baud20K  Bitrate = raw.PCAN_BAUD_20K
	Baud10K  Bitrate = raw.PCAN_BAUD_10K
	Baud5K   Bitrate = raw.PCAN_BAUD_5K
)

// ReceiveMode 控制 Bus 内部 reader goroutine 的等待策略。
type ReceiveMode int

// 接收模式常量。
const (
	// ModeAuto：Windows + 驱动支持事件 → Event；否则退回 Polling。
	// 库的默认模式：兼顾延迟与可移植性。
	ModeAuto ReceiveMode = iota
	// ModePolling：以 WithPollInterval 设定的间隔轮询底层。
	ModePolling
	// ModeEvent：Windows Event Handle 阻塞等待，CPU 占用最低。
	ModeEvent
)

// FilterMode 用于 SetFilter。
type FilterMode uint8

// 过滤器模式：区分 11 位 / 29 位 ID。
const (
	FilterStandard FilterMode = 0 // 接收 11 位 ID
	FilterExtended FilterMode = 1 // 接收 29 位 ID
)

// 默认参数值。
const (
	defaultBitrate       Bitrate       = Baud1M
	defaultPollInterval  time.Duration = time.Millisecond
	defaultRxBufferSize  int           = 1024
	defaultErrBufferSize int           = 16
)

// config 内部聚合所有 Option 的可配置项。
type config struct {
	bitrate       Bitrate
	receiveMode   ReceiveMode
	pollInterval  time.Duration
	rxBufferSize  int
	errBufferSize int
	logger        Logger
	linux         linuxConfig // 仅 Linux 构建有真实字段；其他平台为空 struct
}

// newDefaultConfig 返回填充了默认值的 config。
func newDefaultConfig() *config {
	return &config{
		bitrate:       defaultBitrate,
		receiveMode:   ModeAuto,
		pollInterval:  defaultPollInterval,
		rxBufferSize:  defaultRxBufferSize,
		errBufferSize: defaultErrBufferSize,
		logger:        noopLogger{},
	}
}

// Option 用于配置 Open / OpenFD。
type Option func(*config)

// WithBitrate 设置 Classical CAN 波特率，默认 Baud1M。
//
// OpenFD 时此选项被忽略 —— FD 波特率由 OpenFD 的 fdBitrate 字符串决定。
func WithBitrate(b Bitrate) Option { return func(c *config) { c.bitrate = b } }

// WithReceiveMode 设置接收模式，默认 ModeAuto。
func WithReceiveMode(m ReceiveMode) Option { return func(c *config) { c.receiveMode = m } }

// WithPollInterval 设置 Polling 模式下的轮询间隔，默认 1ms。
//
// 非正值（≤0）会被忽略并保留默认。
// 仅 ModePolling（或 ModeAuto 降级到 Polling）时生效。
func WithPollInterval(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.pollInterval = d
		}
	}
}

// WithRxBufferSize 设置接收 channel 容量，默认 1024。
// 非正值会被忽略并保留默认。
func WithRxBufferSize(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.rxBufferSize = n
		}
	}
}

// WithErrBufferSize 设置错误 channel 容量，默认 16。
// 非正值会被忽略并保留默认。
func WithErrBufferSize(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.errBufferSize = n
		}
	}
}

// WithLogger 注入日志接口。默认 noopLogger 不打印任何东西。
// 传入 nil 会被忽略。
func WithLogger(l Logger) Option {
	return func(c *config) {
		if l != nil {
			c.logger = l
		}
	}
}

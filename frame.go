package pcanbasic

import (
	"time"
)

// Frame 代表一帧 CAN 报文，统一承载 Classical / Extended / Remote / FD。
//
// 通过 Flags 区分帧类型；Data 长度 + Flags 决定底层映射到 TPCANMsg 还是 TPCANMsgFD。
// 接收到的帧会填充 TimestampMicros（PCAN 自带）和 ReceivedAt（Go 端时刻）。
type Frame struct {
	ID    uint32     // CAN 标识符（11 位 / 29 位由 FlagExtended 决定）
	Data  []byte     // 数据载荷
	Flags FrameFlags // 帧类型标志位

	TimestampMicros uint64    // PCAN 提供的微秒时间戳（接收帧才有效）
	ReceivedAt      time.Time // Go 进程接收到该帧的时刻（接收帧才有效）
}

// FrameFlags 是 Frame 类型/属性位的组合。
type FrameFlags uint16

// Frame 标志位常量。
const (
	FlagExtended FrameFlags = 1 << iota // 29 位扩展 ID
	FlagRemote                          // 远程帧（仅 Classical CAN）
	FlagFD                              // CAN FD 帧
	FlagBRS                             // FD 加速段（仅 FD 帧有意义）
	FlagESI                             // FD 错误状态指示（仅 FD 帧有意义）
)

// Has 判断 Flags 中是否包含指定位。
func (f Frame) Has(flag FrameFlags) bool { return f.Flags&flag == flag }

// 标准 CAN ID 上限。
const (
	maxStdID uint32 = 0x7FF
	maxExtID uint32 = 0x1FFFFFFF
)

// fdValidLengths 列出 CAN FD 协议允许的离散数据长度。
var fdValidLengths = map[int]bool{
	0: true, 1: true, 2: true, 3: true, 4: true, 5: true, 6: true,
	7: true, 8: true, 12: true, 16: true, 20: true, 24: true,
	32: true, 48: true, 64: true,
}

// NewFrame 构造一帧标准 Classical CAN 报文（11 位 ID）。
//
// ID 必须 ≤ 0x7FF；data 长度必须 ≤ 8。
// data 会被深拷贝，调用者后续修改原切片不影响已构造的 Frame。
func NewFrame(id uint32, data []byte) (Frame, error) {
	if id > maxStdID {
		return Frame{}, ErrIDOutOfRange
	}
	if len(data) > 8 {
		return Frame{}, ErrDataTooLong
	}
	return Frame{
		ID:   id,
		Data: append([]byte(nil), data...),
	}, nil
}

// NewExtendedFrame 构造一帧扩展 Classical CAN 报文（29 位 ID）。
//
// ID 必须 ≤ 0x1FFFFFFF；data 长度必须 ≤ 8。
func NewExtendedFrame(id uint32, data []byte) (Frame, error) {
	if id > maxExtID {
		return Frame{}, ErrIDOutOfRange
	}
	if len(data) > 8 {
		return Frame{}, ErrDataTooLong
	}
	return Frame{
		ID:    id,
		Data:  append([]byte(nil), data...),
		Flags: FlagExtended,
	}, nil
}

// NewRemoteFrame 构造一帧远程请求帧（Classical CAN 专用）。
//
// dlc 表示请求长度（≤ 8）；extended 控制是否使用 29 位扩展 ID。
// 远程帧没有真实数据，Data 字段会被填充 dlc 个零字节用于标记长度。
func NewRemoteFrame(id uint32, dlc uint8, extended bool) (Frame, error) {
	if dlc > 8 {
		return Frame{}, ErrDataTooLong
	}
	limit := maxStdID
	flags := FlagRemote
	if extended {
		limit = maxExtID
		flags |= FlagExtended
	}
	if id > limit {
		return Frame{}, ErrIDOutOfRange
	}
	return Frame{
		ID:    id,
		Data:  make([]byte, dlc),
		Flags: flags,
	}, nil
}

// NewFDFrame 构造一帧 CAN FD 报文。
//
// data 长度必须属于 {0..8, 12, 16, 20, 24, 32, 48, 64}。
// brs 控制是否启用加速段；extended 控制是否使用 29 位扩展 ID。
// FD 协议无 RTR，因此不允许 Remote 标志。
func NewFDFrame(id uint32, data []byte, brs, extended bool) (Frame, error) {
	if !fdValidLengths[len(data)] {
		return Frame{}, ErrInvalidFDLength
	}
	limit := maxStdID
	flags := FlagFD
	if extended {
		limit = maxExtID
		flags |= FlagExtended
	}
	if id > limit {
		return Frame{}, ErrIDOutOfRange
	}
	if brs {
		flags |= FlagBRS
	}
	return Frame{
		ID:    id,
		Data:  append([]byte(nil), data...),
		Flags: flags,
	}, nil
}

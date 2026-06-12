package gocan

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Crush251/gocan/raw"
)

// 库内部错误（参数校验、状态等）。
var (
	// ErrIDOutOfRange 表示 CAN ID 超出范围（标准 11 位 / 扩展 29 位）。
	ErrIDOutOfRange = errors.New("can: CAN ID out of range")
	// ErrDataTooLong 表示数据长度超过该帧类型允许的最大长度。
	ErrDataTooLong = errors.New("can: data length exceeds capacity")
	// ErrInvalidFDLength 表示 FD 帧 data 长度不在 {0..8, 12, 16, 20, 24, 32, 48, 64} 中。
	ErrInvalidFDLength = errors.New("can: invalid CAN FD data length")
	// ErrRemoteOnFD 表示在 FD 帧上指定了 Remote 标志（FD 协议无 RTR）。
	ErrRemoteOnFD = errors.New("can: remote frame not allowed on CAN FD")
	// ErrBusClosed 表示 Bus 已被关闭，后续操作非法。
	ErrBusClosed = errors.New("can: bus is closed")
	// ErrNotSupported 表示当前平台不支持此操作（如非 Windows 试图打开真实通道）。
	ErrNotSupported = errors.New("can: operation not supported on this platform")
	// ErrDLLNotFound 表示 PCANBasic.dll 加载失败。
	ErrDLLNotFound = errors.New("can: PCANBasic.dll not found or failed to load")
	// ErrFDNotSupportedOnBus 表示在非 FD Bus 上尝试发送 FD 帧。
	ErrFDNotSupportedOnBus = errors.New("can: FD frame requires a bus opened with OpenFD")
)

// 队列状态相关错误。
var (
	// ErrQueueEmpty 表示接收队列暂时为空（TryRead 用）。
	ErrQueueEmpty = errors.New("can: receive queue empty")
	// ErrQueueOverrun 表示接收队列被覆盖（应用读取过慢）。
	ErrQueueOverrun = errors.New("can: receive queue overrun")
	// ErrQueueXmtFull 表示发送队列已满。
	ErrQueueXmtFull = errors.New("can: transmit queue full")
)

// 总线状态相关错误（位掩码语义，多个可同时为真）。
var (
	ErrBusLight   = errors.New("can: bus light")
	ErrBusHeavy   = errors.New("can: bus heavy")
	ErrBusPassive = errors.New("can: bus passive")
	ErrBusOff     = errors.New("can: bus off")
)

// API / 驱动层错误。
var (
	// ErrNotInitialized 对应 PCAN_ERROR_INITIALIZE：通道未被初始化。
	ErrNotInitialized = errors.New("can: channel not initialized")
	// ErrIllHandle 对应 PCAN_ERROR_ILLHANDLE：非法通道句柄。
	ErrIllHandle = errors.New("can: invalid channel handle")
	// ErrIllParamType 对应 PCAN_ERROR_ILLPARAMTYPE。
	ErrIllParamType = errors.New("can: invalid parameter type")
	// ErrIllParamValue 对应 PCAN_ERROR_ILLPARAMVAL。
	ErrIllParamValue = errors.New("can: invalid parameter value")
	// ErrIllOperation 对应 PCAN_ERROR_ILLOPERATION：非法操作（如平台不支持）。
	ErrIllOperation = errors.New("can: illegal operation")
	// ErrNoDriver 对应 PCAN_ERROR_NODRIVER：驱动未加载。
	ErrNoDriver = errors.New("can: driver not loaded")
	// ErrUnknown 对应 PCAN_ERROR_UNKNOWN。
	ErrUnknown = errors.New("can: unknown error")
)

// Error 是一次 PCAN API 调用产生的错误。
//
// Code 保留原始的 PCAN 错误位掩码（可能是多个 bit 的 OR 组合），
// 用 errors.Is(err, ErrXxx) 可精确判断"是否包含某种错误"。
type Error struct {
	Code raw.TPCANStatus // 原始 PCAN 错误位掩码
	Op   string          // 触发错误的操作，如 "CAN_Initialize"
	Msg  string          // 通过 CAN_GetErrorText 取到的可读描述（可能为空）
}

// Error 实现 error。
func (e *Error) Error() string {
	if e.Msg != "" {
		return fmt.Sprintf("can: %s failed: 0x%08X: %s",
			e.Op, uint32(e.Code), e.Msg)
	}
	return fmt.Sprintf("can: %s failed: 0x%08X", e.Op, uint32(e.Code))
}

// Has 判断错误码中是否包含某个具体错误位。
//
// 特别处理 PCAN_ERROR_OK (0)：仅当 e.Code 也是 0 时才算"包含"。
// 否则按位掩码 AND 判断。
func (e *Error) Has(code raw.TPCANStatus) bool {
	if code == raw.PCAN_ERROR_OK {
		return e.Code == raw.PCAN_ERROR_OK
	}
	return e.Code&code == code
}

// Is 让一个 *Error 可以同时匹配多个哨兵错误（位掩码语义）。
//
// 因为 PCAN 错误码本质是位掩码，单次 API 调用可能同时报告 BUSOFF|QOVERRUN，
// 此时 errors.Is(err, ErrBusOff) 和 errors.Is(err, ErrQueueOverrun) 都应为 true。
func (e *Error) Is(target error) bool {
	switch target {
	case ErrQueueEmpty:
		return e.Has(raw.PCAN_ERROR_QRCVEMPTY)
	case ErrQueueOverrun:
		return e.Has(raw.PCAN_ERROR_QOVERRUN)
	case ErrQueueXmtFull:
		return e.Has(raw.PCAN_ERROR_QXMTFULL)
	case ErrBusLight:
		return e.Has(raw.PCAN_ERROR_BUSLIGHT)
	case ErrBusHeavy:
		return e.Has(raw.PCAN_ERROR_BUSHEAVY)
	case ErrBusPassive:
		return e.Has(raw.PCAN_ERROR_BUSPASSIVE)
	case ErrBusOff:
		return e.Has(raw.PCAN_ERROR_BUSOFF)
	case ErrNotInitialized:
		return e.Has(raw.PCAN_ERROR_INITIALIZE)
	case ErrIllHandle:
		return e.Has(raw.PCAN_ERROR_ILLHANDLE)
	case ErrIllParamValue:
		return e.Has(raw.PCAN_ERROR_ILLPARAMVAL)
	case ErrIllParamType:
		return e.Has(raw.PCAN_ERROR_ILLPARAMTYPE)
	case ErrIllOperation:
		return e.Has(raw.PCAN_ERROR_ILLOPERATION)
	case ErrNoDriver:
		return e.Has(raw.PCAN_ERROR_NODRIVER)
	}
	return false
}

// SendManyError 标识 SendMany 中第 Index 帧（0-based）发送失败。
//
// Frame 是失败帧的深拷贝，调用方可安全持有以供日志或重试使用。
// 已成功发送的帧不会回滚（CAN 总线无事务概念）。
type SendManyError struct {
	Index int   // 失败的帧下标
	Frame Frame // 失败的帧本身（深拷贝）
	Err   error // 底层错误
}

// Error 实现 error。
func (e *SendManyError) Error() string {
	return fmt.Sprintf("can: SendMany failed at frame[%d]: %v", e.Index, e.Err)
}

// Unwrap 让 errors.Is/errors.As 可以穿透到内部错误。
func (e *SendManyError) Unwrap() error { return e.Err }

// 多 Bus 群组错误。
var (
	// ErrInvalidName 表示 BusGroup.Add 收到的 name 为空字符串。
	ErrInvalidName = errors.New("gocan: invalid bus name")
	// ErrDuplicateName 表示 BusGroup 中已存在同名 Bus。
	ErrDuplicateName = errors.New("gocan: duplicate bus name in group")
)

// GroupCloseError 聚合 BusGroup.Close 时多个 Bus 的失败。
//
// Causes 按名字索引每个失败 Bus 的错误；成功关闭的 Bus 不出现在 map 中。
// errors.Is 会按 Causes 逐个尝试匹配，因此可以 errors.Is(err, ErrBusClosed) 等。
type GroupCloseError struct {
	Causes map[string]error
}

// Error 实现 error。
func (e *GroupCloseError) Error() string {
	names := make([]string, 0, len(e.Causes))
	for n := range e.Causes {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("gocan: BusGroup.Close failed for ")
	for i, n := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s: %v", n, e.Causes[n])
	}
	return b.String()
}

// Unwrap 返回所有底层错误，便于 errors.Is/errors.As 穿透。
func (e *GroupCloseError) Unwrap() []error {
	out := make([]error, 0, len(e.Causes))
	for _, err := range e.Causes {
		out = append(out, err)
	}
	return out
}

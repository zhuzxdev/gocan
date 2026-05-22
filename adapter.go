package pcanbasic

import (
	"unsafe"

	"github.com/Crush251/pcanbasic_go/raw"
)

// rawAdapter 是 Bus 用到的底层调用接口抽象，便于测试时注入 fake。
//
// 不导出 → 不是稳定 API；外部应使用顶层 Bus 或 raw 子包。
type rawAdapter interface {
	Initialize(ch raw.TPCANHandle, br raw.TPCANBaudrate) raw.TPCANStatus
	InitializeFD(ch raw.TPCANHandle, bitrateFD string) raw.TPCANStatus
	Uninitialize(ch raw.TPCANHandle) raw.TPCANStatus

	Read(ch raw.TPCANHandle, m *raw.TPCANMsg, t *raw.TPCANTimestamp) raw.TPCANStatus
	ReadFD(ch raw.TPCANHandle, m *raw.TPCANMsgFD, t *raw.TPCANTimestampFD) raw.TPCANStatus
	Write(ch raw.TPCANHandle, m *raw.TPCANMsg) raw.TPCANStatus
	WriteFD(ch raw.TPCANHandle, m *raw.TPCANMsgFD) raw.TPCANStatus

	GetStatus(ch raw.TPCANHandle) raw.TPCANStatus
	GetErrorText(code raw.TPCANStatus, lang uint16) (string, raw.TPCANStatus)
	Reset(ch raw.TPCANHandle) raw.TPCANStatus

	// 参数 / 过滤器 API（阶段 5 引入）。
	SetValue(ch raw.TPCANHandle, p raw.TPCANParameter, buf unsafe.Pointer, n uint32) raw.TPCANStatus
	GetValue(ch raw.TPCANHandle, p raw.TPCANParameter, buf unsafe.Pointer, n uint32) raw.TPCANStatus
	FilterMessages(ch raw.TPCANHandle, fromID, toID uint32, mode raw.TPCANMessageType) raw.TPCANStatus
}

// liveAdapter 是生产实现：薄包装直接调 raw.*。
type liveAdapter struct{}

func (liveAdapter) Initialize(ch raw.TPCANHandle, br raw.TPCANBaudrate) raw.TPCANStatus {
	return raw.Initialize(ch, br)
}

func (liveAdapter) InitializeFD(ch raw.TPCANHandle, b string) raw.TPCANStatus {
	return raw.InitializeFD(ch, b)
}

func (liveAdapter) Uninitialize(ch raw.TPCANHandle) raw.TPCANStatus {
	return raw.Uninitialize(ch)
}

func (liveAdapter) Read(ch raw.TPCANHandle, m *raw.TPCANMsg, t *raw.TPCANTimestamp) raw.TPCANStatus {
	return raw.Read(ch, m, t)
}

func (liveAdapter) ReadFD(ch raw.TPCANHandle, m *raw.TPCANMsgFD, t *raw.TPCANTimestampFD) raw.TPCANStatus {
	return raw.ReadFD(ch, m, t)
}

func (liveAdapter) Write(ch raw.TPCANHandle, m *raw.TPCANMsg) raw.TPCANStatus {
	return raw.Write(ch, m)
}

func (liveAdapter) WriteFD(ch raw.TPCANHandle, m *raw.TPCANMsgFD) raw.TPCANStatus {
	return raw.WriteFD(ch, m)
}

func (liveAdapter) GetStatus(ch raw.TPCANHandle) raw.TPCANStatus {
	return raw.GetStatus(ch)
}

func (liveAdapter) GetErrorText(code raw.TPCANStatus, lang uint16) (string, raw.TPCANStatus) {
	return raw.GetErrorText(code, lang)
}

func (liveAdapter) Reset(ch raw.TPCANHandle) raw.TPCANStatus {
	return raw.Reset(ch)
}

func (liveAdapter) SetValue(ch raw.TPCANHandle, p raw.TPCANParameter, buf unsafe.Pointer, n uint32) raw.TPCANStatus {
	return raw.SetValue(ch, p, buf, n)
}

func (liveAdapter) GetValue(ch raw.TPCANHandle, p raw.TPCANParameter, buf unsafe.Pointer, n uint32) raw.TPCANStatus {
	return raw.GetValue(ch, p, buf, n)
}

func (liveAdapter) FilterMessages(ch raw.TPCANHandle, fromID, toID uint32, mode raw.TPCANMessageType) raw.TPCANStatus {
	return raw.FilterMessages(ch, fromID, toID, mode)
}

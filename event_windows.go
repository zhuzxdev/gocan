//go:build windows

package pcanbasic

import (
	"fmt"
	"unsafe"

	"github.com/Crush251/pcanbasic_go/raw"
	"golang.org/x/sys/windows"
)

// setupEventMode 创建接收事件 + abort 事件，并把接收事件注册给 PCAN。
//
// 失败时清理已创建的句柄并返回错误。
// 成功后 reader 应调用 waitEventOrAbort 等待数据。
func (b *Bus) setupEventMode() error {
	// 接收事件：PCAN 在有新帧时 SetEvent；reader 在此 wait。
	rxEvent, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return fmt.Errorf("CreateEvent(rx): %w", err)
	}
	// abort 事件：Close 触发，唤醒 reader 退出。
	abortEvent, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		_ = windows.CloseHandle(rxEvent)
		return fmt.Errorf("CreateEvent(abort): %w", err)
	}
	// 通过 CAN_SetValue 把接收事件句柄交给 PCAN。
	handle := uint32(rxEvent)
	s := b.adapt.SetValue(b.ch, raw.PCAN_RECEIVE_EVENT,
		unsafe.Pointer(&handle), uint32(unsafe.Sizeof(handle)))
	if s != raw.PCAN_ERROR_OK {
		_ = windows.CloseHandle(rxEvent)
		_ = windows.CloseHandle(abortEvent)
		return wrapStatus(b.adapt, "CAN_SetValue(PCAN_RECEIVE_EVENT)", s)
	}
	b.eventHandle = uintptr(rxEvent)
	b.abortHandle = uintptr(abortEvent)
	b.useEvent = true
	return nil
}

// closeEventMode 释放 Event 模式相关句柄。Polling 模式下是 no-op。
func (b *Bus) closeEventMode() {
	if !b.useEvent {
		return
	}
	// 触发 abort 事件，唤醒可能阻塞在 wait 的 reader。
	if b.abortHandle != 0 {
		_ = windows.SetEvent(windows.Handle(b.abortHandle))
	}
}

// finalizeEventMode 在 reader 退出之后调用，安全关闭句柄。
// closeEventMode 仅负责唤醒，CloseHandle 必须放在 reader 真正退出后。
func (b *Bus) finalizeEventMode() {
	if !b.useEvent {
		return
	}
	if b.eventHandle != 0 {
		_ = windows.CloseHandle(windows.Handle(b.eventHandle))
		b.eventHandle = 0
	}
	if b.abortHandle != 0 {
		_ = windows.CloseHandle(windows.Handle(b.abortHandle))
		b.abortHandle = 0
	}
}

// waitEventOrAbort 阻塞等待接收事件或 abort 事件。
// 返回 true 表示有数据应该尝试读取；false 表示需要退出 reader。
func (b *Bus) waitEventOrAbort() bool {
	handles := []windows.Handle{
		windows.Handle(b.eventHandle),
		windows.Handle(b.abortHandle),
	}
	r, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
	if err != nil {
		return false
	}
	// WAIT_OBJECT_0 = 接收事件；WAIT_OBJECT_0+1 = abort 事件。
	return r == windows.WAIT_OBJECT_0
}

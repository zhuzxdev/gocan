//go:build !windows

package pcanbasic

// 非 Windows 平台没有真正的 PCAN Event 句柄机制，所有 Event API 都是占位。
// ModeEvent 会在 openWith 中报错；ModeAuto 会降级到 Polling。

func (b *Bus) setupEventMode() error {
	return ErrNotSupported
}

func (b *Bus) closeEventMode() {}

func (b *Bus) finalizeEventMode() {}

func (b *Bus) waitEventOrAbort() bool { return false }

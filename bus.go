package pcanbasic

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Crush251/pcanbasic_go/raw"
)

// Bus 表示一个已初始化的 CAN/CAN FD 通道。
//
// 必须使用 Close 释放资源。Close 是幂等的，可以从多个 goroutine 安全调用。
type Bus struct {
	ch    raw.TPCANHandle
	adapt rawAdapter
	cfg   *config
	isFD  bool

	// 接收侧。reader goroutine 在阶段 4 引入。
	rxCh    chan Frame
	errCh   chan error
	closing chan struct{}

	// 关闭管理。
	closeOnce sync.Once
	closed    atomic.Bool
}

// Open 打开一个 Classical CAN 通道。
//
// ch 是通道句柄（USBBus1..USBBus16 等）；
// opts 用于配置波特率、接收模式、buffer 大小、Logger 等。
func Open(ch Channel, opts ...Option) (*Bus, error) {
	return openWith(liveAdapter{}, ch, false, "", opts...)
}

// OpenFD 打开一个 CAN FD 通道。
//
// fdBitrate 是 PCAN 官方格式的字符串，详见 docs/can-fd.md。
// WithBitrate 对 OpenFD 无效（FD 波特率完全由 fdBitrate 决定）。
func OpenFD(ch Channel, fdBitrate string, opts ...Option) (*Bus, error) {
	return openWith(liveAdapter{}, ch, true, fdBitrate, opts...)
}

// openWith 是 Open/OpenFD 的内部入口，注入 rawAdapter 以便测试。
func openWith(adapt rawAdapter, ch Channel, fd bool, fdBitrate string, opts ...Option) (*Bus, error) {
	cfg := newDefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	var status raw.TPCANStatus
	if fd {
		status = adapt.InitializeFD(ch, fdBitrate)
	} else {
		status = adapt.Initialize(ch, cfg.bitrate)
	}
	if status != raw.PCAN_ERROR_OK {
		op := "CAN_Initialize"
		if fd {
			op = "CAN_InitializeFD"
		}
		return nil, wrapStatus(adapt, op, status)
	}

	b := &Bus{
		ch:      ch,
		adapt:   adapt,
		cfg:     cfg,
		isFD:    fd,
		rxCh:    make(chan Frame, cfg.rxBufferSize),
		errCh:   make(chan error, cfg.errBufferSize),
		closing: make(chan struct{}),
	}
	b.startReader()
	return b, nil
}

// ReadOne 阻塞从接收队列取一帧，直到 ctx 取消或 Bus 关闭。
//
// Bus 关闭时返回 ErrBusClosed。
// ctx 被取消时返回 ctx.Err()。
func (b *Bus) ReadOne(ctx context.Context) (Frame, error) {
	select {
	case f, ok := <-b.rxCh:
		if !ok {
			return Frame{}, ErrBusClosed
		}
		return f, nil
	case <-ctx.Done():
		return Frame{}, ctx.Err()
	}
}

// TryRead 非阻塞读一帧。队列空时返回 ErrQueueEmpty；Bus 已关闭返回 ErrBusClosed。
func (b *Bus) TryRead() (Frame, error) {
	select {
	case f, ok := <-b.rxCh:
		if !ok {
			return Frame{}, ErrBusClosed
		}
		return f, nil
	default:
		return Frame{}, ErrQueueEmpty
	}
}

// Send 同步发送一帧。Close 后调用返回 ErrBusClosed。
//
// ctx 在 Send 入口被一次性检查（Send 自身是同步调用，不会阻塞太久），
// 真正长时阻塞应通过 SendMany 或上层超时机制控制。
func (b *Bus) Send(ctx context.Context, f Frame) error {
	if b.closed.Load() {
		return ErrBusClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !b.isFD && f.Has(FlagFD) {
		return ErrFDNotSupportedOnBus
	}

	if f.Has(FlagFD) {
		m := toRawMsgFD(f)
		if s := b.adapt.WriteFD(b.ch, &m); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "CAN_WriteFD", s)
		}
		return nil
	}
	m := toRawMsg(f)
	if s := b.adapt.Write(b.ch, &m); s != raw.PCAN_ERROR_OK {
		return wrapStatus(b.adapt, "CAN_Write", s)
	}
	return nil
}

// SendMany 顺序逐帧发送，任意一帧失败立即返回 *SendManyError。
//
// 已成功发送的帧不会回滚 —— CAN 总线无事务概念。
// ctx 在每帧前检查一次，便于及时取消大批量发送。
func (b *Bus) SendMany(ctx context.Context, frames []Frame) error {
	for i, f := range frames {
		if err := ctx.Err(); err != nil {
			return &SendManyError{Index: i, Frame: copyFrame(f), Err: err}
		}
		if err := b.Send(ctx, f); err != nil {
			return &SendManyError{Index: i, Frame: copyFrame(f), Err: err}
		}
	}
	return nil
}

// Status 查询通道当前状态。
//
// 返回的 Status 是位掩码，使用 StatusHas 判断具体位。
func (b *Bus) Status() (Status, error) {
	if b.closed.Load() {
		return 0, ErrBusClosed
	}
	return Status(b.adapt.GetStatus(b.ch)), nil
}

// Reset 复位通道，清空 PCAN 内部的收发队列。
// 通常用于 BUSOFF 恢复。
func (b *Bus) Reset() error {
	if b.closed.Load() {
		return ErrBusClosed
	}
	if s := b.adapt.Reset(b.ch); s != raw.PCAN_ERROR_OK {
		return wrapStatus(b.adapt, "CAN_Reset", s)
	}
	return nil
}

// Close 释放底层通道。幂等：多次调用安全。
//
// 流程：标记 closed → 关闭 closing channel 通知 reader → 等 reader 退出
// （表现为 rxCh 被 reader 关闭）→ Uninitialize 释放底层资源 → 关闭 errCh。
func (b *Bus) Close() error {
	var err error
	b.closeOnce.Do(func() {
		b.closed.Store(true)
		close(b.closing)
		// 等 reader goroutine 退出 —— 它在退出时关闭 rxCh。
		for range b.rxCh {
			// 丢弃剩余帧，避免 reader 阻塞在 rxCh 发送。
		}
		if s := b.adapt.Uninitialize(b.ch); s != raw.PCAN_ERROR_OK {
			err = wrapStatus(b.adapt, "CAN_Uninitialize", s)
		}
		close(b.errCh)
	})
	return err
}

// Receive 返回流式接收 channel。Bus 关闭时 channel 也关闭。
//
// 推荐用法：在专门的 goroutine 里 `for f := range bus.Receive()`。
func (b *Bus) Receive() <-chan Frame { return b.rxCh }

// Errors 返回接收侧的异步错误流。Bus 关闭时 channel 也关闭。
// QRCVEMPTY（队列空）不会出现在这里 —— 它是正常状态，不算错误。
func (b *Bus) Errors() <-chan error { return b.errCh }

// wrapStatus 把 raw 状态码包成 *Error，附带 GetErrorText 取到的描述。
func wrapStatus(adapt rawAdapter, op string, code raw.TPCANStatus) *Error {
	msg, _ := adapt.GetErrorText(code, raw.LanguageEnglish)
	return &Error{Code: code, Op: op, Msg: msg}
}

// copyFrame 深拷贝 Frame.Data 用于 SendManyError 持久持有。
func copyFrame(f Frame) Frame {
	out := f
	if f.Data != nil {
		out.Data = append([]byte(nil), f.Data...)
	}
	return out
}

// toRawMsg 把 Classical Frame 转成底层 TPCANMsg。
func toRawMsg(f Frame) raw.TPCANMsg {
	var mt raw.TPCANMessageType
	if f.Has(FlagExtended) {
		mt |= raw.PCAN_MESSAGE_EXTENDED
	}
	if f.Has(FlagRemote) {
		mt |= raw.PCAN_MESSAGE_RTR
	}
	m := raw.TPCANMsg{
		ID:      f.ID,
		MsgType: mt,
		Len:     uint8(len(f.Data)),
	}
	copy(m.Data[:], f.Data)
	return m
}

// toRawMsgFD 把 FD Frame 转成底层 TPCANMsgFD。
func toRawMsgFD(f Frame) raw.TPCANMsgFD {
	var mt raw.TPCANMessageType
	mt |= raw.PCAN_MESSAGE_FD
	if f.Has(FlagExtended) {
		mt |= raw.PCAN_MESSAGE_EXTENDED
	}
	if f.Has(FlagBRS) {
		mt |= raw.PCAN_MESSAGE_BRS
	}
	if f.Has(FlagESI) {
		mt |= raw.PCAN_MESSAGE_ESI
	}
	m := raw.TPCANMsgFD{
		ID:      f.ID,
		MsgType: mt,
		DLC:     dataLenToDLC(len(f.Data)),
	}
	copy(m.Data[:], f.Data)
	return m
}

// dataLenToDLC 把字节长度转成 FD 协议的 DLC 编码（0..15）。
func dataLenToDLC(n int) uint8 {
	switch n {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8:
		return uint8(n)
	case 12:
		return 9
	case 16:
		return 10
	case 20:
		return 11
	case 24:
		return 12
	case 32:
		return 13
	case 48:
		return 14
	case 64:
		return 15
	}
	return 0
}

// dlcToDataLen 把 FD 的 DLC 编码反解出字节长度。
func dlcToDataLen(d uint8) int {
	switch d {
	case 9:
		return 12
	case 10:
		return 16
	case 11:
		return 20
	case 12:
		return 24
	case 13:
		return 32
	case 14:
		return 48
	case 15:
		return 64
	default:
		if d <= 8 {
			return int(d)
		}
		return 0
	}
}

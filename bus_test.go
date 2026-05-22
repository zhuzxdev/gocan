package pcanbasic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Crush251/pcanbasic_go/raw"
)

// openTest 是测试专用入口，注入 fake adapter 打开 Classical 通道。
func openTest(t *testing.T, adapt rawAdapter, opts ...Option) *Bus {
	t.Helper()
	bus, err := openWith(adapt, USBBus1, false, "", opts...)
	if err != nil {
		t.Fatalf("openWith: %v", err)
	}
	return bus
}

// openTestFD 是测试专用入口，注入 fake adapter 打开 FD 通道。
func openTestFD(t *testing.T, adapt rawAdapter, opts ...Option) *Bus {
	t.Helper()
	bus, err := openWith(adapt, USBBus1, true, "f_clock=80000000", opts...)
	if err != nil {
		t.Fatalf("openWith FD: %v", err)
	}
	return bus
}

func TestOpen_InitFailureMaps(t *testing.T) {
	f := newFakeAdapter()
	f.initializeReturn = raw.PCAN_ERROR_INITIALIZE
	_, err := openWith(f, USBBus1, false, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrNotInitialized) {
		t.Errorf("expected ErrNotInitialized, got %v", err)
	}
}

func TestOpenFD_InitFailureMaps(t *testing.T) {
	f := newFakeAdapter()
	f.initializeFDReturn = raw.PCAN_ERROR_ILLPARAMVAL
	_, err := openWith(f, USBBus1, true, "bad bitrate string")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrIllParamValue) {
		t.Errorf("expected ErrIllParamValue, got %v", err)
	}
}

func TestSend_OK(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	defer bus.Close()

	frame, _ := NewFrame(0x123, []byte{1, 2, 3})
	if err := bus.Send(context.Background(), frame); err != nil {
		t.Fatal(err)
	}
	if f.writeCalls != 1 {
		t.Errorf("writeCalls = %d, want 1", f.writeCalls)
	}
	if f.lastWrittenMsg.ID != 0x123 || f.lastWrittenMsg.Len != 3 {
		t.Errorf("bad msg: %+v", f.lastWrittenMsg)
	}
}

func TestSend_ExtendedFlag(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	defer bus.Close()

	frame, _ := NewExtendedFrame(0x1FFF, []byte{1})
	if err := bus.Send(context.Background(), frame); err != nil {
		t.Fatal(err)
	}
	if f.lastWrittenMsg.MsgType&raw.PCAN_MESSAGE_EXTENDED == 0 {
		t.Error("expected EXTENDED bit on msg")
	}
}

func TestSend_RemoteFlag(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	defer bus.Close()

	frame, _ := NewRemoteFrame(0x123, 4, false)
	if err := bus.Send(context.Background(), frame); err != nil {
		t.Fatal(err)
	}
	if f.lastWrittenMsg.MsgType&raw.PCAN_MESSAGE_RTR == 0 {
		t.Error("expected RTR bit on msg")
	}
}

func TestSend_AfterClose(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	bus.Close()
	frame, _ := NewFrame(0x1, nil)
	err := bus.Send(context.Background(), frame)
	if !errors.Is(err, ErrBusClosed) {
		t.Errorf("got %v, want ErrBusClosed", err)
	}
}

func TestSend_CtxCancel(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	frame, _ := NewFrame(0x1, nil)
	err := bus.Send(ctx, frame)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

func TestSend_PCANWriteError(t *testing.T) {
	f := newFakeAdapter()
	f.writeReturn = raw.PCAN_ERROR_QXMTFULL
	bus := openTest(t, f)
	defer bus.Close()

	frame, _ := NewFrame(0x1, nil)
	err := bus.Send(context.Background(), frame)
	if !errors.Is(err, ErrQueueXmtFull) {
		t.Errorf("got %v, want ErrQueueXmtFull", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	if err := bus.Close(); err != nil {
		t.Fatal(err)
	}
	if err := bus.Close(); err != nil {
		t.Fatalf("second Close should not error: %v", err)
	}
	if f.uninitializeCalls != 1 {
		t.Errorf("uninitializeCalls = %d, want 1", f.uninitializeCalls)
	}
}

func TestSend_FDFrameOnClassicalBus(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	defer bus.Close()

	frame, _ := NewFDFrame(0x1, []byte{1, 2}, false, false)
	err := bus.Send(context.Background(), frame)
	if !errors.Is(err, ErrFDNotSupportedOnBus) {
		t.Errorf("got %v, want ErrFDNotSupportedOnBus", err)
	}
}

func TestSend_ClassicalFrameOnFDBus(t *testing.T) {
	f := newFakeAdapter()
	bus := openTestFD(t, f)
	defer bus.Close()

	frame, _ := NewFrame(0x1, []byte{1, 2})
	if err := bus.Send(context.Background(), frame); err != nil {
		t.Fatalf("Classical frame on FD bus should be allowed: %v", err)
	}
	if f.writeCalls != 1 {
		t.Errorf("expected Write to be used for Classical frame on FD bus, got writeCalls=%d",
			f.writeCalls)
	}
}

func TestSend_FDFrameOnFDBus(t *testing.T) {
	f := newFakeAdapter()
	bus := openTestFD(t, f)
	defer bus.Close()

	frame, _ := NewFDFrame(0x1, []byte{1, 2}, true, false)
	if err := bus.Send(context.Background(), frame); err != nil {
		t.Fatal(err)
	}
	if f.writeFDCalls != 1 {
		t.Errorf("writeFDCalls = %d, want 1", f.writeFDCalls)
	}
	if f.lastWrittenMsgFD.MsgType&raw.PCAN_MESSAGE_BRS == 0 {
		t.Error("expected BRS bit set on FD msg")
	}
	if f.lastWrittenMsgFD.MsgType&raw.PCAN_MESSAGE_FD == 0 {
		t.Error("expected FD bit set on FD msg")
	}
}

func TestSendMany_AllOK(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	defer bus.Close()

	frames := []Frame{}
	for i := 0; i < 5; i++ {
		fr, _ := NewFrame(uint32(0x100+i), []byte{byte(i)})
		frames = append(frames, fr)
	}
	if err := bus.SendMany(context.Background(), frames); err != nil {
		t.Fatal(err)
	}
	if f.writeCalls != 5 {
		t.Errorf("writeCalls = %d, want 5", f.writeCalls)
	}
}

func TestSendMany_PartialFailure(t *testing.T) {
	f := newFakeAdapter()
	f.writeSequence = []raw.TPCANStatus{
		raw.PCAN_ERROR_OK,
		raw.PCAN_ERROR_OK,
		raw.PCAN_ERROR_QXMTFULL,
	}
	bus := openTest(t, f)
	defer bus.Close()

	frames := []Frame{}
	for i := 0; i < 5; i++ {
		fr, _ := NewFrame(uint32(0x100+i), []byte{byte(i)})
		frames = append(frames, fr)
	}
	err := bus.SendMany(context.Background(), frames)
	if err == nil {
		t.Fatal("expected error")
	}
	var sme *SendManyError
	if !errors.As(err, &sme) {
		t.Fatalf("expected *SendManyError, got %T", err)
	}
	if sme.Index != 2 {
		t.Errorf("Index = %d, want 2", sme.Index)
	}
	if !errors.Is(err, ErrQueueXmtFull) {
		t.Error("expected errors.Is(err, ErrQueueXmtFull)")
	}
}

func TestSendMany_CtxCancel(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	frames := []Frame{}
	for i := 0; i < 5; i++ {
		fr, _ := NewFrame(uint32(0x100+i), nil)
		frames = append(frames, fr)
	}
	err := bus.SendMany(ctx, frames)
	var sme *SendManyError
	if !errors.As(err, &sme) {
		t.Fatalf("expected *SendManyError, got %v", err)
	}
	if sme.Index != 0 {
		t.Errorf("Index = %d, want 0", sme.Index)
	}
	if !errors.Is(sme.Err, context.Canceled) {
		t.Errorf("Err = %v, want context.Canceled", sme.Err)
	}
}

func TestStatus_OK(t *testing.T) {
	f := newFakeAdapter()
	f.statusReturn = raw.PCAN_ERROR_OK
	bus := openTest(t, f)
	defer bus.Close()
	s, err := bus.Status()
	if err != nil {
		t.Fatal(err)
	}
	if s != StatusOK {
		t.Errorf("status = 0x%X, want OK", uint32(s))
	}
}

func TestStatus_AfterClose(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	bus.Close()
	_, err := bus.Status()
	if !errors.Is(err, ErrBusClosed) {
		t.Errorf("got %v, want ErrBusClosed", err)
	}
}

func TestReset_OK(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	defer bus.Close()
	if err := bus.Reset(); err != nil {
		t.Fatal(err)
	}
	if f.resetCalls != 1 {
		t.Errorf("resetCalls = %d, want 1", f.resetCalls)
	}
}

func TestReset_AfterClose(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	bus.Close()
	if err := bus.Reset(); !errors.Is(err, ErrBusClosed) {
		t.Errorf("got %v, want ErrBusClosed", err)
	}
}

// dataLenToDLC / dlcToDataLen 是 FD 长度编码相关的内部函数。
func TestDataLenDLC_Roundtrip(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 20, 24, 32, 48, 64} {
		dlc := dataLenToDLC(n)
		back := dlcToDataLen(dlc)
		if back != n {
			t.Errorf("len=%d → dlc=%d → back=%d", n, dlc, back)
		}
	}
}

// ---- reader / Receive / ReadOne / TryRead ----

func TestReader_SuppressQRCVEMPTY(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f, WithPollInterval(2*time.Millisecond))
	defer bus.Close()

	time.Sleep(20 * time.Millisecond)

	select {
	case e := <-bus.Errors():
		t.Fatalf("errCh should not receive QRCVEMPTY: %v", e)
	default:
	}
}

func TestReader_DrainsQueue(t *testing.T) {
	f := newFakeAdapter()
	for i := 0; i < 5; i++ {
		f.push(raw.TPCANMsg{ID: uint32(0x100 + i), Len: 1, Data: [8]byte{byte(i)}})
	}
	bus := openTest(t, f, WithPollInterval(time.Millisecond))
	defer bus.Close()

	got := 0
	deadline := time.After(500 * time.Millisecond)
	for got < 5 {
		select {
		case fr := <-bus.Receive():
			if fr.ID != uint32(0x100+got) {
				t.Errorf("ID[%d] = 0x%X", got, fr.ID)
			}
			got++
		case <-deadline:
			t.Fatalf("timeout after %d frames", got)
		}
	}
}

func TestReader_FlagsRoundtrip(t *testing.T) {
	f := newFakeAdapter()
	f.push(raw.TPCANMsg{
		ID:      0x1FFFFFFF,
		MsgType: raw.PCAN_MESSAGE_EXTENDED | raw.PCAN_MESSAGE_RTR,
		Len:     4,
	})
	bus := openTest(t, f, WithPollInterval(time.Millisecond))
	defer bus.Close()

	select {
	case fr := <-bus.Receive():
		if !fr.Has(FlagExtended) || !fr.Has(FlagRemote) {
			t.Errorf("missing flags: %b", fr.Flags)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for frame")
	}
}

func TestReader_FDFrame(t *testing.T) {
	f := newFakeAdapter()
	f.pushFD(raw.TPCANMsgFD{
		ID:      0x123,
		MsgType: raw.PCAN_MESSAGE_FD | raw.PCAN_MESSAGE_BRS,
		DLC:     10, // = 16 bytes
	})
	bus := openTestFD(t, f, WithPollInterval(time.Millisecond))
	defer bus.Close()

	select {
	case fr := <-bus.Receive():
		if !fr.Has(FlagFD) || !fr.Has(FlagBRS) {
			t.Errorf("missing flags: %b", fr.Flags)
		}
		if len(fr.Data) != 16 {
			t.Errorf("len(Data) = %d, want 16", len(fr.Data))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for FD frame")
	}
}

func TestReadOne_OK(t *testing.T) {
	f := newFakeAdapter()
	f.push(raw.TPCANMsg{ID: 0x123, Len: 2, Data: [8]byte{1, 2}})
	bus := openTest(t, f, WithPollInterval(time.Millisecond))
	defer bus.Close()

	fr, err := bus.ReadOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fr.ID != 0x123 || len(fr.Data) != 2 {
		t.Errorf("bad frame: %+v", fr)
	}
	if fr.ReceivedAt.IsZero() {
		t.Error("expected ReceivedAt to be set")
	}
}

func TestReadOne_CtxCancel(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f, WithPollInterval(50*time.Millisecond))
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := bus.ReadOne(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

func TestReadOne_AfterClose(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	bus.Close()
	_, err := bus.ReadOne(context.Background())
	if !errors.Is(err, ErrBusClosed) {
		t.Errorf("got %v, want ErrBusClosed", err)
	}
}

func TestTryRead_Empty(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f, WithPollInterval(50*time.Millisecond))
	defer bus.Close()
	_, err := bus.TryRead()
	if !errors.Is(err, ErrQueueEmpty) {
		t.Errorf("got %v, want ErrQueueEmpty", err)
	}
}

func TestTryRead_HasFrame(t *testing.T) {
	f := newFakeAdapter()
	f.push(raw.TPCANMsg{ID: 0x42, Len: 1, Data: [8]byte{0xAB}})
	bus := openTest(t, f, WithPollInterval(time.Millisecond))
	defer bus.Close()

	// 等 reader 把帧推到 rxCh。
	var fr Frame
	deadline := time.After(200 * time.Millisecond)
	for {
		var err error
		fr, err = bus.TryRead()
		if err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("never got frame: %v", err)
		case <-time.After(5 * time.Millisecond):
		}
	}
	if fr.ID != 0x42 {
		t.Errorf("ID = 0x%X, want 0x42", fr.ID)
	}
}

func TestTryRead_AfterClose(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	bus.Close()
	_, err := bus.TryRead()
	if !errors.Is(err, ErrBusClosed) {
		t.Errorf("got %v, want ErrBusClosed", err)
	}
}

func TestReceive_AfterClose(t *testing.T) {
	f := newFakeAdapter()
	bus := openTest(t, f)
	bus.Close()

	fr, ok := <-bus.Receive()
	if ok {
		t.Errorf("expected closed channel, got frame %+v", fr)
	}
}

func TestReader_PropagatesPCANError(t *testing.T) {
	// 注入一个非 OK / 非 QRCVEMPTY 的状态，应该出现在 Errors() 里。
	// 这里用一个 fakeAdapter 子类型不方便，直接修改 read 路径太重，
	// 改用：先 push 正常帧让 reader 取走，再切换 statusReturn 不会改变 Read 行为；
	// 简单做法：直接构造一种 adapter 让 Read 返回 BUSOFF。
	f := &errInjectingAdapter{readErr: raw.PCAN_ERROR_BUSOFF}
	bus, err := openWith(f, USBBus1, false, "", WithPollInterval(time.Millisecond))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer bus.Close()

	select {
	case e := <-bus.Errors():
		if !errors.Is(e, ErrBusOff) {
			t.Errorf("got %v, want ErrBusOff", e)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected BUSOFF on Errors()")
	}
}

func TestConcurrent_SendReceive(t *testing.T) {
	f := newFakeAdapter()
	for i := 0; i < 50; i++ {
		f.push(raw.TPCANMsg{ID: uint32(i), Len: 1, Data: [8]byte{byte(i)}})
	}
	bus := openTest(t, f, WithPollInterval(time.Millisecond))
	defer bus.Close()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			fr, _ := NewFrame(uint32(i), []byte{byte(i)})
			_ = bus.Send(context.Background(), fr)
		}
		close(done)
	}()
	received := 0
	deadline := time.After(1 * time.Second)
	for received < 50 {
		select {
		case <-bus.Receive():
			received++
		case <-deadline:
			t.Fatalf("only got %d frames", received)
		}
	}
	<-done
}

// errInjectingAdapter 是一个最小 rawAdapter，仅用于让 Read 持续返回特定错误。
type errInjectingAdapter struct {
	readErr raw.TPCANStatus
}

func (a *errInjectingAdapter) Initialize(raw.TPCANHandle, raw.TPCANBaudrate) raw.TPCANStatus {
	return raw.PCAN_ERROR_OK
}
func (a *errInjectingAdapter) InitializeFD(raw.TPCANHandle, string) raw.TPCANStatus {
	return raw.PCAN_ERROR_OK
}
func (a *errInjectingAdapter) Uninitialize(raw.TPCANHandle) raw.TPCANStatus {
	return raw.PCAN_ERROR_OK
}
func (a *errInjectingAdapter) Read(raw.TPCANHandle, *raw.TPCANMsg, *raw.TPCANTimestamp) raw.TPCANStatus {
	return a.readErr
}
func (a *errInjectingAdapter) ReadFD(raw.TPCANHandle, *raw.TPCANMsgFD, *raw.TPCANTimestampFD) raw.TPCANStatus {
	return a.readErr
}
func (a *errInjectingAdapter) Write(raw.TPCANHandle, *raw.TPCANMsg) raw.TPCANStatus {
	return raw.PCAN_ERROR_OK
}
func (a *errInjectingAdapter) WriteFD(raw.TPCANHandle, *raw.TPCANMsgFD) raw.TPCANStatus {
	return raw.PCAN_ERROR_OK
}
func (a *errInjectingAdapter) GetStatus(raw.TPCANHandle) raw.TPCANStatus {
	return raw.PCAN_ERROR_OK
}
func (a *errInjectingAdapter) GetErrorText(raw.TPCANStatus, uint16) (string, raw.TPCANStatus) {
	return "fake", raw.PCAN_ERROR_OK
}
func (a *errInjectingAdapter) Reset(raw.TPCANHandle) raw.TPCANStatus { return raw.PCAN_ERROR_OK }

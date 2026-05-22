package pcanbasic

import (
	"errors"
	"testing"

	"github.com/Crush251/pcanbasic_go/raw"
)

func TestError_Has_OKBoundary(t *testing.T) {
	e := &Error{Code: raw.PCAN_ERROR_BUSOFF}
	if e.Has(raw.PCAN_ERROR_OK) {
		t.Error("Has(OK) on non-OK error should be false")
	}

	ok := &Error{Code: raw.PCAN_ERROR_OK}
	if !ok.Has(raw.PCAN_ERROR_OK) {
		t.Error("Has(OK) on OK error should be true")
	}
}

func TestError_Has_SingleBit(t *testing.T) {
	e := &Error{Code: raw.PCAN_ERROR_BUSOFF}
	if !e.Has(raw.PCAN_ERROR_BUSOFF) {
		t.Error("expected BUSOFF bit set")
	}
	if e.Has(raw.PCAN_ERROR_QOVERRUN) {
		t.Error("expected QOVERRUN bit NOT set")
	}
}

func TestError_Has_MultipleBits(t *testing.T) {
	e := &Error{Code: raw.PCAN_ERROR_BUSOFF | raw.PCAN_ERROR_QOVERRUN}
	if !e.Has(raw.PCAN_ERROR_BUSOFF) {
		t.Error("expected BUSOFF bit set")
	}
	if !e.Has(raw.PCAN_ERROR_QOVERRUN) {
		t.Error("expected QOVERRUN bit set")
	}
	if e.Has(raw.PCAN_ERROR_BUSPASSIVE) {
		t.Error("expected BUSPASSIVE bit NOT set")
	}
}

func TestError_Is_Bitmask(t *testing.T) {
	e := &Error{Code: raw.PCAN_ERROR_BUSOFF | raw.PCAN_ERROR_QOVERRUN}
	if !errors.Is(e, ErrBusOff) {
		t.Error("expected errors.Is(e, ErrBusOff) to be true")
	}
	if !errors.Is(e, ErrQueueOverrun) {
		t.Error("expected errors.Is(e, ErrQueueOverrun) to be true")
	}
	if errors.Is(e, ErrBusPassive) {
		t.Error("expected errors.Is(e, ErrBusPassive) to be false")
	}
}

func TestError_Is_NotInitialized(t *testing.T) {
	e := &Error{Code: raw.PCAN_ERROR_INITIALIZE}
	if !errors.Is(e, ErrNotInitialized) {
		t.Errorf("expected ErrNotInitialized, got code 0x%X", uint32(e.Code))
	}
}

func TestError_Error_Format(t *testing.T) {
	e := &Error{Op: "CAN_Initialize", Code: raw.PCAN_ERROR_INITIALIZE, Msg: "channel not initialized"}
	got := e.Error()
	if got == "" {
		t.Error("Error() should not be empty")
	}

	e2 := &Error{Op: "CAN_Read", Code: raw.PCAN_ERROR_QRCVEMPTY}
	if e2.Error() == "" {
		t.Error("Error() without msg should still be non-empty")
	}
}

func TestSendManyError_Unwrap(t *testing.T) {
	inner := errors.New("boom")
	e := &SendManyError{Index: 3, Err: inner}
	if !errors.Is(e, inner) {
		t.Error("expected SendManyError to unwrap to inner error")
	}
	if e.Error() == "" {
		t.Error("expected non-empty Error()")
	}
}

func TestSendManyError_IsThroughUnwrap(t *testing.T) {
	pcanErr := &Error{Code: raw.PCAN_ERROR_QXMTFULL}
	e := &SendManyError{Index: 1, Err: pcanErr}
	if !errors.Is(e, ErrQueueXmtFull) {
		t.Error("errors.Is should reach through SendManyError to *Error.Is")
	}
}

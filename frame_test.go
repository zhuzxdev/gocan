package pcanbasic

import (
	"errors"
	"testing"
)

func TestNewFrame_OK(t *testing.T) {
	f, err := NewFrame(0x123, []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.ID != 0x123 || len(f.Data) != 3 || f.Has(FlagExtended) {
		t.Errorf("bad frame: %+v", f)
	}
}

func TestNewFrame_IDOutOfRange(t *testing.T) {
	_, err := NewFrame(0x800, nil)
	if !errors.Is(err, ErrIDOutOfRange) {
		t.Errorf("got %v, want ErrIDOutOfRange", err)
	}
}

func TestNewFrame_DataTooLong(t *testing.T) {
	_, err := NewFrame(0x1, make([]byte, 9))
	if !errors.Is(err, ErrDataTooLong) {
		t.Errorf("got %v, want ErrDataTooLong", err)
	}
}

func TestNewFrame_DataIsCopied(t *testing.T) {
	src := []byte{1, 2, 3}
	f, _ := NewFrame(0x1, src)
	src[0] = 0xFF
	if f.Data[0] == 0xFF {
		t.Error("Data was not deep-copied")
	}
}

func TestNewFrame_EmptyData(t *testing.T) {
	f, err := NewFrame(0x123, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Data) != 0 {
		t.Errorf("len(Data) = %d, want 0", len(f.Data))
	}
}

func TestNewExtendedFrame_OK(t *testing.T) {
	f, err := NewExtendedFrame(0x1FFFFFFF, []byte{1, 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Has(FlagExtended) {
		t.Error("expected FlagExtended set")
	}
}

func TestNewExtendedFrame_IDOutOfRange(t *testing.T) {
	_, err := NewExtendedFrame(0x20000000, nil)
	if !errors.Is(err, ErrIDOutOfRange) {
		t.Errorf("got %v, want ErrIDOutOfRange", err)
	}
}

func TestNewRemoteFrame_OK(t *testing.T) {
	f, err := NewRemoteFrame(0x123, 4, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Has(FlagRemote) {
		t.Error("expected FlagRemote set")
	}
	if len(f.Data) != 4 {
		t.Errorf("len(Data) = %d, want 4", len(f.Data))
	}
}

func TestNewRemoteFrame_Extended(t *testing.T) {
	f, err := NewRemoteFrame(0x1FFFFFFF, 8, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Has(FlagRemote) || !f.Has(FlagExtended) {
		t.Errorf("expected Remote+Extended flags, got %b", f.Flags)
	}
}

func TestNewRemoteFrame_DLCTooLarge(t *testing.T) {
	_, err := NewRemoteFrame(0x123, 9, false)
	if !errors.Is(err, ErrDataTooLong) {
		t.Errorf("got %v, want ErrDataTooLong", err)
	}
}

func TestNewRemoteFrame_StdIDOutOfRange(t *testing.T) {
	_, err := NewRemoteFrame(0x800, 0, false)
	if !errors.Is(err, ErrIDOutOfRange) {
		t.Errorf("got %v, want ErrIDOutOfRange", err)
	}
}

func TestNewFDFrame_ValidLengths(t *testing.T) {
	for _, n := range []int{0, 1, 4, 8, 12, 16, 20, 24, 32, 48, 64} {
		_, err := NewFDFrame(0x1, make([]byte, n), false, false)
		if err != nil {
			t.Errorf("len=%d: unexpected error: %v", n, err)
		}
	}
}

func TestNewFDFrame_InvalidLengths(t *testing.T) {
	for _, n := range []int{9, 10, 11, 13, 17, 25, 65} {
		_, err := NewFDFrame(0x1, make([]byte, n), false, false)
		if !errors.Is(err, ErrInvalidFDLength) {
			t.Errorf("len=%d: got %v, want ErrInvalidFDLength", n, err)
		}
	}
}

func TestNewFDFrame_BRSFlag(t *testing.T) {
	f, err := NewFDFrame(0x1, []byte{1}, true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Has(FlagBRS) || !f.Has(FlagFD) {
		t.Error("expected FlagBRS and FlagFD set")
	}
}

func TestNewFDFrame_Extended(t *testing.T) {
	f, err := NewFDFrame(0x1FFFFFFF, []byte{1, 2, 3, 4}, false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Has(FlagExtended) || !f.Has(FlagFD) {
		t.Errorf("expected Extended+FD flags, got %b", f.Flags)
	}
}

func TestNewFDFrame_IDOutOfRange(t *testing.T) {
	_, err := NewFDFrame(0x20000000, []byte{1}, false, true)
	if !errors.Is(err, ErrIDOutOfRange) {
		t.Errorf("got %v, want ErrIDOutOfRange", err)
	}
}

func TestFrame_Has(t *testing.T) {
	f := Frame{Flags: FlagFD | FlagBRS}
	if !f.Has(FlagFD) {
		t.Error("expected FlagFD")
	}
	if !f.Has(FlagBRS) {
		t.Error("expected FlagBRS")
	}
	if f.Has(FlagExtended) {
		t.Error("did not expect FlagExtended")
	}
	if !f.Has(FlagFD | FlagBRS) {
		t.Error("expected Has(FD|BRS) to be true")
	}
}

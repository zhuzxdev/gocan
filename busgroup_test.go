package gocan

import (
	"errors"
	"testing"

	"github.com/Crush251/gocan/raw"
)

// withFakeOpener 替换 BusGroup 内部的 Open 钩子为基于 fakeAdapter 的版本，
// 并在测试结束时恢复原始 Open。返回 fakeAdapter 以便测试断言。
func withFakeOpener(t *testing.T) *fakeAdapter {
	t.Helper()
	fake := newFakeAdapter()
	prevOpen := busOpenFn
	prevOpenFD := busOpenFDFn
	busOpenFn = func(ch Channel, opts ...Option) (*Bus, error) {
		return openWith(fake, ch, false, "", opts...)
	}
	busOpenFDFn = func(ch Channel, br string, opts ...Option) (*Bus, error) {
		return openWith(fake, ch, true, br, opts...)
	}
	t.Cleanup(func() {
		busOpenFn = prevOpen
		busOpenFDFn = prevOpenFD
	})
	return fake
}

func TestBusGroup_AddGetNames(t *testing.T) {
	withFakeOpener(t)
	g := NewBusGroup(0)

	bus, err := g.Add("front", raw.PCAN_USBBUS1)
	if err != nil {
		t.Fatalf("Add front: %v", err)
	}
	if bus == nil {
		t.Fatal("Add returned nil bus")
	}
	got, ok := g.Get("front")
	if !ok || got != bus {
		t.Errorf("Get(front) = %v %v, want %v true", got, ok, bus)
	}
	if _, ok := g.Get("missing"); ok {
		t.Error("Get(missing) returned ok=true")
	}

	if _, err := g.Add("rear", raw.PCAN_USBBUS2); err != nil {
		t.Fatalf("Add rear: %v", err)
	}
	names := g.Names()
	if len(names) != 2 || names[0] != "front" || names[1] != "rear" {
		t.Errorf("Names() = %v, want [front rear]", names)
	}
	for _, b := range []string{"front", "rear"} {
		if got, _ := g.Get(b); got != nil {
			_ = got.Close()
		}
	}
}

func TestBusGroup_AddRejectsEmptyName(t *testing.T) {
	withFakeOpener(t)
	g := NewBusGroup(0)
	if _, err := g.Add("", raw.PCAN_USBBUS1); !errors.Is(err, ErrInvalidName) {
		t.Errorf("Add(\"\") err = %v, want ErrInvalidName", err)
	}
}

func TestBusGroup_AddRejectsDuplicate(t *testing.T) {
	withFakeOpener(t)
	g := NewBusGroup(0)
	bus, err := g.Add("a", raw.PCAN_USBBUS1)
	if err != nil {
		t.Fatalf("first Add: %v", err)
	}
	defer bus.Close()
	if _, err := g.Add("a", raw.PCAN_USBBUS2); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("duplicate Add err = %v, want ErrDuplicateName", err)
	}
}

func TestBusGroup_AddFD(t *testing.T) {
	withFakeOpener(t)
	g := NewBusGroup(0)
	bus, err := g.AddFD("fd", raw.PCAN_USBBUS1, "f_clock=80000000,nom_brp=10,nom_tseg1=12,nom_tseg2=3,nom_sjw=1")
	if err != nil {
		t.Fatalf("AddFD: %v", err)
	}
	defer bus.Close()
	if got, _ := g.Get("fd"); got != bus {
		t.Errorf("Get(fd) mismatch")
	}
}

func TestBusGroup_EachOrder(t *testing.T) {
	withFakeOpener(t)
	g := NewBusGroup(0)
	channels := map[string]Channel{
		"c": raw.PCAN_USBBUS1,
		"a": raw.PCAN_USBBUS2,
		"b": raw.PCAN_USBBUS3,
	}
	for _, n := range []string{"c", "a", "b"} {
		if _, err := g.Add(n, channels[n]); err != nil {
			t.Fatalf("Add %s: %v", n, err)
		}
	}
	var seen []string
	g.Each(func(name string, _ *Bus) { seen = append(seen, name) })
	if len(seen) != 3 || seen[0] != "a" || seen[1] != "b" || seen[2] != "c" {
		t.Errorf("Each order = %v, want [a b c]", seen)
	}
}

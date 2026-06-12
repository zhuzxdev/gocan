//go:build linux

package gocan

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Crush251/gocan/raw"
)

// vcanIface 返回一个可用 vcan 接口名；不存在则跳过测试。
func vcanIface(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"vcan0", "vcan1"} {
		if _, err := net.InterfaceByName(name); err == nil {
			return name
		}
	}
	t.Skip("vcan unavailable: create with scripts/setup-vcan.sh")
	return ""
}

func TestSocketCANIntegration_LoopbackRecvOwn(t *testing.T) {
	iface := vcanIface(t)
	bus, err := Open(SocketCAN(iface),
		WithLoopback(true),
		WithRecvOwnMsgs(true),
		WithRxBufferSize(64),
	)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer bus.Close()

	frame, _ := NewFrame(0x123, []byte{0xDE, 0xAD})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := bus.Send(ctx, frame); err != nil {
		t.Fatalf("Send: %v", err)
	}
	got, err := bus.ReadOne(ctx)
	if err != nil {
		t.Fatalf("ReadOne: %v", err)
	}
	if got.ID != 0x123 || len(got.Data) != 2 || got.Data[0] != 0xDE || got.Data[1] != 0xAD {
		t.Errorf("frame mismatch: %+v", got)
	}
}

func TestSocketCANIntegration_TimestampPopulated(t *testing.T) {
	iface := vcanIface(t)
	bus, err := Open(SocketCAN(iface),
		WithLoopback(true),
		WithRecvOwnMsgs(true),
		WithRecvTimestamp(RxTimestampNano),
	)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer bus.Close()

	frame, _ := NewFrame(0x42, []byte{1, 2, 3})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	before := time.Now().UnixMicro()
	if err := bus.Send(ctx, frame); err != nil {
		t.Fatalf("Send: %v", err)
	}
	got, err := bus.ReadOne(ctx)
	if err != nil {
		t.Fatalf("ReadOne: %v", err)
	}
	if got.TimestampMicros == 0 {
		t.Error("TimestampMicros = 0, want kernel timestamp")
	}
	if int64(got.TimestampMicros) < before {
		t.Errorf("TimestampMicros %d earlier than send time %d", got.TimestampMicros, before)
	}
}

func TestSocketCANIntegration_SetErrFilterNoError(t *testing.T) {
	iface := vcanIface(t)
	bus, err := Open(SocketCAN(iface))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer bus.Close()
	if err := bus.SetErrFilter(raw.CANErrBusOff); err != nil {
		t.Errorf("SetErrFilter: %v", err)
	}
	if err := bus.SetErrFilter(0); err != nil {
		t.Errorf("SetErrFilter(0): %v", err)
	}
}

func TestSocketCANIntegration_BusGroupFanIn(t *testing.T) {
	iface0 := vcanIface(t)
	if _, err := net.InterfaceByName("vcan1"); err != nil {
		t.Skip("vcan1 unavailable: skip multi-bus test")
	}
	g := NewBusGroup(8)
	if _, err := g.AddFD("a", SocketCAN(iface0), "",
		WithLoopback(true), WithRecvOwnMsgs(true)); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if _, err := g.AddFD("b", SocketCAN("vcan1"), "",
		WithLoopback(true), WithRecvOwnMsgs(true)); err != nil {
		t.Fatalf("Add b: %v", err)
	}
	defer g.Close()

	for _, name := range []string{"a", "b"} {
		bus, _ := g.Get(name)
		f, _ := NewFrame(0x100, []byte{byte(name[0])})
		if err := bus.Send(context.Background(), f); err != nil {
			t.Fatalf("Send on %s: %v", name, err)
		}
	}
	deadline := time.After(2 * time.Second)
	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case sf := <-g.Receive():
			got[sf.Source] = true
		case <-deadline:
			t.Fatalf("only got %v", got)
		}
	}

	// 显式触发已关闭后的 add 失败语义。
	if err := g.Close(); err != nil {
		var gce *GroupCloseError
		if !errors.As(err, &gce) {
			t.Errorf("Close err = %v, want *GroupCloseError or nil", err)
		}
	}
}

//go:build linux

package gocan

import (
	"testing"
	"time"
)

func TestWithLoopback_SetsField(t *testing.T) {
	cfg := newDefaultConfig()
	WithLoopback(false)(cfg)
	if cfg.linux.loopback == nil || *cfg.linux.loopback != false {
		t.Errorf("loopback = %v, want pointer to false", cfg.linux.loopback)
	}
}

func TestWithRecvOwnMsgs_SetsField(t *testing.T) {
	cfg := newDefaultConfig()
	WithRecvOwnMsgs(true)(cfg)
	if cfg.linux.recvOwnMsgs == nil || *cfg.linux.recvOwnMsgs != true {
		t.Errorf("recvOwnMsgs = %v, want pointer to true", cfg.linux.recvOwnMsgs)
	}
}

func TestWithErrFilter_SetsField(t *testing.T) {
	cfg := newDefaultConfig()
	WithErrFilter(CANErrBusOff | CANErrTxTimeout)(cfg)
	if cfg.linux.errFilter == nil {
		t.Fatal("errFilter is nil")
	}
	want := uint32(CANErrBusOff | CANErrTxTimeout)
	if *cfg.linux.errFilter != want {
		t.Errorf("errFilter = 0x%X, want 0x%X", *cfg.linux.errFilter, want)
	}
}

func TestWithJoinFilters_SetsField(t *testing.T) {
	cfg := newDefaultConfig()
	WithJoinFilters(true)(cfg)
	if cfg.linux.joinFilters == nil || *cfg.linux.joinFilters != true {
		t.Errorf("joinFilters = %v, want true", cfg.linux.joinFilters)
	}
}

func TestWithRecvTimestamp_SetsField(t *testing.T) {
	cfg := newDefaultConfig()
	WithRecvTimestamp(RxTimestampNano)(cfg)
	if cfg.linux.rxTimestamp != RxTimestampNano {
		t.Errorf("rxTimestamp = %v, want %v", cfg.linux.rxTimestamp, RxTimestampNano)
	}
}

func TestWithSocketBuffers_SetsFields(t *testing.T) {
	cfg := newDefaultConfig()
	WithSocketBuffers(64*1024, 32*1024)(cfg)
	if cfg.linux.soRcvBuf != 64*1024 {
		t.Errorf("soRcvBuf = %d, want %d", cfg.linux.soRcvBuf, 64*1024)
	}
	if cfg.linux.soSndBuf != 32*1024 {
		t.Errorf("soSndBuf = %d, want %d", cfg.linux.soSndBuf, 32*1024)
	}
}

func TestWithRWTimeout_SetsFields(t *testing.T) {
	cfg := newDefaultConfig()
	WithRWTimeout(500*time.Millisecond, 250*time.Millisecond)(cfg)
	if cfg.linux.readTimeout != 500*time.Millisecond {
		t.Errorf("readTimeout = %v", cfg.linux.readTimeout)
	}
	if cfg.linux.writeTimeout != 250*time.Millisecond {
		t.Errorf("writeTimeout = %v", cfg.linux.writeTimeout)
	}
}

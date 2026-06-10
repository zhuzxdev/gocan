//go:build linux

package gocan

import "testing"

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

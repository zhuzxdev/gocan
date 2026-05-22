package pcanbasic

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	c := newDefaultConfig()
	if c.bitrate != Baud1M {
		t.Errorf("bitrate = %v, want Baud1M", c.bitrate)
	}
	if c.receiveMode != ModeAuto {
		t.Errorf("receiveMode = %v, want ModeAuto", c.receiveMode)
	}
	if c.pollInterval != time.Millisecond {
		t.Errorf("pollInterval = %v, want 1ms", c.pollInterval)
	}
	if c.rxBufferSize != 1024 {
		t.Errorf("rxBufferSize = %d, want 1024", c.rxBufferSize)
	}
	if c.errBufferSize != 16 {
		t.Errorf("errBufferSize = %d, want 16", c.errBufferSize)
	}
	if c.logger == nil {
		t.Error("logger should default to noopLogger")
	}
}

func TestOption_Apply(t *testing.T) {
	c := newDefaultConfig()
	WithBitrate(Baud500K)(c)
	WithReceiveMode(ModeEvent)(c)
	WithPollInterval(5 * time.Millisecond)(c)
	WithRxBufferSize(4096)(c)
	WithErrBufferSize(64)(c)

	if c.bitrate != Baud500K {
		t.Error("bitrate not applied")
	}
	if c.receiveMode != ModeEvent {
		t.Error("receiveMode not applied")
	}
	if c.pollInterval != 5*time.Millisecond {
		t.Error("pollInterval not applied")
	}
	if c.rxBufferSize != 4096 {
		t.Error("rxBufferSize not applied")
	}
	if c.errBufferSize != 64 {
		t.Error("errBufferSize not applied")
	}
}

func TestOption_RejectsNonPositive(t *testing.T) {
	c := newDefaultConfig()
	WithPollInterval(0)(c)
	WithRxBufferSize(0)(c)
	WithErrBufferSize(-1)(c)
	WithLogger(nil)(c)

	if c.pollInterval != time.Millisecond {
		t.Error("pollInterval should ignore non-positive")
	}
	if c.rxBufferSize != 1024 {
		t.Error("rxBufferSize should ignore non-positive")
	}
	if c.errBufferSize != 16 {
		t.Error("errBufferSize should ignore non-positive")
	}
	if c.logger == nil {
		t.Error("logger should ignore nil")
	}
}

// 自定义 Logger 实现，验证注入路径。
type recordLogger struct {
	debugN, infoN, warnN int
}

func (r *recordLogger) Debugf(string, ...any) { r.debugN++ }
func (r *recordLogger) Infof(string, ...any)  { r.infoN++ }
func (r *recordLogger) Warnf(string, ...any)  { r.warnN++ }

func TestWithLogger_Injects(t *testing.T) {
	rl := &recordLogger{}
	c := newDefaultConfig()
	WithLogger(rl)(c)
	if c.logger != rl {
		t.Error("logger not injected")
	}
}

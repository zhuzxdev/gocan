//go:build linux

package raw

import (
	"testing"
	"time"
)

// 这些测试不依赖真实 vcan：它们走 SocketCANHandle 注册路径但绑定到不存在
// 的接口，Initialize 会失败。我们只测函数对未初始化通道的错误码正确。
func TestSetCANRawSockoptInt_UninitializedChannel(t *testing.T) {
	ch := SocketCANHandle("__nonexistent_test_iface__")
	if got := SetCANRawSockoptInt(ch, 0, 0); got != PCAN_ERROR_INITIALIZE {
		t.Errorf("got %v, want PCAN_ERROR_INITIALIZE", got)
	}
}

func TestSetSocketBuffers_NoOpOnZero(t *testing.T) {
	ch := SocketCANHandle("__nonexistent_test_iface__")
	if got := SetSocketBuffers(ch, 0, 0); got != PCAN_ERROR_INITIALIZE {
		t.Errorf("got %v, want PCAN_ERROR_INITIALIZE", got)
	}
}

func TestSetReadWriteTimeout_NoOpOnZero(t *testing.T) {
	ch := SocketCANHandle("__nonexistent_test_iface__")
	if got := SetReadWriteTimeout(ch, 0, 0); got != PCAN_ERROR_INITIALIZE {
		t.Errorf("got %v, want PCAN_ERROR_INITIALIZE", got)
	}
}

func TestUpdateLinuxChannel_NotInRegistry(t *testing.T) {
	// 构造一个孤立的 *linuxChannel 指针，registry 中没有它的句柄。
	// updateLinuxChannel 应静默 no-op（既不 panic，也不 mutate）。
	c := &linuxChannel{}
	called := false
	updateLinuxChannel(SocketCANHandle("__nonexistent_test_iface__"), c, func(*linuxChannel) {
		called = true
	})
	if called {
		t.Error("mut should not be called when channel is not in registry")
	}
}

var _ = time.Second // 保持 import 不报错

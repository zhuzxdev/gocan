package gocan

import (
	"sort"
	"sync"
	"sync/atomic"
)

const defaultGroupOutSize = 1024

// SourcedFrame 是 BusGroup.Receive 返回的合流帧，
// 把帧和发出它的 Bus 名字打包在一起。
type SourcedFrame struct {
	Source string
	Frame  Frame
}

// BusGroup 管理一组按业务名字索引的 *Bus。
// 零值不可用，必须通过 NewBusGroup 构造。所有方法并发安全。
type BusGroup struct {
	mu        sync.RWMutex
	buses     map[string]*Bus
	out       chan SourcedFrame
	closing   chan struct{}
	closed    atomic.Bool
	fanWg     sync.WaitGroup
	closeOnce sync.Once
}

// busOpenFn / busOpenFDFn 是 Open / OpenFD 的可替换钩子，仅供测试注入 fake adapter。
// 生产代码外部不应替换。
var (
	busOpenFn   = Open
	busOpenFDFn = OpenFD
)

// NewBusGroup 创建空 group。
// outBufferSize 是 Receive 合并 channel 的容量；非正值用默认 1024。
func NewBusGroup(outBufferSize int) *BusGroup {
	if outBufferSize <= 0 {
		outBufferSize = defaultGroupOutSize
	}
	return &BusGroup{
		buses:   make(map[string]*Bus),
		out:     make(chan SourcedFrame, outBufferSize),
		closing: make(chan struct{}),
	}
}

// Add 打开一个 Classical CAN 通道并以 name 加入 group。
//   - name 为空 → ErrInvalidName
//   - name 重复 → ErrDuplicateName
//   - 底层 Open 失败 → 返回相应错误，group 状态不变
//   - group 已 Close → ErrBusClosed
func (g *BusGroup) Add(name string, ch Channel, opts ...Option) (*Bus, error) {
	return g.add(name, false, "", ch, opts...)
}

// AddFD 等价于 Add，但调底层 OpenFD。
func (g *BusGroup) AddFD(name string, ch Channel, fdBitrate string, opts ...Option) (*Bus, error) {
	return g.add(name, true, fdBitrate, ch, opts...)
}

func (g *BusGroup) add(name string, fd bool, fdBitrate string, ch Channel, opts ...Option) (*Bus, error) {
	if name == "" {
		return nil, ErrInvalidName
	}
	if g.closed.Load() {
		return nil, ErrBusClosed
	}
	g.mu.RLock()
	_, exists := g.buses[name]
	g.mu.RUnlock()
	if exists {
		return nil, ErrDuplicateName
	}

	var (
		bus *Bus
		err error
	)
	if fd {
		bus, err = busOpenFDFn(ch, fdBitrate, opts...)
	} else {
		bus, err = busOpenFn(ch, opts...)
	}
	if err != nil {
		return nil, err
	}

	g.mu.Lock()
	if g.closed.Load() {
		g.mu.Unlock()
		_ = bus.Close()
		return nil, ErrBusClosed
	}
	if _, exists := g.buses[name]; exists {
		g.mu.Unlock()
		_ = bus.Close()
		return nil, ErrDuplicateName
	}
	g.buses[name] = bus
	g.mu.Unlock()
	return bus, nil
}

// Get 按名字取 Bus；不存在返回 nil, false。
func (g *BusGroup) Get(name string) (*Bus, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	bus, ok := g.buses[name]
	return bus, ok
}

// Names 返回当前 group 内所有 Bus 名字的拷贝（已排序）。
func (g *BusGroup) Names() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	names := make([]string, 0, len(g.buses))
	for n := range g.buses {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Each 在持有读锁下按 Names 顺序遍历每个 Bus。
// fn 内禁止调 Add/AddFD/Close —— 会死锁，race 模式下会被检测出来。
func (g *BusGroup) Each(fn func(name string, bus *Bus)) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	names := make([]string, 0, len(g.buses))
	for n := range g.buses {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fn(n, g.buses[n])
	}
}

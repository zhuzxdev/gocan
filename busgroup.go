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
	g.fanWg.Add(1)
	go g.fanIn(name, bus)
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

// Receive 返回合并接收 channel；group 关闭时该 channel 也关闭。
// 反压策略：写入 out 阻塞 → 反压回对应 Bus 的 fan-in goroutine
// （不丢帧；需要更高吞吐时调大 NewBusGroup 的 outBufferSize）。
func (g *BusGroup) Receive() <-chan SourcedFrame { return g.out }

// fanIn 把单个 Bus 的接收帧打上来源标签转发到 g.out。
// closing 关闭或 Bus rxCh 关闭时退出。
func (g *BusGroup) fanIn(name string, bus *Bus) {
	defer g.fanWg.Done()
	rx := bus.Receive()
	for {
		select {
		case <-g.closing:
			return
		case f, ok := <-rx:
			if !ok {
				return
			}
			select {
			case <-g.closing:
				return
			case g.out <- SourcedFrame{Source: name, Frame: f}:
			}
		}
	}
}

// Close 并发关闭所有 Bus，聚合错误。幂等。
// 关闭顺序：closing → 等所有 fan-in 退出 → Close 每个 Bus → close(out)。
// 返回非 nil 时一定是 *GroupCloseError。
func (g *BusGroup) Close() error {
	var causes map[string]error
	g.closeOnce.Do(func() {
		g.closed.Store(true)
		close(g.closing)
		g.fanWg.Wait()

		g.mu.Lock()
		defer g.mu.Unlock()
		causes = make(map[string]error)
		for name, bus := range g.buses {
			if err := bus.Close(); err != nil {
				causes[name] = err
			}
		}
		close(g.out)
	})
	if len(causes) == 0 {
		return nil
	}
	return &GroupCloseError{Causes: causes}
}

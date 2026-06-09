# BusGroup 与 SocketCAN 自定义参数 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 gocan 加 BusGroup 多通道管理器，并补齐 Linux SocketCAN 后端的 7 个常用 setsockopt 选项与 2 个运行期可调方法，配 4 个示例 + 整套文档。

**Architecture:** BusGroup 在顶层包新增，按业务名字索引一组 *Bus，提供合流接收和聚合关闭；Linux Option 通过 build-tag 隔离的 `linuxConfig` + `applyPlatformOptions` 钩子在 `Open` 之后、`startReader` 之前一次性应用；`raw` 子包加薄 setsockopt 封装，时间戳路径从 `read()` 切到 `recvmsg()`；其他平台用 `_other.go` 提供 ErrNotSupported 占位以保跨平台编译。

**Tech Stack:** Go 1.22 标准库；`golang.org/x/sys/unix`（已有依赖）；vcan 用 `iproute2`（仅本地 / 集成测试需要）。

**Spec:** `docs/superpowers/specs/2026-06-08-busgroup-and-socketcan-options-design.md`

---

## File Structure

新增：

- `busgroup.go` — `BusGroup` 类型、`SourcedFrame`、`NewBusGroup`、`Add`/`AddFD`/`Get`/`Names`/`Each`/`Receive`/`Close`
- `busgroup_test.go` — fakeAdapter 单元
- `options_linux.go` — Linux 专属 Option（带 `//go:build linux`）
- `options_linux_test.go`
- `bus_platform_linux.go` / `bus_platform_other.go` — `applyPlatformOptions` 钩子按平台分发
- `bus_socketcan_linux.go` / `bus_socketcan_other.go` — `*Bus` 的 `SetErrFilter` / `SetJoinFilters` 方法（其他平台 `ErrNotSupported`）
- `socketcan_options_integration_linux_test.go` — 真 vcan 集成测试，无 vcan 时 `t.Skip`
- `raw/socketcan_options_linux.go` — 薄封装 setsockopt(SOL_CAN_RAW, …) 等
- `raw/socketcan_options_linux_test.go`
- `raw/can_err_linux.go` — `CAN_ERR_*` 常量
- `examples/11_busgroup_socketcan/main.go`
- `examples/12_busgroup_fan_in/main.go`
- `examples/13_socketcan_loopback/main.go`
- `examples/14_socketcan_advanced/main.go`
- `scripts/setup-vcan.sh`
- `docs/socketcan-options.md`

修改（仅加字段/方法/钩子调用，不破坏既有签名）：

- `errors.go` — 加 `ErrInvalidName`、`ErrDuplicateName`、`*GroupCloseError`
- `options.go` — `config` 加 `linux linuxConfig` 字段
- `bus.go` — `openWith` 在 `Initialize` 成功后、`startReader` 之前调一行 `applyPlatformOptions(b, cfg)`
- `raw/socketcan_linux.go` — `linuxChannel` 加几个字段持久化已应用 sockopt；`Read`/`ReadFD` 在启用 `WithRecvTimestamp` 时切到 `recvmsg`
- `justfile` — 加 `vcan-up` / `vcan-down` 别名

---

## Task List

1. Sentinel errors + GroupCloseError
2. BusGroup 核心（NewBusGroup / Add / Get / Names）
3. BusGroup AddFD
4. BusGroup Each
5. BusGroup Receive 合流
6. BusGroup Close 聚合错误
7. 平台 Option 管线（linuxConfig + applyPlatformOptions 钩子）
8. raw setsockopt 薄封装
9. WithLoopback + WithRecvOwnMsgs
10. WithErrFilter + raw/can_err_linux.go
11. WithJoinFilters
12. WithRecvTimestamp + recvmsg 路径
13. WithSocketBuffers + WithRWTimeout
14. SetErrFilter / SetJoinFilters 运行期方法
15. vcan 集成测试
16. setup-vcan.sh + justfile 别名
17. example 11 — busgroup_socketcan
18. example 12 — busgroup_fan_in
19. example 13 — socketcan_loopback
20. example 14 — socketcan_advanced
21. docs/socketcan-options.md

## Task 1: Sentinel errors + GroupCloseError

**Files:**
- Modify: `errors.go`
- Test: `errors_test.go`（已有，追加测试）

- [ ] **Step 1: Write failing test**

在 `errors_test.go` 末尾追加：

```go
func TestGroupCloseError_ErrorAndUnwrap(t *testing.T) {
	a := errors.New("a fail")
	b := ErrBusClosed
	gce := &GroupCloseError{Causes: map[string]error{"front": a, "rear": b}}

	msg := gce.Error()
	if !strings.Contains(msg, "front") || !strings.Contains(msg, "rear") {
		t.Errorf("Error() = %q, want both bus names", msg)
	}
	if !errors.Is(gce, ErrBusClosed) {
		t.Errorf("errors.Is(gce, ErrBusClosed) = false, want true")
	}
	unwrapped := gce.Unwrap()
	if len(unwrapped) != 2 {
		t.Errorf("Unwrap returned %d errors, want 2", len(unwrapped))
	}
}

func TestSentinelGroupErrors(t *testing.T) {
	if ErrInvalidName == nil || ErrDuplicateName == nil {
		t.Fatal("sentinel errors must be non-nil")
	}
	if ErrInvalidName.Error() == ErrDuplicateName.Error() {
		t.Error("ErrInvalidName and ErrDuplicateName must have distinct messages")
	}
}
```

确认 `errors_test.go` 顶部 import 包含 `strings` 和 `errors`。

- [ ] **Step 2: Run test to confirm failure**

```
go test ./... -run 'TestGroupCloseError|TestSentinelGroupErrors' -v
```

Expected: FAIL（`undefined: GroupCloseError` / `undefined: ErrInvalidName`）

- [ ] **Step 3: Implement in `errors.go`**

在文件末尾追加：

```go
// 多 Bus 群组错误。
var (
	// ErrInvalidName 表示 BusGroup.Add 收到的 name 为空字符串。
	ErrInvalidName = errors.New("gocan: invalid bus name")
	// ErrDuplicateName 表示 BusGroup 中已存在同名 Bus。
	ErrDuplicateName = errors.New("gocan: duplicate bus name in group")
)

// GroupCloseError 聚合 BusGroup.Close 时多个 Bus 的失败。
//
// Causes 按名字索引每个失败 Bus 的错误；成功关闭的 Bus 不出现在 map 中。
// errors.Is 会按 Causes 逐个尝试匹配，因此可以 errors.Is(err, ErrBusClosed) 等。
type GroupCloseError struct {
	Causes map[string]error
}

// Error 实现 error。
func (e *GroupCloseError) Error() string {
	names := make([]string, 0, len(e.Causes))
	for n := range e.Causes {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("gocan: BusGroup.Close failed for ")
	for i, n := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s: %v", n, e.Causes[n])
	}
	return b.String()
}

// Unwrap 返回所有底层错误，便于 errors.Is/errors.As 穿透。
func (e *GroupCloseError) Unwrap() []error {
	out := make([]error, 0, len(e.Causes))
	for _, err := range e.Causes {
		out = append(out, err)
	}
	return out
}
```

文件顶部 import 增加 `"sort"` 和 `"strings"`。

- [ ] **Step 4: Run test to confirm pass**

```
go test ./... -run 'TestGroupCloseError|TestSentinelGroupErrors' -v
go vet ./...
```

Expected: 全部 PASS。

- [ ] **Step 5: Commit**

```
git add errors.go errors_test.go
git commit -m "feat: add GroupCloseError and BusGroup sentinel errors"
```

## Task 2: BusGroup 核心（NewBusGroup / Add / Get / Names）

**Files:**
- Create: `busgroup.go`
- Create: `busgroup_test.go`

- [ ] **Step 1: Write `busgroup.go` skeleton + 公开类型**

```go
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
	mu       sync.RWMutex
	buses    map[string]*Bus
	out      chan SourcedFrame
	closing  chan struct{}
	closed   atomic.Bool
	fanWg    sync.WaitGroup
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
```

注意：fan-in goroutine、Each、Close 在后续 Task 中加。`closing` channel 已建好，留给 Task 5/6 用。

- [ ] **Step 2: Write tests in `busgroup_test.go`**

```go
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
```

- [ ] **Step 3: Run tests to confirm pass**

```
go test ./... -run TestBusGroup -race -v
go vet ./...
```

Expected: PASS。

- [ ] **Step 4: Commit**

```
git add busgroup.go busgroup_test.go
git commit -m "feat: add BusGroup core (NewBusGroup/Add/Get/Names)"
```

## Task 3: BusGroup AddFD

**Files:**
- Modify: `busgroup.go`
- Modify: `busgroup_test.go`

- [ ] **Step 1: Write failing test**

在 `busgroup_test.go` 末尾追加：

```go
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
```

- [ ] **Step 2: Run to confirm failure**

```
go test ./... -run TestBusGroup_AddFD -v
```

Expected: FAIL（`undefined: g.AddFD`）

- [ ] **Step 3: Implement in `busgroup.go`**

在 `Add` 之后添加：

```go
// AddFD 等价于 Add，但调底层 OpenFD。
func (g *BusGroup) AddFD(name string, ch Channel, fdBitrate string, opts ...Option) (*Bus, error) {
	return g.add(name, true, fdBitrate, ch, opts...)
}
```

- [ ] **Step 4: Run to confirm pass**

```
go test ./... -run TestBusGroup -race -v
```

Expected: PASS。

- [ ] **Step 5: Commit**

```
git add busgroup.go busgroup_test.go
git commit -m "feat: add BusGroup.AddFD"
```

## Task 4: BusGroup.Each

**Files:**
- Modify: `busgroup.go`
- Modify: `busgroup_test.go`

- [ ] **Step 1: Write failing test**

```go
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
```

- [ ] **Step 2: Run to confirm failure**

```
go test ./... -run TestBusGroup_EachOrder -v
```

Expected: FAIL（`undefined: g.Each`）。

- [ ] **Step 3: Implement in `busgroup.go`**

```go
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
```

- [ ] **Step 4: Run to confirm pass**

```
go test ./... -run TestBusGroup -race -v
```

Expected: PASS。

- [ ] **Step 5: Commit**

```
git add busgroup.go busgroup_test.go
git commit -m "feat: add BusGroup.Each"
```

## Task 5: BusGroup.Receive 合流（fan-in）

**Files:**
- Modify: `busgroup.go`
- Modify: `busgroup_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestBusGroup_ReceiveFanIn(t *testing.T) {
	fake := withFakeOpener(t)
	g := NewBusGroup(8)

	if _, err := g.Add("a", raw.PCAN_USBBUS1); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if _, err := g.Add("b", raw.PCAN_USBBUS2); err != nil {
		t.Fatalf("Add b: %v", err)
	}

	fake.push(raw.TPCANMsg{ID: 0x111, Len: 1, Data: [8]byte{0xAA}})
	fake.push(raw.TPCANMsg{ID: 0x222, Len: 1, Data: [8]byte{0xBB}})

	got := map[string]uint32{}
	for i := 0; i < 2; i++ {
		select {
		case sf := <-g.Receive():
			got[sf.Source] = sf.Frame.ID
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for frame %d", i)
		}
	}
	if len(got) == 0 {
		t.Fatal("no frames received")
	}
	for src, id := range got {
		if id != 0x111 && id != 0x222 {
			t.Errorf("source %s id=0x%X unexpected", src, id)
		}
	}
}
```

`busgroup_test.go` 顶部 import 加 `"time"`。

- [ ] **Step 2: Run to confirm failure**

```
go test ./... -run TestBusGroup_ReceiveFanIn -v -timeout 30s
```

Expected: FAIL（`undefined: g.Receive`）。

- [ ] **Step 3: Implement in `busgroup.go`**

修改 `add` 方法，把成功路径末尾改为：

```go
	g.buses[name] = bus
	g.fanWg.Add(1)
	go g.fanIn(name, bus)
	g.mu.Unlock()
	return bus, nil
```

新增方法：

```go
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
```

- [ ] **Step 4: Run to confirm pass**

```
go test ./... -run TestBusGroup -race -v -timeout 30s
```

Expected: PASS。

- [ ] **Step 5: Commit**

```
git add busgroup.go busgroup_test.go
git commit -m "feat: add BusGroup.Receive fan-in"
```

## Task 6: BusGroup.Close 聚合错误

**Files:**
- Modify: `busgroup.go`
- Modify: `busgroup_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestBusGroup_CloseAggregates(t *testing.T) {
	fake := withFakeOpener(t)
	g := NewBusGroup(0)
	if _, err := g.Add("a", raw.PCAN_USBBUS1); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Add("b", raw.PCAN_USBBUS2); err != nil {
		t.Fatal(err)
	}
	fake.uninitializeReturn = raw.PCAN_ERROR_ILLPARAMVAL
	err := g.Close()
	if err == nil {
		t.Fatal("Close returned nil, want aggregate error")
	}
	gce, ok := err.(*GroupCloseError)
	if !ok {
		t.Fatalf("Close returned %T, want *GroupCloseError", err)
	}
	if len(gce.Causes) != 2 {
		t.Errorf("Causes has %d entries, want 2", len(gce.Causes))
	}
}

func TestBusGroup_CloseIdempotent(t *testing.T) {
	withFakeOpener(t)
	g := NewBusGroup(0)
	if _, err := g.Add("a", raw.PCAN_USBBUS1); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := g.Add("late", raw.PCAN_USBBUS2); !errors.Is(err, ErrBusClosed) {
		t.Errorf("Add after Close err = %v, want ErrBusClosed", err)
	}
}

func TestBusGroup_ReceiveClosesAfterClose(t *testing.T) {
	withFakeOpener(t)
	g := NewBusGroup(0)
	if _, err := g.Add("a", raw.PCAN_USBBUS1); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = g.Close()
	}()
	for range g.Receive() {
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```
go test ./... -run TestBusGroup_Close -v -timeout 30s
```

Expected: FAIL（`undefined: g.Close`）。

- [ ] **Step 3: Implement in `busgroup.go`**

```go
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
```

- [ ] **Step 4: Run to confirm pass**

```
go test ./... -run TestBusGroup -race -v -timeout 30s
go vet ./...
```

Expected: PASS。

- [ ] **Step 5: Commit**

```
git add busgroup.go busgroup_test.go
git commit -m "feat: add BusGroup.Close with aggregated errors"
```

## Task 7: 平台 Option 管线（linuxConfig + applyPlatformOptions 钩子）

**Files:**
- Modify: `options.go`
- Modify: `bus.go`
- Create: `bus_platform_linux.go`
- Create: `bus_platform_other.go`

这一步只搭骨架；具体 setsockopt 逻辑在后续 Task 加。

- [ ] **Step 1: Modify `options.go`**

在 `config` 结构体定义里末尾追加一行：

```go
type config struct {
	bitrate       Bitrate
	receiveMode   ReceiveMode
	pollInterval  time.Duration
	rxBufferSize  int
	errBufferSize int
	logger        Logger
	linux         linuxConfig // 仅 Linux 构建有真实字段；其他平台为空 struct
}
```

- [ ] **Step 2: Create `bus_platform_linux.go`**

```go
//go:build linux

package gocan

// linuxConfig 聚合 Linux 专属 Option 的可配置项。
// 所有字段使用 pointer / 0 哨兵：未传 Option 时不调对应 setsockopt。
type linuxConfig struct {
	// 字段在后续 Task 中陆续添加（WithLoopback 等）。
}

// applyPlatformOptions 在 Initialize 成功后、startReader 之前调用，
// 把 cfg.linux 的字段写到底层 socket 上。任意一项失败 → Uninitialize 回滚 → 返回错误。
func applyPlatformOptions(b *Bus, cfg *config) error {
	// 后续 Task 在此追加调用。
	return nil
}
```

- [ ] **Step 3: Create `bus_platform_other.go`**

```go
//go:build !linux

package gocan

// linuxConfig 在非 Linux 平台是空 struct，零成本。
type linuxConfig struct{}

// applyPlatformOptions 在非 Linux 平台是 no-op。
func applyPlatformOptions(b *Bus, cfg *config) error { return nil }
```

- [ ] **Step 4: Modify `bus.go` to call hook**

在 `openWith` 内，紧接 `Initialize` 成功的判断之后、`b := &Bus{...}` 之前不能调（因为 b 还没建）。正确位置：在 `b.startReader()` 之前。把：

```go
	switch cfg.receiveMode {
	case ModePolling:
```

修改前的 `b := &Bus{...}` 块仍然保留。在它之后、`switch cfg.receiveMode` 之前插入：

```go
	if err := applyPlatformOptions(b, cfg); err != nil {
		_ = adapt.Uninitialize(ch)
		return nil, err
	}
```

- [ ] **Step 5: Verify build on both platforms**

```
go vet ./...
GOOS=linux   go build ./...
GOOS=windows go build ./...
GOOS=darwin  go build ./...
go test ./... -run TestBusGroup -race -v -timeout 30s
```

Expected: 全部通过。

- [ ] **Step 6: Commit**

```
git add options.go bus.go bus_platform_linux.go bus_platform_other.go
git commit -m "feat: add platform option hook (no-op for now)"
```

## Task 8: raw setsockopt 薄封装

**Files:**
- Create: `raw/socketcan_options_linux.go`
- Create: `raw/socketcan_options_linux_test.go`

提供一组 Linux 专用的薄封装函数，对外提供按 channel handle 找到底层 socket fd 然后 setsockopt 的能力。封装住 `getLinuxChannel` 的查找 + 错误码映射。

- [ ] **Step 1: Create `raw/socketcan_options_linux.go`**

```go
//go:build linux

package raw

import (
	"time"

	"golang.org/x/sys/unix"
)

// SetCANRawSockoptInt 把一个 int 值写到 SOL_CAN_RAW 层的指定选项。
// 用于 CAN_RAW_LOOPBACK / CAN_RAW_RECV_OWN_MSGS / CAN_RAW_FD_FRAMES 等 0/1 开关。
func SetCANRawSockoptInt(ch TPCANHandle, opt int, value int) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if err := unix.SetsockoptInt(c.fd, unix.SOL_CAN_RAW, opt, value); err != nil {
		return errnoToStatus(err)
	}
	return PCAN_ERROR_OK
}

// SetCANRawErrFilter 写 CAN_RAW_ERR_FILTER。
func SetCANRawErrFilter(ch TPCANHandle, mask uint32) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if err := unix.SetsockoptInt(c.fd, unix.SOL_CAN_RAW, unix.CAN_RAW_ERR_FILTER, int(mask)); err != nil {
		return errnoToStatus(err)
	}
	c.errFilter = mask
	c.errFilterSet = true
	return PCAN_ERROR_OK
}

// SetCANRawJoinFilters 写 CAN_RAW_JOIN_FILTERS（true=AND，false=OR）。
// 内核 < 4.1 不支持，setsockopt 返回 ENOPROTOOPT；调用方应识别该状态。
func SetCANRawJoinFilters(ch TPCANHandle, and bool) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	v := 0
	if and {
		v = 1
	}
	if err := unix.SetsockoptInt(c.fd, unix.SOL_CAN_RAW, unix.CAN_RAW_JOIN_FILTERS, v); err != nil {
		return errnoToStatus(err)
	}
	c.joinFilters = and
	c.joinFiltersSet = true
	return PCAN_ERROR_OK
}

// SetSocketBuffers 写 SO_RCVBUF / SO_SNDBUF（任一非正值则跳过该方向）。
func SetSocketBuffers(ch TPCANHandle, rcv, snd int) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if rcv > 0 {
		if err := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_RCVBUF, rcv); err != nil {
			return errnoToStatus(err)
		}
	}
	if snd > 0 {
		if err := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_SNDBUF, snd); err != nil {
			return errnoToStatus(err)
		}
	}
	return PCAN_ERROR_OK
}

// SetReadWriteTimeout 写 SO_RCVTIMEO / SO_SNDTIMEO。零值表示不设置。
func SetReadWriteTimeout(ch TPCANHandle, read, write time.Duration) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if read > 0 {
		tv := unix.NsecToTimeval(read.Nanoseconds())
		if err := unix.SetsockoptTimeval(c.fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
			return errnoToStatus(err)
		}
	}
	if write > 0 {
		tv := unix.NsecToTimeval(write.Nanoseconds())
		if err := unix.SetsockoptTimeval(c.fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO, &tv); err != nil {
			return errnoToStatus(err)
		}
	}
	return PCAN_ERROR_OK
}

// EnableRxTimestamp 启用内核时间戳（mode 1=SO_TIMESTAMP, 2=SO_TIMESTAMPNS, 3=SO_TIMESTAMPING(HW)）。
// linuxChannel 持久化 mode，read 路径据此决定走 read 还是 recvmsg。
// HW 不被支持时降级到 NS（mode=2）。
func EnableRxTimestamp(ch TPCANHandle, mode uint8) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	switch mode {
	case 1:
		if err := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_TIMESTAMP, 1); err != nil {
			return errnoToStatus(err)
		}
	case 2:
		if err := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_TIMESTAMPNS, 1); err != nil {
			return errnoToStatus(err)
		}
	case 3:
		flags := unix.SOF_TIMESTAMPING_RX_HARDWARE | unix.SOF_TIMESTAMPING_RAW_HARDWARE | unix.SOF_TIMESTAMPING_SOFTWARE | unix.SOF_TIMESTAMPING_RX_SOFTWARE
		if err := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_TIMESTAMPING, flags); err != nil {
			// 硬件时间戳失败 → 降级到 NS
			if e := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_TIMESTAMPNS, 1); e != nil {
				return errnoToStatus(e)
			}
			c.rxTimestampMode = 2
			return PCAN_ERROR_OK
		}
	default:
		return PCAN_ERROR_ILLPARAMVAL
	}
	c.rxTimestampMode = mode
	return PCAN_ERROR_OK
}
```

注意：`linuxChannel` 字段（errFilter / errFilterSet / joinFilters / joinFiltersSet / rxTimestampMode）将在 Step 2 加。

- [ ] **Step 2: Modify `raw/socketcan_linux.go`**

把 `linuxChannel` 结构体扩成：

```go
type linuxChannel struct {
	fd              int
	isFD            bool
	filters         []unix.CanFilter
	errFilter       uint32
	errFilterSet    bool
	joinFilters     bool
	joinFiltersSet  bool
	rxTimestampMode uint8 // 0=off, 1=SO_TIMESTAMP, 2=SO_TIMESTAMPNS, 3=SO_TIMESTAMPING
}
```

- [ ] **Step 3: Write unit test `raw/socketcan_options_linux_test.go`**

```go
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

var _ = time.Second // 保持 import 不报错
```

集成层面（带 vcan 的真测试）放在 Task 15 的整合用例里。

- [ ] **Step 4: Run tests**

```
go vet ./...
go test ./raw -run TestSet -race -v
```

Expected: PASS。

- [ ] **Step 5: Commit**

```
git add raw/socketcan_options_linux.go raw/socketcan_options_linux_test.go raw/socketcan_linux.go
git commit -m "feat(raw): add SocketCAN setsockopt thin wrappers"
```

## Task 9: WithLoopback + WithRecvOwnMsgs

**Files:**
- Modify: `bus_platform_linux.go`
- Create: `options_linux.go`
- Create: `options_linux_test.go`

- [ ] **Step 1: Write failing test `options_linux_test.go`**

```go
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
```

- [ ] **Step 2: Run to confirm failure**

```
go test ./... -run 'TestWithLoopback|TestWithRecvOwnMsgs' -v
```

Expected: FAIL（`undefined`）。

- [ ] **Step 3: Update `bus_platform_linux.go`**

把 `linuxConfig` 替换为：

```go
type linuxConfig struct {
	loopback    *bool
	recvOwnMsgs *bool
}
```

把 `applyPlatformOptions` 替换为：

```go
import (
	"golang.org/x/sys/unix"

	"github.com/Crush251/gocan/raw"
)

func applyPlatformOptions(b *Bus, cfg *config) error {
	lc := &cfg.linux
	if lc.loopback != nil {
		v := 0
		if *lc.loopback {
			v = 1
		}
		if s := raw.SetCANRawSockoptInt(b.ch, unix.CAN_RAW_LOOPBACK, v); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(CAN_RAW_LOOPBACK)", s)
		}
	}
	if lc.recvOwnMsgs != nil {
		v := 0
		if *lc.recvOwnMsgs {
			v = 1
		}
		if s := raw.SetCANRawSockoptInt(b.ch, unix.CAN_RAW_RECV_OWN_MSGS, v); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(CAN_RAW_RECV_OWN_MSGS)", s)
		}
	}
	return nil
}
```

- [ ] **Step 4: Create `options_linux.go`**

```go
//go:build linux

package gocan

// WithLoopback 设置 CAN_RAW_LOOPBACK（默认内核为 true，即本地回环开启）。
// 关闭后，本进程发出的帧不会被同主机其他 socket 看到。
func WithLoopback(enabled bool) Option {
	return func(c *config) {
		v := enabled
		c.linux.loopback = &v
	}
}

// WithRecvOwnMsgs 设置 CAN_RAW_RECV_OWN_MSGS。开启后会收到本 socket 自己发出的帧
// （需配合 WithLoopback(true)；通常用于自发自收的回归测试）。
func WithRecvOwnMsgs(enabled bool) Option {
	return func(c *config) {
		v := enabled
		c.linux.recvOwnMsgs = &v
	}
}
```

- [ ] **Step 5: Run tests + cross-compile**

```
go test ./... -run 'TestWithLoopback|TestWithRecvOwnMsgs|TestBusGroup' -race -v
GOOS=windows go build ./...
GOOS=darwin  go build ./...
go vet ./...
```

Expected: PASS。

- [ ] **Step 6: Commit**

```
git add bus_platform_linux.go options_linux.go options_linux_test.go
git commit -m "feat: add WithLoopback and WithRecvOwnMsgs options"
```

## Task 10: WithErrFilter + raw/can_err_linux.go

**Files:**
- Create: `raw/can_err_linux.go`
- Modify: `bus_platform_linux.go`
- Modify: `options_linux.go`
- Modify: `options_linux_test.go`

- [ ] **Step 1: Create `raw/can_err_linux.go`**

```go
//go:build linux

package raw

import "golang.org/x/sys/unix"

// CAN 错误帧位掩码：用于 CAN_RAW_ERR_FILTER。完整定义参见 linux/can/error.h。
const (
	CANErrTxTimeout    uint32 = unix.CAN_ERR_TX_TIMEOUT
	CANErrLostArb      uint32 = unix.CAN_ERR_LOSTARB
	CANErrCrtl         uint32 = unix.CAN_ERR_CRTL
	CANErrProt         uint32 = unix.CAN_ERR_PROT
	CANErrTrx          uint32 = unix.CAN_ERR_TRX
	CANErrAck          uint32 = unix.CAN_ERR_ACK
	CANErrBusOff       uint32 = unix.CAN_ERR_BUSOFF
	CANErrBusError     uint32 = unix.CAN_ERR_BUSERROR
	CANErrRestarted    uint32 = unix.CAN_ERR_RESTARTED
	CANErrMaskAll      uint32 = unix.CAN_ERR_MASK
)
```

- [ ] **Step 2: Re-export at top-level package**

为减少业务侧 import `raw` 包的负担，在 `options_linux.go` 顶部追加：

```go
import "github.com/Crush251/gocan/raw"

// CAN 错误帧位掩码（与 raw 包对应常量等价）。
const (
	CANErrTxTimeout = raw.CANErrTxTimeout
	CANErrLostArb   = raw.CANErrLostArb
	CANErrCrtl      = raw.CANErrCrtl
	CANErrProt      = raw.CANErrProt
	CANErrTrx       = raw.CANErrTrx
	CANErrAck       = raw.CANErrAck
	CANErrBusOff    = raw.CANErrBusOff
	CANErrBusError  = raw.CANErrBusError
	CANErrRestarted = raw.CANErrRestarted
	CANErrMaskAll   = raw.CANErrMaskAll
)
```

- [ ] **Step 3: Write failing test `options_linux_test.go` 末尾追加**

```go
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
```

- [ ] **Step 4: Run to confirm failure**

```
go test ./... -run TestWithErrFilter -v
```

Expected: FAIL。

- [ ] **Step 5: Implement WithErrFilter in `options_linux.go`**

```go
// WithErrFilter 启用 CAN_RAW_ERR_FILTER，只接收 mask 中位标记的错误帧类型。
// 参见 raw/can_err_linux.go 中的 CANErr* 常量。
func WithErrFilter(mask uint32) Option {
	return func(c *config) {
		v := mask
		c.linux.errFilter = &v
	}
}
```

在 `bus_platform_linux.go` 的 `linuxConfig` 加字段：

```go
type linuxConfig struct {
	loopback    *bool
	recvOwnMsgs *bool
	errFilter   *uint32
}
```

在 `applyPlatformOptions` 末尾、`return nil` 之前追加：

```go
	if lc.errFilter != nil {
		if s := raw.SetCANRawErrFilter(b.ch, *lc.errFilter); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(CAN_RAW_ERR_FILTER)", s)
		}
	}
```

- [ ] **Step 6: Run tests + cross-compile**

```
go test ./... -run 'TestWith|TestBusGroup' -race -v
GOOS=windows go build ./...
go vet ./...
```

Expected: PASS。

- [ ] **Step 7: Commit**

```
git add raw/can_err_linux.go bus_platform_linux.go options_linux.go options_linux_test.go
git commit -m "feat: add WithErrFilter and CAN_ERR_* constants"
```

## Task 11: WithJoinFilters

**Files:**
- Modify: `bus_platform_linux.go`
- Modify: `options_linux.go`
- Modify: `options_linux_test.go`

- [ ] **Step 1: Write failing test**

`options_linux_test.go` 末尾追加：

```go
func TestWithJoinFilters_SetsField(t *testing.T) {
	cfg := newDefaultConfig()
	WithJoinFilters(true)(cfg)
	if cfg.linux.joinFilters == nil || *cfg.linux.joinFilters != true {
		t.Errorf("joinFilters = %v, want true", cfg.linux.joinFilters)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```
go test ./... -run TestWithJoinFilters -v
```

Expected: FAIL。

- [ ] **Step 3: Implement in `options_linux.go`**

```go
// WithJoinFilters 设置 CAN_RAW_JOIN_FILTERS 语义：
// true 表示多个 SetFilter 范围必须**全部**匹配（AND）；false / 默认是任一匹配（OR）。
// 内核 < 4.1 不支持，setsockopt 会返回 ENOPROTOOPT；调用 Open 时会返回包含
// PCAN_ERROR_ILLPARAMVAL 的 *Error，提示需要 ≥ 4.1。
func WithJoinFilters(and bool) Option {
	return func(c *config) {
		v := and
		c.linux.joinFilters = &v
	}
}
```

- [ ] **Step 4: Modify `bus_platform_linux.go`**

`linuxConfig` 加字段：

```go
type linuxConfig struct {
	loopback    *bool
	recvOwnMsgs *bool
	errFilter   *uint32
	joinFilters *bool
}
```

`applyPlatformOptions` 末尾追加：

```go
	if lc.joinFilters != nil {
		if s := raw.SetCANRawJoinFilters(b.ch, *lc.joinFilters); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(CAN_RAW_JOIN_FILTERS)", s)
		}
	}
```

- [ ] **Step 5: Run tests + cross-compile**

```
go test ./... -run 'TestWith|TestBusGroup' -race -v
GOOS=windows go build ./...
go vet ./...
```

Expected: PASS。

- [ ] **Step 6: Commit**

```
git add bus_platform_linux.go options_linux.go options_linux_test.go
git commit -m "feat: add WithJoinFilters option"
```

## Task 12: WithRecvTimestamp + recvmsg 路径

**Files:**
- Modify: `raw/socketcan_linux.go`（让 Read/ReadFD 在启用时间戳时走 recvmsg）
- Modify: `bus_platform_linux.go`
- Modify: `options_linux.go`
- Modify: `options_linux_test.go`

设计：`linuxChannel.rxTimestampMode` ≠ 0 时，`Read`/`ReadFD` 走 `unix.Recvmsg`，从 cmsg 取出 `timeval` / `timespec`，转 μs 写到 `TPCANTimestamp` 的 Millis/Micros 字段。

- [ ] **Step 1: Write failing test**

`options_linux_test.go` 末尾追加：

```go
func TestWithRecvTimestamp_SetsField(t *testing.T) {
	cfg := newDefaultConfig()
	WithRecvTimestamp(RxTimestampNano)(cfg)
	if cfg.linux.rxTimestamp != RxTimestampNano {
		t.Errorf("rxTimestamp = %v, want %v", cfg.linux.rxTimestamp, RxTimestampNano)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```
go test ./... -run TestWithRecvTimestamp -v
```

Expected: FAIL。

- [ ] **Step 3: Add `RxTimestamp` type in `options_linux.go`**

```go
// RxTimestamp 选择内核给入帧打时间戳的机制。
// 默认 RxTimestampNone 不启用；启用时 Frame.TimestampMicros 由内核提供。
type RxTimestamp uint8

const (
	RxTimestampNone     RxTimestamp = 0
	RxTimestampSecond   RxTimestamp = 1 // SO_TIMESTAMP（μs 精度）
	RxTimestampNano     RxTimestamp = 2 // SO_TIMESTAMPNS（ns 精度）
	RxTimestampHardware RxTimestamp = 3 // SO_TIMESTAMPING + RX_HARDWARE，不支持时降级到 NS
)

// WithRecvTimestamp 启用内核接收时间戳，结果写入 Frame.TimestampMicros。
// 不传该 Option 时保持现有行为：SocketCAN 后端用 time.Now() 合成时间戳。
func WithRecvTimestamp(mode RxTimestamp) Option {
	return func(c *config) {
		c.linux.rxTimestamp = mode
	}
}
```

- [ ] **Step 4: Modify `bus_platform_linux.go`**

`linuxConfig` 加字段：

```go
type linuxConfig struct {
	loopback    *bool
	recvOwnMsgs *bool
	errFilter   *uint32
	joinFilters *bool
	rxTimestamp RxTimestamp
}
```

`applyPlatformOptions` 末尾追加：

```go
	if lc.rxTimestamp != RxTimestampNone {
		if s := raw.EnableRxTimestamp(b.ch, uint8(lc.rxTimestamp)); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(SO_TIMESTAMP*)", s)
		}
	}
```

- [ ] **Step 5: Modify `raw/socketcan_linux.go` Read paths**

把 `Read` 函数改为：

```go
func Read(ch TPCANHandle, m *TPCANMsg, t *TPCANTimestamp) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if c.rxTimestampMode != 0 {
		return readClassicalWithTimestamp(c, m, t)
	}
	var buf [linuxCANFDFrameSize]byte
	n, err := unix.Read(c.fd, buf[:])
	if err != nil {
		return errnoToReadStatus(err)
	}
	if n != linuxCANFrameSize {
		return PCAN_ERROR_ILLDATA
	}
	if status := decodeLinuxCANFrame(buf[:linuxCANFrameSize], m); status != PCAN_ERROR_OK {
		return status
	}
	if t != nil {
		fillTimestamp(t)
	}
	return PCAN_ERROR_OK
}
```

`ReadFD` 同理：在入口判断 `c.rxTimestampMode != 0` 时走 `readFDWithTimestamp`。

新增辅助函数（追加到 socketcan_linux.go 末尾）：

```go
// readClassicalWithTimestamp 用 recvmsg 取一帧 Classical CAN 报文 + 内核时间戳。
func readClassicalWithTimestamp(c *linuxChannel, m *TPCANMsg, t *TPCANTimestamp) TPCANStatus {
	var buf [linuxCANFrameSize]byte
	var oob [128]byte
	n, oobn, _, _, err := unix.Recvmsg(c.fd, buf[:], oob[:], 0)
	if err != nil {
		return errnoToReadStatus(err)
	}
	if n != linuxCANFrameSize {
		return PCAN_ERROR_ILLDATA
	}
	if status := decodeLinuxCANFrame(buf[:linuxCANFrameSize], m); status != PCAN_ERROR_OK {
		return status
	}
	if t != nil {
		writeKernelTimestamp(oob[:oobn], t)
	}
	return PCAN_ERROR_OK
}

// readFDWithTimestamp 同上，FD 帧。
func readFDWithTimestamp(c *linuxChannel, m *TPCANMsgFD, t *TPCANTimestampFD) TPCANStatus {
	var buf [linuxCANFDFrameSize]byte
	var oob [128]byte
	n, oobn, _, _, err := unix.Recvmsg(c.fd, buf[:], oob[:], 0)
	if err != nil {
		return errnoToReadStatus(err)
	}
	switch n {
	case linuxCANFrameSize:
		var cm TPCANMsg
		if status := decodeLinuxCANFrame(buf[:linuxCANFrameSize], &cm); status != PCAN_ERROR_OK {
			return status
		}
		m.ID = cm.ID
		m.MsgType = cm.MsgType
		m.DLC = cm.Len
		copy(m.Data[:], cm.Data[:])
	case linuxCANFDFrameSize:
		if status := decodeLinuxCANFDFrame(buf[:], m); status != PCAN_ERROR_OK {
			return status
		}
	default:
		return PCAN_ERROR_ILLDATA
	}
	if t != nil {
		var ts TPCANTimestamp
		writeKernelTimestamp(oob[:oobn], &ts)
		// FD 时间戳直接是 μs 值
		*t = uint64(ts.MillisOverflow)*1000*(1<<32) + uint64(ts.Millis)*1000 + uint64(ts.Micros)
	}
	return PCAN_ERROR_OK
}

// writeKernelTimestamp 把控制消息里的 timeval/timespec 解析成微秒，写入 TPCANTimestamp。
// 不识别的 cmsg 退回到 fillTimestamp。
func writeKernelTimestamp(oob []byte, t *TPCANTimestamp) {
	cmsgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		fillTimestamp(t)
		return
	}
	for _, cm := range cmsgs {
		if cm.Header.Level != unix.SOL_SOCKET {
			continue
		}
		switch cm.Header.Type {
		case unix.SO_TIMESTAMP:
			if len(cm.Data) >= int(unsafe.Sizeof(unix.Timeval{})) {
				tv := *(*unix.Timeval)(unsafe.Pointer(&cm.Data[0]))
				totalUs := uint64(tv.Sec)*1_000_000 + uint64(tv.Usec)
				timestampFromMicros(totalUs, t)
				return
			}
		case unix.SO_TIMESTAMPNS:
			if len(cm.Data) >= int(unsafe.Sizeof(unix.Timespec{})) {
				ts := *(*unix.Timespec)(unsafe.Pointer(&cm.Data[0]))
				totalUs := uint64(ts.Sec)*1_000_000 + uint64(ts.Nsec)/1000
				timestampFromMicros(totalUs, t)
				return
			}
		case unix.SO_TIMESTAMPING:
			// 三段 timespec：software / hw_software_legacy / hw
			if len(cm.Data) >= 3*int(unsafe.Sizeof(unix.Timespec{})) {
				p := unsafe.Pointer(&cm.Data[0])
				sw := *(*unix.Timespec)(p)
				hw := *(*unix.Timespec)(unsafe.Add(p, 2*int(unsafe.Sizeof(unix.Timespec{}))))
				chosen := hw
				if hw.Sec == 0 && hw.Nsec == 0 {
					chosen = sw
				}
				totalUs := uint64(chosen.Sec)*1_000_000 + uint64(chosen.Nsec)/1000
				timestampFromMicros(totalUs, t)
				return
			}
		}
	}
	fillTimestamp(t)
}

// timestampFromMicros 拆 μs 总数到 TPCANTimestamp 的 Millis/Overflow/Micros 三字段。
func timestampFromMicros(totalUs uint64, t *TPCANTimestamp) {
	totalMillis := totalUs / 1000
	t.Millis = uint32(totalMillis)
	t.MillisOverflow = uint16(totalMillis >> 32)
	t.Micros = uint16(totalUs % 1000)
}
```

文件顶部 import 加 `"unsafe"` 和确认 `"golang.org/x/sys/unix"` 已存在。

- [ ] **Step 6: Run tests + cross-compile**

```
go test ./... -run 'TestWith|TestBusGroup' -race -v
GOOS=windows go build ./...
go vet ./...
```

Expected: PASS。

- [ ] **Step 7: Commit**

```
git add raw/socketcan_linux.go bus_platform_linux.go options_linux.go options_linux_test.go
git commit -m "feat: add WithRecvTimestamp with recvmsg-based kernel timestamps"
```

## Task 13: WithSocketBuffers + WithRWTimeout

**Files:**
- Modify: `bus_platform_linux.go`
- Modify: `options_linux.go`
- Modify: `options_linux_test.go`

- [ ] **Step 1: Write failing tests**

`options_linux_test.go` 末尾追加：

```go
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
```

文件 import 加 `"time"`。

- [ ] **Step 2: Run to confirm failure**

```
go test ./... -run 'TestWithSocketBuffers|TestWithRWTimeout' -v
```

Expected: FAIL。

- [ ] **Step 3: Implement options in `options_linux.go`**

```go
import "time"

// WithSocketBuffers 设置 SO_RCVBUF / SO_SNDBUF。任一非正值则跳过对应方向。
// 实际生效值受内核 net.core.rmem_max / wmem_max 上限限制。
func WithSocketBuffers(rcvBytes, sndBytes int) Option {
	return func(c *config) {
		if rcvBytes > 0 {
			c.linux.soRcvBuf = rcvBytes
		}
		if sndBytes > 0 {
			c.linux.soSndBuf = sndBytes
		}
	}
}

// WithRWTimeout 设置 SO_RCVTIMEO / SO_SNDTIMEO。零值表示该方向不设超时。
// 注意：当前 reader goroutine 用 polling 循环 + 短读，超时通常无显著影响；
// 主要用于 SocketCAN 在某些场景下避免 read() 永远阻塞。
func WithRWTimeout(read, write time.Duration) Option {
	return func(c *config) {
		if read > 0 {
			c.linux.readTimeout = read
		}
		if write > 0 {
			c.linux.writeTimeout = write
		}
	}
}
```

- [ ] **Step 4: Modify `bus_platform_linux.go`**

`linuxConfig` 加字段：

```go
type linuxConfig struct {
	loopback     *bool
	recvOwnMsgs  *bool
	errFilter    *uint32
	joinFilters  *bool
	rxTimestamp  RxTimestamp
	soRcvBuf     int
	soSndBuf     int
	readTimeout  time.Duration
	writeTimeout time.Duration
}
```

文件 import 加 `"time"`。

`applyPlatformOptions` 末尾追加：

```go
	if lc.soRcvBuf > 0 || lc.soSndBuf > 0 {
		if s := raw.SetSocketBuffers(b.ch, lc.soRcvBuf, lc.soSndBuf); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(SO_RCVBUF/SO_SNDBUF)", s)
		}
	}
	if lc.readTimeout > 0 || lc.writeTimeout > 0 {
		if s := raw.SetReadWriteTimeout(b.ch, lc.readTimeout, lc.writeTimeout); s != raw.PCAN_ERROR_OK {
			return wrapStatus(b.adapt, "setsockopt(SO_RCVTIMEO/SO_SNDTIMEO)", s)
		}
	}
```

- [ ] **Step 5: Run tests + cross-compile**

```
go test ./... -run 'TestWith|TestBusGroup' -race -v
GOOS=windows go build ./...
go vet ./...
```

Expected: PASS。

- [ ] **Step 6: Commit**

```
git add bus_platform_linux.go options_linux.go options_linux_test.go
git commit -m "feat: add WithSocketBuffers and WithRWTimeout options"
```

## Task 14: SetErrFilter / SetJoinFilters 运行期方法

**Files:**
- Create: `bus_socketcan_linux.go`
- Create: `bus_socketcan_other.go`

- [ ] **Step 1: Create `bus_socketcan_linux.go`**

```go
//go:build linux

package gocan

import "github.com/Crush251/gocan/raw"

// SetErrFilter 运行期更新 CAN_RAW_ERR_FILTER 掩码。
// setsockopt 失败时返回错误，linuxChannel 里持久化的 mask 保持原值（与内核状态一致）。
func (b *Bus) SetErrFilter(mask uint32) error {
	if b.closed.Load() {
		return ErrBusClosed
	}
	if s := raw.SetCANRawErrFilter(b.ch, mask); s != raw.PCAN_ERROR_OK {
		return wrapStatus(b.adapt, "setsockopt(CAN_RAW_ERR_FILTER)", s)
	}
	return nil
}

// SetJoinFilters 运行期更新 CAN_RAW_JOIN_FILTERS（true=AND, false=OR）。
// setsockopt 失败时返回错误，linuxChannel 里持久化的标志保持原值。
func (b *Bus) SetJoinFilters(and bool) error {
	if b.closed.Load() {
		return ErrBusClosed
	}
	if s := raw.SetCANRawJoinFilters(b.ch, and); s != raw.PCAN_ERROR_OK {
		return wrapStatus(b.adapt, "setsockopt(CAN_RAW_JOIN_FILTERS)", s)
	}
	return nil
}
```

- [ ] **Step 2: Create `bus_socketcan_other.go`**

```go
//go:build !linux

package gocan

// SetErrFilter 在非 Linux 平台返回 ErrNotSupported。
func (b *Bus) SetErrFilter(mask uint32) error { return ErrNotSupported }

// SetJoinFilters 在非 Linux 平台返回 ErrNotSupported。
func (b *Bus) SetJoinFilters(and bool) error { return ErrNotSupported }
```

- [ ] **Step 3: Add basic test**

`busgroup_test.go`（或新文件 `bus_socketcan_runtime_test.go`）末尾追加：

```go
func TestBusSetErrFilter_OnClosedBus(t *testing.T) {
	withFakeOpener(t)
	g := NewBusGroup(0)
	bus, err := g.Add("a", raw.PCAN_USBBUS1)
	if err != nil {
		t.Fatal(err)
	}
	_ = g.Close()
	if err := bus.SetErrFilter(0); err == nil {
		t.Error("expected non-nil error after Close")
	}
}
```

- [ ] **Step 4: Verify cross-platform build**

```
GOOS=linux   go build ./...
GOOS=windows go build ./...
GOOS=darwin  go build ./...
go test ./... -race -v -timeout 30s
go vet ./...
```

Expected: PASS。

- [ ] **Step 5: Commit**

```
git add bus_socketcan_linux.go bus_socketcan_other.go busgroup_test.go
git commit -m "feat: add Bus.SetErrFilter / SetJoinFilters runtime methods"
```

## Task 15: vcan 集成测试

**Files:**
- Create: `socketcan_options_integration_linux_test.go`

- [ ] **Step 1: Create test file**

```go
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
```

- [ ] **Step 2: Run integration tests**

```
sudo ./scripts/setup-vcan.sh up vcan0 vcan1   # 见 Task 16
go test ./... -run TestSocketCANIntegration -race -v -timeout 60s
```

无 vcan 时全部 SKIP；有 vcan 时全部 PASS。

- [ ] **Step 3: Commit**

```
git add socketcan_options_integration_linux_test.go
git commit -m "test: add SocketCAN options vcan integration tests"
```

## Task 16: setup-vcan.sh + justfile 别名

**Files:**
- Create: `scripts/setup-vcan.sh`
- Modify: `justfile`

- [ ] **Step 1: Create directory and script**

```
mkdir -p scripts
```

`scripts/setup-vcan.sh`：

```bash
#!/usr/bin/env bash
# scripts/setup-vcan.sh — 创建用于本地开发 / 集成测试的虚拟 CAN 接口。
# 用法:
#   sudo ./scripts/setup-vcan.sh [up|down] [iface...]
# 示例:
#   sudo ./scripts/setup-vcan.sh up vcan0 vcan1
#   sudo ./scripts/setup-vcan.sh down vcan0 vcan1
#
# 默认: up vcan0 vcan1
# 需要 root（modprobe + ip link）。
set -euo pipefail

ACTION=${1:-up}
shift || true
IFACES=("$@")
if [ ${#IFACES[@]} -eq 0 ]; then
    IFACES=(vcan0 vcan1)
fi

require_root() {
    if [ "$EUID" -ne 0 ]; then
        echo "需要 root 权限：请使用 sudo 运行" >&2
        exit 1
    fi
}

ensure_module() {
    if ! lsmod | grep -q '^vcan'; then
        echo "正在加载 vcan 内核模块..."
        modprobe vcan
    fi
}

up_iface() {
    local iface=$1
    if ip link show "$iface" >/dev/null 2>&1; then
        echo "[skip] $iface 已存在"
    else
        ip link add "$iface" type vcan
        echo "[add ] $iface"
    fi
    ip link set "$iface" up
    echo "[up  ] $iface"
}

down_iface() {
    local iface=$1
    if ip link show "$iface" >/dev/null 2>&1; then
        ip link set "$iface" down
        ip link delete "$iface"
        echo "[del ] $iface"
    else
        echo "[skip] $iface 不存在"
    fi
}

require_root
case "$ACTION" in
    up)
        ensure_module
        for iface in "${IFACES[@]}"; do up_iface "$iface"; done
        ;;
    down)
        for iface in "${IFACES[@]}"; do down_iface "$iface"; done
        ;;
    *)
        echo "未知动作: $ACTION (期望 up 或 down)" >&2
        exit 1
        ;;
esac
```

赋可执行权限：

```
chmod +x scripts/setup-vcan.sh
```

- [ ] **Step 2: Add justfile alias**

`justfile` 末尾追加：

```
# 创建本地开发 / 集成测试用的虚拟 CAN 接口
vcan-up:
    sudo scripts/setup-vcan.sh up vcan0 vcan1

# 删除虚拟 CAN 接口
vcan-down:
    sudo scripts/setup-vcan.sh down vcan0 vcan1
```

- [ ] **Step 3: Smoke test**

```
just vcan-up
ip link show vcan0
just vcan-down
```

Expected: vcan0/vcan1 创建后状态 `state UP`；down 后接口消失。

- [ ] **Step 4: Commit**

```
git add scripts/setup-vcan.sh justfile
git commit -m "build: add setup-vcan.sh helper and justfile aliases"
```

## Task 17: example 11 — busgroup_socketcan

**Files:**
- Create: `examples/11_busgroup_socketcan/main.go`

- [ ] **Step 1: Create file**

```go
// 运行: go run ./examples/11_busgroup_socketcan
// 前置: Linux + 已用 scripts/setup-vcan.sh up vcan0 vcan1 创建虚拟 CAN 接口
//
// 演示用 BusGroup 同时管理两个 SocketCAN 通道，合并接收循环 + 一行收尾。
// 替代手写 sync.WaitGroup + 双份 reader goroutine 的旧式写法。

package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"

	"github.com/Crush251/gocan"
)

func main() {
	g := gocan.NewBusGroup(0)
	defer func() {
		if err := g.Close(); err != nil {
			var gce *gocan.GroupCloseError
			if errors.As(err, &gce) {
				for name, e := range gce.Causes {
					log.Printf("close %s: %v", name, e)
				}
			} else {
				log.Printf("close: %v", err)
			}
		}
	}()

	if _, err := g.Add("front", gocan.SocketCAN("vcan0")); err != nil {
		log.Fatalf("add front: %v", err)
	}
	if _, err := g.Add("chassis", gocan.SocketCAN("vcan1")); err != nil {
		log.Fatalf("add chassis: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case sf := <-g.Receive():
			log.Printf("[%s] id=0x%X dlc=%d data=%X", sf.Source, sf.Frame.ID, len(sf.Frame.Data), sf.Frame.Data)
		}
	}
}
```

- [ ] **Step 2: Verify compiles**

```
GOOS=linux go build ./examples/11_busgroup_socketcan
go vet ./examples/11_busgroup_socketcan
```

Expected: 无错。

- [ ] **Step 3: Commit**

```
git add examples/11_busgroup_socketcan/main.go
git commit -m "docs(examples): add busgroup_socketcan example"
```

## Task 18: example 12 — busgroup_fan_in

**Files:**
- Create: `examples/12_busgroup_fan_in/main.go`

- [ ] **Step 1: Create file**

```go
// 运行: go run ./examples/12_busgroup_fan_in
// 前置: Linux + scripts/setup-vcan.sh up vcan0 vcan1 vcan2
//
// 演示用 SourcedFrame 的 Source 字段做业务路由：合并接收 + switch 分发。
// 退出前用 BusGroup.Each 给每个 Bus 打一次 Status 做诊断。

package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/Crush251/gocan"
)

func handleEngine(f gocan.Frame)    { log.Printf("engine: 0x%X %X", f.ID, f.Data) }
func handleBody(f gocan.Frame)      { log.Printf("body: 0x%X %X", f.ID, f.Data) }
func handleTelemetry(f gocan.Frame) { log.Printf("telemetry: 0x%X %X", f.ID, f.Data) }

func main() {
	g := gocan.NewBusGroup(0)
	defer func() {
		g.Each(func(name string, bus *gocan.Bus) {
			st, err := bus.Status()
			if err != nil {
				log.Printf("[%s] status err: %v", name, err)
				return
			}
			log.Printf("[%s] final status=0x%X", name, uint32(st))
		})
		_ = g.Close()
	}()

	for _, p := range []struct{ name, iface string }{
		{"engine", "vcan0"},
		{"body", "vcan1"},
		{"telemetry", "vcan2"},
	} {
		if _, err := g.Add(p.name, gocan.SocketCAN(p.iface)); err != nil {
			log.Fatalf("add %s: %v", p.name, err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case sf := <-g.Receive():
			switch sf.Source {
			case "engine":
				handleEngine(sf.Frame)
			case "body":
				handleBody(sf.Frame)
			case "telemetry":
				handleTelemetry(sf.Frame)
			default:
				log.Printf("unknown source %q", sf.Source)
			}
		}
	}
}
```

- [ ] **Step 2: Verify compiles**

```
GOOS=linux go build ./examples/12_busgroup_fan_in
go vet ./examples/12_busgroup_fan_in
```

Expected: 无错。

- [ ] **Step 3: Commit**

```
git add examples/12_busgroup_fan_in/main.go
git commit -m "docs(examples): add busgroup_fan_in example"
```

## Task 19: example 13 — socketcan_loopback

**Files:**
- Create: `examples/13_socketcan_loopback/main.go`

- [ ] **Step 1: Create file**

```go
// 运行: go run ./examples/13_socketcan_loopback
// 前置: Linux + scripts/setup-vcan.sh up vcan0
//
// 演示 WithLoopback(true) + WithRecvOwnMsgs(true) 的组合：
// 单 vcan 接口上自发自收，验证收发链路无需外部硬件即可做回归测试。

package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Crush251/gocan"
)

const totalFrames = 100

func main() {
	bus, err := gocan.Open(
		gocan.SocketCAN("vcan0"),
		gocan.WithLoopback(true),    // 默认就是 true，这里显式声明
		gocan.WithRecvOwnMsgs(true), // 关键：让本 socket 收到自己发的帧
		gocan.WithRxBufferSize(totalFrames*2),
	)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	got := make(map[uint32]int)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for i := 0; i < totalFrames; i++ {
			f, err := bus.ReadOne(ctx)
			if err != nil {
				log.Printf("read[%d]: %v", i, err)
				return
			}
			got[f.ID]++
		}
	}()

	for i := 0; i < totalFrames; i++ {
		f, _ := gocan.NewFrame(uint32(0x100+i), []byte{byte(i)})
		if err := bus.Send(context.Background(), f); err != nil {
			log.Fatalf("send[%d]: %v", i, err)
		}
	}
	wg.Wait()

	pass := true
	for i := 0; i < totalFrames; i++ {
		if got[uint32(0x100+i)] != 1 {
			log.Printf("missing 0x%X (count=%d)", 0x100+i, got[uint32(0x100+i)])
			pass = false
		}
	}
	if pass {
		log.Printf("PASS: %d frames sent and received", totalFrames)
	} else {
		log.Printf("FAIL: see missing frames above")
	}

	// 试试看：把 WithRecvOwnMsgs(true) 注释掉重跑，
	// 你会看到 ReadOne 在 5 秒后超时——本 socket 不会收到自己发的帧。
}
```

- [ ] **Step 2: Verify compiles**

```
GOOS=linux go build ./examples/13_socketcan_loopback
go vet ./examples/13_socketcan_loopback
```

Expected: 无错。

- [ ] **Step 3: Commit**

```
git add examples/13_socketcan_loopback/main.go
git commit -m "docs(examples): add socketcan_loopback self-send-receive example"
```

## Task 20: example 14 — socketcan_advanced

**Files:**
- Create: `examples/14_socketcan_advanced/main.go`

- [ ] **Step 1: Create file**

```go
// 运行: go run ./examples/14_socketcan_advanced
// 前置: Linux + scripts/setup-vcan.sh up vcan0
//
// 演示完整的 SocketCAN 调试 + 调优组合：
//   1. WithErrFilter         订阅总线错误帧用于诊断
//   2. WithJoinFilters       多 SetFilter 范围用 AND 语义
//   3. WithRecvTimestamp     内核纳秒时间戳直接写入 Frame.TimestampMicros
//   4. WithSocketBuffers     为高吞吐场景扩 socket 缓冲区
//   5. WithRWTimeout         读超时防止 reader 永远阻塞
//
// 末尾演示运行期通过 SetErrFilter(0) 关闭错误帧订阅。

package main

import (
	"context"
	"log"
	"time"

	"github.com/Crush251/gocan"
)

func main() {
	bus, err := gocan.Open(
		gocan.SocketCAN("vcan0"),
		gocan.WithLoopback(true),
		gocan.WithRecvOwnMsgs(true),

		// (1) 错误帧诊断：BUSOFF 等关键事件会被解码成 raw 错误帧投递。
		gocan.WithErrFilter(gocan.CANErrBusOff|gocan.CANErrTxTimeout|gocan.CANErrCrtl),

		// (2) AND 语义：后续 SetFilter 必须同时匹配才接收（默认 OR）。
		gocan.WithJoinFilters(true),

		// (3) 内核纳秒时间戳：精度高于 time.Now() 合成方案。
		gocan.WithRecvTimestamp(gocan.RxTimestampNano),

		// (4) 高吞吐：rcv 2 MiB / snd 1 MiB（受 net.core.rmem_max 限制）。
		gocan.WithSocketBuffers(2*1024*1024, 1*1024*1024),

		// (5) 读超时 500 ms；不限制写。
		gocan.WithRWTimeout(500*time.Millisecond, 0),
	)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer bus.Close()

	// 手动设两条不相交范围的滤波器；JOIN_FILTERS=true 下 AND 语义会让两条
	// 都必须命中——在不相交的两条上没有交集，等于"什么都不收"。
	if err := bus.SetFilter(0x100, 0x10F, gocan.FilterStandard); err != nil {
		log.Fatalf("SetFilter A: %v", err)
	}
	if err := bus.SetFilter(0x200, 0x20F, gocan.FilterStandard); err != nil {
		log.Fatalf("SetFilter B: %v", err)
	}

	// 发一帧 0x100：因为 AND 语义没法同时落在 0x200..0x20F，所以收不到。
	frame, _ := gocan.NewFrame(0x100, []byte{0x42})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := bus.Send(ctx, frame); err != nil {
		log.Printf("send: %v", err)
	}
	if _, err := bus.ReadOne(ctx); err != nil {
		log.Printf("(预期) 在 AND 滤波下读不到帧: %v", err)
	}

	// 演示运行期改变错误帧订阅。
	if err := bus.SetErrFilter(0); err != nil {
		log.Fatalf("SetErrFilter(0): %v", err)
	}
	log.Printf("错误帧订阅已关闭")
}
```

- [ ] **Step 2: Verify compiles**

```
GOOS=linux go build ./examples/14_socketcan_advanced
go vet ./examples/14_socketcan_advanced
```

Expected: 无错。

- [ ] **Step 3: Commit**

```
git add examples/14_socketcan_advanced/main.go
git commit -m "docs(examples): add socketcan_advanced full options example"
```

## Task 21: docs/socketcan-options.md

**Files:**
- Create: `docs/socketcan-options.md`

- [ ] **Step 1: Create skeleton + content**

```markdown
# SocketCAN 自定义参数

Linux SocketCAN 后端在 `Open` / `OpenFD` 时可通过一组 Linux 专属 Option 调整内核 socket 行为。本页是这些 Option 的完整速查 + 与 Linux 内核 setsockopt 的映射，配合 `examples/13_socketcan_loopback/`、`examples/14_socketcan_advanced/` 一起阅读。

> 仅 Linux 构建可见。在非 Linux 平台调用 `WithLoopback(...)` 等会编译期报错（防止 silently no-op）。

## 1. 速查

| Option | 内核 setsockopt | 内核版本要求 | 何时设置 |
|---|---|---|---|
| `WithLoopback(bool)` | `SOL_CAN_RAW` `CAN_RAW_LOOPBACK` | 3.6+ | Open 时 |
| `WithRecvOwnMsgs(bool)` | `SOL_CAN_RAW` `CAN_RAW_RECV_OWN_MSGS` | 3.6+ | Open 时 |
| `WithErrFilter(uint32)` | `SOL_CAN_RAW` `CAN_RAW_ERR_FILTER` | 3.6+ | Open 时；运行期可用 `SetErrFilter` |
| `WithJoinFilters(bool)` | `SOL_CAN_RAW` `CAN_RAW_JOIN_FILTERS` | **4.1+** | Open 时；运行期可用 `SetJoinFilters` |
| `WithRecvTimestamp(mode)` | `SOL_SOCKET` `SO_TIMESTAMP` / `SO_TIMESTAMPNS` / `SO_TIMESTAMPING` | 取决于 mode | Open 时（运行期改需要重打开） |
| `WithSocketBuffers(rcv, snd)` | `SOL_SOCKET` `SO_RCVBUF` / `SO_SNDBUF` | always | Open 时 |
| `WithRWTimeout(rTO, wTO)` | `SOL_SOCKET` `SO_RCVTIMEO` / `SO_SNDTIMEO` | always | Open 时 |

## 2. 错误处理总则

任意 setsockopt 失败 → `Uninitialize(ch)` 回滚底层资源 → 返回包含 `"setsockopt(NAME)"` 操作名的 `*Error`。`errors.Is(err, ErrIllParamValue)` 等可命中 PCAN 状态映射。

## 3. 详解

### 3.1 WithLoopback

控制 `CAN_RAW_LOOPBACK`：本机其他 socket 是否能看到本进程发出的帧。默认（不调）= 内核默认 `true`。**关闭时本机无法做自发自收回归。**

### 3.2 WithRecvOwnMsgs

控制 `CAN_RAW_RECV_OWN_MSGS`：本 socket 是否能收到自己发出的帧。需配合 `WithLoopback(true)`。配合 vcan 即可做单进程的回归测试，参见 `examples/13_socketcan_loopback`。

### 3.3 WithErrFilter

把内核错误帧（CAN_ERR_FRAME）转成业务可读的事件流。`mask` 是 `CANErrBusOff | CANErrTxTimeout | ...` 的 OR。常用位：

| 常量 | 触发场景 |
|---|---|
| `CANErrBusOff` | 控制器进入 BUSOFF |
| `CANErrTxTimeout` | 发送超时 |
| `CANErrLostArb` | 仲裁丢失 |
| `CANErrCrtl` | 控制器状态变化（错误被动等）|
| `CANErrProt` | 协议错误（CRC、stuff 等）|
| `CANErrBusError` | 总线错误（电气层）|
| `CANErrRestarted` | 自动重启 |
| `CANErrMaskAll` | 所有位 |

### 3.4 WithJoinFilters

控制 `CAN_RAW_JOIN_FILTERS`：多个 `SetFilter` 范围之间是 AND 还是 OR。默认 OR（任一命中即接收）；启用后改为 AND（全部命中才接收）。**需要内核 ≥ 4.1**——更老内核 setsockopt 返回 `ENOPROTOOPT`，会被映射为 PCAN 错误。

### 3.5 WithRecvTimestamp

启用内核接收时间戳，结果直接写入 `Frame.TimestampMicros`：

| `RxTimestamp` 值 | 含义 |
|---|---|
| `RxTimestampNone` | 不启用，沿用 `time.Now()` 合成时间戳（默认）|
| `RxTimestampSecond` | `SO_TIMESTAMP`：μs 级 |
| `RxTimestampNano` | `SO_TIMESTAMPNS`：ns 级 |
| `RxTimestampHardware` | `SO_TIMESTAMPING + RX_HARDWARE`：硬件时间戳，不被支持时降级到 `RxTimestampNano` |

启用后内部从 `read(2)` 切换到 `recvmsg(2) + cmsg`，每帧多一次小拷贝；不启用时性能与之前一致。

### 3.6 WithSocketBuffers

`SO_RCVBUF` / `SO_SNDBUF`，单位字节。**实际上限受 `net.core.rmem_max` / `wmem_max`** 限制；调大需要相应调系统参数（`sysctl -w net.core.rmem_max=8388608`）。

### 3.7 WithRWTimeout

`SO_RCVTIMEO` / `SO_SNDTIMEO`。零值（默认）= 不设置 = 阻塞读 / 写。注意 reader goroutine 当前用 polling 循环 + 短读，read timeout 主要影响异常路径下的退出延迟。

## 4. 运行期可调

仅 `SetErrFilter` 和 `SetJoinFilters` 暴露为 `*Bus` 方法。其他参数（loopback、buffer、超时、时间戳模式）变更会涉及 reader goroutine 协调，超出当前实现范围；需要时整体 `Close` 后重新 `Open`。

## 5. 比特率配置（不在库里）

SocketCAN 的比特率 / sample-point / restart-ms 由内核 netlink 配置，不属于应用进程职责。开发环境用项目自带的脚本：

```bash
sudo ./scripts/setup-vcan.sh up vcan0 vcan1
```

生产环境用 `iproute2`：

```bash
sudo ip link set can0 down
sudo ip link set can0 type can bitrate 500000 sample-point 0.875 restart-ms 100
sudo ip link set can0 up
```

## 6. 与 PCAN 后端的差异

| 概念 | Linux SocketCAN | Windows PCAN |
|---|---|---|
| 监听模式 | `WithRecvOwnMsgs(false)`（默认）| `PCAN_LISTEN_ONLY`（本轮未实现）|
| 自发自收 | `WithLoopback(true) + WithRecvOwnMsgs(true)` | 取决于硬件回环（不一致）|
| 错误帧 | `WithErrFilter` 订阅 | `PCAN_ALLOW_ERROR_FRAMES`（本轮未实现）|
| 比特率 | `ip link set can0 type can bitrate ...` | `WithBitrate(...)` Open 时设 |
| FD 比特率 | `ip link set can0 type can ... dbitrate ... fd on` | `OpenFD(ch, "f_clock=...,nom_brp=...")` |

后续单独 PR 补 Windows PCAN 专属 Option，使两侧体验拉齐。

## 参考

- [`man 7 socket`](https://man7.org/linux/man-pages/man7/socket.7.html)（`SO_TIMESTAMP*` / `SO_RCVBUF` 等）
- [`Documentation/networking/can.rst`](https://www.kernel.org/doc/html/latest/networking/can.html)
- 项目 spec：`docs/superpowers/specs/2026-06-08-busgroup-and-socketcan-options-design.md`
```

- [ ] **Step 2: Lint markdown lightly**

```
ls docs/socketcan-options.md
wc -l docs/socketcan-options.md
```

- [ ] **Step 3: Commit**

```
git add docs/socketcan-options.md
git commit -m "docs: add SocketCAN options reference"
```

---

## Self-Review

实施完所有 21 个 Task 后，做最终验证：

- [ ] **跨平台编译**

```
GOOS=linux   go build ./...
GOOS=windows go build ./...
GOOS=darwin  go build ./...
```

- [ ] **测试 + race**

```
go vet ./...
go test ./... -race -timeout 120s
```

- [ ] **vcan 集成测试**

```
just vcan-up
go test ./... -run TestSocketCANIntegration -race -v -timeout 60s
just vcan-down
```

- [ ] **示例编译**

```
for d in examples/11_busgroup_socketcan examples/12_busgroup_fan_in examples/13_socketcan_loopback examples/14_socketcan_advanced; do
    GOOS=linux go build ./$d || exit 1
done
```

- [ ] **API 稳定**：手工核对 `Open` / `OpenFD` / `*Bus` 公开方法签名与本 PR 之前一致。

- [ ] **不变量 §2**：再核对 spec 第 2 章的 7 条不变量逐条满足。





















</content>

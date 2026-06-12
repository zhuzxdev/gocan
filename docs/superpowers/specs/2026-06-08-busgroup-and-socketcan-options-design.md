# BusGroup 与 SocketCAN 自定义参数 — 设计文档

- 日期：2026-06-08
- 范围：本轮只动 Linux SocketCAN 后端；Windows PCAN 后端不动
- 依赖：基于现有 `2026-05-22-gocan-design.md` 的 v0.x 实现

## 1. 背景与动机

现有代码已经能在同进程打开多个 `*Bus`（参见 `examples/06_multi_channel/`），但**库本身没有提供"群组管理"封装**——多通道场景下 `Close`、`Receive` fan-in、按业务名字访问 Bus、错误聚合都要业务方自己写。

同时 Linux SocketCAN 后端目前只实现了最低限度：打开 socket、`CAN_RAW_FD_FRAMES`、`CAN_RAW_FILTER`。SocketCAN 在 Linux 上常用的 setsockopt 选项（loopback、RecvOwnMsgs、错误帧掩码、内核时间戳、socket buffer、读写超时、JOIN_FILTERS）**全部缺失**，导致用户没办法做：

- 自发自收的回归测试（loopback + RECV_OWN_MSGS）
- 高吞吐场景的 buffer 调优
- 内核精度时间戳采集
- 错误帧诊断订阅
- 多 filter 的 AND 组合

本轮通过两件事解决：
1. 新增 `BusGroup` 类型，按业务名字索引、合流接收、聚合关闭
2. 新增 7 个 Linux 专属 Option + 2 个运行期可调方法，覆盖上述常用 SocketCAN 调参点

## 2. 不变量（兼容性硬约束）

1. 所有现有 `Open` / `OpenFD` / `*Bus` 公开方法签名**不变**
2. 所有现有 `WithXxx` Option **不变**
3. 调用方不传任何新 Option 时，行为和今天**字节级一致**
4. `examples/06_multi_channel/` 不动，作为"不用 BusGroup 也能用"的对照保留
5. `raw` 子包公开 API **不变**，本轮只在 raw 内**新增** `socketcan_options_linux.go` 文件
6. 跨平台编译：`*Bus` 上的 SocketCAN 运行期方法在非 Linux 平台返回 `ErrNotSupported`（复用 `errors.go` 已有的哨兵错误），不报编译错；Linux Option 函数仅在 Linux 构建可见，其他平台调用 → 编译期错误（防止 silently no-op）
7. 不引入新依赖，仅用标准库和已有的 `golang.org/x/sys/unix`

## 3. 文件布局

新增的文件（不改任何现有目录结构）：

```
gocan/
├── busgroup.go                                    # BusGroup 类型与 API
├── busgroup_test.go                               # 单元（fakeAdapter）
├── options_linux.go                               # Linux 专属 Option
├── options_linux_test.go
├── bus_platform_linux.go                          # applyPlatformOptions Linux 实现
├── bus_platform_other.go                          # applyPlatformOptions 空实现
├── bus_socketcan_linux.go                         # *Bus 上的 SetErrFilter / SetJoinFilters
├── bus_socketcan_other.go                         # 其他平台返回 ErrUnsupported
├── socketcan_options_integration_linux_test.go    # 真 vcan 集成测试
├── docs/socketcan-options.md
├── scripts/setup-vcan.sh
└── examples/
    ├── 11_busgroup_socketcan/main.go
    ├── 12_busgroup_fan_in/main.go
    ├── 13_socketcan_loopback/main.go
    └── 14_socketcan_advanced/main.go

raw/
├── socketcan_options_linux.go                     # 薄封装 setsockopt(SOL_CAN_RAW, …)
├── socketcan_options_linux_test.go
└── can_err_linux.go                               # CAN_ERR_* 常量（用于 WithErrFilter）
```

修改的现有文件（仅加字段/方法，不破坏既有签名）：

- `raw/socketcan_linux.go` — `linuxChannel` 加几个字段持久化已应用的 sockopt（`SetErrFilter` / `SetJoinFilters` 这类运行期变更需要更新这里以便排错和单元断言）
- `bus.go` — `openWith` 在 `Initialize` 成功后、`startReader` 之前调用一行新增钩子 `applyPlatformOptions(b, cfg)`；该钩子由 `bus_platform_linux.go` / `bus_platform_other.go` 按平台提供（Linux 应用 setsockopt，其他平台 no-op）。除这一行调用外不修改任何既有逻辑
- `options.go` — `config` 结构体加一个 `linux linuxConfig` 字段；`linuxConfig` 在 Linux 构建是真实结构、其他平台是空 struct（用 build tag 隔离），零成本。不改任何既有 Option 函数

## 4. BusGroup API

```go
// SourcedFrame 是 BusGroup.Receive 返回的合流帧，
// 把帧和发出它的 Bus 名字打包在一起。
type SourcedFrame struct {
    Source string
    Frame  Frame
}

// BusGroup 管理一组按业务名字索引的 *Bus。
// 零值不可用，必须通过 NewBusGroup 构造。
// 所有方法并发安全。
type BusGroup struct { /* unexported */ }

// NewBusGroup 创建空 group。
// outBufferSize 是 Receive 合并 channel 的容量；非正值用默认 defaultGroupOutSize = 1024。
func NewBusGroup(outBufferSize int) *BusGroup

// Add 打开一个 Classical CAN 通道并以 name 加入 group。
//   - name 为空 → ErrInvalidName
//   - name 重复 → ErrDuplicateName
//   - 底层 Open 失败 → 返回相应 *Error，group 状态不变
//   - 成功后内部为该 Bus 启动一个 fan-in goroutine
func (g *BusGroup) Add(name string, ch Channel, opts ...Option) (*Bus, error)

// AddFD 等价于 Add，但调底层 OpenFD。
func (g *BusGroup) AddFD(name string, ch Channel, fdBitrate string, opts ...Option) (*Bus, error)

// Get 按名字取 Bus；不存在返回 nil, false。
func (g *BusGroup) Get(name string) (*Bus, bool)

// Names 返回当前 group 内所有 Bus 名字的拷贝（已排序）。
func (g *BusGroup) Names() []string

// Each 在持有读锁下按 Names 顺序遍历每个 Bus。
// fn 内禁止调 Add/AddFD/Close —— 会死锁，race 模式下会被检测出来。
func (g *BusGroup) Each(fn func(name string, bus *Bus))

// Receive 返回合并接收 channel；group 关闭时该 channel 也关闭。
// 反压策略：写入 out 阻塞 → 反压回对应 Bus 的 fan-in goroutine
// （不丢帧，让上游感知慢消费者；需要更高吞吐时调大 outBufferSize）。
func (g *BusGroup) Receive() <-chan SourcedFrame

// Close 并发关闭所有 Bus，聚合错误。幂等。
// 关闭顺序：closing → 等所有 fan-in 退出 → Close 每个 Bus → close(out)。
// 返回非 nil 时一定是 *GroupCloseError。
func (g *BusGroup) Close() error
```

### 4.1 关键决策

- **Add 内部调 Open**：不接受外部已构造好的 `*Bus`。让 group 同时拥有 Open 和 Close 全过程，避免半托管。需要手工 Open 时仍可用 `examples/06_multi_channel/` 的方式。
- **慢消费者用阻塞反压**：单 Bus 的 `rxCh` 满时 reader 已经会丢帧；合流层再丢就是双重丢失。阻塞让用户感知到处理不过来，可以扩 buffer 或加并行处理。
- **Each 故意不传可写入的 group 句柄**：避免迭代中 Add/Close 的死锁陷阱。
- **不提供 Remove(name)**：YAGNI，业务上动态拔通道少见。需要时整体 Close + 重建 group。

### 4.2 错误类型

```go
var ErrInvalidName    = errors.New("gocan: invalid bus name")
var ErrDuplicateName  = errors.New("gocan: duplicate bus name in group")
// 复用 errors.go 已有的 ErrNotSupported（"can: operation not supported on this platform"）
// 用于 *Bus 上的 SocketCAN 运行期方法在非 Linux 平台的占位返回。

// GroupCloseError 聚合 BusGroup.Close 时多个 Bus 的失败。
type GroupCloseError struct {
    Causes map[string]error
}

func (e *GroupCloseError) Error() string
func (e *GroupCloseError) Unwrap() []error
// errors.Is(err, ErrBusClosed) 等会按 Causes 逐个尝试匹配
```

## 5. Linux 专属 Option（7 项）

全部加在新文件 `options_linux.go`（带 `//go:build linux`）。其他平台编译时函数不存在 → 误用编译期报错。

### 5.1 linuxConfig 子结构

```go
//go:build linux

type linuxConfig struct {
    loopback        *bool          // nil = 内核默认（true）
    recvOwnMsgs     *bool          // nil = 内核默认（false）
    errFilter       *uint32        // nil = 不设置；非 nil 写入 CAN_RAW_ERR_FILTER
    joinFilters     *bool          // nil = 不设置（默认 OR）；true = AND
    rxTimestamp     RxTimestamp    // RxTimestampNone | Sec | Nano | Hardware
    soRcvBuf        int            // 0 = 不设置
    soSndBuf        int            // 0 = 不设置
    readTimeout     time.Duration  // 0 = 不设置
    writeTimeout    time.Duration  // 0 = 不设置
}
```

`config.linux` 字段在非 Linux 构建是空 struct，零成本。

### 5.2 Option 清单

| Option | setsockopt | 默认/不设置时 | 错误处理 |
|---|---|---|---|
| `WithLoopback(bool)` | `SOL_CAN_RAW` `CAN_RAW_LOOPBACK` | 内核默认 true | Open 失败回滚 |
| `WithRecvOwnMsgs(bool)` | `SOL_CAN_RAW` `CAN_RAW_RECV_OWN_MSGS` | 内核默认 false | Open 失败回滚 |
| `WithErrFilter(mask uint32)` | `SOL_CAN_RAW` `CAN_RAW_ERR_FILTER` | 不设置 | Open 失败回滚 |
| `WithJoinFilters(and bool)` | `SOL_CAN_RAW` `CAN_RAW_JOIN_FILTERS` | 不设置（OR） | Open 失败回滚；老内核 ENOPROTOOPT 也回滚（错误信息提示需 ≥4.1） |
| `WithRecvTimestamp(mode RxTimestamp)` | `SOL_SOCKET` `SO_TIMESTAMP` / `SO_TIMESTAMPNS` / `SO_TIMESTAMPING(HW)` | 不设置 | Open 失败回滚；硬件时间戳不被支持 → Logger 记录并降级到 `SO_TIMESTAMPNS`（纳秒软件时间戳）|
| `WithSocketBuffers(rcv, snd int)` | `SOL_SOCKET` `SO_RCVBUF` / `SO_SNDBUF` | 不设置 | Open 失败回滚 |
| `WithRWTimeout(rTO, wTO time.Duration)` | `SOL_SOCKET` `SO_RCVTIMEO` / `SO_SNDTIMEO` | 不设置（无超时） | Open 失败回滚 |

### 5.3 RxTimestamp 模式

```go
type RxTimestamp uint8
const (
    RxTimestampNone     RxTimestamp = iota // 不启用（默认）
    RxTimestampSecond                      // SO_TIMESTAMP（μs 精度）
    RxTimestampNano                        // SO_TIMESTAMPNS（ns 精度）
    RxTimestampHardware                    // SO_TIMESTAMPING + RX_HARDWARE，不支持时降级
)
```

CAN_ERR_* 错误位掩码常量在新文件 `raw/can_err_linux.go` 重新导出，业务侧用 `gocan.CANErrTxTimeout | gocan.CANErrBusOff` 即可。

### 5.4 时间戳暴露策略

`Frame` 已有 `TimestampMicros uint64` 和 `ReceivedAt time.Time` 字段（见 `frame.go`）。当前 SocketCAN 后端在 `raw/socketcan_linux.go` 的 `fillTimestamp` 用 `time.Now()` 合成一个 PCAN 风格的时间戳——精度受用户态调度影响，不是真正的内核时间戳。

`WithRecvTimestamp(mode)` 的语义是把这个合成时间戳替换成**内核时间戳**：

- 实现方式：当 `mode != RxTimestampNone` 时，SocketCAN 后端从 `read(2)` 切换到 `recvmsg(2)`，从控制消息（`SCM_TIMESTAMP` / `SCM_TIMESTAMPNS` / `SCM_TIMESTAMPING`）里取出 `struct timeval` / `struct timespec`，转成 μs 写入 `Frame.TimestampMicros`
- 不启用该 Option 时，`Read`/`ReadFD` 走原来的 `read()` 路径，`fillTimestamp` 行为不变（兼容性保证）
- `Frame.ReceivedAt` 始终是 `time.Now()`（Go 进程层面的接收时刻）
- 公开 API 不新增方法。`ReadOne` / `Receive` / `BusGroup.Receive` 直接返回带正确 `TimestampMicros` 的 Frame

`linuxChannel` 加一个 `rxTimestampMode RxTimestamp` 字段，`Read`/`ReadFD` 在入口处分支选择 `read()` 或 `recvmsg()`。

### 5.5 生效时机与回滚

所有 setsockopt 在 `Initialize` 成功后、`startReader` 之前一次性应用。任意一项失败 → `Uninitialize(ch)` 回滚 → 返回 `*Error{Op: "setsockopt(NAME)"}`。

零值兼容：所有字段是 pointer 或 0 哨兵。**没传 Option 的字段一律不调 setsockopt**。既存调用者行为完全不变。

## 6. *Bus 运行期方法

```go
// bus_socketcan_linux.go

// SetErrFilter 运行期更新 CAN_RAW_ERR_FILTER 掩码。
// setsockopt 失败时返回错误，linuxChannel 里持久化的 mask 保持原值（与内核状态一致）。
func (b *Bus) SetErrFilter(mask uint32) error

// SetJoinFilters 运行期更新 CAN_RAW_JOIN_FILTERS（true = AND，false = OR）。
// setsockopt 失败时返回错误，linuxChannel 里持久化的标志保持原值。
func (b *Bus) SetJoinFilters(and bool) error
```

只暴露这两个的理由：其他参数（buffer 大小、超时、loopback、时间戳模式）运行期改会触发 reader goroutine 协调，超出本轮范围。

跨平台编译屏障 `bus_socketcan_other.go`（`//go:build !linux`）提供同名方法返回 `ErrNotSupported`，让跨平台代码能编译过。

## 7. 4 个示例

### 7.1 examples/11_busgroup_socketcan/main.go — BusGroup 最小演示

替代 `06_multi_channel` 的"两个 *Bus 各管各的"写法。剧本：

1. `gocan.NewBusGroup(0)` 建空 group
2. `group.AddFD("front", gocan.SocketCAN("vcan0"), "")` 加 front
3. 同样加 chassis（vcan1）
4. `defer group.Close()` 一行收尾
5. `for sf := range group.Receive()` 打印 `[front] id=0x123 ...`
6. SIGINT 退出，末尾打印 Close 聚合错误（如有）

目标 ~60 行（比 `06_multi_channel` 少一半）。

### 7.2 examples/12_busgroup_fan_in/main.go — 多通道 fan-in + 业务路由

演示 `SourcedFrame` 用业务名字做路由：

1. 添加三通道 `engine` / `body` / `telemetry`
2. 主循环 `for sf := range group.Receive()`，`switch sf.Source` 走不同处理函数
3. 退出前用 `group.Each` 打每个 Bus 的 `bus.Status()` 做收尾诊断

目标 ~80 行。

### 7.3 examples/13_socketcan_loopback/main.go — 自发自收回归测试

演示 `WithLoopback(true) + WithRecvOwnMsgs(true)` 的组合，单 vcan 跑收发闭环：

1. `Open(SocketCAN("vcan0"), WithLoopback(true), WithRecvOwnMsgs(true))`
2. reader goroutine 收帧、计数
3. 主线程发 100 帧 ID=0x100..0x163
4. 收到 100 帧、ID 全匹配 → PASS
5. 注释段说明：去掉 `WithRecvOwnMsgs(true)` 时收不到自己发的帧

目标 ~70 行。

### 7.4 examples/14_socketcan_advanced/main.go — 全家桶

四段串起来：

```go
// (1) 错误帧诊断
gocan.WithErrFilter(gocan.CANErrBusOff | gocan.CANErrTxTimeout | gocan.CANErrCrtl)

// (2) 过滤器 AND 语义
gocan.WithJoinFilters(true)

// (3) 内核纳秒时间戳，业务用 ReadOneTimestamped 拿时间
gocan.WithRecvTimestamp(gocan.RxTimestampNano)

// (4) 高吞吐调优
gocan.WithSocketBuffers(2*1024*1024, 1*1024*1024)
gocan.WithRWTimeout(500*time.Millisecond, 0)
```

每段 5~10 行注释说明使用场景。运行期段最后演示 `bus.SetErrFilter(0)` 关掉错误帧订阅。

目标 ~130 行（本批最长）。

### 7.5 共同约定

- 顶端注释延续 `examples/01_send_classical/`：`// 运行: go run ...` / `// 前置: ...`
- 所有示例只在 Linux 有意义；注释里写明"仅 Linux 可运行"
- 全部用 `signal.NotifyContext` 处理 SIGINT
- 注释里引用 `docs/socketcan-options.md` 对应章节

## 8. scripts/setup-vcan.sh

40 行 bash，`set -euo pipefail`，错误信息中文。

```
用法: sudo ./scripts/setup-vcan.sh [up|down] [iface...]
默认: up vcan0 vcan1
```

逻辑：
- 检查 vcan 模块；缺则 modprobe vcan
- 对每个 iface：`ip link add <iface> type vcan` + `ip link set <iface> up`
- down 子命令：set down + delete
- 已存在的接口幂等跳过

`justfile` 加两条 alias：`just vcan-up` / `just vcan-down`。

## 9. docs/socketcan-options.md（约 700 行成稿）

```
# SocketCAN 自定义参数

## 1. 概览（一张表对应 7 个 Option ↔ kernel setsockopt ↔ 内核版本要求）
## 2. 何时设置（Open 时一次性应用 vs 运行期方法）
## 3. 错误处理总则（任意 setsockopt 失败 → Uninitialize 回滚 → *Error）
## 4. 各 Option 详解
   4.1 WithLoopback(bool)               - CAN_RAW_LOOPBACK
   4.2 WithRecvOwnMsgs(bool)            - CAN_RAW_RECV_OWN_MSGS
   4.3 WithErrFilter(uint32)            - CAN_RAW_ERR_FILTER（含 CAN_ERR_* 位掩码表）
   4.4 WithJoinFilters(bool)            - CAN_RAW_JOIN_FILTERS（≥ 4.1）
   4.5 WithRecvTimestamp(RxTimestamp)   - SO_TIMESTAMP / NS / TIMESTAMPING
   4.6 WithSocketBuffers(rcv,snd int)   - SO_RCVBUF / SO_SNDBUF（受 net.core.rmem_max）
   4.7 WithRWTimeout(rTO,wTO)           - SO_RCVTIMEO / SO_SNDTIMEO
## 5. 运行期可调（SetErrFilter / SetJoinFilters）
## 6. 比特率配置（不在库里 — 用 ip link 或 scripts/setup-vcan.sh）
## 7. 与 PCAN 后端的差异速查表
```

每节附最小代码片段 + `man 7 socket` / `Documentation/networking/can.rst` 引用。读者预期：已经懂 SocketCAN 概念但没用过 gocan 的工程师。**不重复 kernel 文档**，只讲映射。

## 10. 测试策略

| 层 | 文件 | 内容 | 真硬件 |
|---|---|---|---|
| 单元 | `busgroup_test.go` | Add/Get/Names/Each/Close 全路径，重复名/空名/Close 后再用，错误聚合 | 否，复用 fakeAdapter |
| 单元 | `options_linux_test.go` | 每个 Option 把 linuxConfig 对应字段写对了；零值时不动 | 否 |
| 单元 | `raw/socketcan_options_linux_test.go` | setsockopt 包装函数对 enum / pointer 处理正确 | 否（用 socketpair / unix-domain 假 socket） |
| 集成 | `socketcan_options_integration_linux_test.go` | 打开 vcan，依次设/取每个 Option，loopback + RecvOwnMsgs 自发自收闭环 | 是，没 vcan 时 `t.Skip("vcan unavailable")` |
| 编译 | `bus_socketcan_other.go` | 跨平台代码用 `b.SetErrFilter(...)` 在 darwin/windows 也能编译过 | N/A |

CI 矩阵保持现状（`go vet ./...` + `go test ./...`）。集成测试需要 vcan 才跑全 → CI 自然 skip，本地 + 真机才会真跑。

## 11. 工作量估算

| 模块 | 代码 | 测试 | 文档 |
|---|---|---|---|
| BusGroup | ~250 行 | ~300 行 | README 加段落 |
| Linux Option（含 raw 薄封装） | ~200 行 | ~250 行单元 + ~150 行集成 | docs/socketcan-options.md ~700 行 |
| Bus 运行期方法 | ~50 行 | ~80 行 | 同文档 |
| 4 个示例 | ~340 行 | — | 各文件顶部注释 |
| setup-vcan.sh + justfile alias | ~50 行 | — | docs 引用 |
| **合计** | **~890 行 Go** | **~780 行测试** | **~700 行 md + 50 行 sh** |

合并后约 8~10 个 commit、1 个 PR，按主题切分：busgroup → linux options 基础 → 时间戳 → buffer/timeout → bus 运行期方法 → 示例 → 文档/脚本。

## 12. 范围之外（明确不做）

- Windows PCAN 后端的 PCAN_LISTEN_ONLY / PCAN_ALLOW_*_FRAMES / PCAN_BUSOFF_AUTORESET 等专属 Option（后续单独 PR）
- 跨平台 Option 抽象层（明确否决：会隐藏平台差异、增加调试难度）
- netlink 动态设比特率 / sample-point（明确否决：要 root，跨内核兼容麻烦；改用脚本）
- BusGroup.Remove(name)（YAGNI）
- Frame 加 RxTime 字段（不需要——Frame 已有 `TimestampMicros` 和 `ReceivedAt`）
- 新增 `ReadOneTimestamped` 方法（不需要——直接通过现有 `Frame.TimestampMicros` 暴露）
- 运行期改 buffer / 超时 / loopback / 时间戳模式（涉及 reader 协调，超出本轮）

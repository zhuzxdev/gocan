# pcanbasic_go 设计文档

- **版本**：v0.1.0 设计稿
- **日期**：2026-05-22
- **作者**：zhuzhixiang
- **仓库**：`git@github.com:Crush251/pcanbasic_go.git`
- **本地路径**：`/home/linkerhand/recode/Go_win_can/`
- **Module path**：`github.com/Crush251/pcanbasic_go`

---

## 1. 背景与目标

### 1.1 背景

PEAK-System 的 PCAN-USB 系列是工业 / 机器人控制中常用的 CAN 接口卡。其官方驱动包
`PCANBasic` 提供 Windows、Linux、macOS 多平台支持，但**官方仅为 C/C++、C#、Pascal、
VB.NET、Java、Python 提供绑定**，没有 Go 版本。

在我们的实际场景中：

- **Linux** 端：直接走 `socket-can`，社区有成熟的 Go 库（如 `brutella/can`）
- **Windows** 端：目前依赖 Python 实现（`can-bridge-win`），通过 HTTP 桥接给 Go
  上层调用，存在以下痛点：
  - 多一层进程与 HTTP 序列化开销
  - 需要 PyInstaller 打包，部署体积大、问题定位链路长
  - 团队主体技术栈是 Go，长期维护 Python 是负担

GitHub 上检索 `PCANBasic + Go` 当前**零开源仓库**，这是个明确的空缺。

### 1.2 目标

构建一个**Windows 专用的 Go 版 PCANBasic 封装库** `pcanbasic_go`，使 Go 程序无需
经过 Python/C++ 中间层即可直接收发 CAN/CAN FD 报文。

非目标（v0.1）：

- ❌ 不做 HTTP / RPC 桥接服务（那是 `can-bridge-win` 的下一个 Go 重写项目，复用本库）
- ❌ 不做 Linux 真机支持（Linux 走 socket-can，未来由"统一 CAN 抽象包"按需调用）
- ❌ 不做 CAN XL（延后到 v0.3）

### 1.3 范围

| 维度 | v0.1 范围 |
|---|---|
| CAN 标准 | Classical CAN + CAN FD |
| 通道 | `PCAN_USBBUS1` 至 `PCAN_USBBUS16`（与 Python 版一致）；其余通道常量也定义但不强保 |
| 接收模型 | Polling + Event（Windows Event Handle）+ Auto |
| 平台 | Windows 真实工作；Linux/macOS 编译桩 |
| API 层级 | 顶层 `pcanbasic`（高层 Bus/Frame）+ 子包 `raw`（C API 1:1） |

---

## 2. 总体架构

### 2.1 包结构

```
github.com/Crush251/pcanbasic_go         # 顶层高层 API
├── doc.go
├── pcanbasic.go        # Open / OpenFD
├── bus.go              # Bus 类型 + reader goroutine
├── frame.go            # Frame + 构造器
├── options.go          # Option 模式
├── errors.go           # *Error + 哨兵
├── status.go           # Status (alias raw.TPCANStatus)
├── adapter.go          # rawAdapter 接口 + liveAdapter
└── raw/                # 低层 C API 绑定（公开）
    ├── doc.go
    ├── types.go
    ├── consts.go
    ├── api_windows.go  # syscall 实现
    └── api_other.go    # 非 Windows 桩
```

### 2.2 调用链

```
用户代码
  │
  ▼
pcanbasic.Bus.Send(ctx, frame)
  │
  │  Frame → raw.TPCANMsg / TPCANMsgFD
  ▼
rawAdapter (默认 liveAdapter)
  │
  ▼
raw.Write / raw.WriteFD
  │
  ▼
windows.LazyProc.Call → PCANBasic.dll
```

### 2.3 双层的意义

- **顶层 `pcanbasic`**：给 95% 用户使用，提供 idiomatic Go API（channel、context、
  Option、typed error）
- **子包 `raw`**：保留 PCAN 全部低层能力（Trace / Flash / 设备信息 / SetValue 任意
  参数等），供：
  - PCAN 高级功能需求方
  - 未来"跨平台 CAN 抽象包"（独立 module，无法 import internal）
  - 临时绕过顶层封装的调试场景

---

## 3. 详细 API 设计

### 3.1 `Frame` 与构造器

```go
// Frame 代表一帧 CAN 报文，统一承载 Classical / Extended / Remote / FD。
type Frame struct {
    ID    uint32     // 11 位（标准）或 29 位（扩展）
    Data  []byte     // Classical: 0..8；FD: 0..64（仅离散 DLC 值）
    Flags FrameFlags

    // TimestampMicros 是 PCAN 驱动给出的原始时间戳（微秒）。
    // Windows 下为"自系统启动以来的微秒数"；不同平台语义可能不同，
    // 因此不直接转换为 time.Time，请按需自行处理。
    TimestampMicros uint64

    // ReceivedAt 是本库收到该帧并构造 Frame 时的本地墙钟时间。
    // 发送 Frame 时此字段被忽略。
    ReceivedAt time.Time
}

type FrameFlags uint16

const (
    FlagExtended FrameFlags = 1 << iota // 29 位扩展 ID
    FlagRemote                          // 远程帧（仅 Classical 合法）
    FlagFD                              // FD 帧
    FlagBRS                             // 加速段（FD 专用）
    FlagESI                             // 错误状态指示（FD 专用）
)
```

构造器（全部返回 `(Frame, error)`，**深拷贝 data**）：

```go
func NewFrame(id uint32, data []byte) (Frame, error)
func NewExtendedFrame(id uint32, data []byte) (Frame, error)
func NewRemoteFrame(id uint32, dlc uint8, extended bool) (Frame, error)
func NewFDFrame(id uint32, data []byte, brs, extended bool) (Frame, error)
```

校验规则：

| 项 | 规则 | 失败 |
|---|---|---|
| 标准 ID | `id <= 0x7FF` | `ErrIDOutOfRange` |
| 扩展 ID | `id <= 0x1FFFFFFF` | `ErrIDOutOfRange` |
| Classical 数据长度 | `len(data) <= 8` | `ErrDataTooLong` |
| Remote DLC | `dlc <= 8` | `ErrDataTooLong` |
| FD 数据长度 | `len(data) ∈ {0,1,2,3,4,5,6,7,8,12,16,20,24,32,48,64}` | `ErrInvalidFDLength` |
| FD + Remote | 不允许（FD 协议无 RTR） | `ErrRemoteOnFD` |

实现要点：`Data: append([]byte(nil), data...)` 防止外部改写偷偷修改 Frame。

### 3.2 `Bus` 主类型

```go
// Open 打开 Classical CAN 通道。
func Open(ch Channel, opts ...Option) (*Bus, error)

// OpenFD 打开 CAN FD 通道。fdBitrate 是 PCAN 官方定义的比特率字符串
// （见 docs/can-fd.md，例如 "f_clock=80000000,nom_brp=2,nom_tseg1=63,..."）。
func OpenFD(ch Channel, fdBitrate string, opts ...Option) (*Bus, error)

// 关闭 Bus。幂等：多次调用安全，仅第一次真正释放底层资源。
func (b *Bus) Close() error

// 发送
func (b *Bus) Send(ctx context.Context, f Frame) error
func (b *Bus) SendMany(ctx context.Context, frames []Frame) error

// 接收
func (b *Bus) Receive() <-chan Frame                       // 流式接收
func (b *Bus) Errors() <-chan error                        // 接收侧异步错误
func (b *Bus) ReadOne(ctx context.Context) (Frame, error)  // 阻塞读一帧（不返回 ErrQueueEmpty）
func (b *Bus) TryRead() (Frame, error)                     // 非阻塞，队列空返回 ErrQueueEmpty

// 状态 / 控制
func (b *Bus) Status() (Status, error)
func (b *Bus) Reset() error
func (b *Bus) SetFilter(idMin, idMax uint32, mode FilterMode) error
func (b *Bus) ResetFilter() error
```

**关键行为承诺**（写入 godoc）：

1. 任意时刻只有一个内部 reader goroutine 调底层 `Read` / `ReadFD`；
   `Receive` / `ReadOne` / `TryRead` 三者都从 reader 维护的 `rxCh` 取数据。
2. `Close()` 后调 `Send` 返回 `ErrBusClosed`；`<-Receive()` 立即得到 `(zero, false)`；
   `<-Errors()` 也已关闭。
3. `Close()` 幂等：第二次开始为 no-op，不会重复 `Uninitialize`。
4. `SendMany` 顺序逐帧发送；任意一帧失败立即返回 `*SendManyError`，**已成功发送的
   不回滚**；ctx 取消时同样返回 `*SendManyError{Err: ctx.Err()}`。
5. OpenFD 打开的 Bus **允许**发送非 FD（Classical）帧（PCAN 文档与 python-can 行为一致）；
   Open（Classical）打开的 Bus **拒绝**发送 FD 帧，返回 `ErrFDNotSupportedOnBus`。

### 3.3 `Option` 与默认值

```go
type Option func(*config)

func WithBitrate(b Bitrate) Option            // 默认 Baud1M
func WithReceiveMode(m ReceiveMode) Option    // 默认 ModeAuto
func WithPollInterval(d time.Duration) Option // 默认 1ms（仅 Polling 时生效）
func WithRxBufferSize(n int) Option           // 默认 1024（rx channel 容量）
func WithErrBufferSize(n int) Option          // 默认 16
func WithLogger(l Logger) Option              // 默认 noopLogger

type ReceiveMode int

const (
    // ModeAuto：库自行决定。Windows + 驱动支持事件 → Event；否则退回 Polling。
    ModeAuto ReceiveMode = iota
    ModePolling
    ModeEvent
)
```

### 3.4 `Channel` / `Bitrate` 常量

直接 alias `raw` 常量，避免重复定义：

```go
type Channel = raw.TPCANHandle
type Bitrate = raw.TPCANBaudrate

const (
    USBBus1  Channel = raw.PCAN_USBBUS1
    USBBus2  Channel = raw.PCAN_USBBUS2
    // ... USBBus16
    Baud1M   Bitrate = raw.PCAN_BAUD_1M
    Baud500K Bitrate = raw.PCAN_BAUD_500K
    Baud250K Bitrate = raw.PCAN_BAUD_250K
    Baud125K Bitrate = raw.PCAN_BAUD_125K
    Baud100K Bitrate = raw.PCAN_BAUD_100K
    Baud50K  Bitrate = raw.PCAN_BAUD_50K
    Baud20K  Bitrate = raw.PCAN_BAUD_20K
    Baud10K  Bitrate = raw.PCAN_BAUD_10K
    Baud5K   Bitrate = raw.PCAN_BAUD_5K
)
```

### 3.5 错误与 Status

#### 3.5.1 `Error` 类型

```go
type Error struct {
    Code raw.TPCANStatus
    Op   string
    Msg  string
}

func (e *Error) Error() string {
    if e.Msg != "" {
        return fmt.Sprintf("pcanbasic: %s failed: 0x%08X: %s",
            e.Op, uint32(e.Code), e.Msg)
    }
    return fmt.Sprintf("pcanbasic: %s failed: 0x%08X", e.Op, uint32(e.Code))
}

// Has 判断错误码中是否包含某个具体错误位。
// 特别处理 PCAN_ERROR_OK (0)：仅当 e.Code 也是 0 时才算"包含"。
func (e *Error) Has(code raw.TPCANStatus) bool {
    if code == raw.PCAN_ERROR_OK {
        return e.Code == raw.PCAN_ERROR_OK
    }
    return e.Code&code == code
}

// Is 让单个 *Error 可以同时匹配多个哨兵（位掩码语义）。
func (e *Error) Is(target error) bool {
    switch target {
    case ErrQueueEmpty:    return e.Has(raw.PCAN_ERROR_QRCVEMPTY)
    case ErrQueueOverrun:  return e.Has(raw.PCAN_ERROR_QOVERRUN)
    case ErrQueueXmtFull:  return e.Has(raw.PCAN_ERROR_QXMTFULL)
    case ErrBusLight:      return e.Has(raw.PCAN_ERROR_BUSLIGHT)
    case ErrBusHeavy:      return e.Has(raw.PCAN_ERROR_BUSHEAVY)
    case ErrBusPassive:    return e.Has(raw.PCAN_ERROR_BUSPASSIVE)
    case ErrBusOff:        return e.Has(raw.PCAN_ERROR_BUSOFF)
    case ErrNotInitialized:return e.Has(raw.PCAN_ERROR_INITIALIZE)
    case ErrIllHandle:     return e.Has(raw.PCAN_ERROR_ILLHANDLE)
    case ErrIllParamValue: return e.Has(raw.PCAN_ERROR_ILLPARAMVAL)
    case ErrIllParamType:  return e.Has(raw.PCAN_ERROR_ILLPARAMTYPE)
    case ErrIllOperation:  return e.Has(raw.PCAN_ERROR_ILLOPERATION)
    case ErrNoDriver:      return e.Has(raw.PCAN_ERROR_NODRIVER)
    }
    return false
}
```

#### 3.5.2 哨兵错误清单

```go
var (
    // 库参数校验类
    ErrIDOutOfRange       = errors.New("pcanbasic: CAN ID out of range")
    ErrDataTooLong        = errors.New("pcanbasic: data length exceeds capacity")
    ErrInvalidFDLength    = errors.New("pcanbasic: invalid CAN FD data length")
    ErrRemoteOnFD         = errors.New("pcanbasic: remote frame not allowed on CAN FD")
    ErrBusClosed          = errors.New("pcanbasic: bus is closed")
    ErrNotSupported       = errors.New("pcanbasic: operation not supported on this platform")
    ErrDLLNotFound        = errors.New("pcanbasic: PCANBasic.dll not found or failed to load")
    ErrFDNotSupportedOnBus = errors.New("pcanbasic: FD frame requires a bus opened with OpenFD")

    // 队列状态
    ErrQueueEmpty   = errors.New("pcanbasic: receive queue empty")
    ErrQueueOverrun = errors.New("pcanbasic: receive queue overrun")
    ErrQueueXmtFull = errors.New("pcanbasic: transmit queue full")

    // 总线状态
    ErrBusLight   = errors.New("pcanbasic: bus light")
    ErrBusHeavy   = errors.New("pcanbasic: bus heavy")
    ErrBusPassive = errors.New("pcanbasic: bus passive")
    ErrBusOff     = errors.New("pcanbasic: bus off")

    // API / 驱动
    ErrNotInitialized = errors.New("pcanbasic: channel not initialized")
    ErrIllHandle      = errors.New("pcanbasic: invalid channel handle")
    ErrIllParamType   = errors.New("pcanbasic: invalid parameter type")
    ErrIllParamValue  = errors.New("pcanbasic: invalid parameter value")
    ErrIllOperation   = errors.New("pcanbasic: illegal operation")
    ErrNoDriver       = errors.New("pcanbasic: driver not loaded")
    ErrUnknown        = errors.New("pcanbasic: unknown error")
)
```

#### 3.5.3 `SendManyError`

```go
// SendManyError 标识 SendMany 中第 Index 帧（0-based）发送失败。
type SendManyError struct {
    Index int    // 失败的帧下标
    Frame Frame  // 失败的帧本身（深拷贝，不会被外部改动）
    Err   error  // 底层错误（通常是 *Error）
}

func (e *SendManyError) Error() string {
    return fmt.Sprintf("pcanbasic: SendMany failed at frame[%d]: %v", e.Index, e.Err)
}
func (e *SendManyError) Unwrap() error { return e.Err }
```

#### 3.5.4 `Status`

```go
type Status = raw.TPCANStatus

const (
    StatusOK           Status = raw.PCAN_ERROR_OK
    StatusBusLight     Status = raw.PCAN_ERROR_BUSLIGHT
    StatusBusHeavy     Status = raw.PCAN_ERROR_BUSHEAVY
    StatusBusPassive   Status = raw.PCAN_ERROR_BUSPASSIVE
    StatusBusOff       Status = raw.PCAN_ERROR_BUSOFF
    StatusQueueOverrun Status = raw.PCAN_ERROR_QOVERRUN
)

// 处理 0 特殊情况
func StatusHas(s, bit Status) bool {
    if bit == StatusOK {
        return s == StatusOK
    }
    return s&bit == bit
}
```

### 3.6 `raw` 子包

```go
type (
    TPCANHandle      uint16
    TPCANStatus      uint32
    TPCANBaudrate    uint16
    TPCANType        uint8
    TPCANMessageType uint8
    TPCANParameter   uint8
)

// 注意：字段顺序与 C 结构 TPCANMsg 一致。
// 因 Go 自然对齐，unsafe.Sizeof(TPCANMsg{}) 可能为 16 而非 C 的 14。
// 这不影响 PCANBasic 调用 —— DLL 仅访问前 14 字节，尾部 padding 是 Go 自有内存，
// 对端不感知。
type TPCANMsg struct {
    ID      uint32
    MsgType TPCANMessageType
    Len     uint8
    Data    [8]byte
}

type TPCANMsgFD struct {
    ID      uint32
    MsgType TPCANMessageType
    DLC     uint8
    Data    [64]byte
}

type TPCANTimestamp struct {
    Millis         uint32
    MillisOverflow uint16
    Micros         uint16
}

type TPCANTimestampFD = uint64

// 显式触发 PCANBasic.dll 加载；失败时所有后续调用返回 PCAN_ERROR_NODRIVER。
func EnsureLoaded() error

// 与 C API 1:1
func Initialize(ch TPCANHandle, br TPCANBaudrate) TPCANStatus
func InitializeFD(ch TPCANHandle, bitrateFD string) TPCANStatus
func Uninitialize(ch TPCANHandle) TPCANStatus
func Reset(ch TPCANHandle) TPCANStatus
func Read(ch TPCANHandle, m *TPCANMsg, ts *TPCANTimestamp) TPCANStatus
func ReadFD(ch TPCANHandle, m *TPCANMsgFD, ts *TPCANTimestampFD) TPCANStatus
func Write(ch TPCANHandle, m *TPCANMsg) TPCANStatus
func WriteFD(ch TPCANHandle, m *TPCANMsgFD) TPCANStatus
func GetStatus(ch TPCANHandle) TPCANStatus
func GetErrorText(code TPCANStatus, lang uint16) (string, TPCANStatus)
func FilterMessages(ch TPCANHandle, fromID, toID uint32, mode TPCANMessageType) TPCANStatus
func GetValue(ch TPCANHandle, param TPCANParameter, buf unsafe.Pointer, n uint32) TPCANStatus
func SetValue(ch TPCANHandle, param TPCANParameter, buf unsafe.Pointer, n uint32) TPCANStatus
```

#### 3.6.1 DLL 加载策略

```go
// 路径搜索顺序：
//   1. 环境变量 PCANBASIC_DLL_PATH（绝对路径或文件名）
//   2. "PCANBasic.dll"（Windows LoadLibrary 标准搜索）
// 加载失败 → 所有 procX 设为 nil；每个 API 调用检测到 nil 直接返回 PCAN_ERROR_NODRIVER。
```

#### 3.6.2 非 Windows 桩（`api_other.go`）

```go
//go:build !windows

// 不打印任何东西。所有函数返回 PCAN_ERROR_ILLOPERATION，由高层映射为 ErrNotSupported。
func Initialize(ch TPCANHandle, br TPCANBaudrate) TPCANStatus {
    return PCAN_ERROR_ILLOPERATION
}
// ... 其余同理
```

---

## 4. 接收模型实现

### 4.1 单 reader 架构

```
                          ┌─────────────────────┐
   PCAN 驱动队列 ───────►  │ 单 reader goroutine  │  rxCh ──► Receive() / ReadOne() / TryRead()
   （up to 32768/ch）      │  + Event handle 等待  │  errCh ──► Errors()
                          │  或 ticker poll      │
                          └─────────────────────┘
```

**重要**：所有 PCAN 接收调用都由这一个 goroutine 完成，避免两个调用点同时抢空底层队列
导致数据归属混乱。

### 4.2 reader goroutine 主循环

```go
func (b *Bus) readerLoop() {
    defer close(b.rxCh)
    for {
        // 1) 等待"有消息"信号
        if err := b.waitForData(); err != nil {
            return // ctx canceled / event handle closed
        }

        // 2) 一直 drain 到队列空（PCAN 文档要求）
        for {
            f, err := b.readNative()
            if errors.Is(err, errQueueEmpty) {
                break // 仅作"队列空"信号，绝不推到 errCh
            }
            if err != nil {
                select {
                case b.errCh <- err:
                default: // errCh 满则丢弃最旧
                }
                break
            }
            select {
            case b.rxCh <- f:
            case <-b.closing:
                return
            }
        }
    }
}
```

`waitForData()`：

- `ModeEvent` → `WaitForSingleObject(eventHandle, INFINITE)`
- `ModePolling` → `time.Sleep(pollInterval)`
- `ModeAuto` → 启动时尝试注册 `PCAN_RECEIVE_EVENT`，成功用 Event，失败回退 Polling，
  并通过 `Logger.Info` 告知

### 4.3 Close 语义

```
1. 关闭 b.closing channel
2. (Event 模式) SetEvent(eventHandle) 唤醒 reader
3. 等 reader 退出（reader 关闭 rxCh）
4. CAN_Uninitialize
5. (Event 模式) CloseHandle(eventHandle)
6. 关闭 errCh
7. 标记 b.closed = true（用于幂等）
```

Close 幂等通过 `sync.Once` + `atomic.Bool` 保证。

### 4.4 ReadOne / TryRead

```go
func (b *Bus) ReadOne(ctx context.Context) (Frame, error) {
    select {
    case f, ok := <-b.rxCh:
        if !ok {
            return Frame{}, ErrBusClosed
        }
        return f, nil
    case <-ctx.Done():
        return Frame{}, ctx.Err()
    }
}

func (b *Bus) TryRead() (Frame, error) {
    select {
    case f, ok := <-b.rxCh:
        if !ok {
            return Frame{}, ErrBusClosed
        }
        return f, nil
    default:
        return Frame{}, ErrQueueEmpty
    }
}
```

---

## 5. 测试策略

### 5.1 三层架构

```
┌─────────────────────────────────────────────────────────────────┐
│  Layer 1: 纯 Go 逻辑单测  (任何 OS 都跑)                          │
│           frame / errors / options / status                     │
├─────────────────────────────────────────────────────────────────┤
│  Layer 2: Bus 行为单测 (fake rawAdapter 注入)                    │
│           覆盖 reader / Close / 错误传播 / ctx 取消等             │
├─────────────────────────────────────────────────────────────────┤
│  Layer 3: 真机集成测试 (//go:build pcanhardware)                  │
│           Windows + PCAN-USB + 驱动，CI 默认不跑                  │
└─────────────────────────────────────────────────────────────────┘
```

### 5.2 `rawAdapter` 接口（最小集）

```go
type rawAdapter interface {
    Initialize(ch raw.TPCANHandle, br raw.TPCANBaudrate) raw.TPCANStatus
    InitializeFD(ch raw.TPCANHandle, bitrateFD string) raw.TPCANStatus
    Uninitialize(ch raw.TPCANHandle) raw.TPCANStatus

    Read(ch raw.TPCANHandle, m *raw.TPCANMsg, t *raw.TPCANTimestamp) raw.TPCANStatus
    ReadFD(ch raw.TPCANHandle, m *raw.TPCANMsgFD, t *raw.TPCANTimestampFD) raw.TPCANStatus
    Write(ch raw.TPCANHandle, m *raw.TPCANMsg) raw.TPCANStatus
    WriteFD(ch raw.TPCANHandle, m *raw.TPCANMsgFD) raw.TPCANStatus

    GetStatus(ch raw.TPCANHandle) raw.TPCANStatus
    GetErrorText(code raw.TPCANStatus, lang uint16) (string, raw.TPCANStatus)
    Reset(ch raw.TPCANHandle) raw.TPCANStatus
}
```

按 YAGNI 收紧；后续阶段加 `FilterMessages` / `SetValue` / `GetValue` 等。

### 5.3 必须覆盖的测试用例

| 用例 | 期望行为 |
|---|---|
| `TestFrame_NewXxx_Validation` | 各种越界/非法长度返回对应哨兵 |
| `TestFrame_DataIsCopied` | 改原 `data` 不影响构造好的 Frame |
| `TestError_Has_OKBoundary` | `Has(0)` 仅在 `Code==0` 时返回 true |
| `TestError_Is_Bitmask` | 复合 Code 同时匹配多个哨兵 |
| `TestStatus_Has_OKBoundary` | 同上 |
| `TestClose_Idempotent` | 连续 Close 两次不 panic、Uninitialize 只调 1 次 |
| `TestSend_AfterClose` | 返回 `ErrBusClosed` |
| `TestReceive_AfterClose` | rxCh 已关，立即得到 zero + ok=false |
| `TestErrorsChannel_AfterClose` | errCh 关闭 |
| `TestOpenClassical_RejectFDFrame` | 返回 `ErrFDNotSupportedOnBus` |
| `TestOpenFD_AcceptClassicalFrame` | 允许发送非 FD 帧 |
| `TestReader_SuppressQRCVEMPTY` | fake 持续 QRCVEMPTY，errCh 始终空 |
| `TestSendMany_PartialFailure` | 第 N 帧失败 → `*SendManyError{Index:N}` |
| `TestSendMany_CtxCancel` | ctx 取消 → `*SendManyError{Err:ctx.Err()}` |
| `TestReadOne_CtxCancel` | 立即返回 `ctx.Err()` |
| `TestTryRead_Empty` | 队列空返回 `ErrQueueEmpty` |
| `TestConcurrent_SendAndReceive` | 多 goroutine 并发，`-race` 必须过 |

### 5.4 覆盖率目标

- 纯 Go 逻辑（frame/options/errors/status）单测覆盖率 ≥ 80%
- Bus 行为路径覆盖（不强求行覆盖率）：
  打开 / 关闭 / Close 幂等 / Send / SendMany / 部分失败 / ctx 取消 /
  reader 启动 / QRCVEMPTY 抑制 / 错误传播 / Close 唤醒退出 /
  ReadOne / TryRead / 并发 race

### 5.5 真机测试

```go
//go:build pcanhardware

package pcanbasic_test
```

- 运行：`go test -tags=pcanhardware ./...`
- 需求：Windows + PCAN-USB + 已安装驱动 + DLL 在搜索路径
- 步骤文档：`docs/hardware-test-setup.md`
- CI 暂不跑（v0.2 再考虑自托管 Windows runner）

---

## 6. 文档与示例

### 6.1 文档结构

```
README.md                                              # 中文，主入口
README_en.md                                           # 英文简版
CHANGELOG.md                                           # Keep a Changelog 中文
docs/
├── quickstart.md
├── architecture.md
├── can-fd.md
├── error-handling.md
├── platform-support.md
├── hardware-test-setup.md
├── troubleshooting.md                                 # 含 FAQ
└── superpowers/specs/2026-05-22-pcanbasic-go-design.md
```

所有公共类型/函数必须有中文 godoc，包括：用途、参数取值范围、返回错误清单、阻塞性、
线程安全性，并尽量配 `Example_Xxx`。

### 6.2 示例（10 个）

```
examples/
├── 01_send_classical/main.go      # 发送一帧 Classical CAN
├── 02_receive_polling/main.go     # 用 Polling 模式接收
├── 03_receive_event/main.go       # 用 Event 模式接收（推荐生产）
├── 04_send_fd/main.go             # 发送 CAN FD（含 BRS）
├── 05_receive_fd/main.go          # 接收 CAN FD
├── 06_multi_channel/main.go       # 同时操作 USBBus1 + USBBus2
├── 07_filter/main.go              # SetFilter / ResetFilter
├── 08_status_and_reset/main.go    # 监测 Status，BUSOFF 时 Reset
├── 09_with_logger/main.go         # 集成 slog
└── 10_using_raw/main.go           # 直接调 raw 子包做高级操作
```

每个示例 ≤ 100 行，中文注释开头，运行命令在文件首行示例。

---

## 7. 版本与 CI

### 7.1 SemVer

- v0.x 阶段明示"可能 break"
- v1.0.0 时冻结 API

### 7.2 v0.1.0 完成定义（DoD）

- ✅ Classical + FD 收发可在真机跑通（见 hardware-test-setup.md）
- ✅ 三种接收模式可工作
- ✅ Bus 完整高层 + raw 子包基础 API
- ✅ 全部测试用例（§5.3）通过；`-race` 通过
- ✅ 10 个示例可编译
- ✅ 中文文档齐全
- ✅ Linux/Windows CI 全绿

### 7.3 路线图

| 版本 | 主要内容 |
|---|---|
| v0.2.0 | LookUpChannel、设备信息、Trace；Linux libpcanbasic.so 适配 |
| v0.3.0 | CAN XL |
| v1.0.0 | API 冻结 |

### 7.4 CI（GitHub Actions）

```yaml
jobs:
  linux:
    runs-on: ubuntu-latest
    steps:
      - go vet ./...
      - golangci-lint run
      - go test -race ./...
  windows:
    runs-on: windows-latest
    steps:
      - go vet ./...
      - go test ./...        # 不带 pcanhardware tag，纯 fake，不调真实 DLL
```

**测试代码约束**（写入 CONTRIBUTING）：

- `*_test.go` 不得直接调用真实 `raw.Initialize/Read/Write/...`（除真机测试外）
- `raw/api_test.go` 仅校验常量值、类型 size、函数签名编译，禁止 `procX.Call`
- 真机测试一律加 `//go:build pcanhardware`

---

## 8. 仓库初始化清单

```
1. cd /home/linkerhand/recode/Go_win_can
2. git init -b main
3. git config user.name "zhuzhixiang"            # 仓库级，确保署名一致
4. git config user.email "1849346915@qq.com"
5. git remote add origin git@github.com:Crush251/pcanbasic_go.git
6. 首次 commit：
     chore: initial repo scaffolding and design doc
   范围：LICENSE / README / CHANGELOG / .gitignore / go.mod / 设计文档
7. 本地暂不 push，由用户审阅 spec 后决定
```

**Commit 署名规则**：仅以 `zhuzhixiang <1849346915@qq.com>`，禁止 `Co-Authored-By: Claude`
等任何 AI 标记。

---

## 9. 决策记录

| # | 决策 | 关键理由 |
|---|---|---|
| 1 | 只做库，不做 HTTP 桥接 | 让库可被多个上层项目复用，HTTP 桥接是另一个项目 |
| 2 | 双层包（高层 + raw） | 95% 用户用高层；高级用户/上层抽象包能用 raw |
| 3 | `raw` 公开而非 `internal/raw` | 未来跨平台抽象包是独立 module，无法 import internal |
| 4 | 同时支 Classical + FD | FD 在新硬件普及，多写一套不重 |
| 5 | 三接收模式 + Auto 默认 | Event 性能最佳，Polling 兼容，Auto 给最佳默认 |
| 6 | Frame 统一类型 + Flags | 一个类型同时表达 Classical/Extended/Remote/FD，API 表面更小 |
| 7 | 构造器返回 `(Frame, error)` | 必须校验 ID 范围、FD DLC 离散值等 |
| 8 | 构造器深拷贝 data | 防止外部改写偷偷改已构造 Frame |
| 9 | `Error.Is(target)` 而非 `Unwrap` | 位掩码可同时匹配多个哨兵，Unwrap 只能匹配一个 |
| 10 | `Status` alias raw 常量 | 不能用 iota 重造，会与官方位值冲突 |
| 11 | ReadOne 永不返回 QRCVEMPTY | 它是"队列空"信号而非异常；TryRead 才暴露 |
| 12 | 单 reader goroutine | 避免两个调用点抢空底层队列 |
| 13 | OpenFD 允许 Classical 帧；Open 拒绝 FD 帧 | 与 PCAN/python-can 行为一致 |
| 14 | SendMany 不回滚已成功帧 | CAN 总线无事务概念，回滚是错误抽象 |
| 15 | `SendManyError{Index, Frame, Err}` | 用户需要知道是哪一帧失败 |
| 16 | 非 Windows 平台不在 init 打印 | 库不应污染调用方日志 |
| 17 | License MIT | 用户指定 |
| 18 | Commit 仅用 zhuzhixiang | 用户明确要求，不带 Claude 标记 |

---

## 10. 待办（spec 阶段不解决，留给 plan/实施）

- [ ] FD 比特率字符串构造助手是否需要（如 `pcanbasic.NewFDBitrate(...)`）
- [ ] `Logger` 接口最终签名（直接复用 `log/slog`？）
- [ ] 真机 CI 自托管 runner 方案
- [ ] `examples/10_using_raw` 选哪个 raw 功能演示（候选：GetValue 设备 ID / Hardware 版本）
- [ ] 是否提供 `pcanbasic.Available()` 探测 DLL 是否可用的工具函数

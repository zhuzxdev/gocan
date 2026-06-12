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

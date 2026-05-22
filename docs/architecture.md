# 架构

## 双层结构

```
+----------------------------------------------------+
|  顶层 idiomatic API                                |
|  Bus / Frame / Option / Error / Logger / SendMany  |
+----------------------------------------------------+
                       │ 调用
                       ▼
+----------------------------------------------------+
|  raw 子包  (薄包装 + syscall)                       |
|  Initialize / Read / ReadFD / Write / WriteFD ...  |
+----------------------------------------------------+
                       │ Windows syscall
                       ▼
                 PCANBasic.dll
```

- **顶层包**：把 PCAN 的 C 语义翻译成 Go 习惯——channel、context、interface、`errors.Is`。99% 的用户只用这层。
- **raw 子包**：和 DLL 一一对应，零抽象。需要 PCAN 特殊参数（`PCAN_API_VERSION`、`PCAN_LISTEN_ONLY` 等）时直接调它。

## 单 reader goroutine 模型

```
                +---------------------+
   PCAN queue → | reader goroutine    |
                |  loop:              |
                |   1. waitForData()  |
                |   2. drain via Read |
                |   3. push → rxCh    |
                +----------+----------+
                           │
            +--------------+--------------+
            ▼              ▼              ▼
        Receive()      ReadOne(ctx)   TryRead()
```

为什么单 reader：

- PCAN 的 `CAN_Read` 是非阻塞、共享队列。多 goroutine 直接调用会争抢同一个队列，且无法保证顺序。
- 集中到一个 goroutine 负责底层 Read 之后，上层并发拿帧只是从 channel 取，天然安全。

`Send` / `Status` / `Reset` 是无状态查询/动作，可以从任意 goroutine 直接调用，不走 reader。

## 接收模式

| 模式 | 平台 | 实现 | 何时用 |
|---|---|---|---|
| `ModePolling` | 全平台 | 周期 `time.Sleep(pollInterval)` 后 drain 队列 | 想完全可移植；可接受 ~1ms 抖动 |
| `ModeEvent` | Windows | `CreateEvent` + `WaitForMultipleObjects` | 低延迟、低 CPU；硬件支持 |
| `ModeAuto`（默认） | 全平台 | 先试 Event，失败降级 Polling | 不确定时这是好默认 |

## Close 流程

```
Close()
  │
  ├─ closeOnce.Do:
  │    ├─ b.closed = true            # 后续 Send/Status/... 返回 ErrBusClosed
  │    ├─ close(b.closing)           # 通知 reader 退出
  │    ├─ (Event) SetEvent(abort)    # 唤醒阻塞在 WaitFor... 的 reader
  │    ├─ for range b.rxCh { ... }   # 等 reader 关闭 rxCh
  │    ├─ (Event) CloseHandle(...)   # 释放 event 句柄
  │    ├─ CAN_Uninitialize           # 释放 PCAN 通道
  │    └─ close(b.errCh)             # Errors() 收到关闭信号
  ▼
```

幂等保证：`sync.Once` + `atomic.Bool`。多 goroutine 同时 Close 是安全的，第二次 Close 返回 `nil` 不报错。

## 错误模型

PCAN 错误码本身是 **位掩码**：一次 `CAN_Read` 可能同时报告 `BUSOFF | QOVERRUN`。库的处理：

1. 完整保留原码到 `Error.Code`
2. 实现 `Is(target error) bool` —— 同一 `*Error` 可同时匹配 `ErrBusOff`、`ErrQueueOverrun`
3. 用户通过 `errors.Is(err, ErrXxx)` 判断"是否包含某种错误位"

详情见 `docs/error-handling.md`。

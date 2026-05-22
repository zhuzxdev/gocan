# 错误处理

## `*Error` 结构

库内对 PCAN 状态码的封装：

```go
type Error struct {
    Code raw.TPCANStatus // 原始位掩码（可能是多 bit 的 OR）
    Op   string          // 触发错误的操作，如 "CAN_Read"
    Msg  string          // CAN_GetErrorText 返回的可读描述
}
```

`Error()` 输出形如：

```
pcanbasic: CAN_Read failed: 0x00000040: bus light error
```

## `errors.Is` 用法

PCAN 错误是位掩码——一次调用可能同时报 `BUSOFF | QOVERRUN`。库为此实现了 `(*Error).Is(target)` 方法，让 `errors.Is` 按位匹配：

```go
err := bus.Send(ctx, fr)
switch {
case errors.Is(err, pcanbasic.ErrBusOff):
    log.Println("总线断开，尝试 Reset 恢复")
    _ = bus.Reset()
case errors.Is(err, pcanbasic.ErrQueueXmtFull):
    time.Sleep(time.Millisecond)
    // 退避后重试
case errors.Is(err, context.Canceled):
    return // 调用方主动取消
case err != nil:
    log.Printf("send: %v", err)
}
```

## 常用哨兵错误

| 错误 | 对应 PCAN code | 何时出现 |
|---|---|---|
| `ErrBusClosed` | — (库内部) | 在 `Close()` 之后调用任意方法 |
| `ErrFDNotSupportedOnBus` | — (库内部) | 在非 FD Bus 上发 FD 帧 |
| `ErrIDOutOfRange` | — | `NewFrame` ID 超 11/29 位 |
| `ErrDataTooLong` | — | Classical >8 字节 |
| `ErrInvalidFDLength` | — | FD 长度不在合法离散集合 |
| `ErrNotInitialized` | `0x04000000` | 通道未初始化 / 已被释放 |
| `ErrIllHandle` | `0x01000000` | 非法句柄 |
| `ErrIllParamValue` | `0x00008000` | 参数值非法（如 FD 位率字符串错） |
| `ErrIllOperation` | `0x00080000` | 平台/驱动不支持（如 Linux 调 Open） |
| `ErrNoDriver` | `0x02000000` | DLL 未加载 |
| `ErrQueueEmpty` | `0x00000020` | **TryRead** 返回，正常情况不算错误 |
| `ErrQueueOverrun` | `0x00000040` | 上层读取过慢，PCAN 接收队列被覆盖 |
| `ErrQueueXmtFull` | `0x00000080` | 发送队列满 |
| `ErrBusLight` | `0x00000002` | 错误计数升高 |
| `ErrBusHeavy` | `0x00000004` | 错误计数继续升 |
| `ErrBusPassive` | `0x00040000` | 进入 passive 状态 |
| `ErrBusOff` | `0x00000010` | 总线断开（需 Reset） |

完整列表见 `errors.go`。

## SendMany 错误

批量发送失败返回 `*SendManyError`：

```go
err := bus.SendMany(ctx, frames)
var sme *pcanbasic.SendManyError
if errors.As(err, &sme) {
    log.Printf("第 %d 帧失败: id=0x%X err=%v",
        sme.Index, sme.Frame.ID, sme.Err)
}
// errors.Is 会穿透到 Unwrap 出的内部错误：
if errors.Is(err, pcanbasic.ErrBusOff) { /* ... */ }
```

注意：已成功发送的帧不会回滚——CAN 总线无事务概念。

## BUSOFF 恢复模式

```go
for {
    select {
    case e := <-bus.Errors():
        if errors.Is(e, pcanbasic.ErrBusOff) {
            log.Println("BUSOFF, resetting")
            if err := bus.Reset(); err != nil {
                log.Printf("reset failed: %v", err)
            }
            // Reset 之后通常还要重放业务握手帧
        }
    case fr := <-bus.Receive():
        // ... 正常处理
    }
}
```

## 异步错误 vs 调用错误

- **调用错误**：`Send`/`Status`/`Reset` 等的返回值，与具体调用 1:1。
- **异步错误**：`Errors()` channel，来自 reader goroutine——多为 BUSOFF / QOVERRUN 这类总线侧错误。

`QRCVEMPTY` 是"队列空"，不算错误，永远不会出现在 `Errors()`；只有 `TryRead` 会把它作为 `ErrQueueEmpty` 返回。

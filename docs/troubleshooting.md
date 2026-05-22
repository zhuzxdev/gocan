# 故障排查

## DLL 找不到

**症状**：`Open` 返回 `pcanbasic: PCANBasic.dll not found or failed to load`，或调用任何 API 返回 `ErrNoDriver`。

**原因 + 修复**：

1. DLL 不在搜索路径。最稳的做法：

   ```cmd
   set PCANBASIC_DLL_PATH=C:\Program Files\PEAK-System\PCAN-Basic API\x64\PCANBasic.dll
   ```

2. 32/64 位不匹配（错误 `0xC000007B`）。`go env GOARCH` 必须和 DLL 架构一致。

3. 没装 PCAN 驱动。先装 <https://www.peak-system.com/quick/DrvSetup>。

## `0x00008000` ILLPARAMVAL

通常是 `OpenFD` 的 `fdBitrate` 字符串语法错误或值组合不被硬件支持。

- 检查字段名拼写：`nom_brp` `nom_tseg1` `nom_tseg2` `nom_sjw` `data_brp` `data_tseg1` `data_tseg2` `data_sjw`
- 时钟必须是硬件支持值（80MHz / 60MHz / 40MHz / 20MHz）
- 仲裁段位率必须 ≤ 数据段位率
- 实在解不出来时，参考硬件手册推荐组合（见 `docs/can-fd.md`）

## `0x04000000` INITIALIZE

通道已被占用，或上次没干净 `Close`。

- 重启程序通常能恢复（Windows 在进程退出时强制释放句柄）
- 检查是不是别的进程（PCAN-View、另一个测试程序）也占着这条 USB

## BUSOFF（`0x00000010`）

总线进入 off 状态，需要主动 `Reset`：

```go
if errors.Is(err, pcanbasic.ErrBusOff) {
    _ = bus.Reset()
    // 真实工程里还要重发握手帧、重设过滤器
}
```

常见诱因：

- 接线错（CAN-H/CAN-L 反、缺端电阻）
- 总线上没有其他节点 ACK（单节点单元测试时容易遇到）
- 位率不匹配

## 接收一直没数据

按概率从高到低排查：

1. **位率不对**：发送端 `Baud500K` 但本端 `Baud1M`，看似在跑但永远收不到。
2. **过滤器没复位**：调过 `SetFilter` 之后忘了 `ResetFilter`，新 ID 被滤掉。
3. **Polling 间隔太长**：默认 `1ms` 已经很激进，但你显式设大了。
4. **接收端走 Event 模式但驱动版本太老不支持**：用 `WithLogger` 注入 logger，库会打 `event mode unavailable, falling back to polling: ...`。

## `QXMTFULL`（`0x00000080`）发送队列满

发太快，PCAN 内部队列满了。

```go
for {
    err := bus.Send(ctx, fr)
    if errors.Is(err, pcanbasic.ErrQueueXmtFull) {
        time.Sleep(time.Millisecond)
        continue
    }
    break
}
```

也可以加大发送间隔，或检查接收方是不是没在收（CAN 帧需要被 ACK 才会从发送队列离开）。

## Close 卡住

如果发现 `Close()` 不返回：

- Event 模式下：检查 `abort` 事件是否成功 `SetEvent`（极少见，通常是 PCAN 驱动 bug）
- Polling 模式下：reader 在 `select { rxCh <- f ... <-closing }` 上等，正常应该被 `closing` 唤醒 —— 如果没唤醒可能是 `closing` channel 被复用了（库内部不会，但用户拿到 *Bus 后乱改字段就会）

## FAQ

**Q：能不能不用 cgo？**
A：本库就是不用 cgo——通过 `golang.org/x/sys/windows` 的 syscall 直接调 DLL 导出函数。完全纯 Go。

**Q：Linux 上为什么 Open 失败？**
A：本库专为 Windows + PCAN-USB 设计。Linux 用 SocketCAN（`github.com/brutella/can` 等）。详见 `docs/platform-support.md`。

**Q：能多个 goroutine 同时 Send 吗？**
A：可以。`Send` 直接调底层 `CAN_Write`，PCAN 驱动内部串行化。

**Q：可以热插拔吗？**
A：不行。PCAN-USB 拔掉后下一次 `Send/Read` 会报 `ILLHWHANDLE` 或类似错误，需要 `Close()` 重新 `Open`。

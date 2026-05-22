# 快速开始

3 步跑通第一帧 CAN。

## 1. 安装 PCAN 驱动

下载并安装 PEAK-System 官方驱动：
<https://www.peak-system.com/quick/DrvSetup>

安装后 Windows 设备管理器里应能看到 `PCAN-USB` 设备节点。

## 2. 放置 PCANBasic.dll

PCAN 驱动安装包自带 `PCANBasic.dll`（通常位于 `C:\Program Files\PEAK-System\PCAN-Basic API\x64\PCANBasic.dll`）。

程序加载顺序遵循 Windows 标准 DLL 搜索：

1. 与可执行文件同目录
2. `PATH` 中
3. 环境变量 `PCANBASIC_DLL_PATH` 指向的绝对路径（库特有，最高优先级）

最简单的做法：把 `PCANBasic.dll` 复制到可执行文件旁边。

## 3. 跑示例

```bash
git clone https://github.com/Crush251/pcanbasic_go
cd pcanbasic_go
go run ./examples/01_send_classical -channel=USBBus1
```

预期输出：

```
sent: id=0x123 data=01020304
```

## 最小代码片段

```go
package main

import (
    "context"
    "log"

    "github.com/Crush251/pcanbasic_go"
)

func main() {
    bus, err := pcanbasic.Open(pcanbasic.USBBus1,
        pcanbasic.WithBitrate(pcanbasic.Baud1M))
    if err != nil { log.Fatal(err) }
    defer bus.Close()

    f, _ := pcanbasic.NewFrame(0x123, []byte{1, 2, 3})
    if err := bus.Send(context.Background(), f); err != nil {
        log.Fatal(err)
    }
}
```

## 接收一帧

```go
fr, err := bus.ReadOne(ctx)   // 阻塞直到 ctx 取消或 Bus 关闭
if err != nil { /* ... */ }
log.Printf("rx id=0x%X data=%X", fr.ID, fr.Data)
```

更多模式（流式 `Receive()`、非阻塞 `TryRead()`、Event 模式等）见 `examples/`。

## 下一步

- 多通道：`examples/06_multi_channel`
- CAN FD：`docs/can-fd.md`
- 故障恢复：`examples/08_status_and_reset`
- 架构与并发模型：`docs/architecture.md`

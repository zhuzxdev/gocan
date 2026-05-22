# CAN FD

## 打开 FD 通道

```go
const fdBitrate = "f_clock=80000000, " +
    "nom_brp=2, nom_tseg1=63, nom_tseg2=16, nom_sjw=16, " +
    "data_brp=2, data_tseg1=15, data_tseg2=4, data_sjw=4"

bus, err := pcanbasic.OpenFD(pcanbasic.USBBus1, fdBitrate)
```

> 与 Classical 不同，FD 不接受 `WithBitrate`——波特率全部由这个字符串决定。
> `WithBitrate` 对 `OpenFD` 是 no-op。

## fdBitrate 字符串

格式由 PEAK 官方定义，键值对用逗号分隔：

| 键 | 含义 |
|---|---|
| `f_clock` | CAN 控制器时钟（Hz），常见 80MHz、60MHz、40MHz、20MHz |
| `nom_brp` / `nom_tseg1` / `nom_tseg2` / `nom_sjw` | 仲裁段（位率 + 采样点） |
| `data_brp` / `data_tseg1` / `data_tseg2` / `data_sjw` | 数据段（高速段） |

具体推荐值见你的硬件手册（PCAN-USB Pro FD、PCAN-USB FD 等）。

## 帧构造

```go
fr, err := pcanbasic.NewFDFrame(
    id,
    data,       // 长度必须是合法 FD DLC：0..8, 12, 16, 20, 24, 32, 48, 64
    true,       // BRS：数据段切换到高速
    false,      // ESI：仅接收方关心，发送时通常填 false
)
```

非法长度（如 9、10）会返回 `ErrInvalidFDLength`。

## DLC 编码表

PCAN 底层用 4-bit 的 DLC 值表示 0..15，其中 9..15 不再代表字节数，而是离散映射：

| DLC | bytes |
|---|---|
| 0..8 | 0..8 |
| 9 | 12 |
| 10 | 16 |
| 11 | 20 |
| 12 | 24 |
| 13 | 32 |
| 14 | 48 |
| 15 | 64 |

库内部自动转换；用户层只关心 `len(Data)`。

## Classical 帧上 FD Bus

允许：Classical Bus 不能发 FD 帧（`ErrFDNotSupportedOnBus`），但 FD Bus 可以发普通 Classical 帧——库自动走 `CAN_Write` 而不是 `CAN_WriteFD`。

```go
busFD, _ := pcanbasic.OpenFD(ch, fdBitrate)
classical, _ := pcanbasic.NewFrame(0x100, []byte{1, 2})
busFD.Send(ctx, classical)   // ✓ 允许
```

## 标志位

| 标志 | Classical | FD | 含义 |
|---|---|---|---|
| `FlagExtended` | ✓ | ✓ | 29 位 ID |
| `FlagRemote` | ✓ | ✗ | RTR 远程帧（FD 协议无 RTR） |
| `FlagFD` | ✗ | ✓ | FD 帧（库内部由 `NewFDFrame` 自动加） |
| `FlagBRS` | ✗ | ✓ | 数据段位率切换 |
| `FlagESI` | ✗ | ✓ | 发送节点处于 error passive |

## 接收

接收逻辑透明：`Receive() / ReadOne()` 拿到 `Frame` 后，用 `fr.Has(FlagFD)` / `fr.Has(FlagBRS)` 判断即可。

```go
fr, _ := bus.ReadOne(ctx)
if fr.Has(pcanbasic.FlagFD) {
    // 64 字节大帧也走同一个 Frame.Data
}
```

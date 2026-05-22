# 平台支持

## 支持矩阵

| 平台 | 架构 | 真机 | 编译 | 单元测试 |
|---|---|---|---|---|
| Windows | amd64 | ✓ | ✓ | ✓ |
| Windows | 386 | ✓（驱动支持） | ✓ | ✓ |
| Linux | any | ✗ | ✓ | ✓（fake adapter） |
| macOS | any | ✗ | ✓ | ✓（fake adapter） |

> "真机"意味着 `Open()` 能成功返回可工作的 `Bus`；非 Windows 上 `Open()` 会返回
> `ErrIllOperation`（底层 `PCAN_ERROR_ILLOPERATION`），但编译和测试都能跑。

## DLL 加载策略

Windows 上库通过 `golang.org/x/sys/windows.LazyDLL` 加载 `PCANBasic.dll`。
加载顺序：

1. 环境变量 `PCANBASIC_DLL_PATH` 指向的路径（绝对或相对）
2. 默认值 `"PCANBasic.dll"`，按 Windows 标准 DLL 搜索路径解析（exe 同目录 → System32 → PATH）

```bash
# 自定义 DLL 位置
set PCANBASIC_DLL_PATH=C:\PEAK\PCAN-Basic API\x64\PCANBasic.dll
your-app.exe
```

加载失败时所有后续调用返回 `PCAN_ERROR_NODRIVER`（对应 `ErrNoDriver`）。
`sync.Once` 保证同一进程内只尝试加载一次。

## 32 位 vs 64 位

PCAN-USB 驱动同时提供 x86 和 x64 两份 DLL，路径分别是：

- `C:\Program Files\PEAK-System\PCAN-Basic API\x86\PCANBasic.dll`
- `C:\Program Files\PEAK-System\PCAN-Basic API\x64\PCANBasic.dll`

**Go 程序的位数必须和 DLL 一致**（`GOARCH=amd64` → x64 DLL）。否则加载时报 `0xC000007B`。

## 非 Windows 行为

Linux/macOS 上：

- `raw.EnsureLoaded()` 直接返回 `nil`（无事可做）
- `raw.Initialize / InitializeFD / Read / Write / ...` 全部返回 `PCAN_ERROR_ILLOPERATION`
- 顶层 `Open / OpenFD` 因此返回 `*Error{Code: PCAN_ERROR_ILLOPERATION}`，即 `errors.Is(err, ErrIllOperation)` 为 true
- 单元测试通过 fake adapter 注入：跑 `go test ./...` 在任何平台都能通过

这让 CI 可以在 Linux 上跑（便宜、并行），而真实硬件验证在 Windows runner 上跑。

## SocketCAN？

本库**不**支持 Linux SocketCAN。如果你的目标平台是 Linux，请用 `github.com/brutella/can` 或类似的 SocketCAN Go 库。
本库只为 Windows + PCAN-USB 这一组合存在，正是为了填补 Linux 已经有 SocketCAN、而 Windows 上长期只有 Python 的空白。

# pcanbasic_go

> PEAK-System **PCANBasic.dll** 的 Go 语言封装库（Windows 专用）。

[![Go Reference](https://pkg.go.dev/badge/github.com/Crush251/pcanbasic_go.svg)](https://pkg.go.dev/github.com/Crush251/pcanbasic_go)
[![CI](https://github.com/Crush251/pcanbasic_go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Crush251/pcanbasic_go/actions/workflows/ci.yml)
[![CodeQL](https://github.com/Crush251/pcanbasic_go/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/Crush251/pcanbasic_go/actions/workflows/codeql.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Crush251/pcanbasic_go)](https://goreportcard.com/report/github.com/Crush251/pcanbasic_go)
[![codecov](https://codecov.io/gh/Crush251/pcanbasic_go/branch/main/graph/badge.svg)](https://codecov.io/gh/Crush251/pcanbasic_go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`pcanbasic_go` 在 Windows 平台用纯 Go（`syscall` 调用，无 CGO）封装 PEAK-System 的
`PCANBasic.dll`，让 Go 程序能直接收发 CAN / CAN FD 报文，无须再依赖 Python 或 C++ 中间层。

> ⚠️ **当前为预发布阶段**（`v0.x`），API 仍可能调整。本仓库目前处于**初始化阶段**，
> 完整设计见
> [docs/superpowers/specs/2026-05-22-pcanbasic-go-design.md](docs/superpowers/specs/2026-05-22-pcanbasic-go-design.md)。

---

## 为什么做这个

- **Linux** 端可以用 [`socketcan`](https://github.com/brutella/can) 等成熟 Go 库直连 CAN
- **Windows** 端 PEAK 官方仅提供 C/C++、C#、Java、Python 等绑定，**没有 Go 封装**
- 现有的 Python 桥接（`can-bridge-win`）需要打包 `PyInstaller`、引入 `python-can` 依赖，
  在嵌入到机器人控制等纯 Go 项目时既笨重又难以追踪问题

`pcanbasic_go` 旨在补上这块空缺，作为未来跨平台 CAN 抽象层（Linux 走 socketcan、
Windows 走 PCANBasic）的 Windows 后端。

---

## 特性（v0.1 计划）

- ✅ Classical CAN 与 CAN FD 双标准支持
- ✅ 高层 `Bus` API：`Send` / `SendMany` / `Receive` / `ReadOne` / `TryRead` / `Status` / `Reset` / `SetFilter`
- ✅ 三种接收模式：`ModeAuto` / `ModePolling` / `ModeEvent`（Windows Event 驱动）
- ✅ 子包 `raw`：与 PCANBasic C API 1:1 对应的低层绑定
- ✅ 错误处理：位掩码语义 + `errors.Is` 哨兵
- ✅ 非 Windows 平台编译桩，便于 Linux/macOS 上做 lint / vet / 单元测试
- ✅ 完整的中文文档与 10 个示例

详细范围见 [设计文档](docs/superpowers/specs/2026-05-22-pcanbasic-go-design.md)。

---

## 快速开始（计划交付时）

```go
package main

import (
    "context"
    "log"

    "github.com/Crush251/pcanbasic_go"
)

func main() {
    bus, err := pcanbasic.Open(
        pcanbasic.USBBus1,
        pcanbasic.WithBitrate(pcanbasic.Baud1M),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer bus.Close()

    frame, _ := pcanbasic.NewFrame(0x123, []byte{0x01, 0x02, 0x03, 0x04})
    if err := bus.Send(context.Background(), frame); err != nil {
        log.Fatal(err)
    }
}
```

---

## 系统要求

- Windows（v0.1 真机仅支持 Windows；非 Windows 平台仅可编译，无法实际通信）
- Go 1.22+
- 已安装 PEAK PCAN 驱动（[官方下载](https://www.peak-system.com/Drivers.523.0.html)）
- `PCANBasic.dll` 与 Go 程序架构匹配（amd64 → 64 位 DLL；386 → 32 位 DLL）

---

## 路线图

| 版本 | 主要内容 |
|---|---|
| v0.1.0 | Classical + FD 收发、Bus 完整高层、raw 子包基础 API、文档与示例（Windows 后端） |
| v0.2.0 | LookUpChannel、设备信息查询、Trace；**Linux 后端走 socketcan**（参考 [python-can](https://python-can.readthedocs.io/) 的多后端抽象），同一套 `Bus` API、按平台编译期切换后端 |
| v1.0.0 | API 冻结，进入严格兼容承诺 |

---

## 许可证

[MIT](LICENSE)

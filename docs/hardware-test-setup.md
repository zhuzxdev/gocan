# 硬件测试环境搭建

本库的单元测试用 fake adapter，所有平台都能跑。但真机验证只能在 Windows + 真实 PCAN 设备上做——这篇文档说明怎么搭这套环境。

## 硬件

最便宜的入门组合：

- **两根 PCAN-USB**（PEAK-System 官方），一根发一根收
- **CAN 标准 9-pin D-Sub 双绞对接线**（或自己用杜邦线接 CAN-H/CAN-L + 120Ω 端电阻）

更便宜方案：一根 PCAN-USB + 任意能发帧的 CAN 节点（ECU、Arduino + MCP2515 等）。

## 软件

### 1. 安装 PEAK 驱动

<https://www.peak-system.com/quick/DrvSetup>

安装完成后：

- 设备管理器里能看到 "PCAN-USB"（或型号名）
- `C:\Program Files\PEAK-System\PCAN-Basic API\` 下能找到 DLL

### 2. 准备 DLL

把目标架构（x64 通常）的 `PCANBasic.dll` 复制到测试可执行文件所在目录，或设环境变量：

```cmd
set PCANBASIC_DLL_PATH=C:\Program Files\PEAK-System\PCAN-Basic API\x64\PCANBasic.dll
```

### 3. 跑示例验证

```cmd
go run ./examples/01_send_classical -channel=USBBus1
go run ./examples/02_receive_polling -channel=USBBus2
```

两根 USB 互通的话，第二条命令应能看到第一条发出的 0x123 帧。

## 跑带硬件标签的测试

为了在 CI 上跳过真机测试，建议把所有依赖真硬件的测试加 build tag：

```go
//go:build pcanhardware

package pcanbasic_test
```

本地运行：

```cmd
go test -tags=pcanhardware ./...
```

CI 上不加这个 tag，只跑 fake adapter 的测试。

## 常见环境问题

- **"PCANBasic.dll not found"**：DLL 不在搜索路径里。最稳妥的做法是绝对路径设 `PCANBASIC_DLL_PATH`。
- **`0xC000007B` 加载失败**：32/64 位不匹配。确认 `GOARCH` 与 DLL 架构一致。
- **`PCAN_ERROR_INITIALIZE (0x04000000)`**：通道已被其他进程占用，或本进程上一次没干净 `Close`。重启程序通常即可。
- **真实总线没有 ACK**：单节点 + 没接收方时发送会失败。最低只需在两端各加 120Ω 端电阻并接好 CAN-H/CAN-L。

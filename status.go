package pcanbasic

import "github.com/Crush251/pcanbasic_go/raw"

// Status 是 PCAN 通道当前状态的位掩码值。
//
// 直接 alias raw.TPCANStatus 以保证与官方常量值一致；
// 与 *Error.Code 是同一底层类型，便于互操作。
type Status = raw.TPCANStatus

// 通道状态常量。注意这些是位掩码，可以多个同时置位（如 BUSOFF|QOVERRUN）。
const (
	StatusOK           Status = raw.PCAN_ERROR_OK
	StatusBusLight     Status = raw.PCAN_ERROR_BUSLIGHT
	StatusBusHeavy     Status = raw.PCAN_ERROR_BUSHEAVY
	StatusBusPassive   Status = raw.PCAN_ERROR_BUSPASSIVE
	StatusBusOff       Status = raw.PCAN_ERROR_BUSOFF
	StatusQueueOverrun Status = raw.PCAN_ERROR_QOVERRUN
)

// StatusHas 判断 status 中是否包含指定的位。
//
// 特别处理 StatusOK (0)：仅当 status 也是 0 时才算"包含"，
// 否则按位掩码 AND 判断。
func StatusHas(status, bit Status) bool {
	if bit == StatusOK {
		return status == StatusOK
	}
	return status&bit == bit
}

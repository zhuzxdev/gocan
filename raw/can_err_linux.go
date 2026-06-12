//go:build linux

package raw

import "golang.org/x/sys/unix"

// CAN 错误帧位掩码：用于 CAN_RAW_ERR_FILTER。完整定义参见 linux/can/error.h。
const (
	CANErrTxTimeout uint32 = unix.CAN_ERR_TX_TIMEOUT
	CANErrLostArb   uint32 = unix.CAN_ERR_LOSTARB
	CANErrCrtl      uint32 = unix.CAN_ERR_CRTL
	CANErrProt      uint32 = unix.CAN_ERR_PROT
	CANErrTrx       uint32 = unix.CAN_ERR_TRX
	CANErrAck       uint32 = unix.CAN_ERR_ACK
	CANErrBusOff    uint32 = unix.CAN_ERR_BUSOFF
	CANErrBusError  uint32 = unix.CAN_ERR_BUSERROR
	CANErrRestarted uint32 = unix.CAN_ERR_RESTARTED
	CANErrMaskAll   uint32 = unix.CAN_ERR_MASK
)

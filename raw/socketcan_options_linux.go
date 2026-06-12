//go:build linux

package raw

import (
	"time"

	"golang.org/x/sys/unix"
)

// SetCANRawSockoptInt 把一个 int 值写到 SOL_CAN_RAW 层的指定选项。
// 仅用于 0/1 开关：CAN_RAW_LOOPBACK / CAN_RAW_RECV_OWN_MSGS / CAN_RAW_FD_FRAMES。
// 调用方负责传入合法 opt：错用 CAN_RAW_FILTER / ERR_FILTER / JOIN_FILTERS 会绕过
// linuxChannel 的状态簿记，导致后续状态查询与内核真实状态不一致。
func SetCANRawSockoptInt(ch TPCANHandle, opt int, value int) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if err := unix.SetsockoptInt(c.fd, unix.SOL_CAN_RAW, opt, value); err != nil {
		return errnoToStatus(err)
	}
	return PCAN_ERROR_OK
}

// SetCANRawErrFilter 写 CAN_RAW_ERR_FILTER。
func SetCANRawErrFilter(ch TPCANHandle, mask uint32) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if err := unix.SetsockoptInt(c.fd, unix.SOL_CAN_RAW, unix.CAN_RAW_ERR_FILTER, int(mask)); err != nil {
		return errnoToStatus(err)
	}
	updateLinuxChannel(ch, c, func(c *linuxChannel) {
		c.errFilter = mask
		c.errFilterSet = true
	})
	return PCAN_ERROR_OK
}

// SetCANRawJoinFilters 写 CAN_RAW_JOIN_FILTERS（true=AND，false=OR）。
// 内核 < 4.1 不支持，setsockopt 返回 ENOPROTOOPT；调用方应识别该状态。
func SetCANRawJoinFilters(ch TPCANHandle, and bool) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	v := 0
	if and {
		v = 1
	}
	if err := unix.SetsockoptInt(c.fd, unix.SOL_CAN_RAW, unix.CAN_RAW_JOIN_FILTERS, v); err != nil {
		return errnoToStatus(err)
	}
	updateLinuxChannel(ch, c, func(c *linuxChannel) {
		c.joinFilters = and
		c.joinFiltersSet = true
	})
	return PCAN_ERROR_OK
}

// SetSocketBuffers 写 SO_RCVBUF / SO_SNDBUF（任一非正值则跳过该方向）。
func SetSocketBuffers(ch TPCANHandle, rcv, snd int) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if rcv > 0 {
		if err := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_RCVBUF, rcv); err != nil {
			return errnoToStatus(err)
		}
	}
	if snd > 0 {
		if err := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_SNDBUF, snd); err != nil {
			return errnoToStatus(err)
		}
	}
	return PCAN_ERROR_OK
}

// SetReadWriteTimeout 写 SO_RCVTIMEO / SO_SNDTIMEO。零值表示不设置。
func SetReadWriteTimeout(ch TPCANHandle, read, write time.Duration) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if read > 0 {
		tv := unix.NsecToTimeval(read.Nanoseconds())
		if err := unix.SetsockoptTimeval(c.fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
			return errnoToStatus(err)
		}
	}
	if write > 0 {
		tv := unix.NsecToTimeval(write.Nanoseconds())
		if err := unix.SetsockoptTimeval(c.fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO, &tv); err != nil {
			return errnoToStatus(err)
		}
	}
	return PCAN_ERROR_OK
}

// EnableRxTimestamp 启用内核时间戳（mode 1=SO_TIMESTAMP, 2=SO_TIMESTAMPNS, 3=SO_TIMESTAMPING(HW)）。
// linuxChannel 持久化 mode，read 路径据此决定走 read 还是 recvmsg。
// 当 mode=3 但内核或硬件不支持时，按 spec 静默降级到 NS（mode=2），
// 持久化的 rxTimestampMode 会反映实际生效的模式，调用方可读取以判断是否降级。
func EnableRxTimestamp(ch TPCANHandle, mode uint8) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	switch mode {
	case 1:
		if err := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_TIMESTAMP, 1); err != nil {
			return errnoToStatus(err)
		}
	case 2:
		if err := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_TIMESTAMPNS, 1); err != nil {
			return errnoToStatus(err)
		}
	case 3:
		flags := unix.SOF_TIMESTAMPING_RX_HARDWARE | unix.SOF_TIMESTAMPING_RAW_HARDWARE | unix.SOF_TIMESTAMPING_SOFTWARE | unix.SOF_TIMESTAMPING_RX_SOFTWARE
		if err := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_TIMESTAMPING, flags); err != nil {
			// 硬件时间戳失败 → 降级到 NS
			if e := unix.SetsockoptInt(c.fd, unix.SOL_SOCKET, unix.SO_TIMESTAMPNS, 1); e != nil {
				return errnoToStatus(e)
			}
			updateLinuxChannel(ch, c, func(c *linuxChannel) {
				c.rxTimestampMode = 2
			})
			return PCAN_ERROR_OK
		}
	default:
		return PCAN_ERROR_ILLPARAMVAL
	}
	updateLinuxChannel(ch, c, func(c *linuxChannel) {
		c.rxTimestampMode = mode
	})
	return PCAN_ERROR_OK
}

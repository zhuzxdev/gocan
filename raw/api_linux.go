//go:build linux

package raw

import (
	"errors"
	"net"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	linuxCANFrameSize   = 16
	linuxCANFDFrameSize = 72

	linuxCANFDBRS = 0x01
	linuxCANFDESI = 0x02
)

var linuxChannels = struct {
	mu sync.Mutex
	m  map[TPCANHandle]*linuxChannel
}{m: make(map[TPCANHandle]*linuxChannel)}

type linuxChannel struct {
	fd              int
	isFD            bool
	filters         []unix.CanFilter
	errFilter       uint32
	errFilterSet    bool
	joinFilters     bool
	joinFiltersSet  bool
	rxTimestampMode uint8 // 0=off, 1=SO_TIMESTAMP, 2=SO_TIMESTAMPNS, 3=SO_TIMESTAMPING
}

// EnsureLoaded 在 Linux SocketCAN 后端无需加载动态库。
func EnsureLoaded() error { return nil }

// Initialize 打开 Linux SocketCAN Classical CAN 通道。
func Initialize(ch TPCANHandle, br TPCANBaudrate) TPCANStatus {
	return initializeSocketCAN(ch, false)
}

// InitializeFD 打开 Linux SocketCAN CAN FD 通道。
func InitializeFD(ch TPCANHandle, bitrateFD string) TPCANStatus {
	return initializeSocketCAN(ch, true)
}

func initializeSocketCAN(ch TPCANHandle, fd bool) TPCANStatus {
	iface, ok := socketCANInterface(ch)
	if !ok {
		return PCAN_ERROR_ILLOPERATION
	}
	if ch == PCAN_NONEBUS || iface == "" {
		return PCAN_ERROR_ILLPARAMVAL
	}

	linuxChannels.mu.Lock()
	if _, exists := linuxChannels.m[ch]; exists {
		linuxChannels.mu.Unlock()
		return PCAN_ERROR_HWINUSE
	}
	linuxChannels.mu.Unlock()

	nic, err := net.InterfaceByName(iface)
	if err != nil {
		return PCAN_ERROR_ILLPARAMVAL
	}

	sock, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW|unix.SOCK_NONBLOCK, unix.CAN_RAW)
	if err != nil {
		return errnoToStatus(err)
	}
	if fd {
		if err := unix.SetsockoptInt(sock, unix.SOL_CAN_RAW, unix.CAN_RAW_FD_FRAMES, 1); err != nil {
			_ = unix.Close(sock)
			return errnoToStatus(err)
		}
	}
	if err := unix.Bind(sock, &unix.SockaddrCAN{Ifindex: nic.Index}); err != nil {
		_ = unix.Close(sock)
		return errnoToStatus(err)
	}

	linuxChannels.mu.Lock()
	defer linuxChannels.mu.Unlock()
	if _, exists := linuxChannels.m[ch]; exists {
		_ = unix.Close(sock)
		return PCAN_ERROR_HWINUSE
	}
	linuxChannels.m[ch] = &linuxChannel{fd: sock, isFD: fd}
	return PCAN_ERROR_OK
}

// Uninitialize 关闭 Linux SocketCAN socket。
func Uninitialize(ch TPCANHandle) TPCANStatus {
	c, status := takeLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if err := unix.Close(c.fd); err != nil {
		return errnoToStatus(err)
	}
	return PCAN_ERROR_OK
}

func takeLinuxChannel(ch TPCANHandle) (*linuxChannel, TPCANStatus) {
	linuxChannels.mu.Lock()
	defer linuxChannels.mu.Unlock()

	c, ok := linuxChannels.m[ch]
	if !ok {
		return nil, PCAN_ERROR_INITIALIZE
	}
	delete(linuxChannels.m, ch)
	return c, PCAN_ERROR_OK
}

func getLinuxChannel(ch TPCANHandle) (*linuxChannel, TPCANStatus) {
	linuxChannels.mu.Lock()
	defer linuxChannels.mu.Unlock()

	c, ok := linuxChannels.m[ch]
	if !ok {
		return nil, PCAN_ERROR_INITIALIZE
	}
	return c, PCAN_ERROR_OK
}

// updateLinuxChannel 在 setsockopt 成功后安全地把状态写回 linuxChannel。
// 若期间通道被 Uninitialize/Initialize 替换，写入会被跳过（current != c）。
func updateLinuxChannel(ch TPCANHandle, c *linuxChannel, mut func(*linuxChannel)) {
	linuxChannels.mu.Lock()
	if current, ok := linuxChannels.m[ch]; ok && current == c {
		mut(current)
	}
	linuxChannels.mu.Unlock()
}

// Reset 在 SocketCAN 后端无内部队列可重置，保留为成功的空操作。
func Reset(ch TPCANHandle) TPCANStatus {
	_, status := getLinuxChannel(ch)
	return status
}

// GetStatus 返回 Linux SocketCAN 通道状态。
func GetStatus(ch TPCANHandle) TPCANStatus {
	_, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	return PCAN_ERROR_OK
}

// Read 读取一帧 Classical CAN 报文。
// 当 c.rxTimestampMode != 0 时切换到 recvmsg 路径，从 SCM 控制消息提取内核时间戳；
// 否则保持 unix.Read + fillTimestamp 行为。注意：rxTimestampMode 仅在 Open 阶段
// （applyPlatformOptions → EnableRxTimestamp）写入一次，之后由 reader goroutine
// 只读访问，无并发写入风险。
func Read(ch TPCANHandle, m *TPCANMsg, t *TPCANTimestamp) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if c.rxTimestampMode != 0 {
		return readClassicalWithTimestamp(c, m, t)
	}
	var buf [linuxCANFDFrameSize]byte
	n, err := unix.Read(c.fd, buf[:])
	if err != nil {
		return errnoToReadStatus(err)
	}
	if n != linuxCANFrameSize {
		return PCAN_ERROR_ILLDATA
	}
	if status := decodeLinuxCANFrame(buf[:linuxCANFrameSize], m); status != PCAN_ERROR_OK {
		return status
	}
	if t != nil {
		fillTimestamp(t)
	}
	return PCAN_ERROR_OK
}

// ReadFD 读取一帧 CAN FD 报文。
// 与 Read 类似，c.rxTimestampMode != 0 时走 recvmsg 路径。
func ReadFD(ch TPCANHandle, m *TPCANMsgFD, t *TPCANTimestampFD) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if c.rxTimestampMode != 0 {
		return readFDWithTimestamp(c, m, t)
	}
	var buf [linuxCANFDFrameSize]byte
	n, err := unix.Read(c.fd, buf[:])
	if err != nil {
		return errnoToReadStatus(err)
	}
	switch n {
	case linuxCANFrameSize:
		var cm TPCANMsg
		if status := decodeLinuxCANFrame(buf[:linuxCANFrameSize], &cm); status != PCAN_ERROR_OK {
			return status
		}
		m.ID = cm.ID
		m.MsgType = cm.MsgType
		m.DLC = cm.Len
		copy(m.Data[:], cm.Data[:])
	case linuxCANFDFrameSize:
		if status := decodeLinuxCANFDFrame(buf[:], m); status != PCAN_ERROR_OK {
			return status
		}
	default:
		return PCAN_ERROR_ILLDATA
	}
	if t != nil {
		*t = uint64(time.Now().UnixNano() / int64(time.Microsecond))
	}
	return PCAN_ERROR_OK
}

// Write 发送一帧 Classical CAN 报文。
func Write(ch TPCANHandle, m *TPCANMsg) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	buf, status := encodeLinuxCANFrame(m)
	if status != PCAN_ERROR_OK {
		return status
	}
	return writeAll(c.fd, buf[:])
}

// WriteFD 发送一帧 CAN FD 报文。
func WriteFD(ch TPCANHandle, m *TPCANMsgFD) TPCANStatus {
	c, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	if !c.isFD {
		return PCAN_ERROR_ILLOPERATION
	}
	buf, status := encodeLinuxCANFDFrame(m)
	if status != PCAN_ERROR_OK {
		return status
	}
	return writeAll(c.fd, buf[:])
}

func writeAll(fd int, buf []byte) TPCANStatus {
	n, err := unix.Write(fd, buf)
	if err != nil {
		return errnoToWriteStatus(err)
	}
	if n != len(buf) {
		return PCAN_ERROR_QXMTFULL
	}
	return PCAN_ERROR_OK
}

// FilterMessages 为 Linux SocketCAN raw socket 追加一组内核过滤器。
func FilterMessages(ch TPCANHandle, fromID, toID uint32, mode TPCANMessageType) TPCANStatus {
	newFilters, status := socketCANFilters(fromID, toID, mode)
	if status != PCAN_ERROR_OK {
		return status
	}

	linuxChannels.mu.Lock()
	c, ok := linuxChannels.m[ch]
	if !ok {
		linuxChannels.mu.Unlock()
		return PCAN_ERROR_INITIALIZE
	}
	filters := append(append([]unix.CanFilter(nil), c.filters...), newFilters...)
	linuxChannels.mu.Unlock()

	if len(filters) > unix.CAN_RAW_FILTER_MAX {
		return PCAN_ERROR_RESOURCE
	}
	if err := unix.SetsockoptCanRawFilter(c.fd, unix.SOL_CAN_RAW, unix.CAN_RAW_FILTER, filters); err != nil {
		return errnoToStatus(err)
	}

	linuxChannels.mu.Lock()
	if current, ok := linuxChannels.m[ch]; ok && current == c {
		current.filters = filters
	}
	linuxChannels.mu.Unlock()
	return PCAN_ERROR_OK
}

// GetValue 当前 Linux SocketCAN 最小后端暂不支持 PCAN 参数查询。
func GetValue(ch TPCANHandle, p TPCANParameter, buf unsafe.Pointer, n uint32) TPCANStatus {
	_, status := getLinuxChannel(ch)
	if status != PCAN_ERROR_OK {
		return status
	}
	return PCAN_ERROR_ILLOPERATION
}

// SetValue 支持 Linux SocketCAN 后端需要的最小 PCAN 参数集合。
func SetValue(ch TPCANHandle, p TPCANParameter, buf unsafe.Pointer, n uint32) TPCANStatus {
	if p != PCAN_MESSAGE_FILTER {
		_, status := getLinuxChannel(ch)
		if status != PCAN_ERROR_OK {
			return status
		}
		return PCAN_ERROR_ILLOPERATION
	}
	if buf == nil || n < uint32(unsafe.Sizeof(uint32(0))) {
		return PCAN_ERROR_ILLPARAMVAL
	}
	value := *(*uint32)(buf)
	if TPCANParameter(value) != PCAN_FILTER_OPEN {
		return PCAN_ERROR_ILLOPERATION
	}

	linuxChannels.mu.Lock()
	c, ok := linuxChannels.m[ch]
	if !ok {
		linuxChannels.mu.Unlock()
		return PCAN_ERROR_INITIALIZE
	}
	linuxChannels.mu.Unlock()

	if err := unix.SetsockoptCanRawFilter(c.fd, unix.SOL_CAN_RAW, unix.CAN_RAW_FILTER, nil); err != nil {
		return errnoToStatus(err)
	}

	linuxChannels.mu.Lock()
	if current, ok := linuxChannels.m[ch]; ok && current == c {
		current.filters = nil
	}
	linuxChannels.mu.Unlock()
	return PCAN_ERROR_OK
}

// GetErrorText 返回 Linux SocketCAN 后端的错误描述。
func GetErrorText(code TPCANStatus, lang uint16) (string, TPCANStatus) {
	switch code {
	case PCAN_ERROR_OK:
		return "no error", PCAN_ERROR_OK
	case PCAN_ERROR_QRCVEMPTY:
		return "receive queue empty", PCAN_ERROR_OK
	case PCAN_ERROR_QXMTFULL:
		return "transmit queue full", PCAN_ERROR_OK
	case PCAN_ERROR_NODRIVER:
		return "SocketCAN is not available", PCAN_ERROR_OK
	case PCAN_ERROR_HWINUSE:
		return "SocketCAN channel is already open", PCAN_ERROR_OK
	case PCAN_ERROR_ILLPARAMVAL:
		return "invalid SocketCAN interface", PCAN_ERROR_OK
	case PCAN_ERROR_INITIALIZE:
		return "SocketCAN channel is not initialized", PCAN_ERROR_OK
	case PCAN_ERROR_ILLOPERATION:
		return "operation is not supported by the SocketCAN backend", PCAN_ERROR_OK
	case PCAN_ERROR_ILLDATA:
		return "invalid CAN frame data", PCAN_ERROR_OK
	case PCAN_ERROR_RESOURCE:
		return "SocketCAN filter limit exceeded", PCAN_ERROR_OK
	default:
		return "unknown SocketCAN error", PCAN_ERROR_OK
	}
}

func socketCANFilters(fromID, toID uint32, mode TPCANMessageType) ([]unix.CanFilter, TPCANStatus) {
	if fromID > toID {
		return nil, PCAN_ERROR_ILLPARAMVAL
	}
	idFlag := uint32(0)
	idMask := uint32(unix.CAN_SFF_MASK)
	limit := uint32(unix.CAN_SFF_MASK)
	if mode&PCAN_MESSAGE_EXTENDED != 0 {
		idFlag = unix.CAN_EFF_FLAG
		idMask = unix.CAN_EFF_MASK
		limit = unix.CAN_EFF_MASK
	}
	if fromID > limit || toID > limit {
		return nil, PCAN_ERROR_ILLPARAMVAL
	}

	filters := make([]unix.CanFilter, 0)
	for start := fromID; start <= toID; {
		block := largestAlignedFilterBlock(start, toID)
		filters = append(filters, unix.CanFilter{
			Id:   idFlag | start,
			Mask: unix.CAN_EFF_FLAG | unix.CAN_RTR_FLAG | idMask&^uint32(block-1),
		})
		if block > toID-start {
			break
		}
		start += block
	}
	return filters, PCAN_ERROR_OK
}

func largestAlignedFilterBlock(start, end uint32) uint32 {
	remaining := end - start + 1
	block := uint32(1)
	for block <= remaining/2 {
		next := block << 1
		if start&(next-1) != 0 {
			break
		}
		block = next
	}
	for block > remaining {
		block >>= 1
	}
	return block
}

func fillTimestamp(t *TPCANTimestamp) {
	micros := uint64(time.Now().UnixNano() / int64(time.Microsecond))
	t.Millis = uint32(micros / 1000)
	t.MillisOverflow = uint16((micros / 1000) >> 32)
	t.Micros = uint16(micros % 1000)
}

func encodeLinuxCANFrame(m *TPCANMsg) ([linuxCANFrameSize]byte, TPCANStatus) {
	var buf [linuxCANFrameSize]byte
	if m == nil || m.Len > 8 {
		return buf, PCAN_ERROR_ILLDATA
	}
	canID := linuxCANID(m.ID, m.MsgType)
	if canID == 0 && m.ID != 0 {
		return buf, PCAN_ERROR_ILLDATA
	}
	nativeEndian.PutUint32(buf[0:4], canID)
	buf[4] = m.Len
	copy(buf[8:], m.Data[:m.Len])
	return buf, PCAN_ERROR_OK
}

func decodeLinuxCANFrame(buf []byte, m *TPCANMsg) TPCANStatus {
	if len(buf) != linuxCANFrameSize || m == nil {
		return PCAN_ERROR_ILLDATA
	}
	canID := nativeEndian.Uint32(buf[0:4])
	length := buf[4]
	if length > 8 {
		return PCAN_ERROR_ILLDATA
	}
	id, mt := pcanID(canID)
	m.ID = id
	m.MsgType = mt
	m.Len = length
	m.Data = [8]byte{}
	copy(m.Data[:], buf[8:8+int(length)])
	return PCAN_ERROR_OK
}

func encodeLinuxCANFDFrame(m *TPCANMsgFD) ([linuxCANFDFrameSize]byte, TPCANStatus) {
	var buf [linuxCANFDFrameSize]byte
	if m == nil {
		return buf, PCAN_ERROR_ILLDATA
	}
	length := dlcToDataLen(m.DLC)
	if length > 64 {
		return buf, PCAN_ERROR_ILLDATA
	}
	canID := linuxCANID(m.ID, m.MsgType)
	if canID == 0 && m.ID != 0 {
		return buf, PCAN_ERROR_ILLDATA
	}
	nativeEndian.PutUint32(buf[0:4], canID)
	buf[4] = uint8(length)
	if m.MsgType&PCAN_MESSAGE_BRS != 0 {
		buf[5] |= linuxCANFDBRS
	}
	if m.MsgType&PCAN_MESSAGE_ESI != 0 {
		buf[5] |= linuxCANFDESI
	}
	copy(buf[8:], m.Data[:length])
	return buf, PCAN_ERROR_OK
}

func decodeLinuxCANFDFrame(buf []byte, m *TPCANMsgFD) TPCANStatus {
	if len(buf) != linuxCANFDFrameSize || m == nil {
		return PCAN_ERROR_ILLDATA
	}
	length := int(buf[4])
	if length > 64 {
		return PCAN_ERROR_ILLDATA
	}
	id, mt := pcanID(nativeEndian.Uint32(buf[0:4]))
	mt |= PCAN_MESSAGE_FD
	if buf[5]&linuxCANFDBRS != 0 {
		mt |= PCAN_MESSAGE_BRS
	}
	if buf[5]&linuxCANFDESI != 0 {
		mt |= PCAN_MESSAGE_ESI
	}
	m.ID = id
	m.MsgType = mt
	m.DLC = dataLenToDLC(length)
	m.Data = [64]byte{}
	copy(m.Data[:], buf[8:8+length])
	return PCAN_ERROR_OK
}

func linuxCANID(id uint32, mt TPCANMessageType) uint32 {
	if mt&PCAN_MESSAGE_EXTENDED != 0 {
		if id > unix.CAN_EFF_MASK {
			return 0
		}
		id |= unix.CAN_EFF_FLAG
	} else if id > unix.CAN_SFF_MASK {
		return 0
	}
	if mt&PCAN_MESSAGE_RTR != 0 {
		id |= unix.CAN_RTR_FLAG
	}
	return id
}

func pcanID(canID uint32) (uint32, TPCANMessageType) {
	var mt TPCANMessageType
	if canID&unix.CAN_RTR_FLAG != 0 {
		mt |= PCAN_MESSAGE_RTR
	}
	if canID&unix.CAN_EFF_FLAG != 0 {
		mt |= PCAN_MESSAGE_EXTENDED
		return canID & unix.CAN_EFF_MASK, mt
	}
	return canID & unix.CAN_SFF_MASK, mt
}

func dlcToDataLen(dlc uint8) int {
	switch dlc {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8:
		return int(dlc)
	case 9:
		return 12
	case 10:
		return 16
	case 11:
		return 20
	case 12:
		return 24
	case 13:
		return 32
	case 14:
		return 48
	case 15:
		return 64
	default:
		return 65
	}
}

func dataLenToDLC(n int) uint8 {
	switch n {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8:
		return uint8(n)
	case 12:
		return 9
	case 16:
		return 10
	case 20:
		return 11
	case 24:
		return 12
	case 32:
		return 13
	case 48:
		return 14
	case 64:
		return 15
	default:
		return 0
	}
}

func errnoToReadStatus(err error) TPCANStatus {
	if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
		return PCAN_ERROR_QRCVEMPTY
	}
	return errnoToStatus(err)
}

func errnoToWriteStatus(err error) TPCANStatus {
	if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.ENOBUFS) {
		return PCAN_ERROR_QXMTFULL
	}
	return errnoToStatus(err)
}

func errnoToStatus(err error) TPCANStatus {
	switch {
	case err == nil:
		return PCAN_ERROR_OK
	case errors.Is(err, unix.ENODEV), errors.Is(err, unix.ENXIO), errors.Is(err, unix.ENETDOWN):
		return PCAN_ERROR_ILLPARAMVAL
	case errors.Is(err, unix.EPERM), errors.Is(err, unix.EACCES), errors.Is(err, unix.EAFNOSUPPORT), errors.Is(err, unix.EPROTONOSUPPORT):
		return PCAN_ERROR_NODRIVER
	case errors.Is(err, unix.EINVAL):
		return PCAN_ERROR_ILLPARAMVAL
	default:
		return PCAN_ERROR_UNKNOWN
	}
}

type byteOrder interface {
	Uint32([]byte) uint32
	PutUint32([]byte, uint32)
}

var nativeEndian byteOrder = nativeByteOrder()

func nativeByteOrder() byteOrder {
	var x uint16 = 0x0102
	b := (*[2]byte)(unsafe.Pointer(&x))
	if b[0] == 0x02 {
		return littleEndian{}
	}
	return bigEndian{}
}

type littleEndian struct{}

func (littleEndian) Uint32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func (littleEndian) PutUint32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

type bigEndian struct{}

func (bigEndian) Uint32(b []byte) uint32 {
	return uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
}

func (bigEndian) PutUint32(b []byte, v uint32) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}

// readClassicalWithTimestamp 用 recvmsg 取一帧 Classical CAN 报文 + 内核时间戳。
func readClassicalWithTimestamp(c *linuxChannel, m *TPCANMsg, t *TPCANTimestamp) TPCANStatus {
	var buf [linuxCANFrameSize]byte
	var oob [128]byte
	n, oobn, _, _, err := unix.Recvmsg(c.fd, buf[:], oob[:], 0)
	if err != nil {
		return errnoToReadStatus(err)
	}
	if n != linuxCANFrameSize {
		return PCAN_ERROR_ILLDATA
	}
	if status := decodeLinuxCANFrame(buf[:linuxCANFrameSize], m); status != PCAN_ERROR_OK {
		return status
	}
	if t != nil {
		writeKernelTimestamp(oob[:oobn], t)
	}
	return PCAN_ERROR_OK
}

// readFDWithTimestamp 同上，FD 帧。Linux 既可能给我们 16-byte Classical 也可能 72-byte FD。
func readFDWithTimestamp(c *linuxChannel, m *TPCANMsgFD, t *TPCANTimestampFD) TPCANStatus {
	var buf [linuxCANFDFrameSize]byte
	var oob [128]byte
	n, oobn, _, _, err := unix.Recvmsg(c.fd, buf[:], oob[:], 0)
	if err != nil {
		return errnoToReadStatus(err)
	}
	switch n {
	case linuxCANFrameSize:
		var cm TPCANMsg
		if status := decodeLinuxCANFrame(buf[:linuxCANFrameSize], &cm); status != PCAN_ERROR_OK {
			return status
		}
		m.ID = cm.ID
		m.MsgType = cm.MsgType
		m.DLC = cm.Len
		copy(m.Data[:], cm.Data[:])
	case linuxCANFDFrameSize:
		if status := decodeLinuxCANFDFrame(buf[:], m); status != PCAN_ERROR_OK {
			return status
		}
	default:
		return PCAN_ERROR_ILLDATA
	}
	if t != nil {
		var ts TPCANTimestamp
		writeKernelTimestamp(oob[:oobn], &ts)
		// FD timestamp 直接是 μs 总数
		*t = uint64(ts.MillisOverflow)*1000*(1<<32) + uint64(ts.Millis)*1000 + uint64(ts.Micros)
	}
	return PCAN_ERROR_OK
}

// writeKernelTimestamp 把控制消息里的 timeval/timespec 解析成微秒，写入 TPCANTimestamp。
// 不识别的 cmsg 退回到 fillTimestamp。
func writeKernelTimestamp(oob []byte, t *TPCANTimestamp) {
	cmsgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		fillTimestamp(t)
		return
	}
	for _, cm := range cmsgs {
		if cm.Header.Level != unix.SOL_SOCKET {
			continue
		}
		switch cm.Header.Type {
		case unix.SO_TIMESTAMP:
			if len(cm.Data) >= int(unsafe.Sizeof(unix.Timeval{})) {
				tv := *(*unix.Timeval)(unsafe.Pointer(&cm.Data[0]))
				totalUs := uint64(tv.Sec)*1_000_000 + uint64(tv.Usec)
				timestampFromMicros(totalUs, t)
				return
			}
		case unix.SO_TIMESTAMPNS:
			if len(cm.Data) >= int(unsafe.Sizeof(unix.Timespec{})) {
				ts := *(*unix.Timespec)(unsafe.Pointer(&cm.Data[0]))
				totalUs := uint64(ts.Sec)*1_000_000 + uint64(ts.Nsec)/1000
				timestampFromMicros(totalUs, t)
				return
			}
		case unix.SO_TIMESTAMPING:
			// 三段 timespec：software / hw_software_legacy / hw
			if len(cm.Data) >= 3*int(unsafe.Sizeof(unix.Timespec{})) {
				p := unsafe.Pointer(&cm.Data[0])
				sw := *(*unix.Timespec)(p)
				hw := *(*unix.Timespec)(unsafe.Add(p, 2*int(unsafe.Sizeof(unix.Timespec{}))))
				chosen := hw
				if hw.Sec == 0 && hw.Nsec == 0 {
					chosen = sw
				}
				totalUs := uint64(chosen.Sec)*1_000_000 + uint64(chosen.Nsec)/1000
				timestampFromMicros(totalUs, t)
				return
			}
		}
	}
	fillTimestamp(t)
}

// timestampFromMicros 拆 μs 总数到 TPCANTimestamp 的 Millis/Overflow/Micros 三字段。
func timestampFromMicros(totalUs uint64, t *TPCANTimestamp) {
	totalMillis := totalUs / 1000
	t.Millis = uint32(totalMillis)
	t.MillisOverflow = uint16(totalMillis >> 32)
	t.Micros = uint16(totalUs % 1000)
}

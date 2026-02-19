package protocol

import (
	"encoding/binary"
	"errors"
)

// 帧结构：
// +----------------+------------------+
// | Channel ID (2) | Control Sig (2) |
// +----------------+------------------+
// |         Data (变长)              |
// +----------------------------------+

const (
	// HeaderSize 帧头部大小（4字节）
	HeaderSize = 4
	// MaxFrameSize 最大帧大小（10MB），防止恶意客户端耗尽内存
	MaxFrameSize = 10 * 1024 * 1024
)

var (
	// ErrFrameTooShort 帧数据太短
	ErrFrameTooShort = errors.New("frame too short")
	// ErrFrameTooLarge 帧数据太大
	ErrFrameTooLarge = errors.New("frame too large")
)

// EncodeFrame 编码帧数据
// channelID: 通道ID（0表示控制消息）
// signal: 控制信号
// data: 数据内容
// 返回: 完整的帧字节数组
func EncodeFrame(channelID, signal uint16, data []byte) []byte {
	frame := make([]byte, HeaderSize+len(data))

	// 写入头部（大端序）
	binary.BigEndian.PutUint16(frame[0:2], channelID)
	binary.BigEndian.PutUint16(frame[2:4], signal)

	// 写入数据
	if len(data) > 0 {
		copy(frame[HeaderSize:], data)
	}

	return frame
}

// DecodeFrame 解码帧数据
// frame: 完整的帧字节数组
// 返回: channelID, signal, data, error
func DecodeFrame(frame []byte) (uint16, uint16, []byte, error) {
	if len(frame) < HeaderSize {
		return 0, 0, nil, ErrFrameTooShort
	}

	if len(frame) > MaxFrameSize {
		return 0, 0, nil, ErrFrameTooLarge
	}

	// 读取头部（大端序）
	channelID := binary.BigEndian.Uint16(frame[0:2])
	signal := binary.BigEndian.Uint16(frame[2:4])

	// 读取数据
	var data []byte
	if len(frame) > HeaderSize {
		data = frame[HeaderSize:]
	}

	return channelID, signal, data, nil
}

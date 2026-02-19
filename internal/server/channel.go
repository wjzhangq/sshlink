package server

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/wjzhangq/sshlink/internal/common"
	"github.com/wjzhangq/sshlink/internal/protocol"
)

// Channel 通道
type Channel struct {
	id      uint16
	tcpConn net.Conn
	client  *Client
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	writeMu sync.Mutex
}

// NewChannel 创建新通道
func NewChannel(id uint16, tcpConn net.Conn, client *Client) *Channel {
	ctx, cancel := context.WithCancel(context.Background())

	return &Channel{
		id:      id,
		tcpConn: tcpConn,
		client:  client,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动通道
func (ch *Channel) Start() {
	common.SafeGoWithName("channel-tcp-to-ws", ch.tcpToWs)
}

// Close 关闭通道
func (ch *Channel) Close() {
	ch.cancel()

	if ch.tcpConn != nil {
		ch.tcpConn.Close()
	}

	ch.wg.Wait()
}

// tcpToWs TCP -> WebSocket
func (ch *Channel) tcpToWs() {
	ch.wg.Add(1)
	defer ch.wg.Done()
	defer ch.client.closeChannel(ch.id)

	buf := make([]byte, 32*1024) // 32KB 缓冲区

	for {
		select {
		case <-ch.ctx.Done():
			return
		default:
			ch.tcpConn.SetReadDeadline(time.Now().Add(120 * time.Second))

			n, err := ch.tcpConn.Read(buf)
			if err != nil {
				if err != io.EOF {
					common.Debug("channel %d tcp read error: %v", ch.id, err)
				}
				// 通知客户端关闭通道
				ch.client.SendFrame(ch.id, protocol.SIG_CHANNEL_CLOSE, nil)
				return
			}

			if n > 0 {
				// 发送数据到客户端
				if err := ch.client.SendFrame(ch.id, protocol.SIG_CHANNEL_DATA, buf[:n]); err != nil {
					common.Error("channel %d send data error: %v", ch.id, err)
					return
				}

				common.Debug("channel %d sent %d bytes to client", ch.id, n)
			}
		}
	}
}

// WriteToTCP 写入TCP连接
func (ch *Channel) WriteToTCP(data []byte) {
	ch.writeMu.Lock()
	defer ch.writeMu.Unlock()

	ch.tcpConn.SetWriteDeadline(time.Now().Add(30 * time.Second))

	n, err := ch.tcpConn.Write(data)
	if err != nil {
		common.Error("channel %d tcp write error: %v", ch.id, err)
		ch.Close()
		return
	}

	common.Debug("channel %d wrote %d bytes to tcp", ch.id, n)
}

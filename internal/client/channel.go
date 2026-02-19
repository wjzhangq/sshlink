package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/wjzhangq/sshlink/internal/common"
	"github.com/wjzhangq/sshlink/internal/protocol"
)

// LocalChannel 客户端本地通道（连接到本地 SSH 服务）
type LocalChannel struct {
	id      uint16
	conn    net.Conn
	client  *Client
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	writeMu sync.Mutex
}

// newLocalChannel 创建本地通道并连接本地 SSH
func newLocalChannel(id uint16, sshPort string, client *Client) (*LocalChannel, error) {
	addr := fmt.Sprintf("127.0.0.1:%s", sshPort)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to local SSH %s error: %w", addr, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &LocalChannel{
		id:     id,
		conn:   conn,
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Start 启动通道双向转发
func (ch *LocalChannel) Start() {
	common.SafeGoWithName(fmt.Sprintf("lchan-%d-local-to-ws", ch.id), ch.localToWs)
}

// Close 关闭通道
func (ch *LocalChannel) Close() {
	ch.cancel()
	if ch.conn != nil {
		ch.conn.Close()
	}
	ch.wg.Wait()
}

// WriteToLocal 将 WebSocket 数据写入本地 SSH 连接
func (ch *LocalChannel) WriteToLocal(data []byte) {
	ch.writeMu.Lock()
	defer ch.writeMu.Unlock()

	ch.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if _, err := ch.conn.Write(data); err != nil {
		common.Error("channel %d write to local SSH error: %v", ch.id, err)
		ch.Close()
	}
}

// localToWs 本地 SSH -> WebSocket
func (ch *LocalChannel) localToWs() {
	ch.wg.Add(1)
	defer ch.wg.Done()
	defer ch.client.closeChannel(ch.id)

	buf := make([]byte, 32*1024)

	for {
		select {
		case <-ch.ctx.Done():
			return
		default:
			ch.conn.SetReadDeadline(time.Now().Add(120 * time.Second))

			n, err := ch.conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					common.Debug("channel %d local SSH read error: %v", ch.id, err)
				}
				// 通知服务端关闭通道
				ch.client.sendFrame(ch.id, protocol.SIG_CHANNEL_CLOSE, nil)
				return
			}

			if n > 0 {
				if err := ch.client.sendFrame(ch.id, protocol.SIG_CHANNEL_DATA, buf[:n]); err != nil {
					common.Error("channel %d send data error: %v", ch.id, err)
					return
				}
				common.Debug("channel %d sent %d bytes to server", ch.id, n)
			}
		}
	}
}

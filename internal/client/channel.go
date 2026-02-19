package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/wjzhangq/sshlink/internal/common"
	"github.com/wjzhangq/sshlink/internal/protocol"
)

// LocalChannel represents a client-side channel connected to the local SSH service.
type LocalChannel struct {
	id      uint16
	conn    net.Conn
	client  *Client
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	writeMu sync.Mutex
}

// newLocalChannel dials the local SSH port and returns a ready LocalChannel.
func newLocalChannel(id uint16, sshPort string, client *Client) (*LocalChannel, error) {
	port, err := strconv.Atoi(sshPort)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid SSH port: %s", sshPort)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
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

// Start launches the bidirectional forwarding goroutine.
func (ch *LocalChannel) Start() {
	common.SafeGoWithName(fmt.Sprintf("lchan-%d-local-to-ws", ch.id), ch.localToWs)
}

// Close cancels the channel context and closes the underlying TCP connection.
func (ch *LocalChannel) Close() {
	ch.cancel()
	if ch.conn != nil {
		ch.conn.Close()
	}
	ch.wg.Wait()
}

// WriteToLocal writes WebSocket data into the local SSH connection.
func (ch *LocalChannel) WriteToLocal(data []byte) {
	ch.writeMu.Lock()
	defer ch.writeMu.Unlock()

	ch.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if _, err := ch.conn.Write(data); err != nil {
		common.Error("channel %d write to local SSH error: %v", ch.id, err)
		ch.Close()
	}
}

// localToWs forwards data from the local SSH connection to the WebSocket server.
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
				// notify server to close channel
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

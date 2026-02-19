package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wjzhangq/sshlink/internal/common"
	"github.com/wjzhangq/sshlink/internal/protocol"
)

var (
	// ErrNoAvailablePort 无可用端口
	ErrNoAvailablePort = errors.New("no available port")
	// ErrClientNotFound 客户端未找到
	ErrClientNotFound = errors.New("client not found")
)

// ClientInfo 客户端注册信息
type ClientInfo struct {
	Username   string    // 用户名
	Hostname   string    // 主机名
	Model      string    // 电脑型号
	Arch       string    // CPU架构
	SSHPort    string    // SSH端口
	IP         string    // 客户端IP
	RemotePort int       // 服务端分配的端口
	Connected  time.Time // 连接时间
}

// Client 客户端连接
type Client struct {
	id         string
	info       ClientInfo
	conn       *websocket.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	channels   map[uint16]*Channel
	channelsMu sync.RWMutex
	nextChanID uint16
	wg         sync.WaitGroup
	listener   net.Listener
	lastHB     atomic.Int64 // 最后心跳时间（Unix时间戳）
	writeMu    sync.Mutex   // WebSocket写锁
}

// ClientManager 客户端管理器
type ClientManager struct {
	mu             sync.RWMutex
	clients        map[string]*Client
	allocatedPorts map[int]bool
	basePort       int
	maxClients     int
	maxChannels    int
	publicKey      string
	configMgr      *ConfigManager
}

// NewClientManager 创建客户端管理器
func NewClientManager(basePort, maxClients, maxChannels int, publicKey string) *ClientManager {
	return &ClientManager{
		clients:        make(map[string]*Client),
		allocatedPorts: make(map[int]bool),
		basePort:       basePort,
		maxClients:     maxClients,
		maxChannels:    maxChannels,
		publicKey:      publicKey,
		configMgr:      NewConfigManager(),
	}
}

// AllocatePort 分配可用端口
func (cm *ClientManager) AllocatePort() (int, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 从 basePort 开始查找第一个未分配的端口
	for port := cm.basePort; port < 65535; port++ {
		if !cm.allocatedPorts[port] {
			cm.allocatedPorts[port] = true
			return port, nil
		}
	}

	return 0, ErrNoAvailablePort
}

// ReleasePort 释放端口
func (cm *ClientManager) ReleasePort(port int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.allocatedPorts, port)
}

// GenerateClientID 生成客户端ID
func (cm *ClientManager) GenerateClientID(remotePort int) string {
	return fmt.Sprintf("sshlink%d", remotePort-cm.basePort)
}

// AddClient 添加客户端
func (cm *ClientManager) AddClient(client *Client) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.maxClients > 0 && len(cm.clients) >= cm.maxClients {
		return errors.New("max clients reached")
	}

	cm.clients[client.id] = client
	return nil
}

// RemoveClient 移除客户端
func (cm *ClientManager) RemoveClient(clientID string) {
	cm.mu.Lock()
	client, exists := cm.clients[clientID]
	if exists {
		delete(cm.clients, clientID)
	}
	cm.mu.Unlock()

	if exists {
		client.Close()
	}
}

// GetClient 获取客户端
func (cm *ClientManager) GetClient(clientID string) (*Client, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	client, exists := cm.clients[clientID]
	if !exists {
		return nil, ErrClientNotFound
	}

	return client, nil
}

// Count 获取客户端数量
func (cm *ClientManager) Count() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.clients)
}

// CloseAll 关闭所有客户端
func (cm *ClientManager) CloseAll() {
	cm.mu.Lock()
	clients := make([]*Client, 0, len(cm.clients))
	for _, client := range cm.clients {
		clients = append(clients, client)
	}
	cm.clients = make(map[string]*Client)
	cm.mu.Unlock()

	// 并发关闭所有客户端
	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			c.Close()
		}(client)
	}
	wg.Wait()
}

// NewClient 创建新客户端
func NewClient(id string, info ClientInfo, conn *websocket.Conn, listener net.Listener, maxChannels int) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		id:       id,
		info:     info,
		conn:     conn,
		ctx:      ctx,
		cancel:   cancel,
		channels: make(map[uint16]*Channel),
		listener: listener,
	}

	client.lastHB.Store(time.Now().Unix())

	return client
}

// Start 启动客户端处理
func (c *Client) Start() {
	common.SafeGoWithName(fmt.Sprintf("client-%s-read", c.id), c.readLoop)
	common.SafeGoWithName(fmt.Sprintf("client-%s-heartbeat", c.id), c.heartbeatLoop)
	common.SafeGoWithName(fmt.Sprintf("client-%s-accept", c.id), c.acceptLoop)
}

// Close 关闭客户端
func (c *Client) Close() error {
	// 取消 context
	c.cancel()

	// 关闭监听器
	if c.listener != nil {
		c.listener.Close()
	}

	// 关闭所有通道
	c.channelsMu.Lock()
	channels := make([]*Channel, 0, len(c.channels))
	for _, ch := range c.channels {
		channels = append(channels, ch)
	}
	c.channels = make(map[uint16]*Channel)
	c.channelsMu.Unlock()

	for _, ch := range channels {
		ch.Close()
	}

	// 关闭 WebSocket 连接
	c.conn.Close()

	// 等待所有 goroutine 退出
	c.wg.Wait()

	common.Info("client %s closed", c.id)
	return nil
}

// SendFrame 发送帧数据
func (c *Client) SendFrame(channelID, signal uint16, data []byte) error {
	frame := protocol.EncodeFrame(channelID, signal, data)

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return c.conn.WriteMessage(websocket.BinaryMessage, frame)
}

// readLoop 读取消息循环
func (c *Client) readLoop() {
	c.wg.Add(1)
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			c.conn.SetReadDeadline(time.Now().Add(120 * time.Second))

			_, data, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err,
					websocket.CloseGoingAway,
					websocket.CloseAbnormalClosure) {
					common.Error("client %s websocket error: %v", c.id, err)
				}
				return
			}

			c.handleMessage(data)
		}
	}
}

// handleMessage 处理消息
func (c *Client) handleMessage(data []byte) {
	channelID, signal, payload, err := protocol.DecodeFrame(data)
	if err != nil {
		common.Error("client %s decode frame error: %v", c.id, err)
		return
	}

	common.Debug("client %s received signal=%d channelID=%d dataLen=%d",
		c.id, signal, channelID, len(payload))

	switch signal {
	case protocol.SIG_HEARTBEAT:
		c.lastHB.Store(time.Now().Unix())
		// 回复心跳
		c.SendFrame(0, protocol.SIG_HEARTBEAT, nil)

	case protocol.SIG_CHANNEL_DATA:
		c.handleChannelData(channelID, payload)

	case protocol.SIG_CHANNEL_CLOSE:
		c.closeChannel(channelID)

	default:
		common.Error("client %s unknown signal: %d", c.id, signal)
	}
}

// handleChannelData 处理通道数据
func (c *Client) handleChannelData(channelID uint16, data []byte) {
	c.channelsMu.RLock()
	ch, exists := c.channels[channelID]
	c.channelsMu.RUnlock()

	if !exists {
		common.Error("client %s channel %d not found", c.id, channelID)
		return
	}

	ch.WriteToTCP(data)
}

// closeChannel 关闭通道
func (c *Client) closeChannel(channelID uint16) {
	c.channelsMu.Lock()
	ch, exists := c.channels[channelID]
	if exists {
		delete(c.channels, channelID)
	}
	c.channelsMu.Unlock()

	if exists {
		ch.Close()
		common.Info("client %s channel %d closed", c.id, channelID)
	}
}

// heartbeatLoop 心跳循环
func (c *Client) heartbeatLoop() {
	c.wg.Add(1)
	defer c.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			// 发送心跳
			if err := c.SendFrame(0, protocol.SIG_HEARTBEAT, nil); err != nil {
				common.Error("client %s send heartbeat error: %v", c.id, err)
				return
			}

			// 检查心跳超时
			lastHB := c.lastHB.Load()
			if time.Now().Unix()-lastHB > 90 {
				common.Error("client %s heartbeat timeout", c.id)
				return
			}
		}
	}
}

// acceptLoop 接受TCP连接循环
func (c *Client) acceptLoop() {
	c.wg.Add(1)
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			conn, err := c.listener.Accept()
			if err != nil {
				select {
				case <-c.ctx.Done():
					return
				default:
					common.Error("client %s accept error: %v", c.id, err)
					continue
				}
			}

			// 创建新通道
			c.createChannel(conn)
		}
	}
}

// createChannel 创建新通道
func (c *Client) createChannel(tcpConn net.Conn) {
	c.channelsMu.Lock()
	channelID := c.nextChanID
	c.nextChanID++

	ch := NewChannel(channelID, tcpConn, c)
	c.channels[channelID] = ch
	c.channelsMu.Unlock()

	common.Info("client %s created channel %d", c.id, channelID)

	// 通知客户端创建通道
	if err := c.SendFrame(channelID, protocol.SIG_NEW_CHANNEL, nil); err != nil {
		common.Error("client %s send new channel error: %v", c.id, err)
		c.closeChannel(channelID)
		return
	}

	// 启动通道
	ch.Start()
}

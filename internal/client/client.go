package client

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wjzhangq/sshlink/internal/common"
	"github.com/wjzhangq/sshlink/internal/protocol"
)

// Config holds client configuration.
type Config struct {
	ServerURL    string
	Hostname     string
	SSHPort      string
	ReconnectCfg ReconnectConfig
}

// Client manages the WebSocket connection and SSH channel multiplexing.
type Client struct {
	cfg      Config
	clientID string // assigned by server

	conn    *websocket.Conn
	connMu  sync.Mutex
	writeMu sync.Mutex

	connected atomic.Bool

	channels   map[uint16]*LocalChannel
	channelsMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	lastHB atomic.Int64
}

// NewClient creates a new Client.
func NewClient(cfg Config) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		cfg:      cfg,
		channels: make(map[uint16]*LocalChannel),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start connects to the server and launches the reconnect monitor.
func (c *Client) Start() error {
	if err := c.connect(); err != nil {
		return err
	}

	common.SafeGoWithName("client-reconnect", c.reconnectLoop)
	return nil
}

// Stop shuts down the client and waits for all goroutines to exit.
func (c *Client) Stop() {
	c.cancel()
	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.connMu.Unlock()
	c.wg.Wait()
}

// sendFrame encodes and sends a protocol frame over the WebSocket connection.
func (c *Client) sendFrame(channelID, signal uint16, data []byte) error {
	frame := protocol.EncodeFrame(channelID, signal, data)

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return conn.WriteMessage(websocket.BinaryMessage, frame)
}

// connect dials the server and registers the client.
func (c *Client) connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(c.ctx, c.cfg.ServerURL, nil)
	if err != nil {
		return fmt.Errorf("dial %s error: %w", c.cfg.ServerURL, err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	// register
	if err := c.register(conn); err != nil {
		conn.Close()
		c.connMu.Lock()
		c.conn = nil
		c.connMu.Unlock()
		return fmt.Errorf("register error: %w", err)
	}

	c.connected.Store(true)
	c.lastHB.Store(time.Now().Unix())

	// start read loop and heartbeat
	c.wg.Add(2)
	common.SafeGoWithName("client-read", func() {
		defer c.wg.Done()
		c.readLoop(conn)
	})
	common.SafeGoWithName("client-heartbeat", func() {
		defer c.wg.Done()
		c.heartbeatLoop()
	})

	return nil
}

// register sends the registration frame and waits for the server ACK.
func (c *Client) register(conn *websocket.Conn) error {
	hostname := c.cfg.Hostname
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	username := currentUsername()
	model := machineModel()
	arch := runtime.GOARCH
	sshPort := c.cfg.SSHPort
	if sshPort == "" {
		sshPort = "22"
	}

	payload := fmt.Sprintf("%s|%s|%s|%s|%s", username, hostname, model, arch, sshPort)
	frame := protocol.EncodeFrame(0, protocol.SIG_REGISTER, []byte(payload))

	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("send register error: %w", err)
	}

	// receive ACK
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read register ack error: %w", err)
	}

	_, signal, payload2, err := protocol.DecodeFrame(data)
	if err != nil {
		return fmt.Errorf("decode ack error: %w", err)
	}
	if signal != protocol.SIG_REGISTER_ACK {
		return fmt.Errorf("unexpected signal: %d", signal)
	}

	// parse ACK: clientID\npubKey
	parts := strings.SplitN(string(payload2), "\n", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid ack data")
	}

	c.clientID = strings.TrimSpace(parts[0])
	pubKey := strings.TrimSpace(parts[1])

	common.Info("registered as %s", c.clientID)

	// add server public key to authorized_keys
	if err := AddAuthorizedKey(pubKey); err != nil {
		common.Error("add authorized key error: %v", err)
	}

	return nil
}

// readLoop reads messages from the WebSocket connection until disconnected or context cancelled.
func (c *Client) readLoop(conn *websocket.Conn) {
	defer func() {
		c.connected.Store(false)
		conn.Close()
		c.closeAllChannels()
		common.Info("disconnected from server")
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(120 * time.Second))

			_, data, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err,
					websocket.CloseGoingAway,
					websocket.CloseAbnormalClosure) {
					common.Error("websocket error: %v", err)
				}
				return
			}

			c.handleMessage(data)
		}
	}
}

// handleMessage dispatches an incoming server frame to the appropriate handler.
func (c *Client) handleMessage(data []byte) {
	channelID, signal, payload, err := protocol.DecodeFrame(data)
	if err != nil {
		common.Error("decode frame error: %v", err)
		return
	}

	switch signal {
	case protocol.SIG_HEARTBEAT:
		c.lastHB.Store(time.Now().Unix())
		c.sendFrame(0, protocol.SIG_HEARTBEAT, nil)

	case protocol.SIG_NEW_CHANNEL:
		c.openChannel(channelID)

	case protocol.SIG_CHANNEL_DATA:
		c.channelsMu.RLock()
		ch, ok := c.channels[channelID]
		c.channelsMu.RUnlock()
		if ok {
			ch.WriteToLocal(payload)
		}

	case protocol.SIG_CHANNEL_CLOSE:
		c.closeChannel(channelID)

	default:
		common.Error("unknown signal: %d", signal)
	}
}

// openChannel connects to the local SSH service and starts forwarding for channelID.
func (c *Client) openChannel(channelID uint16) {
	sshPort := c.cfg.SSHPort
	if sshPort == "" {
		sshPort = "22"
	}

	ch, err := newLocalChannel(channelID, sshPort, c)
	if err != nil {
		common.Error("open channel %d error: %v", channelID, err)
		c.sendFrame(channelID, protocol.SIG_CHANNEL_CLOSE, nil)
		return
	}

	c.channelsMu.Lock()
	c.channels[channelID] = ch
	c.channelsMu.Unlock()

	common.Info("channel %d opened", channelID)
	ch.Start()
}

// closeChannel closes and removes the channel identified by channelID.
func (c *Client) closeChannel(channelID uint16) {
	c.channelsMu.Lock()
	ch, ok := c.channels[channelID]
	if ok {
		delete(c.channels, channelID)
	}
	c.channelsMu.Unlock()

	if ok {
		ch.Close()
		common.Info("channel %d closed", channelID)
	}
}

// closeAllChannels closes every open channel and clears the channel map.
func (c *Client) closeAllChannels() {
	c.channelsMu.Lock()
	channels := make([]*LocalChannel, 0, len(c.channels))
	for _, ch := range c.channels {
		channels = append(channels, ch)
	}
	c.channels = make(map[uint16]*LocalChannel)
	c.channelsMu.Unlock()

	for _, ch := range channels {
		ch.Close()
	}
}

// heartbeatLoop sends periodic heartbeats and closes the connection on timeout.
func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if !c.connected.Load() {
				return
			}

			if err := c.sendFrame(0, protocol.SIG_HEARTBEAT, nil); err != nil {
				common.Error("send heartbeat error: %v", err)
				return
			}

			if time.Now().Unix()-c.lastHB.Load() > 90 {
				common.Error("heartbeat timeout")
				c.connMu.Lock()
				if c.conn != nil {
					c.conn.Close()
				}
				c.connMu.Unlock()
				return
			}
		}
	}
}

// reconnectLoop monitors the connection and retries with exponential backoff on disconnect.
func (c *Client) reconnectLoop() {
	cfg := c.cfg.ReconnectCfg
	delay := cfg.InitialDelay

	for attempt := 1; ; attempt++ {
		// wait until disconnected
		for c.connected.Load() {
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
		}

		select {
		case <-c.ctx.Done():
			return
		default:
		}

		// check max retries
		if cfg.MaxRetries > 0 && attempt > cfg.MaxRetries {
			common.Error("max retries (%d) reached, giving up", cfg.MaxRetries)
			c.cancel()
			return
		}

		common.Info("reconnecting... (attempt %d, delay %s)", attempt, delay)

		select {
		case <-time.After(delay):
		case <-c.ctx.Done():
			return
		}

		if err := c.connect(); err != nil {
			common.Error("reconnect failed: %v", err)
			delay = time.Duration(float64(delay) * cfg.BackoffMultiplier)
			if delay > cfg.MaxDelay {
				delay = cfg.MaxDelay
			}
			continue
		}

		common.Info("reconnected successfully")
		delay = cfg.InitialDelay // reset delay on success
		attempt = 0
	}
}

// currentUsername returns the current OS username.
func currentUsername() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}
	return "unknown"
}

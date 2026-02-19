package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wjzhangq/sshlink/internal/common"
	"github.com/wjzhangq/sshlink/internal/protocol"
)

// Server 服务端
type Server struct {
	listenAddr  string
	listenPort  int
	basePort    int
	publicKey   string
	maxClients  int
	maxChannels int
	healthPort  int

	clientMgr *ClientManager
	upgrader  websocket.Upgrader
	metrics   *ServerMetrics

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	shutdown chan struct{}
}

// Config 服务端配置
type Config struct {
	ListenAddr    string
	ListenPort    int
	BasePort      int
	PublicKeyPath string
	MaxClients    int
	MaxChannels   int
	HealthPort    int
}

// NewServer 创建服务端
func NewServer(cfg Config) (*Server, error) {
	// 读取公钥
	publicKey, err := readPublicKey(cfg.PublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read public key error: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		listenAddr:  cfg.ListenAddr,
		listenPort:  cfg.ListenPort,
		basePort:    cfg.BasePort,
		publicKey:   publicKey,
		maxClients:  cfg.MaxClients,
		maxChannels: cfg.MaxChannels,
		healthPort:  cfg.HealthPort,
		ctx:         ctx,
		cancel:      cancel,
		shutdown:    make(chan struct{}),
		metrics:     NewServerMetrics(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	s.clientMgr = NewClientManager(cfg.BasePort, cfg.MaxClients, cfg.MaxChannels, publicKey)

	return s, nil
}

// Start 启动服务端
func (s *Server) Start() error {
	// 备份 SSH 配置
	if err := s.clientMgr.configMgr.Backup(); err != nil {
		return fmt.Errorf("backup SSH config error: %w", err)
	}

	// 启动健康检查服务
	if s.healthPort > 0 {
		healthSrv := NewHealthServer(s.healthPort, s.clientMgr, s.metrics)
		healthSrv.Start()
	}

	// 启动定期指标输出（每60秒）
	s.metrics.StartReporter(60*time.Second, s.shutdown)

	// 启动 HTTP 服务
	addr := fmt.Sprintf("%s:%d", s.listenAddr, s.listenPort)
	http.HandleFunc("/", s.handleWebSocket)

	server := &http.Server{
		Addr: addr,
	}

	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		common.Info("shutting down gracefully...")
		s.Shutdown()
		server.Shutdown(context.Background())
	}()

	common.Info("server listening on %s", addr)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

// Shutdown 优雅关闭
func (s *Server) Shutdown() {
	close(s.shutdown)
	s.cancel()

	// 关闭所有客户端
	s.clientMgr.CloseAll()

	// 等待所有 goroutine 退出
	s.wg.Wait()

	common.Info("server stopped")
}

// handleWebSocket 处理 WebSocket 连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 升级为 WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		common.Error("upgrade error: %v", err)
		return
	}

	// 获取客户端 IP
	clientIP := getClientIP(r)
	common.Info("new connection from %s", clientIP)

	// 处理客户端注册
	if err := s.handleClientRegister(conn, clientIP); err != nil {
		common.Error("handle client register error: %v", err)
		conn.Close()
	}
}

// handleClientRegister 处理客户端注册
func (s *Server) handleClientRegister(conn *websocket.Conn, clientIP string) error {
	// 设置读取超时
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	// 读取注册消息
	_, data, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read register message error: %w", err)
	}

	// 解码帧
	channelID, signal, payload, err := protocol.DecodeFrame(data)
	if err != nil {
		return fmt.Errorf("decode frame error: %w", err)
	}

	if channelID != 0 || signal != protocol.SIG_REGISTER {
		return fmt.Errorf("invalid register message: channelID=%d signal=%d", channelID, signal)
	}

	// 解析注册信息: 用户名|主机名|电脑型号|CPU架构|SSH端口
	parts := strings.Split(string(payload), "|")
	if len(parts) != 5 {
		return fmt.Errorf("invalid register data: %s", string(payload))
	}

	info := ClientInfo{
		Username:  parts[0],
		Hostname:  parts[1],
		Model:     parts[2],
		Arch:      parts[3],
		SSHPort:   parts[4],
		IP:        clientIP,
		Connected: time.Now(),
	}

	// 分配端口
	remotePort, err := s.clientMgr.AllocatePort()
	if err != nil {
		return fmt.Errorf("allocate port error: %w", err)
	}

	info.RemotePort = remotePort

	// 生成客户端 ID
	clientID := s.clientMgr.GenerateClientID(remotePort)

	// 启动本地监听
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", remotePort))
	if err != nil {
		s.clientMgr.ReleasePort(remotePort)
		return fmt.Errorf("listen error: %w", err)
	}

	// 创建客户端
	client := NewClient(clientID, info, conn, listener, s.maxChannels)

	// 添加到管理器
	if err := s.clientMgr.AddClient(client); err != nil {
		listener.Close()
		s.clientMgr.ReleasePort(remotePort)
		return fmt.Errorf("add client error: %w", err)
	}

	// 发送注册确认: 客户端ID\n公钥
	ackData := fmt.Sprintf("%s\n%s", clientID, s.publicKey)
	ackFrame := protocol.EncodeFrame(0, protocol.SIG_REGISTER_ACK, []byte(ackData))

	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if err := conn.WriteMessage(websocket.BinaryMessage, ackFrame); err != nil {
		s.clientMgr.RemoveClient(clientID)
		s.clientMgr.ReleasePort(remotePort)
		return fmt.Errorf("send register ack error: %w", err)
	}

	// 添加 SSH 配置
	homeDir, _ := os.UserHomeDir()
	identityFile := filepath.Join(homeDir, ".ssh", "id_rsa")

	comment := fmt.Sprintf("user=%s, hostname=%s, ip=%s, remote_port=%d, local_port=%s, connected=%s",
		info.Username, info.Hostname, info.IP, info.RemotePort, info.SSHPort,
		info.Connected.Format(time.RFC3339))

	hostConfig := HostConfig{
		Host:         clientID,
		HostName:     "127.0.0.1",
		User:         info.Username,
		Port:         remotePort,
		IdentityFile: identityFile,
		Comment:      comment,
	}

	if err := s.clientMgr.configMgr.AddHost(hostConfig); err != nil {
		common.Error("add SSH config error: %v", err)
	}

	common.Info("client %s registered: %s@%s (%s %s) -> 127.0.0.1:%d",
		clientID, info.Username, info.Hostname, info.Model, info.Arch, remotePort)

	s.metrics.IncClients()

	// 启动客户端处理
	client.Start()

	// 等待客户端断开
	go func() {
		<-client.ctx.Done()

		// 清理资源
		s.clientMgr.RemoveClient(clientID)
		s.clientMgr.ReleasePort(remotePort)
		s.clientMgr.configMgr.RemoveHost(clientID)
		s.metrics.DecClients()

		common.Info("client %s disconnected", clientID)
	}()

	return nil
}

// getClientIP 获取客户端IP
func getClientIP(r *http.Request) string {
	// 尝试从 X-Forwarded-For 获取
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// 尝试从 X-Real-IP 获取
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// 从 RemoteAddr 获取
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// readPublicKey 读取公钥文件
func readPublicKey(path string) (string, error) {
	// 展开 ~ 为用户主目录
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(homeDir, path[2:])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

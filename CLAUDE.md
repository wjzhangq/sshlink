# CLAUDE.md - sshlink 项目开发指南

## 项目简介

sshlink 是一个 SSH 反向代理工具，允许通过 WebSocket 隧道访问内网机器的 SSH 服务。

**核心组件：**
- `links`（服务端）：运行在公网服务器，接受客户端连接并提供 SSH 代理
- `linkc`（客户端）：运行在内网机器，连接服务端并转发 SSH 流量

## 技术栈

- **语言**：Go 1.18+
- **WebSocket**：`github.com/gorilla/websocket`
- **协议**：自定义二进制协议（4字节头部 + 可变长数据）
- **平台**：服务端 Linux，客户端跨平台（Windows/Linux/macOS，x64/ARM）

## 核心技术要点

### 1. 自定义二进制协议

```
帧结构（4字节头部）：
+----------------+------------------+
| Channel ID (2) | Control Sig (2) |
+----------------+------------------+
|         Data (变长)              |
+----------------------------------+
```

**控制信号：**
- `0x0001` SIG_REGISTER - 客户端注册
- `0x0002` SIG_REGISTER_ACK - 注册确认
- `0x0003` SIG_NEW_CHANNEL - 创建新通道
- `0x0004` SIG_CHANNEL_DATA - 数据传输
- `0x0005` SIG_CHANNEL_CLOSE - 关闭通道
- `0x0006` SIG_HEARTBEAT - 心跳

### 2. 多路复用机制

- 单个 WebSocket 连接承载多个 SSH 会话
- 通过 Channel ID 区分不同会话（0 保留给控制消息）
- 服务端为每个新 SSH 连接分配递增的 Channel ID

### 3. SSH 配置自动管理

服务端在 `~/.ssh/config` 中动态维护客户端配置：

```
Host sshlink1  # user=haodou, hostname=my-laptop, ip=203.0.113.1, remote_port=1001, local_port=22, connected=2025-02-19T10:00:00Z
    HostName 127.0.0.1
    User haodou
    Port 1001
    IdentityFile ~/.ssh/id_rsa
    StrictHostKeyChecking accept-new
```

**关键点：**
- 注释中包含完整客户端信息
- 客户端断开时自动清理配置
- 启动时备份原配置到 `~/.ssh/config.bak.时间戳`

### 4. 免密登录机制

- 服务端将公钥发送给客户端
- 客户端自动添加到 `~/.ssh/authorized_keys`
- 跨平台权限处理（Linux/macOS: 600，Windows: 确保可读）

## 代码结构建议

```
sshlink/
├── cmd/
│   ├── links/          # 服务端入口
│   │   └── main.go
│   └── linkc/          # 客户端入口
│       └── main.go
├── internal/
│   ├── protocol/       # 协议定义和编解码
│   │   ├── frame.go    # 帧结构
│   │   └── signal.go   # 控制信号常量
│   ├── server/         # 服务端逻辑
│   │   ├── manager.go  # 客户端管理器
│   │   ├── config.go   # SSH 配置管理和原子操作
│   │   ├── channel.go  # 通道管理
│   │   ├── forward.go  # 端口转发
│   │   ├── metrics.go  # 监控指标
│   │   └── health.go   # 健康检查
│   ├── client/         # 客户端逻辑
│   │   ├── ssh.go      # SSH 服务检查
│   │   ├── keys.go     # 公钥管理
│   │   ├── channel.go  # 通道处理
│   │   └── reconnect.go # 重连机制
│   └── common/         # 共享工具
│       ├── logger.go   # 日志
│       ├── utils.go    # 工具函数
│       └── safego.go   # Panic 恢复
├── go.mod
└── go.sum
```

## 关键实现细节

### 1. 服务端端口分配

```go
// 客户端管理器维护已分配端口集合
type ClientManager struct {
    mu            sync.RWMutex
    clients       map[string]*Client
    allocatedPorts map[int]bool  // 已分配的端口集合
    basePort      int
}

// 高效查找可用端口：O(n) 时间复杂度，n 为已分配端口数
func (cm *ClientManager) allocatePort() (int, error) {
    cm.mu.Lock()
    defer cm.mu.Unlock()

    // 从 basePort 开始查找第一个未分配的端口
    for port := cm.basePort; port < 65535; port++ {
        if !cm.allocatedPorts[port] {
            cm.allocatedPorts[port] = true
            return port, nil
        }
    }
    return 0, errors.New("no available port")
}

// 释放端口
func (cm *ClientManager) releasePort(port int) {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    delete(cm.allocatedPorts, port)
}
```

### 2. 客户端 ID 生成

```go
// 规则：sshlink + (remote_port - base_port)
clientID := fmt.Sprintf("sshlink%d", remotePort - basePort)
```

**稳定序号机制：**
- 客户端首次注册成功后，将分配到的 clientID 写入可执行文件同级目录的 `.sshlink_id` 文件
- 再次连接时读取该文件，提取序号作为 `preferred_num` 发送给服务端
- 服务端优先尝试分配该序号对应的端口（`basePort + preferred_num`）
- 若端口空闲则直接复用，保持序号不变；若被占用则正常分配新端口并更新缓存

### 3. 注册信息格式

```
客户端 -> 服务端：用户名|主机名|电脑型号|CPU架构|SSH端口|preferred_num
服务端 -> 客户端：客户端ID\n公钥内容
```

`preferred_num` 为客户端上次分配到的序号（整数字符串），首次连接时为空。服务端若该序号对应端口空闲则优先分配，否则正常分配新端口。向后兼容：5 字段的旧客户端仍可正常连接。

### 4. 跨平台 SSH 检查

**Windows:**
```go
// 检查 sshd 服务或 OpenSSH 可执行文件
_, err := exec.Command("powershell", "Get-Service", "sshd").Output()
```

**Linux:**
```go
// 检查 sshd 进程
_, err := exec.Command("systemctl", "is-active", "sshd").Output()
```

**macOS:**
```go
// 检查远程登录状态
_, err := exec.Command("systemsetup", "-getremotelogin").Output()
```

### 5. 并发安全

所有共享数据结构必须加锁：
```go
type ClientManager struct {
    mu      sync.RWMutex
    clients map[string]*Client
}
```

### 6. 心跳机制

- 每 30 秒发送一次心跳
- 连续 3 次无响应（90 秒）视为断开
- 双向心跳（客户端和服务端都发送）

### 7. Context 生命周期管理

**所有连接必须使用 context 控制生命周期：**

```go
// 服务端客户端结构
type Client struct {
    id       string
    conn     *websocket.Conn
    ctx      context.Context
    cancel   context.CancelFunc
    channels map[uint16]*Channel
    wg       sync.WaitGroup  // 等待所有 goroutine 退出
}

func (c *Client) Start() {
    c.ctx, c.cancel = context.WithCancel(context.Background())

    // 启动 goroutine 时增加计数
    c.wg.Add(1)
    go c.readLoop()

    c.wg.Add(1)
    go c.heartbeatLoop()
}

func (c *Client) Close() error {
    // 1. 取消 context，通知所有 goroutine 退出
    c.cancel()

    // 2. 关闭所有通道
    for _, ch := range c.channels {
        ch.Close()
    }

    // 3. 关闭 WebSocket 连接
    c.conn.Close()

    // 4. 等待所有 goroutine 退出
    c.wg.Wait()

    return nil
}

func (c *Client) readLoop() {
    defer c.wg.Done()

    for {
        select {
        case <-c.ctx.Done():
            return  // 优雅退出
        default:
            c.conn.SetReadDeadline(time.Now().Add(120 * time.Second))
            _, data, err := c.conn.ReadMessage()
            if err != nil {
                return
            }
            c.handleMessage(data)
        }
    }
}

// 通道也使用 context 控制
type Channel struct {
    id       uint16
    tcpConn  net.Conn
    ctx      context.Context
    cancel   context.CancelFunc
    wg       sync.WaitGroup
}

func (ch *Channel) Start() {
    ch.ctx, ch.cancel = context.WithCancel(context.Background())

    ch.wg.Add(2)
    go ch.tcpToWs()  // TCP -> WebSocket
    go ch.wsToTcp()  // WebSocket -> TCP
}

func (ch *Channel) Close() {
    ch.cancel()
    if ch.tcpConn != nil {
        ch.tcpConn.Close()
    }
    ch.wg.Wait()
}
```

### 8. Panic 恢复机制

**所有 goroutine 必须有 panic 恢复：**

```go
// common/safego.go
func SafeGo(fn func()) {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                log.Printf("panic recovered: %v\n%s", r, debug.Stack())
            }
        }()
        fn()
    }()
}

// 使用示例
func (c *Client) Start() {
    SafeGo(c.readLoop)
    SafeGo(c.heartbeatLoop)
}
```

### 9. 客户端指数退避重连机制

**重连配置：**

```go
type ReconnectConfig struct {
    InitialDelay      time.Duration  // 初始延迟，默认 1s
    MaxDelay          time.Duration  // 最大延迟，默认 60s
    MaxRetries        int            // 最大重试次数，0=无限，默认 0
    BackoffMultiplier float64        // 退避倍数，默认 2.0
}

type Client struct {
    serverURL      string
    reconnectCfg   ReconnectConfig
    conn           *websocket.Conn
    connected      atomic.Bool
    ctx            context.Context
    cancel         context.CancelFunc
}
```

**重连实现：**

```go
func (c *Client) Start() error {
    c.ctx, c.cancel = context.WithCancel(context.Background())

    // 首次连接
    if err := c.connect(); err != nil {
        return err
    }

    // 启动重连监控
    go c.reconnectLoop()

    return nil
}

func (c *Client) reconnectLoop() {
    for {
        select {
        case <-c.ctx.Done():
            return
        default:
            if !c.connected.Load() {
                c.attemptReconnect()
            }
            time.Sleep(1 * time.Second)
        }
    }
}

func (c *Client) attemptReconnect() {
    delay := c.reconnectCfg.InitialDelay

    for attempt := 1; ; attempt++ {
        // 检查最大重试次数
        if c.reconnectCfg.MaxRetries > 0 && attempt > c.reconnectCfg.MaxRetries {
            log.Printf("max retries (%d) reached, giving up", c.reconnectCfg.MaxRetries)
            c.cancel()
            return
        }

        log.Printf("reconnecting... (attempt %d, delay %s)", attempt, delay)

        // 等待延迟
        select {
        case <-time.After(delay):
        case <-c.ctx.Done():
            return
        }

        // 尝试连接
        if err := c.connect(); err != nil {
            log.Printf("reconnect failed: %v", err)

            // 指数退避
            delay = time.Duration(float64(delay) * c.reconnectCfg.BackoffMultiplier)
            if delay > c.reconnectCfg.MaxDelay {
                delay = c.reconnectCfg.MaxDelay
            }
            continue
        }

        // 连接成功
        log.Println("reconnected successfully")
        return
    }
}

func (c *Client) connect() error {
    // 1. 建立 WebSocket 连接
    dialer := websocket.Dialer{
        HandshakeTimeout: 10 * time.Second,
    }

    conn, _, err := dialer.Dial(c.serverURL, nil)
    if err != nil {
        return fmt.Errorf("dial failed: %w", err)
    }

    c.conn = conn

    // 2. 重新注册
    if err := c.register(); err != nil {
        conn.Close()
        return fmt.Errorf("register failed: %w", err)
    }

    // 3. 标记为已连接
    c.connected.Store(true)

    // 4. 启动消息处理
    go c.readLoop()
    go c.heartbeatLoop()

    return nil
}

func (c *Client) readLoop() {
    defer func() {
        c.connected.Store(false)
        c.conn.Close()
        c.closeAllChannels()
    }()

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
                    log.Printf("websocket error: %v", err)
                }
                return  // 触发重连
            }

            c.handleMessage(data)
        }
    }
}
```

### 10. 并发控制和限流

```go
type Server struct {
    maxClients   int
    maxChannels  int  // 每个客户端最大通道数
    clientSem    chan struct{}  // 信号量控制并发
}

func NewServer(maxClients, maxChannels int) *Server {
    return &Server{
        maxClients:  maxClients,
        maxChannels: maxChannels,
        clientSem:   make(chan struct{}, maxClients),
    }
}

func (s *Server) acceptClient(conn *websocket.Conn) error {
    // 尝试获取信号量
    select {
    case s.clientSem <- struct{}{}:
        // 成功，继续处理
    default:
        // 达到最大连接数，拒绝
        conn.WriteMessage(websocket.CloseMessage,
            websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server full"))
        conn.Close()
        return errors.New("max clients reached")
    }

    // 客户端断开时释放信号量
    defer func() { <-s.clientSem }()

    // 处理客户端...
    return nil
}

// 限制每个客户端的通道数
func (c *Client) createChannel(channelID uint16) error {
    c.mu.Lock()
    defer c.mu.Unlock()

    if len(c.channels) >= c.maxChannels {
        return errors.New("max channels reached")
    }

    // 创建通道...
    return nil
}
```

### 11. SSH 配置文件原子操作

```go
// 使用临时文件 + 原子重命名，避免配置损坏
func (cm *ConfigManager) updateConfig(hosts []HostConfig) error {
    // 1. 读取现有配置
    existingHosts, err := cm.readNonSSHLinkHosts()
    if err != nil {
        return err
    }

    // 2. 合并配置
    allHosts := append(existingHosts, hosts...)

    // 3. 写入临时文件
    tmpFile := cm.configPath + ".tmp"
    if err := cm.writeConfigToFile(tmpFile, allHosts); err != nil {
        return err
    }

    // 4. 原子重命名（覆盖原文件）
    if err := os.Rename(tmpFile, cm.configPath); err != nil {
        os.Remove(tmpFile)  // 清理临时文件
        return err
    }

    return nil
}

// 启动时验证配置文件完整性
func (cm *ConfigManager) validateConfig() error {
    data, err := os.ReadFile(cm.configPath)
    if err != nil {
        // 尝试从备份恢复
        return cm.restoreFromBackup()
    }

    // 验证配置格式
    return nil
}
```

### 12. 监控和健康检查

```go
type ServerMetrics struct {
    mu              sync.RWMutex
    activeClients   int
    totalChannels   int
    bytesReceived   uint64
    bytesSent       uint64
    errors          uint64
    startTime       time.Time
}

func (m *ServerMetrics) Report() {
    m.mu.RLock()
    defer m.mu.RUnlock()

    uptime := time.Since(m.startTime)
    log.Printf("Metrics: clients=%d, channels=%d, rx=%d, tx=%d, errors=%d, uptime=%s",
        m.activeClients, m.totalChannels, m.bytesReceived, m.bytesSent,
        m.errors, uptime)
}

// 定期输出指标
func (s *Server) metricsLoop() {
    ticker := time.NewTicker(60 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            s.metrics.Report()
        case <-s.shutdown:
            return
        }
    }
}

// HTTP 健康检查端点
func (s *Server) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
    status := map[string]interface{}{
        "status":  "ok",
        "clients": s.clientManager.Count(),
        "uptime":  time.Since(s.metrics.startTime).String(),
    }
    json.NewEncoder(w).Encode(status)
}
```

### 13. 优雅关闭

```go
type Server struct {
    shutdown chan struct{}
    wg       sync.WaitGroup
}

func (s *Server) Start() error {
    s.shutdown = make(chan struct{})

    // 监听系统信号
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigChan
        log.Println("shutting down gracefully...")
        s.Shutdown()
    }()

    return nil
}

func (s *Server) Shutdown() {
    close(s.shutdown)

    // 1. 停止接受新连接
    s.listener.Close()

    // 2. 关闭所有客户端连接
    s.clientManager.CloseAll()

    // 3. 等待所有 goroutine 退出
    s.wg.Wait()

    log.Println("server stopped")
}
```

## 配置参数

### 服务端配置

```go
type ServerConfig struct {
    // 基础配置
    ListenAddr    string
    ListenPort    int
    BasePort      int
    PublicKeyPath string

    // 稳定性配置
    MaxClients           int           // 最大客户端数，默认 1000
    MaxChannelsPerClient int           // 每客户端最大通道数，默认 10
    ReadTimeout          time.Duration // 读超时，默认 120s
    WriteTimeout         time.Duration // 写超时，默认 30s
    HeartbeatInterval    time.Duration // 心跳间隔，默认 30s
    HeartbeatTimeout     time.Duration // 心跳超时，默认 90s

    // 监控配置
    MetricsInterval time.Duration // 指标输出间隔，默认 60s
    HealthCheckPort int           // 健康检查端口，默认 8081
}
```

### 客户端配置

```go
type ClientConfig struct {
    // 基础配置
    ServerURL string
    Hostname  string

    // 重连配置
    ReconnectEnabled  bool          // 是否启用重连，默认 true
    InitialDelay      time.Duration // 初始延迟，默认 1s
    MaxDelay          time.Duration // 最大延迟，默认 60s
    MaxRetries        int           // 最大重试次数，0=无限，默认 0
    BackoffMultiplier float64       // 退避倍数，默认 2.0

    // 超时配置
    ConnectTimeout    time.Duration // 连接超时，默认 10s
    ReadTimeout       time.Duration // 读超时，默认 120s
    WriteTimeout      time.Duration // 写超时，默认 30s
    HeartbeatInterval time.Duration // 心跳间隔，默认 30s
}
```

## 开发注意事项

### 安全性
- 仅操作 `sshlink` 开头的 Host 配置，不影响其他配置
- 备份原始 SSH 配置
- 正确设置文件权限（authorized_keys: 600）

### 稳定性要求（必须遵守）
- **所有连接必须使用 context 控制生命周期**
- **所有 goroutine 必须有 panic 恢复机制**
- **使用 sync.WaitGroup 确保 goroutine 正确退出**
- **实现优雅关闭，避免资源泄漏**
- **客户端必须实现指数退避重连机制**
- **服务端必须实现并发控制和限流**
- **SSH 配置文件必须使用原子操作**

### 错误处理
- WebSocket 连接断开时清理所有资源
- 端口占用时自动递增查找
- SSH 服务未安装时提供明确的安装指令
- 所有错误路径都有日志记录
- 区分可恢复错误和致命错误

### 日志
- 标准格式：时间戳 + 级别 + 消息
- `-v` 参数启用详细日志
- 记录所有关键操作（注册、通道创建/关闭、配置更新）
- 记录重连尝试和结果
- 定期输出监控指标

### 资源清理
- 确保所有 goroutine 正确退出（使用 WaitGroup）
- 关闭所有 TCP 连接和监听器
- 更新 SSH 配置文件
- 释放分配的端口
- 清理临时文件

### 测试建议
1. **单元测试**：协议编解码、端口分配、配置解析
2. **集成测试**：完整的注册流程、通道建立、数据转发
3. **跨平台测试**：Windows/Linux/macOS 客户端
4. **压力测试**：多客户端、多通道并发
5. **稳定性测试**：
   - 长时间运行测试（24-72小时）
   - 内存泄漏检测（pprof）
   - goroutine 泄漏检测
   - 文件描述符监控
6. **重连测试**：
   - 服务端重启测试
   - 网络中断模拟
   - 心跳超时测试
   - 指数退避验证

## 实现顺序建议

1. **协议层**：定义帧结构和控制信号
2. **服务端基础**：WebSocket 监听、客户端管理
3. **客户端基础**：连接服务端、注册流程
4. **SSH 配置管理**：读写 config 文件
5. **公钥管理**：authorized_keys 处理
6. **通道管理**：多路复用实现
7. **数据转发**：双向流量转发
8. **心跳机制**：保活和断线检测
9. **跨平台适配**：SSH 检查和路径处理
10. **日志和错误处理**：完善日志和异常处理

## 命令行示例

**服务端：**
```bash
links -p 8080 -h 0.0.0.0 -i ~/.ssh/id_rsa.pub -b 1001 -v
```

**客户端：**
```bash
linkc ws://server-address:8080 -h my-hostname -v
```

**使用：**
```bash
ssh sshlink1  # 直接连接到内网机器
```

## 参考资源

- WebSocket RFC: https://tools.ietf.org/html/rfc6455
- SSH 配置格式: `man ssh_config`
- Go WebSocket 库: https://github.com/gorilla/websocket

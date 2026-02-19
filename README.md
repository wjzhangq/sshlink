# sshlink

通过 WebSocket 隧道访问内网机器 SSH 服务的反向代理工具。

## 工作原理

```
用户                  公网服务器 (links)          内网机器 (linkc)
 │                          │                          │
 │  ssh sshlink1            │                          │
 ├─────────────────────────>│  WebSocket 隧道          │
 │                          │<─────────────────────────┤
 │                          │   多路复用 SSH 流量       │
 │<─────────────────────────┤─────────────────────────>│
```

- `links`：运行在公网服务器，接受客户端注册，监听本地端口并转发 SSH 流量
- `linkc`：运行在内网机器，连接服务端，将流量转发到本地 SSH 服务

客户端注册后，服务端自动在 `~/.ssh/config` 中写入配置，用户可直接通过 `ssh sshlink1` 连接内网机器，无需手动配置。

## 安装

**前置条件：** Go 1.18+

```bash
git clone https://github.com/wjzhangq/sshlink.git
cd sshlink
go build -o links ./cmd/links
go build -o linkc ./cmd/linkc
```

## 快速开始

**1. 在公网服务器上启动服务端**

```bash
./links -p 8080 -i ~/.ssh/id_rsa.pub
```

**2. 在内网机器上启动客户端**

```bash
./linkc ws://your-server:8080
```

**3. 直接 SSH 连接内网机器**

```bash
ssh sshlink1
```

## 命令行参数

### links（服务端）

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-h` | `0.0.0.0` | 监听地址 |
| `-p` | `8080` | WebSocket 监听端口 |
| `-b` | `10000` | SSH 代理起始端口 |
| `-i` | `~/.ssh/id_rsa.pub` | 公钥路径（用于免密登录） |
| `--max-clients` | `1000` | 最大同时连接客户端数 |
| `--max-channels` | `10` | 每个客户端最大并发 SSH 会话数 |
| `-v` | `false` | 开启详细日志 |

```bash
./links -p 8080 -h 0.0.0.0 -i ~/.ssh/id_rsa.pub -b 10000 --max-clients 500 -v
```

### linkc（客户端）

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `<server-url>` | 必填 | 服务端 WebSocket 地址 |
| `-h` | 系统主机名 | 自定义主机名 |
| `-ssh-port` | `22` | 本地 SSH 端口 |
| `--max-retries` | `0`（无限） | 最大重连次数 |
| `--initial-delay` | `1s` | 初始重连延迟 |
| `--max-delay` | `60s` | 最大重连延迟 |
| `-v` | `false` | 开启详细日志 |

```bash
./linkc ws://your-server:8080 -h my-laptop --ssh-port 22 -v
```

## SSH 配置自动管理

客户端连接后，服务端自动在 `~/.ssh/config` 中写入：

```
Host sshlink1  # user=alice, hostname=my-laptop, ip=203.0.113.1, remote_port=1001, local_port=22, connected=2025-01-01T10:00:00Z
    HostName 127.0.0.1
    User alice
    Port 1001
    IdentityFile ~/.ssh/id_rsa
    StrictHostKeyChecking accept-new
```

- 仅操作 `sshlink` 前缀的 Host，不影响其他配置
- 客户端断开时自动清理对应配置

## 免密登录

服务端将公钥（`-i` 指定）发送给客户端，客户端自动追加到 `~/.ssh/authorized_keys`，实现免密 SSH 登录。

## 健康检查

服务端在 `-p` 端口提供 HTTP 接口：

```bash
# 健康状态
curl http://your-server:8080/health

# 监控指标
curl http://your-server:8080/metrics
```

响应示例：

```json
{
  "status": "ok",
  "clients": 3,
  "uptime": "2h30m15s"
}
```

## 技术细节

- **传输层**：WebSocket（`github.com/gorilla/websocket`）
- **多路复用**：单个 WebSocket 连接承载多个 SSH 会话，通过 Channel ID 区分
- **协议**：自定义二进制帧（4 字节头部：Channel ID + 控制信号）
- **重连**：指数退避，默认 1s → 2s → 4s → ... → 60s
- **心跳**：每 60 秒发送，180 秒无响应视为断开
- **并发安全**：全局 mutex/RWMutex 保护，context 控制 goroutine 生命周期

## 平台支持

| 平台 | 服务端 | 客户端 |
|------|--------|--------|
| Linux | ✅ | ✅ |
| macOS | — | ✅ |
| Windows | — | ✅ |

服务端建议运行在 Linux 公网服务器上。

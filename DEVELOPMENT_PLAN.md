# sshlink 开发计划

## 项目概述

开发一个基于 WebSocket 的 SSH 反向代理工具，包含服务端（links）和客户端（linkc）两个组件。

## 开发阶段

### 阶段 1：项目初始化和协议层

**目标**：搭建项目结构，实现核心协议

#### 任务清单

- [x] 1.1 初始化 Go 项目
  - 创建 `go.mod`
  - 安装依赖：`github.com/gorilla/websocket`
  - 创建目录结构（cmd/、internal/）

- [x] 1.2 实现协议层（`internal/protocol/`）
  - 定义控制信号常量（signal.go）
  - 实现帧编码函数：`EncodeFrame(channelID, signal uint16, data []byte) []byte`
  - 实现帧解码函数：`DecodeFrame(frame []byte) (channelID, signal uint16, data []byte, err error)`
  - 编写单元测试

- [x] 1.3 实现日志模块（`internal/common/logger.go`）
  - 支持不同日志级别（INFO/ERROR/DEBUG）
  - 支持 `-v` 详细模式
  - 统一日志格式（时间戳 + 级别 + 消息）

- [x] 1.4 实现 Panic 恢复工具（`internal/common/safego.go`）
  - 实现 SafeGo 函数包装 goroutine
  - 捕获 panic 并记录堆栈
  - 确保程序不会因单个 goroutine panic 而崩溃

**验收标准**：
- 协议编解码测试通过
- 日志输出格式正确
- Panic 恢复机制测试通过

---

### 阶段 2：服务端核心功能（3-4天）

**目标**：实现服务端基础架构和客户端管理

#### 任务清单

- [x] 2.1 WebSocket 服务器（`internal/server/manager.go`）
  - 启动 WebSocket 监听
  - 处理客户端连接
  - 维护客户端连接映射（map[clientID]*Client）
  - 实现并发安全（sync.RWMutex）
  - **使用 context 控制每个客户端生命周期**
  - **使用 sync.WaitGroup 管理 goroutine**

- [x] 2.2 客户端注册流程
  - 接收 `SIG_REGISTER` 消息
  - 解析注册信息（用户名|主机名|型号|架构|端口）
  - 从 WebSocket 连接获取客户端 IP
  - 分配客户端 ID（sshlink + 序号）
  - 分配本地监听端口（使用端口集合，O(n) 查找）
  - 发送 `SIG_REGISTER_ACK`（客户端ID + 公钥）

- [x] 2.3 SSH 配置管理（`internal/server/config.go`）
  - 读取 `~/.ssh/config` 文件
  - 备份配置文件（config.bak.时间戳）
  - **使用临时文件 + 原子重命名更新配置**
  - 添加 Host 配置（包含注释中的完整信息）
  - 删除指定 Host 配置
  - 仅操作 `sshlink` 开头的 Host
  - 启动时验证配置完整性

- [x] 2.4 端口转发器（`internal/server/channel.go` + `manager.go`）
  - 为每个客户端启动本地 TCP 监听
  - 接受 SSH 连接
  - 分配 Channel ID（从 1 开始递增）
  - 发送 `SIG_NEW_CHANNEL` 到客户端

- [x] 2.5 并发控制（`internal/server/manager.go`）
  - **实现信号量控制最大客户端数**
  - **限制每个客户端的最大通道数**
  - 达到限制时拒绝新连接

- [x] 2.6 监控和健康检查（`internal/server/metrics.go`, `health.go`）
  - **实现指标收集（客户端数、通道数、流量等）**
  - **定期输出指标日志**
  - **提供 HTTP 健康检查端点**

- [x] 2.7 优雅关闭（`cmd/links/main.go`）
  - **监听 SIGINT/SIGTERM 信号**
  - **关闭所有客户端连接**
  - **等待所有 goroutine 退出**

- [x] 2.8 命令行参数解析（`cmd/links/main.go`）
  - `-p`：WebSocket 端口（默认 8080）
  - `-h`：监听地址（默认 0.0.0.0）
  - `-i`：公钥路径（默认 ~/.ssh/id_rsa.pub）
  - `-b`：起始端口（默认 1001）
  - `-v`：详细日志
  - **`--max-clients`：最大客户端数（默认 1000）**
  - **`--max-channels`：每客户端最大通道数（默认 10）**
  - **`--health-port`：健康检查端口（默认 8081）**

**验收标准**：
- 服务端能启动并监听 WebSocket
- 能接受客户端注册并分配 ID 和端口
- SSH 配置文件正确更新且使用原子操作
- 并发控制生效
- 健康检查端点可访问
- 优雅关闭无资源泄漏

---

### 阶段 3：客户端核心功能（3-4天）

**目标**：实现客户端连接和注册

#### 任务清单

- [x] 3.1 SSH 服务检查（`internal/client/ssh.go`）
  - Windows：检查 sshd 服务或 OpenSSH 可执行文件
  - Linux：检查 sshd 进程或服务状态
  - macOS：检查远程登录状态
  - 未安装时提供安装命令提示

- [x] 3.2 系统信息收集
  - 获取当前用户名（`os.UserHomeDir()` + 解析）
  - 获取主机名（`os.Hostname()` 或命令行参数）
  - 获取 CPU 架构（`runtime.GOARCH`）
  - 获取电脑型号（系统调用或 runtime.GOOS）
  - 检测 SSH 端口（默认 22）

- [x] 3.3 WebSocket 连接（`internal/client/client.go`）
  - **使用 context 控制连接生命周期**
  - 连接服务端 WebSocket（设置连接超时）
  - 发送 `SIG_REGISTER` 消息
  - 接收 `SIG_REGISTER_ACK`
  - 解析客户端 ID 和公钥

- [x] 3.4 公钥管理（`internal/client/keys.go`）
  - 读取 `~/.ssh/authorized_keys`（跨平台路径）
  - 检查公钥是否已存在
  - 追加公钥（如果不存在）
  - 设置文件权限：
    - Linux/macOS: 600
    - Windows: 确保可读

- [x] 3.5 指数退避重连机制（`internal/client/reconnect.go` + `client.go`）
  - **实现 ReconnectConfig 配置结构**
  - **实现 reconnectLoop 监控连接状态**
  - **实现 attemptReconnect 指数退避逻辑**
  - **连接失败时自动重连**
  - **重连成功后重新注册**
  - **记录重连尝试和结果**

- [x] 3.6 连接状态管理
  - **使用 atomic.Bool 标记连接状态**
  - **使用 sync.WaitGroup 管理 goroutine**
  - **断开时关闭所有活跃通道**

- [x] 3.7 命令行参数解析（`cmd/linkc/main.go`）
  - 第一个参数：WebSocket 地址（必须）
  - `-h`：自定义主机名（可选）
  - `-v`：详细日志
  - **`--max-retries`：最大重试次数（默认 0=无限）**
  - **`--initial-delay`：初始重连延迟（默认 1s）**
  - **`--max-delay`：最大重连延迟（默认 60s）**

**验收标准**：
- 客户端能检测 SSH 服务状态
- 能成功连接服务端并完成注册
- 公钥正确添加到 authorized_keys
- 重连机制正常工作
- 指数退避延迟正确

---

### 阶段 4：通道管理和数据转发（3-4天）

**目标**：实现多路复用和双向数据转发

#### 任务清单

- [x] 4.1 服务端通道管理（`internal/server/channel.go`）
  - 维护通道映射（map[channelID]*Channel）
  - **Channel 结构包含 context 和 WaitGroup**
  - Channel 结构：包含 TCP 连接、WebSocket 连接、缓冲区
  - 创建新通道（分配 ID、发送 SIG_NEW_CHANNEL）
  - 关闭通道（发送 SIG_CHANNEL_CLOSE、清理资源）
  - **使用 SafeGo 启动 goroutine**

- [x] 4.2 服务端数据转发
  - TCP -> WebSocket：读取 TCP 数据，封装为 SIG_CHANNEL_DATA，发送
  - WebSocket -> TCP：接收 SIG_CHANNEL_DATA，写入 TCP 连接
  - 处理 SIG_CHANNEL_CLOSE：关闭对应 TCP 连接
  - 错误处理：任一端断开时清理通道
  - **设置读写超时**

- [x] 4.3 客户端通道管理（`internal/client/channel.go`）
  - 维护通道映射（map[channelID]*LocalConn）
  - **通道结构包含 context 和 WaitGroup**
  - 接收 SIG_NEW_CHANNEL：连接本地 SSH 端口（127.0.0.1:22）
  - 关闭通道：关闭本地 TCP 连接
  - **使用 SafeGo 启动 goroutine**

- [x] 4.4 客户端数据转发
  - 本地 SSH -> WebSocket：读取本地连接，封装为 SIG_CHANNEL_DATA，发送
  - WebSocket -> 本地 SSH：接收 SIG_CHANNEL_DATA，写入本地连接
  - 处理 SIG_CHANNEL_CLOSE：关闭本地连接
  - 错误处理：任一端断开时清理通道
  - **设置读写超时**

- [x] 4.5 并发处理
  - 每个通道独立 goroutine 处理
  - **使用 sync.WaitGroup 管理 goroutine 生命周期**
  - **使用 context 控制 goroutine 退出**
  - 确保资源正确清理

**验收标准**：
- 能建立 SSH 连接并成功登录
- 多个 SSH 会话可以同时工作
- 连接断开时资源正确清理
- 无 goroutine 泄漏
- 无连接泄漏

---

### 阶段 5：心跳和连接管理（1-2天）

**目标**：实现保活机制和断线处理

#### 任务清单

- [x] 5.1 心跳发送
  - 服务端：每 30 秒发送 SIG_HEARTBEAT（Channel ID = 0）
  - 客户端：每 30 秒发送 SIG_HEARTBEAT（Channel ID = 0）
  - 使用 `time.Ticker` 实现定时发送
  - **使用 context 控制心跳 goroutine**

- [x] 5.2 心跳检测
  - 记录最后接收心跳时间
  - 超过 90 秒无响应视为断开
  - **触发清理流程和重连（客户端）**

- [x] 5.3 客户端断开处理（服务端）
  - 关闭所有活跃通道
  - 关闭本地监听端口
  - **释放分配的端口**
  - 从 `~/.ssh/config` 删除 Host 配置
  - 清理内存中的客户端状态
  - 记录日志

- [x] 5.4 服务端断开处理（客户端）
  - 关闭所有活跃通道
  - 关闭 WebSocket 连接
  - **标记连接状态为断开**
  - **触发自动重连机制**

**验收标准**：
- 心跳正常工作，连接保持稳定
- 客户端断开时服务端正确清理
- 服务端断开时客户端自动重连
- 心跳超时触发重连

---

### 阶段 6：跨平台适配（2-3天）

**目标**：确保在 Windows/Linux/macOS 上正常工作

#### 任务清单

- [ ] 6.1 Windows 适配
  - SSH 服务检查（PowerShell 命令）
  - authorized_keys 路径（%USERPROFILE%\.ssh\）
  - 文件权限处理（确保可读）
  - 安装提示（Add-WindowsCapability）

- [ ] 6.2 Linux 适配
  - SSH 服务检查（systemctl 或 service）
  - authorized_keys 路径（~/.ssh/）
  - 文件权限（chmod 600）
  - 安装提示（apt/yum/dnf）

- [ ] 6.3 macOS 适配
  - SSH 服务检查（systemsetup）
  - authorized_keys 路径（~/.ssh/）
  - 文件权限（chmod 600）
  - 启用提示（systemsetup -setremotelogin on）

- [ ] 6.4 架构支持
  - 编译 x64 和 ARM 版本
  - 正确识别 CPU 架构（runtime.GOARCH）

**验收标准**：
- 在三个平台上都能正常运行
- SSH 服务检查准确
- 文件权限设置正确

---

### 阶段 7：测试和优化（2-3天）

**目标**：全面测试和性能优化

#### 任务清单

- [ ] 7.1 单元测试
  - 协议编解码测试
  - SSH 配置解析测试
  - 端口分配测试
  - 公钥管理测试

- [ ] 7.2 集成测试
  - 完整注册流程测试
  - 单通道数据转发测试
  - 多通道并发测试
  - 断线重连测试

- [ ] 7.3 跨平台测试
  - Windows 客户端测试
  - Linux 客户端测试
  - macOS 客户端测试
  - 混合平台测试（不同平台客户端同时连接）

- [ ] 7.4 压力测试
  - 多客户端并发连接（10+ 客户端）
  - 单客户端多通道（10+ SSH 会话）
  - 大数据量传输（文件传输测试）
  - 长时间运行稳定性测试（24 小时）

- [ ] 7.5 性能优化
  - 减少内存分配（使用 sync.Pool）
  - 优化锁粒度
  - 调整缓冲区大小
  - 性能分析（pprof）

- [ ] 7.6 错误处理完善
  - 所有错误路径都有日志
  - 资源泄漏检查
  - 边界条件处理

- [ ] 7.7 稳定性测试
  - **长时间运行测试（24-72小时）**
  - **内存泄漏检测（pprof）**
  - **goroutine 泄漏检测**
  - **文件描述符监控**
  - **并发竞态检测（go test -race）**

- [ ] 7.8 重连机制测试
  - **服务端重启测试**
  - **网络中断模拟（iptables）**
  - **心跳超时测试**
  - **指数退避验证**
  - **最大重试次数验证**

**验收标准**：
- 所有测试通过
- 无内存泄漏
- 无 goroutine 泄漏
- 无竞态条件
- 性能满足要求（延迟 < 100ms，吞吐量 > 10MB/s）
- 重连机制稳定可靠

---

### 阶段 8：文档和发布（1-2天）

**目标**：完善文档，准备发布

#### 任务清单

- [ ] 8.1 代码文档
  - 为所有公开函数添加注释
  - 添加包级别文档
  - 生成 godoc

- [ ] 8.2 用户文档
  - README.md（项目介绍、安装、使用）
  - 安装指南（各平台编译和安装）
  - 使用示例（常见场景）
  - 故障排查（常见问题）

- [ ] 8.3 编译和打包
  - 编写 Makefile 或构建脚本
  - 交叉编译各平台二进制
  - 创建发布包（tar.gz/zip）

- [ ] 8.4 发布准备
  - 版本号管理（语义化版本）
  - CHANGELOG.md
  - LICENSE 文件
  - GitHub Release

**验收标准**：
- 文档完整清晰
- 各平台二进制可用
- 发布包完整

---

## 开发优先级

### P0（必须完成）
- 协议层实现
- 服务端客户端注册流程
- SSH 配置管理（原子操作）
- 通道管理和数据转发
- **Context 生命周期管理**
- **Panic 恢复机制**
- **客户端指数退避重连**
- 基本的错误处理和日志

### P1（重要）
- 心跳机制
- 跨平台适配
- **并发控制和限流**
- **优雅关闭**
- **监控和健康检查**
- 资源清理
- 基本测试
- **稳定性测试**

### P2（可选）
- 性能优化
- 详细文档
- 压力测试

## 风险和注意事项

### 技术风险
1. **WebSocket 稳定性**：长连接可能被中间网络设备断开
   - 缓解：实现心跳机制和自动重连

2. **并发安全**：多个 goroutine 访问共享数据
   - 缓解：严格使用锁保护，进行竞态检测（`go test -race`）

3. **资源泄漏**：连接、goroutine、文件句柄泄漏
   - 缓解：使用 defer 确保清理，编写清理测试

4. **SSH 配置破坏**：误操作可能破坏用户现有配置
   - 缓解：启动时备份，仅操作 sshlink 开头的 Host

### 平台兼容性风险
1. **Windows SSH 服务**：可能未安装或配置不同
   - 缓解：提供详细的检查和安装指导

2. **文件权限**：不同平台权限模型不同
   - 缓解：针对每个平台单独处理

3. **路径差异**：Windows 使用反斜杠，路径格式不同
   - 缓解：使用 `filepath` 包处理路径

## 时间估算

- **总开发时间**：15-20 天
- **测试时间**：3-5 天
- **文档和发布**：1-2 天
- **总计**：19-27 天（约 4-5 周）

## 里程碑

1. **Week 1**：完成协议层和服务端基础（阶段 1-2）
2. **Week 2**：完成客户端基础和通道管理（阶段 3-4）
3. **Week 3**：完成心跳机制和跨平台适配（阶段 5-6）
4. **Week 4**：测试、优化和文档（阶段 7-8）

## 下一步行动

1. 创建项目目录结构
2. 初始化 Go 模块
3. 开始实现协议层
4. 编写第一个单元测试

---

**文档版本**：v1.0
**创建日期**：2026-02-20
**最后更新**：2026-02-20

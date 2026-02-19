package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wjzhangq/sshlink/internal/common"
	"github.com/wjzhangq/sshlink/internal/server"
)

func main() {
	// 命令行参数
	listenAddr := flag.String("h", "0.0.0.0", "监听地址")
	listenPort := flag.Int("p", 8080, "WebSocket 端口")
	basePort := flag.Int("b", 10000, "起始端口")
	publicKeyPath := flag.String("i", "~/.ssh/id_rsa.pub", "公钥路径")
	maxClients := flag.Int("max-clients", 1000, "最大客户端数")
	maxChannels := flag.Int("max-channels", 10, "每客户端最大通道数")
	verbose := flag.Bool("v", false, "详细日志")

	flag.Parse()

	// 设置日志级别
	common.SetVerbose(*verbose)

	// 创建服务端配置
	cfg := server.Config{
		ListenAddr:    *listenAddr,
		ListenPort:    *listenPort,
		BasePort:      *basePort,
		PublicKeyPath: *publicKeyPath,
		MaxClients:    *maxClients,
		MaxChannels:   *maxChannels,
	}

	// 创建服务端
	srv, err := server.NewServer(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create server error: %v\n", err)
		os.Exit(1)
	}

	// 启动服务端
	common.Info("starting sshlink server...")
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

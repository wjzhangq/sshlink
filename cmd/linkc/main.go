package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wjzhangq/sshlink/internal/client"
	"github.com/wjzhangq/sshlink/internal/common"
)

func main() {
	hostname := flag.String("h", "", "自定义主机名（默认使用系统主机名）")
	sshPort := flag.String("ssh-port", "22", "本地 SSH 端口")
	maxRetries := flag.Int("max-retries", 0, "最大重试次数（0=无限）")
	initialDelay := flag.Duration("initial-delay", 1*time.Second, "初始重连延迟")
	maxDelay := flag.Duration("max-delay", 60*time.Second, "最大重连延迟")
	verbose := flag.Bool("v", false, "详细日志")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: linkc [options] <server-url>\n\n")
		fmt.Fprintf(os.Stderr, "Example: linkc ws://server:8080\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	serverURL := flag.Arg(0)
	common.SetVerbose(*verbose)

	// 检查 SSH 服务
	running, hint := client.CheckSSHService()
	if !running {
		fmt.Fprintf(os.Stderr, "WARNING: SSH service not running.\n%s\n\n", hint)
	}

	cfg := client.Config{
		ServerURL: serverURL,
		Hostname:  *hostname,
		SSHPort:   *sshPort,
		ReconnectCfg: client.ReconnectConfig{
			InitialDelay:      *initialDelay,
			MaxDelay:          *maxDelay,
			MaxRetries:        *maxRetries,
			BackoffMultiplier: 2.0,
		},
	}

	c := client.NewClient(cfg)

	common.Info("connecting to %s...", serverURL)
	if err := c.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "connect error: %v\n", err)
		os.Exit(1)
	}

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	common.Info("shutting down...")
	c.Stop()
}

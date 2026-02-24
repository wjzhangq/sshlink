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

// 编译时注入的默认服务器地址
// 使用: go build -ldflags "-X main.defaultServerURL=ws://your-server:8080"
var defaultServerURL string

func main() {
	hostname := flag.String("h", "", "custom hostname (default: system hostname)")
	sshPort := flag.String("ssh-port", "22", "local SSH port")
	maxRetries := flag.Int("max-retries", 0, "max reconnect attempts (0=unlimited)")
	initialDelay := flag.Duration("initial-delay", 1*time.Second, "initial reconnect delay")
	maxDelay := flag.Duration("max-delay", 60*time.Second, "max reconnect delay")
	verbose := flag.Bool("v", false, "verbose logging")

	flag.Usage = func() {
		if defaultServerURL != "" {
			fmt.Fprintf(os.Stderr, "Usage: linkc [options] [server-url]\n\n")
			fmt.Fprintf(os.Stderr, "Default server: %s\n\n", defaultServerURL)
		} else {
			fmt.Fprintf(os.Stderr, "Usage: linkc [options] <server-url>\n\n")
		}
		fmt.Fprintf(os.Stderr, "Example: linkc ws://server:8080\n")
		fmt.Fprintf(os.Stderr, "         (will connect to ws://server:8080/sshlink/ws)\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	var serverURL string
	if flag.NArg() >= 1 {
		serverURL = flag.Arg(0) + "/sshlink/ws"
	} else if defaultServerURL != "" {
		serverURL = defaultServerURL + "/sshlink/ws"
	} else {
		flag.Usage()
		os.Exit(1)
	}
	common.SetVerbose(*verbose)

	// check SSH service
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

	// wait for signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	common.Info("shutting down...")
	c.Stop()
}

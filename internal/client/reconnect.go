package client

import "time"

// ReconnectConfig 重连配置
type ReconnectConfig struct {
	InitialDelay      time.Duration // 初始延迟，默认 1s
	MaxDelay          time.Duration // 最大延迟，默认 60s
	MaxRetries        int           // 最大重试次数，0=无限
	BackoffMultiplier float64       // 退避倍数，默认 2.0
}

// DefaultReconnectConfig 默认重连配置
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		InitialDelay:      1 * time.Second,
		MaxDelay:          60 * time.Second,
		MaxRetries:        0,
		BackoffMultiplier: 2.0,
	}
}

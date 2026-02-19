package client

import "time"

// ReconnectConfig holds reconnection parameters.
type ReconnectConfig struct {
	InitialDelay      time.Duration // initial delay, default 1s
	MaxDelay          time.Duration // max delay, default 60s
	MaxRetries        int           // max retries, 0=unlimited
	BackoffMultiplier float64       // backoff multiplier, default 2.0
}

// DefaultReconnectConfig returns the default reconnection configuration.
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		InitialDelay:      1 * time.Second,
		MaxDelay:          60 * time.Second,
		MaxRetries:        0,
		BackoffMultiplier: 2.0,
	}
}

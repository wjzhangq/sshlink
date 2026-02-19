package server

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/wjzhangq/sshlink/internal/common"
)

// ServerMetrics 服务端监控指标
type ServerMetrics struct {
	mu            sync.RWMutex
	startTime     time.Time
	activeClients atomic.Int64
	totalChannels atomic.Int64
	bytesReceived atomic.Uint64
	bytesSent     atomic.Uint64
	errors        atomic.Uint64
}

// NewServerMetrics 创建监控指标
func NewServerMetrics() *ServerMetrics {
	return &ServerMetrics{
		startTime: time.Now(),
	}
}

func (m *ServerMetrics) IncClients()  { m.activeClients.Add(1) }
func (m *ServerMetrics) DecClients()  { m.activeClients.Add(-1) }
func (m *ServerMetrics) IncChannels() { m.totalChannels.Add(1) }
func (m *ServerMetrics) DecChannels() { m.totalChannels.Add(-1) }
func (m *ServerMetrics) AddBytesReceived(n uint64) { m.bytesReceived.Add(n) }
func (m *ServerMetrics) AddBytesSent(n uint64)     { m.bytesSent.Add(n) }
func (m *ServerMetrics) IncErrors()                { m.errors.Add(1) }

// Snapshot 获取当前指标快照
func (m *ServerMetrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		ActiveClients: m.activeClients.Load(),
		TotalChannels: m.totalChannels.Load(),
		BytesReceived: m.bytesReceived.Load(),
		BytesSent:     m.bytesSent.Load(),
		Errors:        m.errors.Load(),
		Uptime:        time.Since(m.startTime).String(),
	}
}

// MetricsSnapshot 指标快照
type MetricsSnapshot struct {
	ActiveClients int64  `json:"active_clients"`
	TotalChannels int64  `json:"total_channels"`
	BytesReceived uint64 `json:"bytes_received"`
	BytesSent     uint64 `json:"bytes_sent"`
	Errors        uint64 `json:"errors"`
	Uptime        string `json:"uptime"`
}

// Report 输出指标日志
func (m *ServerMetrics) Report() {
	s := m.Snapshot()
	common.Info("metrics: clients=%d, channels=%d, rx=%d, tx=%d, errors=%d, uptime=%s",
		s.ActiveClients, s.TotalChannels, s.BytesReceived, s.BytesSent, s.Errors, s.Uptime)
}

// StartReporter 启动定期指标输出
func (m *ServerMetrics) StartReporter(interval time.Duration, stop <-chan struct{}) {
	common.SafeGoWithName("metrics-reporter", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.Report()
			case <-stop:
				return
			}
		}
	})
}

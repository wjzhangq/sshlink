package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wjzhangq/sshlink/internal/common"
)

// HealthServer 健康检查 HTTP 服务
type HealthServer struct {
	port      int
	clientMgr *ClientManager
	metrics   *ServerMetrics
}

// NewHealthServer 创建健康检查服务
func NewHealthServer(port int, clientMgr *ClientManager, metrics *ServerMetrics) *HealthServer {
	return &HealthServer{
		port:      port,
		clientMgr: clientMgr,
		metrics:   metrics,
	}
}

// Start 启动健康检查服务
func (h *HealthServer) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/metrics", h.handleMetrics)

	addr := fmt.Sprintf(":%d", h.port)
	srv := &http.Server{Addr: addr, Handler: mux}

	common.SafeGoWithName("health-server", func() {
		common.Info("health check server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			common.Error("health server error: %v", err)
		}
	})
}

func (h *HealthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	snap := h.metrics.Snapshot()
	resp := map[string]interface{}{
		"status":  "ok",
		"clients": h.clientMgr.Count(),
		"uptime":  snap.Uptime,
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *HealthServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.metrics.Snapshot())
}

package api

import (
	"net/http"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

// Health returns a simple readiness payload.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := vm.WithClusterID(r.Context(), clusterIDFromRequest(r))
	meta, err := h.vm.ClusterMetadata(ctx)
	timestamp := meta.Timestamp
	status := "ok"

	switch err {
	case nil:
		agentStatus, statusErr := h.vm.AgentStatus(ctx)
		if statusErr != nil {
			status = "degraded"
		} else if agentStatus.Status != "connected" {
			status = "degraded"
		}
	case vm.ErrNoData:
		status = "initializing"
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":        status,
		"clusterId":     meta.ID,
		"clusterName":   meta.Name,
		"clusterType":   meta.Type,
		"clusterRegion": meta.Region,
		"version":       meta.Version,
		"timestamp":     timestamp,
	})
}

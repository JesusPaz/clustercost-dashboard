package api

import (
	"net/http"

	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

// AgentStatus returns the aggregated agent connection status.
func (h *Handler) AgentStatus(w http.ResponseWriter, r *http.Request) {
	ctx := vm.WithClusterID(r.Context(), clusterIDFromRequest(r))
	status, err := h.vm.AgentStatus(ctx)
	if err != nil {
		if err == vm.ErrNoData {
			writeError(w, http.StatusServiceUnavailable, "agent data not yet available")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

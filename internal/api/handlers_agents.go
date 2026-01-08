package api

import (
	"net/http"

	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

// Agents returns configured agents and their last known status.
func (h *Handler) Agents(w http.ResponseWriter, r *http.Request) {
	ctx := vm.WithClusterID(r.Context(), clusterIDFromRequest(r))
	agents, err := h.vm.Agents(ctx)
	if err != nil {
		if err == vm.ErrNoData {
			writeError(w, http.StatusServiceUnavailable, "agent data not yet available")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

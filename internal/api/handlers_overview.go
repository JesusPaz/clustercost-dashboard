package api

import (
	"net/http"

	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

// Overview serves the aggregated overview payload.
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limitTopNamespaces"), 5, 20)

	ctx := vm.WithClusterID(r.Context(), clusterIDFromRequest(r))
	overview, err := h.vm.Overview(ctx, limit)
	if err != nil {
		if err == vm.ErrNoData {
			writeError(w, http.StatusServiceUnavailable, "data not yet available")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

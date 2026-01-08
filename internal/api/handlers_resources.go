package api

import (
	"net/http"

	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

// Resources exposes cluster-wide efficiency metrics.
func (h *Handler) Resources(w http.ResponseWriter, r *http.Request) {
	ctx := vm.WithClusterID(r.Context(), clusterIDFromRequest(r))
	resp, err := h.vm.Resources(ctx)
	if err != nil {
		if err == vm.ErrNoData {
			writeError(w, http.StatusServiceUnavailable, "data not yet available")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

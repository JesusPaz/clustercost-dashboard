package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/clustercost/clustercost-dashboard/internal/store"
	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

const (
	defaultNodeLimit = 100
	maxNodeLimit     = 500
)

// Nodes returns node utilization information.
func (h *Handler) Nodes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctx := vm.WithClusterID(r.Context(), clusterIDFromRequest(r))
	filter := store.NodeFilter{
		Search: q.Get("search"),
		Limit:  parseLimit(q.Get("limit"), defaultNodeLimit, maxNodeLimit),
		Offset: parseOffset(q.Get("offset")),
	}

	resp, err := h.vm.NodeList(ctx, filter)
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

// NodeDetail returns a single node entry.
func (h *Handler) NodeDetail(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "node name is required")
		return
	}

	ctx := vm.WithClusterID(r.Context(), clusterIDFromRequest(r))
	node, err := h.vm.NodeDetail(ctx, name)
	if err != nil {
		if err == vm.ErrNoData {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, node)
}

package api

import (
	"net/http"
	"time"

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
		Window: q.Get("window"), // "24h", "7d", "30d"
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

// NodeStats returns historical usage and cost stats for a node.
func (h *Handler) NodeStats(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "node name is required")
		return
	}
	windowStr := r.URL.Query().Get("window")
	window, _ := time.ParseDuration(windowStr)
	if window <= 0 {
		window = 24 * time.Hour
	}

	ctx := vm.WithClusterID(r.Context(), clusterIDFromRequest(r))
	stats, err := h.vm.GetNodeStats(ctx, "", name, window)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// NodePods returns the list of pods for a node with P95 metrics (Pod Audit).
func (h *Handler) NodePods(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "node name is required")
		return
	}
	windowStr := r.URL.Query().Get("window")
	window, _ := time.ParseDuration(windowStr)
	if window <= 0 {
		window = 24 * time.Hour
	}

	ctx := vm.WithClusterID(r.Context(), clusterIDFromRequest(r))
	pods, err := h.vm.GetNodePods(ctx, "", name, window)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, pods)
}

package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/clustercost/clustercost-dashboard/internal/store"
	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

const (
	defaultNamespaceLimit = 50
	maxNamespaceLimit     = 200
)

// Namespaces exposes namespace level cost metrics with filtering and pagination.
func (h *Handler) Namespaces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctx := vm.WithClusterID(r.Context(), clusterIDFromRequest(r))

	filter := store.NamespaceFilter{
		Environment: q.Get("environment"),
		Search:      q.Get("search"),
		Limit:       parseLimit(q.Get("limit"), defaultNamespaceLimit, maxNamespaceLimit),
		Offset:      parseOffset(q.Get("offset")),
	}

	resp, err := h.vm.NamespaceList(ctx, filter)
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

// NamespaceDetail returns a single namespace entry.
func (h *Handler) NamespaceDetail(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "namespace is required")
		return
	}

	ctx := vm.WithClusterID(r.Context(), clusterIDFromRequest(r))
	ns, err := h.vm.NamespaceDetail(ctx, name)
	if err != nil {
		if err == vm.ErrNoData {
			writeError(w, http.StatusNotFound, "namespace not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ns)
}

package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/store"
	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

type NetworkTopologyResponse struct {
	ClusterID      string              `json:"clusterId"`
	Namespace      string              `json:"namespace,omitempty"`
	Start          time.Time           `json:"start"`
	End            time.Time           `json:"end"`
	Edges          []store.NetworkEdge `json:"edges"`
	TotalEdges     int                 `json:"totalEdges"`
	RequestedLimit int                 `json:"requestedLimit"`
	Timestamp      time.Time           `json:"timestamp"`
}

func (h *Handler) NetworkTopology(w http.ResponseWriter, r *http.Request) {
	clusterID := clusterIDFromRequest(r)
	namespace := r.URL.Query().Get("namespace")
	limit := parseLimit(r.URL.Query().Get("limit"), 2000, 10000)

	start, end, err := parseTimeRange(r, 1*time.Hour)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid time range")
		return
	}
	if start.IsZero() || end.IsZero() {
		writeError(w, http.StatusBadRequest, "time range required")
		return
	}

	edges, err := h.vm.NetworkTopology(r.Context(), store.NetworkTopologyOptions{
		ClusterID: clusterID,
		Namespace: namespace,
		Start:     start,
		End:       end,
		Limit:     limit,
	})
	if err != nil {
		if errors.Is(err, vm.ErrNoData) {
			writeJSON(w, http.StatusOK, NetworkTopologyResponse{
				ClusterID:      clusterID,
				Namespace:      namespace,
				Start:          start,
				End:            end,
				Edges:          []store.NetworkEdge{},
				TotalEdges:     0,
				RequestedLimit: limit,
				Timestamp:      time.Now().UTC(),
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to query network topology")
		return
	}

	writeJSON(w, http.StatusOK, NetworkTopologyResponse{
		ClusterID:      clusterID,
		Namespace:      namespace,
		Start:          start,
		End:            end,
		Edges:          edges,
		TotalEdges:     len(edges),
		RequestedLimit: limit,
		Timestamp:      time.Now().UTC(),
	})
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/clustercost/clustercost-dashboard/internal/auth"
	"github.com/clustercost/clustercost-dashboard/internal/db"
	"github.com/clustercost/clustercost-dashboard/internal/finops"
	"github.com/clustercost/clustercost-dashboard/internal/static"
	"github.com/clustercost/clustercost-dashboard/internal/store"
)

// MetricsProvider defines the data backend used by API handlers.
type MetricsProvider interface {
	Overview(ctx context.Context, limit int) (store.OverviewPayload, error)
	NamespaceList(ctx context.Context, filter store.NamespaceFilter) (store.NamespaceListResponse, error)
	NamespaceDetail(ctx context.Context, name string) (store.NamespaceSummary, error)
	NodeList(ctx context.Context, filter store.NodeFilter) (store.NodeListResponse, error)
	NodeDetail(ctx context.Context, name string) (store.NodeSummary, error)
	Resources(ctx context.Context) (store.ResourcesPayload, error)
	AgentStatus(ctx context.Context) (store.AgentStatusPayload, error)
	Agents(ctx context.Context) ([]store.AgentInfo, error)
	ClusterMetadata(ctx context.Context) (store.ClusterMetadata, error)
	NetworkTopology(ctx context.Context, opts store.NetworkTopologyOptions) ([]store.NetworkEdge, error)
	GetNodeStats(ctx context.Context, clusterID, nodeName string, window time.Duration) (store.NodeStats, error)
	GetNodePods(ctx context.Context, clusterID, nodeName string, window time.Duration) ([]store.PodMetrics, error)
}

// Handler wires HTTP requests to the VictoriaMetrics client.
type Handler struct {
	vm     MetricsProvider
	db     *db.Store
	store  *store.Store
	finops *finops.Engine
}

// NewRouter builds the HTTP router serving both JSON APIs and static assets.
func NewRouter(vmClient MetricsProvider, db *db.Store, st *store.Store, finopsEngine *finops.Engine) http.Handler {
	h := &Handler{
		vm:     vmClient,
		db:     db,
		store:  st,
		finops: finopsEngine,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions}, // Added POST for Login
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Route("/api", func(api chi.Router) {
		// Public routes
		api.Get("/health", h.Health)
		api.Post("/login", h.Login)

		// Protected routes
		api.Group(func(protected chi.Router) {
			protected.Use(auth.Middleware)
			protected.Route("/cost", func(cost chi.Router) {
				cost.Get("/overview", h.Overview)
				cost.Get("/namespaces", h.Namespaces)
				cost.Get("/namespaces/{name}", h.NamespaceDetail)
				cost.Get("/nodes", h.Nodes)
				cost.Get("/nodes/{name}", h.NodeDetail)
				cost.Get("/nodes/{name}/stats", h.NodeStats)
				cost.Get("/nodes/{name}/pods", h.NodePods)
				cost.Get("/resources", h.Resources)
			})
			protected.Get("/agent", h.AgentStatus)
			protected.Get("/agents", h.Agents)

			protected.Route("/finops", func(finops chi.Router) {
				finops.Get("/efficiency", h.EfficiencyReport)
			})

			protected.Route("/network", func(network chi.Router) {
				network.Get("/topology", h.NetworkTopology)
			})
		})
	})

	r.Handle("/*", static.Handler())
	r.Handle("/", static.Handler())

	return r
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

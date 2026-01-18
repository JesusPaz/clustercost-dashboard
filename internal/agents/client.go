package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"
)

// Client talks to ClusterCost agents over HTTP.
type Client struct {
	httpClient *http.Client
}

// NewClient builds a client with a configurable timeout.
func NewClient(timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Client{httpClient: &http.Client{Timeout: timeout}}
}

func (c *Client) FetchHealth(ctx context.Context, baseURL string) (HealthResponse, error) {
	var resp HealthResponse
	if err := c.get(ctx, baseURL, "/agent/v1/health", &resp); err != nil {
		return HealthResponse{}, err
	}
	return resp, nil
}

func (c *Client) FetchNamespaces(ctx context.Context, baseURL string) (NamespacesResponse, error) {
	var resp NamespacesResponse
	if err := c.get(ctx, baseURL, "/agent/v1/namespaces", &resp); err != nil {
		return NamespacesResponse{}, err
	}
	return resp, nil
}

func (c *Client) FetchNodes(ctx context.Context, baseURL string) (NodesResponse, error) {
	var resp NodesResponse
	if err := c.get(ctx, baseURL, "/agent/v1/nodes", &resp); err != nil {
		return NodesResponse{}, err
	}
	return resp, nil
}

func (c *Client) FetchResources(ctx context.Context, baseURL string) (ResourcesResponse, error) {
	var resp ResourcesResponse
	if err := c.get(ctx, baseURL, "/agent/v1/resources", &resp); err != nil {
		return ResourcesResponse{}, err
	}
	return resp, nil
}

func (c *Client) get(ctx context.Context, baseURL, endpoint string, target any) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid agent URL %s: %w", baseURL, err)
	}
	u.Path = path.Join(u.Path, endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call agent: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("agent responded with status %d", res.StatusCode)
	}

	dec := json.NewDecoder(res.Body)
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

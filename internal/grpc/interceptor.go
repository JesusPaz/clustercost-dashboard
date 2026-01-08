package grpc

import (
	"context"
	"strings"

	"github.com/clustercost/clustercost-dashboard/internal/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthInterceptor validates the authorization token in the context metadata.
type AuthInterceptor struct {
	validTokens  map[string]string // map[token]agentName
	defaultToken string
}

// NewAuthInterceptor creates a new interceptor with the valid tokens from config.
func NewAuthInterceptor(agents []config.AgentConfig, defaultToken string) *AuthInterceptor {
	validTokens := make(map[string]string)
	for _, agent := range agents {
		if agent.Token != "" {
			validTokens[agent.Token] = agent.Name
		}
	}
	return &AuthInterceptor{
		validTokens:  validTokens,
		defaultToken: defaultToken,
	}
}

// Unary returns a UnaryServerInterceptor that validates the token.
func (i *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if len(i.validTokens) == 0 && i.defaultToken == "" {
			// If no tokens are configured, allow unauthenticated access.
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "metadata is not provided")
		}

		values := md["authorization"]
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, "authorization token is not provided")
		}

		accessToken := strings.TrimPrefix(values[0], "Bearer ")

		// Check default token first
		if i.defaultToken != "" && accessToken == i.defaultToken {
			// If default token is used, we might not know the agent name yet.
			// It will be extracted from the request body in the handler.
			return handler(ctx, req)
		}

		// Check specific agent tokens
		agentName, ok := i.validTokens[accessToken]
		if ok {
			// Inject agent name into context
			newCtx := context.WithValue(ctx, "agent_name", agentName)
			return handler(newCtx, req)
		}

		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
}

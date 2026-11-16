package integration

import (
	"context"
	"encoding/json"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	mcpCallsShown = 100
	mcpCallsKept  = 30 * 24 * time.Hour
)

// LogMCPCalls keeps what an assistant did; the MCP adapter calls it after each tool call.
func (s *Service) LogMCPCalls(ctx context.Context, calls []domain.MCPCall) error {
	return s.Store.Integrations().LogMCPCalls(ctx, calls)
}

func (s *Service) MCPCalls(ctx context.Context, collectionID string) ([]domain.MCPCall, error) {
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	return s.Store.Integrations().MCPCalls(ctx, collectionID, mcpCallsShown)
}

func (s *Service) KeyMCPCalls(ctx context.Context, keyID string) ([]domain.MCPCall, error) {
	if err := validate.ID(keyID); err != nil {
		return nil, err
	}
	return s.Store.Integrations().KeyMCPCalls(ctx, keyID, reqctx.Actor(ctx), mcpCallsShown)
}

func (s *Service) PruneMCPCalls(ctx context.Context) error {
	return s.Store.Integrations().PruneMCPCalls(ctx, time.Now().Add(-mcpCallsKept))
}

// TestMCPTool lets an owner try a tool before saving it: the same checks as a
// save, then one run with args, as the owner.
func (s *Service) TestMCPTool(ctx context.Context, collectionID string, in input.MCPToolInput, args map[string]json.RawMessage) ([]domain.Record, int64, int, error) {
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, 0, 0, err
	}
	if _, byKey := reqctx.Key(ctx); byKey {
		return nil, 0, 0, apperror.New(apperror.CodeForbidden, "an API key cannot test MCP tools")
	}
	t, err := s.checkMCPTool(ctx, collectionID, in)
	if err != nil {
		return nil, 0, 0, err
	}
	return s.RunMCPTool(ctx, *t, args)
}

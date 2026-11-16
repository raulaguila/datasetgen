package mcp

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

const maxCallError = 200

type touchedKey struct{}

// touched gathers the collections one tool call acted in, for the owners' log.
type touched struct{ ids []string }

// touch marks a collection the running tool call acted in.
func touch(ctx context.Context, collectionID string) {
	if t, ok := ctx.Value(touchedKey{}).(*touched); ok && collectionID != "" {
		t.ids = append(t.ids, collectionID)
	}
}

// logCalls keeps each tool call in the log of every collection it acted in: a
// collection named in the arguments (when the key reaches it) or one a tool marked.
// shortcut: calls that name no collection (list_collections, delete_records) go unlogged; deletes are in the audit log.
func (t tools) logCalls(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		call, isCall := req.(*mcp.CallToolRequest)
		if method != "tools/call" || !isCall || call.Params == nil {
			return next(ctx, method, req)
		}
		key, _ := reqctx.Key(ctx)
		tc := &touched{}
		ctx = context.WithValue(ctx, touchedKey{}, tc)
		var args struct {
			CollectionID string `json:"collection_id"`
		}
		if json.Unmarshal(call.Params.Arguments, &args) == nil {
			if _, ok := key.Scopes[args.CollectionID]; ok {
				touch(ctx, args.CollectionID)
			}
		}

		res, err := next(ctx, method, req)

		ok, msg := err == nil, ""
		if err != nil {
			msg = err.Error()
		} else if r, isResult := res.(*mcp.CallToolResult); isResult && r.IsError {
			ok = false
			if len(r.Content) > 0 {
				if text, isText := r.Content[0].(*mcp.TextContent); isText {
					msg = text.Text
				}
			}
		}
		if len(msg) > maxCallError {
			msg = msg[:maxCallError]
		}
		slices.Sort(tc.ids)
		calls := make([]domain.MCPCall, 0, len(tc.ids))
		for _, id := range slices.Compact(tc.ids) {
			calls = append(calls, domain.MCPCall{CollectionID: id, KeyID: key.ID, KeyName: key.Name, UserID: reqctx.Actor(ctx),
				Tool: call.Params.Name, OK: ok, Error: msg})
		}
		// The answer goes out whatever the log does: it only shows owners what happened.
		_ = t.app.Integrations.LogMCPCalls(context.WithoutCancel(ctx), calls)
		return res, err
	}
}

package mcp

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// addOwnTools adds the tools the collections' owners published for this key.
// A failed listing leaves only the built-in tools: they still answer.
// shortcut: one query per MCP request; cache per key if tool listing shows up in load.
func (t tools) addOwnTools(ctx context.Context, s *mcp.Server) {
	own, err := t.app.Integrations.KeyMCPTools(ctx)
	if err != nil {
		return
	}
	for _, tool := range own {
		s.AddTool(&mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description + "\n(Collection: " + tool.CollectionName + ". Read-only; returns up to the page size with the total.)",
			InputSchema: inputSchema(tool),
			Annotations: readOnly,
		}, t.runOwnTool(tool))
	}
}

// inputSchema is the JSON Schema of a tool's arguments, typed from its fields.
func inputSchema(t domain.MCPTool) map[string]any {
	props := map[string]any{"page": map[string]any{"type": "integer", "minimum": 1, "description": "page number, from 1"}}
	required := []string{}
	for _, p := range t.Params {
		prop := map[string]any{"type": p.Type}
		if p.Op == "in" {
			prop = map[string]any{"type": "array", "items": prop, "minItems": 1, "maxItems": 100}
		}
		if p.Description != "" {
			prop["description"] = p.Description
		}
		props[p.Name] = prop
		if p.Required {
			required = append(required, p.Name)
		}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func (t tools) runOwnTool(tool domain.MCPTool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]json.RawMessage
		if raw := req.Params.Arguments; len(raw) > 0 {
			if err := json.Unmarshal(raw, &args); err != nil {
				return failed("arguments must be a JSON object"), nil
			}
		}
		items, total, page, err := t.app.Integrations.RunMCPTool(ctx, tool, args)
		if err != nil {
			return failed(err.Error()), nil
		}
		out := map[string]any{"total": total, "page": page, "records": recordsOf(items)}
		text, _ := json.Marshal(out)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}, StructuredContent: out}, nil
	}
}

func failed(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

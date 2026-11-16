package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/schema"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	maxMCPTools     = 20
	maxToolParams   = 10
	maxToolFields   = 50
	maxToolDescribe = 1000
	maxToolRecords  = 50 // per call, as search_records
	// PageArg is the argument every owner's tool takes for the next page.
	PageArg = "page"
)

var (
	toolName  = regexp.MustCompile(`^[a-z][a-z0-9_]{2,47}$`)
	paramName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
)

func (s *Service) MCPTools(ctx context.Context, collectionID string) ([]domain.MCPTool, error) {
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	return s.Store.Integrations().MCPTools(ctx, collectionID)
}

// checkMCPTool validates the settings against the collection's schema and tries
// the fixed conditions and sort once, so a bad tool fails here and not when an
// assistant calls it.
func (s *Service) checkMCPTool(ctx context.Context, collectionID string, in input.MCPToolInput) (*domain.MCPTool, error) {
	bad := func(field, msg string) error { return apperror.New(apperror.CodeInvalidInput, msg).WithField(field) }
	t := &domain.MCPTool{
		CollectionID: collectionID, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), Enabled: in.Enabled,
		Conditions: in.Conditions, Sort: in.Sort, Order: in.Order, Limit: in.Limit,
	}
	if !toolName.MatchString(t.Name) || slices.Contains(domain.MCPBuiltinTools, t.Name) {
		return nil, bad("name", "name must be 3-48 of a-z, 0-9 and _, start with a letter, and not be a built-in tool")
	}
	if n := utf8.RuneCountInString(t.Description); n < 1 || n > maxToolDescribe {
		return nil, bad("description", "description must be 1-1000 characters: it is what the assistant reads to pick the tool")
	}
	if t.Conditions.Rules == nil {
		t.Conditions.Rules = []domain.Condition{}
	}
	if t.Conditions.Any && len(t.Conditions.Groups) > 0 {
		return nil, bad("conditions", "fixed conditions with groups must be joined by AND")
	}
	if t.Sort == "" {
		t.Sort = "updated_at"
	}
	if t.Order == "" {
		t.Order = "desc"
	}
	if t.Order != "asc" && t.Order != "desc" {
		return nil, bad("order", "order must be asc or desc")
	}
	if t.Limit == 0 {
		t.Limit = 20
	}
	if t.Limit < 1 || t.Limit > maxToolRecords {
		return nil, bad("limit", "limit must be 1 to 50")
	}
	c, err := s.Store.Collections().FindByID(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	if len(in.Params) > maxToolParams {
		return nil, bad("params", "a tool takes up to 10 parameters")
	}
	seen := map[string]bool{}
	for _, p := range in.Params {
		p.Name, p.Description = strings.TrimSpace(p.Name), strings.TrimSpace(p.Description)
		if !paramName.MatchString(p.Name) || p.Name == PageArg || seen[p.Name] {
			return nil, bad("params", fmt.Sprintf("parameter %q: names are 1-40 of a-z, 0-9 and _, unique, and not page", p.Name))
		}
		seen[p.Name] = true
		if utf8.RuneCountInString(p.Description) > 200 {
			return nil, bad("params", fmt.Sprintf("parameter %s: description up to 200 characters", p.Name))
		}
		if !slices.Contains(domain.MCPToolOps, p.Op) {
			return nil, bad("params", fmt.Sprintf("parameter %s: op must be one of %s", p.Name, strings.Join(domain.MCPToolOps, ", ")))
		}
		typ, list, ok := schema.ScalarType(c.Schema, p.Field)
		if !ok || list != (p.Op == "has") || (p.Op == "contains" && typ != "string") {
			return nil, bad("params", fmt.Sprintf("parameter %s: %s cannot be compared with %s", p.Name, p.Field, p.Op))
		}
		p.Type = typ
		t.Params = append(t.Params, p)
	}
	props := schema.PropertyOrder(c.Schema)
	if f, isField := strings.CutPrefix(t.Sort, "data."); (isField && !slices.Contains(props, f)) || (!isField && t.Sort != "updated_at" && t.Sort != "created_at") {
		return nil, bad("sort", "sort must be created_at, updated_at or data.<field>")
	}
	if len(in.Fields) > maxToolFields {
		return nil, bad("fields", "a tool returns up to 50 fields")
	}
	for _, f := range in.Fields {
		if !slices.Contains(props, f) {
			return nil, bad("fields", fmt.Sprintf("%s is not a field of the collection", f))
		}
		if !slices.Contains(t.Fields, f) {
			t.Fields = append(t.Fields, f)
		}
	}
	if _, _, err := s.Records.List(ctx, domain.RecordFilter{CollectionID: collectionID, Page: domain.NewPage(1, 1), Where: &t.Conditions, Sort: t.Sort, Order: t.Order}); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) CreateMCPTool(ctx context.Context, collectionID string, in input.MCPToolInput) (*domain.MCPTool, error) {
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	if _, byKey := reqctx.Key(ctx); byKey {
		return nil, apperror.New(apperror.CodeForbidden, "an API key cannot make MCP tools")
	}
	t, err := s.checkMCPTool(ctx, collectionID, in)
	if err != nil {
		return nil, err
	}
	t.CreatedBy = reqctx.Actor(ctx)
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		existing, err := uow.Integrations().MCPTools(ctx, collectionID)
		if err != nil {
			return err
		}
		if len(existing) >= maxMCPTools {
			return apperror.New(apperror.CodeInvalidInput, "a collection holds up to 20 MCP tools; delete one first")
		}
		if err := uow.Integrations().CreateMCPTool(ctx, t); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.mcp_tool_create", collectionID, map[string]any{"collection_id": collectionID, "tool": t.Name})
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) UpdateMCPTool(ctx context.Context, collectionID, id string, in input.MCPToolInput) (*domain.MCPTool, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	if _, byKey := reqctx.Key(ctx); byKey {
		return nil, apperror.New(apperror.CodeForbidden, "an API key cannot change MCP tools")
	}
	t, err := s.checkMCPTool(ctx, collectionID, in)
	if err != nil {
		return nil, err
	}
	t.ID = id
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := uow.Integrations().UpdateMCPTool(ctx, t); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.mcp_tool_update", collectionID, map[string]any{"collection_id": collectionID, "tool": t.Name})
	})
	if err != nil {
		return nil, err
	}
	return s.Store.Integrations().MCPTool(ctx, id)
}

func (s *Service) DeleteMCPTool(ctx context.Context, collectionID, id string) error {
	if err := validate.ID(id); err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := s.owner(ctx, uow, collectionID); err != nil {
			return err
		}
		name, err := uow.Integrations().DeleteMCPTool(ctx, collectionID, id)
		if err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.mcp_tool_delete", collectionID, map[string]any{"collection_id": collectionID, "tool": name})
	})
}

// KeyMCPTools lists the tools the calling API key sees: the enabled ones of
// the collections it reaches. A name two collections share goes to the older tool.
func (s *Service) KeyMCPTools(ctx context.Context) ([]domain.MCPTool, error) {
	key, ok := reqctx.Key(ctx)
	if !ok {
		return nil, nil
	}
	ids := make([]string, 0, len(key.Scopes))
	for id := range key.Scopes {
		ids = append(ids, id)
	}
	all, err := s.Store.Integrations().KeyMCPTools(ctx, ids)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := all[:0]
	for _, t := range all {
		if !seen[t.Name] {
			seen[t.Name] = true
			out = append(out, t)
		}
	}
	return out, nil
}

// RunMCPTool searches with the tool's fixed conditions and one rule per given
// argument, all joined by AND, and keeps only the tool's fields of each record.
// It runs as the key's member, so the key scope and member role apply.
func (s *Service) RunMCPTool(ctx context.Context, t domain.MCPTool, args map[string]json.RawMessage) ([]domain.Record, int64, int, error) {
	where, err := toolWhere(t, args)
	if err != nil {
		return nil, 0, 0, err
	}
	page := 1
	if raw, ok := args[PageArg]; ok && string(raw) != "null" {
		if json.Unmarshal(raw, &page) != nil || page < 1 {
			return nil, 0, 0, apperror.New(apperror.CodeInvalidInput, "page must be a whole number from 1").WithField(PageArg)
		}
	}
	filter := domain.RecordFilter{CollectionID: t.CollectionID, Where: where, Sort: t.Sort, Order: t.Order, Page: domain.NewPage(page, t.Limit)}
	items, total, err := s.Records.List(ctx, filter)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(t.Fields) > 0 {
		for i := range items {
			var data map[string]json.RawMessage
			if json.Unmarshal(items[i].Data, &data) == nil {
				kept := make(map[string]json.RawMessage, len(t.Fields))
				for _, f := range t.Fields {
					if v, ok := data[f]; ok {
						kept[f] = v
					}
				}
				items[i].Data, _ = json.Marshal(kept)
			}
		}
	}
	return items, total, filter.Page.Number, nil
}

// toolWhere joins the fixed conditions with the arguments' rules by AND.
func toolWhere(t domain.MCPTool, args map[string]json.RawMessage) (*domain.Where, error) {
	var rules []domain.Condition
	for _, p := range t.Params {
		raw, ok := args[p.Name]
		if !ok || string(bytes.TrimSpace(raw)) == "null" {
			if p.Required {
				return nil, apperror.New(apperror.CodeInvalidInput, p.Name+" is required").WithField(p.Name)
			}
			continue
		}
		if !argFits(raw, p.Type, p.Op == "in") {
			want := p.Type
			if p.Op == "in" {
				want = "a list of " + p.Type
			}
			return nil, apperror.New(apperror.CodeInvalidInput, p.Name+" must be "+want).WithField(p.Name)
		}
		rules = append(rules, domain.Condition{Field: p.Field, Op: p.Op, Value: raw})
	}
	fixed := t.Conditions
	switch {
	case fixed.Empty():
		if len(rules) == 0 {
			return nil, nil
		}
		return &domain.Where{Rules: rules}, nil
	case !fixed.Any:
		return &domain.Where{Rules: append(slices.Clone(fixed.Rules), rules...), Groups: fixed.Groups}, nil
	default: // OR-joined rules, no groups (checked on save): they become one group
		return &domain.Where{Rules: rules, Groups: []domain.Where{{Any: true, Rules: fixed.Rules}}}, nil
	}
}

func argFits(raw json.RawMessage, typ string, list bool) bool {
	if list {
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil || len(items) == 0 || len(items) > 100 {
			return false
		}
		for _, it := range items {
			if !argFits(it, typ, false) {
				return false
			}
		}
		return true
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&v) != nil {
		return false
	}
	switch typ {
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "integer":
		n, ok := v.(json.Number)
		_, err := n.Int64()
		return ok && err == nil
	case "number":
		_, ok := v.(json.Number)
		return ok
	}
	return false
}

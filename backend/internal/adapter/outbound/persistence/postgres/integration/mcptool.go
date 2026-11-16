package integration

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
)

type mcpToolRow struct {
	ID             uuid.UUID `gorm:"column:id"`
	CollectionID   uuid.UUID `gorm:"column:collection_id"`
	CollectionName string    `gorm:"column:collection_name"`
	Name           string    `gorm:"column:name"`
	Description    string    `gorm:"column:description"`
	Enabled        bool      `gorm:"column:enabled"`
	Conditions     []byte    `gorm:"column:conditions"`
	Params         []byte    `gorm:"column:params"`
	Fields         []byte    `gorm:"column:fields"`
	Sort           string    `gorm:"column:sort"`
	Order          string    `gorm:"column:sort_order"`
	Limit          int       `gorm:"column:max_records"`
	CreatedBy      uuid.UUID `gorm:"column:created_by"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (m mcpToolRow) toDomain() domain.MCPTool {
	t := domain.MCPTool{
		ID: m.ID.String(), CollectionID: m.CollectionID.String(), CollectionName: m.CollectionName, Name: m.Name, Description: m.Description,
		Enabled: m.Enabled, Sort: m.Sort, Order: m.Order, Limit: m.Limit, CreatedBy: m.CreatedBy.String(), CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	_ = json.Unmarshal(m.Conditions, &t.Conditions)
	_ = json.Unmarshal(m.Params, &t.Params)
	_ = json.Unmarshal(m.Fields, &t.Fields)
	return t
}

const mcpToolColumns = "t.id, t.collection_id, t.name, t.description, t.enabled, t.conditions, t.params, t.fields, t.sort, t.sort_order, t.max_records, t.created_by, t.created_at, t.updated_at"

func toolRows(rows []mcpToolRow) []domain.MCPTool {
	out := make([]domain.MCPTool, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out
}

// MCPTools lists at most 50: the use case caps a collection at 20.
func (r *Repository) MCPTools(ctx context.Context, collectionID string) ([]domain.MCPTool, error) {
	var rows []mcpToolRow
	err := r.db.WithContext(ctx).Table("app.mcp_tool t").Select(mcpToolColumns).
		Where("t.collection_id = ?", collectionID).Order("t.name").Limit(50).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "MCP tool")
	}
	return toolRows(rows), nil
}

func (r *Repository) MCPTool(ctx context.Context, id string) (*domain.MCPTool, error) {
	var m mcpToolRow
	if err := r.db.WithContext(ctx).Table("app.mcp_tool t").Select(mcpToolColumns).Where("t.id = ?", id).Take(&m).Error; err != nil {
		return nil, shared.WrapError(err, "MCP tool")
	}
	t := m.toDomain()
	return &t, nil
}

// KeyMCPTools lists the enabled tools of live collections, oldest first.
// shortcut: at most 200 tools per key (a key reaches 50 collections of up to 20); past that the newest are left out.
func (r *Repository) KeyMCPTools(ctx context.Context, collectionIDs []string) ([]domain.MCPTool, error) {
	if len(collectionIDs) == 0 {
		return nil, nil
	}
	var rows []mcpToolRow
	err := r.db.WithContext(ctx).Table("app.mcp_tool t").Select(mcpToolColumns+", c.name AS collection_name").
		Joins("JOIN app.collection c ON c.id = t.collection_id AND c.deleted_at IS NULL").
		Where("t.enabled AND t.collection_id = ANY(string_to_array(?, ',')::uuid[])", strings.Join(collectionIDs, ",")).
		Order("t.created_at").Limit(200).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "MCP tool")
	}
	return toolRows(rows), nil
}

func mcpToolJSON(t *domain.MCPTool) (cond, params, fields string, err error) {
	if t.Params == nil {
		t.Params = []domain.MCPToolParam{}
	}
	if t.Fields == nil {
		t.Fields = []string{}
	}
	var b [3][]byte
	for i, v := range []any{t.Conditions, t.Params, t.Fields} {
		if b[i], err = json.Marshal(v); err != nil {
			return
		}
	}
	return string(b[0]), string(b[1]), string(b[2]), nil
}

func (r *Repository) CreateMCPTool(ctx context.Context, t *domain.MCPTool) error {
	cond, params, fields, err := mcpToolJSON(t)
	if err != nil {
		return err
	}
	var m mcpToolRow
	err = r.db.WithContext(ctx).Raw(`
		INSERT INTO app.mcp_tool (collection_id, name, description, enabled, conditions, params, fields, sort, sort_order, max_records, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id, created_at, updated_at`,
		t.CollectionID, t.Name, t.Description, t.Enabled, cond, params, fields, t.Sort, t.Order, t.Limit, t.CreatedBy).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "MCP tool")
	}
	t.ID, t.CreatedAt, t.UpdatedAt = m.ID.String(), m.CreatedAt, m.UpdatedAt
	return nil
}

func (r *Repository) UpdateMCPTool(ctx context.Context, t *domain.MCPTool) error {
	cond, params, fields, err := mcpToolJSON(t)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Exec(`
		UPDATE app.mcp_tool SET name = ?, description = ?, enabled = ?, conditions = ?, params = ?, fields = ?, sort = ?, sort_order = ?, max_records = ?, updated_at = now()
		WHERE id = ? AND collection_id = ?`,
		t.Name, t.Description, t.Enabled, cond, params, fields, t.Sort, t.Order, t.Limit, t.ID, t.CollectionID)
	if res.Error != nil {
		return shared.WrapError(res.Error, "MCP tool")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "MCP tool")
	}
	return nil
}

func (r *Repository) DeleteMCPTool(ctx context.Context, collectionID, id string) (string, error) {
	var names []string
	err := r.db.WithContext(ctx).Raw(`DELETE FROM app.mcp_tool WHERE id = ? AND collection_id = ? RETURNING name`, id, collectionID).Find(&names).Error
	if err != nil {
		return "", shared.WrapError(err, "MCP tool")
	}
	if len(names) == 0 {
		return "", shared.WrapError(gorm.ErrRecordNotFound, "MCP tool")
	}
	return names[0], nil
}

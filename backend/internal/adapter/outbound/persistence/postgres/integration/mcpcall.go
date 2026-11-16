package integration

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
)

type mcpCallIn struct {
	CollectionID string  `json:"collection_id"`
	KeyID        *string `json:"key_id"`
	KeyName      string  `json:"key_name"`
	UserID       string  `json:"user_id"`
	Tool         string  `json:"tool"`
	OK           bool    `json:"ok"`
	Error        string  `json:"error"`
}

// LogMCPCalls inserts the calls in one statement. A key deleted meanwhile
// leaves key_id empty instead of failing the log.
func (r *Repository) LogMCPCalls(ctx context.Context, calls []domain.MCPCall) error {
	if len(calls) == 0 {
		return nil
	}
	rows := make([]mcpCallIn, len(calls))
	for i, c := range calls {
		rows[i] = mcpCallIn{CollectionID: c.CollectionID, KeyName: c.KeyName, UserID: c.UserID, Tool: c.Tool, OK: c.OK, Error: c.Error}
		if c.KeyID != "" {
			rows[i].KeyID = &c.KeyID
		}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return shared.WrapError(r.db.WithContext(ctx).Exec(`
		INSERT INTO app.mcp_call (collection_id, key_id, key_name, user_id, tool, ok, error)
		SELECT x.collection_id, (SELECT k.id FROM app.api_key k WHERE k.id = x.key_id), x.key_name, x.user_id, x.tool, x.ok, x.error
		FROM json_to_recordset(?::json) AS x(collection_id uuid, key_id uuid, key_name text, user_id uuid, tool text, ok boolean, error text)`,
		string(list)).Error, "mcp call")
}

type mcpCallRow struct {
	CollectionID   uuid.UUID  `gorm:"column:collection_id"`
	CollectionName string     `gorm:"column:collection_name"`
	KeyID          *uuid.UUID `gorm:"column:key_id"`
	KeyName        string     `gorm:"column:key_name"`
	UserID         uuid.UUID  `gorm:"column:user_id"`
	UserName       string     `gorm:"column:user_name"`
	Tool           string     `gorm:"column:tool"`
	OK             bool       `gorm:"column:ok"`
	Error          string     `gorm:"column:error"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

func (r *Repository) MCPCalls(ctx context.Context, collectionID string, limit int) ([]domain.MCPCall, error) {
	var rows []mcpCallRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.collection_id, c.key_id, c.key_name, c.user_id, u.name AS user_name, c.tool, c.ok, c.error, c.created_at
		FROM app.mcp_call c JOIN usr.usr_user u ON u.id = c.user_id
		WHERE c.collection_id = ? ORDER BY c.created_at DESC, c.id DESC LIMIT ?`, collectionID, limit).Find(&rows).Error
	return mcpCalls(rows, err)
}

func (r *Repository) KeyMCPCalls(ctx context.Context, keyID, userID string, limit int) ([]domain.MCPCall, error) {
	var rows []mcpCallRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.collection_id, col.name AS collection_name, c.key_id, c.key_name, c.user_id, c.tool, c.ok, c.error, c.created_at
		FROM app.mcp_call c JOIN app.collection col ON col.id = c.collection_id
		WHERE c.key_id = ? AND c.user_id = ? ORDER BY c.created_at DESC, c.id DESC LIMIT ?`, keyID, userID, limit).Find(&rows).Error
	return mcpCalls(rows, err)
}

func mcpCalls(rows []mcpCallRow, err error) ([]domain.MCPCall, error) {
	if err != nil {
		return nil, shared.WrapError(err, "mcp call")
	}
	out := make([]domain.MCPCall, len(rows))
	for i, m := range rows {
		out[i] = domain.MCPCall{CollectionID: m.CollectionID.String(), CollectionName: m.CollectionName, KeyName: m.KeyName, UserID: m.UserID.String(), UserName: m.UserName,
			Tool: m.Tool, OK: m.OK, Error: m.Error, CreatedAt: m.CreatedAt}
		if m.KeyID != nil {
			out[i].KeyID = m.KeyID.String()
		}
	}
	return out, nil
}

func (r *Repository) PruneMCPCalls(ctx context.Context, before time.Time) error {
	return shared.WrapError(r.db.WithContext(ctx).Exec(`DELETE FROM app.mcp_call WHERE created_at < ?`, before).Error, "mcp call")
}

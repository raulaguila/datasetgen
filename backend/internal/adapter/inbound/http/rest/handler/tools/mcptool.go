package tools

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type mcpParamDTO struct {
	Name        string `json:"name" example:"cidade"`
	Description string `json:"description,omitempty" example:"Cidade do cliente"`
	Field       string `json:"field" example:"cidade"`
	Op          string `json:"op" example:"eq" enums:"eq,neq,contains,gt,gte,lt,lte,in,has"`
	Type        string `json:"type,omitempty" example:"string"` // set by the server from the schema
	Required    bool   `json:"required"`
}

type mcpToolDTO struct {
	ID          string        `json:"id"`
	Name        string        `json:"name" example:"clientes_por_cidade"`
	Description string        `json:"description"`
	Enabled     bool          `json:"enabled"`
	Conditions  domain.Where  `json:"conditions" swaggertype:"object"`
	Params      []mcpParamDTO `json:"params"`
	Fields      []string      `json:"fields"`
	Sort        string        `json:"sort" example:"updated_at"`
	Order       string        `json:"order" example:"desc"`
	Limit       int           `json:"limit" example:"20"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

func toMCPToolDTO(t domain.MCPTool) mcpToolDTO {
	params := make([]mcpParamDTO, len(t.Params))
	for i, p := range t.Params {
		params[i] = mcpParamDTO(p)
	}
	fields := t.Fields
	if fields == nil {
		fields = []string{}
	}
	return mcpToolDTO{ID: t.ID, Name: t.Name, Description: t.Description, Enabled: t.Enabled, Conditions: t.Conditions, Params: params,
		Fields: fields, Sort: t.Sort, Order: t.Order, Limit: t.Limit, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

type mcpToolBody struct {
	Name        string        `json:"name" example:"clientes_por_cidade"`
	Description string        `json:"description" example:"Clientes ativos de uma cidade, com nome e telefone."`
	Enabled     bool          `json:"enabled"`
	Conditions  domain.Where  `json:"conditions" swaggertype:"object"`
	Params      []mcpParamDTO `json:"params"`
	Fields      []string      `json:"fields"`
	Sort        string        `json:"sort,omitempty" example:"updated_at"`
	Order       string        `json:"order,omitempty" example:"desc" enums:"asc,desc"`
	Limit       int           `json:"limit,omitempty" example:"20"`
}

func (b mcpToolBody) input() input.MCPToolInput {
	params := make([]domain.MCPToolParam, len(b.Params))
	for i, p := range b.Params {
		params[i] = domain.MCPToolParam(p)
	}
	return input.MCPToolInput{Name: b.Name, Description: b.Description, Enabled: b.Enabled, Conditions: b.Conditions, Params: params,
		Fields: b.Fields, Sort: b.Sort, Order: b.Order, Limit: b.Limit}
}

// @Summary A collection's MCP tools; owners
// @Tags MCP
// @Produce json
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]mcpToolDTO}
// @Failure 403 {object} mapper.errorBody
// @Router /v1/collections/{id}/mcp-tools [get]
func (h *Handler) MCPTools(c fiber.Ctx) error {
	list, err := h.integrations.MCPTools(c.Context(), c.Params("id"))
	if err != nil {
		return fail(c, err)
	}
	out := make([]mcpToolDTO, 0, len(list))
	for _, t := range list {
		out = append(out, toMCPToolDTO(t))
	}
	return c.JSON(fiber.Map{"items": out})
}

// @Summary Publish an MCP tool; owners
// @Description A read-only search that API keys reaching the collection see on /v1/mcp as a tool of its own: fixed conditions, parameters compared with fields (each becomes an AND rule when given), the fields returned (empty: all), sort and page size.
// @Tags MCP
// @Accept json
// @Produce json
// @Param id path string true "Collection ID"
// @Param body body mcpToolBody true "Tool"
// @Security BearerAuth
// @Success 201 {object} mcpToolDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Router /v1/collections/{id}/mcp-tools [post]
func (h *Handler) CreateMCPTool(c fiber.Ctx) error {
	var body mcpToolBody
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	t, err := h.integrations.CreateMCPTool(c.Context(), c.Params("id"), body.input())
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toMCPToolDTO(*t))
}

// @Summary Change an MCP tool; owners
// @Tags MCP
// @Accept json
// @Produce json
// @Param id path string true "Collection ID"
// @Param toolId path string true "Tool ID"
// @Param body body mcpToolBody true "Tool"
// @Security BearerAuth
// @Success 200 {object} mcpToolDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{id}/mcp-tools/{toolId} [put]
func (h *Handler) UpdateMCPTool(c fiber.Ctx) error {
	var body mcpToolBody
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	t, err := h.integrations.UpdateMCPTool(c.Context(), c.Params("id"), c.Params("toolId"), body.input())
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toMCPToolDTO(*t))
}

// @Summary Delete an MCP tool; owners
// @Tags MCP
// @Param id path string true "Collection ID"
// @Param toolId path string true "Tool ID"
// @Security BearerAuth
// @Success 204
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{id}/mcp-tools/{toolId} [delete]
func (h *Handler) DeleteMCPTool(c fiber.Ctx) error {
	if err := h.integrations.DeleteMCPTool(c.Context(), c.Params("id"), c.Params("toolId")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type mcpTestBody struct {
	Tool mcpToolBody                `json:"tool"`
	Args map[string]json.RawMessage `json:"args" swaggertype:"object"`
}

type mcpTestRecordDTO struct {
	ID        string          `json:"id"`
	Data      json.RawMessage `json:"data" swaggertype:"object"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type mcpTestDTO struct {
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
	Records []mcpTestRecordDTO `json:"records"`
}

// @Summary Try an MCP tool before saving it; owners
// @Description Checks the settings as a save does, then runs them once with args (the assistant's arguments) and answers what the assistant would get.
// @Tags MCP
// @Accept json
// @Produce json
// @Param id path string true "Collection ID"
// @Param body body mcpTestBody true "Tool and arguments"
// @Security BearerAuth
// @Success 200 {object} mcpTestDTO
// @Failure 400 {object} mapper.errorBody
// @Router /v1/collections/{id}/mcp-tools/test [post]
func (h *Handler) TestMCPTool(c fiber.Ctx) error {
	var body mcpTestBody
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	items, total, page, err := h.integrations.TestMCPTool(c.Context(), c.Params("id"), body.Tool.input(), body.Args)
	if err != nil {
		return fail(c, err)
	}
	out := mcpTestDTO{Total: total, Page: page, Records: make([]mcpTestRecordDTO, len(items))}
	for i, r := range items {
		out.Records[i] = mcpTestRecordDTO{ID: r.ID, Data: r.Data, UpdatedAt: r.UpdatedAt}
	}
	return c.JSON(out)
}

type mcpCallDTO struct {
	CollectionID   string    `json:"collection_id"`
	CollectionName string    `json:"collection_name,omitempty"` // in a key's log only
	Tool           string    `json:"tool" example:"search_records"`
	KeyName        string    `json:"key_name"`
	KeyLive        bool      `json:"key_live"` // false once the key is deleted
	UserName       string    `json:"user_name"`
	OK             bool      `json:"ok"`
	Error          string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// @Summary What AI assistants did in a collection through MCP (newest 100, last 30 days); owners
// @Tags MCP
// @Produce json
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]mcpCallDTO}
// @Failure 403 {object} mapper.errorBody
// @Router /v1/collections/{id}/mcp-calls [get]
func (h *Handler) MCPCalls(c fiber.Ctx) error {
	list, err := h.integrations.MCPCalls(c.Context(), c.Params("id"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(fiber.Map{"items": toMCPCallDTOs(list)})
}

// @Summary What one of the caller's keys did through MCP (newest 100, last 30 days)
// @Tags API keys
// @Produce json
// @Param keyId path string true "Key ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]mcpCallDTO}
// @Failure 400 {object} mapper.errorBody
// @Router /v1/me/keys/{keyId}/calls [get]
func (h *Handler) KeyMCPCalls(c fiber.Ctx) error {
	list, err := h.integrations.KeyMCPCalls(c.Context(), c.Params("keyId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(fiber.Map{"items": toMCPCallDTOs(list)})
}

func toMCPCallDTOs(list []domain.MCPCall) []mcpCallDTO {
	out := make([]mcpCallDTO, len(list))
	for i, m := range list {
		out[i] = mcpCallDTO{CollectionID: m.CollectionID, CollectionName: m.CollectionName, Tool: m.Tool, KeyName: m.KeyName, KeyLive: m.KeyID != "",
			UserName: m.UserName, OK: m.OK, Error: m.Error, CreatedAt: m.CreatedAt}
	}
	return out
}

// Package mcptool serves the MCP tools a collection's owners publish, and the log of
// what assistants did through MCP.
package mcptool

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type Handler struct {
	svc input.IntegrationService
}

func New(svc input.IntegrationService) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts on /v1/collections/:owner/:slug.
func (h *Handler) Routes(at fiber.Router) {
	at.Get("/mcp-tools", h.MCPTools)
	at.Post("/mcp-tools", h.CreateMCPTool)
	at.Post("/mcp-tools/test", h.TestMCPTool)
	at.Put("/mcp-tools/:toolId", h.UpdateMCPTool)
	at.Delete("/mcp-tools/:toolId", h.DeleteMCPTool)
	at.Get("/mcp-calls", h.MCPCalls)
}

// KeyRoutes mounts one key's call log on /v1/me/keys.
func (h *Handler) KeyRoutes(r fiber.Router) {
	r.Get("/:keyId/calls", h.KeyMCPCalls)
}

type mcpParamDTO struct {
	Name        string `json:"name" example:"cidade"`
	Description string `json:"description,omitempty" example:"Cidade do cliente"`
	Field       string `json:"field" example:"cidade"`
	Op          string `json:"op" example:"eq" enums:"eq,neq,contains,gt,gte,lt,lte,in,has"`
	Type        string `json:"type,omitempty" example:"string"` // set by the server from the schema
	Required    bool   `json:"required"`
} // @name MCPToolParam

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
} // @name MCPTool

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
} // @name MCPToolInput

func (b mcpToolBody) input() input.MCPToolInput {
	params := make([]domain.MCPToolParam, len(b.Params))
	for i, p := range b.Params {
		params[i] = domain.MCPToolParam(p)
	}
	return input.MCPToolInput{Name: b.Name, Description: b.Description, Enabled: b.Enabled, Conditions: b.Conditions, Params: params,
		Fields: b.Fields, Sort: b.Sort, Order: b.Order, Limit: b.Limit}
}

// @Summary A collection's MCP tools; owners
// @Tags MCP tools
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]mcpToolDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID mcptoolList
// @Router /v1/collections/{owner}/{slug}/mcp-tools [get]
func (h *Handler) MCPTools(c fiber.Ctx) error {
	list, err := h.svc.MCPTools(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]mcpToolDTO, 0, len(list))
	for _, t := range list {
		out = append(out, toMCPToolDTO(t))
	}
	return c.JSON(fiber.Map{"items": out})
}

// @Summary Publish an MCP tool; owners
// @Description A read-only search that API keys reaching the collection see on /v1/mcp as a tool of its own: fixed conditions, parameters compared with fields (each becomes an AND rule when given), the fields returned (empty: all), sort and page size.
// @Tags MCP tools
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body mcpToolBody true "Tool"
// @Security BearerAuth
// @Success 201 {object} mcpToolDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID mcptoolCreate
// @Router /v1/collections/{owner}/{slug}/mcp-tools [post]
func (h *Handler) CreateMCPTool(c fiber.Ctx) error {
	var body mcpToolBody
	if err := c.Bind().Body(&body); err != nil {
		return mapper.BadBody(c)
	}
	t, err := h.svc.CreateMCPTool(c.Context(), middleware.CollectionID(c), body.input())
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toMCPToolDTO(*t))
}

// @Summary Change an MCP tool; owners
// @Tags MCP tools
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param toolId path string true "Tool ID"
// @Param body body mcpToolBody true "Tool"
// @Security BearerAuth
// @Success 200 {object} mcpToolDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID mcptoolUpdate
// @Router /v1/collections/{owner}/{slug}/mcp-tools/{toolId} [put]
func (h *Handler) UpdateMCPTool(c fiber.Ctx) error {
	var body mcpToolBody
	if err := c.Bind().Body(&body); err != nil {
		return mapper.BadBody(c)
	}
	t, err := h.svc.UpdateMCPTool(c.Context(), middleware.CollectionID(c), c.Params("toolId"), body.input())
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toMCPToolDTO(*t))
}

// @Summary Delete an MCP tool; owners
// @Tags MCP tools
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param toolId path string true "Tool ID"
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID mcptoolDelete
// @Router /v1/collections/{owner}/{slug}/mcp-tools/{toolId} [delete]
func (h *Handler) DeleteMCPTool(c fiber.Ctx) error {
	if err := h.svc.DeleteMCPTool(c.Context(), middleware.CollectionID(c), c.Params("toolId")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type mcpTestBody struct {
	Tool mcpToolBody                `json:"tool"`
	Args map[string]json.RawMessage `json:"args" swaggertype:"object"`
} // @name MCPToolTestInput

type mcpTestRecordDTO struct {
	ID        string          `json:"id"`
	Data      json.RawMessage `json:"data" swaggertype:"object"`
	UpdatedAt time.Time       `json:"updated_at"`
} // @name MCPToolTestRecord

type mcpTestDTO struct {
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
	Records []mcpTestRecordDTO `json:"records"`
} // @name MCPToolTest

// @Summary Try an MCP tool before saving it; owners
// @Description Checks the settings as a save does, then runs them once with args (the assistant's arguments) and answers what the assistant would get.
// @Tags MCP tools
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body mcpTestBody true "Tool and arguments"
// @Security BearerAuth
// @Success 200 {object} mcpTestDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID mcptoolTest
// @Router /v1/collections/{owner}/{slug}/mcp-tools/test [post]
func (h *Handler) TestMCPTool(c fiber.Ctx) error {
	var body mcpTestBody
	if err := c.Bind().Body(&body); err != nil {
		return mapper.BadBody(c)
	}
	items, total, page, err := h.svc.TestMCPTool(c.Context(), middleware.CollectionID(c), body.Tool.input(), body.Args)
	if err != nil {
		return mapper.WriteError(c, err)
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
	Args           string    `json:"args,omitempty" example:"{\"collection_id\":\"…\",\"rules\":[{\"field\":\"status\",\"op\":\"eq\",\"value\":\"Pago\"}]}"` // the arguments as JSON, cut to 500 characters
	CreatedAt      time.Time `json:"created_at"`
} // @name MCPCall

// @Summary What AI assistants did in a collection through MCP (newest 100, last 30 days); owners
// @Tags MCP tools
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]mcpCallDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID mcptoolCalls
// @Router /v1/collections/{owner}/{slug}/mcp-calls [get]
func (h *Handler) MCPCalls(c fiber.Ctx) error {
	list, err := h.svc.MCPCalls(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(fiber.Map{"items": toMCPCallDTOs(list)})
}

// @Summary What one of the caller's keys did through MCP (newest 100, last 30 days)
// @Tags MCP tools
// @Produce json
// @Param keyId path string true "Key ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]mcpCallDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID mcptoolKeyCalls
// @Router /v1/me/keys/{keyId}/calls [get]
func (h *Handler) KeyMCPCalls(c fiber.Ctx) error {
	list, err := h.svc.KeyMCPCalls(c.Context(), c.Params("keyId"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(fiber.Map{"items": toMCPCallDTOs(list)})
}

func toMCPCallDTOs(list []domain.MCPCall) []mcpCallDTO {
	out := make([]mcpCallDTO, len(list))
	for i, m := range list {
		out[i] = mcpCallDTO{CollectionID: m.CollectionID, CollectionName: m.CollectionName, Tool: m.Tool, KeyName: m.KeyName, KeyLive: m.KeyID != "",
			UserName: m.UserName, OK: m.OK, Error: m.Error, Args: m.Args, CreatedAt: m.CreatedAt}
	}
	return out
}

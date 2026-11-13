package record

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/schema"
)

type versionDTO struct {
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data" swaggertype:"object"`
	SavedAt time.Time       `json:"saved_at"`
}

// @Summary A record's earlier versions, newest first (up to 50)
// @Tags Records
// @Produce json
// @Param id path string true "Record ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]versionDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/records/{id}/versions [get]
func (h *Handler) Versions(c fiber.Ctx) error {
	items, err := h.svc.Versions(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]versionDTO, 0, len(items))
	for _, v := range items {
		out = append(out, versionDTO{Version: v.Version, Data: v.Data, SavedAt: v.SavedAt})
	}
	return c.JSON(fiber.Map{"items": out})
}

type restoreDTO struct {
	Version int `json:"version" example:"2"`
	// CurrentVersion is the version the client read; when sent, a newer row is a 409 conflict.
	CurrentVersion *int `json:"current_version" example:"5"`
}

// @Summary Write an earlier version back as the record's new version
// @Description The old data must match the current schema (422 with issues otherwise).
// @Tags Records
// @Accept json
// @Produce json
// @Param id path string true "Record ID"
// @Param body body restoreDTO true "Version"
// @Security BearerAuth
// @Success 200 {object} recordDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 422 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/records/{id}/restore [post]
func (h *Handler) Restore(c fiber.Ctx) error {
	var body restoreDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	r, err := h.svc.Restore(c.Context(), input.RestoreRecordCommand{ID: c.Params("id"), Version: body.Version, CurrentVersion: body.CurrentVersion})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toDTO(*r))
}

type importDTO struct {
	CollectionID string            `json:"collection_id"`
	Records      []json.RawMessage `json:"records" swaggertype:"array,object"`
	DryRun       bool              `json:"dry_run"`
	SkipInvalid  bool              `json:"skip_invalid"`
	// UpsertKey, a unique field: a record already holding a row's value is updated (its other fields stay).
	UpsertKey string `json:"upsert_key,omitempty"`
}

type importResultDTO struct {
	Created int64            `json:"created"`
	Updated int64            `json:"updated"`
	Invalid int              `json:"invalid"`
	Errors  []importErrorDTO `json:"errors"`
}

type importErrorDTO struct {
	// Index is the record's position in this request.
	Index  int        `json:"index"`
	Issues []issueDTO `json:"issues"`
}

// issueDTO mirrors schema.Issue, which swag does not read.
type issueDTO struct {
	Path    string `json:"path"`
	Keyword string `json:"keyword" example:"required"`
	Message string `json:"message"`
}

// @Summary Check a batch of records against the schema and add them; editors and owners
// @Description Up to 1000 records within the body limit; clients send a large file in batches. With dry_run nothing is written. With invalid records and no skip_invalid, none are written. With upsert_key (a unique field) a row whose value a record already holds updates that record.
// @Tags Records
// @Accept json
// @Produce json
// @Param body body importDTO true "Batch"
// @Security BearerAuth
// @Success 200 {object} importResultDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 413 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/records/import [post]
func (h *Handler) Import(c fiber.Ctx) error {
	var body importDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	res, err := h.svc.Import(c.Context(), input.ImportRecordsCommand{
		CollectionID: body.CollectionID, Records: body.Records, DryRun: body.DryRun, SkipInvalid: body.SkipInvalid, UpsertKey: body.UpsertKey,
	})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := importResultDTO{Created: res.Created, Updated: res.Updated, Invalid: res.Invalid, Errors: make([]importErrorDTO, 0, len(res.Errors))}
	for _, e := range res.Errors {
		issues := make([]issueDTO, 0, len(e.Issues))
		for _, i := range e.Issues {
			issues = append(issues, issueDTO(i))
		}
		out.Errors = append(out.Errors, importErrorDTO{Index: e.Index, Issues: issues})
	}
	return c.JSON(out)
}

// @Summary Download every record of a collection as JSON Lines or CSV
// @Description JSON Lines holds one record's data per line. CSV has one column per field, in the form's order, then any other keys; lists and objects are written as JSON. Up to 100 000 records.
// @Tags Records
// @Produce plain
// @Param collection_id query string true "Collection ID"
// @Param format query string false "File format" Enums(jsonl, csv) default(jsonl)
// @Param valid query bool false "Only records that match the current schema"
// @Security BearerAuth
// @Success 200 {string} string
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/records/export [get]
func (h *Handler) Export(c fiber.Ctx) error {
	format := c.Query("format", "jsonl")
	if format != "jsonl" && format != "csv" {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "format must be jsonl or csv").WithField("format"))
	}
	exp, err := h.svc.Export(c.Context(), c.Query("collection_id"), fiber.Query[bool](c, "valid"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	var body []byte
	if format == "csv" {
		c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
		body, err = recordsCSV(exp)
	} else {
		c.Set(fiber.HeaderContentType, "application/x-ndjson; charset=utf-8")
		body = recordsJSONL(exp)
	}
	if err != nil {
		return mapper.WriteError(c, err)
	}
	name := fmt.Sprintf("%s-%s.%s", exp.Name, time.Now().UTC().Format("20060102"), format)
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="`+asciiName(name)+`"; filename*=UTF-8''`+url.PathEscape(name))
	return c.Send(body)
}

func recordsJSONL(exp *input.Export) []byte {
	var buf bytes.Buffer
	for _, r := range exp.Records {
		var compact bytes.Buffer
		if json.Compact(&compact, r) != nil { // stored as jsonb: always valid
			compact.Write(r)
		}
		buf.Write(compact.Bytes())
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func recordsCSV(exp *input.Export) ([]byte, error) {
	columns := schema.PropertyOrder(exp.Schema)
	known := map[string]bool{}
	for _, k := range columns {
		known[k] = true
	}
	rows := make([]map[string]json.RawMessage, 0, len(exp.Records))
	var extra []string
	for _, r := range exp.Records {
		var fields map[string]json.RawMessage
		if json.Unmarshal(r, &fields) != nil {
			fields = map[string]json.RawMessage{"": r} // not an object: the whole value in one column
		}
		for k := range fields {
			if !known[k] {
				known[k] = true
				extra = append(extra, k)
			}
		}
		rows = append(rows, fields)
	}
	sort.Strings(extra)
	columns = append(columns, extra...)

	var buf bytes.Buffer
	buf.WriteString("\uFEFF") // BOM: spreadsheets then read the accents as UTF-8
	w := csv.NewWriter(&buf)
	header := make([]string, len(columns))
	for i, k := range columns {
		header[i] = mapper.CSVCell(k)
	}
	_ = w.Write(header)
	for _, fields := range rows {
		line := make([]string, len(columns))
		for i, k := range columns {
			line[i] = mapper.CSVCell(csvValue(fields[k]))
		}
		_ = w.Write(line)
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// csvValue writes a string as its text and anything else as JSON; missing and null are empty.
func csvValue(v json.RawMessage) string {
	if len(v) == 0 || string(v) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	var compact bytes.Buffer
	if json.Compact(&compact, v) == nil {
		return compact.String()
	}
	return strings.TrimSpace(string(v))
}

// asciiName keeps letters, digits, dot and dash for the plain filename; filename* carries the real one.
func asciiName(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 128 && (r == '.' || r == '-' || r == '_' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9')) {
			return r
		}
		return '_'
	}, s)
}

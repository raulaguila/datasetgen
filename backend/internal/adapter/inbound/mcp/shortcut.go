package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"path"
)

// Shortcuts answers tools/list (GET) and tools/call (POST: the last path segment
// names the tool, the body holds its arguments) as plain JSON, for the API docs
// and quick tests. Each request goes through h as one JSON-RPC call, so the key,
// its scopes, the member's roles and the call log apply as on /v1/mcp.
func Shortcuts(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}
		if r.Method == http.MethodPost {
			args, err := io.ReadAll(r.Body)
			if len(bytes.TrimSpace(args)) == 0 {
				args = []byte("{}")
			}
			if err != nil || !json.Valid(args) {
				invalid(w, "the body must be a JSON object with the tool's arguments")
				return
			}
			call["method"] = "tools/call"
			call["params"] = map[string]any{"name": path.Base(r.URL.Path), "arguments": json.RawMessage(args)}
		}
		body, _ := json.Marshal(call)
		fwd := r.Clone(r.Context())
		fwd.Method = http.MethodPost
		fwd.Body = io.NopCloser(bytes.NewReader(body))
		fwd.ContentLength = int64(len(body))
		fwd.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, fwd)

		var out struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
			maps.Copy(w.Header(), rec.Header()) // a 401 keeps its WWW-Authenticate
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		if out.Error != nil {
			invalid(w, out.Error.Message)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(out.Result)
	})
}

// invalid answers 400 in the REST API's error shape.
func invalid(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": "INVALID_INPUT", "message": message})
}

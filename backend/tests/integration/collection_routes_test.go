package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/pointer"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// The API reaches a collection at /v1/collections/<owner>/<slug>, as the app's pages
// do; its records take an API key there too, and a renamed collection's old path still answers.
func TestHTTPCollectionRoutesUseTheAddress(t *testing.T) {
	e := newEnv(t)
	collections, _ := collectionServices(e)
	keys, _ := toolServices(e, t)
	anaAddr, outsiderAddr := e.activeUser(t), e.activeUser(t)
	ana := reqctx.WithActor(context.Background(), e.userID(t, anaAddr))
	c, err := collections.Create(ana, input.CreateCollectionCommand{Name: "Livros Lidos", Schema: json.RawMessage(`{"type":"object","properties":{"t":{"type":"string"}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	other, err := collections.Create(ana, input.CreateCollectionCommand{Name: "Outra", Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := e.login(anaAddr, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := e.login(outsiderAddr, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	key, err := keys.CreateMyKey(ana, "Leitura", []domain.KeyScope{{CollectionID: c.ID}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	app := e.httpApp()
	defer func() { _ = app.Shutdown() }()
	call := func(method, path, body string, auth [2]string, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if auth[0] != "" {
			req.Header.Set(auth[0], auth[1])
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		data, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, res.StatusCode, want, data)
		}
		var out map[string]any
		_ = json.Unmarshal(data, &out)
		return out
	}
	as := [2]string{"Authorization", "Bearer " + pair.AccessToken}
	stranger := [2]string{"Authorization", "Bearer " + outsider.AccessToken}
	byKey := [2]string{"X-API-Key", key.Secret}
	at := "/v1/collections/" + c.Owner + "/" + c.Slug

	if got := call("GET", at, "", as, 200); got["id"] != c.ID || got["slug"] != "livros-lidos" {
		t.Fatalf("get: %v", got)
	}
	call("GET", "/v1/collections/"+strings.ToUpper(c.Owner)+"/Livros-Lidos", "", as, 200)
	call("GET", at+"/members", "", as, 200)
	call("GET", at, "", stranger, 404) // a collection one cannot open reads as missing
	call("GET", "/v1/collections/"+c.Owner+"/nada", "", as, 404)
	call("GET", "/v1/collections/"+c.ID, "", as, 404) // ids are not addresses

	// Records of the collection: the session and a key both reach them at the address.
	made := call("POST", at+"/records", `{"data":{"t":"a"}}`, as, 201)
	if made["collection_id"] != c.ID {
		t.Fatalf("record: %v", made)
	}
	if got := call("GET", at+"/records", "", byKey, 200); len(got["items"].([]any)) != 1 {
		t.Fatalf("records by key: %v", got)
	}
	call("GET", at+"/records/profile", "", byKey, 200)
	call("POST", at+"/records", `{"data":{"t":"b"}}`, byKey, 403) // the key reads only
	call("GET", "/v1/collections/"+other.Owner+"/"+other.Slug+"/records", "", byKey, 404)
	call("GET", at+"/members", "", byKey, 401) // a key reaches records, not the collection's settings
	call("GET", "/v1/records/"+made["id"].(string), "", byKey, 200)

	// A rename moves the address; the old one still answers.
	if _, err := collections.Update(ana, input.UpdateCollectionCommand{ID: c.ID, Name: pointer.Of("Leituras")}); err != nil {
		t.Fatal(err)
	}
	if got := call("GET", at, "", as, 200); got["slug"] != "leituras" {
		t.Fatalf("old path: %v", got)
	}
	call("PATCH", "/v1/collections/"+c.Owner+"/leituras", `{"description":"x"}`, as, 200)
	// The log and the notifications link to the current address, not to an id.
	audit := call("GET", at+"/audit?action=collection.update", "", as, 200)["items"].([]any)
	if len(audit) == 0 || audit[0].(map[string]any)["target_slug"] != "leituras" || audit[0].(map[string]any)["target_owner"] != c.Owner {
		t.Fatalf("audit: %v", audit)
	}
	call("GET", "/v1/me/notifications", "", as, 200)

	// The trash keeps ids: a deleted collection gave its address up.
	call("POST", "/v1/collections/delete", `{"ids":["`+other.ID+`"]}`, as, 204)
	call("GET", "/v1/collections/"+other.Owner+"/"+other.Slug, "", as, 404)
	call("POST", "/v1/collections/trash/"+other.ID+"/restore", "", as, 204)
	call("GET", "/v1/collections/"+other.Owner+"/"+other.Slug, "", as, 200)
}

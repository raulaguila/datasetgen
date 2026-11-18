package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// Uses the same migrated PostgreSQL fixture as the integration tests. Each
// iteration processes 500 existing records, including two unique fields.
func BenchmarkImportExisting(b *testing.B) {
	e := newEnv(b)
	collections, records := collectionServices(e)
	ctx := reqctx.WithActor(context.Background(), e.userID(b, e.activeUser(b)))
	c, err := collections.Create(ctx, input.CreateCollectionCommand{
		Name:   "Upsert benchmark",
		Schema: json.RawMessage(`{"type":"object","properties":{"sku":{"type":"integer","x-unique":true},"label":{"type":"string","x-unique":true}}}`),
	})
	if err != nil {
		b.Fatal(err)
	}
	cmd := input.ImportRecordsCommand{CollectionID: c.ID, UpsertKey: "sku", Records: make([]json.RawMessage, 500)}
	for i := range cmd.Records {
		cmd.Records[i] = json.RawMessage(fmt.Sprintf(`{"sku":%d,"label":"k%d"}`, i, i))
	}
	if result, err := records.Import(ctx, cmd); err != nil || result.Created != 500 {
		b.Fatal(result, err)
	}
	for _, mode := range []string{"DryRun", "Save"} {
		b.Run(mode, func(b *testing.B) {
			cmd.DryRun = mode == "DryRun"
			b.ReportAllocs()
			for b.Loop() {
				result, err := records.Import(ctx, cmd)
				if err != nil || result.Invalid != 0 || (!cmd.DryRun && result.Updated != 500) {
					b.Fatal(result, err)
				}
			}
		})
	}
}

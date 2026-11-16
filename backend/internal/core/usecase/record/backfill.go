package record

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/schema"
	"github.com/raulaguila/dataset/pkg/validate"
)

const fillBatch = 200

// Jobs returns the record module's job handlers, registered with the job worker.
func Jobs(tx output.TxManager) map[string]policy.JobHandler {
	return map[string]policy.JobHandler{
		policy.JobFillComputed: func(ctx context.Context, payload []byte) error {
			var job struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(payload, &job); err != nil || validate.ID(job.ID) != nil {
				return policy.PermanentJobError(errors.New("record.fill: bad payload"))
			}
			return backfill(ctx, tx, job.ID)
		},
	}
}

// backfill gives the records written before an auto number or formula existed
// (or changed) their value: numbers in the order the records were created,
// after the highest one taken, then every formula again. Each batch is its own
// transaction under the schema's share lock, so a schema change waits for it.
// Filling is an edit: the record gets a new version, as any change does.
// It is safe to repeat: a record holding its number and formula values is left alone.
func backfill(ctx context.Context, tx output.TxManager, id string) error {
	for {
		filled := 0
		err := tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
			raw, err := uow.Collections().SchemaForWrite(ctx, id)
			if err != nil {
				return err
			}
			formulas := schema.Formulas(raw)
			parsed, err := parseFormulas(formulas)
			if err != nil {
				return err
			}
			now := time.Now()
			for _, a := range schema.AutoKeys(raw) {
				if a.Kind != schema.AutoNumber {
					continue
				}
				batch, err := uow.Records().WithoutNumber(ctx, id, a.Key, fillBatch)
				if err != nil {
					return err
				}
				if len(batch) == 0 {
					continue
				}
				// The unique lock keeps a record created meanwhile from taking the same number.
				if err := uow.Records().LockUnique(ctx, id); err != nil {
					return err
				}
				next, err := uow.Records().MaxNumber(ctx, id, a.Key)
				if err != nil {
					return err
				}
				for _, r := range batch {
					var doc map[string]json.RawMessage
					if err := json.Unmarshal(r.Data, &doc); err != nil {
						return err
					}
					next++
					doc[a.Key], _ = json.Marshal(next)
					applyFormulas(doc, parsed, formulas, now)
					if err := save(ctx, uow, r.ID, doc); err != nil {
						return err
					}
				}
				filled += len(batch)
				return nil // one batch per transaction
			}
			return nil
		})
		if apperror.IsNotFound(err) {
			return nil // the collection is gone
		}
		if err != nil {
			return err
		}
		if filled == 0 {
			break
		}
	}
	after := ""
	for {
		var batch int
		err := tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
			raw, err := uow.Collections().SchemaForWrite(ctx, id)
			if err != nil {
				return err
			}
			formulas := schema.Formulas(raw)
			if len(formulas) == 0 {
				return nil
			}
			parsed, err := parseFormulas(formulas)
			if err != nil {
				return err
			}
			records, err := uow.Records().Scan(ctx, id, after, fillBatch)
			if err != nil {
				return err
			}
			batch = len(records)
			now := time.Now()
			for _, r := range records {
				after = r.ID
				var doc map[string]json.RawMessage
				if json.Unmarshal(r.Data, &doc) != nil || doc == nil {
					continue // not an object: the quality check lists it
				}
				applyFormulas(doc, parsed, formulas, now)
				out, err := json.Marshal(doc)
				if err != nil {
					return err
				}
				if same, err := sameJSON(r.Data, out); err != nil || same {
					continue
				}
				if err := save(ctx, uow, r.ID, doc); err != nil {
					return err
				}
			}
			return nil
		})
		if apperror.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if batch < fillBatch {
			return nil
		}
	}
}

func save(ctx context.Context, uow output.UnitOfWork, id string, doc map[string]json.RawMessage) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	_, err = uow.Records().Update(ctx, id, data, nil)
	return err
}

// sameJSON compares two JSON texts by value: jsonb hands keys back in its own order.
func sameJSON(a, b []byte) (bool, error) {
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, &y); err != nil {
		return false, err
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya), nil
}

package product

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/output"
)

// JobCreated runs after a product is created. It is the example of a module's
// background work: replace the log line with what a real module does later,
// such as syncing a search index or calling another system.
const JobCreated = "product.created"

type createdJob struct {
	ID string `json:"id"`
}

// Jobs returns this module's job handlers, registered with the job worker.
func Jobs(log output.Logger) map[string]policy.JobHandler {
	return map[string]policy.JobHandler{
		JobCreated: func(_ context.Context, payload []byte) error {
			var job createdJob
			if err := json.Unmarshal(payload, &job); err != nil {
				return policy.PermanentJobError(err)
			}
			output.Log(log).Info("Product created job ran", slog.String("product_id", job.ID))
			return nil
		},
	}
}

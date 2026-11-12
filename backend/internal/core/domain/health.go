package domain

// CheckStatus is the result of one dependency probe.
type CheckStatus string

const (
	CheckOK   CheckStatus = "ok"
	CheckFail CheckStatus = "fail"
)

// HealthReport aggregates liveness of dependencies.
type HealthReport struct {
	Status string // ok | degraded | down
	Checks map[string]CheckStatus
}

// Aggregate sets Status from Checks: all ok → ok; any fail with some ok → degraded; all fail → down.
func (h *HealthReport) Aggregate() {
	if h.Checks == nil {
		h.Status = "down"
		return
	}
	ok, fail := 0, 0
	for _, s := range h.Checks {
		switch s {
		case CheckOK:
			ok++
		case CheckFail:
			fail++
		}
	}
	switch {
	case fail == 0:
		h.Status = "ok"
	case ok == 0:
		h.Status = "down"
	default:
		h.Status = "degraded"
	}
}

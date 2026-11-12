package health

import (
	"context"
	"sync"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
)

const reportTTL = time.Second

// Service aggregates dependency pings.
type Service struct {
	Postgres output.Pinger

	mu      sync.Mutex
	last    *domain.HealthReport
	at      time.Time
	probing bool
}

var _ input.HealthChecker = (*Service)(nil)

func New(postgres output.Pinger) *Service {
	return &Service{Postgres: postgres, last: report(domain.CheckFail)}
}

// Check reuses a fresh report: /health has no rate limit, so without this every
// anonymous request would reach the database. While one request probes the
// others answer the last report, so a slow database cannot pile them up.
func (s *Service) Check(ctx context.Context) (*domain.HealthReport, error) {
	s.mu.Lock()
	if s.probing || time.Since(s.at) < reportTTL {
		defer s.mu.Unlock()
		return s.last, nil
	}
	s.probing = true
	s.mu.Unlock()

	status := domain.CheckOK
	if err := s.Postgres.Ping(ctx); err != nil {
		status = domain.CheckFail
	}
	fresh := report(status)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.last, s.at, s.probing = fresh, time.Now(), false
	return fresh, nil
}

func report(postgres domain.CheckStatus) *domain.HealthReport {
	r := &domain.HealthReport{Checks: map[string]domain.CheckStatus{"postgres": postgres}}
	r.Aggregate()
	return r
}

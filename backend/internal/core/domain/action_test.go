package domain

import "testing"

func TestAllows(t *testing.T) {
	have := ActionCreate | ActionRead
	if !Allows(have, ActionRead) {
		t.Fatal("expected read allowed")
	}
	if Allows(have, ActionDelete) {
		t.Fatal("expected delete denied")
	}
	if !Allows(ActionCreate|ActionUpdate, ActionCreate|ActionUpdate) {
		t.Fatal("expected both bits")
	}
}

func TestHealthAggregate(t *testing.T) {
	r := &HealthReport{Checks: map[string]CheckStatus{"postgres": CheckOK, "cache": CheckOK}}
	r.Aggregate()
	if r.Status != "ok" {
		t.Fatalf("got %s", r.Status)
	}

	r = &HealthReport{Checks: map[string]CheckStatus{"postgres": CheckOK, "cache": CheckFail}}
	r.Aggregate()
	if r.Status != "degraded" {
		t.Fatalf("got %s", r.Status)
	}

	r = &HealthReport{Checks: map[string]CheckStatus{"postgres": CheckFail, "cache": CheckFail}}
	r.Aggregate()
	if r.Status != "down" {
		t.Fatalf("got %s", r.Status)
	}
}

package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/raulaguila/dataset/pkg/aescrypt"
)

func TestOutboxHoldsSealedTokenUntilDone(t *testing.T) {
	e := newEnv(t)
	res := e.register(t, email())
	ctx := context.Background()

	msgs, err := e.store.Outbox().ClaimPending(ctx, 10, e.clock.now.Add(time.Second), time.Minute)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("claim: %d %v", len(msgs), err)
	}
	if strings.Contains(string(msgs[0].Payload), res.ActivationToken) {
		t.Fatal("plaintext token in outbox")
	}
	var p struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &p); err != nil {
		t.Fatal(err)
	}
	if plain, err := aescrypt.Decrypt(testKey, p.Token); err != nil || plain != res.ActivationToken {
		t.Fatalf("decrypt: %q %v", plain, err)
	}

	if err := e.store.Outbox().MarkSent(ctx, msgs[0].ID, msgs[0].ClaimID, e.clock.now); err != nil {
		t.Fatal(err)
	}
	var left int64
	if err := testDB.Table("sys.outbox").Where("id = ? AND payload ? 'token'", msgs[0].ID).Count(&left).Error; err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatal("sealed token kept after send")
	}
}

func TestCrashedClaimReturnsAfterLease(t *testing.T) {
	e := newEnv(t)
	e.register(t, email())
	ctx := context.Background()
	now := e.clock.now.Add(time.Second)

	first, err := e.store.Outbox().ClaimPending(ctx, 10, now, time.Minute)
	if err != nil || len(first) != 1 {
		t.Fatalf("claim: %d %v", len(first), err)
	}
	again, err := e.store.Outbox().ClaimPending(ctx, 10, now.Add(30*time.Second), time.Minute)
	if err != nil || len(again) != 0 {
		t.Fatalf("double claim: %d %v", len(again), err)
	}
	back, err := e.store.Outbox().ClaimPending(ctx, 10, now.Add(2*time.Minute), time.Minute)
	if err != nil || len(back) != 1 {
		t.Fatalf("lost after crash: %d %v", len(back), err)
	}
}

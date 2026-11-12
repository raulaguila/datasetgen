package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/argon2"
)

// OWASP-oriented interactive defaults (RFC 9106 second recommendation).
const (
	timeCost  = 3
	memoryKiB = 64 * 1024
	threads   = 4
	keyLen    = 32
	saltLen   = 16

	// maxHashes keeps live hash memory at 256 MiB however many CPUs the host has.
	maxHashes = 4
)

// Each hash holds memoryKiB of RAM and runs threads goroutines, so more hashes
// than CPUs/threads add memory without adding throughput.
var active, waiting, operations, cancelled, waitNanos atomic.Int64

// Stats contains process-local hashing pressure, without any password material.
func Stats() map[string]int64 {
	return map[string]int64{"active": active.Load(), "waiting": waiting.Load(), "operations": operations.Load(), "cancelled": cancelled.Load(), "wait_ns": waitNanos.Load(), "capacity": int64(cap(slots))}
}

var slots = make(chan struct{}, max(1, min(runtime.GOMAXPROCS(0)/threads, maxHashes)))

// idKey waits for a slot only as long as ctx lives: nobody reads the answer of
// a request that already timed out.
func idKey(ctx context.Context, plain string, salt []byte, t, m uint32, p uint8, n uint32) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	waiting.Add(1)
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		waiting.Add(-1)
		waitNanos.Add(time.Since(start).Nanoseconds())
		cancelled.Add(1)
		return nil, ctx.Err()
	}
	waiting.Add(-1)
	waitNanos.Add(time.Since(start).Nanoseconds())
	active.Add(1)
	operations.Add(1)
	defer func() { active.Add(-1); <-slots }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return argon2.IDKey([]byte(plain), salt, t, m, p, n), nil
}

// A valid, public PHC value only used to spend the normal verification cost.
// No lazy first-request hash or context.Background work is needed.
var timingHash = fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$MDEyMzQ1Njc4OWFiY2RlZg$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	argon2.Version, memoryKiB, timeCost, threads)

// Waste spends the time of a real verify, so accounts without a usable
// password answer as slowly as accounts with one.
func Waste(ctx context.Context, plain string) {
	_, _ = Verify(ctx, timingHash, plain)
}

// Hash returns a PHC-encoded argon2id string.
func Hash(ctx context.Context, plain string) (string, error) {
	if plain == "" {
		return "", errors.New("empty password")
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash, err := idKey(ctx, plain, salt, timeCost, memoryKiB, threads, keyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memoryKiB,
		timeCost,
		threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// Verify compares plain password against a PHC argon2id hash.
func Verify(ctx context.Context, encoded, plain string) (bool, error) {
	if len(encoded) > 512 {
		return false, errors.New("hash too long")
	}
	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("invalid hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, err
	}
	if version != argon2.Version {
		return false, errors.New("unsupported argon2 version")
	}
	var mem, timeN, pars int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &timeN, &pars); err != nil {
		return false, err
	}
	if timeN < 1 || timeN > timeCost || pars < 1 || pars > threads || mem < 8*pars || mem > memoryKiB {
		return false, errors.New("argon2 parameters outside supported limits")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	if len(salt) < 8 || len(salt) > 64 || len(want) < 16 || len(want) > 64 {
		return false, errors.New("invalid salt or digest length")
	}
	got, err := idKey(ctx, plain, salt, uint32(timeN), uint32(mem), uint8(pars), uint32(len(want)))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

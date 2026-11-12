package integration_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/adapter/outbound/emailpayload"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/rbac"
	"github.com/raulaguila/dataset/internal/adapter/outbound/tokens"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/usecase/account"
	"github.com/raulaguila/dataset/internal/core/usecase/auth"
	"github.com/raulaguila/dataset/pkg/apperror"

	"github.com/raulaguila/dataset/internal/adapter/outbound/files"
)

const strongPassword = "correct horse battery"

var (
	testDB  *gorm.DB
	testKey = []byte(strings.Repeat("k", 32))
	keyPair ed25519.PrivateKey
)

// TestMain needs TEST_POSTGRES_URL pointing at an empty database; CI provides one.
func TestMain(m *testing.M) {
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		fmt.Println("TEST_POSTGRES_URL not set: skipping Postgres tests")
		return
	}
	if err := migrate(url); err != nil {
		fmt.Println("migrate:", err)
		os.Exit(1)
	}
	var err error
	if testDB, err = postgres.Connect(&postgres.Config{URL: url, MaxConns: 20}); err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	if _, keyPair, err = ed25519.GenerateKey(rand.Reader); err != nil {
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func migrate(url string) error {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	files, err := filepath.Glob("../../deploy/db/migrations/*.up.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
	}
	return nil
}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

type env struct {
	svc   *auth.Service
	store *postgres.Store
	clock *clock
}

func newEnv(t testing.TB) *env {
	t.Helper()
	if err := testDB.Exec("DELETE FROM sys.outbox").Error; err != nil {
		t.Fatal(err)
	}
	store := postgres.NewStore(testDB)
	c := &clock{now: time.Now()}
	issuer := &tokens.Issuer{AccessKey: keyPair, RefreshKey: keyPair, AccessExpire: time.Minute, RefreshExpire: time.Hour}
	svc := auth.New(store, store, issuer, c, auth.Config{
		SessionMaxLifetime:           30 * 24 * time.Hour,
		RegistrationTokenTTL:         24 * time.Hour,
		PasswordResetTokenTTL:        2 * time.Hour,
		MaxFailedLoginAttempts:       3,
		AccountLockDuration:          15 * time.Minute,
		AllowManualTokenDelivery:     true,
		AllowRegistration:            true,
		LoginAttemptEmailWindow:      15 * time.Minute,
		LoginAttemptEmailMaxFailures: 5,
		LoginAttemptPairMaxFailures:  5,
		LoginAttemptIPWindow:         time.Minute,
		LoginAttemptIPMaxFailures:    20,
	}, emailpayload.New(testKey))
	return &env{svc: svc, store: store, clock: c}
}

func email() string { return "u" + uuid.NewString()[:8] + "@acme.io" }

func (e *env) register(t testing.TB, addr string) *input.RegisterResult {
	t.Helper()
	res, err := e.svc.Register(context.Background(), input.RegisterCommand{Name: "Ana", Email: addr})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (e *env) activeUser(t testing.TB) string {
	t.Helper()
	addr := email()
	res := e.register(t, addr)
	if err := e.svc.Activate(context.Background(), input.ActivateCommand{Token: res.ActivationToken, Password: strongPassword}); err != nil {
		t.Fatal(err)
	}
	return addr
}

func (e *env) login(addr, pass string) (*domain.TokenPair, error) {
	return e.svc.Login(context.Background(), input.LoginCommand{Email: addr, Password: pass})
}

func (e *env) userStatus(t testing.TB, addr string) string {
	t.Helper()
	u, err := e.store.Users().FindByEmail(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	return u.Status
}

func wantCode(t testing.TB, err error, code apperror.Code) {
	t.Helper()
	if !apperror.IsCode(err, code) {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func (e *env) loginFrom(addr, pass, ip string) (*domain.TokenPair, error) {
	return e.svc.Login(context.Background(), input.LoginCommand{Email: addr, Password: pass, IPAddress: &ip})
}

func (e *env) account() *account.Service {
	return account.New(e.store, rbac.NewRepository(testDB), e.clock, &files.Local{Dir: filepath.Join(os.TempDir(), "dataset-it-files")})
}

func (e *env) userID(t testing.TB, addr string) string {
	t.Helper()
	u, err := e.store.Users().FindByEmail(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func (e *env) loginAt(t testing.TB, addr, ip string) *domain.TokenPair {
	t.Helper()
	pair, err := e.svc.Login(context.Background(), input.LoginCommand{Email: addr, Password: strongPassword, IPAddress: &ip})
	if err != nil {
		t.Fatal(err)
	}
	return pair
}

func (e *env) titles(t testing.TB, userID string) []string {
	t.Helper()
	items, _, err := e.account().Notifications(context.Background(), userID, domain.NewPage(1, 100))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(items))
	for _, n := range items {
		if n.Kind == "" {
			t.Fatalf("notification %q has no kind", n.Title)
		}
		out = append(out, n.Title)
	}
	sort.Strings(out)
	return out
}

// countByIP counts the failures that count against one client, any account.
func countByIP(ctx context.Context, ip string, since time.Time) (int, error) {
	n, err := postgres.NewStore(testDB).Attempts().CountRecentFailures(ctx, "", &ip, since, since)
	return n.IP, err
}

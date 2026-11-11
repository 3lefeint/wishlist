package users

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/3lefeint/wishlist/internal/db"
)

func newTestStore(t *testing.T, admins []string, maxPending int) *Store {
	t.Helper()
	url := os.Getenv("WISHLIST_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("WISHLIST_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE users RESTART IDENTITY"); err != nil {
		t.Fatal(err)
	}
	return NewStore(pool, admins, maxPending)
}

func TestSignInCreatesPendingUser(t *testing.T) {
	s := newTestStore(t, nil, 10)
	u, err := s.SignIn(context.Background(), "  Alice@Example.COM ")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "alice@example.com" || u.Status != StatusPending || u.IsAdmin {
		t.Fatalf("unexpected user %+v", u)
	}
	again, err := s.SignIn(context.Background(), "alice@example.com")
	if err != nil || again.ID != u.ID {
		t.Fatalf("second sign-in: %+v, %v", again, err)
	}
}

func TestSignInAdminFromConfig(t *testing.T) {
	s := newTestStore(t, []string{"Root@Example.com"}, 10)
	u, err := s.SignIn(context.Background(), "root@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u.Status != StatusApproved || !u.IsAdmin {
		t.Fatalf("unexpected user %+v", u)
	}
}

func TestSignInKeepsBlocked(t *testing.T) {
	s := newTestStore(t, nil, 10)
	ctx := context.Background()
	u, _ := s.SignIn(ctx, "bob@example.com")
	if _, err := s.pool.Exec(ctx, "UPDATE users SET status='blocked' WHERE id=$1", u.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.SignIn(ctx, "bob@example.com")
	if err != nil || got.Status != StatusBlocked {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestPendingLimit(t *testing.T) {
	s := newTestStore(t, nil, 2)
	ctx := context.Background()
	for _, e := range []string{"a@example.com", "b@example.com"} {
		if _, err := s.SignIn(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SignIn(ctx, "c@example.com"); !errors.Is(err, ErrPendingLimit) {
		t.Fatalf("err = %v, want ErrPendingLimit", err)
	}
	if _, err := s.SignIn(ctx, "a@example.com"); err != nil {
		t.Fatalf("known user must still sign in: %v", err)
	}
}

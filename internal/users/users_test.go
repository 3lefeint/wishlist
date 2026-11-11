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
	if _, err := pool.Exec(ctx, "TRUNCATE users, circles RESTART IDENTITY CASCADE"); err != nil {
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

func TestSetProfile(t *testing.T) {
	s := newTestStore(t, nil, 10)
	ctx := context.Background()
	u, err := s.SignIn(ctx, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u.ProfileComplete || u.Name != "alice" {
		t.Fatalf("fresh user: %+v", u)
	}
	good := Profile{FirstName: " Alice ", LastName: "Example", Birthday: "1985-12-24", FavoriteColor: "#FF8800"}
	got, err := s.SetProfile(ctx, u.ID, good)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Alice Example" || got.Birthday != "1985-12-24" || got.FavoriteColor != "#ff8800" || !got.ProfileComplete {
		t.Fatalf("profile: %+v", got)
	}
	good.FavoriteColor = ""
	if got, err = s.SetProfile(ctx, u.ID, good); err != nil || got.FavoriteColor != "" {
		t.Fatalf("colour is optional: %+v, %v", got, err)
	}
	for name, p := range map[string]Profile{
		"no first name": {LastName: "E", Birthday: "1985-12-24"},
		"no last name":  {FirstName: "A", Birthday: "1985-12-24"},
		"long name":     {FirstName: string(make([]rune, 81)), LastName: "E", Birthday: "1985-12-24"},
	} {
		if _, err := s.SetProfile(ctx, u.ID, p); !errors.Is(err, ErrInvalidName) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, b := range []string{"", "24.12.1985", "1899-12-31", "2999-01-01"} {
		if _, err := s.SetProfile(ctx, u.ID, Profile{FirstName: "A", LastName: "E", Birthday: b}); !errors.Is(err, ErrInvalidDate) {
			t.Errorf("birthday %q: %v", b, err)
		}
	}
	if _, err := s.SetProfile(ctx, u.ID, Profile{FirstName: "A", LastName: "E", Birthday: "1985-12-24", FavoriteColor: "red"}); !errors.Is(err, ErrInvalidColor) {
		t.Errorf("colour: %v", err)
	}
}

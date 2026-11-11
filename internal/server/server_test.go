package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/3lefeint/wishlist/internal/circles"
	"github.com/3lefeint/wishlist/internal/db"
	"github.com/3lefeint/wishlist/internal/gifts"
	"github.com/3lefeint/wishlist/internal/images"
	"github.com/3lefeint/wishlist/internal/users"
)

func newTestServer(t *testing.T) *httptest.Server {
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
	imgs, err := images.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Store:   users.NewStore(pool, []string{"root@example.com"}, 10),
		Circles: circles.NewStore(pool),
		Gifts:   gifts.NewStore(pool),
		Images:  imgs,
		Auth: func(r *http.Request) (string, error) {
			if e := r.Header.Get("X-Test-Email"); e != "" {
				return e, nil
			}
			return "", errors.New("no identity")
		},
		Ping: pool.Ping,
		Log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func call(t *testing.T, ts *httptest.Server, as, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	if as != "" {
		req.Header.Set("X-Test-Email", as)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestAdminFlow(t *testing.T) {
	ts := newTestServer(t)

	if code, _ := call(t, ts, "", "GET", "/api/me", ""); code != 401 {
		t.Fatalf("anonymous /api/me = %d, want 401", code)
	}
	if code, body := call(t, ts, "alice@example.com", "GET", "/api/me", ""); code != 200 || !strings.Contains(body, `"pending"`) {
		t.Fatalf("/api/me = %d %s", code, body)
	}
	if code, _ := call(t, ts, "alice@example.com", "GET", "/api/admin/users", ""); code != 403 {
		t.Fatalf("pending user on admin route = %d, want 403", code)
	}
	if code, _ := call(t, ts, "", "GET", "/api/admin/users", ""); code != 401 {
		t.Fatalf("anonymous admin route = %d, want 401", code)
	}

	// alice is user 1 (created above); root signs in as admin.
	code, body := call(t, ts, "root@example.com", "GET", "/api/admin/users?status=pending", "")
	if code != 200 || !strings.Contains(body, "alice@example.com") || strings.Contains(body, "root@example.com") {
		t.Fatalf("pending list = %d %s", code, body)
	}
	if code, _ := call(t, ts, "root@example.com", "GET", "/api/admin/users?status=bogus", ""); code != 400 {
		t.Fatalf("bad filter = %d, want 400", code)
	}

	if code, body := call(t, ts, "root@example.com", "PUT", "/api/admin/users/1/status", `{"status":"approved"}`); code != 200 || !strings.Contains(body, `"approved"`) {
		t.Fatalf("approve = %d %s", code, body)
	}
	if code, body := call(t, ts, "alice@example.com", "GET", "/api/me", ""); code != 200 || !strings.Contains(body, `"approved"`) {
		t.Fatalf("alice after approve = %d %s", code, body)
	}
	if code, _ := call(t, ts, "alice@example.com", "GET", "/api/admin/users", ""); code != 403 {
		t.Fatalf("approved non-admin on admin route = %d, want 403", code)
	}
	if code, _ := call(t, ts, "root@example.com", "PUT", "/api/admin/users/1/status", `{"status":"pending"}`); code != 400 {
		t.Fatalf("set pending = %d, want 400", code)
	}
	if code, _ := call(t, ts, "root@example.com", "PUT", "/api/admin/users/99/status", `{"status":"blocked"}`); code != 404 {
		t.Fatalf("unknown user = %d, want 404", code)
	}
	if code, _ := call(t, ts, "root@example.com", "PUT", "/api/admin/users/1/status", `{"status":"blocked"}`); code != 200 {
		t.Fatalf("block = %d", code)
	}

	adminID := 2 // root registered second
	if code, _ := call(t, ts, "root@example.com", "PUT", fmt.Sprintf("/api/admin/users/%d/status", adminID), `{"status":"blocked"}`); code != 409 {
		t.Fatalf("block admin = %d, want 409", code)
	}
	if code, _ := call(t, ts, "root@example.com", "DELETE", fmt.Sprintf("/api/admin/users/%d", adminID), ""); code != 409 {
		t.Fatalf("delete admin = %d, want 409", code)
	}
	if code, _ := call(t, ts, "root@example.com", "DELETE", "/api/admin/users/1", ""); code != 204 {
		t.Fatalf("delete = %d, want 204", code)
	}
	if code, _ := call(t, ts, "root@example.com", "DELETE", "/api/admin/users/1", ""); code != 404 {
		t.Fatalf("delete again = %d, want 404", code)
	}
}

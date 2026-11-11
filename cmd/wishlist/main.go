// Command wishlist is the Wishlist backend.
//
// Without arguments it serves the API. "wishlist migrate" applies the database
// migrations and exits; it needs the migration role in WISHLIST_DATABASE_URL.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/3lefeint/wishlist/internal/access"
	"github.com/3lefeint/wishlist/internal/db"
	"github.com/3lefeint/wishlist/internal/users"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dbURL := os.Getenv("WISHLIST_DATABASE_URL")
	if dbURL == "" {
		log.Error("WISHLIST_DATABASE_URL is required")
		os.Exit(1)
	}

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := db.Migrate(context.Background(), dbURL); err != nil {
			log.Error("migrate", "err", err)
			os.Exit(1)
		}
		log.Info("migrations applied")
		return
	}

	team := os.Getenv("WISHLIST_ACCESS_TEAM_DOMAIN")
	aud := os.Getenv("WISHLIST_ACCESS_AUD")
	if team == "" || aud == "" {
		log.Error("WISHLIST_ACCESS_TEAM_DOMAIN and WISHLIST_ACCESS_AUD are required")
		os.Exit(1)
	}
	addr := os.Getenv("WISHLIST_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	maxPending := 50
	if v := os.Getenv("WISHLIST_MAX_PENDING"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			log.Error("WISHLIST_MAX_PENDING must be a non-negative integer")
			os.Exit(1)
		}
		maxPending = n
	}
	admins := strings.Split(os.Getenv("WISHLIST_ADMIN_EMAILS"), ",")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := users.NewStore(pool, admins, maxPending)
	verifier := access.NewVerifier(team, aud)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /api/me", verifier.Middleware(func(w http.ResponseWriter, r *http.Request, c *access.Claims) {
		u, err := store.SignIn(r.Context(), c.Email)
		if errors.Is(err, users.ErrPendingLimit) {
			http.Error(w, "registration temporarily closed", http.StatusServiceUnavailable)
			return
		}
		if err != nil {
			log.Error("sign in", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(u)
	}))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}

// Command wishlist is the Wishlist backend.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/3lefeint/wishlist/internal/access"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

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

	verifier := access.NewVerifier(team, aud)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /api/me", verifier.Middleware(func(w http.ResponseWriter, r *http.Request, c *access.Claims) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"email": c.Email})
	}))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	log.Info("listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}

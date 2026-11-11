// Package server wires the HTTP API.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/3lefeint/wishlist/internal/circles"
	"github.com/3lefeint/wishlist/internal/gifts"
	"github.com/3lefeint/wishlist/internal/images"
	"github.com/3lefeint/wishlist/internal/users"
)

// Server serves the API.
type Server struct {
	Store   *users.Store
	Circles *circles.Store
	Gifts   *gifts.Store
	// Images stores gift pictures; nil disables uploads.
	Images *images.Store
	// Auth returns the verified e-mail address of the request or an error.
	Auth func(*http.Request) (string, error)
	// Ping checks the database.
	Ping func(context.Context) error
	Log  *slog.Logger
	// Files holds the built frontend; nil serves no frontend.
	Files fs.FS
}

// Handler returns the routes of the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /api/me", s.signedIn(func(w http.ResponseWriter, r *http.Request, u users.User) {
		writeJSON(w, http.StatusOK, u)
	}))
	mux.Handle("PUT /api/me", s.signedIn(s.setProfile))
	s.circleRoutes(mux)
	s.giftRoutes(mux)
	mux.Handle("GET /api/admin/users", s.admin(s.listUsers))
	mux.Handle("PUT /api/admin/users/{id}/status", s.admin(s.setStatus))
	mux.Handle("DELETE /api/admin/users/{id}", s.admin(s.deleteUser))
	if s.Files != nil {
		mux.Handle("GET /", spa(s.Files))
	}
	return mux
}

// signedIn authenticates the request and registers the user on first sight.
func (s *Server) signedIn(next func(http.ResponseWriter, *http.Request, users.User)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email, err := s.Auth(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		u, err := s.Store.SignIn(r.Context(), email)
		if errors.Is(err, users.ErrPendingLimit) {
			http.Error(w, "registration temporarily closed", http.StatusServiceUnavailable)
			return
		}
		if err != nil {
			s.internal(w, "sign in", err)
			return
		}
		next(w, r, u)
	})
}

// admin lets only approved administrators through.
func (s *Server) admin(next func(http.ResponseWriter, *http.Request, users.User)) http.Handler {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request, u users.User) {
		if !u.IsAdmin || u.Status != users.StatusApproved {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r, u)
	})
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, _ users.User) {
	status := r.URL.Query().Get("status")
	switch status {
	case "", users.StatusPending, users.StatusApproved, users.StatusBlocked:
	default:
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	list, err := s.Store.List(r.Context(), status)
	if err != nil {
		s.internal(w, "list users", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) setStatus(w http.ResponseWriter, r *http.Request, _ users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, err := s.Store.SetStatus(r.Context(), id, body.Status)
	if s.storeError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request, _ users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.storeError(w, s.Store.Delete(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// storeError answers for a failed store call and reports whether it did.
func (s *Server) storeError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, users.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, users.ErrAdmin):
		http.Error(w, "administrators cannot be changed", http.StatusConflict)
	case errors.Is(err, users.ErrInvalidStatus):
		http.Error(w, "invalid status", http.StatusBadRequest)
	default:
		s.internal(w, "store", err)
	}
	return true
}

func (s *Server) internal(w http.ResponseWriter, what string, err error) {
	s.Log.Error(what, "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) { return pathInt(w, r, "id") }

func pathInt(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) setProfile(w http.ResponseWriter, r *http.Request, u users.User) {
	var body users.Profile
	if !decode(w, r, &body) {
		return
	}
	updated, err := s.Store.SetProfile(r.Context(), u.ID, body)
	switch {
	case errors.Is(err, users.ErrInvalidName), errors.Is(err, users.ErrInvalidDate), errors.Is(err, users.ErrInvalidColor):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		s.internal(w, "set profile", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// member lets only approved users through.
func (s *Server) member(next func(http.ResponseWriter, *http.Request, users.User)) http.Handler {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request, u users.User) {
		if u.Status != users.StatusApproved {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r, u)
	})
}

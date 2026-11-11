package server

import (
	"io"
	"net/http"
	"os"

	"github.com/3lefeint/wishlist/internal/users"
)

func (s *Server) adminContentRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/admin/circles", s.admin(s.adminListCircles))
	mux.Handle("GET /api/admin/circles/{id}", s.admin(s.adminGetCircle))
	mux.Handle("DELETE /api/admin/circles/{id}", s.admin(s.adminDeleteCircle))
	mux.Handle("POST /api/admin/circles/{id}/members", s.admin(s.adminAddMember))
	mux.Handle("PUT /api/admin/circles/{id}/members/{userID}", s.admin(s.adminSetRole))
	mux.Handle("DELETE /api/admin/circles/{id}/members/{userID}", s.admin(s.adminRemoveMember))
	mux.Handle("GET /api/admin/circles/{id}/gifts", s.admin(s.adminListGifts))
	mux.Handle("PUT /api/admin/gifts/{id}/active", s.admin(s.adminSetActive))
	mux.Handle("DELETE /api/admin/gifts/{id}", s.admin(s.adminDeleteGift))
	mux.Handle("GET /api/admin/gifts/{id}/thumb", s.admin(s.adminThumb))
}

func (s *Server) adminListCircles(w http.ResponseWriter, r *http.Request, _ users.User) {
	list, err := s.Circles.AdminList(r.Context())
	if s.circleError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) adminGetCircle(w http.ResponseWriter, r *http.Request, _ users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := s.Circles.AdminGet(r.Context(), id)
	if s.circleError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) adminDeleteCircle(w http.ResponseWriter, r *http.Request, _ users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.circleError(w, s.Circles.AdminDelete(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminAddMember(w http.ResponseWriter, r *http.Request, _ users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		UserID int64  `json:"user_id"`
		Role   string `json:"role"`
	}
	if !decode(w, r, &body) {
		return
	}
	if s.circleError(w, s.Circles.AdminAddMember(r.Context(), id, body.UserID, body.Role)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminSetRole(w http.ResponseWriter, r *http.Request, _ users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	target, ok := pathInt(w, r, "userID")
	if !ok {
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if !decode(w, r, &body) {
		return
	}
	if s.circleError(w, s.Circles.AdminSetRole(r.Context(), id, target, body.Role)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminRemoveMember(w http.ResponseWriter, r *http.Request, _ users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	target, ok := pathInt(w, r, "userID")
	if !ok {
		return
	}
	if s.circleError(w, s.Circles.AdminRemoveMember(r.Context(), id, target)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminListGifts(w http.ResponseWriter, r *http.Request, _ users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := s.Gifts.AdminList(r.Context(), id)
	if s.giftError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) adminSetActive(w http.ResponseWriter, r *http.Request, _ users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Active bool `json:"active"`
	}
	if !decode(w, r, &body) {
		return
	}
	if s.giftError(w, s.Gifts.AdminSetActive(r.Context(), id, body.Active)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminDeleteGift(w http.ResponseWriter, r *http.Request, _ users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	had, err := s.Gifts.AdminDelete(r.Context(), id)
	if s.giftError(w, err) {
		return
	}
	if had && s.Images != nil {
		s.Images.Remove(id)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminThumb(w http.ResponseWriter, r *http.Request, _ users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.Images == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	f, err := os.Open(s.Images.Path(id, true))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		s.internal(w, "stat image", err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, no-cache")
	http.ServeContent(w, r, "", st.ModTime(), io.ReadSeeker(f))
}

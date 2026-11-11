package server

import (
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/3lefeint/wishlist/internal/circles"
	"github.com/3lefeint/wishlist/internal/gifts"
	"github.com/3lefeint/wishlist/internal/images"
	"github.com/3lefeint/wishlist/internal/users"
)

func (s *Server) giftRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/persons/{id}", s.member(s.getPerson))
	mux.Handle("GET /api/persons/{id}/gifts", s.member(s.listGifts))
	mux.Handle("POST /api/persons/{id}/gifts", s.member(s.createGift))
	mux.Handle("PUT /api/gifts/{id}", s.member(s.updateGift))
	mux.Handle("PUT /api/gifts/{id}/active", s.member(s.setActive))
	mux.Handle("DELETE /api/gifts/{id}", s.member(s.deleteGift))
	mux.Handle("PUT /api/gifts/{id}/claim", s.member(s.claimGift))
	mux.Handle("DELETE /api/gifts/{id}/claim", s.member(s.unclaimGift))
	mux.Handle("POST /api/gifts/{id}/image", s.member(s.uploadImage))
	mux.Handle("DELETE /api/gifts/{id}/image", s.member(s.deleteImage))
	mux.Handle("GET /api/gifts/{id}/image", s.member(s.serveImage(false)))
	mux.Handle("GET /api/gifts/{id}/thumb", s.member(s.serveImage(true)))
	mux.Handle("GET /api/circles/{id}/tags", s.member(s.listTags))
}

func (s *Server) giftError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, gifts.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, gifts.ErrForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, gifts.ErrInvalid), errors.Is(err, images.ErrInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, gifts.ErrClaimed), errors.Is(err, gifts.ErrInactive), errors.Is(err, gifts.ErrOwnList):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		s.internal(w, "gifts", err)
	}
	return true
}

func (s *Server) getPerson(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := s.Circles.GetPerson(r.Context(), u.ID, id)
	if errors.Is(err, circles.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		s.internal(w, "get person", err)
		return
	}
	// Members are needed to name the guardians.
	d, err := s.Circles.Get(r.Context(), u.ID, p.CircleID)
	if err != nil {
		s.internal(w, "get circle", err)
		return
	}
	p.Members = d.Members
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) listGifts(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := s.Gifts.List(r.Context(), u.ID, id)
	if s.giftError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createGift(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in gifts.Input
	if !decode(w, r, &in) {
		return
	}
	gid, err := s.Gifts.Create(r.Context(), u.ID, id, in)
	if s.giftError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": gid})
}

func (s *Server) updateGift(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in gifts.Input
	if !decode(w, r, &in) {
		return
	}
	if s.giftError(w, s.Gifts.Update(r.Context(), u.ID, id, in)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setActive(w http.ResponseWriter, r *http.Request, u users.User) {
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
	if s.giftError(w, s.Gifts.SetActive(r.Context(), u.ID, id, body.Active)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteGift(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	had, err := s.Gifts.Delete(r.Context(), u.ID, id)
	if s.giftError(w, err) {
		return
	}
	if had && s.Images != nil {
		s.Images.Remove(id)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) claimGift(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.giftError(w, s.Gifts.Claim(r.Context(), u.ID, id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) unclaimGift(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.giftError(w, s.Gifts.Unclaim(r.Context(), u.ID, id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) uploadImage(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.Images == nil {
		http.Error(w, "image storage unavailable", http.StatusServiceUnavailable)
		return
	}
	// Check the permission before reading the body.
	if _, err := s.Gifts.ImageAccess(r.Context(), u.ID, id); s.giftError(w, err) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, images.MaxUpload+(1<<20))
	file, _, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "invalid upload", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if err := s.Images.Save(id, file); err != nil {
		s.giftError(w, err)
		return
	}
	if s.giftError(w, s.Gifts.SetImage(r.Context(), u.ID, id, true)) {
		s.Images.Remove(id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteImage(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.giftError(w, s.Gifts.SetImage(r.Context(), u.ID, id, false)) {
		return
	}
	if s.Images != nil {
		s.Images.Remove(id)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveImage(thumb bool) func(http.ResponseWriter, *http.Request, users.User) {
	return func(w http.ResponseWriter, r *http.Request, u users.User) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		has, err := s.Gifts.ImageAccess(r.Context(), u.ID, id)
		if s.giftError(w, err) {
			return
		}
		if !has || s.Images == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		f, err := os.Open(s.Images.Path(id, thumb))
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
}

func (s *Server) listTags(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tags, err := s.Gifts.Tags(r.Context(), u.ID, id)
	if s.giftError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

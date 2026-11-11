package server

import (
	"errors"
	"net/http"

	"github.com/3lefeint/wishlist/internal/circles"
	"github.com/3lefeint/wishlist/internal/users"
)

func (s *Server) circleRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/circles", s.member(s.listCircles))
	mux.Handle("POST /api/circles", s.member(s.createCircle))
	mux.Handle("GET /api/circles/{id}", s.member(s.getCircle))
	mux.Handle("DELETE /api/circles/{id}", s.member(s.deleteCircle))
	mux.Handle("DELETE /api/circles/{id}/members/{userID}", s.member(s.removeMember))
	mux.Handle("POST /api/circles/{id}/invite", s.member(s.createInvite))
	mux.Handle("DELETE /api/circles/{id}/invite", s.member(s.revokeInvite))
	mux.Handle("GET /api/invites/{token}", s.member(s.inviteInfo))
	mux.Handle("POST /api/invites/{token}/join", s.member(s.joinCircle))
	mux.Handle("POST /api/circles/{id}/persons", s.member(s.createPerson))
	mux.Handle("PUT /api/persons/{id}", s.member(s.updatePerson))
	mux.Handle("DELETE /api/persons/{id}", s.member(s.deletePerson))
	mux.Handle("POST /api/persons/{id}/guardians", s.member(s.addGuardian))
	mux.Handle("DELETE /api/persons/{id}/guardians/{userID}", s.member(s.removeGuardian))
}

// circleError answers for a failed circle call and reports whether it did.
func (s *Server) circleError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, circles.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, circles.ErrForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, circles.ErrInvalidName):
		http.Error(w, "invalid name", http.StatusBadRequest)
	case errors.Is(err, users.ErrInvalidDate):
		http.Error(w, "invalid birthday", http.StatusBadRequest)
	case errors.Is(err, circles.ErrInvite):
		http.Error(w, "invite link is invalid or expired", http.StatusNotFound)
	case errors.Is(err, circles.ErrNotMember):
		http.Error(w, "user is not a member of the circle", http.StatusBadRequest)
	case errors.Is(err, circles.ErrLastAdmin), errors.Is(err, circles.ErrLinked):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		s.internal(w, "circles", err)
	}
	return true
}

func (s *Server) listCircles(w http.ResponseWriter, r *http.Request, u users.User) {
	list, err := s.Circles.List(r.Context(), u.ID)
	if s.circleError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createCircle(w http.ResponseWriter, r *http.Request, u users.User) {
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	c, err := s.Circles.Create(r.Context(), u.ID, body.Name)
	if s.circleError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) getCircle(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := s.Circles.Get(r.Context(), u.ID, id)
	if s.circleError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) deleteCircle(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.circleError(w, s.Circles.Delete(r.Context(), u.ID, id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	target, ok := pathInt(w, r, "userID")
	if !ok {
		return
	}
	if s.circleError(w, s.Circles.RemoveMember(r.Context(), u.ID, id, target)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	inv, err := s.Circles.CreateInvite(r.Context(), u.ID, id)
	if s.circleError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.circleError(w, s.Circles.RevokeInvite(r.Context(), u.ID, id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) inviteInfo(w http.ResponseWriter, r *http.Request, u users.User) {
	id, name, err := s.Circles.InviteInfo(r.Context(), r.PathValue("token"))
	if s.circleError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"circle_id": id, "name": name})
}

func (s *Server) joinCircle(w http.ResponseWriter, r *http.Request, u users.User) {
	id, err := s.Circles.Join(r.Context(), u.ID, r.PathValue("token"))
	if s.circleError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"circle_id": id})
}

func (s *Server) createPerson(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name     string `json:"name"`
		Birthday string `json:"birthday"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, err := s.Circles.CreatePerson(r.Context(), u.ID, id, body.Name, body.Birthday)
	if s.circleError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) updatePerson(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name     string `json:"name"`
		Birthday string `json:"birthday"`
	}
	if !decode(w, r, &body) {
		return
	}
	if s.circleError(w, s.Circles.UpdatePerson(r.Context(), u.ID, id, body.Name, body.Birthday)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deletePerson(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.circleError(w, s.Circles.DeletePerson(r.Context(), u.ID, id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addGuardian(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		UserID int64 `json:"user_id"`
	}
	if !decode(w, r, &body) {
		return
	}
	if s.circleError(w, s.Circles.AddGuardian(r.Context(), u.ID, id, body.UserID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeGuardian(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	target, ok := pathInt(w, r, "userID")
	if !ok {
		return
	}
	if s.circleError(w, s.Circles.RemoveGuardian(r.Context(), u.ID, id, target)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

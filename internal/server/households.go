package server

import (
	"net/http"

	"github.com/3lefeint/wishlist/internal/users"
)

func (s *Server) createHousehold(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	h, err := s.Circles.CreateHousehold(r.Context(), u.ID, id, body.Name)
	if s.circleError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, h)
}

func (s *Server) renameHousehold(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	if s.circleError(w, s.Circles.RenameHousehold(r.Context(), u.ID, id, body.Name)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteHousehold(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.circleError(w, s.Circles.DeleteHousehold(r.Context(), u.ID, id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setPersonHousehold reads {"household_id": <id or null>}; the field must be present.
func (s *Server) setPersonHousehold(w http.ResponseWriter, r *http.Request, u users.User) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body map[string]*int64
	if !decode(w, r, &body) {
		return
	}
	hid, present := body["household_id"]
	if !present || (hid != nil && *hid <= 0) {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if s.circleError(w, s.Circles.SetPersonHousehold(r.Context(), u.ID, id, hid)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

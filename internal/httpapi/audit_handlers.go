package httpapi

import (
	"net/http"
)

func (s *Server) handleListChanges(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	entityType := r.URL.Query().Get("entity_type")
	entityID := r.URL.Query().Get("entity_id")
	out, err := s.svc.ListChanges(r.Context(), limit, offset, entityType, entityID)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

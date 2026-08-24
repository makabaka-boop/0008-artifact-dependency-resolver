package httpapi

import (
	"net/http"
	"strconv"

	"artifact-resolver/internal/errcode"
)

func (s *Server) handleCompareVersionDependencies(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	out, err := s.svc.CompareVersionDependencies(r.Context(), name, from, to)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCheckReadiness(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	version := r.PathValue("version")
	out, err := s.svc.CheckReadiness(r.Context(), name, version)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRerunResolution(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeErr(w, errcode.CodeInvalidArgument, "invalid resolution id")
		return
	}
	out, err := s.svc.RerunResolution(r.Context(), id)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

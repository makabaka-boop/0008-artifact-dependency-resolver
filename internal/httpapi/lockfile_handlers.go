package httpapi

import (
	"net/http"
	"strconv"

	"artifact-resolver/internal/errcode"
)

func (s *Server) handleGetLockfileByRequest(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeErr(w, errcode.CodeInvalidArgument, "invalid resolution id")
		return
	}
	out, err := s.svc.GetLockfileByRequest(r.Context(), id)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetLockfileByRef(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	out, err := s.svc.GetLockfileByRef(r.Context(), ref)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

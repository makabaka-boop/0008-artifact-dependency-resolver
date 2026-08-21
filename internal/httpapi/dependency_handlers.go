package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/service"
)

func (s *Server) handleReplaceDependencies(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	version := r.PathValue("version")
	var req struct {
		Dependencies []service.DependencyItem `json:"dependencies"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, errcode.CodeInvalidArgument, "invalid request body")
		return
	}
	deps, err := s.svc.ReplaceDependencies(r.Context(), name, version, req.Dependencies)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	// 复用服务层缓存引擎，将当前累积的解析图落库为审计记录。
	if err := s.svc.PersistResolverState(r.Context(), name, version); err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dependencies": deps})
}

func (s *Server) handleListDependencies(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	version := r.PathValue("version")
	deps, err := s.svc.ListDependencies(r.Context(), name, version)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dependencies": deps})
}

func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Manifest []service.ManifestItem `json:"manifest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, errcode.CodeInvalidManifest, "invalid manifest")
		return
	}
	out, err := s.svc.Resolve(r.Context(), req.Manifest)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListResolutions(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	status := r.URL.Query().Get("status")
	rs, err := s.svc.ListResolutions(r.Context(), limit, offset, status)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) handleGetResolution(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeErr(w, errcode.CodeInvalidArgument, "invalid resolution id")
		return
	}
	req, nodes, err := s.svc.GetResolution(r.Context(), id)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"request": req,
		"nodes":   nodes,
	})
}

func (s *Server) handleCompare(w http.ResponseWriter, r *http.Request) {
	left := r.URL.Query().Get("left")
	right := r.URL.Query().Get("right")
	result, err := s.svc.Compare(r.Context(), left, right)
	if err != nil {
		writeAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"left": left, "right": right, "result": result})
}

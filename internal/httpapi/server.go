package httpapi

import (
	"encoding/json"
	"net/http"

	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/service"
)

// Server 承载 HTTP 路由与处理器。
type Server struct {
	svc *service.Service
	mux *http.ServeMux
}

// NewServer 创建 HTTP 服务。
func NewServer(svc *service.Service) *Server {
	s := &Server{svc: svc, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler 返回根 handler。
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /artifacts", s.handleCreateArtifact)
	s.mux.HandleFunc("GET /artifacts", s.handleListArtifacts)
	s.mux.HandleFunc("GET /artifacts/{name}", s.handleGetArtifact)
	s.mux.HandleFunc("POST /artifacts/{name}/versions", s.handleCreateVersion)
	s.mux.HandleFunc("GET /artifacts/{name}/versions", s.handleListVersions)
	s.mux.HandleFunc("POST /artifacts/{name}/versions/{version}/publish", s.handlePublishVersion)
	s.mux.HandleFunc("POST /artifacts/{name}/versions/{version}/deprecate", s.handleDeprecateVersion)
	s.mux.HandleFunc("DELETE /artifacts/{name}/versions/{version}", s.handleDeleteVersion)
	s.mux.HandleFunc("PUT /artifacts/{name}/versions/{version}/dependencies", s.handleReplaceDependencies)
	s.mux.HandleFunc("GET /artifacts/{name}/versions/{version}/dependencies", s.handleListDependencies)
	s.mux.HandleFunc("POST /resolve", s.handleResolve)
	s.mux.HandleFunc("GET /resolutions", s.handleListResolutions)
	s.mux.HandleFunc("GET /resolutions/{id}", s.handleGetResolution)
	s.mux.HandleFunc("POST /resolutions/{id}/rerun", s.handleRerunResolution)
	s.mux.HandleFunc("GET /resolutions/{id}/lockfile", s.handleGetLockfileByRequest)
	s.mux.HandleFunc("GET /lockfiles/{ref}", s.handleGetLockfileByRef)
	s.mux.HandleFunc("GET /changes", s.handleListChanges)
	s.mux.HandleFunc("GET /artifacts/{name}/versions/diff", s.handleCompareVersionDependencies)
	s.mux.HandleFunc("GET /artifacts/{name}/versions/{version}/readiness", s.handleCheckReadiness)
	s.mux.HandleFunc("GET /compare", s.handleCompare)
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr 输出统一错误信封。
func writeErr(w http.ResponseWriter, code errcode.Code, msg string) {
	writeJSON(w, code.HTTPStatus(), map[string]any{
		"error": map[string]string{"code": string(code), "message": msg},
	})
}

func writeAPIErr(w http.ResponseWriter, err error) {
	if ae, ok := err.(*service.APIError); ok {
		writeErr(w, ae.Code, ae.Message)
		return
	}
	writeErr(w, errcode.CodeInternal, err.Error())
}

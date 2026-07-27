package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neonet/codex-continuity/internal/model"
)

type contextKey string

const userContextKey contextKey = "user"

type HTTPServer struct {
	cfg   Config
	store *Store
	log   *slog.Logger
}

func NewHTTPServer(cfg Config, store *Store, logger *slog.Logger) http.Handler {
	s := &HTTPServer{cfg: cfg, store: store, log: logger}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.Handle("POST /api/v1/auth/logout", s.withSession(http.HandlerFunc(s.logout)))
	mux.Handle("GET /api/v1/me", s.withSession(http.HandlerFunc(s.me)))
	mux.Handle("GET /api/v1/overview", s.withSession(http.HandlerFunc(s.overview)))
	mux.Handle("GET /api/v1/devices", s.withSession(http.HandlerFunc(s.devices)))
	mux.Handle("GET /api/v1/handoffs", s.withSession(http.HandlerFunc(s.handoffs)))
	mux.Handle("GET /api/v1/tokens", s.withSession(http.HandlerFunc(s.tokens)))
	mux.Handle("POST /api/v1/tokens", s.withSession(http.HandlerFunc(s.createToken)))
	mux.Handle("DELETE /api/v1/tokens/{id}", s.withSession(http.HandlerFunc(s.deleteToken)))
	mux.Handle("GET /api/v1/users", s.withAdmin(http.HandlerFunc(s.users)))
	mux.Handle("POST /api/v1/users", s.withAdmin(http.HandlerFunc(s.createUser)))

	mux.Handle("POST /api/v1/client/devices", s.withAPIToken(http.HandlerFunc(s.registerDevice)))
	mux.Handle("GET /api/v1/client/handoffs", s.withAPIToken(http.HandlerFunc(s.clientHandoffs)))
	mux.Handle("POST /api/v1/client/handoffs", s.withAPIToken(http.HandlerFunc(s.uploadHandoff)))
	mux.Handle("GET /api/v1/client/handoffs/{id}/blob", s.withAPIToken(http.HandlerFunc(s.downloadHandoff)))
	mux.Handle("POST /api/v1/client/handoffs/{id}/claim", s.withAPIToken(http.HandlerFunc(s.claimHandoff)))

	if info, err := os.Stat(cfg.DownloadDir); err == nil && info.IsDir() {
		mux.Handle("/downloads/", http.StripPrefix("/downloads/", http.FileServer(http.Dir(cfg.DownloadDir))))
	}
	mux.Handle("/", s.spaHandler())
	return s.recoverer(s.securityHeaders(s.requestLog(mux)))
}

func (s *HTTPServer) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "codex-continuity",
		"time":    time.Now().UTC(),
	})
}

func (s *HTTPServer) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	user, err := s.store.Authenticate(input.Email, input.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "邮箱或密码不正确")
		return
	}
	token, err := s.store.CreateSession(user.ID, s.cfg.SessionTTL)
	if err != nil {
		s.internalError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "continuity_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *HTTPServer) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("continuity_session"); err == nil {
		_ = s.store.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "continuity_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *HTTPServer) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": currentUser(r)})
}

func (s *HTTPServer) overview(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.Overview(currentUser(r).ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *HTTPServer) devices(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.ListDevices(currentUser(r).ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": result})
}

func (s *HTTPServer) handoffs(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.ListHandoffs(currentUser(r).ID, 100)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"handoffs": result})
}

func (s *HTTPServer) users(w http.ResponseWriter, _ *http.Request) {
	result, err := s.store.ListUsers()
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": result})
}

func (s *HTTPServer) createUser(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
		Role        string `json:"role"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	user, err := s.store.CreateUser(input.Email, input.DisplayName, input.Password, input.Role)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *HTTPServer) tokens(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.ListTokens(currentUser(r).ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": result})
}

func (s *HTTPServer) createToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	token, plain, err := s.store.CreateToken(currentUser(r).ID, input.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "secret": plain})
}

func (s *HTTPServer) deleteToken(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteToken(currentUser(r).ID, r.PathValue("id")); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "令牌不存在")
			return
		}
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *HTTPServer) registerDevice(w http.ResponseWriter, r *http.Request) {
	var input model.Device
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "设备名称不能为空")
		return
	}
	result, err := s.store.UpsertDevice(currentUser(r).ID, input)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device": result})
}

func (s *HTTPServer) clientHandoffs(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.ListHandoffs(currentUser(r).ID, 100)
	if err != nil {
		s.internalError(w, err)
		return
	}
	target := strings.TrimSpace(r.URL.Query().Get("target"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	filtered := make([]model.Handoff, 0, len(result))
	for _, h := range result {
		if status != "" && h.Status != status {
			continue
		}
		if target != "" && h.TargetDeviceName != "" && !strings.EqualFold(h.TargetDeviceName, target) {
			continue
		}
		filtered = append(filtered, h)
	}
	writeJSON(w, http.StatusOK, map[string]any{"handoffs": filtered})
}

type uploadMetadata struct {
	ProjectName      string          `json:"projectName"`
	WorkspaceKey     string          `json:"workspaceKey"`
	SourceDeviceID   string          `json:"sourceDeviceId"`
	TargetDeviceName string          `json:"targetDeviceName"`
	Manifest         json.RawMessage `json:"manifest"`
}

func (s *HTTPServer) uploadHandoff(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "上传包无效或超过大小限制")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	var metadata uploadMetadata
	if raw := r.FormValue("metadata"); raw == "" || json.Unmarshal([]byte(raw), &metadata) != nil {
		writeError(w, http.StatusBadRequest, "metadata 无效")
		return
	}
	user := currentUser(r)
	if !s.store.DeviceOwnedBy(user.ID, metadata.SourceDeviceID) {
		writeError(w, http.StatusBadRequest, "来源设备无效")
		return
	}
	file, header, err := r.FormFile("blob")
	if err != nil {
		writeError(w, http.StatusBadRequest, "缺少加密 blob")
		return
	}
	defer file.Close()
	if header.Size <= 0 {
		writeError(w, http.StatusBadRequest, "加密 blob 为空")
		return
	}
	blobName := newID() + ".ccx"
	relativePath := filepath.Join("blobs", blobName)
	absolutePath := filepath.Join(s.cfg.DataDir, relativePath)
	size, err := saveUpload(file, absolutePath)
	if err != nil {
		s.internalError(w, err)
		return
	}
	handoff, err := s.store.CreateHandoff(CreateHandoffParams{
		UserID:           user.ID,
		ProjectName:      metadata.ProjectName,
		WorkspaceKey:     metadata.WorkspaceKey,
		SourceDeviceID:   metadata.SourceDeviceID,
		TargetDeviceName: metadata.TargetDeviceName,
		Manifest:         metadata.Manifest,
		BlobPath:         relativePath,
		BlobSize:         size,
	})
	if err != nil {
		_ = os.Remove(absolutePath)
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"handoff": handoff})
}

func (s *HTTPServer) downloadHandoff(w http.ResponseWriter, r *http.Request) {
	handoff, relativePath, err := s.store.HandoffByID(currentUser(r).ID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "交接包不存在")
			return
		}
		s.internalError(w, err)
		return
	}
	path := filepath.Join(s.cfg.DataDir, filepath.Clean(relativePath))
	if !strings.HasPrefix(path, s.cfg.DataDir+string(os.PathSeparator)) {
		writeError(w, http.StatusInternalServerError, "交接包路径无效")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.ccx"`, handoff.ID))
	http.ServeFile(w, r, path)
}

func (s *HTTPServer) claimHandoff(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TargetDeviceName string `json:"targetDeviceName"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := s.store.ClaimHandoff(currentUser(r).ID, r.PathValue("id"), input.TargetDeviceName); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "交接包不存在")
			return
		}
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *HTTPServer) withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("continuity_session")
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		user, err := s.store.UserBySession(cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "登录已失效")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

func (s *HTTPServer) withAdmin(next http.Handler) http.Handler {
	return s.withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).Role != model.RoleAdmin {
			writeError(w, http.StatusForbidden, "需要管理员权限")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (s *HTTPServer) withAPIToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "缺少客户端令牌")
			return
		}
		user, err := s.store.UserByAPIToken(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "客户端令牌无效")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

func currentUser(r *http.Request) model.User {
	user, _ := r.Context().Value(userContextKey).(model.User)
	return user
}

func (s *HTTPServer) spaHandler() http.Handler {
	webDir, err := filepath.Abs(s.cfg.WebDir)
	if err != nil {
		webDir = s.cfg.WebDir
	}
	fileServer := http.FileServer(http.Dir(webDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(webDir, filepath.Clean(strings.TrimPrefix(r.URL.Path, "/")))
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		index := filepath.Join(webDir, "index.html")
		if _, err := os.Stat(index); err != nil {
			writeJSON(w, http.StatusOK, map[string]string{
				"message": "Codex Continuity API 正在运行，管理端尚未构建",
			})
			return
		}
		http.ServeFile(w, r, index)
	})
}

func (s *HTTPServer) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func (s *HTTPServer) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		if !strings.HasPrefix(r.URL.Path, "/assets/") {
			s.log.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
		}
	})
}

func (s *HTTPServer) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.Error("panic", "error", recovered)
				writeError(w, http.StatusInternalServerError, "服务内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *HTTPServer) internalError(w http.ResponseWriter, err error) {
	s.log.Error("request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "服务内部错误")
}

func saveUpload(src multipart.File, destination string) (int64, error) {
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	size, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return 0, copyErr
	}
	return size, closeErr
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "请求数据格式不正确")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

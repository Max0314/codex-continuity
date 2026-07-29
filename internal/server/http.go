package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neonet/codex-continuity/internal/model"
)

type contextKey string

const userContextKey contextKey = "user"

const (
	uploadMultipartOverheadBytes = 8 << 20
	maxUploadMetadataBytes       = 8 << 20
)

var errUploadTooLarge = errors.New("upload exceeds configured blob limit")

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
	mux.HandleFunc("POST /api/v1/client/auth/register", s.clientRegister)
	mux.HandleFunc("POST /api/v1/client/auth/login", s.clientLogin)
	mux.HandleFunc("POST /api/v1/client/auth/recover", s.clientRecover)
	mux.HandleFunc("POST /api/v1/client/auth/refresh", s.clientRefresh)
	mux.HandleFunc("POST /api/v1/client/auth/logout", s.clientLogout)
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

	mux.Handle("POST /api/v1/client/devices", s.withClientAuth(http.HandlerFunc(s.registerDevice)))
	mux.Handle("POST /api/v1/client/diagnostics/upload-test", s.withClientAuth(http.HandlerFunc(s.uploadTest)))
	mux.Handle("GET /api/v1/client/handoffs", s.withClientAuth(http.HandlerFunc(s.clientHandoffs)))
	mux.Handle("POST /api/v1/client/handoffs", s.withClientAuth(http.HandlerFunc(s.uploadHandoff)))
	mux.Handle("GET /api/v1/client/handoffs/{id}/blob", s.withClientAuth(http.HandlerFunc(s.downloadHandoff)))
	mux.Handle("POST /api/v1/client/handoffs/{id}/claim", s.withClientAuth(http.HandlerFunc(s.claimHandoff)))

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
		Email      string `json:"email"`
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	identifier := strings.TrimSpace(input.Identifier)
	if identifier == "" {
		identifier = input.Email
	}
	user, err := s.store.Authenticate(identifier, input.Password)
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

func (s *HTTPServer) clientRegister(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username        string `json:"username"`
		DisplayName     string `json:"displayName"`
		Password        string `json:"password"`
		KeySalt         string `json:"keySalt"`
		WrappedKey      string `json:"wrappedKey"`
		RecoveryKeyHash string `json:"recoveryKeyHash"`
		LegacyToken     string `json:"legacyToken"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	account, err := s.store.RegisterAccount(RegisterAccountParams{
		Username:        input.Username,
		DisplayName:     input.DisplayName,
		Password:        input.Password,
		KeySalt:         input.KeySalt,
		WrappedKey:      input.WrappedKey,
		RecoveryKeyHash: input.RecoveryKeyHash,
		LegacyToken:     input.LegacyToken,
	}, s.cfg.ClientAccessTTL, s.cfg.ClientRefreshTTL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeClientAccount(w, r, http.StatusCreated, account)
}

func (s *HTTPServer) clientLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	account, err := s.store.AuthenticateClient(
		input.Username,
		input.Password,
		s.cfg.ClientAccessTTL,
		s.cfg.ClientRefreshTTL,
	)
	if err != nil {
		message := "用户名或密码不正确"
		if !errors.Is(err, ErrNotFound) {
			message = err.Error()
		}
		writeError(w, http.StatusUnauthorized, message)
		return
	}
	writeClientAccount(w, r, http.StatusOK, account)
}

func (s *HTTPServer) clientRecover(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username        string `json:"username"`
		Password        string `json:"password"`
		RecoveryKeyHash string `json:"recoveryKeyHash"`
		KeySalt         string `json:"keySalt"`
		WrappedKey      string `json:"wrappedKey"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	account, err := s.store.RecoverAccount(
		input.Username,
		input.Password,
		input.RecoveryKeyHash,
		input.KeySalt,
		input.WrappedKey,
		s.cfg.ClientAccessTTL,
		s.cfg.ClientRefreshTTL,
	)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "用户名或恢复密钥不正确")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeClientAccount(w, r, http.StatusOK, account)
}

func (s *HTTPServer) clientRefresh(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	account, err := s.store.RefreshClientSession(
		input.RefreshToken,
		s.cfg.ClientAccessTTL,
		s.cfg.ClientRefreshTTL,
	)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "登录已失效，请重新登录")
		return
	}
	writeClientAccount(w, r, http.StatusOK, account)
}

func (s *HTTPServer) clientLogout(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.RefreshToken != "" {
		_ = s.store.DeleteClientSession(input.RefreshToken)
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeClientAccount(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	account ClientAccount,
) {
	writeJSON(w, status, map[string]any{
		"user":             account.User,
		"accessToken":      account.Session.AccessToken,
		"refreshToken":     account.Session.RefreshToken,
		"accessExpiresAt":  account.Session.AccessExpiresAt,
		"refreshExpiresAt": account.Session.RefreshExpiresAt,
		"keySalt":          account.KeySalt,
		"wrappedKey":       account.WrappedKey,
		"transportSecure":  requestIsSecure(r),
	})
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
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
		if errors.Is(err, ErrDeviceConflict) || errors.Is(err, ErrDeviceNotOwned) {
			message := strings.TrimPrefix(err.Error(), ErrDeviceConflict.Error()+"：")
			message = strings.TrimPrefix(message, ErrDeviceNotOwned.Error()+"：")
			writeError(w, http.StatusConflict, message)
			return
		}
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device": result})
}

func (s *HTTPServer) uploadTest(w http.ResponseWriter, r *http.Request) {
	const maxDiagnosticUpload = 1 << 20
	started := time.Now()
	reader := http.MaxBytesReader(w, r.Body, maxDiagnosticUpload)
	defer reader.Close()
	payload, err := io.ReadAll(reader)
	if err != nil {
		writeError(w, http.StatusBadRequest, "上传测试包无效或超过 1 MB")
		return
	}
	if len(payload) < 28 {
		writeError(w, http.StatusBadRequest, "上传测试包过小")
		return
	}
	digest := sha256.Sum256(payload)
	writeJSON(w, http.StatusOK, map[string]any{
		"receivedBytes": len(payload),
		"sha256":        hex.EncodeToString(digest[:]),
		"discarded":     true,
		"durationMs":    time.Since(started).Milliseconds(),
	})
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
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes+uploadMultipartOverheadBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "上传请求必须使用 multipart/form-data")
		return
	}

	var rawMetadata []byte
	var absolutePath string
	var relativePath string
	var size int64
	cleanupBlob := func() {
		if absolutePath != "" {
			_ = os.Remove(absolutePath)
		}
	}
	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}
		if partErr != nil {
			cleanupBlob()
			writeError(w, http.StatusRequestEntityTooLarge, "上传内容超过服务端大小限制")
			return
		}
		switch part.FormName() {
		case "metadata":
			payload, readErr := io.ReadAll(io.LimitReader(part, maxUploadMetadataBytes+1))
			_ = part.Close()
			if readErr != nil {
				cleanupBlob()
				writeError(w, http.StatusBadRequest, "metadata 读取失败")
				return
			}
			if len(payload) > maxUploadMetadataBytes {
				cleanupBlob()
				writeError(w, http.StatusRequestEntityTooLarge, "metadata 超过 8 MiB 限制")
				return
			}
			rawMetadata = payload
		case "blob":
			if absolutePath != "" {
				_ = part.Close()
				cleanupBlob()
				writeError(w, http.StatusBadRequest, "只能上传一个加密 blob")
				return
			}
			blobName := newID() + ".ccx"
			relativePath = filepath.Join("blobs", blobName)
			absolutePath = filepath.Join(s.cfg.DataDir, relativePath)
			size, err = saveUpload(part, absolutePath, s.cfg.MaxUploadBytes)
			_ = part.Close()
			if errors.Is(err, errUploadTooLarge) {
				cleanupBlob()
				writeError(
					w,
					http.StatusRequestEntityTooLarge,
					fmt.Sprintf("加密同步包超过 %d MiB 上限", s.cfg.MaxUploadBytes/(1024*1024)),
				)
				return
			}
			if err != nil {
				cleanupBlob()
				s.internalError(w, err)
				return
			}
		default:
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
		}
	}

	var metadata uploadMetadata
	if len(rawMetadata) == 0 || json.Unmarshal(rawMetadata, &metadata) != nil {
		cleanupBlob()
		writeError(w, http.StatusBadRequest, "metadata 无效")
		return
	}
	user := currentUser(r)
	if !s.store.DeviceOwnedBy(user.ID, metadata.SourceDeviceID) {
		cleanupBlob()
		writeError(w, http.StatusBadRequest, "来源设备无效")
		return
	}
	if absolutePath == "" {
		writeError(w, http.StatusBadRequest, "缺少加密 blob")
		return
	}
	if size <= 0 {
		cleanupBlob()
		writeError(w, http.StatusBadRequest, "加密 blob 为空")
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

func (s *HTTPServer) withClientAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "缺少客户端令牌")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		user, err := s.store.UserByClientAccessToken(token)
		if err != nil {
			user, err = s.store.UserByAPIToken(token)
		}
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
		recorder := &statusResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		if !strings.HasPrefix(r.URL.Path, "/assets/") {
			s.log.Info(
				"http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"bytes", recorder.bytes,
				"duration", time.Since(started),
			)
		}
	})
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(payload []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	written, err := w.ResponseWriter.Write(payload)
	w.bytes += int64(written)
	return written, err
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

func saveUpload(src io.Reader, destination string, maxBytes int64) (int64, error) {
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	size, copyErr := io.Copy(dst, io.LimitReader(src, maxBytes+1))
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return 0, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return 0, closeErr
	}
	if size > maxBytes {
		_ = os.Remove(destination)
		return 0, errUploadTooLarge
	}
	return size, nil
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

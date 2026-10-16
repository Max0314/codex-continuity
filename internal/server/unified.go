package server

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/neonet/codex-continuity/internal/model"
	"io"
	"net/http"
	"strings"
	"time"
)

// Identities are obtained only from the protected platform endpoint. Never
// accept a subject/account ID or forwarded identity headers from the browser.
func (s *HTTPServer) platformIdentity(r *http.Request, grant any) (string, string, error) {
	endpoint := s.cfg.IdentityEndpoint
	body := []byte(`{}`)
	if grant != nil {
		endpoint = strings.TrimSuffix(endpoint, "/identity") + "/desktop/exchange"
		body, _ = json.Marshal(grant)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Host = "continuity.fribench.cn"
	req.Header.Set("X-Fribench-App", "codex-continuity")
	req.SetBasicAuth("fribench-codex-continuity", s.cfg.IdentitySecret)
	req.Header.Set("Content-Type", "application/json")
	if grant == nil {
		cookie, err := r.Cookie("__Host-fribench_codex_continuity")
		if err != nil {
			return "", "", ErrNotFound
		}
		req.AddCookie(cookie)
	}
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", ErrNotFound
	}
	var identity struct {
		Sub           string `json:"sub"`
		Name          string `json:"name"`
		Authenticated bool   `json:"authenticated"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&identity) != nil || identity.Sub == "" || len(identity.Sub) > 200 {
		return "", "", ErrNotFound
	}
	return identity.Sub, identity.Name, nil
}

func (s *Store) unifiedUser(sub, name string) (model.User, error) {
	id := "fb_" + tokenDigest(sub)[:28]
	if len(name) > 100 {
		name = string([]rune(name)[:min(50, len([]rune(name)))])
	}
	if name == "" {
		name = "Fribench 用户"
	}
	_, err := s.db.Exec(`INSERT INTO users(id,username,email,display_name,password_hash,key_salt,wrapped_key,recovery_key_hash,role,created_at,external_subject) VALUES(?,?,?,?,?,'','','','member',?,?) ON CONFLICT(external_subject) DO UPDATE SET display_name=excluded.display_name`, id, id, id+"@identity.invalid", name, "!unified", nowText(), sub)
	if err != nil {
		return model.User{}, err
	}
	return scanUser(s.db.QueryRow("SELECT id,username,email,display_name,role,created_at FROM users WHERE external_subject=?", sub))
}

func (s *HTTPServer) unifiedLogin(w http.ResponseWriter, r *http.Request) {
	if s.cfg.IdentitySecret == "" {
		writeError(w, 503, "统一登录尚未配置")
		return
	}
	var input struct {
		Code     string `json:"code"`
		Verifier string `json:"code_verifier"`
		Redirect string `json:"redirect_uri"`
		KeyHash  string `json:"keyHash"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if raw, err := hex.DecodeString(input.KeyHash); err != nil || len(raw) != 32 {
		writeError(w, 400, "密钥指纹无效")
		return
	}
	sub, name, err := s.platformIdentity(r, map[string]string{"code": input.Code, "code_verifier": input.Verifier, "redirect_uri": input.Redirect})
	if err != nil {
		writeError(w, 401, "连接已过期，请重新从客户端发起")
		return
	}
	user, err := s.store.unifiedUser(sub, name)
	if err != nil {
		s.internalError(w, err)
		return
	}
	// First writer wins. Only a fingerprint is held by the server, not a key.
	_, err = s.store.db.Exec("UPDATE users SET key_salt='recovery-v1',wrapped_key=?,recovery_key_hash=? WHERE id=? AND key_salt=''", input.KeyHash, input.KeyHash, user.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	var keyHash string
	if err = s.store.db.QueryRow("SELECT wrapped_key FROM users WHERE id=?", user.ID).Scan(&keyHash); err != nil {
		s.internalError(w, err)
		return
	}
	session, err := s.store.CreateClientSession(user.ID, s.cfg.ClientAccessTTL, s.cfg.ClientRefreshTTL)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeClientAccount(w, r, 200, ClientAccount{User: user, KeySalt: "recovery-v1", WrappedKey: keyHash, Session: session})
}

func (s *HTTPServer) guestPreview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Guest  string `json:"guest"`
		SHA256 string `json:"sha256"`
		Size   int64  `json:"size"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	raw, err := hex.DecodeString(input.SHA256)
	if err != nil || len(raw) != 32 || len(input.Guest) < 20 || len(input.Guest) > 100 || input.Size < 28 || input.Size > s.cfg.MaxUploadBytes {
		writeError(w, 400, "快照摘要或大小无效")
		return
	}
	user := currentUser(r)
	tx, err := s.store.db.Begin()
	if err != nil {
		s.internalError(w, err)
		return
	}
	defer tx.Rollback()
	if err = lockUploads(tx); err != nil {
		s.internalError(w, err)
		return
	}
	var count int
	if err = tx.QueryRow("SELECT COUNT(*) FROM continuity_guest_claims WHERE user_id=? OR guest_hash=?", user.ID, tokenDigest(input.Guest)).Scan(&count); err != nil {
		s.internalError(w, err)
		return
	}
	if count > 0 {
		writeError(w, 409, "该账号或游客存档已使用过绑定机会")
		return
	}
	if err = tx.QueryRow("SELECT COUNT(*) FROM handoffs WHERE user_id=?", user.ID).Scan(&count); err != nil {
		s.internalError(w, err)
		return
	}
	if count > 0 {
		writeError(w, 409, "账号已有快照，不会覆盖；请使用正常同步")
		return
	}
	id := newID()
	_, err = tx.Exec("DELETE FROM continuity_guest_stages WHERE user_id=? OR expires_at<?", user.ID, nowText())
	if err == nil {
		_, err = tx.Exec("INSERT INTO continuity_guest_stages(id,user_id,guest_hash,digest,size,expires_at) VALUES(?,?,?,?,?,?)", id, user.ID, tokenDigest(input.Guest), input.SHA256, input.Size, time.Now().UTC().Add(30*time.Minute).Format(time.RFC3339Nano))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"stage": id, "size": input.Size, "expiresIn": 1800, "notice": "仅确认上传成功后消耗本服务的一次机会；取消不会上传。"})
}

func lockUploads(tx *transaction) error {
	if tx.postgres {
		_, err := tx.Exec("SELECT pg_advisory_xact_lock(41043,24)")
		return err
	}
	return nil
}

var errStorageConflict = errors.New("快照冲突或空间不足；原始本地数据未被修改")

// A single transaction owns quota checks, the optional once-only claim and the
// handoff. Nothing consumes the claim if the insert/commit fails.
func (s *Store) createOwnedHandoff(p CreateHandoffParams, stage, digest string) (model.Handoff, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return model.Handoff{}, false, err
	}
	defer tx.Rollback()
	if err = lockUploads(tx); err != nil {
		return model.Handoff{}, false, err
	}
	if stage != "" {
		var prior, priorDigest string
		var priorSize int64
		err = tx.QueryRow("SELECT handoff_id,digest,size FROM continuity_guest_claims WHERE user_id=? AND stage_id=?", p.UserID, stage).Scan(&prior, &priorDigest, &priorSize)
		if err == nil {
			if priorDigest != digest || priorSize != p.BlobSize {
				return model.Handoff{}, false, errStorageConflict
			}
			if err = tx.Commit(); err != nil {
				return model.Handoff{}, false, err
			}
			h, _, e := s.HandoffByID(p.UserID, prior)
			return h, true, e
		}
		var guest, expected, expires string
		var size int64
		err = tx.QueryRow("SELECT guest_hash,digest,size,expires_at FROM continuity_guest_stages WHERE id=? AND user_id=?", stage, p.UserID).Scan(&guest, &expected, &size, &expires)
		if err != nil || expected != digest || size != p.BlobSize || !parseTime(expires).After(time.Now()) {
			return model.Handoff{}, false, errStorageConflict
		}
		var n int
		if err = tx.QueryRow("SELECT COUNT(*) FROM handoffs WHERE user_id=?", p.UserID).Scan(&n); err != nil || n != 0 {
			return model.Handoff{}, false, errStorageConflict
		}
		_, err = tx.Exec("INSERT INTO continuity_guest_claims(user_id,guest_hash,stage_id,handoff_id,digest,size,created_at) VALUES(?,?,?,'',?,?,?)", p.UserID, guest, stage, digest, p.BlobSize, nowText())
		if err != nil {
			return model.Handoff{}, false, errStorageConflict
		}
	}
	var used, total int64
	var count int
	if err = tx.QueryRow("SELECT COALESCE(SUM(blob_size),0),COUNT(*) FROM handoffs WHERE user_id=?", p.UserID).Scan(&used, &count); err != nil {
		return model.Handoff{}, false, err
	}
	if err = tx.QueryRow("SELECT COALESCE(SUM(blob_size),0) FROM handoffs").Scan(&total); err != nil {
		return model.Handoff{}, false, err
	}
	if count >= 1000 || used+p.BlobSize > 2<<30 || total+p.BlobSize > 8<<30 {
		return model.Handoff{}, false, errStorageConflict
	}
	h := model.Handoff{ID: newID(), ProjectName: strings.TrimSpace(p.ProjectName), WorkspaceKey: p.WorkspaceKey, SourceDeviceID: p.SourceDeviceID, TargetDeviceName: p.TargetDeviceName, Status: model.HandoffPending, BlobSize: p.BlobSize, CreatedAt: time.Now().UTC()}
	if h.WorkspaceKey == "" || len(h.WorkspaceKey) > 200 || len(h.ProjectName) > 300 || !json.Valid(p.Manifest) {
		return model.Handoff{}, false, fmt.Errorf("快照元数据无效")
	}
	_, err = tx.Exec("INSERT INTO handoffs(id,user_id,project_name,workspace_key,source_device_id,target_device_name,status,manifest_json,blob_path,blob_size,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)", h.ID, p.UserID, h.ProjectName, h.WorkspaceKey, h.SourceDeviceID, h.TargetDeviceName, h.Status, string(p.Manifest), p.BlobPath, p.BlobSize, h.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return model.Handoff{}, false, err
	}
	if stage != "" {
		_, err = tx.Exec("UPDATE continuity_guest_claims SET handoff_id=? WHERE user_id=? AND stage_id=?", h.ID, p.UserID, stage)
		if err != nil {
			return model.Handoff{}, false, err
		}
		_, err = tx.Exec("DELETE FROM continuity_guest_stages WHERE user_id=?", p.UserID)
		if err != nil {
			return model.Handoff{}, false, err
		}
	}
	err = tx.Commit()
	return h, false, err
}

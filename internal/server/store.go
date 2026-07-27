package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neonet/codex-continuity/internal/model"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db      *sql.DB
	dataDir string
}

type authenticatedUser struct {
	model.User
	PasswordHash string
}

func OpenStore(cfg Config) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "blobs"), 0o700); err != nil {
		return nil, fmt.Errorf("create data directories: %w", err)
	}
	if err := os.MkdirAll(cfg.DownloadDir, 0o755); err != nil {
		return nil, fmt.Errorf("create download directory: %w", err)
	}
	dbPath := filepath.Join(cfg.DataDir, "continuity.db")
	dsn := "file:" + filepath.ToSlash(dbPath) + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, dataDir: cfg.DataDir}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.bootstrapAdmin(cfg); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE COLLATE NOCASE,
  display_name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK(role IN ('admin','member')),
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE TABLE IF NOT EXISTS api_tokens (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  prefix TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  last_used_at TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens(user_id);
CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  hostname TEXT NOT NULL,
  os TEXT NOT NULL,
  client_version TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(user_id, name)
);
CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_id);
CREATE TABLE IF NOT EXISTS handoffs (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  project_name TEXT NOT NULL,
  workspace_key TEXT NOT NULL,
  source_device_id TEXT NOT NULL REFERENCES devices(id),
  target_device_name TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK(status IN ('pending','claimed')),
  manifest_json TEXT NOT NULL,
  blob_path TEXT NOT NULL,
  blob_size INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  claimed_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_handoffs_user_created ON handoffs(user_id, created_at DESC);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return nil
}

func (s *Store) bootstrapAdmin(cfg Config) error {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	passwordHash, err := hashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		"INSERT INTO users(id,email,display_name,password_hash,role,created_at) VALUES(?,?,?,?,?,?)",
		newID(), cfg.AdminEmail, cfg.AdminName, passwordHash, model.RoleAdmin, nowText(),
	)
	return err
}

func (s *Store) Authenticate(email, password string) (model.User, error) {
	row := s.db.QueryRow(`
SELECT id,email,display_name,password_hash,role,created_at
FROM users WHERE email=?`, strings.ToLower(strings.TrimSpace(email)))
	user, err := scanAuthenticatedUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	if !checkPassword(user.PasswordHash, password) {
		return model.User{}, ErrNotFound
	}
	return user.User, nil
}

func (s *Store) CreateSession(userID string, ttl time.Duration) (string, error) {
	plain, digest, err := randomToken("cs_", 32)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	_, err = s.db.Exec(
		"INSERT INTO sessions(token_hash,user_id,expires_at,created_at) VALUES(?,?,?,?)",
		digest, userID, now.Add(ttl).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	return plain, err
}

func (s *Store) UserBySession(token string) (model.User, error) {
	row := s.db.QueryRow(`
SELECT u.id,u.email,u.display_name,u.role,u.created_at
FROM sessions s JOIN users u ON u.id=s.user_id
WHERE s.token_hash=? AND s.expires_at>?`, tokenDigest(token), nowText())
	return scanUser(row)
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token_hash=?", tokenDigest(token))
	return err
}

func (s *Store) UserByAPIToken(token string) (model.User, error) {
	digest := tokenDigest(token)
	row := s.db.QueryRow(`
SELECT u.id,u.email,u.display_name,u.role,u.created_at
FROM api_tokens t JOIN users u ON u.id=t.user_id
WHERE t.token_hash=?`, digest)
	user, err := scanUser(row)
	if err != nil {
		return model.User{}, err
	}
	_, _ = s.db.Exec("UPDATE api_tokens SET last_used_at=? WHERE token_hash=?", nowText(), digest)
	return user, nil
}

func (s *Store) ListUsers() ([]model.User, error) {
	rows, err := s.db.Query("SELECT id,email,display_name,role,created_at FROM users ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, user)
	}
	return result, rows.Err()
}

func (s *Store) CreateUser(email, displayName, password, role string) (model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	displayName = strings.TrimSpace(displayName)
	if email == "" || displayName == "" || len(password) < 10 {
		return model.User{}, fmt.Errorf("email、姓名不能为空，密码至少 10 位")
	}
	if role != model.RoleAdmin && role != model.RoleMember {
		role = model.RoleMember
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return model.User{}, err
	}
	user := model.User{
		ID:          newID(),
		Email:       email,
		DisplayName: displayName,
		Role:        role,
		CreatedAt:   time.Now().UTC(),
	}
	_, err = s.db.Exec(
		"INSERT INTO users(id,email,display_name,password_hash,role,created_at) VALUES(?,?,?,?,?,?)",
		user.ID, user.Email, user.DisplayName, passwordHash, user.Role, user.CreatedAt.Format(time.RFC3339Nano),
	)
	return user, err
}

func (s *Store) ListTokens(userID string) ([]model.APIToken, error) {
	rows, err := s.db.Query(`
SELECT id,name,prefix,last_used_at,created_at FROM api_tokens
WHERE user_id=? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.APIToken{}
	for rows.Next() {
		var token model.APIToken
		var lastUsed sql.NullString
		var created string
		if err := rows.Scan(&token.ID, &token.Name, &token.Prefix, &lastUsed, &created); err != nil {
			return nil, err
		}
		token.CreatedAt = parseTime(created)
		if lastUsed.Valid {
			t := parseTime(lastUsed.String)
			token.LastUsedAt = &t
		}
		result = append(result, token)
	}
	return result, rows.Err()
}

func (s *Store) CreateToken(userID, name string) (model.APIToken, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.APIToken{}, "", fmt.Errorf("令牌名称不能为空")
	}
	plain, digest, err := randomToken("ct_", 32)
	if err != nil {
		return model.APIToken{}, "", err
	}
	token := model.APIToken{
		ID:        newID(),
		Name:      name,
		Prefix:    plain[:min(12, len(plain))],
		CreatedAt: time.Now().UTC(),
	}
	_, err = s.db.Exec(`
INSERT INTO api_tokens(id,user_id,name,prefix,token_hash,created_at)
VALUES(?,?,?,?,?,?)`, token.ID, userID, token.Name, token.Prefix, digest, token.CreatedAt.Format(time.RFC3339Nano))
	return token, plain, err
}

func (s *Store) DeleteToken(userID, tokenID string) error {
	result, err := s.db.Exec("DELETE FROM api_tokens WHERE id=? AND user_id=?", tokenID, userID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpsertDevice(userID string, device model.Device) (model.Device, error) {
	now := time.Now().UTC()
	var existingID, created string
	err := s.db.QueryRow("SELECT id,created_at FROM devices WHERE user_id=? AND name=?", userID, device.Name).
		Scan(&existingID, &created)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return model.Device{}, err
	}
	if existingID != "" {
		device.ID = existingID
		device.CreatedAt = parseTime(created)
		device.LastSeenAt = now
		_, err = s.db.Exec(`
UPDATE devices SET hostname=?,os=?,client_version=?,last_seen_at=? WHERE id=?`,
			device.Hostname, device.OS, device.ClientVersion, now.Format(time.RFC3339Nano), device.ID)
		return device, err
	}
	if device.ID == "" {
		device.ID = newID()
	}
	device.CreatedAt = now
	device.LastSeenAt = now
	_, err = s.db.Exec(`
INSERT INTO devices(id,user_id,name,hostname,os,client_version,last_seen_at,created_at)
VALUES(?,?,?,?,?,?,?,?)`,
		device.ID, userID, device.Name, device.Hostname, device.OS, device.ClientVersion,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	return device, err
}

func (s *Store) ListDevices(userID string) ([]model.Device, error) {
	rows, err := s.db.Query(`
SELECT id,name,hostname,os,client_version,last_seen_at,created_at
FROM devices WHERE user_id=? ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Device{}
	for rows.Next() {
		var d model.Device
		var lastSeen, created string
		if err := rows.Scan(&d.ID, &d.Name, &d.Hostname, &d.OS, &d.ClientVersion, &lastSeen, &created); err != nil {
			return nil, err
		}
		d.LastSeenAt = parseTime(lastSeen)
		d.CreatedAt = parseTime(created)
		result = append(result, d)
	}
	return result, rows.Err()
}

func (s *Store) DeviceOwnedBy(userID, deviceID string) bool {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM devices WHERE user_id=? AND id=?", userID, deviceID).Scan(&count)
	return err == nil && count == 1
}

type CreateHandoffParams struct {
	UserID           string
	ProjectName      string
	WorkspaceKey     string
	SourceDeviceID   string
	TargetDeviceName string
	Manifest         json.RawMessage
	BlobPath         string
	BlobSize         int64
}

func (s *Store) CreateHandoff(p CreateHandoffParams) (model.Handoff, error) {
	h := model.Handoff{
		ID:               newID(),
		ProjectName:      strings.TrimSpace(p.ProjectName),
		WorkspaceKey:     strings.TrimSpace(p.WorkspaceKey),
		SourceDeviceID:   p.SourceDeviceID,
		TargetDeviceName: strings.TrimSpace(p.TargetDeviceName),
		Status:           model.HandoffPending,
		BlobSize:         p.BlobSize,
		CreatedAt:        time.Now().UTC(),
	}
	if h.ProjectName == "" {
		h.ProjectName = "工作区交接"
	}
	if h.WorkspaceKey == "" {
		return model.Handoff{}, fmt.Errorf("workspaceKey 不能为空")
	}
	if len(p.Manifest) == 0 {
		p.Manifest = json.RawMessage(`{}`)
	}
	_, err := s.db.Exec(`
INSERT INTO handoffs(
 id,user_id,project_name,workspace_key,source_device_id,target_device_name,
 status,manifest_json,blob_path,blob_size,created_at
) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		h.ID, p.UserID, h.ProjectName, h.WorkspaceKey, h.SourceDeviceID, h.TargetDeviceName,
		h.Status, string(p.Manifest), p.BlobPath, h.BlobSize, h.CreatedAt.Format(time.RFC3339Nano),
	)
	return h, err
}

func (s *Store) ListHandoffs(userID string, limit int) ([]model.Handoff, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`
SELECT h.id,h.project_name,h.workspace_key,h.source_device_id,d.name,
       h.target_device_name,h.status,h.manifest_json,h.blob_size,h.created_at,h.claimed_at
FROM handoffs h JOIN devices d ON d.id=h.source_device_id
WHERE h.user_id=? ORDER BY h.created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Handoff{}
	for rows.Next() {
		h, _, err := scanHandoff(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

func (s *Store) HandoffByID(userID, handoffID string) (model.Handoff, string, error) {
	row := s.db.QueryRow(`
SELECT h.id,h.project_name,h.workspace_key,h.source_device_id,d.name,
       h.target_device_name,h.status,h.manifest_json,h.blob_size,h.created_at,h.claimed_at,h.blob_path
FROM handoffs h JOIN devices d ON d.id=h.source_device_id
WHERE h.user_id=? AND h.id=?`, userID, handoffID)
	h, path, err := scanHandoffWithPath(row)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return h, path, err
}

func (s *Store) ClaimHandoff(userID, handoffID, targetDeviceName string) error {
	now := nowText()
	result, err := s.db.Exec(`
UPDATE handoffs SET status='claimed',claimed_at=?,target_device_name=?
WHERE user_id=? AND id=?`, now, targetDeviceName, userID, handoffID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Overview(userID string) (model.Overview, error) {
	var result model.Overview
	onlineAfter := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339Nano)
	monthStart := time.Now().UTC()
	monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, time.UTC)
	queries := []struct {
		query string
		args  []any
		dest  any
	}{
		{"SELECT COUNT(*) FROM devices WHERE user_id=? AND last_seen_at>=?", []any{userID, onlineAfter}, &result.OnlineDevices},
		{"SELECT COUNT(*) FROM handoffs WHERE user_id=? AND status='pending'", []any{userID}, &result.Pending},
		{"SELECT COUNT(*) FROM handoffs WHERE user_id=? AND created_at>=?", []any{userID, monthStart.Format(time.RFC3339Nano)}, &result.Monthly},
		{"SELECT COALESCE(SUM(blob_size),0) FROM handoffs WHERE user_id=?", []any{userID}, &result.StorageBytes},
	}
	for _, q := range queries {
		if err := s.db.QueryRow(q.query, q.args...).Scan(q.dest); err != nil {
			return result, err
		}
	}
	var err error
	result.RecentHandoffs, err = s.ListHandoffs(userID, 8)
	if err != nil {
		return result, err
	}
	result.Devices, err = s.ListDevices(userID)
	return result, err
}

func scanAuthenticatedUser(row interface{ Scan(...any) error }) (authenticatedUser, error) {
	var u authenticatedUser
	var created string
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.Role, &created)
	u.CreatedAt = parseTime(created)
	return u, err
}

func scanUser(row interface{ Scan(...any) error }) (model.User, error) {
	var u model.User
	var created string
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &created)
	u.CreatedAt = parseTime(created)
	return u, err
}

func scanHandoff(row interface{ Scan(...any) error }) (model.Handoff, string, error) {
	var h model.Handoff
	var manifest, created string
	var claimed sql.NullString
	err := row.Scan(
		&h.ID, &h.ProjectName, &h.WorkspaceKey, &h.SourceDeviceID, &h.SourceDeviceName,
		&h.TargetDeviceName, &h.Status, &manifest, &h.BlobSize, &created, &claimed,
	)
	if err != nil {
		return h, "", err
	}
	h.CreatedAt = parseTime(created)
	if claimed.Valid {
		t := parseTime(claimed.String)
		h.ClaimedAt = &t
	}
	_ = json.Unmarshal([]byte(manifest), &h.Manifest)
	return h, "", nil
}

func scanHandoffWithPath(row interface{ Scan(...any) error }) (model.Handoff, string, error) {
	var h model.Handoff
	var manifest, created, path string
	var claimed sql.NullString
	err := row.Scan(
		&h.ID, &h.ProjectName, &h.WorkspaceKey, &h.SourceDeviceID, &h.SourceDeviceName,
		&h.TargetDeviceName, &h.Status, &manifest, &h.BlobSize, &created, &claimed, &path,
	)
	if err != nil {
		return h, "", err
	}
	h.CreatedAt = parseTime(created)
	if claimed.Valid {
		t := parseTime(claimed.String)
		h.ClaimedAt = &t
	}
	_ = json.Unmarshal([]byte(manifest), &h.Manifest)
	return h, path, nil
}

func nowText() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

func newID() string {
	plain, _, err := randomToken("", 16)
	if err != nil {
		panic(err)
	}
	return plain
}

func (s *Store) CleanupExpiredSessions(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.db.Exec("DELETE FROM sessions WHERE expires_at<?", nowText())
		}
	}
}

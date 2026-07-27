package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address        string
	DataDir        string
	WebDir         string
	PublicURL      string
	AdminEmail     string
	AdminPassword  string
	AdminName      string
	SessionTTL     time.Duration
	MaxUploadBytes int64
	CookieSecure   bool
	TrustProxy     bool
	DownloadDir    string
}

func LoadConfig() (Config, error) {
	dataDir := env("CONTINUITY_DATA_DIR", "./data")
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve data directory: %w", err)
	}
	maxMiB, err := strconv.ParseInt(env("CONTINUITY_MAX_UPLOAD_MIB", "512"), 10, 64)
	if err != nil || maxMiB < 1 {
		return Config{}, fmt.Errorf("CONTINUITY_MAX_UPLOAD_MIB must be a positive integer")
	}
	cfg := Config{
		Address:        env("CONTINUITY_ADDR", ":8080"),
		DataDir:        absDataDir,
		WebDir:         env("CONTINUITY_WEB_DIR", "./web/dist"),
		PublicURL:      strings.TrimRight(env("CONTINUITY_PUBLIC_URL", "http://localhost:8080"), "/"),
		AdminEmail:     strings.ToLower(strings.TrimSpace(env("CONTINUITY_ADMIN_EMAIL", "admin@example.com"))),
		AdminPassword:  os.Getenv("CONTINUITY_ADMIN_PASSWORD"),
		AdminName:      env("CONTINUITY_ADMIN_NAME", "系统管理员"),
		SessionTTL:     7 * 24 * time.Hour,
		MaxUploadBytes: maxMiB * 1024 * 1024,
		CookieSecure:   envBool("CONTINUITY_COOKIE_SECURE", false),
		TrustProxy:     envBool("CONTINUITY_TRUST_PROXY", false),
		DownloadDir:    env("CONTINUITY_DOWNLOAD_DIR", filepath.Join(absDataDir, "downloads")),
	}
	if cfg.AdminPassword == "" {
		return Config{}, fmt.Errorf("CONTINUITY_ADMIN_PASSWORD 不能为空")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes"
}

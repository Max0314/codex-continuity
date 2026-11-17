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
	DatabaseURL       string
	IdentityEndpoint  string
	IdentitySecret    string
	Address           string
	DataDir           string
	WebDir            string
	PublicURL         string
	AdminEmail        string
	AdminPassword     string
	AdminName         string
	SessionTTL        time.Duration
	ClientAccessTTL   time.Duration
	ClientRefreshTTL  time.Duration
	MaxUploadBytes    int64
	HandoffRetention  time.Duration
	HandoffVersions   int
	UserStorageBytes  int64
	TotalStorageBytes int64
	CookieSecure      bool
	TrustProxy        bool
	DownloadDir       string
}

func LoadConfig() (Config, error) {
	dataDir := env("CONTINUITY_DATA_DIR", "./data")
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve data directory: %w", err)
	}
	maxMiB, err := strconv.ParseInt(env("CONTINUITY_MAX_UPLOAD_MIB", "500"), 10, 64)
	if err != nil || maxMiB < 1 {
		return Config{}, fmt.Errorf("CONTINUITY_MAX_UPLOAD_MIB must be a positive integer")
	}
	retentionDays, err := positiveEnvInt64("CONTINUITY_HANDOFF_RETENTION_DAYS", 14)
	if err != nil {
		return Config{}, err
	}
	versions, err := positiveEnvInt64("CONTINUITY_HANDOFF_VERSIONS", 2)
	if err != nil || versions > 100 {
		return Config{}, fmt.Errorf("CONTINUITY_HANDOFF_VERSIONS must be an integer between 1 and 100")
	}
	userStorageMiB, err := positiveEnvInt64("CONTINUITY_USER_STORAGE_MIB", 2048)
	if err != nil {
		return Config{}, err
	}
	totalStorageMiB, err := positiveEnvInt64("CONTINUITY_TOTAL_STORAGE_MIB", 10240)
	if err != nil {
		return Config{}, err
	}
	if userStorageMiB < maxMiB {
		return Config{}, fmt.Errorf("CONTINUITY_USER_STORAGE_MIB must be at least CONTINUITY_MAX_UPLOAD_MIB")
	}
	if totalStorageMiB < userStorageMiB {
		return Config{}, fmt.Errorf("CONTINUITY_TOTAL_STORAGE_MIB must be at least CONTINUITY_USER_STORAGE_MIB")
	}
	cfg := Config{
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		IdentityEndpoint:  env("FRIBENCH_IDENTITY_ENDPOINT", "http://fribench-services-api-1:8080/internal/identity"),
		IdentitySecret:    os.Getenv("FRIBENCH_CLIENT_SECRET"),
		Address:           env("CONTINUITY_ADDR", ":8080"),
		DataDir:           absDataDir,
		WebDir:            env("CONTINUITY_WEB_DIR", "./web/dist"),
		PublicURL:         strings.TrimRight(env("CONTINUITY_PUBLIC_URL", "http://localhost:8080"), "/"),
		AdminEmail:        strings.ToLower(strings.TrimSpace(env("CONTINUITY_ADMIN_EMAIL", "admin@example.com"))),
		AdminPassword:     os.Getenv("CONTINUITY_ADMIN_PASSWORD"),
		AdminName:         env("CONTINUITY_ADMIN_NAME", "系统管理员"),
		SessionTTL:        7 * 24 * time.Hour,
		ClientAccessTTL:   15 * time.Minute,
		ClientRefreshTTL:  3650 * 24 * time.Hour,
		MaxUploadBytes:    maxMiB * 1024 * 1024,
		HandoffRetention:  time.Duration(retentionDays) * 24 * time.Hour,
		HandoffVersions:   int(versions),
		UserStorageBytes:  userStorageMiB * 1024 * 1024,
		TotalStorageBytes: totalStorageMiB * 1024 * 1024,
		CookieSecure:      envBool("CONTINUITY_COOKIE_SECURE", false),
		TrustProxy:        envBool("CONTINUITY_TRUST_PROXY", false),
		DownloadDir:       env("CONTINUITY_DOWNLOAD_DIR", filepath.Join(absDataDir, "downloads")),
	}
	if cfg.AdminPassword == "" {
		return Config{}, fmt.Errorf("CONTINUITY_ADMIN_PASSWORD 不能为空")
	}
	return cfg, nil
}

func positiveEnvInt64(key string, fallback int64) (int64, error) {
	value, err := strconv.ParseInt(env(key, strconv.FormatInt(fallback, 10)), 10, 64)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
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

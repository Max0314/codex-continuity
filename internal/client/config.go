package client

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Config struct {
	ServerURL     string    `json:"serverUrl"`
	Token         string    `json:"token"`
	Root          string    `json:"root"`
	DeviceName    string    `json:"deviceName"`
	DeviceID      string    `json:"deviceId,omitempty"`
	EncryptionKey string    `json:"encryptionKey"`
	SyncScope     SyncScope `json:"syncScope,omitempty"`
}

const (
	DefaultSyncDays     = 7
	MaxAllowedBundleMiB = 500
)

type SyncScope struct {
	Days              int      `json:"days"`
	ProjectPaths      []string `json:"projectPaths,omitempty"`
	IncludeArchived   bool     `json:"includeArchived"`
	IncludeUnassigned bool     `json:"includeUnassigned"`
	MaxBundleMiB      int      `json:"maxBundleMiB"`
}

func DefaultSyncScope() SyncScope {
	return SyncScope{
		Days:         DefaultSyncDays,
		MaxBundleMiB: MaxAllowedBundleMiB,
	}
}

func (c Config) EffectiveSyncScope() SyncScope {
	scope := c.SyncScope
	if scope.Days == 0 && scope.MaxBundleMiB == 0 && len(scope.ProjectPaths) == 0 &&
		!scope.IncludeArchived && !scope.IncludeUnassigned {
		return DefaultSyncScope()
	}
	if scope.MaxBundleMiB == 0 {
		scope.MaxBundleMiB = MaxAllowedBundleMiB
	}
	return scope
}

func DefaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "CodexContinuity", "config.json"), nil
}

func LoadConfig(path string) (Config, error) {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return Config{}, err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("读取配置失败（请先执行 init）: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("配置文件无效: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func SaveConfig(path string, cfg Config) error {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return err
		}
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (c Config) Validate() error {
	if !strings.HasPrefix(c.ServerURL, "http://") && !strings.HasPrefix(c.ServerURL, "https://") {
		return fmt.Errorf("server URL 必须以 http:// 或 https:// 开头")
	}
	if c.Token == "" || c.Root == "" || c.DeviceName == "" {
		return fmt.Errorf("token、root 和 deviceName 不能为空")
	}
	if _, err := c.KeyBytes(); err != nil {
		return err
	}
	scope := c.EffectiveSyncScope()
	if scope.Days != 0 && scope.Days != 2 && scope.Days != 5 && scope.Days != 7 {
		return fmt.Errorf("同步时间范围只能是 2 天、5 天、7 天或不限制")
	}
	if scope.MaxBundleMiB < 1 || scope.MaxBundleMiB > MaxAllowedBundleMiB {
		return fmt.Errorf("单个加密同步包上限必须在 1 到 %d MiB 之间", MaxAllowedBundleMiB)
	}
	for _, projectPath := range scope.ProjectPaths {
		path := filepath.Clean(strings.TrimSpace(projectPath))
		if path == "" || filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("同步项目路径无效: %s", projectPath)
		}
	}
	return nil
}

func (c Config) KeyBytes() ([]byte, error) {
	key, err := base64.RawStdEncoding.DecodeString(c.EncryptionKey)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("encryptionKey 必须是 32 字节密钥的 Base64")
	}
	return key, nil
}

func NewEncryptionKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(key), nil
}

func PlatformName() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

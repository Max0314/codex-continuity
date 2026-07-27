package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/neonet/codex-continuity/internal/client"
	"github.com/neonet/codex-continuity/internal/model"
)

const version = "0.3.1"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = initCommand(os.Args[2:])
	case "status":
		err = statusCommand(os.Args[2:])
	case "scan":
		err = scanCommand(os.Args[2:])
	case "publish":
		err = publishCommand(os.Args[2:])
	case "flush":
		err = flushCommand(os.Args[2:])
	case "export":
		err = exportCommand(os.Args[2:])
	case "import":
		err = importCommand(os.Args[2:])
	case "list":
		err = listCommand(os.Args[2:])
	case "takeover":
		err = takeoverCommand(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println(version)
	default:
		usage()
		err = fmt.Errorf("未知命令: %s", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func initCommand(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	serverURL := flags.String("server", "", "服务端地址")
	token := flags.String("token", "", "管理端创建的客户端 API 令牌")
	root := flags.String("root", `D:\code_CPL`, "固定工作根目录")
	device := flags.String("device", defaultDeviceName(), "设备名称")
	encryptionKey := flags.String("key", "", "两台电脑共享的加密密钥；留空时自动生成")
	configPath := flags.String("config", "", "自定义配置文件路径")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *serverURL == "" || *token == "" {
		return fmt.Errorf("必须提供 --server 和 --token")
	}
	generated := false
	if *encryptionKey == "" {
		var err error
		*encryptionKey, err = client.NewEncryptionKey()
		if err != nil {
			return err
		}
		generated = true
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	cfg := client.Config{
		ServerURL:     strings.TrimRight(*serverURL, "/"),
		Token:         *token,
		Root:          absRoot,
		DeviceName:    *device,
		EncryptionKey: *encryptionKey,
	}
	api := client.NewAPI(cfg)
	hostname, _ := os.Hostname()
	registered, err := api.RegisterDevice(model.Device{
		Name:          cfg.DeviceName,
		Hostname:      hostname,
		OS:            client.PlatformName(),
		ClientVersion: version,
	})
	if err != nil {
		return fmt.Errorf("注册设备失败: %w", err)
	}
	cfg.DeviceID = registered.ID
	if err := client.SaveConfig(*configPath, cfg); err != nil {
		return err
	}
	fmt.Printf("设备已注册：%s\n工作根目录：%s\n", cfg.DeviceName, cfg.Root)
	if generated {
		fmt.Printf("\n加密密钥（仅显示这一次，请安全复制到另一台电脑）：\n%s\n", cfg.EncryptionKey)
	}
	return nil
}

func statusCommand(args []string) error {
	cfg, _, err := loadWithCommonFlags("status", args)
	if err != nil {
		return err
	}
	if err := client.NewAPI(cfg).Health(); err != nil {
		return err
	}
	fmt.Printf("服务端可用：%s\n设备：%s\n工作根目录：%s\n", cfg.ServerURL, cfg.DeviceName, cfg.Root)
	return nil
}

func scanCommand(args []string) error {
	cfg, _, err := loadWithCommonFlags("scan", args)
	if err != nil {
		return err
	}
	manifest, err := client.ScanWithOptions(cfg.Root, cfg.DeviceName, cfg.EffectiveSyncScope())
	if err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(manifest, "", "  ")
	fmt.Println(string(raw))
	return nil
}

func publishCommand(args []string) error {
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	configPath := flags.String("config", "", "配置文件路径")
	target := flags.String("target", "", "可选目标设备名")
	queueDir := flags.String("queue-dir", "", "上传失败时保存加密快照的离线队列目录")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, err := client.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	build, encryptedPath, cleanup, err := buildEncryptedSnapshot(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	metadata := client.UploadMetadata{
		ProjectName:      fmt.Sprintf("%s 工作区（%d 个项目）", build.Manifest.RootName, len(build.Manifest.Projects)),
		WorkspaceKey:     build.Manifest.WorkspaceKey,
		SourceDeviceID:   cfg.DeviceID,
		TargetDeviceName: *target,
		Manifest:         build.Manifest,
	}
	handoff, err := client.NewAPI(cfg).UploadHandoff(metadata, encryptedPath)
	if err != nil {
		if strings.TrimSpace(*queueDir) == "" {
			return err
		}
		queuedPath, queueErr := queueSnapshot(*queueDir, metadata, encryptedPath)
		if queueErr != nil {
			return fmt.Errorf("上传失败（%v），且保存离线队列失败：%w", err, queueErr)
		}
		fmt.Printf("网络暂不可用；加密快照已保存到离线队列：%s\n", queuedPath)
		return nil
	}
	info, _ := os.Stat(encryptedPath)
	fmt.Printf("同步完成：%s\n项目：%d，会话：%d，包大小：%s\n",
		handoff.ID, len(build.Manifest.Projects), len(build.Manifest.Sessions), humanBytes(info.Size()))
	return nil
}

type queuedSnapshot struct {
	CreatedAt time.Time             `json:"createdAt"`
	Metadata  client.UploadMetadata `json:"metadata"`
}

func buildEncryptedSnapshot(cfg client.Config) (client.BuildResult, string, func(), error) {
	fmt.Println("正在扫描工作区与相关 Codex 会话...")
	scope := cfg.EffectiveSyncScope()
	manifest, err := client.ScanWithOptions(cfg.Root, cfg.DeviceName, scope)
	if err != nil {
		return client.BuildResult{}, "", func() {}, err
	}
	tempDir, err := os.MkdirTemp("", "codex-continuity-snapshot-*")
	if err != nil {
		return client.BuildResult{}, "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	zipPath := filepath.Join(tempDir, "snapshot.zip")
	build, err := client.BuildBundle(manifest, zipPath)
	if err != nil {
		cleanup()
		return client.BuildResult{}, "", func() {}, err
	}
	key, err := cfg.KeyBytes()
	if err != nil {
		cleanup()
		return client.BuildResult{}, "", func() {}, err
	}
	encryptedPath := filepath.Join(tempDir, "snapshot.ccx")
	if err := client.EncryptFile(build.ZipPath, encryptedPath, key); err != nil {
		cleanup()
		return client.BuildResult{}, "", func() {}, err
	}
	info, err := os.Stat(encryptedPath)
	if err != nil {
		cleanup()
		return client.BuildResult{}, "", func() {}, err
	}
	maxBytes := int64(scope.MaxBundleMiB) * 1024 * 1024
	if info.Size() > maxBytes {
		cleanup()
		return client.BuildResult{}, "", func() {}, fmt.Errorf(
			"加密同步包为 %s，超过 %d MiB 上限；请缩短时间范围或减少同步项目",
			humanBytes(info.Size()),
			scope.MaxBundleMiB,
		)
	}
	return build, encryptedPath, cleanup, nil
}

func queueSnapshot(queueDir string, metadata client.UploadMetadata, encryptedPath string) (string, error) {
	if err := os.MkdirAll(queueDir, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	blobPath := filepath.Join(queueDir, name+".ccx")
	metaPath := filepath.Join(queueDir, name+".json")
	if err := copyFileExclusive(encryptedPath, blobPath); err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(queuedSnapshot{CreatedAt: time.Now().UTC(), Metadata: metadata}, "", "  ")
	if err != nil {
		_ = os.Remove(blobPath)
		return "", err
	}
	if err := os.WriteFile(metaPath, raw, 0o600); err != nil {
		_ = os.Remove(blobPath)
		return "", err
	}
	return blobPath, nil
}

func flushCommand(args []string) error {
	flags := flag.NewFlagSet("flush", flag.ContinueOnError)
	configPath := flags.String("config", "", "配置文件路径")
	queueDir := flags.String("queue-dir", "", "离线队列目录")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*queueDir) == "" {
		return fmt.Errorf("必须提供 --queue-dir")
	}
	cfg, err := client.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	entries, err := filepath.Glob(filepath.Join(*queueDir, "*.json"))
	if err != nil {
		return err
	}
	sort.Strings(entries)
	if len(entries) == 0 {
		fmt.Println("离线队列为空")
		return nil
	}
	api := client.NewAPI(cfg)
	for _, metaPath := range entries {
		raw, err := os.ReadFile(metaPath)
		if err != nil {
			return err
		}
		var queued queuedSnapshot
		if err := json.Unmarshal(raw, &queued); err != nil {
			return fmt.Errorf("离线队列元数据无效 %s：%w", metaPath, err)
		}
		blobPath := strings.TrimSuffix(metaPath, filepath.Ext(metaPath)) + ".ccx"
		if _, err := os.Stat(blobPath); err != nil {
			return fmt.Errorf("离线队列缺少加密快照 %s", blobPath)
		}
		handoff, err := api.UploadHandoff(queued.Metadata, blobPath)
		if err != nil {
			return fmt.Errorf("重试上传 %s 失败：%w", filepath.Base(blobPath), err)
		}
		if err := os.Remove(blobPath); err != nil {
			return err
		}
		if err := os.Remove(metaPath); err != nil {
			return err
		}
		fmt.Printf("离线快照已上传：%s\n", handoff.ID)
	}
	return nil
}

func exportCommand(args []string) error {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	configPath := flags.String("config", "", "配置文件路径")
	output := flags.String("output", "", "导出的 .ccx 文件路径")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*output) == "" {
		return fmt.Errorf("必须提供 --output")
	}
	cfg, err := client.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	build, encryptedPath, cleanup, err := buildEncryptedSnapshot(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := copyFileExclusive(encryptedPath, *output); err != nil {
		return err
	}
	info, _ := os.Stat(*output)
	fmt.Printf("加密归档已导出：%s\n项目：%d，会话：%d，包大小：%s\n",
		*output, len(build.Manifest.Projects), len(build.Manifest.Sessions), humanBytes(info.Size()))
	return nil
}

func importCommand(args []string) error {
	flags := flag.NewFlagSet("import", flag.ContinueOnError)
	configPath := flags.String("config", "", "配置文件路径")
	input := flags.String("input", "", "要导入的 .ccx 文件")
	output := flags.String("output", "", "只读续接目录")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*input) == "" || strings.TrimSpace(*output) == "" {
		return fmt.Errorf("必须提供 --input 和 --output")
	}
	cfg, err := client.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(*output); err == nil {
		return fmt.Errorf("目标目录已存在：%s", *output)
	}
	tempDir, err := os.MkdirTemp("", "codex-continuity-import-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	key, err := cfg.KeyBytes()
	if err != nil {
		return err
	}
	zipPath := filepath.Join(tempDir, "archive.zip")
	if err := client.DecryptFile(*input, zipPath, key); err != nil {
		return err
	}
	if err := client.ExtractBundle(zipPath, *output); err != nil {
		return err
	}
	fmt.Printf("归档已导入：%s\n", filepath.Join(*output, "HANDOFF.md"))
	return nil
}

func copyFileExclusive(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return closeErr
	}
	return nil
}

func listCommand(args []string) error {
	cfg, _, err := loadWithCommonFlags("list", args)
	if err != nil {
		return err
	}
	handoffs, err := client.NewAPI(cfg).ListHandoffs(cfg.DeviceName)
	if err != nil {
		return err
	}
	pending := handoffs[:0]
	for _, handoff := range handoffs {
		if handoff.Status == model.HandoffPending {
			pending = append(pending, handoff)
		}
	}
	handoffs = pending
	if len(handoffs) == 0 {
		fmt.Println("当前没有可接管的交接包。")
		return nil
	}
	sort.Slice(handoffs, func(i, j int) bool { return handoffs[i].CreatedAt.After(handoffs[j].CreatedAt) })
	for _, h := range handoffs {
		fmt.Printf("%s  %-28s  来自 %-12s  %s\n",
			h.ID, h.ProjectName, h.SourceDeviceName, h.CreatedAt.Local().Format("2006-01-02 15:04"))
	}
	return nil
}

func takeoverCommand(args []string) error {
	flags := flag.NewFlagSet("takeover", flag.ContinueOnError)
	configPath := flags.String("config", "", "配置文件路径")
	id := flags.String("id", "", "交接 ID；留空接管最新一份")
	output := flags.String("output", "", "解压目录；默认放在工作根目录的 .codex-continuity/handoffs 下")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, err := client.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	api := client.NewAPI(cfg)
	handoffs, err := api.ListHandoffs(cfg.DeviceName)
	if err != nil {
		return err
	}
	sort.Slice(handoffs, func(i, j int) bool { return handoffs[i].CreatedAt.After(handoffs[j].CreatedAt) })
	var selected model.Handoff
	if *id != "" {
		found := false
		for _, handoff := range handoffs {
			if handoff.ID == *id {
				selected = handoff
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("找不到交接 ID %s", *id)
		}
	} else {
		for _, handoff := range handoffs {
			if handoff.Status == model.HandoffPending {
				selected = handoff
				break
			}
		}
		if selected.ID == "" {
			return fmt.Errorf("没有可接管的交接包")
		}
	}
	tempDir, err := os.MkdirTemp("", "codex-continuity-takeover-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	encryptedPath := filepath.Join(tempDir, "handoff.ccx")
	if err := api.DownloadHandoff(selected.ID, encryptedPath); err != nil {
		return err
	}
	key, _ := cfg.KeyBytes()
	zipPath := filepath.Join(tempDir, "handoff.zip")
	if err := client.DecryptFile(encryptedPath, zipPath, key); err != nil {
		return err
	}
	if *output == "" {
		*output = filepath.Join(cfg.Root, ".codex-continuity", "handoffs", selected.ID)
	}
	if _, err := os.Stat(*output); err == nil {
		handoffPath := filepath.Join(*output, "HANDOFF.md")
		if _, handoffErr := os.Stat(handoffPath); handoffErr == nil {
			_ = api.ClaimHandoff(selected.ID, cfg.DeviceName)
			fmt.Printf("该快照已在本机：%s\n", handoffPath)
			return nil
		}
		return fmt.Errorf("目标目录已存在但不是完整快照：%s", *output)
	}
	if err := client.ExtractBundle(zipPath, *output); err != nil {
		return err
	}
	if err := api.ClaimHandoff(selected.ID, cfg.DeviceName); err != nil {
		return fmt.Errorf("文件已解压，但回写接管状态失败: %w", err)
	}
	fmt.Printf("接管完成：%s\n请在 Codex 中打开：%s\n并发送：请读取 HANDOFF.md，核对项目状态后继续未完成工作。\n",
		selected.ProjectName, filepath.Join(*output, "HANDOFF.md"))
	return nil
}

func loadWithCommonFlags(name string, args []string) (client.Config, string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	configPath := flags.String("config", "", "配置文件路径")
	if err := flags.Parse(args); err != nil {
		return client.Config{}, "", err
	}
	cfg, err := client.LoadConfig(*configPath)
	return cfg, *configPath, err
}

func defaultDeviceName() string {
	hostname, err := os.Hostname()
	if err == nil && hostname != "" {
		return hostname
	}
	return runtime.GOOS + "-device"
}

func humanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func usage() {
	fmt.Print(`Codex Continuity 客户端 v` + version + `

用法:
  continuity init     --server URL --token TOKEN --root D:\code_CPL --device 办公室电脑 [--key KEY]
  continuity status
  continuity scan
  continuity publish  [--target 公司电脑]
  continuity list
  continuity takeover [--id HANDOFF_ID]

publish 会一次扫描整个工作根目录，不需要逐项目或逐 Codex 任务操作。
`)
}

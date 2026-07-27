package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/neonet/codex-continuity/internal/client"
	"github.com/neonet/codex-continuity/internal/model"
)

const version = "0.1.0"

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
	manifest, err := client.Scan(cfg.Root, cfg.DeviceName)
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
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, err := client.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	fmt.Println("正在扫描工作区与相关 Codex 会话...")
	manifest, err := client.Scan(cfg.Root, cfg.DeviceName)
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "codex-continuity-publish-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	zipPath := filepath.Join(tempDir, "handoff.zip")
	build, err := client.BuildBundle(manifest, zipPath)
	if err != nil {
		return err
	}
	key, _ := cfg.KeyBytes()
	encryptedPath := filepath.Join(tempDir, "handoff.ccx")
	if err := client.EncryptFile(build.ZipPath, encryptedPath, key); err != nil {
		return err
	}
	projectName := fmt.Sprintf("%s 工作区（%d 个项目）", build.Manifest.RootName, len(build.Manifest.Projects))
	handoff, err := client.NewAPI(cfg).UploadHandoff(client.UploadMetadata{
		ProjectName:      projectName,
		WorkspaceKey:     build.Manifest.WorkspaceKey,
		SourceDeviceID:   cfg.DeviceID,
		TargetDeviceName: *target,
		Manifest:         build.Manifest,
	}, encryptedPath)
	if err != nil {
		return err
	}
	info, _ := os.Stat(encryptedPath)
	fmt.Printf("发布完成：%s\n项目：%d，会话：%d，包大小：%s\n",
		handoff.ID, len(build.Manifest.Projects), len(build.Manifest.Sessions), humanBytes(info.Size()))
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
	if len(handoffs) == 0 {
		return fmt.Errorf("没有可接管的交接包")
	}
	sort.Slice(handoffs, func(i, j int) bool { return handoffs[i].CreatedAt.After(handoffs[j].CreatedAt) })
	selected := handoffs[0]
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
		return fmt.Errorf("目标目录已存在：%s", *output)
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

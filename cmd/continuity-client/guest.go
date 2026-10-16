package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/neonet/codex-continuity/internal/client"
	"io"
	"os"
	"path/filepath"
)

type guestDraft struct {
	Stage    string                `json:"stage"`
	Metadata client.UploadMetadata `json:"metadata"`
	SHA256   string                `json:"sha256"`
	Size     int64                 `json:"size"`
}

func guestPrepareCommand(args []string) error {
	flags := flag.NewFlagSet("guest-prepare", flag.ContinueOnError)
	config := flags.String("config", "", "")
	folder := flags.String("folder", "", "")
	guest := flags.String("guest", "", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *folder == "" || len(*guest) < 20 {
		return fmt.Errorf("缺少本机游客标识或预览目录")
	}
	cfg, err := client.LoadConfig(*config)
	if err != nil {
		return err
	}
	cfg.SyncScope.After = ""
	build, path, cleanup, err := buildEncryptedSnapshot(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	file.Close()
	if err != nil {
		return err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	stage, err := client.NewAPI(cfg).PreviewGuest(*guest, digest, size)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(*folder, 0700); err != nil {
		return err
	}
	if err = copyFileExclusive(path, filepath.Join(*folder, "snapshot.ccx")); err != nil {
		return err
	}
	draft := guestDraft{stage, client.UploadMetadata{ProjectName: "游客工作区归档", WorkspaceKey: build.Manifest.WorkspaceKey, SourceDeviceID: cfg.DeviceID, Manifest: build.Manifest}, digest, size}
	raw, _ := json.Marshal(draft)
	if err = os.WriteFile(filepath.Join(*folder, "preview.json"), raw, 0600); err != nil {
		return err
	}
	fmt.Printf("预览就绪：%d 条会话，%d 字节。尚未上传，确认成功才使用一次机会。", len(build.Manifest.Sessions), size)
	return nil
}
func guestConfirmCommand(args []string) error {
	flags := flag.NewFlagSet("guest-confirm", flag.ContinueOnError)
	config := flags.String("config", "", "")
	folder := flags.String("folder", "", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, err := client.LoadConfig(*config)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(*folder, "preview.json"))
	if err != nil {
		return err
	}
	var draft guestDraft
	if err = json.Unmarshal(raw, &draft); err != nil {
		return err
	}
	file, err := os.Open(filepath.Join(*folder, "snapshot.ccx"))
	if err != nil {
		return err
	}
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	file.Close()
	if err != nil || size != draft.Size || hex.EncodeToString(hash.Sum(nil)) != draft.SHA256 {
		return fmt.Errorf("本地预览包发生变化，请重新预览")
	}
	api := client.NewAPI(cfg)
	api.SetGuestStage(draft.Stage)
	_, err = api.UploadHandoff(draft.Metadata, filepath.Join(*folder, "snapshot.ccx"))
	if err != nil {
		return err
	}
	fmt.Print("游客快照已绑定；本机原始会话保持不变。")
	return nil
}

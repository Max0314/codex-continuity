package client

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxSessionFileSize = 128 * 1024 * 1024
	maxSessionTotal    = 350 * 1024 * 1024
	maxPatchSize       = 4 * 1024 * 1024
)

type Manifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	CreatedAt     time.Time      `json:"createdAt"`
	DeviceName    string         `json:"deviceName"`
	RootName      string         `json:"rootName"`
	WorkspaceKey  string         `json:"workspaceKey"`
	Projects      []ProjectState `json:"projects"`
	Sessions      []SessionState `json:"sessions"`
	Notes         []string       `json:"notes,omitempty"`
}

type ProjectState struct {
	Name           string   `json:"name"`
	RelativePath   string   `json:"relativePath"`
	Branch         string   `json:"branch,omitempty"`
	Commit         string   `json:"commit,omitempty"`
	Dirty          bool     `json:"dirty"`
	ChangedFiles   []string `json:"changedFiles,omitempty"`
	RemoteNames    []string `json:"remoteNames,omitempty"`
	WorkingPatch   string   `json:"workingPatch,omitempty"`
	StagedPatch    string   `json:"stagedPatch,omitempty"`
	InspectionNote string   `json:"inspectionNote,omitempty"`
}

type SessionState struct {
	ID           string    `json:"id"`
	ThreadName   string    `json:"threadName,omitempty"`
	RelativeCWD  string    `json:"relativeCwd"`
	ModifiedAt   time.Time `json:"modifiedAt"`
	ArchivePath  string    `json:"archivePath,omitempty"`
	Size         int64     `json:"size"`
	Included     bool      `json:"included"`
	SkippedCause string    `json:"skippedCause,omitempty"`
	sourcePath   string
}

type BuildResult struct {
	Manifest Manifest
	ZipPath  string
}

func Scan(root, deviceName string) (Manifest, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Manifest{}, err
	}
	projects, err := scanProjects(absRoot)
	if err != nil {
		return Manifest{}, err
	}
	sessions, notes := scanSessions(absRoot)
	return Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		DeviceName:    deviceName,
		RootName:      filepath.Base(absRoot),
		WorkspaceKey:  workspaceKey(absRoot),
		Projects:      projects,
		Sessions:      sessions,
		Notes:         notes,
	}, nil
}

func BuildBundle(manifest Manifest, destination string) (BuildResult, error) {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return BuildResult{}, err
	}
	writer := zip.NewWriter(file)
	cleanup := func(bundleErr error) (BuildResult, error) {
		_ = writer.Close()
		_ = file.Close()
		_ = os.Remove(destination)
		return BuildResult{}, bundleErr
	}

	for index := range manifest.Projects {
		project := &manifest.Projects[index]
		dir := fmt.Sprintf("projects/%02d-%s", index+1, safeArchiveName(project.Name))
		if project.WorkingPatch != "" {
			entry := dir + "/working.patch"
			if err := writeZipBytes(writer, entry, []byte(project.WorkingPatch)); err != nil {
				return cleanup(err)
			}
			project.WorkingPatch = entry
		}
		if project.StagedPatch != "" {
			entry := dir + "/staged.patch"
			if err := writeZipBytes(writer, entry, []byte(project.StagedPatch)); err != nil {
				return cleanup(err)
			}
			project.StagedPatch = entry
		}
	}

	var includedBytes int64
	for index := range manifest.Sessions {
		session := &manifest.Sessions[index]
		if session.sourcePath == "" {
			continue
		}
		if session.Size > maxSessionFileSize {
			session.SkippedCause = "单个会话文件超过 128 MiB"
			continue
		}
		if includedBytes+session.Size > maxSessionTotal {
			session.SkippedCause = "会话总量超过 350 MiB"
			continue
		}
		entryName := "sessions/" + safeArchiveName(session.ID) + ".jsonl"
		if err := writeZipFile(writer, entryName, session.sourcePath, session.Size); err != nil {
			session.SkippedCause = "读取会话失败: " + err.Error()
			continue
		}
		session.Included = true
		session.ArchivePath = entryName
		includedBytes += session.Size
	}

	if err := writeZipBytes(writer, "HANDOFF.md", []byte(renderHandoffMarkdown(manifest))); err != nil {
		return cleanup(err)
	}
	rawManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return cleanup(err)
	}
	if err := writeZipBytes(writer, "manifest.json", rawManifest); err != nil {
		return cleanup(err)
	}
	if err := writer.Close(); err != nil {
		file.Close()
		os.Remove(destination)
		return BuildResult{}, err
	}
	if err := file.Close(); err != nil {
		os.Remove(destination)
		return BuildResult{}, err
	}
	return BuildResult{Manifest: manifest, ZipPath: destination}, nil
}

func ExtractBundle(source, destination string) error {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()
	absDestination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absDestination, 0o700); err != nil {
		return err
	}
	for _, entry := range reader.File {
		target := filepath.Join(absDestination, filepath.FromSlash(entry.Name))
		if target != absDestination && !strings.HasPrefix(target, absDestination+string(os.PathSeparator)) {
			return fmt.Errorf("压缩包包含不安全路径")
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		src, err := entry.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		closeErr := dst.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func scanProjects(root string) ([]ProjectState, error) {
	projectDirs := []string{}
	rootDepth := pathDepth(root)
	skipNames := map[string]bool{
		".git": true, "node_modules": true, ".venv": true, "venv": true,
		"dist": true, "build": true, "target": true, ".idea": true, ".vscode": true,
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root && skipNames[strings.ToLower(entry.Name())] {
			return filepath.SkipDir
		}
		if pathDepth(path)-rootDepth > 3 {
			return filepath.SkipDir
		}
		if path != root {
			if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
				projectDirs = append(projectDirs, path)
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(projectDirs)
	projects := make([]ProjectState, 0, len(projectDirs))
	for _, projectDir := range projectDirs {
		relative, _ := filepath.Rel(root, projectDir)
		project := ProjectState{
			Name:         filepath.Base(projectDir),
			RelativePath: filepath.ToSlash(relative),
		}
		project.Branch, _ = gitOutput(projectDir, "branch", "--show-current")
		project.Commit, _ = gitOutput(projectDir, "rev-parse", "HEAD")
		status, err := gitOutput(projectDir, "status", "--porcelain=v1")
		if err != nil {
			project.InspectionNote = err.Error()
		} else {
			project.Dirty = strings.TrimSpace(status) != ""
			for _, line := range strings.Split(status, "\n") {
				if len(line) > 3 {
					project.ChangedFiles = append(project.ChangedFiles, strings.TrimSpace(line[3:]))
				}
			}
		}
		remotes, _ := gitOutput(projectDir, "remote")
		if remotes != "" {
			project.RemoteNames = strings.Fields(remotes)
		}
		if project.Dirty {
			project.WorkingPatch, _ = limitedGitOutput(projectDir, maxPatchSize, "diff", "--binary", "--no-ext-diff")
			project.StagedPatch, _ = limitedGitOutput(projectDir, maxPatchSize, "diff", "--binary", "--cached", "--no-ext-diff")
		}
		projects = append(projects, project)
	}
	return projects, nil
}

func scanSessions(root string) ([]SessionState, []string) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, []string{"无法定位 Codex 配置目录"}
	}
	// Codex currently stores data in ~/.codex on all supported desktop platforms.
	codexRoot := filepath.Join(filepath.Dir(configDir), ".codex")
	if home, homeErr := os.UserHomeDir(); homeErr == nil {
		codexRoot = filepath.Join(home, ".codex")
	}
	sessionRoot := filepath.Join(codexRoot, "sessions")
	threadNames := readThreadNames(filepath.Join(codexRoot, "session_index.jsonl"))
	sessions := []SessionState{}
	_ = filepath.WalkDir(sessionRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			return nil
		}
		meta, err := readSessionMeta(path)
		if err != nil || meta.CWD == "" || !isUnderRoot(meta.CWD, root) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		relativeCWD, _ := filepath.Rel(root, meta.CWD)
		sessions = append(sessions, SessionState{
			ID:          meta.ID,
			ThreadName:  threadNames[meta.ID],
			RelativeCWD: filepath.ToSlash(relativeCWD),
			ModifiedAt:  info.ModTime().UTC(),
			Size:        info.Size(),
			sourcePath:  path,
		})
		return nil
	})
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].ModifiedAt.After(sessions[j].ModifiedAt) })
	if len(sessions) > 30 {
		sessions = sessions[:30]
	}
	notes := []string{
		"Codex 会话格式属于内部实现；交接包保存原始只读快照，不会直接覆盖目标电脑的 Codex 数据库。",
		"发布后继续产生的新消息不在本快照内；再次执行 publish 可生成新的交接。",
	}
	return sessions, notes
}

type sessionMeta struct {
	ID  string
	CWD string
}

func readSessionMeta(path string) (sessionMeta, error) {
	file, err := os.Open(path)
	if err != nil {
		return sessionMeta{}, err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(io.LimitReader(file, 4*1024*1024), 256*1024)
	line, err := reader.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return sessionMeta{}, err
	}
	var record struct {
		Type    string `json:"type"`
		Payload struct {
			SessionID string `json:"session_id"`
			ID        string `json:"id"`
			CWD       string `json:"cwd"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(line, &record); err != nil {
		return sessionMeta{}, err
	}
	id := record.Payload.SessionID
	if id == "" {
		id = record.Payload.ID
	}
	return sessionMeta{ID: id, CWD: record.Payload.CWD}, nil
}

func readThreadNames(indexPath string) map[string]string {
	result := map[string]string{}
	file, err := os.Open(indexPath)
	if err != nil {
		return result
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		var item struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if json.Unmarshal(scanner.Bytes(), &item) == nil && item.ID != "" {
			result[item.ID] = item.ThreadName
		}
	}
	return result
}

func gitOutput(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	output, err := command.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("git 命令超时")
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func limitedGitOutput(dir string, limit int64, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	output := &cappedBuffer{limit: limit}
	command.Stdout = output
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	result := output.String()
	if output.truncated {
		result += "\n\n[补丁已在 4 MiB 处截断]\n"
	}
	return result, nil
}

func writeZipBytes(writer *zip.Writer, name string, data []byte) error {
	entry, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, bytes.NewReader(data))
	return err
}

func writeZipFile(writer *zip.Writer, name, source string, size int64) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	entry, err := writer.Create(name)
	if err != nil {
		return err
	}
	written, err := io.CopyN(entry, file, size)
	if errors.Is(err, io.EOF) && written == size {
		return nil
	}
	return err
}

type cappedBuffer struct {
	data      bytes.Buffer
	limit     int64
	written   int64
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	originalLength := len(p)
	remaining := b.limit - b.written
	if remaining > 0 {
		toWrite := int64(len(p))
		if toWrite > remaining {
			toWrite = remaining
		}
		_, _ = b.data.Write(p[:toWrite])
		b.written += toWrite
	}
	if int64(originalLength) > remaining {
		b.truncated = true
	}
	return originalLength, nil
}

func (b *cappedBuffer) String() string {
	return b.data.String()
}

func renderHandoffMarkdown(manifest Manifest) string {
	var builder strings.Builder
	builder.WriteString("# Codex 工作接力\n\n")
	builder.WriteString("这是一份只读交接快照。请先核对 Git 分支与提交，再让 Codex 读取本文件、`manifest.json`、相关补丁和需要的会话记录。\n\n")
	builder.WriteString(fmt.Sprintf("- 发布时间：%s\n", manifest.CreatedAt.Local().Format("2006-01-02 15:04:05")))
	builder.WriteString(fmt.Sprintf("- 来源设备：%s\n", manifest.DeviceName))
	builder.WriteString(fmt.Sprintf("- 工作区：%s\n", manifest.RootName))
	builder.WriteString(fmt.Sprintf("- 项目数：%d\n", len(manifest.Projects)))
	builder.WriteString(fmt.Sprintf("- 相关会话数：%d\n\n", len(manifest.Sessions)))
	builder.WriteString("## 项目状态\n\n")
	builder.WriteString("| 项目 | 相对目录 | 分支 | 提交 | 工作区 |\n|---|---|---|---|---|\n")
	for _, project := range manifest.Projects {
		state := "干净"
		if project.Dirty {
			state = "有未提交变更"
		}
		commit := project.Commit
		if len(commit) > 10 {
			commit = commit[:10]
		}
		builder.WriteString(fmt.Sprintf("| %s | `%s` | `%s` | `%s` | %s |\n",
			project.Name, project.RelativePath, project.Branch, commit, state))
	}
	builder.WriteString("\n## 建议接管提示词\n\n")
	builder.WriteString("> 请读取当前 HANDOFF.md 和 manifest.json，核对目标项目 Git 状态。需要时查看 projects 下的补丁和 sessions 下的原始会话快照，然后总结上台电脑做到哪里、未完成事项、风险，并从未完成事项继续。不要直接覆盖现有工作区。\n")
	return builder.String()
}

func workspaceKey(root string) string {
	name := strings.ToLower(filepath.Base(filepath.Clean(root)))
	sum := sha256.Sum256([]byte("codex-continuity:" + name))
	return name + "-" + hex.EncodeToString(sum[:6])
}

func safeArchiveName(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_")
	name = strings.TrimSpace(replacer.Replace(name))
	if name == "" {
		return "unnamed"
	}
	return name
}

func isUnderRoot(path, root string) bool {
	absPath, err1 := filepath.Abs(path)
	absRoot, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil {
		return false
	}
	relative, err := filepath.Rel(absRoot, absPath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func pathDepth(path string) int {
	return len(strings.Split(filepath.Clean(path), string(os.PathSeparator)))
}

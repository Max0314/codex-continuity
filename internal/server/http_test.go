package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neonet/codex-continuity/internal/model"
)

func TestLoginAndClientHandoffFlow(t *testing.T) {
	dataDir := t.TempDir()
	// A missing temp directory proves large multipart uploads do not depend on
	// ParseMultipartForm spilling the blob to the operating-system temp path.
	missingTempDir := filepath.Join(dataDir, "missing-temp")
	t.Setenv("TMP", missingTempDir)
	t.Setenv("TEMP", missingTempDir)
	cfg := Config{
		Address:        ":0",
		DataDir:        dataDir,
		WebDir:         filepath.Join(dataDir, "missing-web"),
		AdminEmail:     "admin@example.com",
		AdminPassword:  "a-strong-test-password",
		AdminName:      "测试管理员",
		SessionTTL:     24 * 60 * 60 * 1e9,
		MaxUploadBytes: 20 << 20,
		DownloadDir:    filepath.Join(dataDir, "downloads"),
	}
	store, err := OpenStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(NewHTTPServer(cfg, store, logger))
	defer server.Close()

	loginBody := `{"email":"admin@example.com","password":"a-strong-test-password"}`
	loginResponse, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	defer loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", loginResponse.StatusCode)
	}
	if len(loginResponse.Cookies()) == 0 {
		t.Fatal("login did not set a session cookie")
	}
	sessionCookie := loginResponse.Cookies()[0]

	tokenRequest := authenticatedJSONRequest(t, http.MethodPost, server.URL+"/api/v1/tokens", `{"name":"测试电脑"}`, sessionCookie)
	tokenResponse, err := http.DefaultClient.Do(tokenRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer tokenResponse.Body.Close()
	var tokenPayload struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(tokenResponse.Body).Decode(&tokenPayload); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tokenPayload.Secret, "ct_") {
		t.Fatalf("unexpected API token: %q", tokenPayload.Secret)
	}

	diagnosticRequest, _ := http.NewRequest(
		http.MethodPost,
		server.URL+"/api/v1/client/diagnostics/upload-test",
		bytes.NewReader(bytes.Repeat([]byte{0x42}, 64*1024)),
	)
	diagnosticRequest.Header.Set("Authorization", "Bearer "+tokenPayload.Secret)
	diagnosticRequest.Header.Set("Content-Type", "application/octet-stream")
	diagnosticResponse, err := http.DefaultClient.Do(diagnosticRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer diagnosticResponse.Body.Close()
	var diagnosticPayload struct {
		ReceivedBytes int    `json:"receivedBytes"`
		SHA256        string `json:"sha256"`
		Discarded     bool   `json:"discarded"`
	}
	if err := json.NewDecoder(diagnosticResponse.Body).Decode(&diagnosticPayload); err != nil {
		t.Fatal(err)
	}
	if diagnosticResponse.StatusCode != http.StatusOK || diagnosticPayload.ReceivedBytes != 64*1024 || !diagnosticPayload.Discarded || len(diagnosticPayload.SHA256) != 64 {
		t.Fatalf("unexpected upload test response: status=%d payload=%#v", diagnosticResponse.StatusCode, diagnosticPayload)
	}

	deviceRequest := bearerJSONRequest(t, http.MethodPost, server.URL+"/api/v1/client/devices", `{
	  "id":"","name":"办公室电脑","hostname":"office-pc","os":"windows/amd64",
	  "clientVersion":"0.1.0","lastSeenAt":"0001-01-01T00:00:00Z","createdAt":"0001-01-01T00:00:00Z"
	}`, tokenPayload.Secret)
	deviceResponse, err := http.DefaultClient.Do(deviceRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer deviceResponse.Body.Close()
	var devicePayload struct {
		Device model.Device `json:"device"`
	}
	if err := json.NewDecoder(deviceResponse.Body).Decode(&devicePayload); err != nil {
		t.Fatal(err)
	}
	if devicePayload.Device.ID == "" {
		t.Fatal("device was not assigned an ID")
	}

	handoffID := uploadTestHandoff(t, server.URL, tokenPayload.Secret, devicePayload.Device.ID)
	listRequest, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/client/handoffs?status=pending&target=%E5%85%AC%E5%8F%B8%E7%94%B5%E8%84%91", nil)
	listRequest.Header.Set("Authorization", "Bearer "+tokenPayload.Secret)
	listResponse, err := http.DefaultClient.Do(listRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer listResponse.Body.Close()
	var listPayload struct {
		Handoffs []model.Handoff `json:"handoffs"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&listPayload); err != nil {
		t.Fatal(err)
	}
	if len(listPayload.Handoffs) != 1 || listPayload.Handoffs[0].ID != handoffID {
		t.Fatalf("unexpected handoff list: %#v", listPayload.Handoffs)
	}
	if listPayload.Handoffs[0].SourceDeviceOS != "windows/amd64" {
		t.Fatalf("handoff source OS = %q, want windows/amd64", listPayload.Handoffs[0].SourceDeviceOS)
	}

	downloadRequest, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/client/handoffs/"+handoffID+"/blob", nil)
	downloadRequest.Header.Set("Authorization", "Bearer "+tokenPayload.Secret)
	downloadResponse, err := http.DefaultClient.Do(downloadRequest)
	if err != nil {
		t.Fatal(err)
	}
	downloaded, _ := io.ReadAll(downloadResponse.Body)
	downloadResponse.Body.Close()
	if string(downloaded) != "encrypted-test-payload" {
		t.Fatalf("downloaded payload = %q", downloaded)
	}

	claimRequest := bearerJSONRequest(t, http.MethodPost, server.URL+"/api/v1/client/handoffs/"+handoffID+"/claim", `{"targetDeviceName":"公司电脑"}`, tokenPayload.Secret)
	claimResponse, err := http.DefaultClient.Do(claimRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer claimResponse.Body.Close()
	if claimResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("claim status = %d", claimResponse.StatusCode)
	}

	largeHandoffID := uploadSizedTestHandoff(
		t,
		server.URL,
		tokenPayload.Secret,
		devicePayload.Device.ID,
		17<<20,
	)
	if largeHandoffID == "" {
		t.Fatal("17 MiB streaming handoff was not created")
	}
}

func uploadTestHandoff(t *testing.T, baseURL, token, deviceID string) string {
	return uploadSizedTestHandoff(t, baseURL, token, deviceID, len("encrypted-test-payload"))
}

func uploadSizedTestHandoff(t *testing.T, baseURL, token, deviceID string, payloadSize int) string {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metadata := map[string]any{
		"projectName":      "code_CPL 工作区（3 个项目）",
		"workspaceKey":     "code_cpl-123",
		"sourceDeviceId":   deviceID,
		"targetDeviceName": "公司电脑",
		"manifest":         map[string]any{"projects": []any{"a", "b", "c"}},
	}
	rawMetadata, _ := json.Marshal(metadata)
	if err := writer.WriteField("metadata", string(rawMetadata)); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("blob", "handoff.ccx")
	if err != nil {
		t.Fatal(err)
	}
	uploadPayload := bytes.Repeat([]byte("encrypted-test-payload"), payloadSize/len("encrypted-test-payload")+1)
	if _, err := part.Write(uploadPayload[:payloadSize]); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	request, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/client/handoffs", &body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("upload status = %d, body = %s", response.StatusCode, raw)
	}
	var payload struct {
		Handoff model.Handoff `json:"handoff"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload.Handoff.ID
}

func authenticatedJSONRequest(t *testing.T, method, url, body string, cookie *http.Cookie) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	return request
}

func bearerJSONRequest(t *testing.T, method, url, body, token string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	return request
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestSaveUploadEnforcesBlobLimitAndRemovesPartialFile(t *testing.T) {
	t.Parallel()
	destination := filepath.Join(t.TempDir(), "oversized.ccx")
	size, err := saveUpload(bytes.NewReader(bytes.Repeat([]byte{0x42}, 1025)), destination, 1024)
	if !errors.Is(err, errUploadTooLarge) {
		t.Fatalf("saveUpload error = %v, want errUploadTooLarge", err)
	}
	if size != 0 {
		t.Fatalf("saveUpload size = %d, want 0", size)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("partial upload was not removed: %v", statErr)
	}
}

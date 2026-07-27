package server

import (
	"bytes"
	"encoding/json"
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
	t.Parallel()
	dataDir := t.TempDir()
	cfg := Config{
		Address:        ":0",
		DataDir:        dataDir,
		WebDir:         filepath.Join(dataDir, "missing-web"),
		AdminEmail:     "admin@example.com",
		AdminPassword:  "a-strong-test-password",
		AdminName:      "测试管理员",
		SessionTTL:     24 * 60 * 60 * 1e9,
		MaxUploadBytes: 16 << 20,
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
}

func uploadTestHandoff(t *testing.T, baseURL, token, deviceID string) string {
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
	if _, err := part.Write([]byte("encrypted-test-payload")); err != nil {
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

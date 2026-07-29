package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
		Address:          ":0",
		DataDir:          dataDir,
		WebDir:           filepath.Join(dataDir, "missing-web"),
		AdminEmail:       "admin@example.com",
		AdminPassword:    "a-strong-test-password",
		AdminName:        "测试管理员",
		SessionTTL:       24 * 60 * 60 * 1e9,
		ClientAccessTTL:  15 * 60 * 1e9,
		ClientRefreshTTL: 30 * 24 * 60 * 60 * 1e9,
		MaxUploadBytes:   20 << 20,
		DownloadDir:      filepath.Join(dataDir, "downloads"),
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

	registerRequest, _ := http.NewRequest(
		http.MethodPost,
		server.URL+"/api/v1/client/auth/register",
		strings.NewReader(fmt.Sprintf(`{
		  "username":"cpl","displayName":"测试管理员","password":"a-new-strong-password",
		  "keySalt":"test-salt","wrappedKey":"test-wrapped-key",
		  "recoveryKeyHash":%q,"legacyToken":%q
		}`, strings.Repeat("b", 64), tokenPayload.Secret)),
	)
	registerRequest.Header.Set("Content-Type", "application/json")
	registerResponse, err := http.DefaultClient.Do(registerRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer registerResponse.Body.Close()
	var clientAuth struct {
		User         model.User `json:"user"`
		AccessToken  string     `json:"accessToken"`
		RefreshToken string     `json:"refreshToken"`
	}
	if err := json.NewDecoder(registerResponse.Body).Decode(&clientAuth); err != nil {
		t.Fatal(err)
	}
	if registerResponse.StatusCode != http.StatusCreated ||
		clientAuth.User.Username != "cpl" ||
		!strings.HasPrefix(clientAuth.AccessToken, "ca_") ||
		!strings.HasPrefix(clientAuth.RefreshToken, "cr_") {
		t.Fatalf("unexpected client registration: status=%d payload=%#v", registerResponse.StatusCode, clientAuth)
	}

	retryRegisterRequest, _ := http.NewRequest(
		http.MethodPost,
		server.URL+"/api/v1/client/auth/register",
		strings.NewReader(fmt.Sprintf(`{
		  "username":"cpl","displayName":"测试管理员","password":"a-new-strong-password",
		  "keySalt":"retry-salt","wrappedKey":"retry-wrapped-key",
		  "recoveryKeyHash":%q,"legacyToken":%q
		}`, strings.Repeat("b", 64), tokenPayload.Secret)),
	)
	retryRegisterRequest.Header.Set("Content-Type", "application/json")
	retryRegisterResponse, err := http.DefaultClient.Do(retryRegisterRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer retryRegisterResponse.Body.Close()
	var retriedClientAuth struct {
		User         model.User `json:"user"`
		AccessToken  string     `json:"accessToken"`
		RefreshToken string     `json:"refreshToken"`
		KeySalt      string     `json:"keySalt"`
		WrappedKey   string     `json:"wrappedKey"`
	}
	if err := json.NewDecoder(retryRegisterResponse.Body).Decode(&retriedClientAuth); err != nil {
		t.Fatal(err)
	}
	if retryRegisterResponse.StatusCode != http.StatusCreated ||
		retriedClientAuth.User.Username != "cpl" ||
		!strings.HasPrefix(retriedClientAuth.AccessToken, "ca_") ||
		retriedClientAuth.AccessToken == clientAuth.AccessToken ||
		retriedClientAuth.KeySalt != "test-salt" ||
		retriedClientAuth.WrappedKey != "test-wrapped-key" {
		t.Fatalf(
			"partial registration retry did not resume safely: status=%d payload=%#v",
			retryRegisterResponse.StatusCode,
			retriedClientAuth,
		)
	}
	clientAuth.AccessToken = retriedClientAuth.AccessToken
	clientAuth.RefreshToken = retriedClientAuth.RefreshToken

	diagnosticRequest, _ := http.NewRequest(
		http.MethodPost,
		server.URL+"/api/v1/client/diagnostics/upload-test",
		bytes.NewReader(bytes.Repeat([]byte{0x42}, 64*1024)),
	)
	diagnosticRequest.Header.Set("Authorization", "Bearer "+clientAuth.AccessToken)
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
	  "id":"mac_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	  "name":"办公室电脑","hostname":"office-pc","os":"windows/amd64",
	  "clientVersion":"0.1.0","lastSeenAt":"0001-01-01T00:00:00Z","createdAt":"0001-01-01T00:00:00Z"
	}`, clientAuth.AccessToken)
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
	if devicePayload.Device.ID != "mac_"+strings.Repeat("a", 64) {
		t.Fatalf("device ID = %q", devicePayload.Device.ID)
	}

	renameRequest := bearerJSONRequest(t, http.MethodPost, server.URL+"/api/v1/client/devices", `{
	  "id":"mac_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	  "name":"办公室电脑（已改名）","hostname":"office-pc","os":"windows/amd64",
	  "clientVersion":"0.4.0","lastSeenAt":"0001-01-01T00:00:00Z","createdAt":"0001-01-01T00:00:00Z"
	}`, clientAuth.AccessToken)
	renameResponse, err := http.DefaultClient.Do(renameRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer renameResponse.Body.Close()
	var renamedPayload struct {
		Device model.Device `json:"device"`
	}
	if err := json.NewDecoder(renameResponse.Body).Decode(&renamedPayload); err != nil {
		t.Fatal(err)
	}
	if renameResponse.StatusCode != http.StatusOK ||
		renamedPayload.Device.ID != devicePayload.Device.ID ||
		renamedPayload.Device.Name != "办公室电脑（已改名）" {
		t.Fatalf("unexpected renamed device: status=%d payload=%#v", renameResponse.StatusCode, renamedPayload)
	}

	handoffID := uploadTestHandoff(t, server.URL, clientAuth.AccessToken, devicePayload.Device.ID)
	listRequest, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/client/handoffs?status=pending&target=%E5%85%AC%E5%8F%B8%E7%94%B5%E8%84%91", nil)
	listRequest.Header.Set("Authorization", "Bearer "+clientAuth.AccessToken)
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
	downloadRequest.Header.Set("Authorization", "Bearer "+clientAuth.AccessToken)
	downloadResponse, err := http.DefaultClient.Do(downloadRequest)
	if err != nil {
		t.Fatal(err)
	}
	downloaded, _ := io.ReadAll(downloadResponse.Body)
	downloadResponse.Body.Close()
	if string(downloaded) != "encrypted-test-payload" {
		t.Fatalf("downloaded payload = %q", downloaded)
	}

	claimRequest := bearerJSONRequest(t, http.MethodPost, server.URL+"/api/v1/client/handoffs/"+handoffID+"/claim", `{"targetDeviceName":"公司电脑"}`, clientAuth.AccessToken)
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
		clientAuth.AccessToken,
		devicePayload.Device.ID,
		17<<20,
	)
	if largeHandoffID == "" {
		t.Fatal("17 MiB streaming handoff was not created")
	}

	refreshRequest, _ := http.NewRequest(
		http.MethodPost,
		server.URL+"/api/v1/client/auth/refresh",
		strings.NewReader(fmt.Sprintf(`{"refreshToken":%q}`, clientAuth.RefreshToken)),
	)
	refreshRequest.Header.Set("Content-Type", "application/json")
	refreshResponse, err := http.DefaultClient.Do(refreshRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer refreshResponse.Body.Close()
	var refreshed struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(refreshResponse.Body).Decode(&refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshResponse.StatusCode != http.StatusOK ||
		refreshed.AccessToken == clientAuth.AccessToken ||
		refreshed.RefreshToken == clientAuth.RefreshToken {
		t.Fatalf("client session was not rotated: status=%d payload=%#v", refreshResponse.StatusCode, refreshed)
	}
	expiredAccessRequest, _ := http.NewRequest(
		http.MethodGet,
		server.URL+"/api/v1/client/handoffs",
		nil,
	)
	expiredAccessRequest.Header.Set("Authorization", "Bearer "+clientAuth.AccessToken)
	expiredAccessResponse, err := http.DefaultClient.Do(expiredAccessRequest)
	if err != nil {
		t.Fatal(err)
	}
	expiredAccessResponse.Body.Close()
	if expiredAccessResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("rotated access token status = %d, want 401", expiredAccessResponse.StatusCode)
	}

	recoverRequest, _ := http.NewRequest(
		http.MethodPost,
		server.URL+"/api/v1/client/auth/recover",
		strings.NewReader(fmt.Sprintf(`{
		  "username":"cpl","password":"a-recovered-strong-password",
		  "recoveryKeyHash":%q,"keySalt":"test-salt-v2","wrappedKey":"test-wrapped-key-v2"
		}`, strings.Repeat("b", 64))),
	)
	recoverRequest.Header.Set("Content-Type", "application/json")
	recoverResponse, err := http.DefaultClient.Do(recoverRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer recoverResponse.Body.Close()
	var recovered struct {
		AccessToken string `json:"accessToken"`
		WrappedKey  string `json:"wrappedKey"`
	}
	if err := json.NewDecoder(recoverResponse.Body).Decode(&recovered); err != nil {
		t.Fatal(err)
	}
	if recoverResponse.StatusCode != http.StatusOK ||
		!strings.HasPrefix(recovered.AccessToken, "ca_") ||
		recovered.WrappedKey != "test-wrapped-key-v2" {
		t.Fatalf("unexpected recovery response: status=%d payload=%#v", recoverResponse.StatusCode, recovered)
	}

	loginWithRecoveredPassword, _ := http.NewRequest(
		http.MethodPost,
		server.URL+"/api/v1/client/auth/login",
		strings.NewReader(`{"username":"cpl","password":"a-recovered-strong-password"}`),
	)
	loginWithRecoveredPassword.Header.Set("Content-Type", "application/json")
	recoveredLoginResponse, err := http.DefaultClient.Do(loginWithRecoveredPassword)
	if err != nil {
		t.Fatal(err)
	}
	recoveredLoginResponse.Body.Close()
	if recoveredLoginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login with recovered password status = %d", recoveredLoginResponse.StatusCode)
	}
}

func TestMACDeviceIDMigratesLegacyDeviceAndHandoffReferences(t *testing.T) {
	dataDir := t.TempDir()
	cfg := Config{
		Address:          ":0",
		DataDir:          dataDir,
		WebDir:           filepath.Join(dataDir, "missing-web"),
		AdminEmail:       "admin@example.com",
		AdminPassword:    "a-strong-test-password",
		AdminName:        "测试管理员",
		SessionTTL:       24 * 60 * 60 * 1e9,
		ClientAccessTTL:  15 * 60 * 1e9,
		ClientRefreshTTL: 30 * 24 * 60 * 60 * 1e9,
		MaxUploadBytes:   20 << 20,
		DownloadDir:      filepath.Join(dataDir, "downloads"),
	}
	store, err := OpenStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	user, err := store.Authenticate(cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		t.Fatal(err)
	}

	legacyID := "legacy-device-id"
	legacy, err := store.UpsertDevice(user.ID, model.Device{
		ID:            legacyID,
		Name:          "ThinkPad-CPL",
		Hostname:      "ThinkPad-CPL",
		OS:            "windows/amd64",
		ClientVersion: "0.3.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := store.CreateHandoff(CreateHandoffParams{
		UserID:         user.ID,
		ProjectName:    "迁移测试",
		WorkspaceKey:   "migration-test",
		SourceDeviceID: legacy.ID,
		Manifest:       json.RawMessage(`{"version":1}`),
		BlobPath:       filepath.Join(dataDir, "legacy.ccx"),
		BlobSize:       42,
	})
	if err != nil {
		t.Fatal(err)
	}

	macID := "mac_" + strings.Repeat("c", 64)
	migrated, err := store.UpsertDevice(user.ID, model.Device{
		ID:            macID,
		Name:          "ThinkPad-CPL",
		Hostname:      "ThinkPad-CPL",
		OS:            "windows/amd64",
		ClientVersion: "0.4.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if migrated.ID != macID || !migrated.CreatedAt.Equal(legacy.CreatedAt) {
		t.Fatalf("unexpected migrated device: %#v (legacy %#v)", migrated, legacy)
	}

	var deviceCount, legacyCount int
	if err := store.db.QueryRow("SELECT COUNT(1) FROM devices WHERE user_id=?", user.ID).
		Scan(&deviceCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(1) FROM devices WHERE id=?", legacyID).
		Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	var handoffSource string
	if err := store.db.QueryRow("SELECT source_device_id FROM handoffs WHERE id=?", handoff.ID).
		Scan(&handoffSource); err != nil {
		t.Fatal(err)
	}
	if deviceCount != 1 || legacyCount != 0 || handoffSource != macID {
		t.Fatalf(
			"migration counts device=%d legacy=%d handoffSource=%q",
			deviceCount,
			legacyCount,
			handoffSource,
		)
	}

	renamed, err := store.UpsertDevice(user.ID, model.Device{
		ID:            macID,
		Name:          "ThinkPad-CPL（已改名）",
		Hostname:      "ThinkPad-CPL",
		OS:            "windows/amd64",
		ClientVersion: "0.4.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ID != macID || renamed.Name != "ThinkPad-CPL（已改名）" {
		t.Fatalf("unexpected renamed device: %#v", renamed)
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

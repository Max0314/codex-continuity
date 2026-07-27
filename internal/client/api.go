package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neonet/codex-continuity/internal/model"
)

type API struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewAPI(cfg Config) *API {
	return &API{
		baseURL: strings.TrimRight(cfg.ServerURL, "/"),
		token:   cfg.Token,
		client:  &http.Client{Timeout: 30 * time.Minute},
	}
}

func (a *API) Health() error {
	req, _ := http.NewRequest(http.MethodGet, a.baseURL+"/api/v1/health", nil)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return responseError(resp)
	}
	return nil
}

func (a *API) RegisterDevice(device model.Device) (model.Device, error) {
	var output struct {
		Device model.Device `json:"device"`
	}
	err := a.doJSON(http.MethodPost, "/api/v1/client/devices", device, &output)
	return output.Device, err
}

type UploadMetadata struct {
	ProjectName      string `json:"projectName"`
	WorkspaceKey     string `json:"workspaceKey"`
	SourceDeviceID   string `json:"sourceDeviceId"`
	TargetDeviceName string `json:"targetDeviceName"`
	Manifest         any    `json:"manifest"`
}

func (a *API) UploadHandoff(metadata UploadMetadata, encryptedPath string) (model.Handoff, error) {
	reader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	rawMetadata, err := json.Marshal(metadata)
	if err != nil {
		return model.Handoff{}, err
	}
	contentType := writer.FormDataContentType()
	go func() {
		var uploadErr error
		defer func() {
			if uploadErr == nil {
				uploadErr = writer.Close()
			}
			_ = pipeWriter.CloseWithError(uploadErr)
		}()
		if uploadErr = writer.WriteField("metadata", string(rawMetadata)); uploadErr != nil {
			return
		}
		part, partErr := writer.CreateFormFile("blob", filepath.Base(encryptedPath))
		if partErr != nil {
			uploadErr = partErr
			return
		}
		file, fileErr := os.Open(encryptedPath)
		if fileErr != nil {
			uploadErr = fileErr
			return
		}
		defer file.Close()
		_, uploadErr = io.Copy(part, file)
	}()
	req, err := http.NewRequest(http.MethodPost, a.baseURL+"/api/v1/client/handoffs", reader)
	if err != nil {
		reader.Close()
		return model.Handoff{}, err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Content-Type", contentType)
	resp, err := a.client.Do(req)
	if err != nil {
		return model.Handoff{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return model.Handoff{}, responseError(resp)
	}
	var output struct {
		Handoff model.Handoff `json:"handoff"`
	}
	err = json.NewDecoder(resp.Body).Decode(&output)
	return output.Handoff, err
}

func (a *API) ListHandoffs(deviceName string) ([]model.Handoff, error) {
	path := "/api/v1/client/handoffs?target=" + url.QueryEscape(deviceName)
	var output struct {
		Handoffs []model.Handoff `json:"handoffs"`
	}
	err := a.doJSON(http.MethodGet, path, nil, &output)
	return output.Handoffs, err
}

func (a *API) DownloadHandoff(id, destination string) error {
	req, err := http.NewRequest(http.MethodGet, a.baseURL+"/api/v1/client/handoffs/"+id+"/blob", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return responseError(resp)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	return closeErr
}

func (a *API) ClaimHandoff(id, deviceName string) error {
	return a.doJSON(
		http.MethodPost,
		"/api/v1/client/handoffs/"+id+"/claim",
		map[string]string{"targetDeviceName": deviceName},
		nil,
	)
}

func (a *API) doJSON(method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, a.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	if output != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(output)
	}
	return nil
}

func responseError(resp *http.Response) error {
	var payload struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload)
	if payload.Error == "" {
		payload.Error = resp.Status
	}
	return fmt.Errorf("服务端返回错误: %s", payload.Error)
}

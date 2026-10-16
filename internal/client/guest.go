package client

import "fmt"

func (a *API) PreviewGuest(guest, digest string, size int64) (string, error) {
	var response struct {
		Stage string `json:"stage"`
	}
	err := a.doJSON("POST", "/api/v1/client/guest-preview", map[string]any{"guest": guest, "sha256": digest, "size": size}, &response)
	if err == nil && response.Stage == "" {
		err = fmt.Errorf("预览响应无效")
	}
	return response.Stage, err
}
func (a *API) SetGuestStage(stage string) { a.guestStage = stage }

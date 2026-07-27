package model

import "time"

const (
	RoleAdmin  = "admin"
	RoleMember = "member"

	HandoffPending = "pending"
	HandoffClaimed = "claimed"
)

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Device struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Hostname      string    `json:"hostname"`
	OS            string    `json:"os"`
	ClientVersion string    `json:"clientVersion"`
	LastSeenAt    time.Time `json:"lastSeenAt"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Handoff struct {
	ID               string     `json:"id"`
	ProjectName      string     `json:"projectName"`
	WorkspaceKey     string     `json:"workspaceKey"`
	SourceDeviceID   string     `json:"sourceDeviceId"`
	SourceDeviceName string     `json:"sourceDeviceName"`
	TargetDeviceName string     `json:"targetDeviceName,omitempty"`
	Status           string     `json:"status"`
	Manifest         any        `json:"manifest,omitempty"`
	BlobSize         int64      `json:"blobSize"`
	CreatedAt        time.Time  `json:"createdAt"`
	ClaimedAt        *time.Time `json:"claimedAt,omitempty"`
}

type APIToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

type Overview struct {
	OnlineDevices  int       `json:"onlineDevices"`
	Pending        int       `json:"pendingHandoffs"`
	Monthly        int       `json:"monthlyHandoffs"`
	StorageBytes   int64     `json:"storageBytes"`
	RecentHandoffs []Handoff `json:"recentHandoffs"`
	Devices        []Device  `json:"devices"`
}

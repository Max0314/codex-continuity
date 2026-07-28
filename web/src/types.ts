export type Role = 'admin' | 'member'

export interface User {
  id: string
  email: string
  displayName: string
  role: Role
  createdAt: string
}

export interface Device {
  id: string
  name: string
  hostname: string
  os: string
  clientVersion: string
  lastSeenAt: string
  createdAt: string
}

export interface Handoff {
  id: string
  projectName: string
  workspaceKey: string
  sourceDeviceId: string
  sourceDeviceName: string
  targetDeviceName?: string
  status: 'pending' | 'claimed'
  manifest?: {
    projects?: unknown[]
    sessions?: unknown[]
  }
  blobSize: number
  createdAt: string
  claimedAt?: string
}

export interface ApiToken {
  id: string
  name: string
  prefix: string
  lastUsedAt?: string
  createdAt: string
}

export interface Overview {
  onlineDevices: number
  pendingHandoffs: number
  monthlyHandoffs: number
  storageBytes: number
  recentHandoffs: Handoff[]
  devices: Device[]
}

export type DesktopReleaseArtifactId = 'standard' | 'offline' | 'portable'
export type WebViewDistributionMode =
  | 'official-download-if-missing'
  | 'bundled-offline'
  | 'system-required'

export interface DesktopReleaseArtifact {
  id: DesktopReleaseArtifactId
  label: string
  fileName: string
  sizeBytes: number
  sha256: string
  webViewMode: WebViewDistributionMode
  recommended: boolean
  requiresInternetIfRuntimeMissing: boolean
}

export interface DesktopReleaseManifest {
  schemaVersion: number
  product: string
  version: string
  generatedAt: string
  platform: 'windows-x64'
  artifacts: DesktopReleaseArtifact[]
}

export type ThemeName = 'blue' | 'teal' | 'violet'
export type PageName =
  | 'overview'
  | 'devices'
  | 'handoffs'
  | 'users'
  | 'tokens'
  | 'downloads'
  | 'settings'

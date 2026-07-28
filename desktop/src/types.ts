export type ThemeName = 'blue' | 'teal' | 'violet'
export type PageName = 'conversations' | 'sync' | 'settings'
export type ConversationSyncStatus = 'synced' | 'local' | 'available' | 'queued' | 'conflict' | 'imported'
export type ContinuationMode = 'native-local' | 'context'
export type SyncPhase = 'idle' | 'scanning' | 'uploading' | 'downloading' | 'queued' | 'error' | 'paused'

export interface PublicSettings {
  serverUrl: string
  root: string
  deviceName: string
  deviceId: string
  autoSync: boolean
  launchAtStartup: boolean
  theme: ThemeName
  syncDays: 0 | 2 | 5 | 7
  selectedProjects: string[]
  includeArchived: boolean
  includeUnassigned: boolean
  maxBundleMiB: number
  hasToken: boolean
  hasEncryptionKey: boolean
  version: string
}

export interface SaveSettingsRequest {
  serverUrl: string
  root: string
  deviceName: string
  autoSync: boolean
  launchAtStartup: boolean
  theme: ThemeName
  syncDays: 0 | 2 | 5 | 7
  selectedProjects: string[]
  includeArchived: boolean
  includeUnassigned: boolean
  maxBundleMiB: number
}

export interface AuthStatus {
  authenticated: boolean
  username: string
  displayName: string
  serverUrl: string
  legacyAccountAvailable: boolean
  transportSecure: boolean
}

export interface LoginAccountRequest {
  serverUrl: string
  username: string
  password: string
}

export interface RegisterAccountRequest extends LoginAccountRequest {
  displayName: string
}

export interface RecoverAccountRequest extends LoginAccountRequest {
  recoveryKey: string
}

export interface AuthActionResult {
  status: AuthStatus
  message: string
  recoveryKey?: string
}

export interface ConnectionResult {
  ok: boolean
  latencyMs: number
  service: string
  checkedAt: string
}

export interface UploadTestResult {
  ok: boolean
  plaintextBytes: number
  encryptedBytes: number
  serverReceivedBytes: number
  latencyMs: number
  digest: string
  discarded: boolean
}

export interface Conversation {
  id: string
  title: string
  preview: string
  projectName: string
  relativeCwd: string
  updatedAt: string
  sourceDeviceName: string
  sourceDeviceOS?: string
  currentDevice: boolean
  local: boolean
  archived: boolean
  syncStatus: ConversationSyncStatus
  size: number
  handoffId?: string
  continuationMode: ContinuationMode
  archivePath?: string
  workspacePath?: string
  unassigned?: boolean
}

export interface SyncRuntime {
  phase: SyncPhase
  lastSuccessAt?: string
  lastAttemptAt?: string
  nextSyncAt?: string
  lastError?: string
  pendingUploads: number
  progress: number
  scannedConversations: number
}

export interface ActivityItem {
  id: string
  kind: string
  title: string
  detail: string
  tone: 'success' | 'info' | 'warning' | 'error'
  time: string
}

export interface DashboardSnapshot {
  configured: boolean
  settings: PublicSettings
  connection?: ConnectionResult
  conversations: Conversation[]
  syncProjects: SyncProjectOption[]
  sync: SyncRuntime
  activities: ActivityItem[]
}

export interface SyncProjectOption {
  relativePath: string
  name: string
  conversationCount: number
  totalBytes: number
  lastUpdatedAt: string
}

export interface ActionResult {
  ok: boolean
  message: string
}

export interface ContinueResult extends ActionResult {
  mode: ContinuationMode
  sessionId: string
  workspacePath: string
  handoffPath?: string
  prompt?: string
}

export interface ArchiveResult extends ActionResult {
  path?: string
}

import { invoke } from '@tauri-apps/api/core'
import type {
  ActionResult,
  ArchiveResult,
  ConnectionResult,
  ContinueResult,
  Conversation,
  DashboardSnapshot,
  PublicSettings,
  SaveSettingsRequest,
  UploadTestResult,
} from './types'

const isTauri = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
const now = () => new Date().toISOString()

let currentSettings: PublicSettings = {
  serverUrl: 'http://127.0.0.1:18787',
  root: 'D:\\code_CPL',
  deviceName: '公司电脑',
  deviceId: 'device-company',
  autoSync: true,
  launchAtStartup: true,
  theme: 'blue',
  syncDays: 7,
  selectedProjects: [],
  includeArchived: false,
  maxBundleMiB: 500,
  hasToken: true,
  hasEncryptionKey: true,
  version: '0.3.1',
}

const mockConversations: Conversation[] = [
  {
    id: '019fa114-1c4f-7e02-88a4-b2acbf0c6701',
    title: '规划 Codex 对话同步方案',
    preview: '确定同步范围、冲突处理、离线优先以及跨设备续接边界',
    projectName: 'codex-continuity',
    relativeCwd: 'codex-continuity',
    updatedAt: new Date(Date.now() - 60_000).toISOString(),
    sourceDeviceName: '公司电脑',
    sourceDeviceOS: 'Windows 11',
    currentDevice: true,
    local: true,
    archived: false,
    syncStatus: 'synced',
    size: 2_621_440,
    handoffId: 'h_01',
    continuationMode: 'native-local',
  },
  {
    id: 'conversation-bi-center',
    title: '汇总分析周报与通知问题',
    preview: '整理本周数据波动、通知延迟原因和后续修复项',
    projectName: 'bi_center',
    relativeCwd: 'bi_center',
    updatedAt: new Date(Date.now() - 3_240_000).toISOString(),
    sourceDeviceName: '办公室电脑',
    sourceDeviceOS: 'Windows 11',
    currentDevice: false,
    local: false,
    archived: false,
    syncStatus: 'available',
    size: 8_912_896,
    handoffId: 'h_02',
    continuationMode: 'context',
  },
  {
    id: 'conversation-bi-center-dingtalk',
    title: '评估接入钉钉荣誉功能',
    preview: '确认开放平台权限、消息卡片流程和审计边界',
    projectName: 'bi_center',
    relativeCwd: 'bi_center',
    updatedAt: new Date(Date.now() - 18_000_000).toISOString(),
    sourceDeviceName: '公司电脑',
    sourceDeviceOS: 'Windows 11',
    currentDevice: true,
    local: true,
    archived: false,
    syncStatus: 'synced',
    size: 3_420_160,
    continuationMode: 'native-local',
  },
  {
    id: 'conversation-bi-center-points',
    title: '核查禅道积分差额',
    preview: '对照月度基线检查积分规则、部门差额和异常用户',
    projectName: 'bi_center',
    relativeCwd: 'bi_center',
    updatedAt: new Date(Date.now() - 92_000_000).toISOString(),
    sourceDeviceName: '办公室电脑',
    sourceDeviceOS: 'Windows 11',
    currentDevice: false,
    local: false,
    archived: false,
    syncStatus: 'synced',
    size: 5_734_400,
    continuationMode: 'context',
  },
  {
    id: 'conversation-bi-center-api',
    title: '分析 TB 数据接口可用性',
    preview: '验证接口响应、字段稳定性和数据延迟情况',
    projectName: 'bi_center',
    relativeCwd: 'bi_center',
    updatedAt: new Date(Date.now() - 176_000_000).toISOString(),
    sourceDeviceName: '公司电脑',
    sourceDeviceOS: 'Windows 11',
    currentDevice: true,
    local: true,
    archived: false,
    syncStatus: 'local',
    size: 2_932_736,
    continuationMode: 'native-local',
  },
  {
    id: 'conversation-bi-center-rules',
    title: '制定积分规则',
    preview: '整理评分维度、默认权重和运营调整方式',
    projectName: 'bi_center',
    relativeCwd: 'bi_center',
    updatedAt: new Date(Date.now() - 351_000_000).toISOString(),
    sourceDeviceName: '办公室电脑',
    sourceDeviceOS: 'Windows 11',
    currentDevice: false,
    local: false,
    archived: false,
    syncStatus: 'synced',
    size: 1_802_240,
    continuationMode: 'context',
  },
  {
    id: 'conversation-bi-center-old',
    title: '整理历史报表归档',
    preview: '迁移旧版报表说明和长期不用的查询模板',
    projectName: 'bi_center',
    relativeCwd: 'bi_center',
    updatedAt: new Date(Date.now() - 1_296_000_000).toISOString(),
    sourceDeviceName: '办公室电脑',
    sourceDeviceOS: 'Windows 11',
    currentDevice: false,
    local: false,
    archived: true,
    syncStatus: 'synced',
    size: 1_048_576,
    continuationMode: 'context',
  },
  {
    id: 'conversation-model-isolation',
    title: '核查模型与数据隔离',
    preview: '检查数据库隔离策略、模型访问边界与脱敏要求',
    projectName: 'code_CPL',
    relativeCwd: 'code_CPL',
    updatedAt: new Date(Date.now() - 20_000_000).toISOString(),
    sourceDeviceName: '公司电脑',
    sourceDeviceOS: 'Windows 11',
    currentDevice: true,
    local: true,
    archived: false,
    syncStatus: 'queued',
    size: 4_263_936,
    continuationMode: 'native-local',
  },
  {
    id: 'conversation-prompt-template',
    title: '优化 Codex Prompt 模板',
    preview: '构建通用模板库，统一上下文、验收标准和输出约束',
    projectName: 'SOP',
    relativeCwd: 'SOP',
    updatedAt: new Date(Date.now() - 86_400_000).toISOString(),
    sourceDeviceName: '办公室电脑',
    sourceDeviceOS: 'Windows 11',
    currentDevice: false,
    local: false,
    archived: false,
    syncStatus: 'synced',
    size: 1_114_112,
    handoffId: 'h_03',
    continuationMode: 'context',
  },
]

let mockSnapshot: DashboardSnapshot = {
  configured: true,
  settings: currentSettings,
  connection: { ok: true, latencyMs: 36, service: 'codex-continuity', checkedAt: now() },
  conversations: mockConversations,
  syncProjects: [
    { relativePath: 'bi_center', name: 'bi_center', conversationCount: 6, totalBytes: 23_850_240, lastUpdatedAt: mockConversations[1].updatedAt },
    { relativePath: 'codex-continuity', name: 'codex-continuity', conversationCount: 1, totalBytes: 2_621_440, lastUpdatedAt: mockConversations[0].updatedAt },
    { relativePath: 'SOP', name: 'SOP', conversationCount: 1, totalBytes: 1_114_112, lastUpdatedAt: mockConversations[8].updatedAt },
  ],
  sync: {
    phase: 'idle',
    lastSuccessAt: new Date(Date.now() - 60_000).toISOString(),
    nextSyncAt: new Date(Date.now() + 240_000).toISOString(),
    pendingUploads: 1,
    progress: 100,
    scannedConversations: mockConversations.length,
  },
  activities: [
    { id: 'a1', kind: 'sync', title: '同步完成', detail: '3 条会话已确认，1 条进入上传队列', tone: 'success', time: new Date(Date.now() - 60_000).toISOString() },
    { id: 'a2', kind: 'device', title: '办公室电脑已上线', detail: 'Windows 11 · 同一账户', tone: 'info', time: new Date(Date.now() - 180_000).toISOString() },
    { id: 'a3', kind: 'archive', title: '收到新的会话快照', detail: '汇总分析周报与通知问题 · 8.5 MB', tone: 'info', time: new Date(Date.now() - 3_240_000).toISOString() },
  ],
}

async function mockDelay<T>(value: T, delay = 360): Promise<T> {
  await new Promise((resolve) => window.setTimeout(resolve, delay))
  return value
}

async function chooseArchive(mode: 'open' | 'save') {
  if (!isTauri) return mode === 'open' ? 'D:\\Downloads\\continuity-backup.ccx' : 'D:\\Downloads\\codex-continuity.ccx'
  const dialog = await import('@tauri-apps/plugin-dialog')
  if (mode === 'open') {
    const selected = await dialog.open({
      multiple: false,
      directory: false,
      filters: [{ name: 'Codex Continuity 加密归档', extensions: ['ccx'] }],
    })
    return typeof selected === 'string' ? selected : undefined
  }
  return dialog.save({
    defaultPath: `codex-continuity-${new Date().toISOString().slice(0, 10)}.ccx`,
    filters: [{ name: 'Codex Continuity 加密归档', extensions: ['ccx'] }],
  })
}

export const desktopApi = {
  dashboard: () =>
    isTauri ? invoke<DashboardSnapshot>('get_dashboard') : mockDelay({ ...mockSnapshot, settings: currentSettings }),

  saveSettings: (request: SaveSettingsRequest) =>
    isTauri
      ? invoke<{ settings: PublicSettings; generatedKey?: string; connection: ConnectionResult }>('save_settings', { request })
      : mockDelay({
          settings: (currentSettings = {
            ...currentSettings,
            ...request,
            hasToken: Boolean(request.token) || currentSettings.hasToken,
            hasEncryptionKey: Boolean(request.encryptionKey) || currentSettings.hasEncryptionKey,
          }),
          generatedKey: request.encryptionKey || currentSettings.hasEncryptionKey
            ? undefined
            : 'hQPXbJcMT6mtIpboRjhHhfGuJs6ssSeQSdTeD1T0aZE',
          connection: { ok: true, latencyMs: 36, service: 'codex-continuity', checkedAt: now() },
        }),

  testConnection: () =>
    isTauri
      ? invoke<ConnectionResult>('test_connection')
      : mockDelay({ ok: true, latencyMs: 36, service: 'codex-continuity', checkedAt: now() }),

  testUpload: () =>
    isTauri
      ? invoke<UploadTestResult>('test_upload')
      : mockDelay({
          ok: true,
          plaintextBytes: 65_536,
          encryptedBytes: 65_564,
          serverReceivedBytes: 65_564,
          latencyMs: 128,
          digest: 'e1a2c3d4f5b6'.repeat(5),
          discarded: true,
        }, 680),

  syncNow: async () => {
    if (isTauri) return invoke<ActionResult>('sync_now')
    mockSnapshot = { ...mockSnapshot, sync: { ...mockSnapshot.sync, phase: 'uploading', progress: 48 } }
    await mockDelay(null, 700)
    const completedAt = now()
    mockSnapshot = {
      ...mockSnapshot,
      conversations: mockSnapshot.conversations.map((item) => ({ ...item, syncStatus: 'synced' })),
      sync: { ...mockSnapshot.sync, phase: 'idle', lastAttemptAt: completedAt, lastSuccessAt: completedAt, pendingUploads: 0, progress: 100 },
    }
    return { ok: true, message: '同步完成，所有会话均为最新状态' }
  },

  continueConversation: (conversationId: string) => {
    if (isTauri) return invoke<ContinueResult>('continue_conversation', { conversationId })
    const conversation = mockSnapshot.conversations.find((item) => item.id === conversationId)
    const mode = conversation?.continuationMode || 'context'
    return mockDelay({
      ok: true,
      message: mode === 'native-local' ? '该会话已在本机，可从 Codex 任务列表继续' : '续接内容已经准备完成',
      mode,
      sessionId: conversationId,
      workspacePath: `${currentSettings.root}\\${conversation?.relativeCwd || ''}`,
      handoffPath: mode === 'context' ? `D:\\code_CPL\\.codex-continuity\\handoffs\\${conversation?.handoffId || 'demo'}\\HANDOFF.md` : undefined,
      prompt: mode === 'context'
        ? '请读取 HANDOFF.md，核对项目状态、未完成事项和风险，然后从未完成事项继续。'
        : '该会话已经存在于本机。请在 Codex 任务列表中搜索标题或会话 ID，然后从原对话继续。',
    }, 650)
  },

  exportArchive: async () => {
    const output = await chooseArchive('save')
    if (!output) return { ok: false, message: '已取消导出' } satisfies ArchiveResult
    return isTauri
      ? invoke<ArchiveResult>('export_archive', { output })
      : mockDelay({ ok: true, message: '加密归档已导出', path: output })
  },

  importArchive: async () => {
    const input = await chooseArchive('open')
    if (!input) return { ok: false, message: '已取消导入' } satisfies ArchiveResult
    return isTauri
      ? invoke<ArchiveResult>('import_archive', { input })
      : mockDelay({ ok: true, message: '归档已导入到本地续接目录', path: input })
  },

  setAutoSync: async (enabled: boolean) => {
    if (isTauri) return invoke<PublicSettings>('set_auto_sync', { enabled })
    currentSettings = { ...currentSettings, autoSync: enabled }
    mockSnapshot = {
      ...mockSnapshot,
      settings: currentSettings,
      sync: { ...mockSnapshot.sync, phase: enabled ? 'idle' : 'paused' },
    }
    return mockDelay(currentSettings, 180)
  },

  setTheme: async (theme: PublicSettings['theme']) => {
    if (isTauri) return invoke<PublicSettings>('set_theme', { theme })
    currentSettings = { ...currentSettings, theme }
    mockSnapshot = { ...mockSnapshot, settings: currentSettings }
    return mockDelay(currentSettings, 120)
  },

  showMain: (page?: string) => (isTauri ? invoke<void>('show_main_window', { page }) : Promise.resolve()),
  hideTray: () => (isTauri ? invoke<void>('hide_tray_window') : Promise.resolve()),
  quit: () => (isTauri ? invoke<void>('quit_app') : Promise.resolve()),
}

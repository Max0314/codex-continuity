import {
  FormEvent,
  ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react'
import {
  Activity,
  Archive,
  ArrowDownToLine,
  ArrowUpFromLine,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  Clock3,
  Cloud,
  Copy,
  Download,
  FileClock,
  FileKey2,
  FolderGit2,
  HardDrive,
  History,
  Import,
  KeyRound,
  Link2,
  ListFilter,
  LogOut,
  Menu,
  MessageSquareText,
  Monitor,
  Play,
  RefreshCw,
  Search,
  Server,
  Settings,
  ShieldCheck,
  Upload,
  X,
} from 'lucide-react'
import { desktopApi } from './api'
import type {
  ContinueResult,
  Conversation,
  ConversationSyncStatus,
  DashboardSnapshot,
  PageName,
  PublicSettings,
  SaveSettingsRequest,
  SyncProjectOption,
  ThemeName,
  UploadTestResult,
} from './types'
import './styles.css'

type Toast = { tone: 'success' | 'error' | 'info'; text: string } | null
type StatusFilter = 'all' | ConversationSyncStatus
type TimeRange = '7d' | '30d' | 'all'
type ConversationProjectGroup = {
  name: string
  conversations: Conversation[]
  latestUpdatedAt: string
  attention: number
}
const selectedConversationStorageKey = 'codex-continuity:selected-conversation'
const collapsedProjectConversationLimit = 4

const navigation: Array<{ id: PageName; label: string; icon: typeof MessageSquareText }> = [
  { id: 'conversations', label: '会话', icon: MessageSquareText },
  { id: 'sync', label: '同步', icon: RefreshCw },
  { id: 'settings', label: '设置', icon: Settings },
]

const themes: Array<{ id: ThemeName; label: string; color: string }> = [
  { id: 'blue', label: '蓝色', color: '#3b82f6' },
  { id: 'teal', label: '青绿', color: '#10b981' },
  { id: 'violet', label: '紫色', color: '#8b5cf6' },
]

export default function App() {
  const isTray = new URLSearchParams(window.location.search).get('view') === 'tray'
  useEffect(() => {
    document.body.classList.toggle('tray-body', isTray)
    return () => document.body.classList.remove('tray-body')
  }, [isTray])
  return isTray ? <TrayPanel /> : <DesktopApp />
}

function pageFromHash(): PageName {
  const value = window.location.hash.replace('#', '') as PageName
  return navigation.some((item) => item.id === value) ? value : 'conversations'
}

function DesktopApp() {
  const [snapshot, setSnapshot] = useState<DashboardSnapshot | null>(null)
  const [page, setPage] = useState<PageName>(pageFromHash)
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [busy, setBusy] = useState('')
  const [toast, setToast] = useState<Toast>(null)
  const [continuation, setContinuation] = useState<ContinueResult | null>(null)
  const [requestedConversationId, setRequestedConversationId] = useState(
    () => window.localStorage.getItem(selectedConversationStorageKey) || '',
  )

  const reload = useCallback(async (quiet = false) => {
    try {
      const next = await desktopApi.dashboard()
      setSnapshot(next)
      document.documentElement.dataset.theme = next.settings.theme
    } catch (error) {
      if (!quiet) setToast({ tone: 'error', text: errorMessage(error) })
    }
  }, [])

  useEffect(() => {
    reload()
    const onHashChange = () => setPage(pageFromHash())
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [reload])

  useEffect(() => {
    const interval = window.setInterval(() => {
      if (document.visibilityState === 'visible' && !busy) reload(true)
    }, 12_000)
    return () => window.clearInterval(interval)
  }, [busy, reload])

  useEffect(() => {
    if (!toast) return
    const timeout = window.setTimeout(() => setToast(null), 4200)
    return () => window.clearTimeout(timeout)
  }, [toast])

  useEffect(() => {
    const handleStorage = (event: StorageEvent) => {
      if (event.key === selectedConversationStorageKey && event.newValue) {
        setRequestedConversationId(event.newValue)
        setPage('conversations')
        window.location.hash = 'conversations'
      }
    }
    window.addEventListener('storage', handleStorage)
    return () => window.removeEventListener('storage', handleStorage)
  }, [])

  function navigate(next: PageName) {
    setPage(next)
    setSidebarOpen(false)
    window.location.hash = next
  }

  async function runAction<T>(
    key: string,
    action: () => Promise<T>,
    onSuccess?: (result: T) => void,
    refresh = true,
  ) {
    setBusy(key)
    try {
      const result = await action()
      onSuccess?.(result)
      if (refresh) await reload(true)
      return result
    } catch (error) {
      setToast({ tone: 'error', text: errorMessage(error) })
      return undefined
    } finally {
      setBusy('')
    }
  }

  if (!snapshot) return <LoadingScreen />

  const syncNow = () => runAction(
    'sync',
    desktopApi.syncNow,
    (result) => setToast({ tone: result.ok ? 'success' : 'info', text: result.message }),
  )

  const continueConversation = (conversation: Conversation) => runAction(
    `continue:${conversation.id}`,
    () => desktopApi.continueConversation(conversation.id),
    (result) => {
      setContinuation(result)
      setToast({ tone: 'success', text: result.message })
    },
  )

  const importArchive = () => runAction(
    'import',
    desktopApi.importArchive,
    (result) => {
      if (result.ok) setToast({ tone: 'success', text: result.message })
    },
  )

  const exportArchive = () => runAction(
    'export',
    desktopApi.exportArchive,
    (result) => {
      if (result.ok) setToast({ tone: 'success', text: result.message })
    },
    false,
  )

  const exportDiagnostics = () => runAction(
    'diagnostics',
    desktopApi.exportDiagnostics,
    (result) => {
      if (result.ok) setToast({ tone: 'success', text: result.message })
    },
    false,
  )

  return (
    <div className="desktop-shell">
      <TopBar settings={snapshot.settings} connection={snapshot.connection} onMenu={() => setSidebarOpen((value) => !value)} />
      <Sidebar
        page={page}
        open={sidebarOpen}
        snapshot={snapshot}
        onSelect={navigate}
      />
      {sidebarOpen ? <button className="sidebar-scrim" aria-label="关闭导航" onClick={() => setSidebarOpen(false)} /> : null}
      <main className="desktop-main">
        {page === 'conversations' ? (
          <ConversationsPage
            snapshot={snapshot}
            busy={busy}
            requestedConversationId={requestedConversationId}
            onSync={syncNow}
            onImport={importArchive}
            onExport={exportArchive}
            onEditSyncScope={() => {
              navigate('settings')
              window.setTimeout(
                () => document.getElementById('sync-scope-settings')?.scrollIntoView({ block: 'start' }),
                0,
              )
            }}
            onContinue={continueConversation}
          />
        ) : null}
        {page === 'sync' ? (
          <SyncPage
            snapshot={snapshot}
            busy={busy}
            onSync={syncNow}
            onAutoSync={(enabled) => runAction(
              'auto-sync',
              () => desktopApi.setAutoSync(enabled),
              () => setToast({ tone: 'success', text: enabled ? '自动同步已开启' : '自动同步已暂停' }),
            )}
            onExportDiagnostics={exportDiagnostics}
          />
        ) : null}
        {page === 'settings' ? (
          <SettingsPage
            settings={snapshot.settings}
            projects={snapshot.syncProjects}
            busy={busy}
            onRun={runAction}
            onSaved={(settings, generatedKey) => {
              setSnapshot({ ...snapshot, configured: true, settings })
              setToast({
                tone: 'success',
                text: generatedKey ? '配置已保存，请立即复制新生成的加密密钥' : '配置已保存并完成设备注册',
              })
            }}
            onToast={setToast}
          />
        ) : null}
      </main>
      {toast ? <ToastView toast={toast} onClose={() => setToast(null)} /> : null}
      {continuation ? <ContinuationDialog result={continuation} onClose={() => setContinuation(null)} /> : null}
    </div>
  )
}

function LoadingScreen() {
  return (
    <div className="loading-screen">
      <BrandMark />
      <span className="spinner" />
      <p>正在读取本机 Codex 会话…</p>
    </div>
  )
}

function TopBar({ settings, connection, onMenu }: {
  settings: PublicSettings
  connection?: DashboardSnapshot['connection']
  onMenu: () => void
}) {
  return (
    <header className="topbar">
      <button className="icon-button mobile-menu" onClick={onMenu} aria-label="打开导航"><Menu size={20} /></button>
      <div className="topbar-brand"><BrandMark /><strong>Codex Continuity</strong></div>
      <div className="topbar-actions">
        <div className="device-chip"><Monitor size={16} /><span>{settings.deviceName || '本机'}</span></div>
        <div className={`connection-badge ${connection ? 'online' : 'offline'}`}>
          <i />{connection ? '已连接' : '离线'}
        </div>
      </div>
    </header>
  )
}

function Sidebar({ page, open, snapshot, onSelect }: {
  page: PageName
  open: boolean
  snapshot: DashboardSnapshot
  onSelect: (page: PageName) => void
}) {
  const needsAttention = snapshot.conversations.filter((item) => ['available', 'queued', 'conflict'].includes(item.syncStatus)).length
  return (
    <aside className={`sidebar ${open ? 'open' : ''}`}>
      <nav aria-label="主导航">
        {navigation.map((item) => {
          const Icon = item.icon
          return (
            <button
              key={item.id}
              className={`nav-item ${page === item.id ? 'active' : ''}`}
              onClick={() => onSelect(item.id)}
              aria-current={page === item.id ? 'page' : undefined}
            >
              <Icon size={19} strokeWidth={1.8} />
              <span>{item.label}</span>
              {item.id === 'sync' && needsAttention > 0 ? <b>{needsAttention}</b> : null}
            </button>
          )
        })}
      </nav>
      <button className="service-status" onClick={() => onSelect('sync')}>
        <span className={`status-dot ${snapshot.connection ? '' : 'offline'}`} />
        <span>
          <strong>{syncPhaseLabel(snapshot)}</strong>
          <small>{snapshot.connection ? `服务器 ${snapshot.connection.latencyMs} ms` : '点击查看同步状态'}</small>
        </span>
        <ChevronRight size={17} />
      </button>
      <div className="sidebar-version"><span>Codex Continuity</span><span>v{snapshot.settings.version}</span></div>
    </aside>
  )
}

function ConversationsPage({
  snapshot,
  busy,
  requestedConversationId,
  onSync,
  onImport,
  onExport,
  onEditSyncScope,
  onContinue,
}: {
  snapshot: DashboardSnapshot
  busy: string
  requestedConversationId: string
  onSync: () => void
  onImport: () => void
  onExport: () => void
  onEditSyncScope: () => void
  onContinue: (conversation: Conversation) => void
}) {
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<StatusFilter>('all')
  const [timeRange, setTimeRange] = useState<TimeRange>('7d')
  const [projectFilter, setProjectFilter] = useState('all')
  const [selectedId, setSelectedId] = useState(snapshot.conversations[0]?.id || '')
  const [expandedProjects, setExpandedProjects] = useState<Set<string>>(() => new Set())
  const [fullyExpandedProjects, setFullyExpandedProjects] = useState<Set<string>>(() => new Set())
  const [groupsInitialized, setGroupsInitialized] = useState(false)

  useEffect(() => {
    if (!snapshot.conversations.some((item) => item.id === selectedId)) {
      setSelectedId(snapshot.conversations[0]?.id || '')
    }
  }, [selectedId, snapshot.conversations])

  useEffect(() => {
    if (requestedConversationId && snapshot.conversations.some((item) => item.id === requestedConversationId)) {
      setSelectedId(requestedConversationId)
    }
  }, [requestedConversationId, snapshot.conversations])

  const filtered = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    const timeCutoff = timeRange === 'all'
      ? Number.NEGATIVE_INFINITY
      : Date.now() - (timeRange === '7d' ? 7 : 30) * 24 * 60 * 60 * 1000
    return snapshot.conversations.filter((conversation) => {
      const matchesQuery = !normalized || [
        conversation.title,
        conversation.projectName,
        conversation.preview,
        conversation.sourceDeviceName,
      ].some((value) => value.toLowerCase().includes(normalized))
      const updatedAt = new Date(conversation.updatedAt).getTime()
      return matchesQuery
        && updatedAt >= timeCutoff
        && (projectFilter === 'all' || conversation.projectName === projectFilter)
        && (filter === 'all' || conversation.syncStatus === filter)
    })
  }, [filter, projectFilter, query, snapshot.conversations, timeRange])

  const projectOptions = useMemo(
    () => Array.from(new Set(snapshot.conversations.map((conversation) => conversation.projectName)))
      .filter(Boolean)
      .sort((left, right) => left.localeCompare(right, 'zh-CN')),
    [snapshot.conversations],
  )

  const projectGroups = useMemo<ConversationProjectGroup[]>(() => {
    const groups = new Map<string, Conversation[]>()
    filtered.forEach((conversation) => {
      const projectName = conversation.projectName || '未归类项目'
      const conversations = groups.get(projectName)
      if (conversations) conversations.push(conversation)
      else groups.set(projectName, [conversation])
    })
    return Array.from(groups.entries())
      .map(([name, conversations]) => {
        const sorted = [...conversations].sort(
          (left, right) => new Date(right.updatedAt).getTime() - new Date(left.updatedAt).getTime(),
        )
        return {
          name,
          conversations: sorted,
          latestUpdatedAt: sorted[0]?.updatedAt || '',
          attention: sorted.filter((item) => ['available', 'queued', 'conflict'].includes(item.syncStatus)).length,
        }
      })
      .sort(
        (left, right) => new Date(right.latestUpdatedAt).getTime() - new Date(left.latestUpdatedAt).getTime(),
      )
  }, [filtered])

  useEffect(() => {
    if (groupsInitialized || projectGroups.length === 0) return
    const initialProjects = projectGroups
      .filter((group, index) => group.attention > 0 || index === 0)
      .slice(0, 3)
      .map((group) => group.name)
    setExpandedProjects(new Set(initialProjects))
    setGroupsInitialized(true)
  }, [groupsInitialized, projectGroups])

  useEffect(() => {
    const requested = snapshot.conversations.find((item) => item.id === requestedConversationId)
    if (!requested) return
    setExpandedProjects((current) => {
      const next = new Set(current)
      next.add(requested.projectName || '未归类项目')
      return next
    })
  }, [requestedConversationId, snapshot.conversations])

  useEffect(() => {
    if (filtered.length === 0) return
    if (!filtered.some((item) => item.id === selectedId)) setSelectedId(filtered[0].id)
  }, [filtered, selectedId])

  const selected = filtered.find((item) => item.id === selectedId) || filtered[0]
  const attention = snapshot.conversations.filter((item) => ['available', 'queued', 'conflict'].includes(item.syncStatus)).length

  function toggleProject(projectName: string) {
    setExpandedProjects((current) => {
      const next = new Set(current)
      if (next.has(projectName)) next.delete(projectName)
      else next.add(projectName)
      return next
    })
  }

  function toggleProjectConversations(projectName: string) {
    setFullyExpandedProjects((current) => {
      const next = new Set(current)
      if (next.has(projectName)) next.delete(projectName)
      else next.add(projectName)
      return next
    })
  }

  return (
    <div className="page conversations-page">
      <PageHeader
        title="会话"
        subtitle={
          <span className="sync-summary">
            {attention === 0 ? <CheckCircle2 size={16} /> : <CircleAlert size={16} />}
            {attention === 0 ? '全部已同步' : `${attention} 项需要处理`}
            <i />最后同步：{displayTime(snapshot.sync.lastSuccessAt)}
          </span>
        }
        actions={(
          <>
            <button className="primary-button" onClick={onSync} disabled={busy === 'sync'}>
              {busy === 'sync' ? <Spinner /> : <RefreshCw size={17} />}立即同步
            </button>
            <button className="secondary-button" onClick={onImport} disabled={busy === 'import'}>
              {busy === 'import' ? <Spinner /> : <Import size={17} />}打开加密归档
            </button>
          </>
        )}
      />

      <div className="conversation-layout">
        <section className="conversation-panel">
          <div className="conversation-toolbar">
            <label className="search-field">
              <Search size={17} />
              <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索会话标题或项目" />
              {query ? <button onClick={() => setQuery('')} aria-label="清空搜索"><X size={15} /></button> : null}
            </label>
            <label className="filter-field project-filter">
              <FolderGit2 size={16} />
              <select
                value={projectFilter}
                onChange={(event) => setProjectFilter(event.target.value)}
                aria-label="查看项目"
              >
                <option value="all">全部项目</option>
                {projectOptions.map((project) => <option key={project} value={project}>{project}</option>)}
              </select>
              <ChevronDown size={15} />
            </label>
            <label className="filter-field">
              <ListFilter size={16} />
              <select value={filter} onChange={(event) => setFilter(event.target.value as StatusFilter)} aria-label="同步状态">
                <option value="all">全部状态</option>
                <option value="synced">已同步</option>
                <option value="available">可在本机继续</option>
                <option value="queued">等待上传</option>
                <option value="local">仅本机</option>
                <option value="conflict">需要处理</option>
                <option value="imported">已导入</option>
              </select>
              <ChevronDown size={15} />
            </label>
            <label className="filter-field time-filter">
              <Clock3 size={16} />
              <select value={timeRange} onChange={(event) => setTimeRange(event.target.value as TimeRange)} aria-label="时间范围">
                <option value="7d">最近一周</option>
                <option value="30d">最近 30 天</option>
                <option value="all">全部时间</option>
              </select>
              <ChevronDown size={15} />
            </label>
            <button className="scope-editor-link" onClick={onEditSyncScope}>
              <Settings size={15} />
              同步范围：{snapshot.settings.selectedProjects.length
                ? `${snapshot.settings.selectedProjects.length}个项目`
                : '全部项目'} · {snapshot.settings.syncDays ? `${snapshot.settings.syncDays}天` : '不限时间'}
            </button>
            <button className="toolbar-link" onClick={onExport} disabled={busy === 'export'}>
              <ArrowUpFromLine size={16} />导出离线备份
            </button>
          </div>

          <div className="project-list" aria-label="按项目分组的 Codex 会话">
            {projectGroups.map((group, groupIndex) => {
              const expanded = query.trim() ? true : expandedProjects.has(group.name)
              const fullyExpanded = fullyExpandedProjects.has(group.name)
              const visibleConversations = fullyExpanded
                ? group.conversations
                : group.conversations.slice(0, collapsedProjectConversationLimit)
              const hiddenCount = group.conversations.length - visibleConversations.length
              return (
                <section className={`project-group ${expanded ? 'expanded' : ''}`} key={group.name}>
                  <button
                    className="project-group-header"
                    onClick={() => toggleProject(group.name)}
                    aria-expanded={expanded}
                  >
                    <span className="project-group-icon"><FolderGit2 size={18} /></span>
                    <span className="project-group-name">
                      <strong>{group.name}</strong>
                      <small>{group.conversations.length} 条会话 · 最近更新 {displayTime(group.latestUpdatedAt)}</small>
                    </span>
                    {group.attention > 0 ? <span className="project-attention">{group.attention} 项待处理</span> : <span className="project-synced"><CheckCircle2 size={14} />已同步</span>}
                    {expanded ? <ChevronDown size={17} /> : <ChevronRight size={17} />}
                  </button>
                  {expanded ? (
                    <div className="project-conversations">
                      {visibleConversations.map((conversation, conversationIndex) => (
                        <ConversationRow
                          key={conversation.id}
                          conversation={conversation}
                          selected={selected?.id === conversation.id}
                          tone={(groupIndex + conversationIndex) % 4}
                          busy={busy === `continue:${conversation.id}`}
                          onSelect={() => setSelectedId(conversation.id)}
                          onContinue={() => onContinue(conversation)}
                        />
                      ))}
                      {group.conversations.length > collapsedProjectConversationLimit ? (
                        <button
                          className="project-show-more"
                          onClick={() => toggleProjectConversations(group.name)}
                        >
                          {fullyExpanded ? '收起更多会话' : `展开显示其余 ${hiddenCount} 条`}
                          {fullyExpanded ? <ChevronDown size={15} /> : <ChevronRight size={15} />}
                        </button>
                      ) : null}
                    </div>
                  ) : null}
                </section>
              )
            })}
            {filtered.length === 0 ? (
              <div className="empty-state">
                <Search size={26} />
                <strong>最近一周没有匹配的会话</strong>
                <span>调整搜索词、状态或时间范围。</span>
              </div>
            ) : null}
          </div>
          <div className="table-footer">
            <span>{projectGroups.length} 个项目 · {filtered.length} 条会话</span>
            <span>{timeRange === '7d' ? '默认显示最近一周' : timeRange === '30d' ? '显示最近 30 天' : '显示全部时间'} · {snapshot.settings.includeArchived ? '包含已归档会话' : '归档会话未纳入同步'}</span>
          </div>
        </section>

        <ConversationDetail
          conversation={selected}
          root={snapshot.settings.root}
          busy={selected ? busy === `continue:${selected.id}` : false}
          onContinue={() => selected && onContinue(selected)}
        />
      </div>
    </div>
  )
}

function ConversationRow({ conversation, selected, tone, busy, onSelect, onContinue }: {
  conversation: Conversation
  selected: boolean
  tone: number
  busy: boolean
  onSelect: () => void
  onContinue: () => void
}) {
  return (
    <div
      className={`conversation-row ${selected ? 'selected' : ''}`}
      onClick={onSelect}
    >
      <button className="conversation-title-cell" onClick={onSelect} aria-pressed={selected}>
        <span className={`conversation-icon tone-${tone}`}><MessageSquareText size={18} /></span>
        <span><strong>{conversation.title}{conversation.archived ? <em className="archive-tag">已归档</em> : null}</strong><small>{conversation.preview}</small></span>
      </button>
      <span className="time-cell"><strong>{displayTime(conversation.updatedAt)}</strong><small>{formatAbsoluteTime(conversation.updatedAt)}</small></span>
      <span className="device-cell"><Monitor size={15} /><span><strong>{conversation.sourceDeviceName}</strong><small>{deviceOSLabel(conversation)}{conversation.currentDevice ? ' · 当前设备' : ''}</small></span></span>
      <span className="status-cell"><StatusBadge status={conversation.syncStatus} /></span>
      <span className="action-cell">
        <button
          className="row-action"
          onClick={(event) => { event.stopPropagation(); onContinue() }}
          disabled={busy}
        >
          {busy ? <Spinner /> : <Play size={15} />}
          {conversation.continuationMode === 'native-local' ? '定位原任务' : '在此设备继续'}
        </button>
      </span>
    </div>
  )
}

function ConversationDetail({ conversation, root, busy, onContinue }: {
  conversation?: Conversation
  root: string
  busy: boolean
  onContinue: () => void
}) {
  if (!conversation) {
    return <aside className="conversation-detail empty"><MessageSquareText size={28} /><span>选择一条会话查看详情</span></aside>
  }
  return (
    <aside className="conversation-detail">
      <div className="detail-header"><strong>{conversation.title}</strong><StatusBadge status={conversation.syncStatus} /></div>
      <div className="detail-body">
        <DetailLine icon={<Monitor />} label="来源设备" value={`${conversation.sourceDeviceName} · ${deviceOSLabel(conversation)}`} />
        <DetailLine icon={<FolderGit2 />} label="项目" value={conversation.projectName} />
        <DetailLine
          icon={<Link2 />}
          label="工作目录映射"
          value={conversation.unassigned
            ? '无项目会话；跨设备续接时使用本机工作区根目录'
            : joinWindowsPath(root, conversation.relativeCwd)}
          code={!conversation.unassigned}
        />
        <DetailLine icon={<Clock3 />} label="最后更新" value={`${formatAbsoluteTime(conversation.updatedAt)} · ${displayTime(conversation.updatedAt)}`} />
        <DetailLine icon={<FileKey2 />} label="加密大小" value={humanBytes(conversation.size)} />
        <DetailLine
          icon={<Activity />}
          label="继续模式"
          value={conversation.continuationMode === 'native-local' ? '本机会话定位' : '安全上下文续接'}
        />
        <div className="continuation-note">
          {conversation.continuationMode === 'native-local'
            ? '该会话已存在于本机 Codex 历史中，应用会提供会话 ID 和工作目录以便定位。'
            : '应用会下载加密快照并生成续接说明，不会覆盖 Codex 的内部会话数据。'}
        </div>
      </div>
      <div className="detail-actions">
        <button className="primary-button wide" onClick={onContinue} disabled={busy}>
          {busy ? <Spinner /> : <MessageSquareText size={17} />}
          {conversation.continuationMode === 'native-local' ? '定位原任务' : '在此设备继续'}
        </button>
      </div>
    </aside>
  )
}

function SyncPage({ snapshot, busy, onSync, onAutoSync, onExportDiagnostics }: {
  snapshot: DashboardSnapshot
  busy: string
  onSync: () => void
  onAutoSync: (enabled: boolean) => void
  onExportDiagnostics: () => void
}) {
  const pending = snapshot.conversations.filter((item) => ['queued', 'local'].includes(item.syncStatus)).length
  const available = snapshot.conversations.filter((item) => item.syncStatus === 'available').length
  return (
    <div className="page sync-page">
      <PageHeader
        title="同步"
        subtitle="查看后台同步、离线队列和跨设备接收状态"
        actions={<button className="primary-button" onClick={onSync} disabled={busy === 'sync'}>{busy === 'sync' ? <Spinner /> : <RefreshCw size={17} />}立即同步</button>}
      />
      <div className="sync-layout">
        <section className="panel sync-hero">
          <div className={`sync-orb ${snapshot.sync.phase}`}><RefreshCw size={28} /></div>
          <div className="sync-hero-copy">
            <span>当前状态</span>
            <strong>{syncPhaseLabel(snapshot)}</strong>
            <p>{syncPhaseDescription(snapshot)}</p>
          </div>
          <div className="sync-progress" aria-label={`同步进度 ${snapshot.sync.progress}%`}>
            <i style={{ width: `${Math.max(0, Math.min(snapshot.sync.progress, 100))}%` }} />
          </div>
          <div className="sync-hero-meta">
            <span><small>上次成功</small><strong>{displayTime(snapshot.sync.lastSuccessAt)}</strong></span>
            <span><small>下次检查</small><strong>{snapshot.settings.autoSync ? displayTime(snapshot.sync.nextSyncAt, true) : '已暂停'}</strong></span>
            <span><small>离线队列</small><strong>{snapshot.sync.pendingUploads} 项</strong></span>
          </div>
        </section>
        <section className="panel sync-control">
          <PanelHeader title="自动同步" />
          <div className="sync-toggle-row">
            <div><strong>后台自动同步</strong><span>检测到 Codex 会话变化后，加密并加入上传队列</span></div>
            <Switch label="自动同步" checked={snapshot.settings.autoSync} onChange={onAutoSync} disabled={busy === 'auto-sync'} />
          </div>
          <div className="sync-rule-list">
            <SyncRule icon={<HardDrive />} title="本地变化监听" value={`${snapshot.sync.scannedConversations} 条会话`} ok />
            <SyncRule icon={<Upload />} title="等待上传" value={`${pending} 条`} ok={pending === 0} />
            <SyncRule icon={<ArrowDownToLine />} title="其他设备可继续" value={`${available} 条`} ok />
            <SyncRule icon={<ShieldCheck />} title="传输与存储" value="AES-256-GCM 密文" ok />
          </div>
        </section>
        {snapshot.sync.lastError ? (
          <section className="sync-error">
            <CircleAlert size={19} />
            <div><strong>最近一次同步失败</strong><span>{snapshot.sync.lastError}</span></div>
            <button onClick={onSync}>重试</button>
          </section>
        ) : null}
        <section className="panel activity-panel">
          <PanelHeader
            title="最近活动"
            action={(
              <button className="panel-link" onClick={onExportDiagnostics} disabled={busy === 'diagnostics'}>
                <Archive size={15} />导出诊断报告
              </button>
            )}
          />
          <ActivityList items={snapshot.activities} />
        </section>
        <section className="panel security-panel">
          <PanelHeader title="数据边界" />
          <SecurityLine icon={<ShieldCheck />} title="客户端加密" value="服务器无法读取正文" />
          <SecurityLine icon={<Cloud />} title="私有服务器" value="无需访问 OpenAI" />
          <SecurityLine icon={<FolderGit2 />} title="项目代码" value="继续通过 Git 同步" />
        </section>
      </div>
    </div>
  )
}

function SettingsPage({ settings, projects, busy, onRun, onSaved, onToast }: {
  settings: PublicSettings
  projects: SyncProjectOption[]
  busy: string
  onRun: <T>(key: string, action: () => Promise<T>, onSuccess?: (result: T) => void, refresh?: boolean) => Promise<T | undefined>
  onSaved: (settings: PublicSettings, generatedKey?: string) => void
  onToast: (toast: Toast) => void
}) {
  const [form, setForm] = useState<SaveSettingsRequest>({
    serverUrl: settings.serverUrl,
    root: settings.root,
    deviceName: settings.deviceName,
    token: '',
    encryptionKey: '',
    autoSync: settings.autoSync,
    launchAtStartup: settings.launchAtStartup,
    theme: settings.theme,
    syncDays: settings.syncDays,
    selectedProjects: settings.selectedProjects,
    includeArchived: settings.includeArchived,
    includeUnassigned: settings.includeUnassigned,
    maxBundleMiB: settings.maxBundleMiB,
  })
  const [connection, setConnection] = useState<number | null>(null)
  const [upload, setUpload] = useState<UploadTestResult | null>(null)
  const [generatedKey, setGeneratedKey] = useState('')
  const selectedProjectSet = useMemo(
    () => new Set(form.selectedProjects.map((path) => path.toLowerCase())),
    [form.selectedProjects],
  )
  const allProjectsSelected = form.selectedProjects.length === 0
  const selectedProjectCount = allProjectsSelected ? projects.length : form.selectedProjects.length
  const selectedProjectBytes = useMemo(
    () => projects
      .filter((project) => allProjectsSelected || selectedProjectSet.has(project.relativePath.toLowerCase()))
      .reduce((total, project) => total + project.totalBytes, 0),
    [allProjectsSelected, projects, selectedProjectSet],
  )

  function toggleProject(relativePath: string) {
    setForm((current) => {
      const available = projects.map((project) => project.relativePath)
      const selected = current.selectedProjects.length === 0
        ? available.filter((path) => path !== relativePath)
        : current.selectedProjects.includes(relativePath)
          ? current.selectedProjects.filter((path) => path !== relativePath)
          : [...current.selectedProjects, relativePath]
      if (selected.length === 0) return current
      return {
        ...current,
        selectedProjects: selected.length === available.length ? [] : selected,
      }
    })
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    await onRun(
      'save',
      () => desktopApi.saveSettings({
        ...form,
        token: form.token?.trim() || undefined,
        encryptionKey: form.encryptionKey?.trim() || undefined,
      }),
      (result) => {
        setConnection(result.connection.latencyMs)
        setGeneratedKey(result.generatedKey || '')
        onSaved(result.settings, result.generatedKey)
      },
    )
  }

  function testConnection() {
    onRun(
      'settings-connection',
      desktopApi.testConnection,
      (result) => {
        setConnection(result.latencyMs)
        onToast({ tone: 'success', text: `连接正常 · ${result.latencyMs} ms` })
      },
      false,
    )
  }

  function testUpload() {
    onRun(
      'settings-upload',
      desktopApi.testUpload,
      (result) => {
        setUpload(result)
        onToast({ tone: 'success', text: '加密上传测试通过，测试包已在服务端丢弃' })
      },
      false,
    )
  }

  async function changeTheme(theme: ThemeName) {
    setForm((current) => ({ ...current, theme }))
    document.documentElement.dataset.theme = theme
    await onRun('theme', () => desktopApi.setTheme(theme), undefined, false)
  }

  return (
    <div className="page settings-page">
      <PageHeader title="设置" subtitle="配置私有服务器、工作区目录和本机安全选项" />
      <form className="settings-layout" onSubmit={submit}>
        <section className="panel settings-form">
          <PanelHeader title="基础配置" />
          <div className="field-grid">
            <Field label="服务端地址" hint="两台电脑都能访问">
              <Input icon={<Server />} value={form.serverUrl} onChange={(value) => setForm({ ...form, serverUrl: value })} placeholder="https://continuity.example.com" required />
            </Field>
            <Field label="设备名称" hint="用于区分来源设备">
              <Input icon={<Monitor />} value={form.deviceName} onChange={(value) => setForm({ ...form, deviceName: value })} placeholder="公司电脑" required />
            </Field>
            <Field label="工作区根目录" hint="目录内可以包含多个项目" wide>
              <Input icon={<FolderGit2 />} value={form.root} onChange={(value) => setForm({ ...form, root: value })} placeholder="D:\\code_CPL" required />
            </Field>
            <Field label="客户端 API 令牌" hint={settings.hasToken ? '已安全保存；留空保持不变' : '从服务端管理页面创建'}>
              <Input icon={<KeyRound />} type="password" value={form.token || ''} onChange={(value) => setForm({ ...form, token: value })} placeholder={settings.hasToken ? '•••••••• 已保存' : 'ct_...'} />
            </Field>
            <Field label="共享加密密钥" hint={settings.hasEncryptionKey ? '已安全保存；两台电脑必须一致' : '留空自动生成'}>
              <Input icon={<ShieldCheck />} type="password" value={form.encryptionKey || ''} onChange={(value) => setForm({ ...form, encryptionKey: value })} placeholder={settings.hasEncryptionKey ? '•••••••• 已保存' : '留空自动生成'} />
            </Field>
          </div>
          {generatedKey ? (
            <div className="generated-key">
              <div><strong>新密钥仅显示这一次</strong><span>请复制到另一台电脑后再关闭。</span></div>
              <code>{generatedKey}</code>
              <CopyButton text={generatedKey} />
            </div>
          ) : null}
          <section id="sync-scope-settings" className="sync-scope-settings" aria-labelledby="sync-scope-title">
            <div className="sync-scope-heading">
              <div>
                <strong id="sync-scope-title">同步范围</strong>
                <span>仅打包符合条件的 Codex 会话；代码仍由 Git 同步</span>
              </div>
              <span className="scope-summary">{selectedProjectCount || '全部'} 个项目 · {humanBytes(selectedProjectBytes)}</span>
            </div>
            <div className="scope-block">
              <div className="scope-label">
                <strong>最近更新时间</strong>
                <span>首次同步建议一周，后续自动同步只处理变化</span>
              </div>
              <div className="segmented-control" role="radiogroup" aria-label="同步时间范围">
                {([
                  [2, '2 天'],
                  [5, '5 天'],
                  [7, '一周'],
                  [0, '不限制'],
                ] as const).map(([days, label]) => (
                  <button
                    key={days}
                    type="button"
                    role="radio"
                    aria-checked={form.syncDays === days}
                    className={form.syncDays === days ? 'selected' : ''}
                    onClick={() => setForm((current) => ({ ...current, syncDays: days }))}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>
            <div className="scope-block project-scope-block">
              <div className="scope-label">
                <strong>同步项目</strong>
                <span>默认全部；取消勾选可缩小上传范围</span>
              </div>
              <div className="project-selector-toolbar">
                <span>{allProjectsSelected ? '全部项目已选择' : `已选择 ${selectedProjectCount}/${projects.length}`}</span>
                <button type="button" onClick={() => setForm((current) => ({ ...current, selectedProjects: [] }))}>选择全部</button>
              </div>
              <div className="project-selector" aria-label="同步项目">
                {projects.length ? projects.map((project) => {
                  const checked = allProjectsSelected || selectedProjectSet.has(project.relativePath.toLowerCase())
                  return (
                    <label key={project.relativePath} className={checked ? 'selected' : ''}>
                      <input type="checkbox" checked={checked} onChange={() => toggleProject(project.relativePath)} />
                      <span className="project-check"><Check size={13} /></span>
                      <span className="project-option-main">
                        <strong>{project.name}</strong>
                        <small>{project.conversationCount} 条会话 · {humanBytes(project.totalBytes)}</small>
                      </span>
                      <code>{project.relativePath}</code>
                    </label>
                  )
                }) : (
                  <div className="project-selector-empty">当前工作区根目录下暂未发现 Codex 会话</div>
                )}
              </div>
            </div>
            <div className="scope-guardrails">
              <SettingToggle
                title="包含已归档会话"
                note="默认关闭；开启后也会扫描 ~/.codex/archived_sessions"
                checked={form.includeArchived}
                onChange={(value) => setForm((current) => ({ ...current, includeArchived: value }))}
              />
              <SettingToggle
                title="同步无项目会话"
                note="默认关闭；开启后会同步工作区根目录之外的会话，但不会上传原始绝对路径"
                checked={form.includeUnassigned}
                onChange={(value) => setForm((current) => ({ ...current, includeUnassigned: value }))}
              />
              <div className="bundle-limit">
                <ShieldCheck size={20} />
                <div><strong>单包硬上限</strong><span>超过时不会上传，会提示缩短时间或减少项目</span></div>
                <b>{form.maxBundleMiB} MiB</b>
              </div>
            </div>
          </section>
          <div className="settings-options">
            <SettingToggle
              title="开机自动启动"
              note="登录 Windows 后在系统托盘运行"
              checked={form.launchAtStartup}
              onChange={(value) => setForm({ ...form, launchAtStartup: value })}
            />
            <SettingToggle
              title="自动同步"
              note="检测会话变化并安全加入后台同步队列"
              checked={form.autoSync}
              onChange={(value) => setForm({ ...form, autoSync: value })}
            />
          </div>
          <div className="form-actions">
            <button className="primary-button" type="submit" disabled={busy === 'save'}>
              {busy === 'save' ? <Spinner /> : <Check size={17} />}保存配置
            </button>
          </div>
        </section>
        <aside className="settings-side">
          <section className="panel">
            <PanelHeader title="主题" />
            <div className="theme-options">
              {themes.map((theme) => (
                <button
                  key={theme.id}
                  type="button"
                  className={form.theme === theme.id ? 'selected' : ''}
                  onClick={() => changeTheme(theme.id)}
                >
                  <i style={{ background: theme.color }} /><span>{theme.label}</span>{form.theme === theme.id ? <Check size={15} /> : null}
                </button>
              ))}
            </div>
          </section>
          <section className="panel validation-panel">
            <PanelHeader title="配置验证" />
            <ValidationStep number={1} title="设备配置" state={settings.hasToken ? '已保存' : '待完成'} done={settings.hasToken} />
            <ValidationStep
              number={2}
              title="连接测试"
              state={connection !== null ? `${connection} ms` : '保存配置后测试'}
              done={connection !== null}
              action={<button type="button" onClick={testConnection} disabled={busy === 'settings-connection'}>{busy === 'settings-connection' ? <Spinner /> : '测试'}</button>}
            />
            <ValidationStep
              number={3}
              title="加密上传"
              state={upload ? `${humanBytes(upload.serverReceivedBytes)} · 已丢弃` : '尚未执行'}
              done={Boolean(upload)}
              action={<button type="button" onClick={testUpload} disabled={busy === 'settings-upload'}>{busy === 'settings-upload' ? <Spinner /> : '测试'}</button>}
            />
          </section>
          <section className="panel version-panel">
            <SecurityLine icon={<Activity />} title="客户端版本" value={`v${settings.version}`} />
            <SecurityLine icon={<KeyRound />} title="凭据存储" value="Windows 凭据管理器" />
          </section>
        </aside>
      </form>
    </div>
  )
}

function ContinuationDialog({ result, onClose }: { result: ContinueResult; onClose: () => void }) {
  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => {
      if (event.target === event.currentTarget) onClose()
    }}>
      <section className="modal" role="dialog" aria-modal="true" aria-labelledby="continuation-title">
        <div className="modal-header">
          <span className="success-mark"><Check size={20} /></span>
          <div>
            <h2 id="continuation-title">{result.mode === 'native-local' ? '已定位本机会话' : '已准备在此设备继续'}</h2>
            <p>{result.message}</p>
          </div>
          <button className="icon-button" onClick={onClose} aria-label="关闭"><X size={19} /></button>
        </div>
        <div className="modal-body">
          <DetailLine icon={<FolderGit2 />} label="工作目录" value={result.workspacePath} code />
          <DetailLine icon={<MessageSquareText />} label="会话 ID" value={result.sessionId} code />
          {result.handoffPath ? <DetailLine icon={<FileClock />} label="续接说明" value={result.handoffPath} code /> : null}
          {result.mode === 'native-local' ? (
            <div className="modal-note">该会话已经存在于本机。请在 Codex 历史中搜索标题或会话 ID，然后从原对话继续。</div>
          ) : (
            <>
              <div className="prompt-box"><span>建议续接提示词</span><p>{result.prompt}</p></div>
              {result.prompt ? <CopyButton text={result.prompt} label="复制提示词" /> : null}
            </>
          )}
        </div>
        <div className="modal-footer"><button className="primary-button" onClick={onClose}>知道了</button></div>
      </section>
    </div>
  )
}

function TrayPanel() {
  const [snapshot, setSnapshot] = useState<DashboardSnapshot | null>(null)
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')
  const reload = useCallback(() => desktopApi.dashboard().then(setSnapshot).catch((error) => setMessage(errorMessage(error))), [])

  useEffect(() => {
    reload()
    const interval = window.setInterval(reload, 15_000)
    return () => window.clearInterval(interval)
  }, [reload])

  async function action(key: string, handler: () => Promise<unknown>, done: string) {
    setBusy(key)
    setMessage('')
    try {
      await handler()
      setMessage(done)
      await reload()
    } catch (error) {
      setMessage(errorMessage(error))
    } finally {
      setBusy('')
    }
  }

  async function openConversation(conversation: Conversation) {
    window.localStorage.setItem(selectedConversationStorageKey, conversation.id)
    await desktopApi.showMain('conversations')
    await desktopApi.hideTray()
  }

  if (!snapshot) return <div className="tray-shell tray-loading"><Spinner />正在读取同步状态…</div>
  const latest = snapshot.conversations.find((item) => item.syncStatus === 'available') || snapshot.conversations[0]
  return (
    <div className="tray-shell">
      <div className="tray-header">
        <BrandMark />
        <div><strong>Codex Continuity</strong><small>{snapshot.connection ? `服务器 · ${snapshot.connection.latencyMs} ms` : '服务器未连接'}</small></div>
        <span className={snapshot.connection ? 'running' : 'stopped'}><i />{snapshot.connection ? '运行中' : '离线'}</span>
      </div>
      <TraySection label="快捷操作">
        <TrayRow icon={<RefreshCw />} title="立即同步" detail={snapshot.sync.pendingUploads ? `${snapshot.sync.pendingUploads} 项等待上传` : '全部为最新'} hint="Ctrl+Alt+P" busy={busy === 'sync'} onClick={() => action('sync', desktopApi.syncNow, '同步完成')} />
        <TrayRow icon={<Play />} title="在此设备继续" detail={latest?.projectName || '暂无会话'} chevron onClick={() => latest && openConversation(latest)} />
      </TraySection>
      <TraySection label="最近会话">
        {snapshot.conversations.slice(0, 3).map((conversation, index) => (
          <TrayRow
            key={conversation.id}
            icon={<MessageSquareText />}
            title={conversation.title}
            detail={`${conversation.projectName} · ${statusLabel(conversation.syncStatus)}`}
            warning={['queued', 'conflict'].includes(conversation.syncStatus)}
            selected={index === 0}
            onClick={() => openConversation(conversation)}
          />
        ))}
        <TrayRow icon={<History />} title="查看全部会话" chevron onClick={() => desktopApi.showMain('conversations')} />
      </TraySection>
      <TraySection label="同步">
        <TrayRow icon={<RefreshCw />} title="自动同步" suffix={<Switch label="自动同步" checked={snapshot.settings.autoSync} onChange={(value) => action('auto', () => desktopApi.setAutoSync(value), value ? '自动同步已开启' : '自动同步已暂停')} />} />
        <TrayRow icon={<Clock3 />} title="最近活动" detail={displayTime(snapshot.sync.lastSuccessAt)} chevron onClick={() => desktopApi.showMain('sync')} />
      </TraySection>
      {message ? <div className="tray-message"><Check size={15} />{message}</div> : null}
      <div className="tray-bottom">
        <TrayRow icon={<Monitor />} title="打开主窗口" onClick={() => desktopApi.showMain('conversations')} />
        <TrayRow icon={<Settings />} title="设置" onClick={() => desktopApi.showMain('settings')} />
      </div>
      <div className="tray-exit"><TrayRow icon={<LogOut />} title="退出 Codex Continuity" onClick={() => desktopApi.quit()} /></div>
    </div>
  )
}

function TraySection({ label, children }: { label: string; children: ReactNode }) {
  return <section className="tray-section"><span className="tray-label">{label}</span>{children}</section>
}

function TrayRow({ icon, title, detail, hint, chevron, warning, selected, suffix, busy, onClick }: {
  icon: ReactNode
  title: string
  detail?: string
  hint?: string
  chevron?: boolean
  warning?: boolean
  selected?: boolean
  suffix?: ReactNode
  busy?: boolean
  onClick?: () => void
}) {
  const content = (
    <>
      <span className="tray-row-icon">{busy ? <Spinner /> : icon}</span>
      <strong>{title}</strong>
      <span className={`tray-row-detail ${warning ? 'warning' : ''}`}>{detail}{hint ? <small>{hint}</small> : null}</span>
      {suffix}{chevron ? <ChevronRight size={19} /> : null}
    </>
  )
  if (!onClick) {
    return <div className={`tray-row tray-row-static ${selected ? 'selected' : ''}`}>{content}</div>
  }
  return <button className={`tray-row ${selected ? 'selected' : ''}`} onClick={onClick} disabled={busy}>{content}</button>
}

function PageHeader({ title, subtitle, actions }: { title: string; subtitle: ReactNode; actions?: ReactNode }) {
  return <div className="page-header"><div><h1>{title}</h1><div className="page-subtitle">{subtitle}</div></div>{actions ? <div className="page-actions">{actions}</div> : null}</div>
}

function PanelHeader({ title, action }: { title: string; action?: ReactNode }) {
  return <div className="panel-header"><h2>{title}</h2>{action}</div>
}

function StatusBadge({ status }: { status: ConversationSyncStatus }) {
  const icon = status === 'synced' ? <CheckCircle2 /> : status === 'available' ? <Download /> : status === 'conflict' ? <CircleAlert /> : <RefreshCw />
  return <span className={`status-badge ${status}`}>{icon}{statusLabel(status)}</span>
}

function DetailLine({ icon, label, value, code }: { icon: ReactNode; label: string; value: string; code?: boolean }) {
  return <div className="detail-line"><span className="detail-icon">{icon}</span><div><small>{label}</small>{code ? <code>{value}</code> : <strong>{value}</strong>}</div></div>
}

function ActivityList({ items }: { items: DashboardSnapshot['activities'] }) {
  const icons: Record<string, ReactNode> = {
    sync: <RefreshCw />,
    device: <Monitor />,
    archive: <Archive />,
    connection: <Cloud />,
    error: <CircleAlert />,
  }
  return (
    <div className="activity-list">
      {items.length ? items.map((item) => (
        <div className="activity-row" key={item.id}>
          <span className={`activity-icon ${item.tone}`}>{icons[item.kind] || <Activity />}</span>
          <span><strong>{item.title}</strong><small>{item.detail}</small></span>
          <time>{displayTime(item.time)}</time>
        </div>
      )) : <div className="empty-inline">暂无同步活动</div>}
    </div>
  )
}

function SyncRule({ icon, title, value, ok }: { icon: ReactNode; title: string; value: string; ok: boolean }) {
  return <div className="sync-rule"><span>{icon}</span><div><strong>{title}</strong><small>{value}</small></div>{ok ? <CheckCircle2 className="ok" /> : <Clock3 className="waiting" />}</div>
}

function SecurityLine({ icon, title, value }: { icon: ReactNode; title: string; value: string }) {
  return <div className="security-line"><span>{icon}{title}</span><strong>{value}</strong></div>
}

function SettingToggle({ title, note, checked, onChange }: { title: string; note: string; checked: boolean; onChange: (value: boolean) => void }) {
  return <div className="setting-toggle"><div><strong>{title}</strong><small>{note}</small></div><Switch label={title} checked={checked} onChange={onChange} /></div>
}

function Switch({ label, checked, onChange, disabled }: { label: string; checked: boolean; onChange: (value: boolean) => void; disabled?: boolean }) {
  return <button type="button" role="switch" aria-checked={checked} aria-label={label} className={`switch ${checked ? 'on' : ''}`} onClick={(event) => { event.stopPropagation(); onChange(!checked) }} disabled={disabled}><i /></button>
}

function Field({ label, hint, children, wide }: { label: string; hint: string; children: ReactNode; wide?: boolean }) {
  return <label className={`field ${wide ? 'wide' : ''}`}><span><strong>{label}</strong><small>{hint}</small></span>{children}</label>
}

function Input({ icon, value, onChange, type = 'text', placeholder, required }: {
  icon: ReactNode
  value: string
  onChange: (value: string) => void
  type?: string
  placeholder?: string
  required?: boolean
}) {
  return <span className="input-with-icon">{icon}<input type={type} value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} required={required} /></span>
}

function ValidationStep({ number, title, state, done, action }: { number: number; title: string; state: string; done: boolean; action?: ReactNode }) {
  return <div className="validation-step"><b>{done ? <Check size={15} /> : number}</b><span><strong>{title}</strong><small>{state}</small></span>{action}</div>
}

function CopyButton({ text, label = '复制' }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      type="button"
      className="copy-button"
      onClick={async () => {
        await navigator.clipboard.writeText(text)
        setCopied(true)
        window.setTimeout(() => setCopied(false), 1600)
      }}
    >
      {copied ? <Check size={15} /> : <Copy size={15} />}{copied ? '已复制' : label}
    </button>
  )
}

function ToastView({ toast, onClose }: { toast: NonNullable<Toast>; onClose: () => void }) {
  return (
    <div className={`toast ${toast.tone}`} role="status">
      <span>{toast.tone === 'success' ? <Check /> : toast.tone === 'error' ? <X /> : <Activity />}</span>
      <strong>{toast.text}</strong>
      <button onClick={onClose} aria-label="关闭提示"><X size={16} /></button>
    </div>
  )
}

function BrandMark() {
  return <span className="brand-mark"><Link2 size={24} strokeWidth={2.35} /></span>
}

function Spinner() {
  return <span className="spinner small" />
}

function statusLabel(status: ConversationSyncStatus) {
  return {
    synced: '全部已同步',
    local: '仅本机',
    available: '可继续',
    queued: '等待上传',
    conflict: '需要处理',
    imported: '已导入',
  }[status]
}

function syncPhaseLabel(snapshot: DashboardSnapshot) {
  if (!snapshot.settings.autoSync || snapshot.sync.phase === 'paused') return '自动同步已暂停'
  if (!snapshot.connection) return snapshot.sync.pendingUploads ? '离线，等待恢复网络' : '服务器未连接'
  return {
    idle: snapshot.sync.pendingUploads ? '正在等待下一次同步' : '全部会话均为最新',
    scanning: '正在检查 Codex 会话',
    uploading: '正在上传加密快照',
    downloading: '正在接收其他设备的会话',
    queued: '已加入离线队列',
    error: '同步需要处理',
    paused: '自动同步已暂停',
  }[snapshot.sync.phase]
}

function syncPhaseDescription(snapshot: DashboardSnapshot) {
  if (snapshot.sync.lastError) return snapshot.sync.lastError
  if (!snapshot.settings.autoSync) return '手动点击“立即同步”仍可同步当前会话。'
  if (!snapshot.connection) return '本机变化会保留在离线队列，网络恢复后自动重试。'
  if (snapshot.sync.pendingUploads) return `${snapshot.sync.pendingUploads} 项已进入持久化队列，不会静默丢失。`
  return '后台监听正常；只在会话发生变化时上传新的加密快照。'
}

function humanBytes(size: number) {
  if (!size || size < 1024) return `${size || 0} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = size
  let index = -1
  do { value /= 1024; index += 1 } while (value >= 1024 && index < units.length - 1)
  return `${value.toFixed(value >= 100 ? 0 : 1)} ${units[index]}`
}

function deviceOSLabel(conversation: Conversation) {
  return conversation.sourceDeviceOS?.trim() || '系统未知'
}

function joinWindowsPath(root: string, relativePath: string) {
  const cleanRoot = root.replace(/[\\/]+$/, '')
  const cleanRelative = relativePath.replace(/^[\\/]+/, '').replace(/\//g, '\\')
  return cleanRelative && cleanRelative !== '.' ? `${cleanRoot}\\${cleanRelative}` : cleanRoot
}

function displayTime(value?: string, future = false) {
  if (!value) return '尚未同步'
  const stamp = new Date(value).getTime()
  if (!Number.isFinite(stamp)) return value
  const delta = future ? stamp - Date.now() : Date.now() - stamp
  if (delta < 0 && !future) return '刚刚'
  if (delta < 60_000) return future ? '不到 1 分钟' : '刚刚'
  if (delta < 3_600_000) return `${Math.max(1, Math.floor(delta / 60_000))} 分钟${future ? '后' : '前'}`
  if (delta < 86_400_000) return `${Math.floor(delta / 3_600_000)} 小时${future ? '后' : '前'}`
  return formatAbsoluteTime(value)
}

function formatAbsoluteTime(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : typeof error === 'string' ? error : '操作失败，请稍后重试'
}

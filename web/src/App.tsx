import { FormEvent, ReactNode, useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity,
  Check,
  ChevronDown,
  ChevronRight,
  CircleUserRound,
  Clock3,
  Code2,
  Copy,
  Database,
  Download,
  HardDrive,
  Home,
  KeyRound,
  Laptop,
  Link2,
  LogOut,
  Menu,
  Monitor,
  Palette,
  Plus,
  RefreshCw,
  Rocket,
  Send,
  Server,
  Settings,
  ShieldCheck,
  Smartphone,
  TerminalSquare,
  Trash2,
  Users,
  X,
} from 'lucide-react'
import { api } from './api'
import type {
  ApiToken,
  Device,
  Handoff,
  Overview,
  PageName,
  ThemeName,
  User,
} from './types'

const navGroups: Array<{
  label: string
  items: Array<{ id: PageName; label: string; icon: typeof Home }>
}> = [
  {
    label: '工作台',
    items: [
      { id: 'overview', label: '总览', icon: Home },
      { id: 'devices', label: '设备', icon: Monitor },
      { id: 'handoffs', label: '会话快照', icon: Clock3 },
    ],
  },
  {
    label: '管理',
    items: [
      { id: 'users', label: '用户', icon: Users },
      { id: 'tokens', label: 'API 令牌', icon: KeyRound },
      { id: 'downloads', label: '桌面客户端', icon: Download },
    ],
  },
  {
    label: '系统',
    items: [{ id: 'settings', label: '设置', icon: Settings }],
  },
]

const themeOptions: Array<{ id: ThemeName; label: string }> = [
  { id: 'blue', label: '蓝色' },
  { id: 'teal', label: '青绿' },
  { id: 'violet', label: '紫色' },
]

export default function App() {
  const [user, setUser] = useState<User | null>(null)
  const [checking, setChecking] = useState(true)
  const [page, setPage] = useState<PageName>('overview')
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [theme, setTheme] = useState<ThemeName>(() => {
    const saved = localStorage.getItem('continuity-theme')
    return saved === 'teal' || saved === 'violet' ? saved : 'blue'
  })

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('continuity-theme', theme)
  }, [theme])

  useEffect(() => {
    api
      .me()
      .then(({ user: current }) => setUser(current))
      .catch(() => setUser(null))
      .finally(() => setChecking(false))
  }, [])

  if (checking) return <AppLoading />
  if (!user) return <Login theme={theme} setTheme={setTheme} onLogin={setUser} />

  return (
    <div className="app-shell">
      <Header
        user={user}
        theme={theme}
        setTheme={setTheme}
        onMenu={() => setSidebarOpen((value) => !value)}
        onLogout={async () => {
          await api.logout().catch(() => undefined)
          setUser(null)
        }}
      />
      <Sidebar
        user={user}
        page={page}
        open={sidebarOpen}
        onSelect={(next) => {
          setPage(next)
          setSidebarOpen(false)
        }}
      />
      {sidebarOpen && <button className="sidebar-scrim" aria-label="关闭菜单" onClick={() => setSidebarOpen(false)} />}
      <main className="main-content">
        {page === 'overview' && <OverviewPage onNavigate={setPage} />}
        {page === 'devices' && <DevicesPage />}
        {page === 'handoffs' && <HandoffsPage />}
        {page === 'users' && <UsersPage currentUser={user} />}
        {page === 'tokens' && <TokensPage />}
        {page === 'downloads' && <DownloadsPage />}
        {page === 'settings' && <SettingsPage theme={theme} setTheme={setTheme} />}
      </main>
    </div>
  )
}

function AppLoading() {
  return (
    <div className="app-loading">
      <BrandMark />
      <span className="spinner" />
      <p>正在连接 Codex Continuity 服务…</p>
    </div>
  )
}

function Login({
  theme,
  setTheme,
  onLogin,
}: {
  theme: ThemeName
  setTheme: (theme: ThemeName) => void
  onLogin: (user: User) => void
}) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const result = await api.login(email, password)
      onLogin(result.user)
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '登录失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-page">
      <section className="login-brand-panel">
        <div className="login-brand-top">
          <BrandMark inverse />
          <span>Codex Continuity</span>
        </div>
        <div className="login-brand-copy">
          <span className="eyebrow">CODEX CONTINUITY</span>
          <h1>让 Codex 会话在不同设备之间，安全延续。</h1>
          <p>自动同步工作根目录关联的 Codex 会话快照。服务端只保存密文，不需要访问 Codex 或 OpenAI。</p>
          <div className="login-feature-list">
            <Feature icon={<ShieldCheck />} title="端到端加密" text="AES-256-GCM 加密后再上传" />
            <Feature icon={<RefreshCw />} title="自动同步与离线队列" text="无需逐项目、逐对话手动发布" />
            <Feature icon={<Server />} title="团队设备管理" text="Docker Compose 私有部署" />
          </div>
        </div>
        <div className="login-brand-footer">
          <span className="status-dot" />
          数据始终由你掌控
        </div>
      </section>
      <section className="login-form-panel">
        <div className="login-theme-switch" aria-label="主题色">
          {themeOptions.map((option) => (
            <button
              key={option.id}
              className={`theme-dot theme-${option.id} ${theme === option.id ? 'active' : ''}`}
              onClick={() => setTheme(option.id)}
              title={option.label}
              aria-label={option.label}
            />
          ))}
        </div>
        <form className="login-form" onSubmit={submit}>
          <div className="login-mobile-brand">
            <BrandMark />
            <span>Codex Continuity</span>
          </div>
          <span className="form-kicker">欢迎回来</span>
          <h2>登录管理空间</h2>
          <p className="form-intro">查看设备状态、会话快照与团队成员。</p>
          <label>
            <span>邮箱</span>
            <input
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="name@company.com"
              autoComplete="username"
              required
            />
          </label>
          <label>
            <span>密码</span>
            <input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="输入登录密码"
              autoComplete="current-password"
              required
            />
          </label>
          {error && <div className="form-error">{error}</div>}
          <button className="primary-button login-submit" type="submit" disabled={busy}>
            {busy ? <span className="button-spinner" /> : '登录'}
            {!busy && <ChevronRight size={18} />}
          </button>
          <div className="login-help">
            首次部署的管理员账号由服务端环境变量设置。请登录后立即创建个人令牌。
          </div>
        </form>
      </section>
    </div>
  )
}

function Feature({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return (
    <div className="login-feature">
      <span>{icon}</span>
      <div>
        <strong>{title}</strong>
        <small>{text}</small>
      </div>
    </div>
  )
}

function Header({
  user,
  theme,
  setTheme,
  onMenu,
  onLogout,
}: {
  user: User
  theme: ThemeName
  setTheme: (theme: ThemeName) => void
  onMenu: () => void
  onLogout: () => void
}) {
  const [accountOpen, setAccountOpen] = useState(false)
  return (
    <header className="top-header">
      <div className="header-brand">
        <button className="mobile-menu-button" onClick={onMenu} aria-label="打开菜单">
          <Menu size={21} />
        </button>
        <BrandMark />
        <span>Codex Continuity</span>
      </div>
      <div className="workspace-selector">个人空间</div>
      <div className="header-actions">
        <div className="theme-picker">
          <span>主题</span>
          {themeOptions.map((option) => (
            <button
              key={option.id}
              className={`theme-choice ${theme === option.id ? 'active' : ''}`}
              onClick={() => setTheme(option.id)}
            >
              <i className={`theme-dot theme-${option.id}`} />
              {option.label}
              {theme === option.id && <Check size={13} />}
            </button>
          ))}
        </div>
        <div className="account-menu">
          <button className="account-trigger" onClick={() => setAccountOpen((value) => !value)}>
            <span className="avatar">{user.displayName.slice(0, 1)}</span>
            <span className="account-copy">
              <strong>{user.displayName}</strong>
              <small>{user.role === 'admin' ? '管理员' : '成员'}</small>
            </span>
            <ChevronDown size={15} />
          </button>
          {accountOpen && (
            <div className="account-popover">
              <div>
                <strong>{user.displayName}</strong>
                <small>{user.email}</small>
              </div>
              <button onClick={onLogout}>
                <LogOut size={16} /> 退出登录
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  )
}

function Sidebar({
  user,
  page,
  open,
  onSelect,
}: {
  user: User
  page: PageName
  open: boolean
  onSelect: (page: PageName) => void
}) {
  return (
    <aside className={`sidebar ${open ? 'open' : ''}`}>
      <nav>
        {navGroups.map((group) => (
          <div className="nav-group" key={group.label}>
            <div className="nav-group-title">
              <span>{group.label}</span>
              <i />
            </div>
            {group.items.map((item) => {
              if (item.id === 'users' && user.role !== 'admin') return null
              const Icon = item.icon
              return (
                <button
                  key={item.id}
                  className={`nav-item ${page === item.id ? 'active' : ''}`}
                  onClick={() => onSelect(item.id)}
                >
                  <Icon size={19} strokeWidth={1.8} />
                  <span>{item.label}</span>
                </button>
              )
            })}
          </div>
        ))}
      </nav>
      <div className="sidebar-footer">
        <div className="server-status">
          <span className="status-dot" />
          <div>
            <strong>服务器运行正常</strong>
            <small>本机加密 · 密文存储</small>
          </div>
          <ChevronRight size={16} />
        </div>
        <div className="version-row">
          <span>版本</span>
          <span>v0.3.1</span>
        </div>
      </div>
    </aside>
  )
}

function OverviewPage({ onNavigate }: { onNavigate: (page: PageName) => void }) {
  const [data, setData] = useState<Overview | null>(null)
  const [error, setError] = useState('')
  const load = useCallback(() => {
    setError('')
    api
      .overview()
      .then(setData)
      .catch((requestError) => setError(requestError.message))
  }, [])
  useEffect(load, [load])

  return (
    <>
      <PageHeader
        eyebrow="工作台"
        title="同步总览"
        description="跨设备安全延续 Codex 工作上下文"
        action={
          <button className="primary-button" onClick={() => onNavigate('downloads')}>
            <Download size={17} /> 下载桌面客户端
          </button>
        }
      />
      {error && <ErrorBanner message={error} retry={load} />}
      <div className="metric-grid">
        <MetricCard
          icon={<Monitor />}
          tone="blue"
          label="在线设备"
          value={data?.onlineDevices ?? '—'}
          footnote="5 分钟内有心跳"
        />
        <MetricCard
          icon={<Send />}
          tone="amber"
          label="待接收快照"
          value={data?.pendingHandoffs ?? '—'}
          footnote="等待另一台设备"
        />
        <MetricCard
          icon={<Activity />}
          tone="green"
          label="本月同步"
          value={data?.monthlyHandoffs ?? '—'}
          footnote="按自然月统计"
        />
        <MetricCard
          icon={<Database />}
          tone="violet"
          label="存储占用"
          value={data ? humanBytes(data.storageBytes) : '—'}
          footnote="仅加密会话快照"
        />
      </div>
      <div className="encryption-banner">
        <ShieldCheck size={19} />
        <span>所有会话快照在上传前已在本机完成加密，服务端只保存密文。</span>
        <button onClick={() => onNavigate('settings')}>了解安全设计 <ChevronRight size={15} /></button>
      </div>
      <div className="dashboard-grid">
        <Panel
          className="handoff-panel"
          title="最近会话快照"
          action={<button className="text-button" onClick={() => onNavigate('handoffs')}>查看全部 <ChevronRight size={15} /></button>}
        >
          <HandoffTable handoffs={data?.recentHandoffs ?? []} loading={!data && !error} />
        </Panel>
        <div className="right-column">
          <Panel
            title="设备状态"
            action={<button className="text-button" onClick={() => onNavigate('devices')}>查看全部设备 <ChevronRight size={15} /></button>}
          >
            <div className="device-list-compact">
              {(data?.devices ?? []).slice(0, 3).map((device) => (
                <DeviceCompact key={device.id} device={device} />
              ))}
              {data && data.devices.length === 0 && <EmptyState compact text="尚未注册客户端设备" />}
              {!data && !error && <SkeletonRows count={2} />}
            </div>
          </Panel>
          <Panel title="快捷操作">
            <QuickAction icon={<KeyRound />} title="生成客户端令牌" text="为新电脑创建独立凭据" onClick={() => onNavigate('tokens')} />
            <QuickAction icon={<Download />} title="下载 Windows 客户端" text="NSIS / MSI，无需命令行配置" onClick={() => onNavigate('downloads')} />
            <QuickAction icon={<Users />} title="邀请团队成员" text="每位成员的数据相互隔离" onClick={() => onNavigate('users')} />
          </Panel>
        </div>
      </div>
    </>
  )
}

function DevicesPage() {
  const [items, setItems] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    api.devices().then((result) => setItems(result.devices)).finally(() => setLoading(false))
  }, [])
  return (
    <>
      <PageHeader eyebrow="工作台" title="设备" description="管理已授权的工作电脑与客户端状态" />
      <Panel title={`全部设备 ${items.length ? `(${items.length})` : ''}`}>
        {loading ? <SkeletonRows count={4} /> : items.length ? (
          <div className="card-list">
            {items.map((device) => (
              <div className="device-card" key={device.id}>
                <span className="device-icon"><Monitor /></span>
                <div className="device-main">
                  <strong>{device.name}</strong>
                  <span>{device.hostname} · {device.os}</span>
                </div>
                <StatusBadge online={isOnline(device.lastSeenAt)} />
                <div className="device-meta">
                  <small>客户端</small><span>v{device.clientVersion}</span>
                </div>
                <div className="device-meta">
                  <small>最后活动</small><span>{relativeTime(device.lastSeenAt)}</span>
                </div>
              </div>
            ))}
          </div>
        ) : <EmptyState icon={<Monitor />} text="尚未注册设备，请先下载客户端并执行 init。" />}
      </Panel>
    </>
  )
}

function HandoffsPage() {
  const [items, setItems] = useState<Handoff[]>([])
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    api.handoffs().then((result) => setItems(result.handoffs)).finally(() => setLoading(false))
  }, [])
  return (
    <>
      <PageHeader eyebrow="工作台" title="会话快照" description="查看跨设备加密快照的同步与接收状态" />
      <Panel title={`全部会话快照 ${items.length ? `(${items.length})` : ''}`}>
        <HandoffTable handoffs={items} loading={loading} detailed />
      </Panel>
    </>
  )
}

function UsersPage({ currentUser }: { currentUser: User }) {
  const [items, setItems] = useState<User[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [error, setError] = useState('')
  const load = useCallback(() => api.users().then((result) => setItems(result.users)).catch((err) => setError(err.message)), [])
  useEffect(() => { load() }, [load])
  return (
    <>
      <PageHeader
        eyebrow="管理"
        title="用户"
        description="为同事创建独立账号；设备、令牌和会话快照数据按用户隔离"
        action={<button className="primary-button" onClick={() => setDialogOpen(true)}><Plus size={17} /> 新建用户</button>}
      />
      {error && <ErrorBanner message={error} retry={load} />}
      <Panel title={`团队成员 (${items.length})`}>
        <div className="data-table-wrap">
          <table className="data-table">
            <thead><tr><th>成员</th><th>角色</th><th>创建时间</th><th>状态</th></tr></thead>
            <tbody>
              {items.map((member) => (
                <tr key={member.id}>
                  <td><div className="person-cell"><span className="avatar small">{member.displayName.slice(0, 1)}</span><div><strong>{member.displayName}{member.id === currentUser.id ? '（我）' : ''}</strong><small>{member.email}</small></div></div></td>
                  <td>{member.role === 'admin' ? '管理员' : '成员'}</td>
                  <td>{formatDate(member.createdAt)}</td>
                  <td><span className="status-badge success"><i /> 正常</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Panel>
      {dialogOpen && <CreateUserModal onClose={() => setDialogOpen(false)} onCreated={() => { setDialogOpen(false); load() }} />}
    </>
  )
}

function CreateUserModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [form, setForm] = useState({ email: '', displayName: '', password: '', role: 'member' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.createUser(form)
      onCreated()
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建失败')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title="新建用户" onClose={onClose}>
      <form className="stacked-form" onSubmit={submit}>
        <label><span>姓名</span><input value={form.displayName} onChange={(e) => setForm({ ...form, displayName: e.target.value })} required /></label>
        <label><span>邮箱</span><input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} required /></label>
        <label><span>初始密码</span><input type="password" minLength={10} value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required /></label>
        <label><span>角色</span><select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}><option value="member">成员</option><option value="admin">管理员</option></select></label>
        {error && <div className="form-error">{error}</div>}
        <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>取消</button><button className="primary-button" disabled={busy}>{busy ? '创建中…' : '创建用户'}</button></div>
      </form>
    </Modal>
  )
}

function TokensPage() {
  const [items, setItems] = useState<ApiToken[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [secret, setSecret] = useState('')
  const load = useCallback(() => api.tokens().then((result) => setItems(result.tokens)), [])
  useEffect(() => { load() }, [load])
  return (
    <>
      <PageHeader
        eyebrow="管理"
        title="API 令牌"
        description="每台电脑使用独立令牌；令牌只能在创建时查看一次"
        action={<button className="primary-button" onClick={() => setDialogOpen(true)}><Plus size={17} /> 创建令牌</button>}
      />
      <Panel title={`客户端令牌 (${items.length})`}>
        {items.length ? (
          <div className="card-list">
            {items.map((token) => (
              <div className="token-card" key={token.id}>
                <span className="token-icon"><KeyRound /></span>
                <div><strong>{token.name}</strong><small>{token.prefix}••••••••</small></div>
                <div className="token-date"><small>最近使用</small><span>{token.lastUsedAt ? relativeTime(token.lastUsedAt) : '从未使用'}</span></div>
                <div className="token-date"><small>创建时间</small><span>{formatDate(token.createdAt)}</span></div>
                <button className="icon-button danger" title="撤销令牌" onClick={async () => { if (confirm(`确认撤销“${token.name}”？`)) { await api.deleteToken(token.id); load() } }}><Trash2 size={17} /></button>
              </div>
            ))}
          </div>
        ) : <EmptyState icon={<KeyRound />} text="还没有客户端令牌，请为第一台电脑创建令牌。" />}
      </Panel>
      {dialogOpen && <CreateTokenModal onClose={() => setDialogOpen(false)} onCreated={(value) => { setDialogOpen(false); setSecret(value); load() }} />}
      {secret && (
        <Modal title="令牌创建成功" onClose={() => setSecret('')}>
          <div className="warning-callout"><KeyRound /><div><strong>请立即复制</strong><p>关闭后无法再次查看，服务端只保存令牌摘要。</p></div></div>
          <CodeCopy value={secret} />
          <div className="modal-actions"><button className="primary-button" onClick={() => setSecret('')}>我已保存</button></div>
        </Modal>
      )}
    </>
  )
}

function CreateTokenModal({ onClose, onCreated }: { onClose: () => void; onCreated: (secret: string) => void }) {
  const [name, setName] = useState('')
  const [error, setError] = useState('')
  async function submit(event: FormEvent) {
    event.preventDefault()
    try {
      const result = await api.createToken(name)
      onCreated(result.secret)
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建失败')
    }
  }
  return (
    <Modal title="创建客户端令牌" onClose={onClose}>
      <form className="stacked-form" onSubmit={submit}>
        <label><span>令牌名称</span><input value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：办公室电脑" autoFocus required /></label>
        <p className="muted-copy">建议每台电脑使用独立令牌，丢失设备时可以只撤销对应令牌。</p>
        {error && <div className="form-error">{error}</div>}
        <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>取消</button><button className="primary-button">创建</button></div>
      </form>
    </Modal>
  )
}

function DownloadsPage() {
  const [devices, setDevices] = useState<Device[]>([])

  useEffect(() => {
    api.devices().then((result) => setDevices(result.devices)).catch(() => setDevices([]))
  }, [])

  const currentVersion = '0.3.1'
  const currentCount = devices.filter((device) => device.clientVersion === currentVersion).length
  const outdatedCount = devices.filter((device) => device.clientVersion && device.clientVersion !== currentVersion).length
  return (
    <>
      <PageHeader eyebrow="管理" title="桌面客户端" description="统一分发 Windows 桌面程序，并查看团队设备的安装与版本状态" />
      <div className="client-release-grid">
        <Panel className="download-hero desktop-release-hero">
          <div className="download-product">
            <span className="download-icon"><Code2 /></span>
            <div>
              <span className="eyebrow">WINDOWS 10 / 11 · X64</span>
              <h2>Codex Continuity 桌面版</h2>
              <p>提供可视化配置、系统托盘、自动同步、离线队列、加密导入导出与跨设备续接，无需使用 PowerShell。</p>
            </div>
          </div>
          <div className="desktop-download-actions">
            <a className="primary-button download-button" href="/downloads/codex-continuity_0.3.1_x64-setup.exe">
              <Download size={18} /> 下载安装程序
            </a>
            <a className="secondary-button" href="/downloads/codex-continuity_0.3.1_x64_zh-CN.msi">
              下载 MSI
            </a>
          </div>
          <div className="download-meta"><span>版本 v{currentVersion}</span><span>NSIS / MSI</span><span>支持静默安装</span><span>内置离线 WebView2</span></div>
        </Panel>
        <Panel title="版本覆盖" className="release-status-card">
          <div className="release-metrics">
            <div><span>登记设备</span><strong>{devices.length}</strong><small>已连接过服务端</small></div>
            <div><span>当前版本</span><strong>{currentCount}</strong><small>v{currentVersion}</small></div>
            <div className={outdatedCount ? 'needs-update' : ''}><span>待升级</span><strong>{outdatedCount}</strong><small>{outdatedCount ? '建议尽快更新' : '全部已更新'}</small></div>
          </div>
          <div className="release-security"><ShieldCheck size={18} /><span><strong>客户端安装包需要代码签名</strong><small>正式分发前配置组织的 Windows EV/OV 签名证书。</small></span></div>
        </Panel>
        <Panel title="安装与激活" className="desktop-onboarding">
          <ol className="step-list">
            <li><span>1</span><div><strong>下载安装程序</strong><p>双击安装后自动打开主窗口，并在系统托盘保持运行。</p></div></li>
            <li><span>2</span><div><strong>创建客户端令牌</strong><p>在“API 令牌”页面创建凭据，复制到桌面客户端的基础配置。</p></div></li>
            <li><span>3</span><div><strong>完成两项测试</strong><p>依次执行“连接测试”和“加密上传测试”，通过后客户端会自动同步。</p></div></li>
          </ol>
        </Panel>
        <Panel title="兼容与批量部署" className="deployment-options">
          <div className="deployment-option"><Rocket size={18} /><span><strong>个人安装</strong><small>使用 NSIS 安装程序，当前用户无需管理员权限。</small></span></div>
          <div className="deployment-option"><Laptop size={18} /><span><strong>企业批量安装</strong><small>通过 MSI、Intune 或 SCCM 静默分发到团队电脑。</small></span></div>
          <div className="deployment-option"><TerminalSquare size={18} /><span><strong>命令行兼容</strong><small>保留 Go 核心程序，自动化脚本可继续使用。</small></span></div>
          <a className="legacy-download" href="/downloads/continuity-windows-amd64.exe"><Download size={15} /> 下载旧版命令行核心 v0.1.0</a>
        </Panel>
      </div>
    </>
  )
}

function SettingsPage({ theme, setTheme }: { theme: ThemeName; setTheme: (theme: ThemeName) => void }) {
  return (
    <>
      <PageHeader eyebrow="系统" title="设置" description="调整界面外观并查看当前安全策略" />
      <div className="settings-grid">
        <Panel title="主题颜色">
          <div className="theme-card-grid">
            {themeOptions.map((option) => (
              <button key={option.id} className={`theme-card ${theme === option.id ? 'active' : ''}`} onClick={() => setTheme(option.id)}>
                <span className={`theme-preview theme-${option.id}`}><i /><i /><i /></span>
                <span><strong>{option.label}主题</strong><small>{theme === option.id ? '正在使用' : '点击切换'}</small></span>
                {theme === option.id && <Check size={18} />}
              </button>
            ))}
          </div>
        </Panel>
        <Panel title="安全策略">
          <div className="security-list">
            <SecurityRow title="传输内容" value="客户端 AES-256-GCM 加密" />
            <SecurityRow title="登录密码" value="Argon2id 哈希" />
            <SecurityRow title="客户端授权" value="按设备创建、随时撤销的令牌" />
            <SecurityRow title="数据隔离" value="按用户强制隔离" />
            <SecurityRow title="网络建议" value="HTTPS 或私有 VPN / Tailscale" />
          </div>
        </Panel>
        <Panel title="产品边界">
          <div className="boundary-copy">
            <ShieldCheck />
            <p>当前版本不会修改目标电脑的 Codex 内部数据库，也不会声称把运行中的原任务原样迁移。它保存同步瞬间的工作区、Git 状态和相关会话只读快照，并在另一台电脑生成明确的续接入口。</p>
          </div>
        </Panel>
      </div>
    </>
  )
}

function PageHeader({ eyebrow, title, description, action }: { eyebrow: string; title: string; description: string; action?: ReactNode }) {
  return (
    <div className="page-header">
      <div><span className="page-eyebrow">{eyebrow}</span><h1>{title}</h1><p>{description}</p></div>
      {action && <div className="page-actions">{action}</div>}
    </div>
  )
}

function MetricCard({ icon, tone, label, value, footnote }: { icon: ReactNode; tone: string; label: string; value: string | number; footnote: string }) {
  return (
    <div className="metric-card">
      <span className={`metric-icon ${tone}`}>{icon}</span>
      <div><span>{label}</span><strong>{value}</strong><small>{footnote}</small></div>
    </div>
  )
}

function Panel({ title, action, className = '', children }: { title?: string; action?: ReactNode; className?: string; children: ReactNode }) {
  return (
    <section className={`panel ${className}`}>
      {(title || action) && <div className="panel-header"><h2>{title}</h2>{action}</div>}
      <div className="panel-body">{children}</div>
    </section>
  )
}

function HandoffTable({ handoffs, loading, detailed = false }: { handoffs: Handoff[]; loading: boolean; detailed?: boolean }) {
  if (loading) return <SkeletonRows count={4} />
  if (!handoffs.length) return <EmptyState icon={<Send />} text="还没有会话快照。客户端完成首次同步后会显示在这里。" />
  return (
    <div className="data-table-wrap">
      <table className="data-table handoff-table">
        <thead><tr><th>工作区</th><th>来源设备</th><th>目标设备</th><th>状态</th>{detailed && <th>大小</th>}<th>更新时间</th></tr></thead>
        <tbody>
          {handoffs.map((handoff, index) => (
            <tr key={handoff.id}>
              <td><div className="project-cell"><span className={`project-icon tone-${index % 3}`}><Code2 /></span><div><strong>{handoff.projectName}</strong><small>{handoff.workspaceKey}</small></div></div></td>
              <td><div className="device-table-cell"><Monitor size={14} /><span>{handoff.sourceDeviceName}</span></div></td>
              <td>{handoff.targetDeviceName || <span className="muted">任意设备</span>}</td>
              <td><span className={`status-badge ${handoff.status === 'claimed' ? 'success' : 'pending'}`}><i /> {handoff.status === 'claimed' ? '已接收' : '可接收'}</span></td>
              {detailed && <td>{humanBytes(handoff.blobSize)}</td>}
              <td>{relativeTime(handoff.createdAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function DeviceCompact({ device }: { device: Device }) {
  const online = isOnline(device.lastSeenAt)
  return (
    <div className="device-compact">
      <span className="device-os-icon"><Monitor /></span>
      <div><strong>{device.name}</strong><small>{device.os} · v{device.clientVersion}</small><span className={online ? 'online' : ''}>{online ? '在线' : '离线'} · {relativeTime(device.lastSeenAt)}</span></div>
      <span className={`pulse-ring ${online ? 'online' : ''}`}>{online ? 'ON' : 'OFF'}</span>
    </div>
  )
}

function QuickAction({ icon, title, text, onClick }: { icon: ReactNode; title: string; text: string; onClick: () => void }) {
  return <button className="quick-action" onClick={onClick}><span>{icon}</span><div><strong>{title}</strong><small>{text}</small></div><ChevronRight size={17} /></button>
}

function EmptyState({ icon, text, compact = false }: { icon?: ReactNode; text: string; compact?: boolean }) {
  return <div className={`empty-state ${compact ? 'compact' : ''}`}>{icon && <span>{icon}</span>}<p>{text}</p></div>
}

function SkeletonRows({ count }: { count: number }) {
  return <div className="skeleton-list">{Array.from({ length: count }).map((_, index) => <div className="skeleton-row" key={index}><i /><span /><span /></div>)}</div>
}

function ErrorBanner({ message, retry }: { message: string; retry: () => void }) {
  return <div className="error-banner"><span>{message}</span><button onClick={retry}><RefreshCw size={15} /> 重试</button></div>
}

function StatusBadge({ online }: { online: boolean }) {
  return <span className={`status-badge ${online ? 'success' : 'neutral'}`}><i /> {online ? '在线' : '离线'}</span>
}

function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
      <div className="modal" role="dialog" aria-modal="true" aria-label={title}>
        <div className="modal-header"><h2>{title}</h2><button className="icon-button" onClick={onClose}><X size={20} /></button></div>
        <div className="modal-body">{children}</div>
      </div>
    </div>
  )
}

function CodeCopy({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="code-copy">
      <code>{value}</code>
      <button onClick={async () => { await navigator.clipboard.writeText(value); setCopied(true); setTimeout(() => setCopied(false), 1500) }}>
        {copied ? <Check size={16} /> : <Copy size={16} />} {copied ? '已复制' : '复制'}
      </button>
    </div>
  )
}

function SecurityRow({ title, value }: { title: string; value: string }) {
  return <div className="security-row"><span><ShieldCheck size={17} />{title}</span><strong>{value}</strong></div>
}

function BrandMark({ inverse = false }: { inverse?: boolean }) {
  return <span className={`brand-mark ${inverse ? 'inverse' : ''}`}><Link2 size={24} strokeWidth={2.3} /></span>
}

function humanBytes(size: number) {
  if (!Number.isFinite(size) || size < 1024) return `${size || 0} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = size
  let index = -1
  do { value /= 1024; index += 1 } while (value >= 1024 && index < units.length - 1)
  return `${value.toFixed(value >= 100 ? 0 : 1)} ${units[index]}`
}

function relativeTime(value: string) {
  const delta = Date.now() - new Date(value).getTime()
  if (delta < 60_000) return '刚刚'
  if (delta < 3_600_000) return `${Math.floor(delta / 60_000)} 分钟前`
  if (delta < 86_400_000) return `${Math.floor(delta / 3_600_000)} 小时前`
  if (delta < 7 * 86_400_000) return `${Math.floor(delta / 86_400_000)} 天前`
  return formatDate(value)
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date(value))
}

function isOnline(lastSeen: string) {
  return Date.now() - new Date(lastSeen).getTime() < 5 * 60_000
}

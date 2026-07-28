import type { ApiToken, DesktopReleaseManifest, Device, Handoff, Overview, User } from './types'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    credentials: 'same-origin',
    ...init,
    headers: {
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })
  if (response.status === 204) return undefined as T
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(body.error || `请求失败（${response.status}）`)
  return body as T
}

export const api = {
  login: (email: string, password: string) =>
    request<{ user: User }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    }),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
  me: () => request<{ user: User }>('/me'),
  overview: () => request<Overview>('/overview'),
  devices: () => request<{ devices: Device[] }>('/devices'),
  handoffs: () => request<{ handoffs: Handoff[] }>('/handoffs'),
  users: () => request<{ users: User[] }>('/users'),
  createUser: (input: { email: string; displayName: string; password: string; role: string }) =>
    request<{ user: User }>('/users', { method: 'POST', body: JSON.stringify(input) }),
  tokens: () => request<{ tokens: ApiToken[] }>('/tokens'),
  createToken: (name: string) =>
    request<{ token: ApiToken; secret: string }>('/tokens', {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
  deleteToken: (id: string) => request<void>(`/tokens/${id}`, { method: 'DELETE' }),
  desktopRelease: async () => {
    const response = await fetch('/downloads/desktop-release.json', {
      credentials: 'same-origin',
      cache: 'no-store',
    })
    if (!response.ok) throw new Error('服务器尚未发布桌面安装包清单')
    return response.json() as Promise<DesktopReleaseManifest>
  },
}

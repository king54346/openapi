import type { ApiResponse, User } from '../types/api'

const USER_STORAGE_KEY = 'openapi_current_user'

export function getStoredUser(): User | null {
  try {
    const raw = localStorage.getItem(USER_STORAGE_KEY)
    if (!raw) return null
    return JSON.parse(raw) as User
  } catch {
    return null
  }
}

export function setStoredUser(user: User | null) {
  try {
    if (user) {
      localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(user))
    } else {
      localStorage.removeItem(USER_STORAGE_KEY)
    }
  } catch {
    // ignore storage quota errors
  }
}

export class ApiError extends Error {
  status: number
  constructor(message: string, status = 400) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

function buildQuery(params?: Record<string, unknown>): string {
  if (!params) return ''
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    sp.set(k, String(v))
  }
  const qs = sp.toString()
  return qs ? `?${qs}` : ''
}

function buildAuthHeaders(extra?: Record<string, string>): Record<string, string> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...extra,
  }
  const user = getStoredUser()
  if (user?.id) {
    headers['Open-Api-User'] = String(user.id)
  }
  return headers
}

async function request<T>(
  method: string,
  path: string,
  options?: {
    params?: Record<string, unknown>
    body?: unknown
    headers?: Record<string, string>
    skipAuthRedirect?: boolean
  },
): Promise<T> {
  const url = `${path}${buildQuery(options?.params)}`
  const init: RequestInit = {
    method,
    credentials: 'include',
    headers: buildAuthHeaders(options?.headers),
  }
  if (options?.body !== undefined) {
    init.body = typeof options.body === 'string' ? options.body : JSON.stringify(options.body)
  }

  const resp = await fetch(url, init)
  const text = await resp.text()
  let parsed: ApiResponse<T> | null = null
  if (text) {
    try {
      parsed = JSON.parse(text) as ApiResponse<T>
    } catch {
      // non-json response
    }
  }

  if (resp.status === 401 && !options?.skipAuthRedirect) {
    setStoredUser(null)
    window.dispatchEvent(new CustomEvent('openapi:unauthorized'))
  }

  if (!resp.ok || !parsed || !parsed.success) {
    const errMsg = parsed?.message || `请求失败 (HTTP ${resp.status})`
    throw new ApiError(errMsg, resp.status)
  }

  return parsed.data
}

export const api = {
  get: <T>(path: string, params?: Record<string, unknown>, skipAuthRedirect = false) =>
    request<T>('GET', path, { params, skipAuthRedirect }),

  post: <T>(path: string, body?: unknown, params?: Record<string, unknown>) =>
    request<T>('POST', path, { body, params }),

  put: <T>(path: string, body?: unknown, params?: Record<string, unknown>) =>
    request<T>('PUT', path, { body, params }),

  del: <T>(path: string, params?: Record<string, unknown>, body?: unknown) =>
    request<T>('DELETE', path, { params, body }),

  // 用于直接调用 /pg/* 或 /v1/* OpenAI 协议格式接口（非 {success, data} 包装）
  rawJson: async <T = unknown>(
    path: string,
    body: unknown,
    extraHeaders?: Record<string, string>,
  ): Promise<T> => {
    const resp = await fetch(path, {
      method: 'POST',
      credentials: 'include',
      headers: buildAuthHeaders(extraHeaders),
      body: JSON.stringify(body),
    })
    const text = await resp.text()
    let data: unknown = text
    try {
      data = JSON.parse(text)
    } catch {
      // keep raw text
    }
    if (!resp.ok) {
      const errObj = data as { error?: { message?: string }; message?: string }
      throw new ApiError(
        errObj?.error?.message || errObj?.message || `请求失败 (HTTP ${resp.status})`,
        resp.status,
      )
    }
    return data as T
  },
}

// Playground 流式 SSE 对话读取器
export async function streamChatCompletion(
  path: string,
  payload: Record<string, unknown>,
  onDelta: (deltaContent: string, reasoningDelta: string) => void,
  signal?: AbortSignal,
  extraHeaders?: Record<string, string>,
): Promise<void> {
  const resp = await fetch(path, {
    method: 'POST',
    credentials: 'include',
    headers: buildAuthHeaders(extraHeaders),
    body: JSON.stringify({ ...payload, stream: true }),
    signal,
  })

  if (!resp.ok) {
    const errText = await resp.text()
    let msg = `HTTP ${resp.status}`
    try {
      const parsed = JSON.parse(errText) as { error?: { message?: string }; message?: string }
      msg = parsed?.error?.message || parsed?.message || msg
    } catch {
      if (errText) msg = errText
    }
    throw new ApiError(msg, resp.status)
  }

  if (!resp.body) {
    throw new ApiError('浏览器不支持流式响应读取', 500)
  }

  const reader = resp.body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buffer = ''

  while (true) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() ?? ''

    for (const rawLine of lines) {
      const line = rawLine.trim()
      if (!line || !line.startsWith('data:')) continue
      const dataStr = line.slice(5).trim()
      if (dataStr === '[DONE]') return
      try {
        const chunk = JSON.parse(dataStr) as {
          choices?: Array<{
            delta?: { content?: string; reasoning_content?: string }
          }>
          error?: { message?: string }
        }
        if (chunk.error?.message) {
          throw new ApiError(chunk.error.message, 500)
        }
        const delta = chunk.choices?.[0]?.delta
        if (delta) {
          onDelta(delta.content ?? '', delta.reasoning_content ?? '')
        }
      } catch (e) {
        if (e instanceof ApiError) throw e
      }
    }
  }
}

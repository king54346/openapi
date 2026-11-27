import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { api, getStoredUser, setStoredUser } from '../api/client'
import type { PublicStatus, SetupStatus, User } from '../types/api'

interface AppContextValue {
  bootstrapping: boolean
  setupStatus: SetupStatus | null
  publicStatus: PublicStatus | null
  user: User | null
  isAdmin: boolean
  isRoot: boolean
  darkMode: boolean
  toggleDarkMode: () => void
  refreshBootstrap: () => Promise<void>
  refreshUser: () => Promise<User | null>
  loginWithCredentials: (username: string, password: string, turnstileToken?: string) => Promise<User>
  logout: () => Promise<void>
}

const AppContext = createContext<AppContextValue | undefined>(undefined)

const THEME_STORAGE_KEY = 'openapi_theme_dark'

export const AppProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [bootstrapping, setBootstrapping] = useState(true)
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null)
  const [publicStatus, setPublicStatus] = useState<PublicStatus | null>(null)
  const [user, setUser] = useState<User | null>(() => getStoredUser())
  const [darkMode, setDarkMode] = useState<boolean>(() => localStorage.getItem(THEME_STORAGE_KEY) === 'true')

  const toggleDarkMode = useCallback(() => {
    setDarkMode((prev) => {
      const next = !prev
      localStorage.setItem(THEME_STORAGE_KEY, String(next))
      return next
    })
  }, [])

  useEffect(() => {
    document.documentElement.dataset.theme = darkMode ? 'dark' : 'light'
  }, [darkMode])

  const refreshUser = useCallback(async (): Promise<User | null> => {
    const cached = getStoredUser()
    if (!cached?.id) {
      setUser(null)
      return null
    }
    try {
      const fresh = await api.get<User>('/api/user/self', undefined, true)
      setStoredUser(fresh)
      setUser(fresh)
      return fresh
    } catch {
      setStoredUser(null)
      setUser(null)
      return null
    }
  }, [])

  const refreshBootstrap = useCallback(async () => {
    try {
      const [setup, status] = await Promise.all([
        api.get<SetupStatus>('/api/setup', undefined, true),
        api.get<PublicStatus>('/api/status', undefined, true),
      ])
      setSetupStatus(setup)
      setPublicStatus(status)
      if (setup.initialized && getStoredUser()?.id) {
        await refreshUser()
      }
    } catch {
      // keep existing state when offline during dev
    } finally {
      setBootstrapping(false)
    }
  }, [refreshUser])

  useEffect(() => {
    void refreshBootstrap()
  }, [refreshBootstrap])

  useEffect(() => {
    const onUnauthorized = () => {
      setUser(null)
      setStoredUser(null)
    }
    window.addEventListener('openapi:unauthorized', onUnauthorized)
    return () => window.removeEventListener('openapi:unauthorized', onUnauthorized)
  }, [])

  const loginWithCredentials = useCallback(
    async (username: string, password: string, turnstileToken?: string): Promise<User> => {
      const params = turnstileToken ? { turnstile: turnstileToken } : undefined
      const loggedIn = await api.post<User>('/api/user/login', { username, password }, params)
      setStoredUser(loggedIn)
      setUser(loggedIn)
      // 登录成功后拉取完整用户信息
      try {
        const full = await api.get<User>('/api/user/self', undefined, true)
        setStoredUser(full)
        setUser(full)
        return full
      } catch {
        return loggedIn
      }
    },
    [],
  )

  const logout = useCallback(async () => {
    try {
      await api.get('/api/user/logout', undefined, true)
    } catch {
      // ignore network error on logout
    } finally {
      setStoredUser(null)
      setUser(null)
    }
  }, [])

  const value = useMemo<AppContextValue>(
    () => ({
      bootstrapping,
      setupStatus,
      publicStatus,
      user,
      isAdmin: (user?.role ?? 0) >= 10,
      isRoot: (user?.role ?? 0) >= 100,
      darkMode,
      toggleDarkMode,
      refreshBootstrap,
      refreshUser,
      loginWithCredentials,
      logout,
    }),
    [
      bootstrapping,
      setupStatus,
      publicStatus,
      user,
      darkMode,
      toggleDarkMode,
      refreshBootstrap,
      refreshUser,
      loginWithCredentials,
      logout,
    ],
  )

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>
}

export function useApp(): AppContextValue {
  const ctx = useContext(AppContext)
  if (!ctx) {
    throw new Error('useApp must be used inside AppProvider')
  }
  return ctx
}

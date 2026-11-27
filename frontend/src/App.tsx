import React from 'react'
import { App as AntdApp, ConfigProvider, Result, Spin, Button, theme } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { BrowserRouter, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AppLayout } from './components/layout/AppLayout'
import { AppProvider, useApp } from './contexts/AppContext'
import { LoginPage } from './pages/auth/LoginPage'
import { SetupPage } from './pages/auth/SetupPage'
import { ChannelPage } from './pages/channel/ChannelPage'
import { ChannelTestPage } from './pages/channel/ChannelTestPage'
import { CronJobPage } from './pages/cron/CronJobPage'
import { DashboardPage } from './pages/dashboard/DashboardPage'
import { LogPage } from './pages/log/LogPage'
import { ModelsPage } from './pages/models/ModelsPage'
import { PersonalPage } from './pages/personal/PersonalPage'
import { PlaygroundPage } from './pages/playground/PlaygroundPage'
import {
  AboutPage,
  HomePage,
  NotFoundPage,
  PrivacyPolicyPage,
  UserAgreementPage,
} from './pages/public/PublicPages'
import { SettingPage } from './pages/setting/SettingPage'
import { TaskPage } from './pages/task/TaskPage'
import { TokenPage } from './pages/token/TokenPage'
import { UserPage } from './pages/user/UserPage'

const FullscreenSpin: React.FC = () => (
  <div
    style={{
      minHeight: '60vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
    }}
  >
    <Spin size="large" />
  </div>
)

const ForbiddenView: React.FC = () => {
  const navigate = useNavigate()
  return (
    <Result
      status="403"
      title="无访问权限"
      subTitle="当前账号角色无权访问该管理模块"
      extra={
        <Button type="primary" onClick={() => navigate('/console')}>
          返回控制台
        </Button>
      }
    />
  )
}

const RequireAuth: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { bootstrapping, setupStatus, user } = useApp()
  const location = useLocation()

  if (bootstrapping) return <FullscreenSpin />
  if (setupStatus && !setupStatus.initialized) {
    return <Navigate to="/setup" replace />
  }
  if (!user) {
    return <Navigate to="/login" state={{ from: location.pathname }} replace />
  }
  return <>{children}</>
}

const RequireAdmin: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { isAdmin } = useApp()
  return (
    <RequireAuth>
      {isAdmin ? children : <ForbiddenView />}
    </RequireAuth>
  )
}

const RequireRoot: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { isRoot } = useApp()
  return (
    <RequireAuth>
      {isRoot ? children : <ForbiddenView />}
    </RequireAuth>
  )
}

const ThemedRouter: React.FC = () => {
  const { darkMode, bootstrapping, setupStatus } = useApp()
  const location = useLocation()

  if (!bootstrapping && setupStatus && !setupStatus.initialized && location.pathname !== '/setup') {
    return (
      <ConfigProvider
        locale={zhCN}
        theme={{
          algorithm: darkMode ? theme.darkAlgorithm : theme.defaultAlgorithm,
          token: { colorPrimary: '#1677ff', borderRadius: 8 },
        }}
      >
        <AntdApp>
          <Navigate to="/setup" replace />
        </AntdApp>
      </ConfigProvider>
    )
  }

  return (
    <ConfigProvider
      locale={zhCN}
      theme={{
        algorithm: darkMode ? theme.darkAlgorithm : theme.defaultAlgorithm,
        token: {
          colorPrimary: '#1677ff',
          borderRadius: 8,
        },
      }}
    >
      <AntdApp>
        <Routes>
          <Route path="/setup" element={<SetupPage />} />
          <Route element={<AppLayout />}>
            <Route path="/" element={<HomePage />} />
            <Route path="/login" element={<LoginPage />} />
            <Route path="/about" element={<AboutPage />} />
            <Route path="/user-agreement" element={<UserAgreementPage />} />
            <Route path="/privacy-policy" element={<PrivacyPolicyPage />} />

            {/* 控制台受保护路由 */}
            <Route
              path="/console"
              element={
                <RequireAuth>
                  <DashboardPage />
                </RequireAuth>
              }
            />
            <Route
              path="/console/playground"
              element={
                <RequireAuth>
                  <PlaygroundPage />
                </RequireAuth>
              }
            />
            <Route
              path="/console/token"
              element={
                <RequireAuth>
                  <TokenPage />
                </RequireAuth>
              }
            />
            <Route
              path="/console/log"
              element={
                <RequireAuth>
                  <LogPage />
                </RequireAuth>
              }
            />
            <Route
              path="/console/task"
              element={
                <RequireAuth>
                  <TaskPage />
                </RequireAuth>
              }
            />
            <Route
              path="/console/personal"
              element={
                <RequireAuth>
                  <PersonalPage />
                </RequireAuth>
              }
            />

            {/* 管理员路由 */}
            <Route
              path="/console/channel"
              element={
                <RequireAdmin>
                  <ChannelPage />
                </RequireAdmin>
              }
            />
            <Route
              path="/console/channel-test"
              element={
                <RequireAdmin>
                  <ChannelTestPage />
                </RequireAdmin>
              }
            />
            <Route
              path="/console/cron-job"
              element={
                <RequireAdmin>
                  <CronJobPage />
                </RequireAdmin>
              }
            />
            <Route
              path="/console/models"
              element={
                <RequireAdmin>
                  <ModelsPage />
                </RequireAdmin>
              }
            />
            <Route
              path="/console/user"
              element={
                <RequireAdmin>
                  <UserPage />
                </RequireAdmin>
              }
            />

            {/* Root 路由 */}
            <Route
              path="/console/setting"
              element={
                <RequireRoot>
                  <SettingPage />
                </RequireRoot>
              }
            />

            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Routes>
      </AntdApp>
    </ConfigProvider>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <AppProvider>
        <ThemedRouter />
      </AppProvider>
    </BrowserRouter>
  )
}

import React, { useEffect, useRef, useState } from 'react'
import { Button, Card, Form, Input, Space, Typography, App } from 'antd'
import { KeyRound, Lock, User as UserIcon } from 'lucide-react'
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { useApp } from '../../contexts/AppContext'

declare global {
  interface Window {
    turnstile?: {
      render: (
        container: HTMLElement,
        options: { sitekey: string; callback: (token: string) => void },
      ) => string
      remove?: (widgetId: string) => void
    }
  }
}

export const LoginPage: React.FC = () => {
  const { user, setupStatus, publicStatus, loginWithCredentials } = useApp()
  const { message } = App.useApp()
  const navigate = useNavigate()
  const location = useLocation()
  const [loading, setLoading] = useState(false)
  const [turnstileToken, setTurnstileToken] = useState('')
  const turnstileRef = useRef<HTMLDivElement | null>(null)

  const redirectTo =
    (location.state as { from?: string } | null)?.from || '/console'

  const turnstileEnabled = Boolean(
    publicStatus?.turnstile_check && publicStatus?.turnstile_site_key,
  )

  useEffect(() => {
    if (!turnstileEnabled || !publicStatus?.turnstile_site_key || !turnstileRef.current) {
      return
    }
    const renderWidget = () => {
      if (window.turnstile && turnstileRef.current) {
        turnstileRef.current.innerHTML = ''
        window.turnstile.render(turnstileRef.current, {
          sitekey: publicStatus.turnstile_site_key,
          callback: (token: string) => setTurnstileToken(token),
        })
      }
    }

    if (window.turnstile) {
      renderWidget()
      return
    }

    const script = document.createElement('script')
    script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
    script.async = true
    script.onload = renderWidget
    document.body.appendChild(script)
  }, [turnstileEnabled, publicStatus?.turnstile_site_key])

  if (setupStatus && !setupStatus.initialized) {
    return <Navigate to="/setup" replace />
  }

  if (user) {
    return <Navigate to={redirectTo} replace />
  }

  const onFinish = async (values: { username: string; password: string }) => {
    if (turnstileEnabled && !turnstileToken) {
      message.warning('请先完成人机验证')
      return
    }
    setLoading(true)
    try {
      await loginWithCredentials(values.username.trim(), values.password, turnstileToken || undefined)
      message.success('登录成功')
      navigate(redirectTo, { replace: true })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      style={{
        minHeight: 'calc(100vh - 60px)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 24,
      }}
    >
      <Card style={{ width: '100%', maxWidth: 420, borderRadius: 14 }}>
        <div style={{ textAlign: 'center', marginBottom: 28 }}>
          <div
            style={{
              width: 46,
              height: 46,
              borderRadius: 12,
              margin: '0 auto 12px',
              background: 'linear-gradient(135deg, #1677ff 0%, #4f46e5 100%)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#fff',
            }}
          >
            <KeyRound size={22} />
          </div>
          <Typography.Title level={4} style={{ marginBottom: 4 }}>
            登录控制台
          </Typography.Title>
          <Typography.Text type="secondary">
            统一大模型网关 · 多协议中继与集群管理
          </Typography.Text>
        </div>

        <Form layout="vertical" onFinish={onFinish}>
          <Form.Item
            name="username"
            label="用户名"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input
              prefix={<UserIcon size={16} style={{ color: '#9ca3af' }} />}
              placeholder="请输入用户名"
              size="large"
              autoComplete="username"
            />
          </Form.Item>

          <Form.Item
            name="password"
            label="密码"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password
              prefix={<Lock size={16} style={{ color: '#9ca3af' }} />}
              placeholder="请输入密码"
              size="large"
              autoComplete="current-password"
            />
          </Form.Item>

          {turnstileEnabled && (
            <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'center' }}>
              <div ref={turnstileRef} />
            </div>
          )}

          <Button type="primary" htmlType="submit" size="large" block loading={loading}>
            立即登录
          </Button>
        </Form>

        <div style={{ marginTop: 20, textAlign: 'center' }}>
          <Space split={<span style={{ color: '#d1d5db' }}>·</span>} size={8}>
            <Link to="/user-agreement" style={{ fontSize: 12 }}>
              用户协议
            </Link>
            <Link to="/privacy-policy" style={{ fontSize: 12 }}>
              隐私政策
            </Link>
            <Link to="/about" style={{ fontSize: 12 }}>
              关于我们
            </Link>
          </Space>
        </div>
      </Card>
    </div>
  )
}

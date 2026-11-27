import React, { useState } from 'react'
import { Alert, Button, Card, Form, Input, Typography, App } from 'antd'
import { ShieldCheck } from 'lucide-react'
import { Navigate, useNavigate } from 'react-router-dom'
import { api } from '../../api/client'
import { useApp } from '../../contexts/AppContext'

export const SetupPage: React.FC = () => {
  const { setupStatus, refreshBootstrap } = useApp()
  const { message } = App.useApp()
  const navigate = useNavigate()
  const [submitting, setSubmitting] = useState(false)

  if (setupStatus?.initialized) {
    return <Navigate to="/login" replace />
  }

  const onFinish = async (values: {
    username: string
    display_name?: string
    password: string
    confirm_password: string
  }) => {
    setSubmitting(true)
    try {
      await api.post('/api/setup', values)
      message.success('系统初始化完成，请使用超级管理员账号登录')
      await refreshBootstrap()
      navigate('/login', { replace: true })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '初始化失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 24,
        background: 'var(--bg-page)',
      }}
    >
      <Card style={{ width: '100%', maxWidth: 460, borderRadius: 14 }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <div
            style={{
              width: 48,
              height: 48,
              borderRadius: 12,
              margin: '0 auto 12px',
              background: 'linear-gradient(135deg, #1677ff 0%, #4f46e5 100%)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#fff',
            }}
          >
            <ShieldCheck size={24} />
          </div>
          <Typography.Title level={4} style={{ marginBottom: 4 }}>
            初始化 OpenAPI 网关
          </Typography.Title>
          <Typography.Text type="secondary">
            首次运行，请创建超级管理员（Root）账号
          </Typography.Text>
        </div>

        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 20 }}
          message="Root 账号拥有渠道、系统设置、性能监控与用户管理的最高权限。"
        />

        <Form layout="vertical" onFinish={onFinish} autoComplete="off">
          <Form.Item
            name="username"
            label="管理员用户名"
            rules={[
              { required: true, message: '请输入管理员用户名' },
              { max: 20, message: '用户名长度不超过 20 个字符' },
            ]}
          >
            <Input placeholder="例如 admin" size="large" />
          </Form.Item>

          <Form.Item name="display_name" label="显示名称（可选）">
            <Input placeholder="例如 系统管理员" size="large" />
          </Form.Item>

          <Form.Item
            name="password"
            label="登录密码"
            rules={[
              { required: true, message: '请输入密码' },
              { min: 8, message: '密码长度至少 8 位' },
            ]}
          >
            <Input.Password placeholder="8-64 位密码" size="large" />
          </Form.Item>

          <Form.Item
            name="confirm_password"
            label="确认密码"
            dependencies={['password']}
            rules={[
              { required: true, message: '请再次输入密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('password') === value) {
                    return Promise.resolve()
                  }
                  return Promise.reject(new Error('两次输入的密码不一致'))
                },
              }),
            ]}
          >
            <Input.Password placeholder="再次输入密码" size="large" />
          </Form.Item>

          <Button type="primary" htmlType="submit" size="large" block loading={submitting}>
            完成初始化
          </Button>
        </Form>
      </Card>
    </div>
  )
}

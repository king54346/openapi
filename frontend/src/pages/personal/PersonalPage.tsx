import React, { useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  Input,
  Popconfirm,
  Space,
  Tag,
  App,
} from 'antd'
import { Copy, KeyRound, RefreshCw, User as UserIcon } from 'lucide-react'
import { api } from '../../api/client'
import { getRoleInfo } from '../../constants/channel'
import { useApp } from '../../contexts/AppContext'

export const PersonalPage: React.FC = () => {
  const { user, refreshUser } = useApp()
  const { message } = App.useApp()
  const [models, setModels] = useState<string[]>([])
  const [accessToken, setAccessToken] = useState('')
  const [generating, setGenerating] = useState(false)
  const [filter, setFilter] = useState('')

  useEffect(() => {
    void refreshUser()
    api
      .get<string[]>('/api/user/models')
      .then((list) => setModels(list || []))
      .catch(() => setModels([]))
  }, [refreshUser])

  const generateToken = async () => {
    setGenerating(true)
    try {
      const token = await api.get<string>('/api/user/token')
      setAccessToken(token)
      message.success('已生成新的系统访问令牌，请立即复制保存')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '生成令牌失败')
    } finally {
      setGenerating(false)
    }
  }

  const copyText = async (text: string, label: string) => {
    await navigator.clipboard.writeText(text)
    message.success(`${label}已复制到剪贴板`)
  }

  const roleInfo = getRoleInfo(user?.role ?? 1)
  const visibleModels = models.filter((m) =>
    m.toLowerCase().includes(filter.trim().toLowerCase()),
  )

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 1000 }}>
      <Card
        title={
          <Space>
            <UserIcon size={17} />
            <span>个人账号信息</span>
          </Space>
        }
        extra={
          <Button size="small" icon={<RefreshCw size={14} />} onClick={() => void refreshUser()}>
            刷新
          </Button>
        }
        style={{ borderRadius: 12 }}
      >
        <Descriptions column={{ xs: 1, sm: 2 }} bordered size="small">
          <Descriptions.Item label="用户 ID">{user?.id}</Descriptions.Item>
          <Descriptions.Item label="用户名">{user?.username}</Descriptions.Item>
          <Descriptions.Item label="显示名称">
            {user?.display_name || user?.username}
          </Descriptions.Item>
          <Descriptions.Item label="角色权限">
            <Tag color={roleInfo.color}>{roleInfo.label}</Tag>
          </Descriptions.Item>
          <Descriptions.Item label="账号状态">
            {user?.status === 1 ? (
              <Tag color="success">正常启用</Tag>
            ) : (
              <Tag color="error">已禁用</Tag>
            )}
          </Descriptions.Item>
          <Descriptions.Item label="邮箱">{user?.email || '-'}</Descriptions.Item>
          <Descriptions.Item label="剩余额度 (Quota)">{user?.quota ?? 0}</Descriptions.Item>
          <Descriptions.Item label="累计已用 / 请求数">
            {user?.used_quota ?? 0} / {user?.request_count ?? 0} 次
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Card
        title={
          <Space>
            <KeyRound size={17} />
            <span>管理接口 Access Token</span>
          </Space>
        }
        style={{ borderRadius: 12 }}
      >
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="系统 Access Token 用于以您的账号身份自动化调用 /api/* 管理接口（需同时携带请求头 Open-Api-User: 用户ID）。调用大模型请前往「令牌管理」创建 sk-... API Key。"
        />

        <Space wrap>
          <Popconfirm
            title="生成新的 Access Token？"
            description="生成后原有的旧 Access Token 将立即失效。"
            onConfirm={() => void generateToken()}
          >
            <Button type="primary" loading={generating}>
              生成 / 重置 Access Token
            </Button>
          </Popconfirm>

          {accessToken && (
            <Space.Compact>
              <Input
                readOnly
                value={accessToken}
                style={{ width: 320, fontFamily: 'monospace' }}
              />
              <Button
                icon={<Copy size={14} />}
                onClick={() => void copyText(accessToken, 'Access Token')}
              >
                复制
              </Button>
            </Space.Compact>
          )}
        </Space>
      </Card>

      <Card
        title={`当前账号可用模型 (${models.length})`}
        extra={
          <Input.Search
            allowClear
            placeholder="搜索模型..."
            style={{ width: 220 }}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
        }
        style={{ borderRadius: 12 }}
      >
        {visibleModels.length === 0 ? (
          <Empty description="暂无匹配模型" />
        ) : (
          <Space size={[6, 8]} wrap>
            {visibleModels.map((m) => (
              <Tag
                key={m}
                style={{ cursor: 'pointer', padding: '3px 8px' }}
                onClick={() => void copyText(m, `模型 ${m} `)}
              >
                {m}
              </Tag>
            ))}
          </Space>
        )}
      </Card>
    </div>
  )
}

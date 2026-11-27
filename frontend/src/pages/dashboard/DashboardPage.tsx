import React, { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Col,
  Empty,
  Input,
  Row,
  Space,
  Statistic,
  Tag,
  Typography,
  App,
} from 'antd'
import {
  Activity,
  Bot,
  Copy,
  FileText,
  KeyRound,
  Network,
  RefreshCw,
  Zap,
} from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { api } from '../../api/client'
import { formatUnixTime, getChannelTypeInfo } from '../../constants/channel'
import { useApp } from '../../contexts/AppContext'
import type { LogStat } from '../../types/api'

export const DashboardPage: React.FC = () => {
  const { user, isAdmin, publicStatus } = useApp()
  const { message } = App.useApp()
  const navigate = useNavigate()

  const [loading, setLoading] = useState(false)
  const [selfStat, setSelfStat] = useState<LogStat | null>(null)
  const [globalStat, setGlobalStat] = useState<LogStat | null>(null)
  const [modelsByType, setModelsByType] = useState<Record<string, string[]>>({})
  const [modelSearch, setModelSearch] = useState('')

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [myStat, modelsMap] = await Promise.all([
        api.get<LogStat>('/api/log/self/stat'),
        api.get<Record<string, string[]>>('/api/models'),
      ])
      setSelfStat(myStat)
      setModelsByType(modelsMap || {})

      if (isAdmin) {
        const gStat = await api.get<LogStat>('/api/log/stat')
        setGlobalStat(gStat)
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载看板数据失败')
    } finally {
      setLoading(false)
    }
  }, [isAdmin, message])

  useEffect(() => {
    void loadData()
  }, [loadData])

  const allModelsCount = new Set(Object.values(modelsByType).flat()).size

  const copyModel = async (name: string) => {
    await navigator.clipboard.writeText(name)
    message.success(`已复制模型名：${name}`)
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
        <div>
          <Typography.Title level={4} style={{ margin: 0 }}>
            欢迎回来，{user?.display_name || user?.username}
          </Typography.Title>
          <Typography.Text type="secondary">
            服务地址：{publicStatus?.server_address || window.location.origin} · 启动时间：
            {formatUnixTime(publicStatus?.start_time)}
          </Typography.Text>
        </div>
        <Space wrap>
          <Button icon={<Bot size={15} />} onClick={() => navigate('/console/playground')}>
            体验中心
          </Button>
          <Button icon={<KeyRound size={15} />} onClick={() => navigate('/console/token')}>
            管理令牌
          </Button>
          <Button
            type="primary"
            icon={<RefreshCw size={15} />}
            loading={loading}
            onClick={() => void loadData()}
          >
            刷新数据
          </Button>
        </Space>
      </div>

      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <Card style={{ borderRadius: 12 }}>
            <Statistic
              title="我的累计请求消耗 (Quota)"
              value={selfStat?.quota ?? 0}
              prefix={<Zap size={18} color="#1677ff" />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card style={{ borderRadius: 12 }}>
            <Statistic
              title="我的近一分钟 RPM / TPM"
              value={`${selfStat?.rpm ?? 0} / ${selfStat?.tpm ?? 0}`}
              prefix={<Activity size={18} color="#52c41a" />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card style={{ borderRadius: 12 }}>
            <Statistic
              title="当前在线可用模型数"
              value={allModelsCount}
              prefix={<Bot size={18} color="#722ed1" />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card style={{ borderRadius: 12 }}>
            <Statistic
              title="账户累计请求次数"
              value={user?.request_count ?? 0}
              prefix={<FileText size={18} color="#fa8c16" />}
            />
          </Card>
        </Col>
      </Row>

      {isAdmin && globalStat && (
        <Card
          size="small"
          title={
            <Space>
              <Network size={16} />
              <span>全站实时汇总（管理员视图）</span>
            </Space>
          }
          style={{ borderRadius: 12 }}
        >
          <Row gutter={[16, 16]}>
            <Col xs={24} sm={8}>
              <Statistic title="全站累计 Quota" value={globalStat.quota} />
            </Col>
            <Col xs={24} sm={8}>
              <Statistic title="全站实时 RPM" value={globalStat.rpm} />
            </Col>
            <Col xs={24} sm={8}>
              <Statistic title="全站实时 TPM" value={globalStat.tpm} />
            </Col>
          </Row>
        </Card>
      )}

      <Card
        title="已启用渠道模型目录（点击模型名可复制）"
        extra={
          <Input.Search
            allowClear
            placeholder="过滤模型名称..."
            style={{ width: 240 }}
            value={modelSearch}
            onChange={(e) => setModelSearch(e.target.value)}
          />
        }
        style={{ borderRadius: 12 }}
      >
        {Object.keys(modelsByType).length === 0 ? (
          <Empty description="当前暂无已启用的渠道模型" />
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            {Object.entries(modelsByType).map(([typeStr, list]) => {
              const filtered = list.filter((m) =>
                m.toLowerCase().includes(modelSearch.trim().toLowerCase()),
              )
              if (filtered.length === 0) return null
              const info = getChannelTypeInfo(Number(typeStr))
              return (
                <div key={typeStr}>
                  <div style={{ marginBottom: 8 }}>
                    <Tag color={info.color}>{info.label}</Tag>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      共 {filtered.length} 个模型
                    </Typography.Text>
                  </div>
                  <Space size={[6, 8]} wrap>
                    {filtered.map((m) => (
                      <Tag
                        key={m}
                        style={{ cursor: 'pointer', padding: '3px 8px' }}
                        onClick={() => void copyModel(m)}
                      >
                        <Space size={4}>
                          <span>{m}</span>
                          <Copy size={11} style={{ opacity: 0.55 }} />
                        </Space>
                      </Tag>
                    ))}
                  </Space>
                </div>
              )
            })}
          </div>
        )}
      </Card>
    </div>
  )
}

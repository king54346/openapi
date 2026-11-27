import React, { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  Divider,
  Input,
  InputNumber,
  Popconfirm,
  Row,
  Space,
  Statistic,
  Switch,
  Tabs,
  Tag,
  Typography,
  App,
} from 'antd'
import {
  Activity,
  Cpu,
  FileText,
  RefreshCw,
  Save,
  Shield,
  Sliders,
  Trash2,
} from 'lucide-react'
import { api } from '../../api/client'
import { useApp } from '../../contexts/AppContext'
import type { AffinityStats, PerformanceStats, StatusTestResult } from '../../types/api'

export const SettingPage: React.FC = () => {
  const { refreshBootstrap } = useApp()
  const { message } = App.useApp()

  const [options, setOptions] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(false)
  const [affinityStats, setAffinityStats] = useState<AffinityStats | null>(null)
  const [perfStats, setPerfStats] = useState<PerformanceStats | null>(null)
  const [statusCheck, setStatusCheck] = useState<StatusTestResult | null>(null)
  const [checkingStatus, setCheckingStatus] = useState(false)

  const loadAll = useCallback(async () => {
    setLoading(true)
    try {
      const [optMap, aff, perf] = await Promise.all([
        api.get<Record<string, string>>('/api/option/'),
        api.get<AffinityStats>('/api/option/channel_affinity_cache'),
        api.get<PerformanceStats>('/api/performance/stats'),
      ])
      setOptions(optMap || {})
      setAffinityStats(aff)
      setPerfStats(perf)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载系统配置失败')
    } finally {
      setLoading(false)
    }
  }, [message])

  useEffect(() => {
    void loadAll()
  }, [loadAll])

  const saveOption = async (key: string, value: string) => {
    try {
      await api.put('/api/option/', { key, value })
      setOptions((prev) => ({ ...prev, [key]: value }))
      message.success(`已保存配置项：${key}`)
      void refreshBootstrap()
      if (key.startsWith('ChannelAffinity')) {
        const aff = await api.get<AffinityStats>('/api/option/channel_affinity_cache')
        setAffinityStats(aff)
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : `保存 ${key} 失败`)
    }
  }

  const runSelfCheck = async () => {
    setCheckingStatus(true)
    try {
      const res = await api.get<StatusTestResult>('/api/status/test')
      setStatusCheck(res)
      message.success(res.success ? '数据库 / Redis / 渠道缓存全部健康' : '存在异常检测项')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '自检请求失败')
    } finally {
      setCheckingStatus(false)
    }
  }

  const boolOpt = (key: string, def = false) =>
    options[key] !== undefined ? options[key] === 'true' : def

  return (
    <Card
      title="系统全局设置 (Root)"
      extra={
        <Button icon={<RefreshCw size={14} />} loading={loading} onClick={() => void loadAll()}>
          刷新全部
        </Button>
      }
      style={{ borderRadius: 12 }}
    >
      <Tabs
        items={[
          {
            key: 'operation',
            label: (
              <Space size={6}>
                <Sliders size={15} />
                <span>转发与会话亲和</span>
              </Space>
            ),
            children: (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
                <Card size="small" title="网关转发核心策略">
                  <Row gutter={[16, 16]}>
                    <Col xs={24} md={12}>
                      <Typography.Text strong>服务对外地址 (ServerAddress)</Typography.Text>
                      <Space.Compact style={{ width: '100%', marginTop: 6 }}>
                        <Input
                          value={options.ServerAddress ?? ''}
                          placeholder="例如 https://api.example.com"
                          onChange={(e) =>
                            setOptions((p) => ({ ...p, ServerAddress: e.target.value }))
                          }
                        />
                        <Button
                          type="primary"
                          icon={<Save size={14} />}
                          onClick={() => void saveOption('ServerAddress', options.ServerAddress ?? '')}
                        >
                          保存
                        </Button>
                      </Space.Compact>
                    </Col>

                    <Col xs={12} md={6}>
                      <Typography.Text strong>原样透传请求体</Typography.Text>
                      <div style={{ marginTop: 8 }}>
                        <Switch
                          checked={boolOpt('PassThroughRequestEnabled')}
                          onChange={(v) => void saveOption('PassThroughRequestEnabled', String(v))}
                        />
                      </div>
                    </Col>

                    <Col xs={12} md={6}>
                      <Typography.Text strong>流式保活 Ping</Typography.Text>
                      <div style={{ marginTop: 8 }}>
                        <Space>
                          <Switch
                            checked={boolOpt('PingIntervalEnabled')}
                            onChange={(v) => void saveOption('PingIntervalEnabled', String(v))}
                          />
                          <InputNumber
                            size="small"
                            min={5}
                            max={600}
                            value={Number(options.PingIntervalSeconds || 60)}
                            addonAfter="s"
                            onChange={(v) =>
                              v && void saveOption('PingIntervalSeconds', String(v))
                            }
                          />
                        </Space>
                      </div>
                    </Col>
                  </Row>

                  <Divider style={{ margin: '16px 0' }} />

                  <Row gutter={[16, 16]}>
                    <Col xs={24} md={12}>
                      <Typography.Text strong>
                        保留推理后缀模型列表 (ThinkingModelBlacklist JSON 数组)
                      </Typography.Text>
                      <Input.TextArea
                        rows={3}
                        style={{ marginTop: 6, fontFamily: 'monospace' }}
                        value={options.ThinkingModelBlacklist ?? '["moonshotai/kimi-k2-thinking"]'}
                        onChange={(e) =>
                          setOptions((p) => ({ ...p, ThinkingModelBlacklist: e.target.value }))
                        }
                      />
                      <Button
                        size="small"
                        type="primary"
                        style={{ marginTop: 6 }}
                        onClick={() =>
                          void saveOption(
                            'ThinkingModelBlacklist',
                            options.ThinkingModelBlacklist ?? '[]',
                          )
                        }
                      >
                        保存列表
                      </Button>
                    </Col>

                    <Col xs={24} md={12}>
                      <Typography.Text strong>
                        阿里百炼同步生图模型列表 (QwenSyncImageModels JSON 数组)
                      </Typography.Text>
                      <Input.TextArea
                        rows={3}
                        style={{ marginTop: 6, fontFamily: 'monospace' }}
                        value={options.QwenSyncImageModels ?? '["qwen-image","wan2.6"]'}
                        onChange={(e) =>
                          setOptions((p) => ({ ...p, QwenSyncImageModels: e.target.value }))
                        }
                      />
                      <Button
                        size="small"
                        type="primary"
                        style={{ marginTop: 6 }}
                        onClick={() =>
                          void saveOption(
                            'QwenSyncImageModels',
                            options.QwenSyncImageModels ?? '[]',
                          )
                        }
                      >
                        保存列表
                      </Button>
                    </Col>
                  </Row>
                </Card>

                <Card
                  size="small"
                  title="跨节点渠道会话亲和性 (Session Affinity)"
                  extra={
                    <Popconfirm
                      title="确认清空全部会话亲和性缓存？"
                      onConfirm={async () => {
                        await api.del('/api/option/channel_affinity_cache')
                        message.success('亲和性缓存已清空')
                        void loadAll()
                      }}
                    >
                      <Button size="small" danger icon={<Trash2 size={13} />}>
                        清空亲和缓存
                      </Button>
                    </Popconfirm>
                  }
                >
                  <Row gutter={[16, 16]} align="middle" style={{ marginBottom: 16 }}>
                    <Col xs={24} sm={8}>
                      <Space>
                        <Typography.Text strong>开启会话亲和路由：</Typography.Text>
                        <Switch
                          checked={boolOpt('ChannelAffinityEnabled', affinityStats?.enabled)}
                          onChange={(v) => void saveOption('ChannelAffinityEnabled', String(v))}
                        />
                      </Space>
                    </Col>
                    <Col xs={12} sm={8}>
                      <Space>
                        <Typography.Text>TTL (秒)：</Typography.Text>
                        <InputNumber
                          min={10}
                          max={86400}
                          value={Number(
                            options.ChannelAffinityTTLSeconds ||
                              affinityStats?.ttl_seconds ||
                              300,
                          )}
                          onChange={(v) =>
                            v && void saveOption('ChannelAffinityTTLSeconds', String(v))
                          }
                        />
                      </Space>
                    </Col>
                    <Col xs={12} sm={8}>
                      <Space>
                        <Typography.Text>单机回退上限：</Typography.Text>
                        <InputNumber
                          min={100}
                          max={1000000}
                          value={Number(
                            options.ChannelAffinityMaxEntries ||
                              affinityStats?.max_entries ||
                              100000,
                          )}
                          onChange={(v) =>
                            v && void saveOption('ChannelAffinityMaxEntries', String(v))
                          }
                        />
                      </Space>
                    </Col>
                  </Row>

                  {affinityStats && (
                    <Row gutter={[12, 12]}>
                      <Col xs={12} sm={6}>
                        <Statistic title="当前活跃会话数" value={affinityStats.entries} />
                      </Col>
                      <Col xs={12} sm={6}>
                        <Statistic title="亲和命中次数 (Hits)" value={affinityStats.hits} />
                      </Col>
                      <Col xs={12} sm={6}>
                        <Statistic title="首次选路未命中 (Misses)" value={affinityStats.misses} />
                      </Col>
                      <Col xs={12} sm={6}>
                        <Statistic
                          title="总命中率"
                          value={((affinityStats.hit_rate || 0) * 100).toFixed(1)}
                          suffix="%"
                        />
                      </Col>
                    </Row>
                  )}
                </Card>
              </div>
            ),
          },
          {
            key: 'content',
            label: (
              <Space size={6}>
                <FileText size={15} />
                <span>站点文案配置</span>
              </Space>
            ),
            children: (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <Typography.Text type="secondary">
                    修改后将通过 Redis Pub/Sub 实时同步至集群所有节点。
                  </Typography.Text>
                  <Button
                    size="small"
                    onClick={async () => {
                      await api.post('/api/option/migrate_console_setting', {})
                      message.success('旧版控制台配置检查与迁移已完成')
                      void loadAll()
                    }}
                  >
                    迁移旧版控制台配置
                  </Button>
                </div>

                {[
                  { key: 'Notice', label: '系统弹窗公告 (Notice)' },
                  { key: 'HomePageContent', label: '首页自定义内容 (HomePageContent)' },
                  { key: 'About', label: '关于页文案 (About)' },
                  { key: 'UserAgreement', label: '用户服务协议 (UserAgreement)' },
                  { key: 'PrivacyPolicy', label: '隐私政策 (PrivacyPolicy)' },
                ].map((item) => (
                  <Card size="small" key={item.key} title={item.label}>
                    <Input.TextArea
                      rows={3}
                      value={options[item.key] ?? ''}
                      onChange={(e) =>
                        setOptions((p) => ({ ...p, [item.key]: e.target.value }))
                      }
                    />
                    <Button
                      size="small"
                      type="primary"
                      style={{ marginTop: 8 }}
                      onClick={() => void saveOption(item.key, options[item.key] ?? '')}
                    >
                      保存 {item.key}
                    </Button>
                  </Card>
                ))}
              </div>
            ),
          },
          {
            key: 'security',
            label: (
              <Space size={6}>
                <Shield size={15} />
                <span>安全验证与请求限流</span>
              </Space>
            ),
            children: (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                <Card size="small" title="Cloudflare Turnstile 人机验证">
                  <Space style={{ marginBottom: 12 }}>
                    <Typography.Text strong>启用登录 Turnstile 校验：</Typography.Text>
                    <Switch
                      checked={boolOpt('TurnstileCheckEnabled')}
                      onChange={(v) => void saveOption('TurnstileCheckEnabled', String(v))}
                    />
                  </Space>
                  <Row gutter={[16, 12]}>
                    <Col xs={24} md={12}>
                      <Typography.Text>Site Key</Typography.Text>
                      <Space.Compact style={{ width: '100%', marginTop: 4 }}>
                        <Input
                          value={options.TurnstileSiteKey ?? ''}
                          onChange={(e) =>
                            setOptions((p) => ({ ...p, TurnstileSiteKey: e.target.value }))
                          }
                        />
                        <Button
                          onClick={() =>
                            void saveOption('TurnstileSiteKey', options.TurnstileSiteKey ?? '')
                          }
                        >
                          保存
                        </Button>
                      </Space.Compact>
                    </Col>
                    <Col xs={24} md={12}>
                      <Typography.Text>Secret Key（敏感字段已脱敏，输入新值后保存）</Typography.Text>
                      <Space.Compact style={{ width: '100%', marginTop: 4 }}>
                        <Input.Password
                          value={options.TurnstileSecretKey ?? ''}
                          onChange={(e) =>
                            setOptions((p) => ({ ...p, TurnstileSecretKey: e.target.value }))
                          }
                        />
                        <Button
                          onClick={() =>
                            void saveOption(
                              'TurnstileSecretKey',
                              options.TurnstileSecretKey ?? '',
                            )
                          }
                        >
                          保存
                        </Button>
                      </Space.Compact>
                    </Col>
                  </Row>
                </Card>

                <Card size="small" title="用户级模型请求速率限制 (Redis 集群共享滑窗)">
                  <Space style={{ marginBottom: 12 }}>
                    <Typography.Text strong>启用按用户模型请求限流：</Typography.Text>
                    <Switch
                      checked={boolOpt('ModelRequestRateLimitEnabled')}
                      onChange={(v) => void saveOption('ModelRequestRateLimitEnabled', String(v))}
                    />
                  </Space>
                  <Row gutter={[16, 12]}>
                    <Col xs={24} sm={8}>
                      <Typography.Text>统计周期 (分钟)</Typography.Text>
                      <InputNumber
                        style={{ width: '100%', marginTop: 4 }}
                        min={1}
                        value={Number(options.ModelRequestRateLimitDurationMinutes || 1)}
                        onChange={(v) =>
                          v &&
                          void saveOption('ModelRequestRateLimitDurationMinutes', String(v))
                        }
                      />
                    </Col>
                    <Col xs={24} sm={8}>
                      <Typography.Text>周期最大总请求数</Typography.Text>
                      <InputNumber
                        style={{ width: '100%', marginTop: 4 }}
                        min={1}
                        value={Number(options.ModelRequestRateLimitCount || 60)}
                        onChange={(v) =>
                          v && void saveOption('ModelRequestRateLimitCount', String(v))
                        }
                      />
                    </Col>
                    <Col xs={24} sm={8}>
                      <Typography.Text>周期最大成功请求数</Typography.Text>
                      <InputNumber
                        style={{ width: '100%', marginTop: 4 }}
                        min={0}
                        value={Number(options.ModelRequestRateLimitSuccessCount || 0)}
                        onChange={(v) =>
                          v !== null &&
                          void saveOption('ModelRequestRateLimitSuccessCount', String(v))
                        }
                      />
                    </Col>
                  </Row>
                </Card>
              </div>
            ),
          },
          {
            key: 'performance',
            label: (
              <Space size={6}>
                <Activity size={15} />
                <span>性能监控与集群自检</span>
              </Space>
            ),
            children: (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                <Card
                  size="small"
                  title="基础设施连通性一键自检 (/api/status/test)"
                  extra={
                    <Button
                      type="primary"
                      size="small"
                      loading={checkingStatus}
                      onClick={() => void runSelfCheck()}
                    >
                      立即执行自检
                    </Button>
                  }
                >
                  {statusCheck ? (
                    <Space wrap size={12}>
                      {statusCheck.items?.map((it) => (
                        <Tag
                          key={it.name}
                          color={it.success ? 'success' : 'error'}
                          style={{ padding: '4px 10px' }}
                        >
                          <Badge
                            status={it.success ? 'success' : 'error'}
                            text={`${it.name}: ${it.success ? '正常' : it.message || '异常'}`}
                          />
                        </Tag>
                      ))}
                    </Space>
                  ) : (
                    <Alert
                      type="info"
                      message="点击右上角「立即执行自检」可实时检测主数据库、Redis 集群与本地渠道缓存健康度。"
                    />
                  )}
                </Card>

                <Card
                  size="small"
                  title={
                    <Space>
                      <Cpu size={16} />
                      <span>节点运行时性能与磁盘缓存监控</span>
                    </Space>
                  }
                  extra={
                    <Space wrap>
                      <Button
                        size="small"
                        onClick={async () => {
                          await api.post('/api/performance/reset_stats')
                          message.success('运行时统计已重置')
                          void loadAll()
                        }}
                      >
                        重置统计
                      </Button>
                      <Button
                        size="small"
                        onClick={async () => {
                          await api.post('/api/performance/gc')
                          message.success('已触发手动 GC')
                          void loadAll()
                        }}
                      >
                        强制 GC
                      </Button>
                      <Button
                        size="small"
                        danger
                        onClick={async () => {
                          await api.del('/api/performance/disk_cache')
                          message.success('磁盘缓存已清理')
                          void loadAll()
                        }}
                      >
                        清理磁盘缓存
                      </Button>
                    </Space>
                  }
                >
                  <pre className="code-block" style={{ maxHeight: 420 }}>
                    {JSON.stringify(perfStats ?? {}, null, 2)}
                  </pre>
                </Card>
              </div>
            ),
          },
        ]}
      />
    </Card>
  )
}

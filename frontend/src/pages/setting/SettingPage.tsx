import React, { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  App,
  Badge,
  Button,
  Card,
  Checkbox,
  Col,
  Descriptions,
  Divider,
  Input,
  InputNumber,
  Popconfirm,
  Progress,
  Radio,
  Row,
  Select,
  Space,
  Statistic,
  Switch,
  Tabs,
  Tag,
  Typography,
} from 'antd'
import {
  Activity,
  FileText,
  GlobeLock,
  RefreshCw,
  Save,
  Shield,
  Sliders,
  Trash2,
} from 'lucide-react'
import { api } from '../../api/client'
import { useApp } from '../../contexts/AppContext'
import type { AffinityStats, PerformanceStats, StatusTestResult } from '../../types/api'

function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return '0 Bytes'
  const units = ['Bytes', 'KB', 'MB', 'GB', 'TB']
  const idx = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const val = bytes / Math.pow(1024, idx)
  return `${val >= 100 ? val.toFixed(0) : val >= 10 ? val.toFixed(1) : val.toFixed(2)} ${units[idx]}`
}

function parseJsonStringArray(raw: string | undefined, fallback: string[] = []): string[] {
  if (raw === undefined) return fallback
  const trimmed = raw.trim()
  if (!trimmed) return []
  try {
    const parsed = JSON.parse(trimmed)
    if (Array.isArray(parsed)) {
      return parsed.map((v) => String(v).trim()).filter(Boolean)
    }
  } catch {
    return trimmed
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
  }
  return fallback
}

interface SSRFFormState {
  enable_ssrf_protection: boolean
  allow_private_ip: boolean
  apply_ip_filter_for_domain: boolean
  domain_filter_mode: boolean
  ip_filter_mode: boolean
  domain_list: string[]
  ip_list: string[]
  allowed_ports: string[]
}

interface PerfFormState {
  disk_cache_enabled: boolean
  disk_cache_threshold_mb: number
  disk_cache_max_size_mb: number
  disk_cache_path: string
  monitor_enabled: boolean
  monitor_cpu_threshold: number
  monitor_memory_threshold: number
  monitor_disk_threshold: number
  admission_enabled: boolean
  admission_soft_limit: number
  admission_hard_limit: number
  admission_max_queue_size: number
  admission_max_wait_ms: number
  admission_priority_whitelist: string
}

const DEFAULT_ALLOWED_PORTS = ['80', '443', '8080', '8443']

export const SettingPage: React.FC = () => {
  const { refreshBootstrap } = useApp()
  const { message } = App.useApp()

  const [options, setOptions] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(false)
  const [savingSSRF, setSavingSSRF] = useState(false)
  const [savingPerf, setSavingPerf] = useState(false)
  const [refreshingPerf, setRefreshingPerf] = useState(false)
  const [affinityStats, setAffinityStats] = useState<AffinityStats | null>(null)
  const [perfStats, setPerfStats] = useState<PerformanceStats | null>(null)
  const [statusCheck, setStatusCheck] = useState<StatusTestResult | null>(null)
  const [checkingStatus, setCheckingStatus] = useState(false)

  const [ssrfForm, setSsrfForm] = useState<SSRFFormState>({
    enable_ssrf_protection: false,
    allow_private_ip: false,
    apply_ip_filter_for_domain: false,
    domain_filter_mode: false,
    ip_filter_mode: false,
    domain_list: [],
    ip_list: [],
    allowed_ports: DEFAULT_ALLOWED_PORTS,
  })

  const [perfForm, setPerfForm] = useState<PerfFormState>({
    disk_cache_enabled: false,
    disk_cache_threshold_mb: 10,
    disk_cache_max_size_mb: 1024,
    disk_cache_path: '',
    monitor_enabled: false,
    monitor_cpu_threshold: 90,
    monitor_memory_threshold: 90,
    monitor_disk_threshold: 90,
    admission_enabled: false,
    admission_soft_limit: 64,
    admission_hard_limit: 128,
    admission_max_queue_size: 256,
    admission_max_wait_ms: 30000,
    admission_priority_whitelist: '',
  })

  const syncFormsFromData = useCallback(
    (optMap: Record<string, string>, perf: PerformanceStats | null) => {
      const boolFromOpt = (k: string, fallback: boolean) =>
        optMap[k] !== undefined ? optMap[k] === 'true' : fallback
      const numFromOpt = (k: string, fallback: number) => {
        if (optMap[k] !== undefined && optMap[k] !== '') {
          const parsed = Number(optMap[k])
          if (!Number.isNaN(parsed)) return parsed
        }
        return fallback
      }

      const ssrf = perf?.ssrf_config
      setSsrfForm({
        enable_ssrf_protection: boolFromOpt(
          'fetch_setting.enable_ssrf_protection',
          ssrf?.enabled ?? false,
        ),
        allow_private_ip: boolFromOpt(
          'fetch_setting.allow_private_ip',
          ssrf?.allow_private_ip ?? false,
        ),
        apply_ip_filter_for_domain: boolFromOpt(
          'fetch_setting.apply_ip_filter_for_domain',
          ssrf?.apply_ip_filter_for_domain ?? false,
        ),
        domain_filter_mode: boolFromOpt(
          'fetch_setting.domain_filter_mode',
          ssrf?.domain_filter_mode ?? false,
        ),
        ip_filter_mode: boolFromOpt(
          'fetch_setting.ip_filter_mode',
          ssrf?.ip_filter_mode ?? false,
        ),
        domain_list: parseJsonStringArray(
          optMap['fetch_setting.domain_list'],
          ssrf?.domain_list ?? [],
        ),
        ip_list: parseJsonStringArray(optMap['fetch_setting.ip_list'], ssrf?.ip_list ?? []),
        allowed_ports: parseJsonStringArray(
          optMap['fetch_setting.allowed_ports'],
          ssrf?.allowed_ports ?? DEFAULT_ALLOWED_PORTS,
        ),
      })

      const cfg = perf?.config
      const adm = perf?.admission_stats
      setPerfForm({
        disk_cache_enabled: boolFromOpt(
          'performance_setting.disk_cache_enabled',
          cfg?.disk_cache_enabled ?? false,
        ),
        disk_cache_threshold_mb: numFromOpt(
          'performance_setting.disk_cache_threshold_mb',
          cfg?.disk_cache_threshold_mb ?? 10,
        ),
        disk_cache_max_size_mb: numFromOpt(
          'performance_setting.disk_cache_max_size_mb',
          cfg?.disk_cache_max_size_mb ?? 1024,
        ),
        disk_cache_path:
          optMap['performance_setting.disk_cache_path'] ?? cfg?.disk_cache_path ?? '',
        monitor_enabled: boolFromOpt(
          'performance_setting.monitor_enabled',
          cfg?.monitor_enabled ?? false,
        ),
        monitor_cpu_threshold: numFromOpt(
          'performance_setting.monitor_cpu_threshold',
          cfg?.monitor_cpu_threshold ?? 90,
        ),
        monitor_memory_threshold: numFromOpt(
          'performance_setting.monitor_memory_threshold',
          cfg?.monitor_memory_threshold ?? 90,
        ),
        monitor_disk_threshold: numFromOpt(
          'performance_setting.monitor_disk_threshold',
          cfg?.monitor_disk_threshold ?? 90,
        ),
        admission_enabled: boolFromOpt(
          'performance_setting.admission_enabled',
          adm?.enabled ?? false,
        ),
        admission_soft_limit: numFromOpt(
          'performance_setting.admission_soft_limit',
          adm?.soft_limit ?? 64,
        ),
        admission_hard_limit: numFromOpt(
          'performance_setting.admission_hard_limit',
          adm?.hard_limit ?? 128,
        ),
        admission_max_queue_size: numFromOpt(
          'performance_setting.admission_max_queue_size',
          adm?.max_queue_size ?? 256,
        ),
        admission_max_wait_ms: numFromOpt(
          'performance_setting.admission_max_wait_ms',
          adm?.max_wait_ms ?? 30000,
        ),
        admission_priority_whitelist:
          optMap['performance_setting.admission_priority_whitelist'] ??
          adm?.priority_whitelist ??
          '',
      })
    },
    [],
  )

  const loadAll = useCallback(async () => {
    setLoading(true)
    try {
      const [optMap, aff, perf] = await Promise.all([
        api.get<Record<string, string>>('/api/option/'),
        api.get<AffinityStats>('/api/option/channel_affinity_cache'),
        api.get<PerformanceStats>('/api/performance/stats'),
      ])
      const normalizedOpts = optMap || {}
      setOptions(normalizedOpts)
      setAffinityStats(aff)
      setPerfStats(perf)
      syncFormsFromData(normalizedOpts, perf)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载系统配置失败')
    } finally {
      setLoading(false)
    }
  }, [message, syncFormsFromData])

  const refreshPerfStatsOnly = useCallback(async () => {
    setRefreshingPerf(true)
    try {
      const perf = await api.get<PerformanceStats>('/api/performance/stats')
      setPerfStats(perf)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '刷新性能统计失败')
    } finally {
      setRefreshingPerf(false)
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

  const saveBatchOptions = async (entries: Record<string, string>, successMsg: string) => {
    for (const [key, value] of Object.entries(entries)) {
      await api.put('/api/option/', { key, value })
    }
    setOptions((prev) => ({ ...prev, ...entries }))
    const perf = await api.get<PerformanceStats>('/api/performance/stats')
    setPerfStats(perf)
    message.success(successMsg)
  }

  const handleSaveSSRF = async () => {
    setSavingSSRF(true)
    try {
      await saveBatchOptions(
        {
          'fetch_setting.enable_ssrf_protection': String(ssrfForm.enable_ssrf_protection),
          'fetch_setting.allow_private_ip': String(ssrfForm.allow_private_ip),
          'fetch_setting.apply_ip_filter_for_domain': String(ssrfForm.apply_ip_filter_for_domain),
          'fetch_setting.domain_filter_mode': String(ssrfForm.domain_filter_mode),
          'fetch_setting.ip_filter_mode': String(ssrfForm.ip_filter_mode),
          'fetch_setting.domain_list': JSON.stringify(ssrfForm.domain_list),
          'fetch_setting.ip_list': JSON.stringify(ssrfForm.ip_list),
          'fetch_setting.allowed_ports': JSON.stringify(ssrfForm.allowed_ports),
        },
        'SSRF防护设置已更新并同步至全集群',
      )
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存 SSRF 防护设置失败')
    } finally {
      setSavingSSRF(false)
    }
  }

  const handleSavePerformance = async () => {
    if (perfForm.admission_hard_limit < perfForm.admission_soft_limit) {
      message.warning('硬水位不能小于软水位')
      return
    }
    setSavingPerf(true)
    try {
      await saveBatchOptions(
        {
          'performance_setting.disk_cache_enabled': String(perfForm.disk_cache_enabled),
          'performance_setting.disk_cache_threshold_mb': String(perfForm.disk_cache_threshold_mb),
          'performance_setting.disk_cache_max_size_mb': String(perfForm.disk_cache_max_size_mb),
          'performance_setting.disk_cache_path': perfForm.disk_cache_path.trim(),
          'performance_setting.monitor_enabled': String(perfForm.monitor_enabled),
          'performance_setting.monitor_cpu_threshold': String(perfForm.monitor_cpu_threshold),
          'performance_setting.monitor_memory_threshold': String(
            perfForm.monitor_memory_threshold,
          ),
          'performance_setting.monitor_disk_threshold': String(perfForm.monitor_disk_threshold),
          'performance_setting.admission_enabled': String(perfForm.admission_enabled),
          'performance_setting.admission_soft_limit': String(perfForm.admission_soft_limit),
          'performance_setting.admission_hard_limit': String(perfForm.admission_hard_limit),
          'performance_setting.admission_max_queue_size': String(
            perfForm.admission_max_queue_size,
          ),
          'performance_setting.admission_max_wait_ms': String(perfForm.admission_max_wait_ms),
          'performance_setting.admission_priority_whitelist':
            perfForm.admission_priority_whitelist.trim(),
        },
        '性能设置已保存并热更新生效',
      )
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存性能设置失败')
    } finally {
      setSavingPerf(false)
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

  // 性能看板计算指标
  const adm = perfStats?.admission_stats
  const hardWatermark = Math.max(adm?.hard_limit || perfForm.admission_hard_limit || 128, 1)
  const admPercent = Math.min(100, Number((((adm?.in_flight ?? 0) / hardWatermark) * 100).toFixed(1)))

  const cacheStats = perfStats?.cache_stats
  const diskMaxBytes =
    cacheStats?.disk_cache_max_bytes || perfForm.disk_cache_max_size_mb * 1024 * 1024 || 1
  const diskUsageBytes = cacheStats?.current_disk_usage_bytes ?? 0
  const diskCachePercent = Math.min(
    100,
    Math.round((diskUsageBytes / Math.max(diskMaxBytes, 1)) * 100),
  )

  const diskSpace = perfStats?.disk_space_info
  const diskSpaceText =
    diskSpace && diskSpace.total > 0
      ? `可用空间: ${formatBytes(diskSpace.free)} / 总空间: ${formatBytes(diskSpace.total)}`
      : '可用空间: 自动检测中'

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
                          onClick={() =>
                            void saveOption('ServerAddress', options.ServerAddress ?? '')
                          }
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
                        value={
                          options.ThinkingModelBlacklist ?? '["moonshotai/kimi-k2-thinking"]'
                        }
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
            key: 'ssrf',
            label: (
              <Space size={6}>
                <GlobeLock size={15} />
                <span>SSRF防护设置</span>
              </Space>
            ),
            children: (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
                <div>
                  <Typography.Title level={5} style={{ margin: 0 }}>
                    SSRF防护设置
                  </Typography.Title>
                  <Typography.Text type="secondary">
                    配置服务器端请求伪造(SSRF)防护，用于保护内网资源安全
                  </Typography.Text>
                </div>

                <div>
                  <Checkbox
                    checked={ssrfForm.enable_ssrf_protection}
                    onChange={(e) =>
                      setSsrfForm((prev) => ({
                        ...prev,
                        enable_ssrf_protection: e.target.checked,
                      }))
                    }
                  >
                    <Typography.Text strong>
                      启用SSRF防护（推荐开启以保护服务器安全）
                    </Typography.Text>
                  </Checkbox>
                  <div style={{ paddingLeft: 24, marginTop: 4 }}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      总开关控制是否启用SSRF防护功能。关闭后将跳过所有SSRF检查，允许访问任意URL。⚠️
                      仅在完全信任环境中关闭此功能。
                    </Typography.Text>
                  </div>
                </div>

                <div>
                  <Checkbox
                    disabled={!ssrfForm.enable_ssrf_protection}
                    checked={ssrfForm.allow_private_ip}
                    onChange={(e) =>
                      setSsrfForm((prev) => ({ ...prev, allow_private_ip: e.target.checked }))
                    }
                  >
                    <Typography.Text>
                      允许访问私有IP地址（127.0.0.1、192.168.x.x等内网地址）
                    </Typography.Text>
                  </Checkbox>
                  <div style={{ paddingLeft: 24, marginTop: 4 }}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      ⚠️
                      安全警告：启用此选项将允许访问内网资源（本地主机、私有网络）。仅在需要访问内部服务且了解安全风险的情况下启用。
                    </Typography.Text>
                  </div>
                </div>

                <div>
                  <Checkbox
                    disabled={!ssrfForm.enable_ssrf_protection}
                    checked={ssrfForm.apply_ip_filter_for_domain}
                    onChange={(e) =>
                      setSsrfForm((prev) => ({
                        ...prev,
                        apply_ip_filter_for_domain: e.target.checked,
                      }))
                    }
                  >
                    <Typography.Text>对域名启用 IP 过滤（实验性）</Typography.Text>
                  </Checkbox>
                  <div style={{ paddingLeft: 24, marginTop: 4 }}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      ⚠️ 此功能为实验性选项。域名可能解析到多个 IPv4/IPv6 地址，若开启，请确保 IP
                      过滤列表覆盖这些地址，否则可能导致访问失败。
                    </Typography.Text>
                  </div>
                </div>

                <div>
                  <Typography.Text strong>
                    {ssrfForm.domain_filter_mode ? '域名白名单' : '域名黑名单'}
                  </Typography.Text>
                  <div>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      支持通配符格式，如：example.com, *.api.example.com
                    </Typography.Text>
                  </div>
                  <div style={{ margin: '8px 0' }}>
                    <Radio.Group
                      disabled={!ssrfForm.enable_ssrf_protection}
                      value={ssrfForm.domain_filter_mode}
                      onChange={(e) =>
                        setSsrfForm((prev) => ({ ...prev, domain_filter_mode: e.target.value }))
                      }
                    >
                      <Radio value={true}>白名单</Radio>
                      <Radio value={false}>黑名单</Radio>
                    </Radio.Group>
                  </div>
                  <Select
                    mode="tags"
                    open={false}
                    disabled={!ssrfForm.enable_ssrf_protection}
                    style={{ width: '100%' }}
                    placeholder="输入域名后回车，如：example.com"
                    tokenSeparators={[',', ' ']}
                    value={ssrfForm.domain_list}
                    onChange={(vals) => setSsrfForm((prev) => ({ ...prev, domain_list: vals }))}
                  />
                </div>

                <div>
                  <Typography.Text strong>
                    {ssrfForm.ip_filter_mode ? 'IP白名单' : 'IP黑名单'}
                  </Typography.Text>
                  <div>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      支持CIDR格式，如：8.8.8.8, 192.168.1.0/24
                    </Typography.Text>
                  </div>
                  <div style={{ margin: '8px 0' }}>
                    <Radio.Group
                      disabled={!ssrfForm.enable_ssrf_protection}
                      value={ssrfForm.ip_filter_mode}
                      onChange={(e) =>
                        setSsrfForm((prev) => ({ ...prev, ip_filter_mode: e.target.value }))
                      }
                    >
                      <Radio value={true}>白名单</Radio>
                      <Radio value={false}>黑名单</Radio>
                    </Radio.Group>
                  </div>
                  <Select
                    mode="tags"
                    open={false}
                    disabled={!ssrfForm.enable_ssrf_protection}
                    style={{ width: '100%' }}
                    placeholder="输入IP地址后回车，如：8.8.8.8"
                    tokenSeparators={[',', ' ']}
                    value={ssrfForm.ip_list}
                    onChange={(vals) => setSsrfForm((prev) => ({ ...prev, ip_list: vals }))}
                  />
                </div>

                <div>
                  <Typography.Text strong>允许的端口</Typography.Text>
                  <div>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      支持单个端口和端口范围，如：80, 443, 8000-8999
                    </Typography.Text>
                  </div>
                  <Select
                    mode="tags"
                    open={false}
                    disabled={!ssrfForm.enable_ssrf_protection}
                    style={{ width: '100%', marginTop: 8 }}
                    placeholder="输入端口后回车，如：80, 443, 8000-8999"
                    tokenSeparators={[',', ' ']}
                    value={ssrfForm.allowed_ports}
                    onChange={(vals) => setSsrfForm((prev) => ({ ...prev, allowed_ports: vals }))}
                  />
                  <div style={{ marginTop: 4 }}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      限制外部请求只能访问指定端口。支持单个端口（80, 443）或端口范围（8000-8999）。空列表允许所有端口。默认包含常用Web端口。
                    </Typography.Text>
                  </div>
                </div>

                <div>
                  <Button
                    type="default"
                    loading={savingSSRF}
                    onClick={() => void handleSaveSSRF()}
                  >
                    更新SSRF防护设置
                  </Button>
                </div>
              </div>
            ),
          },
          {
            key: 'performance',
            label: (
              <Space size={6}>
                <Activity size={15} />
                <span>性能设置</span>
              </Space>
            ),
            children: (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
                {/* ==================== 1. 磁盘缓存设置（磁盘换内存） ==================== */}
                <div>
                  <Typography.Title level={5} style={{ marginTop: 0, marginBottom: 12 }}>
                    磁盘缓存设置（磁盘换内存）
                  </Typography.Title>
                  <Alert
                    type="info"
                    showIcon
                    style={{ marginBottom: 16 }}
                    message="启用磁盘缓存后，大请求体将临时存储到磁盘而非内存，可显著降低内存占用，适用于处理包含大量图片/文件的请求。建议在 SSD 环境下使用。"
                  />
                  <Row gutter={[20, 16]}>
                    <Col xs={24} sm={8}>
                      <Typography.Text>启用磁盘缓存</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <Switch
                          checked={perfForm.disk_cache_enabled}
                          onChange={(v) =>
                            setPerfForm((prev) => ({ ...prev, disk_cache_enabled: v }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        将大请求体临时存储到磁盘
                      </Typography.Text>
                    </Col>

                    <Col xs={24} sm={8}>
                      <Typography.Text>磁盘缓存阈值 (MB)</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <InputNumber
                          min={1}
                          max={2048}
                          style={{ width: '100%' }}
                          disabled={!perfForm.disk_cache_enabled}
                          value={perfForm.disk_cache_threshold_mb}
                          onChange={(v) =>
                            setPerfForm((prev) => ({
                              ...prev,
                              disk_cache_threshold_mb: v ?? 10,
                            }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        请求体超过此大小时使用磁盘缓存
                      </Typography.Text>
                    </Col>

                    <Col xs={24} sm={8}>
                      <Typography.Text>磁盘缓存最大总量 (MB)</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <InputNumber
                          min={10}
                          max={1048576}
                          style={{ width: '100%' }}
                          disabled={!perfForm.disk_cache_enabled}
                          value={perfForm.disk_cache_max_size_mb}
                          onChange={(v) =>
                            setPerfForm((prev) => ({
                              ...prev,
                              disk_cache_max_size_mb: v ?? 1024,
                            }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        {diskSpaceText}
                      </Typography.Text>
                    </Col>

                    {!perfStats?.config?.is_running_in_container && (
                      <Col xs={24} sm={8}>
                        <Typography.Text>缓存目录</Typography.Text>
                        <div style={{ margin: '8px 0 4px' }}>
                          <Input
                            placeholder="例如 /var/cache/app"
                            disabled={!perfForm.disk_cache_enabled}
                            value={perfForm.disk_cache_path}
                            onChange={(e) =>
                              setPerfForm((prev) => ({
                                ...prev,
                                disk_cache_path: e.target.value,
                              }))
                            }
                          />
                        </div>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                          留空使用系统临时目录
                        </Typography.Text>
                      </Col>
                    )}
                  </Row>
                </div>

                {/* ==================== 2. 系统性能监控 ==================== */}
                <div>
                  <Typography.Title level={5} style={{ marginTop: 0, marginBottom: 12 }}>
                    系统性能监控
                  </Typography.Title>
                  <Alert
                    type="info"
                    showIcon
                    style={{ marginBottom: 16 }}
                    message="启用性能监控后，当系统资源使用率超过设定阈值时，将拒绝新的 Relay 请求 (/v1, /v1beta 等)，以保护系统稳定性。"
                  />
                  <Row gutter={[20, 16]}>
                    <Col xs={24} sm={6}>
                      <Typography.Text>启用性能监控</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <Switch
                          checked={perfForm.monitor_enabled}
                          onChange={(v) =>
                            setPerfForm((prev) => ({ ...prev, monitor_enabled: v }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        超过阈值时拒绝新请求
                      </Typography.Text>
                    </Col>

                    <Col xs={24} sm={6}>
                      <Typography.Text>CPU 阈值 (%)</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <InputNumber
                          min={0}
                          max={100}
                          style={{ width: '100%' }}
                          disabled={!perfForm.monitor_enabled}
                          value={perfForm.monitor_cpu_threshold}
                          onChange={(v) =>
                            setPerfForm((prev) => ({
                              ...prev,
                              monitor_cpu_threshold: v ?? 90,
                            }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        CPU 使用率超过此值时拒绝请求
                      </Typography.Text>
                    </Col>

                    <Col xs={24} sm={6}>
                      <Typography.Text>内存 阈值 (%)</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <InputNumber
                          min={0}
                          max={100}
                          style={{ width: '100%' }}
                          disabled={!perfForm.monitor_enabled}
                          value={perfForm.monitor_memory_threshold}
                          onChange={(v) =>
                            setPerfForm((prev) => ({
                              ...prev,
                              monitor_memory_threshold: v ?? 90,
                            }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        内存使用率超过此值时拒绝请求
                      </Typography.Text>
                    </Col>

                    <Col xs={24} sm={6}>
                      <Typography.Text>磁盘 阈值 (%)</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <InputNumber
                          min={0}
                          max={100}
                          style={{ width: '100%' }}
                          disabled={!perfForm.monitor_enabled}
                          value={perfForm.monitor_disk_threshold}
                          onChange={(v) =>
                            setPerfForm((prev) => ({
                              ...prev,
                              monitor_disk_threshold: v ?? 90,
                            }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        磁盘使用率超过此值时拒绝请求
                      </Typography.Text>
                    </Col>
                  </Row>
                </div>

                {/* ==================== 3. 准入控制与平滑背压 ==================== */}
                <div>
                  <Typography.Title level={5} style={{ marginTop: 0, marginBottom: 12 }}>
                    准入控制与平滑背压
                  </Typography.Title>
                  <Alert
                    type="info"
                    showIcon
                    style={{ marginBottom: 16 }}
                    message="启用后按「在途请求数」做平滑背压：未达软水位直接放行；达到软水位后普通请求排队、高优先分组直接放行；达到硬水位后普通请求快速拒绝、仅高优先分组排队。排队超过等待时间或队列已满时返回 503。"
                  />
                  <Row gutter={[20, 16]}>
                    <Col xs={24} sm={6}>
                      <Typography.Text>启用准入控制</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <Switch
                          checked={perfForm.admission_enabled}
                          onChange={(v) =>
                            setPerfForm((prev) => ({ ...prev, admission_enabled: v }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        在途请求过高时排队/降级而非直接拒绝
                      </Typography.Text>
                    </Col>

                    <Col xs={24} sm={6}>
                      <Typography.Text>软水位 (在途请求数)</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <InputNumber
                          min={1}
                          max={100000}
                          style={{ width: '100%' }}
                          disabled={!perfForm.admission_enabled}
                          value={perfForm.admission_soft_limit}
                          onChange={(v) =>
                            setPerfForm((prev) => ({
                              ...prev,
                              admission_soft_limit: v ?? 64,
                            }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        超过后普通请求开始排队
                      </Typography.Text>
                    </Col>

                    <Col xs={24} sm={6}>
                      <Typography.Text>硬水位 (在途请求数)</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <InputNumber
                          min={1}
                          max={100000}
                          style={{ width: '100%' }}
                          disabled={!perfForm.admission_enabled}
                          value={perfForm.admission_hard_limit}
                          onChange={(v) =>
                            setPerfForm((prev) => ({
                              ...prev,
                              admission_hard_limit: v ?? 128,
                            }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        超过后普通请求快速拒绝，高优先仍排队
                      </Typography.Text>
                    </Col>

                    <Col xs={24} sm={6}>
                      <Typography.Text>等待队列上限</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <InputNumber
                          min={0}
                          max={1000000}
                          style={{ width: '100%' }}
                          disabled={!perfForm.admission_enabled}
                          value={perfForm.admission_max_queue_size}
                          onChange={(v) =>
                            setPerfForm((prev) => ({
                              ...prev,
                              admission_max_queue_size: v ?? 256,
                            }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        0 表示不限制
                      </Typography.Text>
                    </Col>

                    <Col xs={24} sm={6}>
                      <Typography.Text>排队最大等待 (ms)</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <InputNumber
                          min={100}
                          max={600000}
                          style={{ width: '100%' }}
                          disabled={!perfForm.admission_enabled}
                          value={perfForm.admission_max_wait_ms}
                          onChange={(v) =>
                            setPerfForm((prev) => ({
                              ...prev,
                              admission_max_wait_ms: v ?? 30000,
                            }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        超时后返回 503
                      </Typography.Text>
                    </Col>

                    <Col xs={24} sm={12}>
                      <Typography.Text>高优先分组白名单</Typography.Text>
                      <div style={{ margin: '8px 0 4px' }}>
                        <Input
                          placeholder="例如 svip,vip"
                          disabled={!perfForm.admission_enabled}
                          value={perfForm.admission_priority_whitelist}
                          onChange={(e) =>
                            setPerfForm((prev) => ({
                              ...prev,
                              admission_priority_whitelist: e.target.value,
                            }))
                          }
                        />
                      </div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        英文逗号分隔，命中的分组/用户/令牌请求优先放行，例如 svip,vip
                      </Typography.Text>
                    </Col>
                  </Row>

                  <div style={{ marginTop: 18 }}>
                    <Button
                      type="default"
                      loading={savingPerf}
                      onClick={() => void handleSavePerformance()}
                    >
                      保存性能设置
                    </Button>
                  </div>
                </div>

                <Divider style={{ margin: '4px 0' }} />

                {/* ==================== 4. 性能监控实时看板 ==================== */}
                <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <Typography.Title level={5} style={{ margin: 0 }}>
                    性能监控
                  </Typography.Title>

                  <Space wrap>
                    <Button
                      loading={refreshingPerf}
                      onClick={() => void refreshPerfStatsOnly()}
                    >
                      刷新统计
                    </Button>
                    <Popconfirm
                      title="确认清理超过 10 分钟未活跃的磁盘缓存文件？"
                      onConfirm={async () => {
                        await api.del('/api/performance/disk_cache')
                        message.success('已清理不活跃磁盘缓存')
                        void refreshPerfStatsOnly()
                      }}
                    >
                      <Button danger>清理不活跃缓存</Button>
                    </Popconfirm>
                    <Button
                      onClick={async () => {
                        await api.post('/api/performance/reset_stats')
                        message.success('累计统计计数已重置')
                        void refreshPerfStatsOnly()
                      }}
                    >
                      重置统计
                    </Button>
                    <Button
                      onClick={async () => {
                        await api.post('/api/performance/gc')
                        message.success('已触发手动 GC')
                        void refreshPerfStatsOnly()
                      }}
                    >
                      执行 GC
                    </Button>
                    <Button
                      loading={checkingStatus}
                      onClick={() => void runSelfCheck()}
                    >
                      基础设施自检
                    </Button>
                  </Space>

                  {statusCheck && (
                    <Space wrap size={10}>
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
                  )}

                  {/* 准入控制卡片 */}
                  <Card
                    size="small"
                    style={{ background: '#fafafa', borderRadius: 10 }}
                    styles={{ body: { padding: '16px 20px' } }}
                  >
                    <Space size={8} style={{ marginBottom: 8 }}>
                      <Typography.Text strong>准入控制</Typography.Text>
                      <Tag color={adm?.enabled ? 'success' : 'default'}>
                        {adm?.enabled ? '已启用' : '未启用'}
                      </Tag>
                    </Space>
                    <Progress
                      percent={admPercent}
                      status={admPercent >= 100 ? 'exception' : 'normal'}
                      format={(p) => `${(p ?? 0).toFixed(1)}%`}
                    />
                    <div
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        flexWrap: 'wrap',
                        gap: 12,
                        margin: '6px 0 12px',
                      }}
                    >
                      <Typography.Text type="secondary">
                        在途请求: {adm?.in_flight ?? 0}
                      </Typography.Text>
                      <Typography.Text type="secondary">
                        软水位: {adm?.soft_limit ?? perfForm.admission_soft_limit}
                      </Typography.Text>
                      <Typography.Text type="secondary">
                        硬水位: {adm?.hard_limit ?? perfForm.admission_hard_limit}
                      </Typography.Text>
                      <Typography.Text type="secondary">
                        排队中: {adm?.queued ?? 0}
                      </Typography.Text>
                    </div>
                    <Space wrap size={8}>
                      <Tag color="blue">累计放行: {adm?.total_admitted ?? 0}</Tag>
                      <Tag color="gold">累计排队: {adm?.total_queued ?? 0}</Tag>
                      <Tag color="red">累计拒绝: {adm?.total_rejected ?? 0}</Tag>
                      <Tag color="orange">排队超时: {adm?.total_timeout ?? 0}</Tag>
                    </Space>
                  </Card>

                  {/* 请求体磁盘缓存 + 请求体内存缓存 */}
                  <Row gutter={[16, 16]}>
                    <Col xs={24} md={12}>
                      <Card
                        size="small"
                        style={{ background: '#fafafa', borderRadius: 10, height: '100%' }}
                        styles={{ body: { padding: '16px 20px' } }}
                      >
                        <Typography.Text strong style={{ display: 'block', marginBottom: 8 }}>
                          请求体磁盘缓存
                        </Typography.Text>
                        <Progress percent={diskCachePercent} />
                        <div
                          style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            margin: '6px 0 12px',
                          }}
                        >
                          <Typography.Text type="secondary">
                            {formatBytes(diskUsageBytes)} / {formatBytes(diskMaxBytes)}
                          </Typography.Text>
                          <Typography.Text type="secondary">
                            活跃文件: {cacheStats?.active_disk_files ?? 0}
                          </Typography.Text>
                        </div>
                        <Tag color="blue">磁盘命中: {cacheStats?.disk_cache_hits ?? 0}</Tag>
                      </Card>
                    </Col>

                    <Col xs={24} md={12}>
                      <Card
                        size="small"
                        style={{ background: '#fafafa', borderRadius: 10, height: '100%' }}
                        styles={{ body: { padding: '16px 20px' } }}
                      >
                        <Typography.Text strong style={{ display: 'block', marginBottom: 14 }}>
                          请求体内存缓存
                        </Typography.Text>
                        <div
                          style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            marginBottom: 26,
                          }}
                        >
                          <Typography.Text>
                            当前缓存大小: {formatBytes(cacheStats?.current_memory_usage_bytes)}
                          </Typography.Text>
                          <Typography.Text>
                            活跃缓存数: {cacheStats?.active_memory_buffers ?? 0}
                          </Typography.Text>
                        </div>
                        <Tag color="green">内存命中: {cacheStats?.memory_cache_hits ?? 0}</Tag>
                      </Card>
                    </Col>
                  </Row>

                  {/* 节点系统资源与 Go 运行时指标 */}
                  <Card
                    size="small"
                    title="节点系统资源与 Go 运行时"
                    style={{ background: '#fafafa', borderRadius: 10 }}
                  >
                    <Descriptions size="small" column={{ xs: 1, sm: 2, md: 4 }}>
                      <Descriptions.Item label="CPU 使用率">
                        {(perfStats?.system_status?.cpu_usage ?? 0).toFixed(1)}%
                      </Descriptions.Item>
                      <Descriptions.Item label="内存使用率">
                        {(perfStats?.system_status?.memory_usage ?? 0).toFixed(1)}%
                      </Descriptions.Item>
                      <Descriptions.Item label="磁盘使用率">
                        {(perfStats?.system_status?.disk_usage ?? 0).toFixed(1)}%
                      </Descriptions.Item>
                      <Descriptions.Item label="Goroutine 数">
                        {perfStats?.memory_stats?.num_goroutine ?? 0}
                      </Descriptions.Item>
                      <Descriptions.Item label="Go 堆内存 (Alloc)">
                        {formatBytes(perfStats?.memory_stats?.alloc)}
                      </Descriptions.Item>
                      <Descriptions.Item label="系统分配内存 (Sys)">
                        {formatBytes(perfStats?.memory_stats?.sys)}
                      </Descriptions.Item>
                      <Descriptions.Item label="GC 累计次数">
                        {perfStats?.memory_stats?.num_gc ?? 0}
                      </Descriptions.Item>
                      <Descriptions.Item label="磁盘缓存目录">
                        <Typography.Text ellipsis style={{ maxWidth: 220 }}>
                          {perfStats?.disk_cache_info?.path || '-'}
                        </Typography.Text>
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>
                </div>
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
                <div
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                  }}
                >
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
        ]}
      />
    </Card>
  )
}

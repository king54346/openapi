import React, { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Col,
  DatePicker,
  Drawer,
  Input,
  Modal,
  Popconfirm,
  Row,
  Segmented,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  App,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { Eye, KeyRound, RefreshCw, Search, Trash2 } from 'lucide-react'
import { api } from '../../api/client'
import { formatUnixTime, LOG_TYPES } from '../../constants/channel'
import { useApp } from '../../contexts/AppContext'
import type { AffinityUsageStat, LogItem, LogStat, PageData } from '../../types/api'

export const LogPage: React.FC = () => {
  const { isAdmin } = useApp()
  const { message } = App.useApp()

  const [scope, setScope] = useState<'all' | 'self'>(isAdmin ? 'all' : 'self')
  const [loading, setLoading] = useState(false)
  const [items, setItems] = useState<LogItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [stat, setStat] = useState<LogStat | null>(null)
  const [affinityUsage, setAffinityUsage] = useState<AffinityUsageStat[]>([])

  // 过滤项
  const [modelName, setModelName] = useState('')
  const [tokenName, setTokenName] = useState('')
  const [username, setUsername] = useState('')
  const [channelId, setChannelId] = useState('')
  const [requestId, setRequestId] = useState('')
  const [logType, setLogType] = useState<number | undefined>(undefined)
  const [timeRange, setTimeRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null] | null>(null)

  // 详情抽屉 & 按 Key 免登录查询弹窗
  const [detailLog, setDetailLog] = useState<LogItem | null>(null)
  const [keyModalOpen, setKeyModalOpen] = useState(false)
  const [queryKey, setQueryKey] = useState('')
  const [keyLogs, setKeyLogs] = useState<LogItem[]>([])
  const [keyLoading, setKeyLoading] = useState(false)

  const buildFilterParams = useCallback(() => {
    return {
      p: page,
      page_size: pageSize,
      model_name: modelName.trim() || undefined,
      token_name: tokenName.trim() || undefined,
      username: scope === 'all' ? username.trim() || undefined : undefined,
      channel: scope === 'all' ? channelId.trim() || undefined : undefined,
      request_id: requestId.trim() || undefined,
      type: logType,
      start_timestamp: timeRange?.[0] ? timeRange[0].unix() : undefined,
      end_timestamp: timeRange?.[1] ? timeRange[1].unix() : undefined,
    }
  }, [channelId, logType, modelName, page, pageSize, requestId, scope, timeRange, tokenName, username])

  const loadLogs = useCallback(async () => {
    setLoading(true)
    try {
      const params = buildFilterParams()
      const listPath = scope === 'all' && isAdmin ? '/api/log/' : '/api/log/self'
      const statPath = scope === 'all' && isAdmin ? '/api/log/stat' : '/api/log/self/stat'

      const [pageData, statData] = await Promise.all([
        api.get<PageData<LogItem>>(listPath, params),
        api.get<LogStat>(statPath, params),
      ])
      setItems(pageData.items || [])
      setTotal(pageData.total || 0)
      setStat(statData)

      if (isAdmin && scope === 'all') {
        api
          .get<AffinityUsageStat[]>('/api/log/channel_affinity_usage_cache')
          .then((u) => setAffinityUsage(u || []))
          .catch(() => setAffinityUsage([]))
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载日志失败')
    } finally {
      setLoading(false)
    }
  }, [buildFilterParams, isAdmin, message, scope])

  useEffect(() => {
    void loadLogs()
  }, [loadLogs])

  const handleClearHistory = async () => {
    // 默认清理 30 天之前的历史日志
    const targetTs = dayjs().subtract(30, 'day').unix()
    try {
      const count = await api.del<number>('/api/log/', { target_timestamp: targetTs })
      message.success(`已清理 30 天前的日志共 ${count} 条`)
      void loadLogs()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '清理日志失败')
    }
  }

  const handleQueryByKey = async () => {
    if (!queryKey.trim()) {
      message.warning('请输入 sk-... 令牌密钥')
      return
    }
    setKeyLoading(true)
    try {
      const res = await api.get<PageData<LogItem>>('/api/log/token', {
        key: queryKey.trim(),
        p: 1,
        page_size: 20,
      })
      setKeyLogs(res.items || [])
    } catch (err) {
      message.error(err instanceof Error ? err.message : '查询失败')
    } finally {
      setKeyLoading(false)
    }
  }

  const columns: ColumnsType<LogItem> = [
    {
      title: '时间',
      dataIndex: 'created_at',
      width: 170,
      render: (v: number) => formatUnixTime(v),
    },
    ...(isAdmin && scope === 'all'
      ? [
          {
            title: '用户',
            dataIndex: 'username',
            width: 110,
          },
          {
            title: '渠道',
            width: 140,
            render: (_: unknown, r: LogItem) =>
              r.channel ? (
                <Tag color="geekblue">
                  #{r.channel} {r.channel_name || ''}
                </Tag>
              ) : (
                '-'
              ),
          },
        ]
      : []),
    {
      title: '类型',
      dataIndex: 'type',
      width: 95,
      render: (t: number) => {
        const info = LOG_TYPES[t] || { label: `类型${t}`, color: 'default' }
        return <Tag color={info.color}>{info.label}</Tag>
      },
    },
    {
      title: '模型',
      dataIndex: 'model_name',
      width: 190,
      render: (m: string, r) =>
        m ? (
          <Space size={4}>
            <Tag color="blue">{m}</Tag>
            {r.is_stream && <Tag color="cyan">流</Tag>}
          </Space>
        ) : (
          '-'
        ),
    },
    {
      title: '令牌名',
      dataIndex: 'token_name',
      width: 130,
      render: (v: string) => v || '-',
    },
    {
      title: '提示 / 补全 Tokens',
      width: 150,
      render: (_, r) => `${r.prompt_tokens || 0} / ${r.completion_tokens || 0}`,
    },
    {
      title: '耗时',
      dataIndex: 'use_time',
      width: 90,
      render: (v: number) => (v > 0 ? `${v}s` : '<1s'),
    },
    {
      title: '详情摘要',
      dataIndex: 'content',
      ellipsis: true,
    },
    {
      title: '查看',
      width: 70,
      fixed: 'right',
      render: (_, r) => (
        <Button
          type="text"
          size="small"
          icon={<Eye size={15} />}
          onClick={() => setDetailLog(r)}
        />
      ),
    },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Row gutter={[12, 12]}>
        <Col xs={24} sm={8}>
          <Card size="small" style={{ borderRadius: 12 }}>
            <Statistic title="当前筛选累计 Quota" value={stat?.quota ?? 0} />
          </Card>
        </Col>
        <Col xs={12} sm={8}>
          <Card size="small" style={{ borderRadius: 12 }}>
            <Statistic title="实时 RPM" value={stat?.rpm ?? 0} />
          </Card>
        </Col>
        <Col xs={12} sm={8}>
          <Card size="small" style={{ borderRadius: 12 }}>
            <Statistic title="实时 TPM" value={stat?.tpm ?? 0} />
          </Card>
        </Col>
      </Row>

      {isAdmin && scope === 'all' && affinityUsage.length > 0 && (
        <Card size="small" title="会话亲和性各渠道命中分布" style={{ borderRadius: 12 }}>
          <Space wrap>
            {affinityUsage.map((item) => (
              <Tag key={item.channel_id} color="purple">
                渠道 #{item.channel_id}：{item.count} 次命中
              </Tag>
            ))}
          </Space>
        </Card>
      )}

      <Card style={{ borderRadius: 12 }}>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            flexWrap: 'wrap',
            gap: 10,
            marginBottom: 14,
          }}
        >
          <Space wrap>
            {isAdmin && (
              <Segmented
                value={scope}
                onChange={(v) => {
                  setPage(1)
                  setScope(v as 'all' | 'self')
                }}
                options={[
                  { value: 'all', label: '全站日志' },
                  { value: 'self', label: '仅我的日志' },
                ]}
              />
            )}
            <Select
              allowClear
              placeholder="日志类型"
              style={{ width: 120 }}
              value={logType}
              onChange={(v) => setLogType(v)}
              options={Object.entries(LOG_TYPES).map(([k, v]) => ({
                value: Number(k),
                label: v.label,
              }))}
            />
            <Input
              allowClear
              placeholder="模型名称"
              style={{ width: 150 }}
              value={modelName}
              onChange={(e) => setModelName(e.target.value)}
            />
            <Input
              allowClear
              placeholder="令牌名称"
              style={{ width: 130 }}
              value={tokenName}
              onChange={(e) => setTokenName(e.target.value)}
            />
            {isAdmin && scope === 'all' && (
              <>
                <Input
                  allowClear
                  placeholder="用户名"
                  style={{ width: 120 }}
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                />
                <Input
                  allowClear
                  placeholder="渠道 ID"
                  style={{ width: 100 }}
                  value={channelId}
                  onChange={(e) => setChannelId(e.target.value)}
                />
              </>
            )}
            <Input
              allowClear
              placeholder="Request ID"
              style={{ width: 160 }}
              value={requestId}
              onChange={(e) => setRequestId(e.target.value)}
            />
            <DatePicker.RangePicker
              showTime
              value={timeRange}
              onChange={(vals) => setTimeRange(vals)}
            />
            <Button
              type="primary"
              icon={<Search size={14} />}
              onClick={() => {
                setPage(1)
                void loadLogs()
              }}
            >
              筛选
            </Button>
          </Space>

          <Space wrap>
            <Button icon={<KeyRound size={14} />} onClick={() => setKeyModalOpen(true)}>
              按 API Key 查日志
            </Button>
            {isAdmin && (
              <Popconfirm
                title="确认清理 30 天之前的历史日志？"
                onConfirm={() => void handleClearHistory()}
              >
                <Button danger icon={<Trash2 size={14} />}>
                  清理30天前日志
                </Button>
              </Popconfirm>
            )}
            <Button icon={<RefreshCw size={14} />} onClick={() => void loadLogs()} />
          </Space>
        </div>

        <Table<LogItem>
          rowKey={(r) => `${r.id}_${r.created_at}_${r.request_id || ''}`}
          loading={loading}
          columns={columns}
          dataSource={items}
          scroll={{ x: 1180 }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条记录`,
            onChange: (p, ps) => {
              setPage(p)
              setPageSize(ps)
            },
          }}
        />
      </Card>

      {/* 日志扩展 JSON 详情 */}
      <Drawer
        title="日志详情"
        width={560}
        open={Boolean(detailLog)}
        onClose={() => setDetailLog(null)}
      >
        {detailLog && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div>
              <Typography.Text type="secondary">Request ID：</Typography.Text>
              <Typography.Text code>{detailLog.request_id || '-'}</Typography.Text>
            </div>
            <div>
              <Typography.Text type="secondary">IP 地址：</Typography.Text>
              <Typography.Text>{detailLog.ip || '-'}</Typography.Text>
            </div>
            <div>
              <Typography.Text type="secondary">内容描述：</Typography.Text>
              <div className="code-block" style={{ marginTop: 6 }}>
                {detailLog.content || '-'}
              </div>
            </div>
            {detailLog.other && (
              <div>
                <Typography.Text type="secondary">附加元数据 (Other JSON)：</Typography.Text>
                <pre className="code-block" style={{ marginTop: 6 }}>
                  {(() => {
                    try {
                      return JSON.stringify(JSON.parse(detailLog.other), null, 2)
                    } catch {
                      return detailLog.other
                    }
                  })()}
                </pre>
              </div>
            )}
          </div>
        )}
      </Drawer>

      {/* 按 API Key 查询弹窗 */}
      <Modal
        title="按 API Key 查询近期调用日志"
        open={keyModalOpen}
        onCancel={() => setKeyModalOpen(false)}
        footer={null}
        width={760}
      >
        <Space style={{ marginBottom: 14, width: '100%' }}>
          <Input.Password
            style={{ width: 420 }}
            placeholder="输入完整 sk-... 令牌密钥"
            value={queryKey}
            onChange={(e) => setQueryKey(e.target.value)}
          />
          <Button type="primary" loading={keyLoading} onClick={() => void handleQueryByKey()}>
            立即查询
          </Button>
        </Space>
        <Table<LogItem>
          size="small"
          rowKey={(r) => `${r.id}_${r.created_at}`}
          loading={keyLoading}
          dataSource={keyLogs}
          pagination={false}
          columns={[
            { title: '时间', dataIndex: 'created_at', width: 165, render: (v: number) => formatUnixTime(v) },
            { title: '模型', dataIndex: 'model_name', width: 160 },
            {
              title: 'Tokens',
              width: 130,
              render: (_, r) => `${r.prompt_tokens} / ${r.completion_tokens}`,
            },
            { title: '详情', dataIndex: 'content', ellipsis: true },
          ]}
        />
      </Modal>
    </div>
  )
}

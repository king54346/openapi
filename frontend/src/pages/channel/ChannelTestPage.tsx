import React, { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Input,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  App,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { Play, RefreshCw, RotateCcw, Search, Trash2 } from 'lucide-react'
import { api } from '../../api/client'
import { formatUnixTime, getChannelTypeInfo } from '../../constants/channel'
import type { ChannelTestRecord, PageData } from '../../types/api'

export const ChannelTestPage: React.FC = () => {
  const { message } = App.useApp()
  const [loading, setLoading] = useState(false)
  const [records, setRecords] = useState<ChannelTestRecord[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([])

  const [channelId, setChannelId] = useState('')
  const [channelTag, setChannelTag] = useState('')
  const [modelName, setModelName] = useState('')
  const [status, setStatus] = useState<number | undefined>(undefined)
  const [retesting, setRetesting] = useState(false)

  const buildFilter = useCallback(
    () => ({
      channel_id: channelId.trim() ? Number(channelId.trim()) : undefined,
      channel_tag: channelTag.trim() || undefined,
      model_name: modelName.trim() || undefined,
      status,
    }),
    [channelId, channelTag, modelName, status],
  )

  const loadRecords = useCallback(async () => {
    setLoading(true)
    try {
      const res = await api.get<PageData<ChannelTestRecord>>('/api/channel/test_records', {
        p: page,
        page_size: pageSize,
        ...buildFilter(),
      })
      setRecords(res.items || [])
      setTotal(res.total || 0)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载测速记录失败')
    } finally {
      setLoading(false)
    }
  }, [buildFilter, message, page, pageSize])

  useEffect(() => {
    void loadRecords()
  }, [loadRecords])

  const handleRetestOne = async (id: number) => {
    setRetesting(true)
    try {
      await api.post(`/api/channel/test_records/${id}/retest`)
      message.success('单条重测完成')
      void loadRecords()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '重测失败')
    } finally {
      setRetesting(false)
    }
  }

  const handleBatchRetest = async () => {
    if (selectedRowKeys.length === 0) return
    setRetesting(true)
    try {
      const res = await api.post<{ total: number; passed: number; failed: number }>(
        '/api/channel/test_records/batch_retest',
        { ids: selectedRowKeys.map(Number) },
      )
      message.success(`批量重测完成：通过 ${res.passed} / 失败 ${res.failed}`)
      setSelectedRowKeys([])
      void loadRecords()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '批量重测失败')
    } finally {
      setRetesting(false)
    }
  }

  const handleRetestFiltered = async () => {
    setRetesting(true)
    try {
      const res = await api.post<{ total: number; passed: number; failed: number }>(
        '/api/channel/test_records/retest_filtered',
        buildFilter(),
      )
      message.success(`按筛选重测完成 (${res.total} 条)：通过 ${res.passed}，失败 ${res.failed}`)
      void loadRecords()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '筛选重测失败')
    } finally {
      setRetesting(false)
    }
  }

  const handleClearAll = async () => {
    try {
      const count = await api.del<number>('/api/channel/test_records/all')
      message.success(`已清空全部测速记录 (${count ?? 0} 条)`)
      void loadRecords()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '清空失败')
    }
  }

  const columns: ColumnsType<ChannelTestRecord> = [
    {
      title: '测试时间',
      dataIndex: 'tested_at',
      width: 170,
      render: (v: number) => formatUnixTime(v),
    },
    {
      title: '渠道',
      width: 200,
      render: (_, r) => {
        const info = getChannelTypeInfo(r.channel_type)
        return (
          <Space size={4} wrap>
            <Tag color={info.color}>#{r.channel_id}</Tag>
            <Typography.Text strong>{r.channel_name}</Typography.Text>
            {r.channel_tag && <Tag color="cyan">{r.channel_tag}</Tag>}
          </Space>
        )
      },
    },
    {
      title: '测试模型',
      dataIndex: 'model_name',
      width: 200,
      render: (m: string) => <Tag color="blue">{m}</Tag>,
    },
    {
      title: '结果',
      dataIndex: 'status',
      width: 95,
      render: (s: number) =>
        s === 1 ? <Tag color="success">通过</Tag> : <Tag color="error">失败</Tag>,
    },
    {
      title: '耗时',
      dataIndex: 'response_time_ms',
      width: 105,
      render: (ms: number) => (ms > 0 ? `${ms} ms` : '-'),
    },
    {
      title: '错误信息',
      dataIndex: 'error_message',
      ellipsis: true,
      render: (v?: string) => v || '-',
    },
    {
      title: '重测',
      width: 85,
      fixed: 'right',
      render: (_, r) => (
        <Button
          type="link"
          size="small"
          loading={retesting}
          icon={<RotateCcw size={13} />}
          onClick={() => void handleRetestOne(r.id)}
        >
          重测
        </Button>
      ),
    },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
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
            <Input
              allowClear
              placeholder="渠道 ID"
              style={{ width: 110 }}
              value={channelId}
              onChange={(e) => setChannelId(e.target.value)}
            />
            <Input
              allowClear
              placeholder="渠道标签"
              style={{ width: 130 }}
              value={channelTag}
              onChange={(e) => setChannelTag(e.target.value)}
            />
            <Input
              allowClear
              placeholder="模型名称"
              style={{ width: 180 }}
              value={modelName}
              onChange={(e) => setModelName(e.target.value)}
            />
            <Select
              allowClear
              placeholder="状态"
              style={{ width: 115 }}
              value={status}
              onChange={setStatus}
              options={[
                { value: 1, label: '通过' },
                { value: 0, label: '失败' },
              ]}
            />
            <Button
              type="primary"
              icon={<Search size={14} />}
              onClick={() => {
                setPage(1)
                void loadRecords()
              }}
            >
              筛选
            </Button>
          </Space>

          <Space wrap>
            {selectedRowKeys.length > 0 && (
              <Button
                type="primary"
                ghost
                loading={retesting}
                icon={<Play size={14} />}
                onClick={() => void handleBatchRetest()}
              >
                批量重测 ({selectedRowKeys.length})
              </Button>
            )}
            <Button
              loading={retesting}
              icon={<RotateCcw size={14} />}
              onClick={() => void handleRetestFiltered()}
            >
              重测当前筛选
            </Button>
            <Popconfirm title="确认清空所有测速记录？" onConfirm={() => void handleClearAll()}>
              <Button danger icon={<Trash2 size={14} />}>
                清空记录
              </Button>
            </Popconfirm>
            <Button icon={<RefreshCw size={14} />} onClick={() => void loadRecords()} />
          </Space>
        </div>

        <Table<ChannelTestRecord>
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={records}
          scroll={{ x: 1020 }}
          rowSelection={{ selectedRowKeys, onChange: setSelectedRowKeys }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条测速记录`,
            onChange: (p, ps) => {
              setPage(p)
              setPageSize(ps)
            },
          }}
        />
      </Card>
    </div>
  )
}

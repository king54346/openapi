import React, { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Drawer,
  Input,
  Progress,
  Segmented,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  App,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { Eye, RefreshCw, Search } from 'lucide-react'
import { api } from '../../api/client'
import { formatUnixTime } from '../../constants/channel'
import { useApp } from '../../contexts/AppContext'
import type { PageData, TaskItem } from '../../types/api'

const STATUS_COLOR: Record<string, string> = {
  SUCCESS: 'success',
  FAILURE: 'error',
  IN_PROGRESS: 'processing',
  QUEUED: 'warning',
  SUBMITTED: 'geekblue',
  NOT_START: 'default',
}

export const TaskPage: React.FC = () => {
  const { isAdmin } = useApp()
  const { message } = App.useApp()

  const [scope, setScope] = useState<'all' | 'self'>(isAdmin ? 'all' : 'self')
  const [loading, setLoading] = useState(false)
  const [items, setItems] = useState<TaskItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)

  const [taskId, setTaskId] = useState('')
  const [status, setStatus] = useState<string | undefined>(undefined)
  const [platform, setPlatform] = useState('')
  const [channelId, setChannelId] = useState('')
  const [selectedTask, setSelectedTask] = useState<TaskItem | null>(null)

  const loadTasks = useCallback(async () => {
    setLoading(true)
    try {
      const path = scope === 'all' && isAdmin ? '/api/task/' : '/api/task/self'
      const res = await api.get<PageData<TaskItem>>(path, {
        p: page,
        page_size: pageSize,
        task_id: taskId.trim() || undefined,
        status,
        platform: platform.trim() || undefined,
        channel_id: scope === 'all' ? channelId.trim() || undefined : undefined,
      })
      setItems(res.items || [])
      setTotal(res.total || 0)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载异步任务失败')
    } finally {
      setLoading(false)
    }
  }, [channelId, isAdmin, message, page, pageSize, platform, scope, status, taskId])

  useEffect(() => {
    void loadTasks()
  }, [loadTasks])

  const columns: ColumnsType<TaskItem> = [
    {
      title: '提交时间',
      dataIndex: 'submit_time',
      width: 170,
      render: (v: number, r) => formatUnixTime(v || r.created_at),
    },
    {
      title: '任务 ID',
      dataIndex: 'task_id',
      width: 210,
      render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
    },
    {
      title: '平台 / 动作',
      width: 150,
      render: (_, r) => (
        <Space size={4}>
          <Tag color="blue">{r.platform || '-'}</Tag>
          {r.action && <Tag>{r.action}</Tag>}
        </Space>
      ),
    },
    ...(isAdmin && scope === 'all'
      ? [
          {
            title: '渠道 ID',
            dataIndex: 'channel_id',
            width: 90,
            render: (v: number) => (v ? `#${v}` : '-'),
          },
        ]
      : []),
    {
      title: '模型',
      width: 170,
      render: (_, r) => r.properties?.origin_model_name || r.properties?.upstream_model_name || '-',
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 120,
      render: (s: string) => <Tag color={STATUS_COLOR[s] || 'default'}>{s}</Tag>,
    },
    {
      title: '进度',
      dataIndex: 'progress',
      width: 140,
      render: (p: string, r) => {
        const num = parseInt((p || '').replace('%', ''), 10)
        const pct = !Number.isNaN(num) ? num : r.status === 'SUCCESS' ? 100 : 0
        return <Progress percent={pct} size="small" />
      },
    },
    {
      title: '结果 / 原因',
      dataIndex: 'fail_reason',
      ellipsis: true,
      render: (v: string) => v || '-',
    },
    {
      title: '详情',
      width: 70,
      fixed: 'right',
      render: (_, r) => (
        <Button
          type="text"
          size="small"
          icon={<Eye size={15} />}
          onClick={() => setSelectedTask(r)}
        />
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
            {isAdmin && (
              <Segmented
                value={scope}
                onChange={(v) => {
                  setPage(1)
                  setScope(v as 'all' | 'self')
                }}
                options={[
                  { value: 'all', label: '全站任务' },
                  { value: 'self', label: '我的任务' },
                ]}
              />
            )}
            <Input
              allowClear
              placeholder="上游 Task ID"
              style={{ width: 190 }}
              value={taskId}
              onChange={(e) => setTaskId(e.target.value)}
            />
            <Select
              allowClear
              placeholder="任务状态"
              style={{ width: 140 }}
              value={status}
              onChange={(v) => setStatus(v)}
              options={[
                { value: 'SUBMITTED', label: 'SUBMITTED' },
                { value: 'QUEUED', label: 'QUEUED' },
                { value: 'IN_PROGRESS', label: 'IN_PROGRESS' },
                { value: 'SUCCESS', label: 'SUCCESS' },
                { value: 'FAILURE', label: 'FAILURE' },
              ]}
            />
            <Input
              allowClear
              placeholder="平台 (如 sora/doubao)"
              style={{ width: 155 }}
              value={platform}
              onChange={(e) => setPlatform(e.target.value)}
            />
            {isAdmin && scope === 'all' && (
              <Input
                allowClear
                placeholder="渠道 ID"
                style={{ width: 100 }}
                value={channelId}
                onChange={(e) => setChannelId(e.target.value)}
              />
            )}
            <Button
              type="primary"
              icon={<Search size={14} />}
              onClick={() => {
                setPage(1)
                void loadTasks()
              }}
            >
              筛选
            </Button>
          </Space>

          <Button icon={<RefreshCw size={14} />} onClick={() => void loadTasks()}>
            刷新
          </Button>
        </div>

        <Table<TaskItem>
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={items}
          scroll={{ x: 1100 }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 个异步任务`,
            onChange: (p, ps) => {
              setPage(p)
              setPageSize(ps)
            },
          }}
        />
      </Card>

      <Drawer
        title={`任务详情：${selectedTask?.task_id || ''}`}
        width={580}
        open={Boolean(selectedTask)}
        onClose={() => setSelectedTask(null)}
      >
        {selectedTask && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div>
              <Typography.Text type="secondary">输入提示词 (Prompt)：</Typography.Text>
              <div className="code-block" style={{ marginTop: 6 }}>
                {selectedTask.properties?.input || '-'}
              </div>
            </div>
            <div>
              <Typography.Text type="secondary">结果 / 错误信息：</Typography.Text>
              <div className="code-block" style={{ marginTop: 6 }}>
                {selectedTask.fail_reason || '-'}
              </div>
            </div>
            <div>
              <Typography.Text type="secondary">原始响应数据 (Data)：</Typography.Text>
              <pre className="code-block" style={{ marginTop: 6 }}>
                {JSON.stringify(selectedTask.data ?? {}, null, 2)}
              </pre>
            </div>
          </div>
        )}
      </Drawer>
    </div>
  )
}

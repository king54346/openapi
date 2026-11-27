import React, { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Drawer,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  App,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { Edit3, History, Play, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { api } from '../../api/client'
import { formatUnixTime } from '../../constants/channel'
import type { CronJob, CronJobRun, PageData } from '../../types/api'

const CRON_PRESETS = [
  { label: '每 5 分钟 (*/5 * * * *)', value: '*/5 * * * *' },
  { label: '每 30 分钟 (*/30 * * * *)', value: '*/30 * * * *' },
  { label: '每小时整点 (0 * * * *)', value: '0 * * * *' },
  { label: '每天凌晨 3 点 (0 3 * * *)', value: '0 3 * * *' },
]

export const CronJobPage: React.FC = () => {
  const { message } = App.useApp()
  const [loading, setLoading] = useState(false)
  const [jobs, setJobs] = useState<CronJob[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [jobTypes, setJobTypes] = useState<string[]>([])

  // 新建 / 编辑
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<CronJob | null>(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm()

  // 运行历史抽屉
  const [runsJob, setRunsJob] = useState<CronJob | null>(null)
  const [runs, setRuns] = useState<CronJobRun[]>([])
  const [runsLoading, setRunsLoading] = useState(false)

  const loadJobs = useCallback(async () => {
    setLoading(true)
    try {
      const [pageData, types] = await Promise.all([
        api.get<PageData<CronJob>>('/api/cron_job/', { p: page, page_size: pageSize }),
        api.get<string[]>('/api/cron_job/types'),
      ])
      setJobs(pageData.items || [])
      setTotal(pageData.total || 0)
      setJobTypes(types || [])
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载定时任务失败')
    } finally {
      setLoading(false)
    }
  }, [message, page, pageSize])

  useEffect(() => {
    void loadJobs()
  }, [loadJobs])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({
      name: '',
      type: jobTypes[0] || 'channel_test',
      cron: '0 * * * *',
      params: '{}',
      enabled: true,
    })
    setModalOpen(true)
  }

  const openEdit = (record: CronJob) => {
    setEditing(record)
    form.setFieldsValue({
      name: record.name,
      type: record.type,
      cron: record.cron,
      params: record.params || '{}',
      enabled: record.enabled,
    })
    setModalOpen(true)
  }

  const handleSave = async () => {
    const vals = await form.validateFields()
    setSaving(true)
    try {
      const payload = {
        name: vals.name.trim(),
        type: vals.type,
        cron: vals.cron.trim(),
        params: (vals.params || '').trim(),
        enabled: Boolean(vals.enabled),
      }
      if (editing) {
        await api.put(`/api/cron_job/${editing.id}`, payload)
        message.success('定时任务已更新')
      } else {
        await api.post('/api/cron_job/', payload)
        message.success('定时任务已创建')
      }
      setModalOpen(false)
      void loadJobs()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const toggleJob = async (job: CronJob, enabled: boolean) => {
    try {
      await api.post(`/api/cron_job/${job.id}/enabled`, { enabled })
      message.success(enabled ? '已启用定时调度' : '已暂停定时调度')
      void loadJobs()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '修改状态失败')
    }
  }

  const runNow = async (job: CronJob) => {
    try {
      await api.post(`/api/cron_job/${job.id}/run`)
      message.success('已触发立即执行，稍后可在执行历史查看结果')
      setTimeout(() => void loadJobs(), 800)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '触发失败')
    }
  }

  const openRuns = async (job: CronJob) => {
    setRunsJob(job)
    setRunsLoading(true)
    try {
      const res = await api.get<PageData<CronJobRun>>(`/api/cron_job/${job.id}/runs`, {
        p: 1,
        page_size: 30,
      })
      setRuns(res.items || [])
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载执行记录失败')
    } finally {
      setRunsLoading(false)
    }
  }

  const columns: ColumnsType<CronJob> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    {
      title: '任务名称',
      dataIndex: 'name',
      width: 190,
      render: (v: string) => <Typography.Text strong>{v}</Typography.Text>,
    },
    {
      title: '执行器类型',
      dataIndex: 'type',
      width: 150,
      render: (t: string) => <Tag color="geekblue">{t}</Tag>,
    },
    {
      title: 'Cron 表达式',
      dataIndex: 'cron',
      width: 150,
      render: (c: string) => <Typography.Text code>{c}</Typography.Text>,
    },
    {
      title: '启用状态',
      dataIndex: 'enabled',
      width: 100,
      render: (en: boolean, r) => (
        <Switch size="small" checked={en} onChange={(c) => void toggleJob(r, c)} />
      ),
    },
    {
      title: '最近执行',
      dataIndex: 'last_run_at',
      width: 170,
      render: (v?: number) => formatUnixTime(v),
    },
    {
      title: '最近摘要',
      dataIndex: 'last_message',
      ellipsis: true,
      render: (v?: string) => v || '-',
    },
    {
      title: '操作',
      width: 170,
      fixed: 'right',
      render: (_, r) => (
        <Space size={4}>
          <Tooltip title="立即运行一次">
            <Button
              type="text"
              size="small"
              icon={<Play size={15} />}
              onClick={() => void runNow(r)}
            />
          </Tooltip>
          <Tooltip title="执行历史">
            <Button
              type="text"
              size="small"
              icon={<History size={15} />}
              onClick={() => void openRuns(r)}
            />
          </Tooltip>
          <Tooltip title="编辑">
            <Button
              type="text"
              size="small"
              icon={<Edit3 size={15} />}
              onClick={() => openEdit(r)}
            />
          </Tooltip>
          <Popconfirm
            title="确认删除该定时任务？"
            onConfirm={async () => {
              await api.del(`/api/cron_job/${r.id}`)
              message.success('已删除')
              void loadJobs()
            }}
          >
            <Button type="text" size="small" danger icon={<Trash2 size={15} />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Alert
        type="info"
        showIcon
        message="多节点对等部署说明：启用 Redis 时，所有网关节点自动通过分布式锁协调 Cron 触发（同一分钟仅单节点执行并自动续租，节点下线由其余实例无缝接管）。"
      />

      <Card style={{ borderRadius: 12 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 14 }}>
          <Typography.Title level={5} style={{ margin: 0 }}>
            定时调度任务
          </Typography.Title>
          <Space>
            <Button icon={<RefreshCw size={14} />} onClick={() => void loadJobs()}>
              刷新
            </Button>
            <Button type="primary" icon={<Plus size={15} />} onClick={openCreate}>
              新建任务
            </Button>
          </Space>
        </div>

        <Table<CronJob>
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={jobs}
          scroll={{ x: 980 }}
          pagination={{
            current: page,
            pageSize,
            total,
            showTotal: (t) => `共 ${t} 个任务`,
            onChange: (p, ps) => {
              setPage(p)
              setPageSize(ps)
            },
          }}
        />
      </Card>

      <Modal
        title={editing ? `编辑定时任务 #${editing.id}` : '新建定时任务'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => void handleSave()}
        confirmLoading={saving}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="任务名称" rules={[{ required: true }]}>
            <Input placeholder="例如 每小时自动巡检测速" />
          </Form.Item>
          <Form.Item name="type" label="任务类型" rules={[{ required: true }]}>
            <Select options={jobTypes.map((t) => ({ value: t, label: t }))} />
          </Form.Item>
          <Form.Item
            name="cron"
            label={
              <Space>
                <span>标准 5 段 Cron 表达式</span>
                <Select
                  size="small"
                  placeholder="常用预设..."
                  style={{ width: 195 }}
                  options={CRON_PRESETS}
                  onChange={(v) => form.setFieldValue('cron', v)}
                />
              </Space>
            }
            rules={[{ required: true }]}
          >
            <Input placeholder="分 时 日 月 周，例如 0 * * * *" style={{ fontFamily: 'monospace' }} />
          </Form.Item>
          <Form.Item name="params" label="执行器参数 (JSON，可选)">
            <Input.TextArea rows={3} style={{ fontFamily: 'monospace' }} />
          </Form.Item>
          <Form.Item name="enabled" label="立即启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title={`执行历史：${runsJob?.name || ''}`}
        width={620}
        open={Boolean(runsJob)}
        onClose={() => setRunsJob(null)}
      >
        <Table<CronJobRun>
          size="small"
          rowKey="id"
          loading={runsLoading}
          dataSource={runs}
          pagination={false}
          columns={[
            {
              title: '时间',
              width: 165,
              render: (_, r) => formatUnixTime(r.started_at || r.created_at),
            },
            {
              title: '触发方式',
              width: 85,
              render: (_, r) => (r.manual ? <Tag color="blue">手动</Tag> : <Tag>定时</Tag>),
            },
            {
              title: '结果',
              width: 85,
              render: (_, r) =>
                r.success || r.status === 'success' ? (
                  <Tag color="success">成功</Tag>
                ) : (
                  <Tag color="error">失败</Tag>
                ),
            },
            {
              title: '耗时',
              width: 90,
              render: (_, r) => `${r.duration_ms ?? r.latency_ms ?? 0} ms`,
            },
            { title: '输出信息', dataIndex: 'message', ellipsis: true },
          ]}
        />
      </Drawer>
    </div>
  )
}

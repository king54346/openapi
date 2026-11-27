import React, { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Col,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Radio,
  Row,
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
import dayjs from 'dayjs'
import {
  BarChart3,
  BookTemplate,
  Copy,
  Edit3,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { api } from '../../api/client'
import { TokenAnalyticsModal } from '../../components/TokenAnalyticsModal'
import { formatUnixTime } from '../../constants/channel'
import type { PageData, Token, TokenAccessTemplate } from '../../types/api'

export const TokenPage: React.FC = () => {
  const { message } = App.useApp()
  const [loading, setLoading] = useState(false)
  const [data, setData] = useState<Token[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [keyword, setKeyword] = useState('')
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([])

  // 可选模型与访问模板
  const [availableModels, setAvailableModels] = useState<string[]>([])
  const [templates, setTemplates] = useState<TokenAccessTemplate[]>([])

  // 新建/编辑弹窗
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<Token | null>(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm()

  // 访问限制模板管理弹窗
  const [tplModalOpen, setTplModalOpen] = useState(false)
  const [tplForm] = Form.useForm()
  const [editingTpl, setEditingTpl] = useState<TokenAccessTemplate | null>(null)

  // 用量分析弹窗
  const [analyticsOpen, setAnalyticsOpen] = useState(false)
  const [analyticsToken, setAnalyticsToken] = useState<Token | null>(null)

  const loadTokens = useCallback(async () => {
    setLoading(true)
    try {
      const path = keyword.trim() ? '/api/token/search' : '/api/token/'
      const res = await api.get<PageData<Token>>(path, {
        p: page,
        page_size: pageSize,
        keyword: keyword.trim() || undefined,
      })
      setData(res.items || [])
      setTotal(res.total || 0)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载令牌失败')
    } finally {
      setLoading(false)
    }
  }, [keyword, message, page, pageSize])

  const loadMeta = useCallback(async () => {
    try {
      const [mList, tplPage] = await Promise.all([
        api.get<string[]>('/api/user/models'),
        api.get<PageData<TokenAccessTemplate>>('/api/token_template/', { p: 1, page_size: 100 }),
      ])
      setAvailableModels(mList || [])
      setTemplates(tplPage.items || [])
    } catch {
      // ignore
    }
  }, [])

  useEffect(() => {
    void loadTokens()
  }, [loadTokens])

  useEffect(() => {
    void loadMeta()
  }, [loadMeta])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({
      name: '',
      unlimited_quota: true,
      remain_quota: 500000,
      never_expire: true,
      model_limits_enabled: false,
      model_limits_type: 0,
      model_list: [],
      allow_ips: '',
      cross_group_retry: false,
    })
    setModalOpen(true)
  }

  const openEdit = (record: Token) => {
    setEditing(record)
    form.setFieldsValue({
      name: record.name,
      unlimited_quota: record.unlimited_quota,
      remain_quota: record.remain_quota,
      never_expire: record.expired_time === -1,
      expired_at: record.expired_time > 0 ? dayjs(record.expired_time * 1000) : undefined,
      model_limits_enabled: record.model_limits_enabled,
      model_limits_type: record.model_limits_type ?? 0,
      model_list: record.model_limits
        ? record.model_limits
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean)
        : [],
      allow_ips: record.allow_ips || '',
      cross_group_retry: record.cross_group_retry,
    })
    setModalOpen(true)
  }

  const applyTemplateToForm = (tplId: number) => {
    const tpl = templates.find((t) => t.id === tplId)
    if (!tpl) return
    form.setFieldsValue({
      model_limits_enabled: tpl.model_limits_enabled,
      model_limits_type: tpl.model_limits_type ?? 0,
      model_list: tpl.model_limits
        ? tpl.model_limits
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean)
        : [],
      allow_ips: tpl.allow_ips || '',
      cross_group_retry: Boolean(tpl.cross_group_retry),
    })
    message.success(`已套用访问模板「${tpl.name}」`)
  }

  const handleSaveToken = async () => {
    const vals = await form.validateFields()
    setSaving(true)
    try {
      const payload = {
        id: editing?.id,
        name: vals.name.trim(),
        unlimited_quota: Boolean(vals.unlimited_quota),
        remain_quota: Number(vals.remain_quota || 0),
        expired_time: vals.never_expire
          ? -1
          : vals.expired_at
            ? dayjs(vals.expired_at).unix()
            : -1,
        model_limits_enabled: Boolean(vals.model_limits_enabled),
        model_limits_type: Number(vals.model_limits_type || 0),
        model_limits: Array.isArray(vals.model_list) ? vals.model_list.join(',') : '',
        allow_ips: (vals.allow_ips || '').trim(),
        cross_group_retry: Boolean(vals.cross_group_retry),
      }
      if (editing) {
        await api.put('/api/token/', payload)
        message.success('令牌已更新')
      } else {
        await api.post('/api/token/', payload)
        message.success('令牌已创建')
      }
      setModalOpen(false)
      void loadTokens()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const toggleStatus = async (record: Token, checked: boolean) => {
    try {
      await api.put(
        '/api/token/',
        { id: record.id, status: checked ? 1 : 2 },
        { status_only: true },
      )
      message.success(checked ? '令牌已启用' : '令牌已禁用')
      void loadTokens()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '状态切换失败')
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await api.del(`/api/token/${id}`)
      message.success('已删除')
      void loadTokens()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  const handleBatchDelete = async () => {
    if (selectedRowKeys.length === 0) return
    try {
      await api.post('/api/token/batch', { ids: selectedRowKeys.map(Number) })
      message.success(`已批量删除 ${selectedRowKeys.length} 个令牌`)
      setSelectedRowKeys([])
      void loadTokens()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '批量删除失败')
    }
  }

  const openAnalytics = (record: Token) => {
    setAnalyticsToken(record)
    setAnalyticsOpen(true)
  }

  const handleSaveTemplate = async () => {
    const vals = await tplForm.validateFields()
    const payload = {
      id: editingTpl?.id,
      name: vals.name.trim(),
      model_limits_enabled: Boolean(vals.model_limits_enabled),
      model_limits_type: Number(vals.model_limits_type || 0),
      model_limits: Array.isArray(vals.model_list) ? vals.model_list.join(',') : '',
      allow_ips: (vals.allow_ips || '').trim(),
      cross_group_retry: Boolean(vals.cross_group_retry),
    }
    try {
      if (editingTpl) {
        await api.put('/api/token_template/', payload)
        message.success('模板已更新')
      } else {
        await api.post('/api/token_template/', payload)
        message.success('模板已新增')
      }
      setEditingTpl(null)
      tplForm.resetFields()
      void loadMeta()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存模板失败')
    }
  }

  const handleDeleteTemplate = async (id: number) => {
    try {
      await api.del(`/api/token_template/${id}`)
      message.success('模板已删除')
      void loadMeta()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除模板失败')
    }
  }

  const copyKey = async (rawKey: string) => {
    const full = rawKey.startsWith('sk-') ? rawKey : `sk-${rawKey}`
    await navigator.clipboard.writeText(full)
    message.success('API Key (sk-...) 已复制')
  }

  const columns: ColumnsType<Token> = [
    {
      title: '名称',
      dataIndex: 'name',
      width: 180,
      render: (v: string, r) => (
        <Space>
          <Typography.Text strong>{v}</Typography.Text>
          {r.model_limits_enabled && <Tag color="purple">限模型</Tag>}
          {r.allow_ips && <Tag color="cyan">限IP</Tag>}
        </Space>
      ),
    },
    {
      title: 'API Key',
      dataIndex: 'key',
      width: 210,
      render: (k: string) => (
        <Space size={6}>
          <Typography.Text code>{`sk-${k.slice(0, 6)}...${k.slice(-4)}`}</Typography.Text>
          <Tooltip title="复制完整 sk- 密钥">
            <Button
              type="text"
              size="small"
              icon={<Copy size={13} />}
              onClick={() => void copyKey(k)}
            />
          </Tooltip>
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      render: (status: number, r) => (
        <Switch
          size="small"
          checked={status === 1}
          checkedChildren="启用"
          unCheckedChildren={status === 3 ? '耗尽' : status === 4 ? '过期' : '禁用'}
          onChange={(checked) => void toggleStatus(r, checked)}
        />
      ),
    },
    {
      title: '额度 (已用 / 剩余)',
      width: 180,
      render: (_, r) => (
        <span>
          {r.used_quota} / {r.unlimited_quota ? <Tag color="green">无限额度</Tag> : r.remain_quota}
        </span>
      ),
    },
    {
      title: '过期时间',
      dataIndex: 'expired_time',
      width: 175,
      render: (v: number) => (v === -1 ? <Tag>永不过期</Tag> : formatUnixTime(v)),
    },
    {
      title: '最近访问',
      dataIndex: 'accessed_time',
      width: 175,
      render: (v: number) => formatUnixTime(v),
    },
    {
      title: '操作',
      width: 150,
      fixed: 'right',
      render: (_, r) => (
        <Space size={4}>
          <Tooltip title="调用分析">
            <Button
              type="text"
              size="small"
              icon={<BarChart3 size={15} />}
              onClick={() => void openAnalytics(r)}
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
          <Popconfirm title="确定删除该令牌？" onConfirm={() => void handleDelete(r.id)}>
            <Button type="text" size="small" danger icon={<Trash2 size={15} />} />
          </Popconfirm>
        </Space>
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
            gap: 12,
            marginBottom: 16,
          }}
        >
          <Space wrap>
            <Input.Search
              allowClear
              placeholder="按令牌名称搜索..."
              style={{ width: 260 }}
              onSearch={(v) => {
                setPage(1)
                setKeyword(v)
              }}
            />
            <Button icon={<RefreshCw size={14} />} onClick={() => void loadTokens()}>
              刷新
            </Button>
          </Space>

          <Space wrap>
            {selectedRowKeys.length > 0 && (
              <Popconfirm
                title={`确认批量删除选中的 ${selectedRowKeys.length} 个令牌？`}
                onConfirm={() => void handleBatchDelete()}
              >
                <Button danger icon={<Trash2 size={14} />}>
                  批量删除 ({selectedRowKeys.length})
                </Button>
              </Popconfirm>
            )}
            <Button icon={<BookTemplate size={14} />} onClick={() => setTplModalOpen(true)}>
              访问限制模板 ({templates.length})
            </Button>
            <Button type="primary" icon={<Plus size={15} />} onClick={openCreate}>
              新建令牌
            </Button>
          </Space>
        </div>

        <Table<Token>
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={data}
          scroll={{ x: 1050 }}
          rowSelection={{
            selectedRowKeys,
            onChange: setSelectedRowKeys,
          }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 个令牌`,
            onChange: (p, ps) => {
              setPage(p)
              setPageSize(ps)
            },
          }}
        />
      </Card>

      {/* 新建 / 编辑令牌弹窗 */}
      <Modal
        title={editing ? `编辑令牌：${editing.name}` : '新建 API 令牌'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => void handleSaveToken()}
        confirmLoading={saving}
        width={620}
      >
        {templates.length > 0 && (
          <div style={{ marginBottom: 14 }}>
            <Select
              allowClear
              style={{ width: '100%' }}
              placeholder="可选：一键套用访问限制模板..."
              options={templates.map((t) => ({ value: t.id, label: `套用模板：${t.name}` }))}
              onChange={(v) => v && applyTemplateToForm(v)}
            />
          </div>
        )}

        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="令牌名称"
            rules={[{ required: true, message: '请输入令牌名称' }]}
          >
            <Input placeholder="例如 生产环境主密钥" maxLength={50} />
          </Form.Item>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="unlimited_quota" label="无限额度" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item noStyle shouldUpdate={(prev, cur) => prev.unlimited_quota !== cur.unlimited_quota}>
                {({ getFieldValue }) =>
                  !getFieldValue('unlimited_quota') && (
                    <Form.Item name="remain_quota" label="剩余额度 (Quota)">
                      <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                  )
                }
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="never_expire" label="永不过期" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item noStyle shouldUpdate={(prev, cur) => prev.never_expire !== cur.never_expire}>
                {({ getFieldValue }) =>
                  !getFieldValue('never_expire') && (
                    <Form.Item name="expired_at" label="到期时间">
                      <DatePicker showTime style={{ width: '100%' }} />
                    </Form.Item>
                  )
                }
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="model_limits_enabled" label="启用模型访问限制" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="cross_group_retry" label="允许跨分组重试" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
          </Row>

          <Form.Item
            noStyle
            shouldUpdate={(prev, cur) => prev.model_limits_enabled !== cur.model_limits_enabled}
          >
            {({ getFieldValue }) =>
              getFieldValue('model_limits_enabled') && (
                <>
                  <Form.Item name="model_limits_type" label="限制模式">
                    <Radio.Group
                      options={[
                        { value: 0, label: '白名单（仅允许所选模型）' },
                        { value: 1, label: '黑名单（禁止访问所选模型）' },
                      ]}
                    />
                  </Form.Item>
                  <Form.Item name="model_list" label="模型列表">
                    <Select
                      mode="tags"
                      placeholder="选择或输入模型名..."
                      options={availableModels.map((m) => ({ value: m, label: m }))}
                    />
                  </Form.Item>
                </>
              )
            }
          </Form.Item>

          <Form.Item name="allow_ips" label="IP 白名单（每行或逗号分隔，留空不限制）">
            <Input.TextArea rows={2} placeholder="例如 1.2.3.4, 10.0.0.0/8" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 访问限制模板管理弹窗 */}
      <Modal
        title="令牌访问限制模板管理"
        open={tplModalOpen}
        onCancel={() => {
          setTplModalOpen(false)
          setEditingTpl(null)
          tplForm.resetFields()
        }}
        footer={null}
        width={680}
      >
        <Form
          form={tplForm}
          layout="vertical"
          initialValues={{ model_limits_enabled: true, model_limits_type: 0 }}
        >
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item name="name" label="模板名称" rules={[{ required: true }]}>
                <Input placeholder="例如 只读轻量模型模板" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="model_limits_type" label="限制类型">
                <Select
                  options={[
                    { value: 0, label: '模型白名单' },
                    { value: 1, label: '模型黑名单' },
                  ]}
                />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="model_list" label="模型列表">
            <Select
              mode="tags"
              options={availableModels.map((m) => ({ value: m, label: m }))}
              placeholder="选择模型..."
            />
          </Form.Item>
          <Form.Item name="allow_ips" label="IP 白名单（可选）">
            <Input placeholder="逗号分隔 IP" />
          </Form.Item>
          <Space style={{ marginBottom: 16 }}>
            <Button type="primary" onClick={() => void handleSaveTemplate()}>
              {editingTpl ? '更新模板' : '保存为新模板'}
            </Button>
            {editingTpl && (
              <Button
                onClick={() => {
                  setEditingTpl(null)
                  tplForm.resetFields()
                }}
              >
                取消编辑
              </Button>
            )}
          </Space>
        </Form>

        <Table<TokenAccessTemplate>
          size="small"
          rowKey="id"
          pagination={false}
          dataSource={templates}
          columns={[
            { title: '模板名', dataIndex: 'name' },
            {
              title: '模式',
              dataIndex: 'model_limits_type',
              render: (t: number) => (t === 1 ? <Tag color="red">黑名单</Tag> : <Tag color="green">白名单</Tag>),
            },
            { title: '模型', dataIndex: 'model_limits', ellipsis: true },
            {
              title: '操作',
              width: 110,
              render: (_, r) => (
                <Space size={4}>
                  <Button
                    type="link"
                    size="small"
                    onClick={() => {
                      setEditingTpl(r)
                      tplForm.setFieldsValue({
                        name: r.name,
                        model_limits_enabled: r.model_limits_enabled,
                        model_limits_type: r.model_limits_type,
                        model_list: r.model_limits ? r.model_limits.split(',').filter(Boolean) : [],
                        allow_ips: r.allow_ips || '',
                      })
                    }}
                  >
                    编辑
                  </Button>
                  <Popconfirm title="删除该模板？" onConfirm={() => void handleDeleteTemplate(r.id)}>
                    <Button type="link" size="small" danger>
                      删除
                    </Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Modal>

      {/* 单令牌调用分析弹窗 */}
      <TokenAnalyticsModal
        open={analyticsOpen}
        token={analyticsToken}
        onClose={() => setAnalyticsOpen(false)}
      />
    </div>
  )
}

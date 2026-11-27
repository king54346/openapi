import React, { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Col,
  Dropdown,
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
import {
  Copy,
  DollarSign,
  Edit3,
  Eye,
  FlaskConical,
  KeyRound,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Search,
  Tags,
  Trash2,
  Wrench,
  Zap,
} from 'lucide-react'
import { api } from '../../api/client'
import {
  CHANNEL_TYPES,
  formatUnixTime,
  getChannelTypeInfo,
} from '../../constants/channel'
import { useApp } from '../../contexts/AppContext'
import type {
  Channel,
  ChannelPageData,
  KeyStatusItem,
  MultiKeyStatusResponse,
  PageData,
  PrefillGroup,
} from '../../types/api'

export const ChannelPage: React.FC = () => {
  const { isRoot } = useApp()
  const { message } = App.useApp()

  const [loading, setLoading] = useState(false)
  const [items, setItems] = useState<Channel[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([])

  // 过滤项
  const [keyword, setKeyword] = useState('')
  const [modelFilter, setModelFilter] = useState('')
  const [typeFilter, setTypeFilter] = useState<number | undefined>(undefined)
  const [statusFilter, setStatusFilter] = useState<string | undefined>(undefined)
  const [tagMode, setTagMode] = useState(false)

  // 模型目录与预填充组
  const [builtinModels, setBuiltinModels] = useState<string[]>([])
  const [prefillGroups, setPrefillGroups] = useState<PrefillGroup[]>([])

  // 新建 / 编辑渠道弹窗
  const [editOpen, setEditOpen] = useState(false)
  const [editing, setEditing] = useState<Channel | null>(null)
  const [saving, setSaving] = useState(false)
  const [fetchingUpstream, setFetchingUpstream] = useState(false)
  const [form] = Form.useForm()

  // 多密钥管理弹窗
  const [multiKeyChannel, setMultiKeyChannel] = useState<Channel | null>(null)
  const [multiKeyData, setMultiKeyData] = useState<MultiKeyStatusResponse | null>(null)
  const [multiKeyPage, setMultiKeyPage] = useState(1)
  const [multiKeyLoading, setMultiKeyLoading] = useState(false)
  const [appendKeysText, setAppendKeysText] = useState('')

  // 批量标签管理弹窗
  const [tagModalOpen, setTagModalOpen] = useState(false)
  const [batchTagValue, setBatchTagValue] = useState('')

  const loadChannels = useCallback(async () => {
    setLoading(true)
    try {
      const res = await api.get<ChannelPageData>('/api/channel/', {
        p: page,
        page_size: pageSize,
        keyword: keyword.trim() || undefined,
        model: modelFilter.trim() || undefined,
        type: typeFilter,
        status: statusFilter,
        tag_mode: tagMode || undefined,
        id_sort: true,
      })
      setItems(res.items || [])
      setTotal(res.total || 0)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载渠道列表失败')
    } finally {
      setLoading(false)
    }
  }, [keyword, message, modelFilter, page, pageSize, statusFilter, tagMode, typeFilter])

  const loadCatalog = useCallback(async () => {
    try {
      const [modelsRaw, prefillPage] = await Promise.all([
        api.get<Array<{ id?: string } | string>>('/api/channel/models'),
        api.get<PageData<PrefillGroup>>('/api/prefill_group/', {
          type: 'model',
          p: 1,
          page_size: 100,
        }),
      ])
      const names = (modelsRaw || [])
        .map((item) => (typeof item === 'string' ? item : item.id || ''))
        .filter(Boolean)
      setBuiltinModels(Array.from(new Set(names)))
      setPrefillGroups(prefillPage.items || [])
    } catch {
      // ignore
    }
  }, [])

  useEffect(() => {
    void loadChannels()
  }, [loadChannels])

  useEffect(() => {
    void loadCatalog()
  }, [loadCatalog])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({
      add_mode: 'single',
      multi_key_mode: 'polling',
      type: 1,
      name: '',
      base_url: '',
      key: '',
      model_list: [],
      priority: 0,
      weight: 1,
      auto_ban: true,
      tag: '',
      model_mapping: '',
      param_override: '',
      header_override: '',
      remark: '',
    })
    setEditOpen(true)
  }

  const openEdit = async (record: Channel) => {
    try {
      const full = await api.get<Channel>(`/api/channel/${record.id}`)
      setEditing(full)
      form.setFieldsValue({
        type: full.type,
        name: full.name,
        base_url: full.base_url || '',
        key: '',
        key_mode: 'replace',
        multi_key_mode: full.channel_info?.multi_key_mode || 'polling',
        model_list: full.models
          ? full.models
              .split(',')
              .map((s) => s.trim())
              .filter(Boolean)
          : [],
        priority: full.priority ?? 0,
        weight: full.weight ?? 1,
        auto_ban: (full.auto_ban ?? 1) === 1,
        tag: full.tag || '',
        model_mapping: full.model_mapping || '',
        param_override: full.param_override || '',
        header_override: full.header_override || '',
        remark: full.remark || '',
      })
      setEditOpen(true)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载渠道详情失败')
    }
  }

  const handleFetchUpstreamModels = async () => {
    setFetchingUpstream(true)
    try {
      let list: string[] = []
      const inputKey = (form.getFieldValue('key') || '').trim()
      if (editing && !inputKey) {
        list = await api.get<string[]>(`/api/channel/fetch_models/${editing.id}`)
      } else {
        const firstKey = inputKey.split('\n').map((s: string) => s.trim()).filter(Boolean)[0] || ''
        if (!firstKey) {
          message.warning('请先填写渠道 API Key 再拉取上游模型')
          return
        }
        list = await api.post<string[]>('/api/channel/fetch_models', {
          type: form.getFieldValue('type'),
          base_url: (form.getFieldValue('base_url') || '').trim(),
          key: firstKey,
        })
      }
      if (list && list.length > 0) {
        const existing: string[] = form.getFieldValue('model_list') || []
        const merged = Array.from(new Set([...existing, ...list]))
        form.setFieldValue('model_list', merged)
        setBuiltinModels((prev) => Array.from(new Set([...prev, ...list])))
        message.success(`成功从上游拉取 ${list.length} 个模型`)
      } else {
        message.info('上游返回模型列表为空')
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : '拉取上游模型失败')
    } finally {
      setFetchingUpstream(false)
    }
  }

  const handleSaveChannel = async () => {
    const vals = await form.validateFields()
    setSaving(true)
    try {
      const modelsStr = Array.isArray(vals.model_list) ? vals.model_list.join(',') : ''
      if (editing) {
        const patch: Record<string, unknown> = {
          id: editing.id,
          name: vals.name.trim(),
          type: vals.type,
          base_url: (vals.base_url || '').trim(),
          models: modelsStr,
          priority: Number(vals.priority || 0),
          weight: Number(vals.weight || 1),
          auto_ban: vals.auto_ban ? 1 : 0,
          tag: (vals.tag || '').trim(),
          model_mapping: (vals.model_mapping || '').trim(),
          param_override: (vals.param_override || '').trim(),
          header_override: (vals.header_override || '').trim(),
          remark: (vals.remark || '').trim(),
          multi_key_mode: vals.multi_key_mode,
        }
        if ((vals.key || '').trim() !== '') {
          patch.key = vals.key.trim()
          patch.key_mode = vals.key_mode || 'replace'
        }
        await api.put('/api/channel/', patch)
        message.success('渠道已更新')
      } else {
        await api.post('/api/channel/', {
          mode: vals.add_mode || 'single',
          multi_key_mode: vals.multi_key_mode || 'polling',
          channel: {
            name: vals.name.trim(),
            type: vals.type,
            base_url: (vals.base_url || '').trim(),
            key: vals.key.trim(),
            models: modelsStr,
            priority: Number(vals.priority || 0),
            weight: Number(vals.weight || 1),
            auto_ban: vals.auto_ban ? 1 : 0,
            tag: (vals.tag || '').trim(),
            model_mapping: (vals.model_mapping || '').trim(),
            param_override: (vals.param_override || '').trim(),
            header_override: (vals.header_override || '').trim(),
            remark: (vals.remark || '').trim(),
          },
        })
        message.success('渠道创建成功')
      }
      setEditOpen(false)
      void loadChannels()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存渠道失败')
    } finally {
      setSaving(false)
    }
  }

  const toggleChannelStatus = async (record: Channel, enabled: boolean) => {
    try {
      await api.put('/api/channel/', { id: record.id, status: enabled ? 1 : 2 })
      message.success(enabled ? '渠道已启用' : '渠道已禁用')
      void loadChannels()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '修改状态失败')
    }
  }

  const handleTestChannel = async (record: Channel, allModels = false) => {
    const hide = message.loading(`正在测试渠道「${record.name}」...`, 0)
    try {
      if (allModels) {
        await api.get(`/api/channel/test/${record.id}/all_models`)
        message.success('全模型测速任务已执行，可前往「测试管理」查看明细')
      } else {
        const res = await api.get<{ time?: number; model?: string }>(`/api/channel/test/${record.id}`)
        message.success(`测速通过 (${res?.time ?? 0}s)`)
      }
      void loadChannels()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '渠道测速失败')
    } finally {
      hide()
    }
  }

  const handleUpdateBalance = async (record: Channel) => {
    try {
      const bal = await api.get<{ balance?: number }>(`/api/channel/update_balance/${record.id}`)
      message.success(`余额已刷新：${bal?.balance ?? 0}`)
      void loadChannels()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '更新余额失败')
    }
  }

  const handleCopyChannel = async (record: Channel) => {
    try {
      await api.post(`/api/channel/copy/${record.id}`)
      message.success('已复制渠道副本')
      void loadChannels()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '复制渠道失败')
    }
  }

  const handleRevealKey = async (record: Channel) => {
    try {
      const res = await api.post<{ key: string }>(`/api/channel/${record.id}/key`)
      Modal.info({
        title: `渠道密钥：${record.name}`,
        width: 560,
        content: (
          <pre className="code-block" style={{ marginTop: 12, maxHeight: 280 }}>
            {res.key}
          </pre>
        ),
      })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '获取密钥失败')
    }
  }

  // 多密钥管理
  const loadMultiKeys = useCallback(
    async (channelId: number, p = 1) => {
      setMultiKeyLoading(true)
      try {
        const res = await api.post<MultiKeyStatusResponse>('/api/channel/multi_key/manage', {
          channel_id: channelId,
          action: 'get_key_status',
          page: p,
          page_size: 10,
        })
        setMultiKeyData(res)
        setMultiKeyPage(p)
      } catch (err) {
        message.error(err instanceof Error ? err.message : '获取多密钥状态失败')
      } finally {
        setMultiKeyLoading(false)
      }
    },
    [message],
  )

  const runMultiKeyAction = async (action: string, keyIndex?: number) => {
    if (!multiKeyChannel) return
    try {
      await api.post('/api/channel/multi_key/manage', {
        channel_id: multiKeyChannel.id,
        action,
        key_index: keyIndex,
      })
      message.success('操作已生效')
      void loadMultiKeys(multiKeyChannel.id, multiKeyPage)
      void loadChannels()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    }
  }

  const handleAppendMultiKeys = async () => {
    if (!multiKeyChannel || !appendKeysText.trim()) return
    try {
      await api.put('/api/channel/', {
        id: multiKeyChannel.id,
        key: appendKeysText.trim(),
        key_mode: 'append',
      })
      setAppendKeysText('')
      message.success('新密钥已追加到轮询池')
      void loadMultiKeys(multiKeyChannel.id, multiKeyPage)
      void loadChannels()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '追加密钥失败')
    }
  }

  const handleBatchSetTag = async () => {
    if (selectedRowKeys.length === 0) return
    try {
      await api.post('/api/channel/batch/tag', {
        ids: selectedRowKeys.map(Number),
        tag: batchTagValue.trim(),
      })
      message.success('批量设置标签成功')
      setTagModalOpen(false)
      setSelectedRowKeys([])
      void loadChannels()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '批量设置标签失败')
    }
  }

  const columns: ColumnsType<Channel> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    {
      title: '名称 / 标签',
      width: 220,
      render: (_, r) => (
        <Space direction="vertical" size={2}>
          <Space size={6}>
            <Typography.Text strong>{r.name}</Typography.Text>
            {r.channel_info?.is_multi_key && (
              <Tag
                color="purple"
                style={{ cursor: 'pointer' }}
                onClick={() => {
                  setMultiKeyChannel(r)
                  void loadMultiKeys(r.id, 1)
                }}
              >
                {r.channel_info.multi_key_size} Keys (
                {r.channel_info.multi_key_mode === 'polling' ? '轮询' : '随机'})
              </Tag>
            )}
          </Space>
          {r.tag && <Tag color="cyan">#{r.tag}</Tag>}
        </Space>
      ),
    },
    {
      title: '渠道类型',
      dataIndex: 'type',
      width: 155,
      render: (t: number) => {
        const info = getChannelTypeInfo(t)
        return <Tag color={info.color}>{info.label}</Tag>
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 120,
      render: (s: number, r) => (
        <Space size={6}>
          <Switch
            size="small"
            checked={s === 1}
            onChange={(checked) => void toggleChannelStatus(r, checked)}
          />
          {s === 3 && <Tag color="error">自动熔断</Tag>}
        </Space>
      ),
    },
    {
      title: '响应延迟',
      dataIndex: 'response_time',
      width: 115,
      render: (ms: number, r) =>
        ms > 0 ? (
          <Tooltip title={`上次测速：${formatUnixTime(r.test_time)}`}>
            <Tag color={ms < 1500 ? 'success' : ms < 4000 ? 'warning' : 'error'}>{ms} ms</Tag>
          </Tooltip>
        ) : (
          <Typography.Text type="secondary">未测速</Typography.Text>
        ),
    },
    {
      title: '优先级 / 权重',
      width: 120,
      render: (_, r) => `P${r.priority ?? 0} / W${r.weight ?? 1}`,
    },
    {
      title: '已用 / 余额',
      width: 145,
      render: (_, r) => `${r.used_quota || 0} / ${r.balance?.toFixed?.(2) ?? 0}`,
    },
    {
      title: '模型数',
      width: 95,
      render: (_, r) => {
        const count = r.models ? r.models.split(',').filter(Boolean).length : 0
        return <Tag>{count} 个</Tag>
      },
    },
    {
      title: '操作',
      width: 210,
      fixed: 'right',
      render: (_, r) => (
        <Space size={2}>
          <Tooltip title="快速测速">
            <Button
              type="text"
              size="small"
              icon={<Zap size={15} />}
              onClick={() => void handleTestChannel(r, false)}
            />
          </Tooltip>
          <Tooltip title="编辑渠道">
            <Button
              type="text"
              size="small"
              icon={<Edit3 size={15} />}
              onClick={() => void openEdit(r)}
            />
          </Tooltip>
          {r.channel_info?.is_multi_key && (
            <Tooltip title="多密钥池管理">
              <Button
                type="text"
                size="small"
                icon={<KeyRound size={15} />}
                onClick={() => {
                  setMultiKeyChannel(r)
                  void loadMultiKeys(r.id, 1)
                }}
              />
            </Tooltip>
          )}
          <Dropdown
            menu={{
              items: [
                {
                  key: 'test_all',
                  icon: <FlaskConical size={14} />,
                  label: '测速该渠道全部模型',
                  onClick: () => void handleTestChannel(r, true),
                },
                {
                  key: 'balance',
                  icon: <DollarSign size={14} />,
                  label: '刷新渠道余额',
                  onClick: () => void handleUpdateBalance(r),
                },
                {
                  key: 'copy',
                  icon: <Copy size={14} />,
                  label: '复制渠道',
                  onClick: () => void handleCopyChannel(r),
                },
                ...(r.tag
                  ? [
                      {
                        key: 'tag_enable',
                        label: `启用标签「${r.tag}」全部渠道`,
                        onClick: async () => {
                          await api.post('/api/channel/tag/enabled', { tag: r.tag })
                          message.success('已启用同标签渠道')
                          void loadChannels()
                        },
                      },
                      {
                        key: 'tag_disable',
                        label: `禁用标签「${r.tag}」全部渠道`,
                        onClick: async () => {
                          await api.post('/api/channel/tag/disabled', { tag: r.tag })
                          message.success('已禁用同标签渠道')
                          void loadChannels()
                        },
                      },
                    ]
                  : []),
                ...(isRoot
                  ? [
                      {
                        key: 'view_key',
                        icon: <Eye size={14} />,
                        label: '查看真实密钥 (Root)',
                        onClick: () => void handleRevealKey(r),
                      },
                    ]
                  : []),
                { type: 'divider' },
                {
                  key: 'delete',
                  danger: true,
                  icon: <Trash2 size={14} />,
                  label: '删除渠道',
                  onClick: async () => {
                    await api.del(`/api/channel/${r.id}`)
                    message.success('渠道已删除')
                    void loadChannels()
                  },
                },
              ],
            }}
          >
            <Button type="text" size="small" icon={<MoreHorizontal size={15} />} />
          </Dropdown>
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
            gap: 10,
            marginBottom: 14,
          }}
        >
          <Space wrap>
            <Input
              allowClear
              placeholder="渠道名称 / 标签关键词"
              style={{ width: 180 }}
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
            />
            <Input
              allowClear
              placeholder="包含模型名"
              style={{ width: 150 }}
              value={modelFilter}
              onChange={(e) => setModelFilter(e.target.value)}
            />
            <Select
              allowClear
              placeholder="渠道类型"
              style={{ width: 165 }}
              value={typeFilter}
              onChange={setTypeFilter}
              options={CHANNEL_TYPES.map((c) => ({ value: c.value, label: c.label }))}
            />
            <Select
              allowClear
              placeholder="状态"
              style={{ width: 125 }}
              value={statusFilter}
              onChange={setStatusFilter}
              options={[
                { value: 'enabled', label: '已启用' },
                { value: 'disabled', label: '已禁用' },
              ]}
            />
            <Space size={4}>
              <Switch size="small" checked={tagMode} onChange={setTagMode} />
              <Typography.Text style={{ fontSize: 13 }}>标签聚合</Typography.Text>
            </Space>
            <Button
              type="primary"
              icon={<Search size={14} />}
              onClick={() => {
                setPage(1)
                void loadChannels()
              }}
            >
              搜索
            </Button>
          </Space>

          <Space wrap>
            {selectedRowKeys.length > 0 && (
              <>
                <Button icon={<Tags size={14} />} onClick={() => setTagModalOpen(true)}>
                  批量设标签 ({selectedRowKeys.length})
                </Button>
                <Popconfirm
                  title={`删除选中的 ${selectedRowKeys.length} 个渠道？`}
                  onConfirm={async () => {
                    await api.post('/api/channel/batch', { ids: selectedRowKeys.map(Number) })
                    message.success('已批量删除')
                    setSelectedRowKeys([])
                    void loadChannels()
                  }}
                >
                  <Button danger icon={<Trash2 size={14} />}>
                    批量删除
                  </Button>
                </Popconfirm>
              </>
            )}

            <Dropdown
              menu={{
                items: [
                  {
                    key: 'test_all_ch',
                    icon: <Zap size={14} />,
                    label: '批量测试全部已启用渠道',
                    onClick: async () => {
                      await api.get('/api/channel/test')
                      message.success('已触发全渠道后台测速')
                    },
                  },
                  {
                    key: 'balance_all',
                    icon: <DollarSign size={14} />,
                    label: '更新全部渠道余额',
                    onClick: async () => {
                      await api.get('/api/channel/update_balance')
                      message.success('已触发余额更新')
                      void loadChannels()
                    },
                  },
                  {
                    key: 'fix_abilities',
                    icon: <Wrench size={14} />,
                    label: '修复渠道能力索引 (/fix)',
                    onClick: async () => {
                      const count = await api.post<number>('/api/channel/fix')
                      message.success(`能力表修复完成 (${count ?? 0})`)
                    },
                  },
                  {
                    key: 'del_disabled',
                    danger: true,
                    icon: <Trash2 size={14} />,
                    label: '一键删除所有已禁用渠道',
                    onClick: async () => {
                      const rows = await api.del<number>('/api/channel/disabled')
                      message.success(`已清理禁用渠道 ${rows ?? 0} 个`)
                      void loadChannels()
                    },
                  },
                ],
              }}
            >
              <Button icon={<Wrench size={14} />}>运维工具</Button>
            </Dropdown>

            <Button icon={<RefreshCw size={14} />} onClick={() => void loadChannels()} />
            <Button type="primary" icon={<Plus size={15} />} onClick={openCreate}>
              新建渠道
            </Button>
          </Space>
        </div>

        <Table<Channel>
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={items}
          scroll={{ x: 1180 }}
          rowSelection={{ selectedRowKeys, onChange: setSelectedRowKeys }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 个渠道`,
            onChange: (p, ps) => {
              setPage(p)
              setPageSize(ps)
            },
          }}
        />
      </Card>

      {/* 新建 / 编辑渠道弹窗 */}
      <Modal
        title={editing ? `编辑渠道 #${editing.id}：${editing.name}` : '新建上游渠道'}
        open={editOpen}
        onCancel={() => setEditOpen(false)}
        onOk={() => void handleSaveChannel()}
        confirmLoading={saving}
        width={760}
      >
        <Form form={form} layout="vertical">
          {!editing && (
            <Form.Item name="add_mode" label="创建模式">
              <Radio.Group
                optionType="button"
                buttonStyle="solid"
                options={[
                  { value: 'single', label: '单渠道' },
                  { value: 'multi_to_single', label: '多密钥聚合单渠道（推荐轮询池）' },
                  { value: 'batch', label: '多行 Key 拆分为多个独立渠道' },
                ]}
              />
            </Form.Item>
          )}

          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.Item name="type" label="渠道供应商类型" rules={[{ required: true }]}>
                <Select
                  showSearch
                  optionFilterProp="label"
                  options={CHANNEL_TYPES.map((c) => ({ value: c.value, label: c.label }))}
                />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item name="name" label="渠道名称" rules={[{ required: true, message: '请输入渠道名称' }]}>
                <Input placeholder="例如 DeepSeek 官方主池" />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col xs={24} sm={14}>
              <Form.Item
                noStyle
                shouldUpdate={(prev, cur) => prev.type !== cur.type}
              >
                {({ getFieldValue }) => {
                  const defaultURL = getChannelTypeInfo(getFieldValue('type') || 1).defaultBaseURL
                  return (
                    <Form.Item
                      name="base_url"
                      label={`自定义 Base URL（留空使用默认 ${defaultURL || '官方地址'}）`}
                    >
                      <Input placeholder={defaultURL || 'https://api.openai.com'} />
                    </Form.Item>
                  )
                }}
              </Form.Item>
            </Col>
            <Col xs={24} sm={10}>
              <Form.Item name="tag" label="渠道标签（可选，用于批量分组）">
                <Input placeholder="例如 prod-pool" />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col xs={24} sm={16}>
              <Form.Item
                name="key"
                label={
                  editing
                    ? '上游 API Key（留空保持原密钥不变；多密钥可换行填写）'
                    : '上游 API Key（多密钥模式每行一个 Key）'
                }
                rules={editing ? [] : [{ required: true, message: '请输入 API Key' }]}
              >
                <Input.TextArea rows={3} placeholder="sk-xxxx（支持多行输入多个 Key）" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={8}>
              {editing && (
                <Form.Item name="key_mode" label="多密钥写入方式">
                  <Select
                    options={[
                      { value: 'replace', label: '覆盖原有密钥' },
                      { value: 'append', label: '追加到现有密钥池' },
                    ]}
                  />
                </Form.Item>
              )}
              <Form.Item name="multi_key_mode" label="多密钥调度策略">
                <Select
                  options={[
                    { value: 'polling', label: '严格顺序轮询 (Redis 零写库)' },
                    { value: 'random', label: '随机抽取 (Random)' },
                  ]}
                />
              </Form.Item>
            </Col>
          </Row>

          <Form.Item
            label={
              <Space wrap>
                <span>支持模型列表</span>
                <Button
                  size="small"
                  type="link"
                  loading={fetchingUpstream}
                  onClick={() => void handleFetchUpstreamModels()}
                >
                  从上游一键拉取模型 (/v1/models)
                </Button>
                {prefillGroups.length > 0 && (
                  <Select
                    size="small"
                    placeholder="从预填充组导入..."
                    style={{ width: 170 }}
                    options={prefillGroups.map((g) => ({ value: g.id, label: g.name }))}
                    onChange={(gid) => {
                      const g = prefillGroups.find((x) => x.id === gid)
                      if (g && Array.isArray(g.items)) {
                        const existing: string[] = form.getFieldValue('model_list') || []
                        form.setFieldValue(
                          'model_list',
                          Array.from(new Set([...existing, ...(g.items as string[])])),
                        )
                      }
                    }}
                  />
                )}
              </Space>
            }
            name="model_list"
            rules={[{ required: true, message: '请至少配置一个模型' }]}
          >
            <Select
              mode="tags"
              placeholder="选择内置模型、从上游拉取或直接输入模型名回车..."
              options={builtinModels.map((m) => ({ value: m, label: m }))}
            />
          </Form.Item>

          <Row gutter={16}>
            <Col xs={8}>
              <Form.Item name="priority" label="优先级 (越大越优先)">
                <InputNumber style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={8}>
              <Form.Item name="weight" label="同优先级权重">
                <InputNumber min={1} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={8}>
              <Form.Item name="auto_ban" label="连续失败自动熔断" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
          </Row>

          <Form.Item name="model_mapping" label="模型名称重定向映射 (JSON，可选)">
            <Input.TextArea
              rows={2}
              placeholder='例如 {"gpt-4o":"deepseek-chat"}'
              style={{ fontFamily: 'monospace' }}
            />
          </Form.Item>

          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.Item name="param_override" label="请求体参数覆盖 (JSON，可选)">
                <Input.TextArea rows={2} style={{ fontFamily: 'monospace' }} />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item name="header_override" label="自定义请求头覆盖 (JSON，可选)">
                <Input.TextArea rows={2} style={{ fontFamily: 'monospace' }} />
              </Form.Item>
            </Col>
          </Row>

          <Form.Item name="remark" label="备注">
            <Input placeholder="可选备注说明" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 多密钥状态管理弹窗 */}
      <Modal
        title={`多密钥池管理：${multiKeyChannel?.name || ''}`}
        open={Boolean(multiKeyChannel)}
        onCancel={() => {
          setMultiKeyChannel(null)
          setMultiKeyData(null)
        }}
        footer={null}
        width={760}
      >
        {multiKeyData && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: 8 }}>
              <Space>
                <Tag color="success">启用中：{multiKeyData.enabled_count}</Tag>
                <Tag color="warning">手动禁用：{multiKeyData.manual_disabled_count}</Tag>
                <Tag color="error">自动禁用：{multiKeyData.auto_disabled_count}</Tag>
              </Space>
              <Space>
                <Button size="small" onClick={() => void runMultiKeyAction('enable_all_keys')}>
                  全部启用
                </Button>
                <Button size="small" onClick={() => void runMultiKeyAction('disable_all_keys')}>
                  全部禁用
                </Button>
                <Popconfirm
                  title="删除池内所有已禁用的子密钥？"
                  onConfirm={() => void runMultiKeyAction('delete_disabled_keys')}
                >
                  <Button size="small" danger>
                    移除所有禁用 Key
                  </Button>
                </Popconfirm>
              </Space>
            </div>

            <Space.Compact style={{ width: '100%' }}>
              <Input
                placeholder="快速追加新密钥（多个可用换行或空格分隔后粘贴）"
                value={appendKeysText}
                onChange={(e) => setAppendKeysText(e.target.value)}
              />
              <Button type="primary" onClick={() => void handleAppendMultiKeys()}>
                追加密钥
              </Button>
            </Space.Compact>

            <Table<KeyStatusItem>
              size="small"
              rowKey="index"
              loading={multiKeyLoading}
              dataSource={multiKeyData.keys || []}
              pagination={{
                current: multiKeyPage,
                pageSize: 10,
                total: multiKeyData.total,
                onChange: (p) => multiKeyChannel && void loadMultiKeys(multiKeyChannel.id, p),
              }}
              columns={[
                { title: '下标', dataIndex: 'index', width: 65, render: (i: number) => `#${i}` },
                {
                  title: '脱敏预览',
                  dataIndex: 'key_preview',
                  render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
                },
                {
                  title: '状态',
                  dataIndex: 'status',
                  width: 105,
                  render: (s: number) =>
                    s === 1 ? (
                      <Tag color="success">已启用</Tag>
                    ) : s === 3 ? (
                      <Tag color="error">自动禁用</Tag>
                    ) : (
                      <Tag color="default">手动禁用</Tag>
                    ),
                },
                { title: '异常原因', dataIndex: 'reason', ellipsis: true },
                {
                  title: '操作',
                  width: 140,
                  render: (_, k) => (
                    <Space size={4}>
                      {k.status === 1 ? (
                        <Button
                          type="link"
                          size="small"
                          onClick={() => void runMultiKeyAction('disable_key', k.index)}
                        >
                          禁用
                        </Button>
                      ) : (
                        <Button
                          type="link"
                          size="small"
                          onClick={() => void runMultiKeyAction('enable_key', k.index)}
                        >
                          启用
                        </Button>
                      )}
                      <Popconfirm
                        title="确认移除该子密钥？"
                        onConfirm={() => void runMultiKeyAction('delete_key', k.index)}
                      >
                        <Button type="link" size="small" danger>
                          移除
                        </Button>
                      </Popconfirm>
                    </Space>
                  ),
                },
              ]}
            />
          </div>
        )}
      </Modal>

      {/* 批量设置标签弹窗 */}
      <Modal
        title={`为选中的 ${selectedRowKeys.length} 个渠道设置标签`}
        open={tagModalOpen}
        onCancel={() => setTagModalOpen(false)}
        onOk={() => void handleBatchSetTag()}
      >
        <Input
          placeholder="输入标签名（留空表示清除标签）"
          value={batchTagValue}
          onChange={(e) => setBatchTagValue(e.target.value)}
        />
      </Modal>
    </div>
  )
}

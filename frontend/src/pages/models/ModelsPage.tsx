import React, { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Col,
  Form,
  Input,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
  App,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  CloudDownload,
  Edit3,
  Plus,
  RefreshCw,
  Search,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { api } from '../../api/client'
import { formatUnixTime } from '../../constants/channel'
import type {
  ModelMeta,
  ModelMetaPageData,
  PageData,
  PrefillGroup,
  SyncPreviewResponse,
  VendorMeta,
} from '../../types/api'

const NAME_RULES = [
  { value: 0, label: '精确匹配 (Exact)' },
  { value: 1, label: '前缀匹配 (Prefix)' },
  { value: 2, label: '包含匹配 (Contains)' },
  { value: 3, label: '后缀匹配 (Suffix)' },
]

export const ModelsPage: React.FC = () => {
  const { message } = App.useApp()

  // ---------- 模型元数据 ----------
  const [modelsLoading, setModelsLoading] = useState(false)
  const [models, setModels] = useState<ModelMeta[]>([])
  const [modelsTotal, setModelsTotal] = useState(0)
  const [mPage, setMPage] = useState(1)
  const [mPageSize, setMPageSize] = useState(20)
  const [mKeyword, setMKeyword] = useState('')
  const [mVendorFilter, setMVendorFilter] = useState<string | undefined>(undefined)

  const [modelModalOpen, setModelModalOpen] = useState(false)
  const [editingModel, setEditingModel] = useState<ModelMeta | null>(null)
  const [modelForm] = Form.useForm()

  // 未配置模型 & 同步上游弹窗
  const [missingOpen, setMissingOpen] = useState(false)
  const [missingModels, setMissingModels] = useState<string[]>([])
  const [syncOpen, setSyncOpen] = useState(false)
  const [syncPreview, setSyncPreview] = useState<SyncPreviewResponse | null>(null)
  const [syncLoading, setSyncLoading] = useState(false)

  // ---------- 供应商 ----------
  const [vendors, setVendors] = useState<VendorMeta[]>([])
  const [vendorsLoading, setVendorsLoading] = useState(false)
  const [vendorModalOpen, setVendorModalOpen] = useState(false)
  const [editingVendor, setEditingVendor] = useState<VendorMeta | null>(null)
  const [vendorForm] = Form.useForm()

  // ---------- 预填充组 ----------
  const [prefills, setPrefills] = useState<PrefillGroup[]>([])
  const [prefillsLoading, setPrefillsLoading] = useState(false)
  const [prefillTypeFilter, setPrefillTypeFilter] = useState<string | undefined>(undefined)
  const [prefillModalOpen, setPrefillModalOpen] = useState(false)
  const [editingPrefill, setEditingPrefill] = useState<PrefillGroup | null>(null)
  const [prefillForm] = Form.useForm()

  const loadVendors = useCallback(async () => {
    setVendorsLoading(true)
    try {
      const res = await api.get<PageData<VendorMeta>>('/api/vendors/', { p: 1, page_size: 200 })
      setVendors(res.items || [])
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载供应商失败')
    } finally {
      setVendorsLoading(false)
    }
  }, [message])

  const loadModels = useCallback(async () => {
    setModelsLoading(true)
    try {
      const hasSearch = Boolean(mKeyword.trim() || mVendorFilter)
      const path = hasSearch ? '/api/models/search' : '/api/models/'
      const res = await api.get<ModelMetaPageData>(path, {
        p: mPage,
        page_size: mPageSize,
        keyword: mKeyword.trim() || undefined,
        vendor: mVendorFilter,
      })
      setModels(res.items || [])
      setModelsTotal(res.total || 0)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载模型元数据失败')
    } finally {
      setModelsLoading(false)
    }
  }, [mKeyword, mPage, mPageSize, mVendorFilter, message])

  const loadPrefills = useCallback(async () => {
    setPrefillsLoading(true)
    try {
      const res = await api.get<PageData<PrefillGroup>>('/api/prefill_group/', {
        type: prefillTypeFilter,
        p: 1,
        page_size: 100,
      })
      setPrefills(res.items || [])
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载预填充组失败')
    } finally {
      setPrefillsLoading(false)
    }
  }, [message, prefillTypeFilter])

  useEffect(() => {
    void loadVendors()
    void loadPrefills()
  }, [loadPrefills, loadVendors])

  useEffect(() => {
    void loadModels()
  }, [loadModels])

  const vendorMap = Object.fromEntries(vendors.map((v) => [v.id, v]))

  // ---------- 模型保存 ----------
  const openCreateModel = (prefillName = '') => {
    setEditingModel(null)
    modelForm.resetFields()
    modelForm.setFieldsValue({
      model_name: prefillName,
      name_rule: 0,
      status_bool: true,
      sync_official_bool: true,
      tags_list: [],
    })
    setModelModalOpen(true)
  }

  const openEditModel = (record: ModelMeta) => {
    setEditingModel(record)
    modelForm.setFieldsValue({
      model_name: record.model_name,
      vendor_id: record.vendor_id || undefined,
      name_rule: record.name_rule ?? 0,
      icon: record.icon || '',
      description: record.description || '',
      tags_list: record.tags
        ? record.tags
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean)
        : [],
      endpoints: record.endpoints || '',
      status_bool: record.status === 1,
      sync_official_bool: (record.sync_official ?? 1) === 1,
    })
    setModelModalOpen(true)
  }

  const saveModel = async () => {
    const vals = await modelForm.validateFields()
    const payload = {
      id: editingModel?.id,
      model_name: vals.model_name.trim(),
      vendor_id: vals.vendor_id || 0,
      name_rule: Number(vals.name_rule || 0),
      icon: (vals.icon || '').trim(),
      description: (vals.description || '').trim(),
      tags: Array.isArray(vals.tags_list) ? vals.tags_list.join(',') : '',
      endpoints: (vals.endpoints || '').trim(),
      status: vals.status_bool ? 1 : 2,
      sync_official: vals.sync_official_bool ? 1 : 0,
    }
    try {
      if (editingModel) {
        await api.put('/api/models/', payload)
        message.success('模型元数据已更新')
      } else {
        await api.post('/api/models/', payload)
        message.success('模型元数据已创建')
      }
      setModelModalOpen(false)
      void loadModels()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    }
  }

  const openMissingModal = async () => {
    setMissingOpen(true)
    try {
      const res = await api.get<{ models: string[]; total: number }>('/api/models/missing')
      setMissingModels(res.models || [])
    } catch (err) {
      message.error(err instanceof Error ? err.message : '扫描未配置模型失败')
    }
  }

  const openSyncModal = async () => {
    setSyncOpen(true)
    setSyncLoading(true)
    try {
      const res = await api.get<SyncPreviewResponse>('/api/models/sync_upstream/preview')
      setSyncPreview(res)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '获取同步预览失败')
    } finally {
      setSyncLoading(false)
    }
  }

  const executeSyncUpstream = async () => {
    setSyncLoading(true)
    try {
      await api.post('/api/models/sync_upstream', { dry_run: false })
      message.success('已同步导入上游模型元数据')
      setSyncOpen(false)
      void loadModels()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '同步失败')
    } finally {
      setSyncLoading(false)
    }
  }

  // ---------- 供应商保存 ----------
  const saveVendor = async () => {
    const vals = await vendorForm.validateFields()
    const payload = {
      id: editingVendor?.id,
      name: vals.name.trim(),
      icon: (vals.icon || '').trim(),
      description: (vals.description || '').trim(),
      status: vals.status_bool ? 1 : 2,
    }
    try {
      if (editingVendor) {
        await api.put('/api/vendors/', payload)
        message.success('供应商已更新')
      } else {
        await api.post('/api/vendors/', payload)
        message.success('供应商已新增')
      }
      setVendorModalOpen(false)
      void loadVendors()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存供应商失败')
    }
  }

  // ---------- 预填充组保存 ----------
  const savePrefill = async () => {
    const vals = await prefillForm.validateFields()
    let parsedItems: unknown = []
    try {
      parsedItems = JSON.parse(vals.items_json)
    } catch {
      parsedItems = String(vals.items_json || '')
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean)
    }
    const payload = {
      id: editingPrefill?.id,
      name: vals.name.trim(),
      type: vals.type,
      description: (vals.description || '').trim(),
      items: parsedItems,
    }
    try {
      if (editingPrefill) {
        await api.put('/api/prefill_group/', payload)
        message.success('预填充组已更新')
      } else {
        await api.post('/api/prefill_group/', payload)
        message.success('预填充组已创建')
      }
      setPrefillModalOpen(false)
      void loadPrefills()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存预填充组失败')
    }
  }

  const modelColumns: ColumnsType<ModelMeta> = [
    { title: 'ID', dataIndex: 'id', width: 65 },
    {
      title: '模型名称',
      dataIndex: 'model_name',
      width: 220,
      render: (m: string, r) => (
        <Space size={4} wrap>
          <Typography.Text strong>{m}</Typography.Text>
          {r.name_rule > 0 && <Tag color="purple">规则#{r.name_rule}</Tag>}
        </Space>
      ),
    },
    {
      title: '所属供应商',
      dataIndex: 'vendor_id',
      width: 140,
      render: (vid?: number) =>
        vid && vendorMap[vid] ? <Tag color="blue">{vendorMap[vid].name}</Tag> : '-',
    },
    {
      title: '标签',
      dataIndex: 'tags',
      width: 200,
      render: (tags?: string) =>
        tags
          ? tags
              .split(',')
              .filter(Boolean)
              .map((t) => <Tag key={t}>{t}</Tag>)
          : '-',
    },
    {
      title: '关联渠道',
      width: 120,
      render: (_, r) => <Tag color="geekblue">{r.bound_channels?.length ?? 0} 个渠道</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 95,
      render: (s: number, r) => (
        <Switch
          size="small"
          checked={s === 1}
          onChange={async (checked) => {
            await api.put(
              '/api/models/',
              { ...r, status: checked ? 1 : 2 },
              { status_only: true },
            )
            void loadModels()
          }}
        />
      ),
    },
    { title: '描述', dataIndex: 'description', ellipsis: true },
    {
      title: '操作',
      width: 110,
      fixed: 'right',
      render: (_, r) => (
        <Space size={4}>
          <Button
            type="text"
            size="small"
            icon={<Edit3 size={15} />}
            onClick={() => openEditModel(r)}
          />
          <Popconfirm
            title="删除该模型元数据？"
            onConfirm={async () => {
              await api.del(`/api/models/${r.id}`)
              message.success('已删除')
              void loadModels()
            }}
          >
            <Button type="text" size="small" danger icon={<Trash2 size={15} />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card style={{ borderRadius: 12 }}>
      <Tabs
        items={[
          {
            key: 'models',
            label: `模型元数据 (${modelsTotal})`,
            children: (
              <div>
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
                      placeholder="搜索模型名 / 描述"
                      style={{ width: 200 }}
                      value={mKeyword}
                      onChange={(e) => setMKeyword(e.target.value)}
                    />
                    <Select
                      allowClear
                      placeholder="按供应商过滤"
                      style={{ width: 165 }}
                      value={mVendorFilter}
                      onChange={setMVendorFilter}
                      options={vendors.map((v) => ({ value: String(v.id), label: v.name }))}
                    />
                    <Button
                      type="primary"
                      icon={<Search size={14} />}
                      onClick={() => {
                        setMPage(1)
                        void loadModels()
                      }}
                    >
                      查询
                    </Button>
                  </Space>

                  <Space wrap>
                    <Button icon={<Sparkles size={14} />} onClick={() => void openMissingModal()}>
                      未登记模型扫描
                    </Button>
                    <Button icon={<CloudDownload size={14} />} onClick={() => void openSyncModal()}>
                      同步渠道模型
                    </Button>
                    <Button icon={<RefreshCw size={14} />} onClick={() => void loadModels()} />
                    <Button
                      type="primary"
                      icon={<Plus size={15} />}
                      onClick={() => openCreateModel()}
                    >
                      新增模型
                    </Button>
                  </Space>
                </div>

                <Table<ModelMeta>
                  rowKey="id"
                  loading={modelsLoading}
                  columns={modelColumns}
                  dataSource={models}
                  scroll={{ x: 980 }}
                  pagination={{
                    current: mPage,
                    pageSize: mPageSize,
                    total: modelsTotal,
                    showSizeChanger: true,
                    onChange: (p, ps) => {
                      setMPage(p)
                      setMPageSize(ps)
                    },
                  }}
                />
              </div>
            ),
          },
          {
            key: 'vendors',
            label: `供应商管理 (${vendors.length})`,
            children: (
              <div>
                <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginBottom: 14 }}>
                  <Button icon={<RefreshCw size={14} />} onClick={() => void loadVendors()} />
                  <Button
                    type="primary"
                    icon={<Plus size={15} />}
                    onClick={() => {
                      setEditingVendor(null)
                      vendorForm.resetFields()
                      vendorForm.setFieldsValue({ status_bool: true })
                      setVendorModalOpen(true)
                    }}
                  >
                    新增供应商
                  </Button>
                </div>
                <Table<VendorMeta>
                  rowKey="id"
                  loading={vendorsLoading}
                  dataSource={vendors}
                  columns={[
                    { title: 'ID', dataIndex: 'id', width: 70 },
                    {
                      title: '供应商名称',
                      dataIndex: 'name',
                      width: 200,
                      render: (v: string) => <Typography.Text strong>{v}</Typography.Text>,
                    },
                    { title: '图标标识', dataIndex: 'icon', width: 160 },
                    {
                      title: '状态',
                      dataIndex: 'status',
                      width: 100,
                      render: (s: number) =>
                        s === 1 ? <Tag color="success">启用</Tag> : <Tag>禁用</Tag>,
                    },
                    { title: '描述', dataIndex: 'description', ellipsis: true },
                    {
                      title: '更新时间',
                      dataIndex: 'updated_time',
                      width: 170,
                      render: (v?: number) => formatUnixTime(v),
                    },
                    {
                      title: '操作',
                      width: 110,
                      render: (_, r) => (
                        <Space size={4}>
                          <Button
                            type="text"
                            size="small"
                            icon={<Edit3 size={15} />}
                            onClick={() => {
                              setEditingVendor(r)
                              vendorForm.setFieldsValue({
                                name: r.name,
                                icon: r.icon || '',
                                description: r.description || '',
                                status_bool: r.status === 1,
                              })
                              setVendorModalOpen(true)
                            }}
                          />
                          <Popconfirm
                            title="确认删除该供应商？"
                            onConfirm={async () => {
                              await api.del(`/api/vendors/${r.id}`)
                              message.success('已删除')
                              void loadVendors()
                            }}
                          >
                            <Button type="text" size="small" danger icon={<Trash2 size={15} />} />
                          </Popconfirm>
                        </Space>
                      ),
                    },
                  ]}
                />
              </div>
            ),
          },
          {
            key: 'prefills',
            label: `预填充组 (${prefills.length})`,
            children: (
              <div>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 14 }}>
                  <Select
                    allowClear
                    placeholder="按类型过滤 (model / tag / endpoint)"
                    style={{ width: 240 }}
                    value={prefillTypeFilter}
                    onChange={setPrefillTypeFilter}
                    options={[
                      { value: 'model', label: '模型组 (model)' },
                      { value: 'tag', label: '标签组 (tag)' },
                      { value: 'endpoint', label: '端点组 (endpoint)' },
                    ]}
                  />
                  <Space>
                    <Button icon={<RefreshCw size={14} />} onClick={() => void loadPrefills()} />
                    <Button
                      type="primary"
                      icon={<Plus size={15} />}
                      onClick={() => {
                        setEditingPrefill(null)
                        prefillForm.resetFields()
                        prefillForm.setFieldsValue({ type: 'model', items_json: '[\n  "gpt-4o"\n]' })
                        setPrefillModalOpen(true)
                      }}
                    >
                      新增预填充组
                    </Button>
                  </Space>
                </div>

                <Table<PrefillGroup>
                  rowKey="id"
                  loading={prefillsLoading}
                  dataSource={prefills}
                  columns={[
                    { title: 'ID', dataIndex: 'id', width: 70 },
                    { title: '组名称', dataIndex: 'name', width: 200 },
                    {
                      title: '类型',
                      dataIndex: 'type',
                      width: 110,
                      render: (t: string) => <Tag color="cyan">{t}</Tag>,
                    },
                    { title: '描述', dataIndex: 'description', width: 220 },
                    {
                      title: '条目预览',
                      render: (_, r) => (
                        <Typography.Text code ellipsis style={{ maxWidth: 360 }}>
                          {JSON.stringify(r.items)}
                        </Typography.Text>
                      ),
                    },
                    {
                      title: '操作',
                      width: 110,
                      render: (_, r) => (
                        <Space size={4}>
                          <Button
                            type="text"
                            size="small"
                            icon={<Edit3 size={15} />}
                            onClick={() => {
                              setEditingPrefill(r)
                              prefillForm.setFieldsValue({
                                name: r.name,
                                type: r.type,
                                description: r.description || '',
                                items_json: JSON.stringify(r.items, null, 2),
                              })
                              setPrefillModalOpen(true)
                            }}
                          />
                          <Popconfirm
                            title="确认删除该预填充组？"
                            onConfirm={async () => {
                              await api.del(`/api/prefill_group/${r.id}`)
                              message.success('已删除')
                              void loadPrefills()
                            }}
                          >
                            <Button type="text" size="small" danger icon={<Trash2 size={15} />} />
                          </Popconfirm>
                        </Space>
                      ),
                    },
                  ]}
                />
              </div>
            ),
          },
        ]}
      />

      {/* 模型元数据弹窗 */}
      <Modal
        title={editingModel ? `编辑模型：${editingModel.model_name}` : '新增模型元数据'}
        open={modelModalOpen}
        onCancel={() => setModelModalOpen(false)}
        onOk={() => void saveModel()}
        width={620}
      >
        <Form form={modelForm} layout="vertical">
          <Row gutter={16}>
            <Col span={14}>
              <Form.Item name="model_name" label="模型名称" rules={[{ required: true }]}>
                <Input placeholder="例如 deepseek-chat / gpt-4o" />
              </Form.Item>
            </Col>
            <Col span={10}>
              <Form.Item name="name_rule" label="匹配规则">
                <Select options={NAME_RULES} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="vendor_id" label="所属供应商">
                <Select
                  allowClear
                  placeholder="选择供应商"
                  options={vendors.map((v) => ({ value: v.id, label: v.name }))}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="icon" label="图标名称（可选）">
                <Input placeholder="例如 OpenAI / DeepSeek" />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="tags_list" label="能力标签">
            <Select
              mode="tags"
              placeholder="输入标签回车，如 对话、视觉、推理、代码..."
            />
          </Form.Item>
          <Form.Item name="description" label="模型简介">
            <Input.TextArea rows={2} />
          </Form.Item>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="status_bool" label="启用状态" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="sync_official_bool" label="允许官方目录同步覆盖" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>

      {/* 供应商弹窗 */}
      <Modal
        title={editingVendor ? '编辑供应商' : '新增供应商'}
        open={vendorModalOpen}
        onCancel={() => setVendorModalOpen(false)}
        onOk={() => void saveVendor()}
      >
        <Form form={vendorForm} layout="vertical">
          <Form.Item name="name" label="供应商名称" rules={[{ required: true }]}>
            <Input placeholder="例如 DeepSeek / Anthropic / OpenAI" />
          </Form.Item>
          <Form.Item name="icon" label="图标标识（可选）">
            <Input placeholder="例如 DeepSeek" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} />
          </Form.Item>
          <Form.Item name="status_bool" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      {/* 预填充组弹窗 */}
      <Modal
        title={editingPrefill ? '编辑预填充组' : '新增预填充组'}
        open={prefillModalOpen}
        onCancel={() => setPrefillModalOpen(false)}
        onOk={() => void savePrefill()}
      >
        <Form form={prefillForm} layout="vertical">
          <Row gutter={12}>
            <Col span={14}>
              <Form.Item name="name" label="组名称" rules={[{ required: true }]}>
                <Input placeholder="例如 常用对话模型组" />
              </Form.Item>
            </Col>
            <Col span={10}>
              <Form.Item name="type" label="类型" rules={[{ required: true }]}>
                <Select
                  options={[
                    { value: 'model', label: 'model' },
                    { value: 'tag', label: 'tag' },
                    { value: 'endpoint', label: 'endpoint' },
                  ]}
                />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="description" label="说明">
            <Input />
          </Form.Item>
          <Form.Item
            name="items_json"
            label="条目内容 (JSON 数组或每行一项文本)"
            rules={[{ required: true }]}
          >
            <Input.TextArea rows={5} style={{ fontFamily: 'monospace' }} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 未登记模型扫描弹窗 */}
      <Modal
        title={`渠道中尚未登记元数据的模型 (${missingModels.length})`}
        open={missingOpen}
        onCancel={() => setMissingOpen(false)}
        footer={null}
      >
        {missingModels.length === 0 ? (
          <Typography.Text type="secondary">所有已启用渠道的模型均已配置元数据。</Typography.Text>
        ) : (
          <Space size={[6, 8]} wrap>
            {missingModels.map((m) => (
              <Tag
                key={m}
                color="orange"
                style={{ cursor: 'pointer', padding: '4px 8px' }}
                onClick={() => {
                  setMissingOpen(false)
                  openCreateModel(m)
                }}
              >
                + 点击登记：{m}
              </Tag>
            ))}
          </Space>
        )}
      </Modal>

      {/* 上游模型同步预览弹窗 */}
      <Modal
        title="从已配置渠道同步模型元数据"
        open={syncOpen}
        onCancel={() => setSyncOpen(false)}
        onOk={() => void executeSyncUpstream()}
        confirmLoading={syncLoading}
        okText="确认导入同步"
      >
        {syncPreview ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Typography.Text>
              预计新增 <strong>{syncPreview.create?.length ?? 0}</strong> 个模型，更新{' '}
              <strong>{syncPreview.update?.length ?? 0}</strong> 个模型（总扫描 {syncPreview.total}{' '}
              项）。
            </Typography.Text>
          </div>
        ) : (
          <Typography.Text type="secondary">正在分析渠道模型目录...</Typography.Text>
        )}
      </Modal>
    </Card>
  )
}

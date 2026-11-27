import React, { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Col,
  Dropdown,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
  App,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { Edit3, MoreHorizontal, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { api } from '../../api/client'
import { getRoleInfo } from '../../constants/channel'
import { useApp } from '../../contexts/AppContext'
import type { PageData, User } from '../../types/api'

export const UserPage: React.FC = () => {
  const { user: currentUser, isRoot } = useApp()
  const { message } = App.useApp()

  const [loading, setLoading] = useState(false)
  const [users, setUsers] = useState<User[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [keyword, setKeyword] = useState('')

  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<User | null>(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm()

  const loadUsers = useCallback(async () => {
    setLoading(true)
    try {
      const path = keyword.trim() ? '/api/user/search' : '/api/user/'
      const res = await api.get<PageData<User>>(path, {
        p: page,
        page_size: pageSize,
        keyword: keyword.trim() || undefined,
      })
      setUsers(res.items || [])
      setTotal(res.total || 0)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载用户列表失败')
    } finally {
      setLoading(false)
    }
  }, [keyword, message, page, pageSize])

  useEffect(() => {
    void loadUsers()
  }, [loadUsers])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ role: 1 })
    setModalOpen(true)
  }

  const openEdit = (u: User) => {
    setEditing(u)
    form.setFieldsValue({
      username: u.username,
      display_name: u.display_name || '',
      email: u.email || '',
      quota: u.quota ?? 0,
      remark: u.remark || '',
      password: '',
    })
    setModalOpen(true)
  }

  const handleSave = async () => {
    const vals = await form.validateFields()
    setSaving(true)
    try {
      if (editing) {
        await api.put('/api/user/', {
          id: editing.id,
          username: vals.username.trim(),
          display_name: (vals.display_name || '').trim(),
          email: (vals.email || '').trim(),
          quota: Number(vals.quota || 0),
          remark: (vals.remark || '').trim(),
          password: (vals.password || '').trim(),
        })
        message.success('用户信息已保存')
      } else {
        await api.post('/api/user/', {
          username: vals.username.trim(),
          password: vals.password,
          display_name: (vals.display_name || '').trim(),
          role: vals.role || 1,
          remark: (vals.remark || '').trim(),
        })
        message.success('用户已创建')
      }
      setModalOpen(false)
      void loadUsers()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存用户失败')
    } finally {
      setSaving(false)
    }
  }

  const manageUser = async (id: number, action: 'enable' | 'disable' | 'promote' | 'demote') => {
    try {
      await api.post('/api/user/manage', { id, action })
      message.success('操作成功')
      void loadUsers()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    }
  }

  const columns: ColumnsType<User> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    {
      title: '用户名 / 昵称',
      width: 200,
      render: (_, r) => (
        <div>
          <Typography.Text strong>{r.username}</Typography.Text>
          {r.display_name && r.display_name !== r.username && (
            <Typography.Text type="secondary" style={{ marginLeft: 6, fontSize: 12 }}>
              ({r.display_name})
            </Typography.Text>
          )}
        </div>
      ),
    },
    {
      title: '角色',
      dataIndex: 'role',
      width: 110,
      render: (role: number) => {
        const info = getRoleInfo(role)
        return <Tag color={info.color}>{info.label}</Tag>
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (s: number) =>
        s === 1 ? <Tag color="success">启用</Tag> : <Tag color="error">已封禁</Tag>,
    },
    {
      title: '额度 (已用 / 剩余)',
      width: 170,
      render: (_, r) => `${r.used_quota ?? 0} / ${r.quota ?? 0}`,
    },
    {
      title: '请求数',
      dataIndex: 'request_count',
      width: 95,
      render: (v?: number) => v ?? 0,
    },
    { title: '备注', dataIndex: 'remark', ellipsis: true },
    {
      title: '操作',
      width: 150,
      fixed: 'right',
      render: (_, r) => {
        const isSelf = r.id === currentUser?.id
        const targetIsRoot = r.role >= 100
        return (
          <Space size={4}>
            <Tooltip title="编辑 / 重置密码">
              <Button
                type="text"
                size="small"
                disabled={targetIsRoot && !isSelf}
                icon={<Edit3 size={15} />}
                onClick={() => openEdit(r)}
              />
            </Tooltip>
            {!isSelf && !targetIsRoot && (
              <>
                <Dropdown
                  menu={{
                    items: [
                      r.status === 1
                        ? {
                            key: 'disable',
                            danger: true,
                            label: '封禁账号（强制踢下线）',
                            onClick: () => void manageUser(r.id, 'disable'),
                          }
                        : {
                            key: 'enable',
                            label: '恢复启用账号',
                            onClick: () => void manageUser(r.id, 'enable'),
                          },
                      ...(isRoot
                        ? [
                            r.role < 10
                              ? {
                                  key: 'promote',
                                  label: '提升为管理员',
                                  onClick: () => void manageUser(r.id, 'promote'),
                                }
                              : {
                                  key: 'demote',
                                  label: '降级为普通用户',
                                  onClick: () => void manageUser(r.id, 'demote'),
                                },
                          ]
                        : []),
                    ],
                  }}
                >
                  <Button type="text" size="small" icon={<MoreHorizontal size={15} />} />
                </Dropdown>
                <Popconfirm
                  title={`确认删除用户「${r.username}」？`}
                  onConfirm={async () => {
                    await api.del(`/api/user/${r.id}`)
                    message.success('用户已删除')
                    void loadUsers()
                  }}
                >
                  <Button type="text" size="small" danger icon={<Trash2 size={15} />} />
                </Popconfirm>
              </>
            )}
          </Space>
        )
      },
    },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Alert
        type="info"
        showIcon
        message="安全联动：封禁、删除用户、调整角色或重置密码时，系统会自动销毁该用户在 Redis 中的全部活跃登录会话，并通过 Pub/Sub 通知全集群节点实时剔除本地 userCache。"
      />

      <Card style={{ borderRadius: 12 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: 10, marginBottom: 14 }}>
          <Space>
            <Input.Search
              allowClear
              placeholder="搜索用户名 / 昵称 / 邮箱..."
              style={{ width: 260 }}
              onSearch={(v) => {
                setPage(1)
                setKeyword(v)
              }}
            />
            <Button icon={<RefreshCw size={14} />} onClick={() => void loadUsers()}>
              刷新
            </Button>
          </Space>

          <Button type="primary" icon={<Plus size={15} />} onClick={openCreate}>
            新建用户
          </Button>
        </div>

        <Table<User>
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={users}
          scroll={{ x: 960 }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 位用户`,
            onChange: (p, ps) => {
              setPage(p)
              setPageSize(ps)
            },
          }}
        />
      </Card>

      <Modal
        title={editing ? `编辑用户：${editing.username}` : '创建新用户'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => void handleSave()}
        confirmLoading={saving}
      >
        <Form form={form} layout="vertical">
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
                <Input placeholder="1-20 位字符" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="display_name" label="显示昵称">
                <Input placeholder="可选显示名" />
              </Form.Item>
            </Col>
          </Row>

          <Form.Item
            name="password"
            label={editing ? '重置密码（留空则不修改密码）' : '初始密码'}
            rules={editing ? [] : [{ required: true, min: 8, message: '密码至少 8 位' }]}
          >
            <Input.Password placeholder="8-64 位字符" />
          </Form.Item>

          {!editing && isRoot && (
            <Form.Item name="role" label="初始角色">
              <Select
                options={[
                  { value: 1, label: '普通用户' },
                  { value: 10, label: '管理员' },
                ]}
              />
            </Form.Item>
          )}

          {editing && (
            <Row gutter={12}>
              <Col span={12}>
                <Form.Item name="email" label="邮箱">
                  <Input />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item name="quota" label="账户额度 (Quota)">
                  <InputNumber min={0} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
            </Row>
          )}

          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

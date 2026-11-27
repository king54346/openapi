import React, { useEffect, useMemo, useState } from 'react'
import {
  Avatar,
  Badge,
  Button,
  Drawer,
  Dropdown,
  Layout,
  Menu,
  Modal,
  Space,
  Tag,
  Tooltip,
  Typography,
  Grid,
} from 'antd'
import type { MenuProps } from 'antd'
import {
  Activity,
  Bell,
  Bot,
  Clock,
  Cpu,
  FileText,
  FlaskConical,
  KeyRound,
  LayoutDashboard,
  ListChecks,
  LogOut,
  Menu as MenuIcon,
  Moon,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Sun,
  User as UserIcon,
  Users,
} from 'lucide-react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api } from '../../api/client'
import { getRoleInfo } from '../../constants/channel'
import { useApp } from '../../contexts/AppContext'

const { Header, Sider, Content } = Layout
const { useBreakpoint } = Grid

const COLLAPSE_KEY = 'openapi_sider_collapsed'

export const AppLayout: React.FC = () => {
  const { user, isAdmin, isRoot, publicStatus, darkMode, toggleDarkMode, logout } = useApp()
  const location = useLocation()
  const navigate = useNavigate()
  const screens = useBreakpoint()
  const isMobile = !screens.md

  const [collapsed, setCollapsed] = useState<boolean>(() => localStorage.getItem(COLLAPSE_KEY) === 'true')
  const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false)
  const [noticeOpen, setNoticeOpen] = useState(false)
  const [noticeContent, setNoticeContent] = useState('')

  const isConsoleRoute = location.pathname.startsWith('/console')

  const toggleCollapse = () => {
    setCollapsed((prev) => {
      const next = !prev
      localStorage.setItem(COLLAPSE_KEY, String(next))
      return next
    })
  }

  const openNotice = async () => {
    setNoticeOpen(true)
    try {
      const text = await api.get<string>('/api/notice', undefined, true)
      setNoticeContent(text || '暂无系统公告')
    } catch {
      setNoticeContent('暂无系统公告')
    }
  }

  useEffect(() => {
    setMobileDrawerOpen(false)
  }, [location.pathname])

  // 参照 new-api/web 的四段式侧边栏分组（聊天体验 / 控制台 / 个人中心 / 管理员），仅保留后端已实现的全部核心能力
  const sidebarItems = useMemo<MenuProps['items']>(() => {
    const groups: NonNullable<MenuProps['items']> = [
      {
        type: 'group',
        label: collapsed ? null : '聊天与体验',
        children: [
          {
            key: '/console/playground',
            icon: <Bot size={16} />,
            label: '体验中心',
          },
        ],
      },
      { type: 'divider' },
      {
        type: 'group',
        label: collapsed ? null : '控制台',
        children: [
          {
            key: '/console',
            icon: <LayoutDashboard size={16} />,
            label: '数据看板',
          },
          {
            key: '/console/token',
            icon: <KeyRound size={16} />,
            label: '令牌管理',
          },
          {
            key: '/console/log',
            icon: <FileText size={16} />,
            label: '使用日志',
          },
          {
            key: '/console/task',
            icon: <ListChecks size={16} />,
            label: '任务日志',
          },
        ],
      },
      { type: 'divider' },
      {
        type: 'group',
        label: collapsed ? null : '个人中心',
        children: [
          {
            key: '/console/personal',
            icon: <UserIcon size={16} />,
            label: '个人设置',
          },
        ],
      },
    ]

    if (isAdmin) {
      const adminChildren: NonNullable<MenuProps['items']> = [
        {
          key: '/console/channel',
          icon: <Network size={16} />,
          label: '渠道管理',
        },
        {
          key: '/console/channel-test',
          icon: <FlaskConical size={16} />,
          label: '测试管理',
        },
        {
          key: '/console/cron-job',
          icon: <Clock size={16} />,
          label: '定时任务',
        },
        {
          key: '/console/models',
          icon: <Cpu size={16} />,
          label: '模型管理',
        },
        {
          key: '/console/user',
          icon: <Users size={16} />,
          label: '用户管理',
        },
      ]
      if (isRoot) {
        adminChildren.push({
          key: '/console/setting',
          icon: <Settings size={16} />,
          label: '系统设置',
        })
      }
      groups.push(
        { type: 'divider' },
        {
          type: 'group',
          label: collapsed ? null : '管理员',
          children: adminChildren,
        },
      )
    }

    return groups
  }, [collapsed, isAdmin, isRoot])

  const selectedSidebarKey = useMemo(() => {
    const p = location.pathname.replace(/\/+$/, '') || '/'
    if (p === '/console') return '/console'
    const prefixes = [
      '/console/playground',
      '/console/token',
      '/console/log',
      '/console/task',
      '/console/personal',
      '/console/channel-test',
      '/console/channel',
      '/console/cron-job',
      '/console/models',
      '/console/user',
      '/console/setting',
    ]
    return prefixes.find((item) => p === item || p.startsWith(`${item}/`)) ?? '/console'
  }, [location.pathname])

  const userDropdownItems: MenuProps['items'] = user
    ? [
        {
          key: 'info',
          disabled: true,
          label: (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
              <Typography.Text strong>{user.display_name || user.username}</Typography.Text>
              <Space size={4}>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  @{user.username}
                </Typography.Text>
                <Tag color={getRoleInfo(user.role).color} style={{ marginInlineEnd: 0 }}>
                  {getRoleInfo(user.role).label}
                </Tag>
              </Space>
            </div>
          ),
        },
        { type: 'divider' },
        {
          key: 'console',
          icon: <LayoutDashboard size={15} />,
          label: '进入控制台',
          onClick: () => navigate('/console'),
        },
        {
          key: 'personal',
          icon: <UserIcon size={15} />,
          label: '个人设置',
          onClick: () => navigate('/console/personal'),
        },
        { type: 'divider' },
        {
          key: 'logout',
          danger: true,
          icon: <LogOut size={15} />,
          label: '退出登录',
          onClick: async () => {
            await logout()
            navigate('/login')
          },
        },
      ]
    : []

  const siderMenuNode = (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, overflowY: 'auto', padding: '8px 6px' }}>
        <Menu
          mode="inline"
          inlineCollapsed={!isMobile && collapsed}
          selectedKeys={[selectedSidebarKey]}
          items={sidebarItems}
          onClick={({ key }) => navigate(key)}
          style={{ borderInlineEnd: 'none', background: 'transparent' }}
        />
      </div>
      {!isMobile && (
        <div
          style={{
            padding: '10px 12px',
            borderTop: '1px solid var(--border-subtle)',
            display: 'flex',
            justifyContent: collapsed ? 'center' : 'flex-end',
          }}
        >
          <Tooltip title={collapsed ? '展开侧边栏' : '收起侧边栏'} placement="right">
            <Button
              type="text"
              size="small"
              icon={collapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
              onClick={toggleCollapse}
            />
          </Tooltip>
        </div>
      )}
    </div>
  )

  return (
    <Layout style={{ minHeight: '100vh', background: 'var(--bg-page)' }}>
      <Header
        style={{
          position: 'sticky',
          top: 0,
          zIndex: 100,
          height: 60,
          lineHeight: '60px',
          padding: isMobile ? '0 12px' : '0 24px',
          background: 'var(--bg-surface)',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <Space size={isMobile ? 8 : 24} align="center">
          {isMobile && isConsoleRoute && (
            <Button
              type="text"
              icon={<MenuIcon size={18} />}
              onClick={() => setMobileDrawerOpen(true)}
            />
          )}
          <Link
            to="/"
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 10,
              textDecoration: 'none',
              color: 'inherit',
            }}
          >
            <div
              style={{
                width: 32,
                height: 32,
                borderRadius: 8,
                background: 'linear-gradient(135deg, #1677ff 0%, #4f46e5 100%)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: '#fff',
              }}
            >
              <Activity size={18} />
            </div>
            <Typography.Text strong style={{ fontSize: 17 }}>
              OpenAPI Gateway
            </Typography.Text>
            {publicStatus?.version && !isMobile && (
              <Tag color="processing" style={{ margin: 0 }}>
                {publicStatus.version}
              </Tag>
            )}
          </Link>

          {!isMobile && (
            <Space size={4}>
              <Button
                type={location.pathname === '/' ? 'primary' : 'text'}
                ghost={location.pathname === '/'}
                onClick={() => navigate('/')}
              >
                首页
              </Button>
              <Button
                type={isConsoleRoute ? 'primary' : 'text'}
                ghost={isConsoleRoute}
                onClick={() => navigate('/console')}
              >
                控制台
              </Button>
              <Button
                type={location.pathname === '/about' ? 'primary' : 'text'}
                ghost={location.pathname === '/about'}
                onClick={() => navigate('/about')}
              >
                关于
              </Button>
            </Space>
          )}
        </Space>

        <Space size={8}>
          <Tooltip title="系统公告">
            <Button
              type="text"
              icon={
                <Badge dot={Boolean(publicStatus)}>
                  <Bell size={17} />
                </Badge>
              }
              onClick={() => void openNotice()}
            />
          </Tooltip>
          <Tooltip title={darkMode ? '切换浅色模式' : '切换深色模式'}>
            <Button
              type="text"
              icon={darkMode ? <Sun size={17} /> : <Moon size={17} />}
              onClick={toggleDarkMode}
            />
          </Tooltip>

          {user ? (
            <Dropdown menu={{ items: userDropdownItems }} trigger={['click']} placement="bottomRight">
              <Button type="text" style={{ height: 40, paddingInline: 8 }}>
                <Space size={8}>
                  <Avatar size={28} style={{ backgroundColor: '#1677ff' }}>
                    {(user.display_name || user.username || 'U').slice(0, 1).toUpperCase()}
                  </Avatar>
                  {!isMobile && (
                    <Typography.Text style={{ maxWidth: 120 }} ellipsis>
                      {user.display_name || user.username}
                    </Typography.Text>
                  )}
                </Space>
              </Button>
            </Dropdown>
          ) : (
            <Button type="primary" onClick={() => navigate('/login')}>
              登录
            </Button>
          )}
        </Space>
      </Header>

      <Layout style={{ background: 'var(--bg-page)' }}>
        {isConsoleRoute && !isMobile && (
          <Sider
            width={228}
            collapsedWidth={64}
            collapsed={collapsed}
            trigger={null}
            style={{
              background: 'var(--bg-surface)',
              borderRight: '1px solid var(--border-subtle)',
              position: 'sticky',
              top: 60,
              height: 'calc(100vh - 60px)',
              overflow: 'hidden',
            }}
          >
            {siderMenuNode}
          </Sider>
        )}

        {isConsoleRoute && isMobile && (
          <Drawer
            title="导航菜单"
            placement="left"
            width={240}
            open={mobileDrawerOpen}
            onClose={() => setMobileDrawerOpen(false)}
            styles={{ body: { padding: 0 } }}
          >
            {siderMenuNode}
          </Drawer>
        )}

        <Content
          style={{
            padding: location.pathname === '/console/playground' ? 0 : isMobile ? 12 : 24,
            minHeight: 'calc(100vh - 60px)',
            overflowX: 'hidden',
          }}
        >
          <Outlet />
        </Content>
      </Layout>

      <Modal
        title="系统公告"
        open={noticeOpen}
        onCancel={() => setNoticeOpen(false)}
        footer={[
          <Button key="close" type="primary" onClick={() => setNoticeOpen(false)}>
            我知道了
          </Button>,
        ]}
      >
        <div className="markdown-prose">{noticeContent || '暂无公告'}</div>
      </Modal>
    </Layout>
  )
}

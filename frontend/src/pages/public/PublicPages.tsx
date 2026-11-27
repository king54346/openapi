import React, { useEffect, useState } from 'react'
import { Button, Card, Col, Result, Row, Skeleton, Space, Tag, Typography } from 'antd'
import { ArrowRight, Bot, Cpu, Layers, Network, ShieldCheck, Zap } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../../api/client'
import { useApp } from '../../contexts/AppContext'

export const HomePage: React.FC = () => {
  const { user, publicStatus } = useApp()
  const navigate = useNavigate()
  const [customContent, setCustomContent] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api
      .get<string>('/api/home_page_content', undefined, true)
      .then((res) => setCustomContent(res || ''))
      .catch(() => setCustomContent(''))
      .finally(() => setLoading(false))
  }, [])

  const serverAddr = publicStatus?.server_address || window.location.origin

  const features = [
    {
      icon: <Layers size={22} color="#1677ff" />,
      title: '全协议原生兼容',
      desc: '同时支持 OpenAI Chat / Responses、Anthropic Claude Messages、Google Gemini Native、Embeddings、Rerank、图像与异步视频任务。',
    },
    {
      icon: <Zap size={22} color="#722ed1" />,
      title: 'Redis 集群级低延迟架构',
      desc: '进程内 L1 鉴权缓存 + Redis Pub/Sub 毫秒级失效广播、跨节点会话亲和性与多密钥零写库原子轮询。',
    },
    {
      icon: <Network size={22} color="#13c2c2" />,
      title: '智能选路与自动熔断',
      desc: '按优先级、权重与会话亲和度智能路由，集群级连续失败自动封禁异常渠道并支持定时测速恢复。',
    },
    {
      icon: <ShieldCheck size={22} color="#52c41a" />,
      title: '精细化令牌权限管控',
      desc: '支持单令牌模型黑白名单、IP 白名单、访问限制模板复用及独立调用统计分析。',
    },
  ]

  return (
    <div style={{ maxWidth: 1160, margin: '0 auto', padding: '24px 12px 48px' }}>
      <Card
        style={{
          borderRadius: 18,
          marginBottom: 24,
          background:
            'linear-gradient(135deg, rgba(22,119,255,0.09) 0%, rgba(79,70,229,0.08) 100%)',
        }}
      >
        <div style={{ padding: '24px 12px', textAlign: 'center' }}>
          <Space size={8} style={{ marginBottom: 12 }}>
            <Tag color="blue">OpenAI / Claude / Gemini 多协议网关</Tag>
            {publicStatus?.channel_affinity && <Tag color="purple">会话亲和已开启</Tag>}
          </Space>
          <Typography.Title level={1} style={{ marginTop: 8, marginBottom: 12 }}>
            高性能企业级 AI 模型中继网关
          </Typography.Title>
          <Typography.Paragraph
            type="secondary"
            style={{ fontSize: 16, maxWidth: 680, margin: '0 auto 28px' }}
          >
            一站式聚合主流大模型供应商，统一标准接口接入，内置体验中心、多密钥池轮询、跨节点亲和路由与全链路观测。
          </Typography.Paragraph>

          <Space size={12} wrap style={{ justifyContent: 'center' }}>
            <Button
              type="primary"
              size="large"
              icon={<ArrowRight size={16} />}
              onClick={() => navigate(user ? '/console' : '/login')}
            >
              {user ? '进入控制台' : '立即登录'}
            </Button>
            <Button
              size="large"
              icon={<Bot size={16} />}
              onClick={() => navigate(user ? '/console/playground' : '/login')}
            >
              打开体验中心
            </Button>
          </Space>

          <div style={{ marginTop: 28, maxWidth: 640, marginInline: 'auto', textAlign: 'left' }}>
            <div className="code-block">
              {`curl ${serverAddr}/v1/chat/completions \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer sk-YOUR_API_KEY" \\\n  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hello!"}]}'`}
            </div>
          </div>
        </div>
      </Card>

      {!loading && customContent.trim() !== '' && (
        <Card title="站点说明" style={{ borderRadius: 14, marginBottom: 24 }}>
          <div className="markdown-prose">{customContent}</div>
        </Card>
      )}

      <Row gutter={[16, 16]}>
        {features.map((item) => (
          <Col xs={24} sm={12} key={item.title}>
            <Card style={{ height: '100%', borderRadius: 14 }}>
              <Space align="start" size={14}>
                <div
                  style={{
                    width: 42,
                    height: 42,
                    borderRadius: 10,
                    background: 'rgba(22, 119, 255, 0.08)',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    flexShrink: 0,
                  }}
                >
                  {item.icon}
                </div>
                <div>
                  <Typography.Title level={5} style={{ marginTop: 0, marginBottom: 6 }}>
                    {item.title}
                  </Typography.Title>
                  <Typography.Text type="secondary">{item.desc}</Typography.Text>
                </div>
              </Space>
            </Card>
          </Col>
        ))}
      </Row>

      <div
        style={{
          marginTop: 40,
          textAlign: 'center',
          color: 'var(--text-secondary)',
          fontSize: 13,
        }}
      >
        <Space split={<span>·</span>} size={12}>
          <span>OpenAPI {publicStatus?.version || ''}</span>
          <Link to="/about">关于</Link>
          <Link to="/user-agreement">用户协议</Link>
          <Link to="/privacy-policy">隐私政策</Link>
        </Space>
      </div>
    </div>
  )
}

const PublicDocViewer: React.FC<{ title: string; endpoint: string; fallback: string }> = ({
  title,
  endpoint,
  fallback,
}) => {
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    api
      .get<string>(endpoint, undefined, true)
      .then((res) => setContent(res || ''))
      .catch(() => setContent(''))
      .finally(() => setLoading(false))
  }, [endpoint])

  return (
    <div style={{ maxWidth: 900, margin: '0 auto' }}>
      <Card
        title={
          <Space>
            <Cpu size={18} />
            <span>{title}</span>
          </Space>
        }
        style={{ borderRadius: 14 }}
      >
        {loading ? (
          <Skeleton active paragraph={{ rows: 6 }} />
        ) : (
          <div className="markdown-prose">{content.trim() || fallback}</div>
        )}
      </Card>
    </div>
  )
}

export const AboutPage: React.FC = () => (
  <PublicDocViewer
    title="关于系统"
    endpoint="/api/about"
    fallback="OpenAPI Gateway — 高性能多协议企业级大模型 API 网关。"
  />
)

export const UserAgreementPage: React.FC = () => (
  <PublicDocViewer
    title="用户服务协议"
    endpoint="/api/user-agreement"
    fallback="管理员尚未配置用户服务协议内容。"
  />
)

export const PrivacyPolicyPage: React.FC = () => (
  <PublicDocViewer
    title="隐私政策"
    endpoint="/api/privacy-policy"
    fallback="管理员尚未配置隐私政策内容。"
  />
)

export const NotFoundPage: React.FC = () => {
  const navigate = useNavigate()
  return (
    <Result
      status="404"
      title="404"
      subTitle="抱歉，您访问的页面不存在"
      extra={
        <Button type="primary" onClick={() => navigate('/')}>
          返回首页
        </Button>
      }
    />
  )
}

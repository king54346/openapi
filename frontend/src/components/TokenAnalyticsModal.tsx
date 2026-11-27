import React, { useEffect, useId, useMemo, useState } from 'react'
import { Button, Col, Empty, Modal, Row, Spin, Table, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { BarChart3, CircleDollarSign, Cpu, TrendingUp, Zap } from 'lucide-react'
import { api } from '../api/client'
import type { Token, TokenAnalytics, TokenDailyStat, TokenModelStat } from '../types/api'

const QUOTA_PER_UNIT = 500000

function formatCompactTokens(num: number): string {
  if (!num || num <= 0) return '0'
  if (num >= 1_000_000_000) return `${(num / 1_000_000_000).toFixed(2)}B`
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(2)}M`
  if (num >= 1_000) return `${(num / 1_000).toFixed(2)}K`
  return num.toLocaleString()
}

function niceCeil(maxVal: number, ticks = 5): number {
  if (maxVal <= 0) return ticks
  const rawStep = maxVal / ticks
  const mag = Math.pow(10, Math.floor(Math.log10(rawStep)))
  const norm = rawStep / mag
  let niceNorm = 10
  if (norm <= 1) niceNorm = 1
  else if (norm <= 2) niceNorm = 2
  else if (norm <= 2.5) niceNorm = 2.5
  else if (norm <= 5) niceNorm = 5
  return niceNorm * mag * ticks
}

function buildSmoothSvgPath(points: Array<{ x: number; y: number }>, baselineY: number): {
  line: string
  area: string
} {
  if (points.length === 0) return { line: '', area: '' }
  if (points.length === 1) {
    const p = points[0]
    return {
      line: `M ${p.x} ${p.y}`,
      area: `M ${p.x} ${baselineY} L ${p.x} ${p.y} L ${p.x} ${baselineY} Z`,
    }
  }

  // Monotone X cubic bezier so curves never overshoot below 0 baseline
  let line = `M ${points[0].x.toFixed(2)} ${points[0].y.toFixed(2)}`
  for (let i = 0; i < points.length - 1; i++) {
    const p0 = points[i]
    const p1 = points[i + 1]
    const dx = (p1.x - p0.x) * 0.42
    const c1x = p0.x + dx
    const c1y = p0.y
    const c2x = p1.x - dx
    const c2y = p1.y
    line += ` C ${c1x.toFixed(2)} ${c1y.toFixed(2)}, ${c2x.toFixed(2)} ${c2y.toFixed(2)}, ${p1.x.toFixed(2)} ${p1.y.toFixed(2)}`
  }

  const first = points[0]
  const last = points[points.length - 1]
  const area = `${line} L ${last.x.toFixed(2)} ${baselineY.toFixed(2)} L ${first.x.toFixed(2)} ${baselineY.toFixed(2)} Z`
  return { line, area }
}

interface DualTrendChartProps {
  daily: TokenDailyStat[]
}

const DualTrendChart: React.FC<DualTrendChartProps> = ({ daily }) => {
  const gradId = useId().replace(/:/g, '')
  const [hoverIdx, setHoverIdx] = useState<number | null>(null)

  const width = 760
  const height = 230
  const padLeft = 48
  const padRight = 64
  const padTop = 30
  const padBottom = 30
  const plotW = width - padLeft - padRight
  const plotH = height - padTop - padBottom
  const baseY = padTop + plotH
  const ticks = 5

  const series = useMemo(
    () =>
      daily.map((d) => {
        const calls = d.call_count ?? d.calls ?? 0
        const prompt = d.prompt_tokens ?? 0
        const completion = d.completion_tokens ?? 0
        return {
          date: d.date,
          shortDate: d.date.length >= 10 ? d.date.slice(5) : d.date,
          calls,
          prompt,
          completion,
          tokens: prompt + completion,
          quota: d.quota ?? 0,
        }
      }),
    [daily],
  )

  const maxCalls = useMemo(
    () => niceCeil(Math.max(...series.map((s) => s.calls), 5), ticks),
    [series],
  )
  const maxTokens = useMemo(
    () => niceCeil(Math.max(...series.map((s) => s.tokens), 100), ticks),
    [series],
  )

  if (series.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无趋势数据" />
  }

  const slotW = plotW / series.length
  const barW = Math.min(26, Math.max(8, slotW * 0.34))
  const labelStride = series.length > 20 ? 3 : series.length > 12 ? 2 : 1

  const tokenPoints = series.map((item, idx) => ({
    x: padLeft + slotW * idx + slotW / 2,
    y: baseY - (item.tokens / maxTokens) * plotH,
  }))
  const { line: tokenLinePath, area: tokenAreaPath } = buildSmoothSvgPath(tokenPoints, baseY)

  const activeItem = hoverIdx !== null ? series[hoverIdx] : null

  return (
    <div style={{ position: 'relative', width: '100%' }}>
      {/* 图例 */}
      <div
        style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          gap: 20,
          marginBottom: 4,
          fontSize: 12,
          color: '#64748b',
        }}
      >
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          <span
            style={{
              width: 8,
              height: 8,
              borderRadius: '50%',
              background: '#6e72f3',
              display: 'inline-block',
            }}
          />
          调用次数
        </span>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          <span
            style={{
              width: 8,
              height: 8,
              borderRadius: '50%',
              background: '#10b981',
              display: 'inline-block',
            }}
          />
          Tokens
        </span>
      </div>

      <svg
        viewBox={`0 0 ${width} ${height}`}
        style={{ width: '100%', height: 'auto', display: 'block', overflow: 'visible' }}
        onMouseLeave={() => setHoverIdx(null)}
      >
        <defs>
          <linearGradient id={`tokArea_${gradId}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#10b981" stopOpacity={0.18} />
            <stop offset="100%" stopColor="#10b981" stopOpacity={0.0} />
          </linearGradient>
        </defs>

        {/* 左右轴单位标题 */}
        <text x={padLeft - 8} y={16} fontSize={11} fill="#94a3b8" textAnchor="start">
          调用次数
        </text>
        <text x={width - padRight + 8} y={16} fontSize={11} fill="#94a3b8" textAnchor="end">
          Tokens
        </text>

        {/* 水平虚线网格 + 左右 Y 轴刻度 */}
        {Array.from({ length: ticks + 1 }).map((_, idx) => {
          const ratio = idx / ticks
          const y = baseY - ratio * plotH
          const callTick = Math.round(ratio * maxCalls)
          const tokTick = ratio * maxTokens
          return (
            <g key={idx}>
              <line
                x1={padLeft}
                y1={y}
                x2={width - padRight}
                y2={y}
                stroke={idx === 0 ? '#e2e8f0' : '#f1f5f9'}
                strokeDasharray={idx === 0 ? undefined : '3 3'}
              />
              <text
                x={padLeft - 8}
                y={y + 4}
                fontSize={11}
                fill="#94a3b8"
                textAnchor="end"
              >
                {callTick}
              </text>
              <text
                x={width - padRight + 8}
                y={y + 4}
                fontSize={11}
                fill="#94a3b8"
                textAnchor="start"
              >
                {tokTick === 0 ? '0' : formatCompactTokens(tokTick)}
              </text>
            </g>
          )
        })}

        {/* 柱状图：每日调用次数 */}
        {series.map((item, idx) => {
          const cx = padLeft + slotW * idx + slotW / 2
          const barH = Math.max(0, (item.calls / maxCalls) * plotH)
          const y = baseY - barH
          const isHovered = hoverIdx === idx
          return (
            <g key={item.date}>
              {isHovered && (
                <rect
                  x={padLeft + slotW * idx + 2}
                  y={padTop}
                  width={Math.max(slotW - 4, 4)}
                  height={plotH}
                  rx={6}
                  fill="rgba(99, 102, 241, 0.05)"
                />
              )}
              {barH > 0 && (
                <rect
                  x={cx - barW / 2}
                  y={y}
                  width={barW}
                  height={barH}
                  rx={4}
                  fill={isHovered ? '#585ce5' : '#7377f5'}
                />
              )}
              {(idx % labelStride === 0 || idx === series.length - 1) && (
                <text
                  x={cx}
                  y={baseY + 18}
                  fontSize={11}
                  fill="#64748b"
                  textAnchor="middle"
                >
                  {item.shortDate}
                </text>
              )}
            </g>
          )
        })}

        {/* 平滑面积折线：每日 Tokens */}
        <path d={tokenAreaPath} fill={`url(#tokArea_${gradId})`} />
        <path
          d={tokenLinePath}
          fill="none"
          stroke="#10b981"
          strokeWidth={2.4}
          strokeLinecap="round"
        />

        {/* Hover 圆点与透明交互热区 */}
        {hoverIdx !== null && tokenPoints[hoverIdx] && (
          <circle
            cx={tokenPoints[hoverIdx].x}
            cy={tokenPoints[hoverIdx].y}
            r={4.5}
            fill="#ffffff"
            stroke="#10b981"
            strokeWidth={2.5}
          />
        )}

        {series.map((item, idx) => (
          <rect
            key={`hit_${item.date}`}
            x={padLeft + slotW * idx}
            y={padTop}
            width={slotW}
            height={plotH + 20}
            fill="transparent"
            style={{ cursor: 'pointer' }}
            onMouseEnter={() => setHoverIdx(idx)}
          />
        ))}
      </svg>

      {activeItem && hoverIdx !== null && (
        <div
          style={{
            position: 'absolute',
            top: 28,
            left: hoverIdx > series.length / 2 ? 56 : undefined,
            right: hoverIdx <= series.length / 2 ? 72 : undefined,
            background: 'rgba(15, 23, 42, 0.92)',
            color: '#f8fafc',
            borderRadius: 8,
            padding: '8px 12px',
            fontSize: 12,
            lineHeight: 1.65,
            pointerEvents: 'none',
            boxShadow: '0 8px 20px rgba(15, 23, 42, 0.2)',
            zIndex: 5,
          }}
        >
          <div style={{ fontWeight: 600, marginBottom: 2 }}>{activeItem.date}</div>
          <div>调用次数：{activeItem.calls.toLocaleString()}</div>
          <div>总 Tokens：{formatCompactTokens(activeItem.tokens)}</div>
          <div>消耗额度：¥{(activeItem.quota / QUOTA_PER_UNIT).toFixed(4)}</div>
        </div>
      )}
    </div>
  )
}

interface TokenAnalyticsModalProps {
  open: boolean
  token: Token | null
  onClose: () => void
}

export const TokenAnalyticsModal: React.FC<TokenAnalyticsModalProps> = ({
  open,
  token,
  onClose,
}) => {
  const [days, setDays] = useState<7 | 14 | 30>(7)
  const [loading, setLoading] = useState(false)
  const [data, setData] = useState<TokenAnalytics | null>(null)

  useEffect(() => {
    if (!open || !token) return
    let cancelled = false
    const fetchAnalytics = async () => {
      setLoading(true)
      try {
        const res = await api.get<TokenAnalytics>(`/api/token/analytics/${token.id}`, { days })
        if (!cancelled) {
          setData(res)
        }
      } catch {
        if (!cancelled) {
          setData(null)
        }
      } finally {
        if (!cancelled) {
          setLoading(false)
        }
      }
    }
    void fetchAnalytics()
    return () => {
      cancelled = true
    }
  }, [open, token, days])

  const totalCalls = data?.summary?.total_calls ?? 0
  const promptTokens =
    data?.summary?.total_prompt_tokens ?? data?.summary?.prompt_tokens ?? 0
  const completionTokens =
    data?.summary?.total_completion_tokens ?? data?.summary?.completion_tokens ?? 0
  const totalTokens = promptTokens + completionTokens
  const totalQuota = data?.summary?.total_quota ?? 0
  const modelStats = data?.model_stats ?? []
  const dailyStats = data?.daily_stats ?? []

  const modelColumns: ColumnsType<TokenModelStat> = [
    {
      title: '模型',
      dataIndex: 'model_name',
      render: (name: string) => (
        <span
          style={{
            display: 'inline-block',
            padding: '2px 8px',
            borderRadius: 5,
            background: '#f8fafc',
            border: '1px solid #e2e8f0',
            color: '#334155',
            fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
            fontSize: 12,
          }}
        >
          {name || '-'}
        </span>
      ),
    },
    {
      title: '调用次数',
      key: 'call_count',
      width: 115,
      sorter: (a, b) => (a.call_count ?? a.calls ?? 0) - (b.call_count ?? b.calls ?? 0),
      render: (_, r) => {
        const c = r.call_count ?? r.calls ?? 0
        return (
          <span
            style={{
              display: 'inline-block',
              minWidth: 36,
              textAlign: 'center',
              padding: '1px 10px',
              borderRadius: 6,
              background: '#eff6ff',
              border: '1px solid #bfdbfe',
              color: '#2563eb',
              fontWeight: 600,
              fontSize: 12,
            }}
          >
            {c.toLocaleString()}
          </span>
        )
      },
    },
    {
      title: (
        <span>
          <span style={{ color: '#6366f1', fontWeight: 700, marginRight: 3 }}>↑</span>输入
        </span>
      ),
      dataIndex: 'prompt_tokens',
      width: 145,
      sorter: (a, b) => (a.prompt_tokens ?? 0) - (b.prompt_tokens ?? 0),
      render: (v: number) => (
        <span style={{ fontVariantNumeric: 'tabular-nums', color: '#1e293b', fontWeight: 500 }}>
          {(v ?? 0).toLocaleString()}
        </span>
      ),
    },
    {
      title: (
        <span>
          <span style={{ color: '#10b981', fontWeight: 700, marginRight: 3 }}>↓</span>输出
        </span>
      ),
      dataIndex: 'completion_tokens',
      width: 135,
      sorter: (a, b) => (a.completion_tokens ?? 0) - (b.completion_tokens ?? 0),
      render: (v: number) => (
        <span style={{ fontVariantNumeric: 'tabular-nums', color: '#1e293b', fontWeight: 500 }}>
          {(v ?? 0).toLocaleString()}
        </span>
      ),
    },
    {
      title: '消耗额度',
      dataIndex: 'quota',
      width: 130,
      sorter: (a, b) => (a.quota ?? 0) - (b.quota ?? 0),
      render: (q: number) => (
        <span
          style={{
            fontVariantNumeric: 'tabular-nums',
            color: '#ea580c',
            fontWeight: 600,
          }}
        >
          ¥{((q ?? 0) / QUOTA_PER_UNIT).toFixed(4)}
        </span>
      ),
    },
  ]

  const kpiCardStyle: React.CSSProperties = {
    border: '1px solid #f1f5f9',
    borderRadius: 12,
    padding: '14px 16px',
    background: '#ffffff',
    boxShadow: '0 1px 2px rgba(15, 23, 42, 0.03)',
    height: '100%',
  }

  const sectionCardStyle: React.CSSProperties = {
    border: '1px solid #f1f5f9',
    borderRadius: 14,
    padding: '16px 18px',
    background: '#ffffff',
    boxShadow: '0 1px 2px rgba(15, 23, 42, 0.03)',
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      width={860}
      centered
      title={
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <div
            style={{
              width: 38,
              height: 38,
              borderRadius: 10,
              background: '#eef2ff',
              color: '#6366f1',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <BarChart3 size={20} />
          </div>
          <div style={{ lineHeight: 1.3 }}>
            <div style={{ fontSize: 16, fontWeight: 700, color: '#0f172a' }}>
              {token?.name || data?.token_name || '令牌'}
            </div>
            <div style={{ fontSize: 12, fontWeight: 400, color: '#94a3b8' }}>调用分析</div>
          </div>
        </div>
      }
      footer={
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            paddingTop: 6,
          }}
        >
          <div
            style={{
              display: 'inline-flex',
              border: '1px solid #e2e8f0',
              borderRadius: 8,
              overflow: 'hidden',
            }}
          >
            {([7, 14, 30] as const).map((d, idx) => {
              const active = days === d
              return (
                <button
                  key={d}
                  type="button"
                  onClick={() => setDays(d)}
                  style={{
                    border: 'none',
                    borderLeft: idx > 0 ? '1px solid #e2e8f0' : 'none',
                    background: active ? '#585ce5' : '#ffffff',
                    color: active ? '#ffffff' : '#475569',
                    padding: '6px 14px',
                    fontSize: 13,
                    fontWeight: active ? 600 : 400,
                    cursor: 'pointer',
                    transition: 'all 0.15s',
                  }}
                >
                  近{d}天
                </button>
              )
            })}
          </div>
          <Button onClick={onClose} style={{ borderRadius: 8, padding: '0 22px' }}>
            关闭
          </Button>
        </div>
      }
    >
      <Spin spinning={loading}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16, marginTop: 8 }}>
          {/* 顶部 4 张核心指标卡片 */}
          <Row gutter={[12, 12]}>
            <Col xs={12} sm={6}>
              <div style={kpiCardStyle}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
                  <span
                    style={{
                      width: 30,
                      height: 30,
                      borderRadius: 8,
                      background: '#eff6ff',
                      color: '#3b82f6',
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                    }}
                  >
                    <Zap size={15} />
                  </span>
                  <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                    总调用次数
                  </Typography.Text>
                </div>
                <div
                  style={{
                    fontSize: 23,
                    fontWeight: 700,
                    color: '#0f172a',
                    fontVariantNumeric: 'tabular-nums',
                  }}
                >
                  {totalCalls.toLocaleString()}
                </div>
                <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>{days}天内</div>
              </div>
            </Col>

            <Col xs={12} sm={6}>
              <div style={kpiCardStyle}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
                  <span
                    style={{
                      width: 30,
                      height: 30,
                      borderRadius: 8,
                      background: '#ecfdf5',
                      color: '#10b981',
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                    }}
                  >
                    <Cpu size={15} />
                  </span>
                  <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                    Tokens
                  </Typography.Text>
                </div>
                <div
                  style={{
                    fontSize: 23,
                    fontWeight: 700,
                    color: '#0f172a',
                    fontVariantNumeric: 'tabular-nums',
                  }}
                >
                  {formatCompactTokens(totalTokens)}
                </div>
                <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>
                  ↑ {formatCompactTokens(promptTokens)} ↓ {formatCompactTokens(completionTokens)}
                </div>
              </div>
            </Col>

            <Col xs={12} sm={6}>
              <div style={kpiCardStyle}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
                  <span
                    style={{
                      width: 30,
                      height: 30,
                      borderRadius: 8,
                      background: '#fff7ed',
                      color: '#f97316',
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                    }}
                  >
                    <CircleDollarSign size={15} />
                  </span>
                  <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                    消耗额度
                  </Typography.Text>
                </div>
                <div
                  style={{
                    fontSize: 23,
                    fontWeight: 700,
                    color: '#0f172a',
                    fontVariantNumeric: 'tabular-nums',
                  }}
                >
                  ¥{(totalQuota / QUOTA_PER_UNIT).toFixed(2)}
                </div>
                <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>累计扣费</div>
              </div>
            </Col>

            <Col xs={12} sm={6}>
              <div style={kpiCardStyle}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
                  <span
                    style={{
                      width: 30,
                      height: 30,
                      borderRadius: 8,
                      background: '#f5f3ff',
                      color: '#8b5cf6',
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                    }}
                  >
                    <TrendingUp size={15} />
                  </span>
                  <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                    模型种类
                  </Typography.Text>
                </div>
                <div
                  style={{
                    fontSize: 23,
                    fontWeight: 700,
                    color: '#0f172a',
                    fontVariantNumeric: 'tabular-nums',
                  }}
                >
                  {modelStats.length}
                </div>
                <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>使用的模型数</div>
              </div>
            </Col>
          </Row>

          {/* 每日调用 & Token 双轴趋势图 */}
          <div style={sectionCardStyle}>
            <div style={{ fontWeight: 700, fontSize: 14, color: '#0f172a', marginBottom: 8 }}>
              每日调用 & Token 趋势
            </div>
            <DualTrendChart daily={dailyStats} />
          </div>

          {/* 模型消耗明细表 */}
          <div style={sectionCardStyle}>
            <div style={{ fontWeight: 700, fontSize: 14, color: '#0f172a', marginBottom: 12 }}>
              模型消耗明细
            </div>
            <Table<TokenModelStat>
              size="small"
              rowKey="model_name"
              pagination={false}
              dataSource={modelStats}
              columns={modelColumns}
              scroll={{ y: 240 }}
              locale={{ emptyText: '选定时间范围内暂无调用记录' }}
            />
          </div>
        </div>
      </Spin>
    </Modal>
  )
}

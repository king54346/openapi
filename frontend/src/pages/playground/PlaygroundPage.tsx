import React, { useEffect, useMemo, useRef, useState } from 'react'
import {
  AutoComplete,
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  InputNumber,
  Menu,
  Row,
  Select,
  Slider,
  Space,
  Switch,
  Tag,
  Tooltip,
  Typography,
  App,
} from 'antd'
import {
  AudioLines,
  Bot,
  Eraser,
  FileSearch,
  Image as ImageIcon,
  Layers,
  Plus,
  Send,
  Square,
  Trash2,
  Video,
} from 'lucide-react'
import { api, getStoredUser, streamChatCompletion } from '../../api/client'

type PlaygroundMode = 'chat' | 'image' | 'video' | 'embed' | 'rerank' | 'audio'

interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  reasoning?: string
}

interface SavedConversation {
  id: string
  sessionId: string
  title: string
  model: string
  systemPrompt: string
  messages: ChatMessage[]
  updatedAt: number
}

const HISTORY_STORAGE_KEY = 'openapi_pg_conversations'

function loadSavedConversations(): SavedConversation[] {
  try {
    return JSON.parse(localStorage.getItem(HISTORY_STORAGE_KEY) || '[]') as SavedConversation[]
  } catch {
    return []
  }
}

function persistConversations(list: SavedConversation[]) {
  try {
    localStorage.setItem(HISTORY_STORAGE_KEY, JSON.stringify(list.slice(0, 30)))
  } catch {
    // ignore
  }
}

export const PlaygroundPage: React.FC = () => {
  const { message } = App.useApp()
  const [mode, setMode] = useState<PlaygroundMode>('chat')
  const [models, setModels] = useState<string[]>([])

  // Chat 状态
  const [conversations, setConversations] = useState<SavedConversation[]>(loadSavedConversations)
  const [convId, setConvId] = useState<string>(() => `conv_${Date.now()}`)
  const [sessionId, setSessionId] = useState<string>(() => `sess_${Date.now()}`)
  const [chatModel, setChatModel] = useState('')
  const [systemPrompt, setSystemPrompt] = useState('')
  const [temperature, setTemperature] = useState<number>(0.7)
  const [maxTokens, setMaxTokens] = useState<number | null>(null)
  const [streamEnabled, setStreamEnabled] = useState(true)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [inputText, setInputText] = useState('')
  const [chatLoading, setChatLoading] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const scrollBottomRef = useRef<HTMLDivElement | null>(null)

  // Image 状态
  const [imgLoading, setImgLoading] = useState(false)
  const [imgUrls, setImgUrls] = useState<string[]>([])

  // Video 状态
  const [videoLoading, setVideoLoading] = useState(false)
  const [videoTaskId, setVideoTaskId] = useState('')
  const [videoResult, setVideoResult] = useState<unknown>(null)

  // Embedding 状态
  const [embedLoading, setEmbedLoading] = useState(false)
  const [embedResult, setEmbedResult] = useState<unknown>(null)

  // Rerank 状态
  const [rerankLoading, setRerankLoading] = useState(false)
  const [rerankResult, setRerankResult] = useState<
    Array<{ index: number; relevance_score: number; document?: { text?: string } | string }>
  >([])

  // Audio 状态
  const [audioLoading, setAudioLoading] = useState(false)
  const [audioUrl, setAudioUrl] = useState('')

  useEffect(() => {
    api
      .get<string[]>('/api/user/models')
      .then((list) => {
        const arr = list || []
        setModels(arr)
        if (arr.length > 0) {
          setChatModel((prev) => prev || arr[0])
        }
      })
      .catch(() => setModels([]))
  }, [])

  useEffect(() => {
    scrollBottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const modelOptions = useMemo(
    () => models.map((m) => ({ value: m, label: m })),
    [models],
  )

  const syncConversationToHistory = (nextMsgs: ChatMessage[], modelName: string, sys: string) => {
    if (nextMsgs.length === 0) return
    const firstUser = nextMsgs.find((m) => m.role === 'user')?.content || '新对话'
    const item: SavedConversation = {
      id: convId,
      sessionId,
      title: firstUser.slice(0, 26),
      model: modelName,
      systemPrompt: sys,
      messages: nextMsgs,
      updatedAt: Date.now(),
    }
    setConversations((prev) => {
      const rest = prev.filter((c) => c.id !== convId)
      const next = [item, ...rest].slice(0, 30)
      persistConversations(next)
      return next
    })
  }

  const startNewChat = () => {
    abortRef.current?.abort()
    setConvId(`conv_${Date.now()}`)
    setSessionId(`sess_${Date.now()}`)
    setMessages([])
    setInputText('')
  }

  const loadConversation = (c: SavedConversation) => {
    abortRef.current?.abort()
    setMode('chat')
    setConvId(c.id)
    setSessionId(c.sessionId || `sess_${Date.now()}`)
    setChatModel(c.model)
    setSystemPrompt(c.systemPrompt)
    setMessages(c.messages)
  }

  const removeConversation = (id: string) => {
    setConversations((prev) => {
      const next = prev.filter((c) => c.id !== id)
      persistConversations(next)
      return next
    })
    if (id === convId) {
      startNewChat()
    }
  }

  const handleSendChat = async () => {
    const prompt = inputText.trim()
    if (!prompt) return
    if (!chatModel.trim()) {
      message.warning('请先选择或输入模型名称')
      return
    }

    const userMsg: ChatMessage = {
      id: `u_${Date.now()}`,
      role: 'user',
      content: prompt,
    }
    const assistantId = `a_${Date.now() + 1}`
    const assistantMsg: ChatMessage = {
      id: assistantId,
      role: 'assistant',
      content: '',
      reasoning: '',
    }

    const historyForReq = [
      ...(systemPrompt.trim()
        ? [{ role: 'system', content: systemPrompt.trim() }]
        : []),
      ...messages.map((m) => ({ role: m.role, content: m.content })),
      { role: 'user', content: prompt },
    ]

    const nextMessages = [...messages, userMsg, assistantMsg]
    setMessages(nextMessages)
    setInputText('')
    setChatLoading(true)

    const payload: Record<string, unknown> = {
      model: chatModel.trim(),
      messages: historyForReq,
      temperature,
    }
    if (maxTokens && maxTokens > 0) {
      payload.max_tokens = maxTokens
    }
    const headers = { 'X-Session-Id': sessionId }

    const controller = new AbortController()
    abortRef.current = controller

    try {
      if (streamEnabled) {
        let accContent = ''
        let accReasoning = ''
        await streamChatCompletion(
          '/pg/chat/completions',
          payload,
          (delta, reasoningDelta) => {
            accContent += delta
            accReasoning += reasoningDelta
            setMessages((curr) =>
              curr.map((m) =>
                m.id === assistantId
                  ? { ...m, content: accContent, reasoning: accReasoning }
                  : m,
              ),
            )
          },
          controller.signal,
          headers,
        )
        const finalList = [...messages, userMsg, { ...assistantMsg, content: accContent, reasoning: accReasoning }]
        syncConversationToHistory(finalList, chatModel.trim(), systemPrompt)
      } else {
        const res = await api.rawJson<{
          choices?: Array<{ message?: { content?: string; reasoning_content?: string } }>
        }>('/pg/chat/completions', { ...payload, stream: false }, headers)
        const msgObj = res.choices?.[0]?.message
        const finalAssistant: ChatMessage = {
          ...assistantMsg,
          content: msgObj?.content || '',
          reasoning: msgObj?.reasoning_content || '',
        }
        const finalList = [...messages, userMsg, finalAssistant]
        setMessages(finalList)
        syncConversationToHistory(finalList, chatModel.trim(), systemPrompt)
      }
    } catch (err) {
      if ((err as Error).name !== 'AbortError') {
        const errMsg = err instanceof Error ? err.message : '请求失败'
        message.error(errMsg)
        setMessages((curr) =>
          curr.map((m) =>
            m.id === assistantId && !m.content ? { ...m, content: `[错误] ${errMsg}` } : m,
          ),
        )
      }
    } finally {
      setChatLoading(false)
      abortRef.current = null
    }
  }

  const handleGenerateImage = async (values: {
    model: string
    prompt: string
    size: string
    n: number
  }) => {
    setImgLoading(true)
    setImgUrls([])
    try {
      const res = await api.rawJson<{
        data?: Array<{ url?: string; b64_json?: string }>
      }>('/pg/images/generations', values)
      const urls = (res.data || [])
        .map((item) =>
          item.url ? item.url : item.b64_json ? `data:image/png;base64,${item.b64_json}` : '',
        )
        .filter(Boolean)
      setImgUrls(urls)
      if (urls.length === 0) message.info('请求已完成，但未返回图片地址')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '图像生成失败')
    } finally {
      setImgLoading(false)
    }
  }

  const handleSubmitVideo = async (values: { model: string; prompt: string }) => {
    setVideoLoading(true)
    try {
      const res = await api.rawJson<{ id?: string; task_id?: string }>('/pg/video/generations', values)
      const tid = res.id || res.task_id || ''
      setVideoTaskId(tid)
      setVideoResult(res)
      message.success(tid ? `视频任务已提交：${tid}` : '任务已提交')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '提交视频任务失败')
    } finally {
      setVideoLoading(false)
    }
  }

  const handlePollVideoTask = async () => {
    if (!videoTaskId.trim()) return
    setVideoLoading(true)
    try {
      const user = getStoredUser()
      const resp = await fetch(`/pg/video/tasks/${encodeURIComponent(videoTaskId.trim())}`, {
        credentials: 'include',
        headers: user?.id ? { 'Open-Api-User': String(user.id) } : undefined,
      })
      const json = (await resp.json()) as unknown
      setVideoResult(json)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '查询视频任务失败')
    } finally {
      setVideoLoading(false)
    }
  }

  const handleEmbedding = async (values: { model: string; input: string }) => {
    setEmbedLoading(true)
    try {
      const res = await api.rawJson('/pg/embeddings', values)
      setEmbedResult(res)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '向量计算失败')
    } finally {
      setEmbedLoading(false)
    }
  }

  const handleRerank = async (values: {
    model: string
    query: string
    documentsRaw: string
    top_n?: number
  }) => {
    const documents = values.documentsRaw
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
    if (documents.length === 0) {
      message.warning('请至少输入一行候选文档')
      return
    }
    setRerankLoading(true)
    try {
      const res = await api.rawJson<{
        results?: Array<{
          index: number
          relevance_score: number
          document?: { text?: string } | string
        }>
      }>('/pg/rerank', {
        model: values.model,
        query: values.query,
        documents,
        top_n: values.top_n || documents.length,
      })
      setRerankResult(res.results || [])
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Rerank 请求失败')
    } finally {
      setRerankLoading(false)
    }
  }

  const handleSpeech = async (values: { model: string; input: string; voice: string }) => {
    setAudioLoading(true)
    try {
      const user = getStoredUser()
      const resp = await fetch('/pg/audio/speech', {
        method: 'POST',
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          ...(user?.id ? { 'Open-Api-User': String(user.id) } : {}),
        },
        body: JSON.stringify(values),
      })
      if (!resp.ok) {
        const errText = await resp.text()
        throw new Error(errText || `HTTP ${resp.status}`)
      }
      const blob = await resp.blob()
      if (audioUrl) URL.revokeObjectURL(audioUrl)
      setAudioUrl(URL.createObjectURL(blob))
    } catch (err) {
      message.error(err instanceof Error ? err.message : '语音合成失败')
    } finally {
      setAudioLoading(false)
    }
  }

  const modeItems = [
    { key: 'chat', icon: <Bot size={15} />, label: '对话 Chat' },
    { key: 'image', icon: <ImageIcon size={15} />, label: '图像生成' },
    { key: 'video', icon: <Video size={15} />, label: '视频任务' },
    { key: 'embed', icon: <Layers size={15} />, label: '向量 Embedding' },
    { key: 'rerank', icon: <FileSearch size={15} />, label: '文档重排 Rerank' },
    { key: 'audio', icon: <AudioLines size={15} />, label: '语音合成 TTS' },
  ]

  return (
    <div style={{ display: 'flex', height: 'calc(100vh - 60px)', overflow: 'hidden' }}>
      {/* 左侧体验中心二级栏 */}
      <div
        style={{
          width: 220,
          borderRight: '1px solid var(--border-subtle)',
          background: 'var(--bg-surface)',
          display: 'flex',
          flexDirection: 'column',
          flexShrink: 0,
        }}
      >
        <div style={{ padding: '12px 12px 6px' }}>
          <Button
            type="primary"
            block
            icon={<Plus size={15} />}
            onClick={startNewChat}
          >
            新建对话
          </Button>
        </div>

        <Menu
          mode="inline"
          selectedKeys={[mode]}
          items={modeItems}
          onClick={({ key }) => setMode(key as PlaygroundMode)}
          style={{ borderInlineEnd: 'none', background: 'transparent' }}
        />

        {mode === 'chat' && (
          <div
            style={{
              flex: 1,
              overflowY: 'auto',
              borderTop: '1px solid var(--border-subtle)',
              padding: '10px 10px',
            }}
          >
            <Typography.Text type="secondary" style={{ fontSize: 12, paddingInline: 6 }}>
              最近对话 ({conversations.length})
            </Typography.Text>
            <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 4 }}>
              {conversations.map((c) => (
                <div
                  key={c.id}
                  onClick={() => loadConversation(c)}
                  style={{
                    padding: '6px 8px',
                    borderRadius: 8,
                    cursor: 'pointer',
                    background:
                      c.id === convId ? 'rgba(22, 119, 255, 0.1)' : 'transparent',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    gap: 6,
                  }}
                >
                  <Typography.Text ellipsis style={{ fontSize: 13, flex: 1 }}>
                    {c.title}
                  </Typography.Text>
                  <Button
                    type="text"
                    size="small"
                    danger
                    icon={<Trash2 size={12} />}
                    onClick={(e) => {
                      e.stopPropagation()
                      removeConversation(c.id)
                    }}
                  />
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* 主操作区 */}
      <div style={{ flex: 1, overflowY: 'auto', padding: 20 }}>
        {mode === 'chat' && (
          <Row gutter={16} style={{ height: '100%' }}>
            <Col xs={24} lg={17} style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
              <Card
                size="small"
                style={{ flex: 1, display: 'flex', flexDirection: 'column', borderRadius: 12 }}
                styles={{
                  body: {
                    flex: 1,
                    display: 'flex',
                    flexDirection: 'column',
                    overflow: 'hidden',
                    padding: 16,
                  },
                }}
              >
                <div style={{ flex: 1, overflowY: 'auto', paddingRight: 6 }}>
                  {messages.length === 0 ? (
                    <div style={{ paddingTop: 80 }}>
                      <Empty description="选择模型并输入内容开始测试网关对话转发" />
                    </div>
                  ) : (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                      {messages.map((m) => (
                        <div
                          key={m.id}
                          style={{
                            alignSelf: m.role === 'user' ? 'flex-end' : 'flex-start',
                            maxWidth: '88%',
                            padding: '10px 14px',
                            borderRadius: 12,
                            background:
                              m.role === 'user'
                                ? '#1677ff'
                                : 'rgba(127, 127, 127, 0.09)',
                            color: m.role === 'user' ? '#fff' : 'inherit',
                          }}
                        >
                          {m.reasoning && (
                            <div
                              style={{
                                fontSize: 12,
                                opacity: 0.75,
                                borderLeft: '2px solid #9ca3af',
                                paddingLeft: 8,
                                marginBottom: 8,
                                whiteSpace: 'pre-wrap',
                              }}
                            >
                              {m.reasoning}
                            </div>
                          )}
                          <div className="markdown-prose">
                            {m.content || (chatLoading && m.role === 'assistant' ? '思考中...' : '')}
                          </div>
                        </div>
                      ))}
                      <div ref={scrollBottomRef} />
                    </div>
                  )}
                </div>

                <div style={{ marginTop: 12, borderTop: '1px solid var(--border-subtle)', paddingTop: 12 }}>
                  <Space.Compact style={{ width: '100%' }}>
                    <Input.TextArea
                      value={inputText}
                      onChange={(e) => setInputText(e.target.value)}
                      placeholder="输入提示词（Ctrl + Enter 发送）..."
                      autoSize={{ minRows: 2, maxRows: 5 }}
                      onKeyDown={(e) => {
                        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
                          e.preventDefault()
                          void handleSendChat()
                        }
                      }}
                    />
                  </Space.Compact>
                  <div style={{ marginTop: 8, display: 'flex', justifyContent: 'space-between' }}>
                    <Tooltip title="清空当前会话消息">
                      <Button icon={<Eraser size={14} />} onClick={() => setMessages([])}>
                        清空消息
                      </Button>
                    </Tooltip>
                    <Space>
                      {chatLoading && (
                        <Button
                          danger
                          icon={<Square size={14} />}
                          onClick={() => abortRef.current?.abort()}
                        >
                          停止
                        </Button>
                      )}
                      <Button
                        type="primary"
                        icon={<Send size={14} />}
                        loading={chatLoading}
                        onClick={() => void handleSendChat()}
                      >
                        发送
                      </Button>
                    </Space>
                  </div>
                </div>
              </Card>
            </Col>

            <Col xs={24} lg={7}>
              <Card size="small" title="请求参数" style={{ borderRadius: 12 }}>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                  <div>
                    <Typography.Text strong style={{ fontSize: 13 }}>
                      模型 (Model)
                    </Typography.Text>
                    <AutoComplete
                      style={{ width: '100%', marginTop: 6 }}
                      options={modelOptions}
                      value={chatModel}
                      onChange={setChatModel}
                      placeholder="选择或手动输入模型名"
                      filterOption={(inputValue, option) =>
                        (option?.value ?? '')
                          .toLowerCase()
                          .includes(inputValue.toLowerCase())
                      }
                    />
                  </div>

                  <div>
                    <Typography.Text strong style={{ fontSize: 13 }}>
                      System Prompt
                    </Typography.Text>
                    <Input.TextArea
                      rows={3}
                      style={{ marginTop: 6 }}
                      value={systemPrompt}
                      onChange={(e) => setSystemPrompt(e.target.value)}
                      placeholder="可选：设定系统角色提示词"
                    />
                  </div>

                  <div>
                    <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                      <Typography.Text strong style={{ fontSize: 13 }}>
                        Temperature
                      </Typography.Text>
                      <Tag>{temperature}</Tag>
                    </div>
                    <Slider
                      min={0}
                      max={2}
                      step={0.1}
                      value={temperature}
                      onChange={setTemperature}
                    />
                  </div>

                  <div>
                    <Typography.Text strong style={{ fontSize: 13 }}>
                      最大输出 Tokens（可选）
                    </Typography.Text>
                    <InputNumber
                      style={{ width: '100%', marginTop: 6 }}
                      min={1}
                      max={128000}
                      value={maxTokens}
                      onChange={(v) => setMaxTokens(v)}
                      placeholder="默认由模型决定"
                    />
                  </div>

                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Typography.Text strong style={{ fontSize: 13 }}>
                      SSE 流式输出
                    </Typography.Text>
                    <Switch checked={streamEnabled} onChange={setStreamEnabled} />
                  </div>

                  <div>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      亲和会话标识 (X-Session-Id)：
                    </Typography.Text>
                    <Input
                      size="small"
                      style={{ marginTop: 4, fontFamily: 'monospace' }}
                      value={sessionId}
                      onChange={(e) => setSessionId(e.target.value)}
                    />
                  </div>
                </div>
              </Card>
            </Col>
          </Row>
        )}

        {mode === 'image' && (
          <Card title="图像生成调试 (/pg/images/generations)" style={{ maxWidth: 880, borderRadius: 12 }}>
            <Form
              layout="vertical"
              initialValues={{ size: '1024x1024', n: 1, model: models[0] || 'dall-e-3' }}
              onFinish={(vals) => void handleGenerateImage(vals)}
            >
              <Row gutter={16}>
                <Col xs={24} sm={12}>
                  <Form.Item name="model" label="图像模型" rules={[{ required: true }]}>
                    <AutoComplete options={modelOptions} placeholder="例如 dall-e-3 / gpt-image-1" />
                  </Form.Item>
                </Col>
                <Col xs={12} sm={6}>
                  <Form.Item name="size" label="尺寸">
                    <Select
                      options={[
                        { value: '1024x1024', label: '1024x1024' },
                        { value: '1024x1792', label: '1024x1792' },
                        { value: '1792x1024', label: '1792x1024' },
                        { value: '512x512', label: '512x512' },
                      ]}
                    />
                  </Form.Item>
                </Col>
                <Col xs={12} sm={6}>
                  <Form.Item name="n" label="张数">
                    <InputNumber min={1} max={4} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>
              <Form.Item name="prompt" label="画面描述 (Prompt)" rules={[{ required: true }]}>
                <Input.TextArea rows={3} placeholder="描述想要生成的图像..." />
              </Form.Item>
              <Button type="primary" htmlType="submit" loading={imgLoading}>
                生成图像
              </Button>
            </Form>

            {imgUrls.length > 0 && (
              <Row gutter={[12, 12]} style={{ marginTop: 20 }}>
                {imgUrls.map((url, idx) => (
                  <Col xs={24} sm={12} key={idx}>
                    <img
                      src={url}
                      alt={`generated-${idx}`}
                      style={{ width: '100%', borderRadius: 10, border: '1px solid var(--border-subtle)' }}
                    />
                  </Col>
                ))}
              </Row>
            )}
          </Card>
        )}

        {mode === 'video' && (
          <Card title="异步视频任务调试 (/pg/video/generations)" style={{ maxWidth: 880, borderRadius: 12 }}>
            <Form layout="vertical" onFinish={(vals) => void handleSubmitVideo(vals)}>
              <Form.Item name="model" label="视频模型" rules={[{ required: true }]}>
                <AutoComplete options={modelOptions} placeholder="例如 sora-2 / doubao-seedance..." />
              </Form.Item>
              <Form.Item name="prompt" label="视频提示词" rules={[{ required: true }]}>
                <Input.TextArea rows={3} placeholder="输入视频画面描述..." />
              </Form.Item>
              <Button type="primary" htmlType="submit" loading={videoLoading}>
                提交生成任务
              </Button>
            </Form>

            <div style={{ marginTop: 20 }}>
              <Space>
                <Input
                  placeholder="输入返回的 task_id 查询进度"
                  style={{ width: 320 }}
                  value={videoTaskId}
                  onChange={(e) => setVideoTaskId(e.target.value)}
                />
                <Button loading={videoLoading} onClick={() => void handlePollVideoTask()}>
                  查询任务状态
                </Button>
              </Space>
              {videoResult !== null && (
                <pre className="code-block" style={{ marginTop: 12 }}>
                  {JSON.stringify(videoResult, null, 2)}
                </pre>
              )}
            </div>
          </Card>
        )}

        {mode === 'embed' && (
          <Card title="向量化调试 (/pg/embeddings)" style={{ maxWidth: 880, borderRadius: 12 }}>
            <Form layout="vertical" onFinish={(vals) => void handleEmbedding(vals)}>
              <Form.Item name="model" label="Embedding 模型" rules={[{ required: true }]}>
                <AutoComplete options={modelOptions} placeholder="例如 text-embedding-3-small / BAAI/bge-large-en-v1.5" />
              </Form.Item>
              <Form.Item name="input" label="输入文本" rules={[{ required: true }]}>
                <Input.TextArea rows={3} placeholder="输入需要计算向量的文本..." />
              </Form.Item>
              <Button type="primary" htmlType="submit" loading={embedLoading}>
                计算向量
              </Button>
            </Form>
            {embedResult !== null && (
              <pre className="code-block" style={{ marginTop: 16, maxHeight: 360 }}>
                {JSON.stringify(embedResult, null, 2)}
              </pre>
            )}
          </Card>
        )}

        {mode === 'rerank' && (
          <Card title="文档重排调试 (/pg/rerank)" style={{ maxWidth: 880, borderRadius: 12 }}>
            <Form layout="vertical" initialValues={{ top_n: 3 }} onFinish={(vals) => void handleRerank(vals)}>
              <Row gutter={16}>
                <Col xs={24} sm={16}>
                  <Form.Item name="model" label="Rerank 模型" rules={[{ required: true }]}>
                    <AutoComplete options={modelOptions} placeholder="例如 BAAI/bge-reranker-v2-m3" />
                  </Form.Item>
                </Col>
                <Col xs={24} sm={8}>
                  <Form.Item name="top_n" label="Top N">
                    <InputNumber min={1} max={50} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>
              <Form.Item name="query" label="查询语句 (Query)" rules={[{ required: true }]}>
                <Input placeholder="输入查询问题..." />
              </Form.Item>
              <Form.Item
                name="documentsRaw"
                label="候选文档列表（每行一条）"
                rules={[{ required: true }]}
              >
                <Input.TextArea rows={5} placeholder={'第一条候选文本\n第二条候选文本\n第三条候选文本'} />
              </Form.Item>
              <Button type="primary" htmlType="submit" loading={rerankLoading}>
                执行重排
              </Button>
            </Form>

            {rerankResult.length > 0 && (
              <div style={{ marginTop: 16, display: 'flex', flexDirection: 'column', gap: 8 }}>
                {rerankResult.map((item, idx) => (
                  <Card size="small" key={idx}>
                    <Space style={{ justifyContent: 'space-between', width: '100%' }}>
                      <Tag color="blue">#{idx + 1} (原索引 {item.index})</Tag>
                      <Tag color="green">相关度得分：{item.relevance_score?.toFixed(4)}</Tag>
                    </Space>
                    {item.document && (
                      <div style={{ marginTop: 6 }}>
                        {typeof item.document === 'string'
                          ? item.document
                          : item.document.text}
                      </div>
                    )}
                  </Card>
                ))}
              </div>
            )}
          </Card>
        )}

        {mode === 'audio' && (
          <Card title="语音合成调试 (/pg/audio/speech)" style={{ maxWidth: 880, borderRadius: 12 }}>
            <Form
              layout="vertical"
              initialValues={{ voice: 'alloy' }}
              onFinish={(vals) => void handleSpeech(vals)}
            >
              <Row gutter={16}>
                <Col xs={24} sm={14}>
                  <Form.Item name="model" label="TTS 模型" rules={[{ required: true }]}>
                    <AutoComplete options={modelOptions} placeholder="例如 tts-1" />
                  </Form.Item>
                </Col>
                <Col xs={24} sm={10}>
                  <Form.Item name="voice" label="音色 (Voice)" rules={[{ required: true }]}>
                    <Input placeholder="alloy / echo / nova / shimmer..." />
                  </Form.Item>
                </Col>
              </Row>
              <Form.Item name="input" label="合成文本" rules={[{ required: true }]}>
                <Input.TextArea rows={3} placeholder="输入要朗读的文字..." />
              </Form.Item>
              <Button type="primary" htmlType="submit" loading={audioLoading}>
                生成语音
              </Button>
            </Form>
            {audioUrl && (
              <div style={{ marginTop: 16 }}>
                <audio controls src={audioUrl} style={{ width: '100%' }} />
              </div>
            )}
          </Card>
        )}
      </div>
    </div>
  )
}

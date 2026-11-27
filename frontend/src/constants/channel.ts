import dayjs from 'dayjs'

export interface ChannelTypeOption {
  value: number
  label: string
  color: string
  defaultBaseURL: string
}

export const CHANNEL_TYPES: ChannelTypeOption[] = [
  { value: 1, label: 'OpenAI / 兼容接口', color: 'green', defaultBaseURL: 'https://api.openai.com' },
  { value: 8, label: '自定义渠道 (Custom)', color: 'teal', defaultBaseURL: '' },
  { value: 14, label: 'Anthropic Claude', color: 'orange', defaultBaseURL: 'https://api.anthropic.com' },
  { value: 24, label: 'Google Gemini', color: 'blue', defaultBaseURL: 'https://generativelanguage.googleapis.com' },
  { value: 43, label: 'DeepSeek', color: 'geekblue', defaultBaseURL: 'https://api.deepseek.com' },
  { value: 26, label: '智谱清言 GLM (ZhipuV4)', color: 'processing', defaultBaseURL: 'https://open.bigmodel.cn' },
  { value: 25, label: 'Kimi / 月之暗面 (Moonshot)', color: 'cyan', defaultBaseURL: 'https://api.moonshot.cn' },
  { value: 40, label: '硅基流动 (SiliconFlow)', color: 'purple', defaultBaseURL: 'https://api.siliconflow.cn' },
  { value: 17, label: '阿里百炼 (DashScope)', color: 'gold', defaultBaseURL: 'https://dashscope.aliyuncs.com' },
  { value: 20, label: 'OpenRouter', color: 'purple', defaultBaseURL: 'https://openrouter.ai/api' },
  { value: 35, label: 'MiniMax', color: 'magenta', defaultBaseURL: 'https://api.minimax.chat' },
  { value: 45, label: '火山方舟 (VolcEngine)', color: 'volcano', defaultBaseURL: 'https://ark.cn-beijing.volces.com' },
  { value: 16, label: '智谱 ChatGLM v3 (旧版)', color: 'default', defaultBaseURL: 'https://open.bigmodel.cn' },
  { value: 54, label: '豆包视频 (DoubaoVideo)', color: 'lime', defaultBaseURL: 'https://ark.cn-beijing.volces.com' },
  { value: 55, label: 'OpenAI Sora', color: 'geekblue', defaultBaseURL: 'https://api.openai.com' },
]

export const CHANNEL_TYPE_MAP = Object.fromEntries(
  CHANNEL_TYPES.map((item) => [item.value, item]),
)

export function getChannelTypeInfo(type: number): ChannelTypeOption {
  return (
    CHANNEL_TYPE_MAP[type] ?? {
      value: type,
      label: `渠道类型 #${type}`,
      color: 'default',
      defaultBaseURL: '',
    }
  )
}

export const ROLE_OPTIONS = [
  { value: 1, label: '普通用户', color: 'default' },
  { value: 10, label: '管理员', color: 'blue' },
  { value: 100, label: '超级管理员 (Root)', color: 'gold' },
]

export function getRoleInfo(role: number) {
  if (role >= 100) return { label: 'Root', color: 'gold' }
  if (role >= 10) return { label: '管理员', color: 'blue' }
  return { label: '普通用户', color: 'default' }
}

export const LOG_TYPES: Record<number, { label: string; color: string }> = {
  0: { label: '未知', color: 'default' },
  1: { label: '充值', color: 'cyan' },
  2: { label: '消费调用', color: 'green' },
  3: { label: '管理操作', color: 'blue' },
  4: { label: '系统事件', color: 'purple' },
  5: { label: '错误日志', color: 'red' },
  6: { label: '退款', color: 'orange' },
}

export function formatUnixTime(ts?: number | null): string {
  if (!ts || ts <= 0) return '-'
  // 兼容秒级与毫秒级时间戳
  const ms = ts > 1e12 ? ts : ts * 1000
  return dayjs(ms).format('YYYY-MM-DD HH:mm:ss')
}

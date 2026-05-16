/**
 * 租户 ASR / TTS / LLM 的 JSON 字段规则（仅前端校验与表单渲染，后端只存 JSON）。
 * 统一使用顶层字段 provider 标识厂商。
 * ASR/TTS 厂商 slug 对齐 pkg/recognizer、pkg/synthesizer；LLM 对齐 pkg/llm。
 * 说明：当前 SIP 嵌入式管线仅执行 qcloud ASR+TTS；其余厂商 JSON 可存盘供后续扩展或网关侧使用。
 */

export type AiFieldType = 'text' | 'password' | 'number'

export interface AiFieldRule {
  key: string
  label: string
  type: AiFieldType
  required?: boolean
  placeholder?: string
}

export interface AiProviderRule {
  provider: string
  label: string
  fields: AiFieldRule[]
}

const qcloudAsrFields: AiFieldRule[] = [
  { key: 'appId', label: 'AppId', type: 'text', required: true, placeholder: '控制台 AppId' },
  { key: 'secretId', label: 'SecretId', type: 'text', required: true },
  { key: 'secretKey', label: 'SecretKey', type: 'password', required: true },
  { key: 'modelType', label: '模型 modelType', type: 'text', placeholder: '默认 16k_zh' },
]

const qcloudTtsFields: AiFieldRule[] = [
  { key: 'appId', label: 'AppId', type: 'text', required: true },
  { key: 'secretId', label: 'SecretId', type: 'text', required: true },
  { key: 'secretKey', label: 'SecretKey', type: 'password', required: true },
  { key: 'voiceType', label: '音色 voiceType', type: 'number', placeholder: '如 101007' },
  { key: 'speed', label: '语速', type: 'number', placeholder: '-2~6' },
  { key: 'sampleRate', label: '采样率 Hz', type: 'number', placeholder: '0=跟随通话 PCM' },
]

const apiKeyModel: AiFieldRule[] = [
  { key: 'apiKey', label: 'API Key', type: 'password', required: true },
  { key: 'model', label: '模型', type: 'text', placeholder: '可选' },
]

const apiKeyBase: AiFieldRule[] = [
  { key: 'apiKey', label: 'API Key', type: 'password', required: true },
  { key: 'baseUrl', label: 'Base URL', type: 'text', placeholder: '服务端点' },
]

export const TENANT_ASR_PROVIDER_RULES: AiProviderRule[] = [
  { provider: 'qcloud', label: '腾讯云 ASR', fields: [...qcloudAsrFields] },
  { provider: 'google', label: 'Google Speech', fields: [...apiKeyBase, { key: 'projectId', label: 'Project ID', type: 'text' }] },
  { provider: 'aliyun', label: '阿里云 ASR', fields: [...apiKeyBase, { key: 'appKey', label: 'AppKey', type: 'text', required: true }] },
  { provider: 'qiniu', label: '七牛 ASR', fields: [...apiKeyModel] },
  { provider: 'funasr', label: 'FunASR', fields: [...apiKeyBase, { key: 'endpoint', label: '服务地址', type: 'text' }] },
  { provider: 'volcengine', label: '火山引擎 ASR（标准）', fields: [...apiKeyModel, { key: 'appId', label: 'App ID', type: 'text' }] },
  { provider: 'volcllmasr', label: '火山引擎 LLM ASR', fields: [...apiKeyModel, { key: 'appId', label: 'App ID', type: 'text' }] },
  {
    provider: 'xfyun_mul',
    label: '科大讯飞多语言',
    fields: [
      { key: 'appId', label: 'AppId', type: 'text', required: true },
      { key: 'apiKey', label: 'API Key', type: 'password', required: true },
      { key: 'apiSecret', label: 'API Secret', type: 'password', required: true },
    ],
  },
  { provider: 'gladia', label: 'Gladia', fields: [...apiKeyModel] },
  { provider: 'funasr_realtime', label: 'FunASR 实时', fields: [...apiKeyBase] },
  { provider: 'whisper', label: 'Whisper', fields: [...apiKeyBase] },
  { provider: 'deepgram', label: 'Deepgram', fields: [...apiKeyModel, { key: 'language', label: '语言', type: 'text' }] },
  { provider: 'aws', label: 'AWS Transcribe', fields: [...apiKeyBase, { key: 'region', label: 'Region', type: 'text', required: true }] },
  { provider: 'baidu', label: '百度 ASR', fields: [{ key: 'apiKey', label: 'API Key', type: 'password', required: true }, { key: 'secretKey', label: 'Secret Key', type: 'password', required: true }] },
  { provider: 'voiceapi', label: 'VoiceAPI', fields: [...apiKeyBase] },
  { provider: 'local', label: '本地 ASR', fields: [{ key: 'endpoint', label: '服务地址', type: 'text', required: true }] },
  { provider: 'openai', label: 'OpenAI Whisper', fields: [...apiKeyBase] },
]

export const TENANT_TTS_PROVIDER_RULES: AiProviderRule[] = [
  { provider: 'qcloud', label: '腾讯云 TTS', fields: [...qcloudTtsFields] },
  { provider: 'qiniu', label: '七牛 TTS', fields: [...apiKeyModel, { key: 'voice', label: '音色', type: 'text' }] },
  { provider: 'xunfei', label: '讯飞 TTS', fields: [{ key: 'appId', label: 'AppId', type: 'text', required: true }, { key: 'apiKey', label: 'API Key', type: 'password', required: true }, { key: 'apiSecret', label: 'API Secret', type: 'password', required: true }] },
  { provider: 'aliyun', label: '阿里云 TTS', fields: [...apiKeyBase, { key: 'voice', label: '音色', type: 'text' }] },
  { provider: 'baidu', label: '百度 TTS', fields: [{ key: 'apiKey', label: 'API Key', type: 'password', required: true }, { key: 'secretKey', label: 'Secret Key', type: 'password', required: true }] },
  { provider: 'azure', label: 'Azure TTS', fields: [...apiKeyBase, { key: 'region', label: 'Region', type: 'text', required: true }, { key: 'voice', label: 'Voice', type: 'text', required: true }] },
  { provider: 'google', label: 'Google Cloud TTS', fields: [...apiKeyBase, { key: 'voice', label: 'Voice', type: 'text' }] },
  { provider: 'aws', label: 'AWS Polly', fields: [...apiKeyBase, { key: 'region', label: 'Region', type: 'text', required: true }, { key: 'voiceId', label: 'Voice ID', type: 'text' }] },
  { provider: 'openai', label: 'OpenAI TTS', fields: [...apiKeyBase, { key: 'model', label: '模型', type: 'text', placeholder: 'tts-1' }, { key: 'voice', label: '音色', type: 'text', placeholder: 'alloy' }] },
  { provider: 'elevenlabs', label: 'ElevenLabs', fields: [...apiKeyModel, { key: 'voiceId', label: 'Voice ID', type: 'text', required: true }] },
  { provider: 'local', label: '本地 TTS', fields: [{ key: 'endpoint', label: '服务地址', type: 'text', required: true }] },
  { provider: 'local_gospeech', label: '本地 go-speech', fields: [{ key: 'voice', label: '音色', type: 'text' }] },
  { provider: 'fishspeech', label: 'FishSpeech', fields: [...apiKeyBase] },
  { provider: 'fishaudio', label: 'Fish Audio', fields: [...apiKeyBase] },
  { provider: 'coqui', label: 'Coqui TTS', fields: [{ key: 'modelPath', label: '模型路径', type: 'text', required: true }] },
  { provider: 'volcengine', label: '火山引擎 TTS', fields: [...apiKeyModel, { key: 'appId', label: 'App ID', type: 'text' }] },
  { provider: 'volcengine_clone', label: '火山克隆 TTS', fields: [...apiKeyModel, { key: 'speakerId', label: 'Speaker ID', type: 'text', required: true }] },
  { provider: 'volcengine_llm', label: '火山 LLM TTS', fields: [...apiKeyModel] },
  { provider: 'volcengine_stream', label: '火山流式 TTS', fields: [...apiKeyModel] },
  { provider: 'minimax', label: 'Minimax', fields: [...apiKeyModel, { key: 'groupId', label: 'Group ID', type: 'text' }] },
]

export const TENANT_LLM_PROVIDER_RULES: AiProviderRule[] = [
  {
    provider: 'openai',
    label: 'OpenAI 兼容',
    fields: [
      { key: 'apiKey', label: 'API Key', type: 'password', required: true },
      { key: 'baseUrl', label: 'Base URL', type: 'text', placeholder: 'https://api.openai.com/v1' },
      { key: 'model', label: '模型', type: 'text' },
    ],
  },
  {
    provider: 'alibaba',
    label: '阿里云百炼',
    fields: [
      { key: 'apiKey', label: 'API Key', type: 'password', required: true },
      { key: 'appId', label: 'App ID', type: 'text', required: true },
      { key: 'model', label: '模型', type: 'text' },
    ],
  },
  {
    provider: 'coze',
    label: 'Coze',
    fields: [
      { key: 'apiKey', label: 'Token / API Key', type: 'password', required: true },
      { key: 'botId', label: 'Bot ID', type: 'text', required: true },
      { key: 'userId', label: 'User ID', type: 'text' },
      { key: 'baseUrl', label: 'Base URL', type: 'text' },
    ],
  },
  {
    provider: 'ollama',
    label: 'Ollama',
    fields: [
      { key: 'baseUrl', label: 'Base URL', type: 'text', required: true, placeholder: 'http://127.0.0.1:11434' },
      { key: 'apiKey', label: 'API Key（可选）', type: 'password' },
    ],
  },
]

export type AiTab = 'asr' | 'tts' | 'llm'

export function providerRulesFor(tab: AiTab): AiProviderRule[] {
  if (tab === 'asr') return TENANT_ASR_PROVIDER_RULES
  if (tab === 'tts') return TENANT_TTS_PROVIDER_RULES
  return TENANT_LLM_PROVIDER_RULES
}

export function ruleFor(tab: AiTab, provider: string): AiProviderRule | undefined {
  const p = String(provider || '').toLowerCase()
  return providerRulesFor(tab).find((x) => x.provider.toLowerCase() === p)
}

export function defaultDraft(tab: AiTab): Record<string, unknown> {
  const first = providerRulesFor(tab)[0]
  return { provider: first?.provider ?? 'qcloud' }
}

export function normalizeDraft(tab: AiTab, raw: unknown): Record<string, unknown> {
  const base = defaultDraft(tab)
  if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
    return { ...base, ...(raw as Record<string, unknown>) }
  }
  return { ...base }
}

export function validateDraft(tab: AiTab, draft: Record<string, unknown>): string | null {
  const prov = String(draft.provider ?? '')
  const def = ruleFor(tab, prov)
  if (!def) return `不支持的 ${tab} 厂商：${prov || '（空）'}`
  for (const f of def.fields) {
    if (!f.required) continue
    const v = draft[f.key]
    if (v === undefined || v === null || String(v).trim() === '') {
      return `「${def.label}」请填写：${f.label}`
    }
  }
  return null
}

/** 提交给后端的 JSON：含 provider + 各厂商字段（空字符串省略） */
export function draftToPayload(tab: AiTab, draft: Record<string, unknown>): Record<string, unknown> {
  const prov = String(draft.provider ?? '')
  const def = ruleFor(tab, prov)
  if (!def) return { provider: prov }
  const out: Record<string, unknown> = { provider: def.provider }
  for (const f of def.fields) {
    const v = draft[f.key]
    if (v === undefined || v === null || v === '') continue
    if (f.type === 'number') {
      const n = typeof v === 'number' ? v : Number(String(v).trim())
      if (!Number.isFinite(n)) continue
      out[f.key] = n
    } else {
      out[f.key] = String(v).trim()
    }
  }
  return out
}

/** Mirrors pkg/sip/outbound HybridScript / HybridStep for the script-manager UI. */

export type HybridTransition = {
  description?: string
  intent?: string
  contains?: string
  equals?: string
  next_id: string
}

export type HybridDTMFTransition = {
  digit: string
  next_id: string
}

export type HybridStepType = 'say' | 'listen' | 'llm_reply' | 'condition' | 'end'

export type HybridStepDraft = {
  id: string
  type: HybridStepType
  prompt: string
  next_id: string
  retry: number
  timeout_ms: number
  fallback_id: string
  listen_timeout_ms: number
  listen_fallback_id: string
  llm_instruction: string
  transitions: HybridTransition[]
  dtmf_transitions: HybridDTMFTransition[]
}

export type HybridScriptDraft = {
  id: string
  version: string
  start_id: string
  max_turns: number
  silence_timeout_ms: number
  end_intents: string[]
  steps: HybridStepDraft[]
}

const STEP_TYPES: HybridStepType[] = ['say', 'listen', 'llm_reply', 'condition', 'end']

function emptyStep(type: HybridStepType): HybridStepDraft {
  return {
    id: '',
    type,
    prompt: '',
    next_id: '',
    retry: 0,
    timeout_ms: 0,
    fallback_id: '',
    listen_timeout_ms: 0,
    listen_fallback_id: '',
    llm_instruction: '',
    transitions: [],
    dtmf_transitions: [],
  }
}

/** Random logical id for hybrid script JSON field `id` (e.g. flow_a1b2c3d4e5). */
export function generateScriptLogicalId(): string {
  const bytes = new Uint8Array(5)
  crypto.getRandomValues(bytes)
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
  return `flow_${hex}`
}

/** Date-only version string YYYY-MM-DD (local calendar). */
export function generateScriptVersionDate(): string {
  const d = new Date()
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

/** Default step graph for a brand-new script; `id` / `version` are placeholders — prefer {@link newHybridScriptDraftWithAutoIdentity}. */
export function defaultHybridScriptDraft(): HybridScriptDraft {
  const begin = 'begin'
  const end = 'end'
  return {
    id: 'my-script-v1',
    version: generateScriptVersionDate(),
    start_id: begin,
    max_turns: 0,
    silence_timeout_ms: 8000,
    end_intents: ['再见', '挂了', '不需要'],
    steps: [
      { ...emptyStep('say'), id: begin, prompt: '您好，这里是智能外呼回访。', next_id: end },
      { ...emptyStep('end'), id: end },
    ],
  }
}

/** New draft with system-generated `id` and `version` (for create flow). */
export function newHybridScriptDraftWithAutoIdentity(): HybridScriptDraft {
  return {
    ...defaultHybridScriptDraft(),
    id: generateScriptLogicalId(),
    version: generateScriptVersionDate(),
  }
}

function normStep(raw: unknown): HybridStepDraft {
  const o = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {}
  const typeRaw = String(o.type ?? 'say').trim()
  const type = (STEP_TYPES.includes(typeRaw as HybridStepType) ? typeRaw : 'say') as HybridStepType
  const trs = Array.isArray(o.transitions)
    ? (o.transitions as unknown[]).map((t) => {
        const x = t && typeof t === 'object' ? (t as Record<string, unknown>) : {}
        return {
          description: String(x.description ?? ''),
          intent: String(x.intent ?? ''),
          contains: String(x.contains ?? ''),
          equals: String(x.equals ?? ''),
          next_id: String(x.next_id ?? ''),
        }
      })
    : []
  const dtmf = Array.isArray(o.dtmf_transitions)
    ? (o.dtmf_transitions as unknown[]).map((d) => {
        const x = d && typeof d === 'object' ? (d as Record<string, unknown>) : {}
        return { digit: String(x.digit ?? ''), next_id: String(x.next_id ?? '') }
      })
    : []
  return {
    id: String(o.id ?? ''),
    type,
    prompt: String(o.prompt ?? ''),
    next_id: String(o.next_id ?? ''),
    retry: Number(o.retry ?? 0) || 0,
    timeout_ms: Number(o.timeout_ms ?? 0) || 0,
    fallback_id: String(o.fallback_id ?? ''),
    listen_timeout_ms: Number(o.listen_timeout_ms ?? 0) || 0,
    listen_fallback_id: String(o.listen_fallback_id ?? ''),
    llm_instruction: String(o.llm_instruction ?? ''),
    transitions: trs,
    dtmf_transitions: dtmf,
  }
}

export function parseHybridScriptDraft(raw: string): { ok: true; spec: HybridScriptDraft } | { ok: false; error: string } {
  raw = raw.trim()
  if (!raw) return { ok: false, error: '脚本内容为空' }
  let o: unknown
  try {
    o = JSON.parse(raw)
  } catch {
    return { ok: false, error: 'JSON 格式无效，请检查括号与引号' }
  }
  if (!o || typeof o !== 'object') return { ok: false, error: '脚本必须是 JSON 对象' }
  const obj = o as Record<string, unknown>
  const id = String(obj.id ?? '').trim()
  const start_id = String(obj.start_id ?? '').trim()
  if (!id) return { ok: false, error: '脚本逻辑 ID（字段 id）不能为空' }
  if (!start_id) return { ok: false, error: '起始步骤 start_id 不能为空' }
  const stepsRaw = Array.isArray(obj.steps) ? obj.steps : []
  if (stepsRaw.length === 0) return { ok: false, error: '至少需要一个步骤（steps）' }
  const steps = stepsRaw.map(normStep)
  const seen = new Set<string>()
  for (const s of steps) {
    const sid = s.id.trim()
    if (!sid) return { ok: false, error: '每个步骤都需要填写步骤 ID' }
    if (seen.has(sid)) return { ok: false, error: `重复的步骤 ID：${sid}` }
    seen.add(sid)
  }
  if (!seen.has(start_id)) return { ok: false, error: `start_id "${start_id}" 在步骤列表中不存在` }
  const endIntents = Array.isArray(obj.end_intents)
    ? (obj.end_intents as unknown[]).map((x) => String(x ?? '').trim()).filter(Boolean)
    : []
  return {
    ok: true,
    spec: {
      id,
      version: String(obj.version ?? ''),
      start_id,
      max_turns: Number(obj.max_turns ?? 0) || 0,
      silence_timeout_ms: Number(obj.silence_timeout_ms ?? 0) || 0,
      end_intents: endIntents.length ? endIntents : ['再见', '挂了', '不需要'],
      steps,
    },
  }
}

function trimTransition(t: HybridTransition): HybridTransition | null {
  const next_id = t.next_id.trim()
  if (!next_id) return null
  const description = t.description?.trim() ?? ''
  const intent = t.intent?.trim() ?? ''
  const contains = t.contains?.trim() ?? ''
  const equals = t.equals?.trim() ?? ''
  return { next_id, description, intent, contains, equals }
}

function serializeStep(s: HybridStepDraft): Record<string, unknown> {
  const id = s.id.trim()
  const type = s.type
  const base: Record<string, unknown> = { id, type }
  const prompt = s.prompt.trim()
  const next_id = s.next_id.trim()
  const fallback_id = s.fallback_id.trim()
  const listen_fallback_id = s.listen_fallback_id.trim()
  const llm_instruction = s.llm_instruction.trim()

  if (type === 'say' || type === 'llm_reply') {
    if (prompt) base.prompt = prompt
  }
  if (next_id) base.next_id = next_id
  if (s.retry > 0) base.retry = s.retry
  if (s.timeout_ms > 0) base.timeout_ms = s.timeout_ms
  if (fallback_id) base.fallback_id = fallback_id
  if (type === 'listen') {
    if (s.listen_timeout_ms > 0) base.listen_timeout_ms = s.listen_timeout_ms
    if (listen_fallback_id) base.listen_fallback_id = listen_fallback_id
    const trs = s.transitions.map(trimTransition).filter(Boolean) as HybridTransition[]
    if (trs.length) base.transitions = trs
    const dtmf = s.dtmf_transitions
      .map((d) => ({ digit: d.digit.trim(), next_id: d.next_id.trim() }))
      .filter((d) => d.digit && d.next_id)
    if (dtmf.length) base.dtmf_transitions = dtmf
  }
  if (type === 'llm_reply' && llm_instruction) base.llm_instruction = llm_instruction
  if (type === 'condition') {
    const trs = s.transitions.map(trimTransition).filter(Boolean) as HybridTransition[]
    if (trs.length) base.transitions = trs
    if (fallback_id) base.fallback_id = fallback_id
  }
  return base
}

export function serializeHybridScriptDraft(spec: HybridScriptDraft): string {
  const id = spec.id.trim()
  const start_id = spec.start_id.trim()
  const version = spec.version.trim()
  const body: Record<string, unknown> = {
    id,
    start_id,
    steps: spec.steps.map(serializeStep),
  }
  if (version) body.version = version
  if (spec.max_turns > 0) body.max_turns = spec.max_turns
  if (spec.silence_timeout_ms > 0) body.silence_timeout_ms = spec.silence_timeout_ms
  const ends = spec.end_intents.map((x) => x.trim()).filter(Boolean)
  if (ends.length) body.end_intents = ends
  return JSON.stringify(body, null, 2)
}

export const HYBRID_STEP_TYPE_LABELS: Record<HybridStepType, string> = {
  say: '播报（TTS）',
  listen: '倾听（语音识别 / 按键）',
  llm_reply: '大模型生成回复',
  condition: '条件分支（关键字）',
  end: '结束',
}

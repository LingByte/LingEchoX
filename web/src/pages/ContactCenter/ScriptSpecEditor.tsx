import { useCallback, useEffect, useState } from 'react'
import { Button } from '@arco-design/web-react'
import {
  ChevronDown,
  ChevronUp,
  Code2,
  LayoutList,
  Plus,
  Trash2,
} from 'lucide-react'
import {
  defaultHybridScriptDraft,
  HYBRID_STEP_TYPE_LABELS,
  type HybridDTMFTransition,
  type HybridScriptDraft,
  type HybridStepDraft,
  type HybridStepType,
  type HybridTransition,
  parseHybridScriptDraft,
  serializeHybridScriptDraft,
} from '@/pages/ContactCenter/scriptSpecTypes'

type Tab = 'visual' | 'json'

function stepIds(spec: HybridScriptDraft): string[] {
  return spec.steps.map((s) => s.id.trim()).filter(Boolean)
}

function nextIdOptions(spec: HybridScriptDraft, exceptStepId: string): { value: string; label: string }[] {
  const ids = stepIds(spec).filter((id) => id !== exceptStepId)
  return [{ value: '', label: '（结束通话）' }, ...ids.map((id) => ({ value: id, label: id }))]
}

function pushDraft(spec: HybridScriptDraft, onChange: (json: string) => void) {
  onChange(serializeHybridScriptDraft(spec))
}

function freshStepId(spec: HybridScriptDraft, prefix: string): string {
  let n = spec.steps.length + 1
  let id = `${prefix}_${n}`
  const taken = new Set(stepIds(spec))
  while (taken.has(id)) {
    n += 1
    id = `${prefix}_${n}`
  }
  return id
}

type Props = {
  value: string
  onChange: (next: string) => void
  /** When set, JSON `id` / `version` are fixed (hidden in visual mode; enforced on save payload). */
  lockedScriptIdentity?: { id: string; version: string }
}

export default function ScriptSpecEditor({ value, onChange, lockedScriptIdentity }: Props) {
  const [tab, setTab] = useState<Tab>('visual')
  const [draft, setDraft] = useState<HybridScriptDraft>(() => defaultHybridScriptDraft())
  const [parseError, setParseError] = useState<string | null>(null)
  const [jsonDraft, setJsonDraft] = useState(value)
  const [openSteps, setOpenSteps] = useState<Record<number, boolean>>({})

  const applyIdentity = useCallback(
    (spec: HybridScriptDraft): HybridScriptDraft => {
      if (!lockedScriptIdentity) return spec
      return { ...spec, id: lockedScriptIdentity.id, version: lockedScriptIdentity.version }
    },
    [lockedScriptIdentity?.id, lockedScriptIdentity?.version],
  )

  const syncFromValue = useCallback(
    (raw: string) => {
      setJsonDraft(raw)
      const r = parseHybridScriptDraft(raw)
      if (r.ok) {
        const merged = applyIdentity(r.spec)
        setDraft(merged)
        setParseError(null)
      } else {
        setParseError(r.error)
      }
    },
    [applyIdentity],
  )

  useEffect(() => {
    syncFromValue(value)
  }, [value, syncFromValue])

  const commitDraft = useCallback(
    (next: HybridScriptDraft) => {
      const merged = applyIdentity(next)
      setDraft(merged)
      pushDraft(merged, onChange)
    },
    [onChange, applyIdentity],
  )

  const toggleStep = (i: number) => {
    setOpenSteps((o) => ({ ...o, [i]: !o[i] }))
  }

  const updateStep = (index: number, patch: Partial<HybridStepDraft>) => {
    const steps = draft.steps.map((s, i) => (i === index ? { ...s, ...patch } : s))
    commitDraft({ ...draft, steps })
  }

  const removeStep = (index: number) => {
    if (draft.steps.length <= 1) return
    const steps = draft.steps.filter((_, i) => i !== index)
    let start_id = draft.start_id
    if (!steps.some((s) => s.id.trim() === start_id)) {
      start_id = steps[0]?.id.trim() ?? ''
    }
    commitDraft({ ...draft, steps, start_id })
  }

  const moveStep = (index: number, dir: -1 | 1) => {
    const j = index + dir
    if (j < 0 || j >= draft.steps.length) return
    const steps = [...draft.steps]
    ;[steps[index], steps[j]] = [steps[j], steps[index]]
    commitDraft({ ...draft, steps })
  }

  const addStep = (type: HybridStepType) => {
    const id =
      type === 'end'
        ? freshStepId(draft, 'end')
        : type === 'say'
          ? freshStepId(draft, 'say')
          : freshStepId(draft, 'step')
    const blank: HybridStepDraft = {
      id,
      type,
      prompt: '',
      next_id: '',
      retry: 0,
      timeout_ms: 0,
      fallback_id: '',
      listen_timeout_ms: type === 'listen' ? 8000 : 0,
      listen_fallback_id: '',
      llm_instruction: '',
      transitions: type === 'listen' || type === 'condition' ? [] : [],
      dtmf_transitions: [],
    }
    const steps = [...draft.steps, blank]
    commitDraft({ ...draft, steps })
    setOpenSteps((o) => ({ ...o, [steps.length - 1]: true }))
  }

  const endIntentsText = draft.end_intents.join('\n')

  const visualUnavailable = parseError !== null

  return (
    <div className="space-y-3">
      <div className="flex rounded-lg border border-border p-0.5 bg-muted/40 text-sm">
        <button
          type="button"
          disabled={visualUnavailable}
          className={`flex-1 flex items-center justify-center gap-1.5 rounded-md py-2 px-2 transition-colors ${
            tab === 'visual' ? 'bg-card shadow-sm font-medium' : 'text-muted-foreground hover:text-foreground'
          } ${visualUnavailable ? 'opacity-50 cursor-not-allowed' : ''}`}
          onClick={() => setTab('visual')}
        >
          <LayoutList className="h-4 w-4 shrink-0" />
          可视化编辑
        </button>
        <button
          type="button"
          className={`flex-1 flex items-center justify-center gap-1.5 rounded-md py-2 px-2 transition-colors ${
            tab === 'json' ? 'bg-card shadow-sm font-medium' : 'text-muted-foreground hover:text-foreground'
          }`}
          onClick={() => {
            setTab('json')
            setJsonDraft(
              visualUnavailable ? value : serializeHybridScriptDraft(applyIdentity(draft)),
            )
          }}
        >
          <Code2 className="h-4 w-4 shrink-0" />
          JSON 源码
        </button>
      </div>

      {parseError && (
        <p className="text-xs text-amber-800 dark:text-amber-200 bg-amber-500/15 border border-amber-500/30 rounded-md px-3 py-2">
          当前脚本无法转换为表单：{parseError}。请在「JSON 源码」中修正，或使用下方按钮载入空白模板（会覆盖当前内容）。
        </p>
      )}

      {tab === 'json' ? (
        <div className="space-y-2">
          <textarea
            className="border border-border rounded-md px-3 py-2 bg-background w-full h-72 font-mono text-xs leading-relaxed"
            value={jsonDraft}
            onChange={(e) => {
              const t = e.target.value
              setJsonDraft(t)
              const r = parseHybridScriptDraft(t)
              if (r.ok) {
                const merged = applyIdentity(r.spec)
                setDraft(merged)
                setParseError(null)
                onChange(serializeHybridScriptDraft(merged))
              } else {
                setParseError(r.error)
                onChange(t)
              }
            }}
          />
          <div className="flex flex-wrap gap-2">
            <Button
              htmlType="button"
              type="outline"
              size="small"
              onClick={() => {
                const base = defaultHybridScriptDraft()
                const next = applyIdentity(base)
                const json = serializeHybridScriptDraft(next)
                setJsonDraft(json)
                setDraft(next)
                setParseError(null)
                onChange(json)
                setTab('visual')
              }}
            >
              载入简单示例模板
            </Button>
          </div>
        </div>
      ) : (
        <div className="space-y-4 max-h-[min(58vh,520px)] overflow-y-auto pr-1">
          <section className="rounded-lg border border-border bg-muted/20 p-3 space-y-2">
            <h4 className="text-sm font-medium text-foreground">脚本概要</h4>
            {lockedScriptIdentity ? (
              <p className="text-xs text-muted-foreground rounded-md bg-muted/40 px-2 py-1.5 font-mono">
                脚本逻辑 ID 与版本由系统自动分配：
                <span className="text-foreground"> {lockedScriptIdentity.id}</span> ·
                <span className="text-foreground"> {lockedScriptIdentity.version}</span>
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">
                「脚本逻辑 ID」会写入外呼 JSON，与数据库里的模板编号不同；用于任务侧关联脚本内容。
              </p>
            )}
            <div className="grid gap-2 sm:grid-cols-2">
              {!lockedScriptIdentity && (
                <>
                  <label className="text-xs space-y-1">
                    <span className="text-muted-foreground">脚本逻辑 ID（id）</span>
                    <input
                      className="border border-border rounded-md px-2 py-1.5 bg-background w-full text-sm"
                      value={draft.id}
                      onChange={(e) => commitDraft({ ...draft, id: e.target.value })}
                    />
                  </label>
                  <label className="text-xs space-y-1">
                    <span className="text-muted-foreground">版本（version）</span>
                    <input
                      className="border border-border rounded-md px-2 py-1.5 bg-background w-full text-sm"
                      value={draft.version}
                      onChange={(e) => commitDraft({ ...draft, version: e.target.value })}
                    />
                  </label>
                </>
              )}
              <label className="text-xs space-y-1 sm:col-span-2">
                <span className="text-muted-foreground">从哪一步开始（start_id）</span>
                <select
                  className="border border-border rounded-md px-2 py-1.5 bg-background w-full text-sm"
                  value={draft.start_id}
                  onChange={(e) => commitDraft({ ...draft, start_id: e.target.value })}
                >
                  {stepIds(draft).map((id) => (
                    <option key={id} value={id}>
                      {id}
                    </option>
                  ))}
                </select>
              </label>
              <label className="text-xs space-y-1">
                <span className="text-muted-foreground">最大轮次（max_turns，0=自动）</span>
                <input
                  type="number"
                  min={0}
                  className="border border-border rounded-md px-2 py-1.5 bg-background w-full text-sm"
                  value={draft.max_turns || ''}
                  placeholder="0"
                  onChange={(e) =>
                    commitDraft({ ...draft, max_turns: Math.max(0, parseInt(e.target.value, 10) || 0) })
                  }
                />
              </label>
              <label className="text-xs space-y-1">
                <span className="text-muted-foreground">默认静音超时 ms（silence_timeout_ms）</span>
                <input
                  type="number"
                  min={0}
                  className="border border-border rounded-md px-2 py-1.5 bg-background w-full text-sm"
                  value={draft.silence_timeout_ms || ''}
                  placeholder="8000"
                  onChange={(e) =>
                    commitDraft({
                      ...draft,
                      silence_timeout_ms: Math.max(0, parseInt(e.target.value, 10) || 0),
                    })
                  }
                />
              </label>
              <label className="text-xs space-y-1 sm:col-span-2">
                <span className="text-muted-foreground">挂断意图短语（每行一条，用户说出时结束会话）</span>
                <textarea
                  className="border border-border rounded-md px-2 py-1.5 bg-background w-full text-sm h-20"
                  value={endIntentsText}
                  onChange={(e) =>
                    commitDraft({
                      ...draft,
                      end_intents: e.target.value.split(/\r?\n/).map((l) => l.trim()).filter(Boolean),
                    })
                  }
                />
              </label>
            </div>
          </section>

          <section className="space-y-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h4 className="text-sm font-medium">流程步骤</h4>
              <div className="flex flex-wrap gap-1">
                {(['say', 'listen', 'condition', 'llm_reply', 'end'] as const).map((t) => (
                  <Button key={t} htmlType="button" type="outline" size="small" onClick={() => addStep(t)}>
                    <Plus className="h-3.5 w-3.5 mr-1" />
                    {HYBRID_STEP_TYPE_LABELS[t]}
                  </Button>
                ))}
              </div>
            </div>

            {draft.steps.length === 0 ? (
              <p className="text-sm text-muted-foreground py-4 text-center border border-dashed border-border rounded-lg">
                暂无步骤，点击上方按钮添加。
              </p>
            ) : (
              <ul className="space-y-2">
                {draft.steps.map((step, i) => {
                  const open = openSteps[i] ?? i >= draft.steps.length - 2
                  const opts = nextIdOptions(draft, step.id.trim())
                  return (
                    <li
                      key={`${i}-${step.id}-${step.type}`}
                      className="rounded-lg border border-border bg-card overflow-hidden"
                    >
                      <div className="flex items-center gap-2 px-2 py-2 bg-muted/30 border-b border-border">
                        <button
                          type="button"
                          className="p-1 rounded hover:bg-muted text-muted-foreground"
                          aria-expanded={open}
                          onClick={() => toggleStep(i)}
                        >
                          {open ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
                        </button>
                        <span className="text-xs font-mono font-medium truncate flex-1">{step.id || `步骤 ${i + 1}`}</span>
                        <span className="text-xs text-muted-foreground hidden sm:inline">
                          {HYBRID_STEP_TYPE_LABELS[step.type]}
                        </span>
                        <div className="flex items-center gap-0.5">
                          <button
                            type="button"
                            className="p-1 rounded hover:bg-muted text-muted-foreground disabled:opacity-30"
                            disabled={i === 0}
                            onClick={() => moveStep(i, -1)}
                            aria-label="上移"
                          >
                            <ChevronUp className="h-4 w-4" />
                          </button>
                          <button
                            type="button"
                            className="p-1 rounded hover:bg-muted text-muted-foreground disabled:opacity-30"
                            disabled={i === draft.steps.length - 1}
                            onClick={() => moveStep(i, 1)}
                            aria-label="下移"
                          >
                            <ChevronDown className="h-4 w-4" />
                          </button>
                          <button
                            type="button"
                            className="p-1 rounded hover:bg-destructive/15 text-destructive"
                            onClick={() => removeStep(i)}
                            aria-label="删除"
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </div>
                      </div>
                      {open && (
                        <div className="p-3 space-y-3 text-sm">
                          <div className="grid gap-2 sm:grid-cols-2">
                            <label className="text-xs space-y-1">
                              <span className="text-muted-foreground">步骤 ID（英文/数字，唯一）</span>
                              <input
                                className="border border-border rounded-md px-2 py-1.5 bg-background w-full font-mono text-xs"
                                value={step.id}
                                onChange={(e) => updateStep(i, { id: e.target.value })}
                              />
                            </label>
                            <label className="text-xs space-y-1">
                              <span className="text-muted-foreground">类型</span>
                              <select
                                className="border border-border rounded-md px-2 py-1.5 bg-background w-full text-xs"
                                value={step.type}
                                onChange={(e) =>
                                  updateStep(i, {
                                    type: e.target.value as HybridStepType,
                                    listen_timeout_ms:
                                      e.target.value === 'listen' && !step.listen_timeout_ms ? 8000 : step.listen_timeout_ms,
                                  })
                                }
                              >
                                {(Object.keys(HYBRID_STEP_TYPE_LABELS) as HybridStepType[]).map((t) => (
                                  <option key={t} value={t}>
                                    {HYBRID_STEP_TYPE_LABELS[t]}
                                  </option>
                                ))}
                              </select>
                            </label>
                          </div>

                          {(step.type === 'say' || step.type === 'llm_reply') && (
                            <label className="text-xs space-y-1 block">
                              <span className="text-muted-foreground">播报文案（prompt）</span>
                              <textarea
                                className="border border-border rounded-md px-2 py-1.5 bg-background w-full min-h-[72px] text-xs"
                                value={step.prompt}
                                onChange={(e) => updateStep(i, { prompt: e.target.value })}
                              />
                            </label>
                          )}

                          {step.type !== 'end' && (
                            <label className="text-xs space-y-1 block max-w-md">
                              <span className="text-muted-foreground">下一步（next_id）</span>
                              <select
                                className="border border-border rounded-md px-2 py-1.5 bg-background w-full text-xs"
                                value={step.next_id}
                                onChange={(e) => updateStep(i, { next_id: e.target.value })}
                              >
                                {opts.map((o) => (
                                  <option key={o.value || '__end__'} value={o.value}>
                                    {o.label}
                                  </option>
                                ))}
                              </select>
                            </label>
                          )}

                          {step.type === 'listen' && (
                            <>
                              <div className="grid gap-2 sm:grid-cols-2">
                                <label className="text-xs space-y-1">
                                  <span className="text-muted-foreground">本步倾听超时 ms（listen_timeout_ms）</span>
                                  <input
                                    type="number"
                                    min={0}
                                    className="border border-border rounded-md px-2 py-1.5 bg-background w-full text-xs"
                                    value={step.listen_timeout_ms || ''}
                                    placeholder="留空则用全局默认"
                                    onChange={(e) =>
                                      updateStep(i, {
                                        listen_timeout_ms: Math.max(0, parseInt(e.target.value, 10) || 0),
                                      })
                                    }
                                  />
                                </label>
                                <label className="text-xs space-y-1">
                                  <span className="text-muted-foreground">超时后的步骤（listen_fallback_id）</span>
                                  <select
                                    className="border border-border rounded-md px-2 py-1.5 bg-background w-full text-xs"
                                    value={step.listen_fallback_id}
                                    onChange={(e) => updateStep(i, { listen_fallback_id: e.target.value })}
                                  >
                                    {opts.map((o) => (
                                      <option key={`fb-${o.value || '__end__'}`} value={o.value}>
                                        {o.label}
                                      </option>
                                    ))}
                                  </select>
                                </label>
                              </div>
                              <p className="text-[11px] text-muted-foreground leading-snug">
                                语义分支需要服务端配置 CHECK_LLM_*；仅按键分流时可只填「按键映射」、不添加语义分支。
                              </p>
                              <BranchList
                                title="语义分支（LLM，描述 → 下一步）"
                                items={step.transitions}
                                kind="description"
                                options={opts}
                                onChange={(transitions) => updateStep(i, { transitions })}
                              />
                              <DtmfList
                                items={step.dtmf_transitions}
                                options={opts}
                                onChange={(dtmf_transitions) => updateStep(i, { dtmf_transitions })}
                              />
                            </>
                          )}

                          {step.type === 'condition' && (
                            <>
                              <label className="text-xs space-y-1 block max-w-md">
                                <span className="text-muted-foreground">未匹配时的步骤（fallback_id）</span>
                                <select
                                  className="border border-border rounded-md px-2 py-1.5 bg-background w-full text-xs"
                                  value={step.fallback_id}
                                  onChange={(e) => updateStep(i, { fallback_id: e.target.value })}
                                >
                                  {opts.map((o) => (
                                    <option key={`cfb-${o.value || '__end__'}`} value={o.value}>
                                      {o.label}
                                    </option>
                                  ))}
                                </select>
                              </label>
                              <BranchList
                                title="关键字分支"
                                items={step.transitions}
                                kind="keywords"
                                options={opts}
                                onChange={(transitions) => updateStep(i, { transitions })}
                              />
                            </>
                          )}

                          {step.type === 'llm_reply' && (
                            <label className="text-xs space-y-1 block">
                              <span className="text-muted-foreground">给大模型的指令（llm_instruction）</span>
                              <textarea
                                className="border border-border rounded-md px-2 py-1.5 bg-background w-full min-h-[56px] text-xs"
                                value={step.llm_instruction}
                                onChange={(e) => updateStep(i, { llm_instruction: e.target.value })}
                              />
                            </label>
                          )}

                          {(step.type === 'say' || step.type === 'listen') && (
                            <details className="text-xs text-muted-foreground">
                              <summary className="cursor-pointer select-none text-foreground/80">高级：重试 / 延时</summary>
                              <div className="grid gap-2 sm:grid-cols-2 mt-2">
                                <label className="space-y-1">
                                  <span>retry</span>
                                  <input
                                    type="number"
                                    min={0}
                                    className="border border-border rounded-md px-2 py-1 bg-background w-full"
                                    value={step.retry || ''}
                                    onChange={(e) =>
                                      updateStep(i, { retry: Math.max(0, parseInt(e.target.value, 10) || 0) })
                                    }
                                  />
                                </label>
                                <label className="space-y-1">
                                  <span>timeout_ms（进入下一步前等待）</span>
                                  <input
                                    type="number"
                                    min={0}
                                    className="border border-border rounded-md px-2 py-1 bg-background w-full"
                                    value={step.timeout_ms || ''}
                                    onChange={(e) =>
                                      updateStep(i, { timeout_ms: Math.max(0, parseInt(e.target.value, 10) || 0) })
                                    }
                                  />
                                </label>
                              </div>
                            </details>
                          )}
                        </div>
                      )}
                    </li>
                  )
                })}
              </ul>
            )}
          </section>
        </div>
      )}
    </div>
  )
}

function BranchList({
  title,
  items,
  kind,
  options,
  onChange,
}: {
  title: string
  items: HybridTransition[]
  kind: 'description' | 'keywords'
  options: { value: string; label: string }[]
  onChange: (next: HybridTransition[]) => void
}) {
  const add = () => {
    onChange([...items, { description: '', intent: '', contains: '', equals: '', next_id: '' }])
  }
  const patch = (idx: number, p: Partial<HybridTransition>) => {
    onChange(items.map((it, i) => (i === idx ? { ...it, ...p } : it)))
  }
  const remove = (idx: number) => {
    onChange(items.filter((_, i) => i !== idx))
  }
  return (
    <div className="rounded-md border border-border/80 bg-muted/10 p-2 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium">{title}</span>
        <Button htmlType="button" type="outline" size="small" className="h-7 text-xs" onClick={add}>
          <Plus className="h-3 w-3 mr-1" />
          添加分支
        </Button>
      </div>
      {items.length === 0 ? (
        <p className="text-[11px] text-muted-foreground py-1">暂无分支</p>
      ) : (
        <ul className="space-y-2">
          {items.map((row, idx) => (
            <li key={idx} className="flex flex-col gap-2 rounded border border-border/60 bg-background p-2">
              {kind === 'description' ? (
                <label className="text-[11px] space-y-1">
                  <span className="text-muted-foreground">用户说法描述（description）</span>
                  <textarea
                    className="border border-border rounded px-2 py-1 w-full text-xs min-h-[48px]"
                    value={row.description}
                    onChange={(e) => patch(idx, { description: e.target.value })}
                    placeholder="例如：用户同意继续、表示方便…"
                  />
                </label>
              ) : (
                <div className="grid gap-2 sm:grid-cols-3">
                  <label className="text-[11px] space-y-1">
                    <span className="text-muted-foreground">完全等于 equals</span>
                    <input
                      className="border border-border rounded px-2 py-1 w-full text-xs"
                      value={row.equals}
                      onChange={(e) => patch(idx, { equals: e.target.value })}
                    />
                  </label>
                  <label className="text-[11px] space-y-1">
                    <span className="text-muted-foreground">包含 contains</span>
                    <input
                      className="border border-border rounded px-2 py-1 w-full text-xs"
                      value={row.contains}
                      onChange={(e) => patch(idx, { contains: e.target.value })}
                    />
                  </label>
                  <label className="text-[11px] space-y-1">
                    <span className="text-muted-foreground">意图 intent</span>
                    <input
                      className="border border-border rounded px-2 py-1 w-full text-xs"
                      value={row.intent}
                      onChange={(e) => patch(idx, { intent: e.target.value })}
                    />
                  </label>
                </div>
              )}
              <div className="flex flex-wrap items-end gap-2">
                <label className="text-[11px] space-y-1 flex-1 min-w-[140px]">
                  <span className="text-muted-foreground">跳到</span>
                  <select
                    className="border border-border rounded-md px-2 py-1 bg-background w-full text-xs"
                    value={row.next_id}
                    onChange={(e) => patch(idx, { next_id: e.target.value })}
                  >
                    <option value="">选择下一步</option>
                    {options
                      .filter((o) => o.value !== '')
                      .map((o) => (
                        <option key={o.value} value={o.value}>
                          {o.label}
                        </option>
                      ))}
                  </select>
                </label>
                <Button htmlType="button" type="outline" size="small" className="h-8 shrink-0" onClick={() => remove(idx)}>
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function DtmfList({
  items,
  options,
  onChange,
}: {
  items: HybridDTMFTransition[]
  options: { value: string; label: string }[]
  onChange: (next: HybridDTMFTransition[]) => void
}) {
  const digits = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '*', '#']
  const add = () => {
    onChange([...items, { digit: '1', next_id: '' }])
  }
  const patch = (idx: number, p: Partial<HybridDTMFTransition>) => {
    onChange(items.map((it, i) => (i === idx ? { ...it, ...p } : it)))
  }
  const remove = (idx: number) => {
    onChange(items.filter((_, i) => i !== idx))
  }
  return (
    <div className="rounded-md border border-border/80 bg-muted/10 p-2 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium">电话按键映射（dtmf_transitions）</span>
        <Button htmlType="button" type="outline" size="small" className="h-7 text-xs" onClick={add}>
          <Plus className="h-3 w-3 mr-1" />
          添加按键
        </Button>
      </div>
      {items.length === 0 ? (
        <p className="text-[11px] text-muted-foreground py-1">未配置按键时将仅靠语音或默认 next</p>
      ) : (
        <ul className="space-y-2">
          {items.map((row, idx) => (
            <li key={idx} className="flex flex-wrap items-end gap-2 rounded border border-border/60 bg-background p-2">
              <label className="text-[11px] space-y-1">
                <span className="text-muted-foreground">按键</span>
                <select
                  className="border border-border rounded-md px-2 py-1 bg-background text-xs"
                  value={row.digit}
                  onChange={(e) => patch(idx, { digit: e.target.value })}
                >
                  {digits.map((d) => (
                    <option key={d} value={d}>
                      {d}
                    </option>
                  ))}
                </select>
              </label>
              <label className="text-[11px] space-y-1 flex-1 min-w-[160px]">
                <span className="text-muted-foreground">跳到</span>
                <select
                  className="border border-border rounded-md px-2 py-1 bg-background w-full text-xs"
                  value={row.next_id}
                  onChange={(e) => patch(idx, { next_id: e.target.value })}
                >
                  <option value="">选择下一步</option>
                  {options
                    .filter((o) => o.value !== '')
                    .map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
                      </option>
                    ))}
                </select>
              </label>
              <Button htmlType="button" type="outline" size="small" className="h-8" onClick={() => remove(idx)}>
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

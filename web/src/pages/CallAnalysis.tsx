import { useCallback, useEffect, useRef, useState } from 'react'
import { Download, Link2, Loader2, RefreshCw, Sparkles, Upload } from 'lucide-react'
import AdminLayout from '@/components/Layout/AdminLayout'
import Card from '@/components/UI/Card'
import Button from '@/components/UI/Button'
import Input from '@/components/UI/Input'
import { getCallAnalysisResult, type CallAnalysisCreateData, type CallAnalysisExportDoc } from '@/api/sipContactCenter'
import { callAnalysisWebSocketURL } from '@/utils/callAnalysisWs'
import { showAlert } from '@/utils/notification'

type Mode = 'upload' | 'url'

type AsrClause = { fragment: string; cumulative: string }

function downloadExportDoc(doc: CallAnalysisExportDoc) {
  const blob = new Blob([JSON.stringify(doc, null, 2)], { type: 'application/json;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `call-analysis-${doc.id}.json`
  a.click()
  URL.revokeObjectURL(a.href)
}

function formatLLMBlock(v: unknown): string {
  if (v == null) return ''
  if (typeof v === 'string') return v
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

function mapCompleteToResult(msg: { export_doc: CallAnalysisExportDoc }): CallAnalysisCreateData {
  const doc = msg.export_doc
  return {
    id: doc.id,
    meta: doc.meta,
    asr: doc.asr,
    llm: doc.llm_analysis,
    llm_raw: doc.llm_raw || '',
    export_doc: doc,
  }
}

function sourceLabel(source: string): string {
  if (source === 'url') return 'URL'
  if (source === 'ws_upload') return '上传（WebSocket）'
  return source === 'upload' ? '上传' : source
}

const CallAnalysis = () => {
  const [mode, setMode] = useState<Mode>('upload')
  const [audioUrl, setAudioUrl] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<CallAnalysisCreateData | null>(null)
  const [liveStage, setLiveStage] = useState('')
  const [asrClauses, setAsrClauses] = useState<AsrClause[]>([])
  const fileRef = useRef<HTMLInputElement>(null)
  const wsRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    return () => {
      if (wsRef.current && (wsRef.current.readyState === WebSocket.OPEN || wsRef.current.readyState === WebSocket.CONNECTING)) {
        wsRef.current.close()
      }
    }
  }, [])

  const runAnalysis = useCallback(async () => {
    setLoading(true)
    setResult(null)
    setAsrClauses([])
    setLiveStage('连接中…')
    let ws: WebSocket | null = null
    try {
      ws = new WebSocket(callAnalysisWebSocketURL())
      wsRef.current = ws

      await new Promise<void>((resolve, reject) => {
        const connTimer = window.setTimeout(() => reject(new Error('WebSocket 连接超时')), 30000)
        ws!.onopen = () => {
          window.clearTimeout(connTimer)
          resolve()
        }
        ws!.onerror = () => {
          window.clearTimeout(connTimer)
          reject(new Error('WebSocket 连接失败'))
        }
      })

      if (mode === 'url') {
        const u = audioUrl.trim()
        if (!u) {
          showAlert('请输入音频 URL（http/https）', 'warning')
          ws.close()
          return
        }
        ws.send(JSON.stringify({ type: 'start', audio_url: u }))
      } else {
        if (!file) {
          showAlert('请选择音频文件', 'warning')
          ws.close()
          return
        }
        ws.send(JSON.stringify({ type: 'start_file', filename: file.name, size_bytes: file.size }))
        const buf = await file.arrayBuffer()
        ws.send(buf)
      }

      await new Promise<void>((resolve, reject) => {
        let settled = false
        const stallMs = 50 * 60 * 1000
        const stallTimer = window.setTimeout(() => {
          if (settled) return
          settled = true
          try {
            ws?.close()
          } catch {
            /* ignore */
          }
          reject(new Error('分析超时（超过 50 分钟）'))
        }, stallMs)

        const done = (fn: () => void) => {
          if (settled) return
          settled = true
          window.clearTimeout(stallTimer)
          fn()
        }

        ws!.onmessage = (ev) => {
          try {
            const msg = JSON.parse(ev.data as string) as Record<string, unknown>
            const typ = String(msg.type || '')
            switch (typ) {
              case 'hello':
                setLiveStage('已连接，开始转写…')
                break
              case 'stage':
                if (msg.stage === 'asr') setLiveStage('语音识别中（实时分句）…')
                if (msg.stage === 'llm') setLiveStage('大模型分析中…')
                break
              case 'asr_sentence':
                setAsrClauses((prev) => [
                  ...prev,
                  { fragment: String(msg.fragment || ''), cumulative: String(msg.cumulative || '') },
                ])
                break
              case 'asr_done':
                setLiveStage('转写完成，等待模型分析…')
                break
              case 'llm_done':
                break
              case 'complete': {
                const doc = msg.export_doc as CallAnalysisExportDoc | undefined
                if (doc && doc.id) {
                  setResult(mapCompleteToResult({ export_doc: doc }))
                  showAlert('分析完成', 'success')
                } else {
                  showAlert('完成但未返回 export_doc', 'error')
                }
                try {
                  ws?.close()
                } catch {
                  /* ignore */
                }
                done(() => resolve())
                break
              }
              case 'error':
                showAlert(String(msg.message || '分析失败'), 'error')
                try {
                  ws?.close()
                } catch {
                  /* ignore */
                }
                done(() => reject(new Error(String(msg.message || 'error'))))
                break
              default:
                break
            }
          } catch {
            showAlert('收到无效消息', 'error')
            try {
              ws?.close()
            } catch {
              /* ignore */
            }
            done(() => reject(new Error('invalid message')))
          }
        }

        ws!.onclose = () => {
          wsRef.current = null
          if (!settled) {
            done(() => reject(new Error('连接已关闭（未完成分析）')))
          }
        }
        ws!.onerror = () => {
          if (!settled) {
            done(() => reject(new Error('WebSocket 异常')))
          }
        }
      })
    } catch (e: unknown) {
      const err = e as { message?: string }
      if (err?.message) {
        showAlert(err.message, 'error')
      }
    } finally {
      setLoading(false)
      setLiveStage('')
      wsRef.current = null
    }
  }, [mode, file, audioUrl])

  const refetchById = useCallback(async () => {
    const id = result?.id?.trim()
    if (!id) return
    setLoading(true)
    try {
      const res = await getCallAnalysisResult(id)
      if (res.code === 200 && res.data) {
        const doc = res.data
        setResult({
          id: doc.id,
          meta: doc.meta,
          asr: doc.asr,
          llm: doc.llm_analysis,
          llm_raw: doc.llm_raw || '',
          export_doc: doc,
        })
        showAlert('已刷新', 'success')
      } else {
        showAlert(res.msg || '记录不存在或已过期', 'error')
      }
    } catch (e: unknown) {
      const err = e as { msg?: string; message?: string }
      showAlert(err?.msg || err?.message || '刷新失败', 'error')
    } finally {
      setLoading(false)
    }
  }, [result?.id])

  const exportDoc = result?.export_doc

  return (
    <AdminLayout
      title="通话 AI 分析"
      description="通过 WebSocket 推流：实时推送 ASR 分句，结束后返回与导出 JSON（与 HTTP 接口同一套 export_doc）。"
      actions={
        exportDoc ? (
          <Button type="button" variant="outline" size="sm" leftIcon={<Download className="w-4 h-4" />} onClick={() => downloadExportDoc(exportDoc)}>
            导出 JSON
          </Button>
        ) : null
      }
    >
      <div className="mx-auto max-w-4xl space-y-6 p-4 pb-12 lg:p-8">
        <Card className="border border-slate-200/80 bg-white/90 p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900/80">
          <div className="mb-4 flex flex-wrap items-center gap-2">
            <span className="text-sm font-medium text-slate-600 dark:text-slate-300">来源</span>
            <div className="inline-flex rounded-lg border border-slate-200 bg-slate-50 p-0.5 dark:border-slate-600 dark:bg-slate-800">
              <button
                type="button"
                onClick={() => {
                  setMode('upload')
                  setAudioUrl('')
                }}
                className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                  mode === 'upload'
                    ? 'bg-white text-blue-600 shadow-sm dark:bg-slate-700 dark:text-blue-400'
                    : 'text-slate-600 hover:text-slate-900 dark:text-slate-400'
                }`}
              >
                <Upload className="h-4 w-4" />
                上传文件
              </button>
              <button
                type="button"
                onClick={() => {
                  setMode('url')
                  setFile(null)
                  if (fileRef.current) fileRef.current.value = ''
                }}
                className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                  mode === 'url'
                    ? 'bg-white text-blue-600 shadow-sm dark:bg-slate-700 dark:text-blue-400'
                    : 'text-slate-600 hover:text-slate-900 dark:text-slate-400'
                }`}
              >
                <Link2 className="h-4 w-4" />
                音频 URL
              </button>
            </div>
          </div>

          {mode === 'upload' ? (
            <div className="space-y-3">
              <input
                ref={fileRef}
                type="file"
                accept="audio/*,.wav,.mp3,.m4a,.ogg,.webm,.flac"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  setFile(f ?? null)
                }}
              />
              <div className="flex flex-wrap items-center gap-3">
                <Button type="button" variant="outline" onClick={() => fileRef.current?.click()}>
                  选择文件
                </Button>
                <span className="text-sm text-slate-600 dark:text-slate-400">
                  {file ? `${file.name}（${(file.size / (1024 * 1024)).toFixed(2)} MB）` : '未选择文件'}
                </span>
              </div>
              <p className="text-xs text-slate-500 dark:text-slate-500">
                经 WebSocket 二进制帧上传；服务端 ffmpeg 解码，单文件不超过 80MB。长音频 ASR 为 1× 实时，请保持页面打开。
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              <Input
                label="音频地址"
                placeholder="https://example.com/recording.mp3"
                value={audioUrl}
                onChange={(e) => setAudioUrl(e.target.value)}
                size="md"
              />
              <p className="text-xs text-slate-500 dark:text-slate-500">需为公网可访问的 http(s) 直链。</p>
            </div>
          )}

          <div className="mt-5 flex flex-wrap gap-3">
            <Button type="button" variant="primary" disabled={loading} loading={loading} leftIcon={<Sparkles className="h-4 w-4" />} onClick={() => void runAnalysis()}>
              开始分析（WebSocket）
            </Button>
            {result?.id ? (
              <Button type="button" variant="outline" disabled={loading} leftIcon={<RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />} onClick={() => void refetchById()}>
                按 ID 刷新
              </Button>
            ) : null}
          </div>
        </Card>

        {loading && (
          <div className="flex flex-col gap-2 rounded-lg border border-blue-100 bg-blue-50/80 px-4 py-3 text-sm text-blue-800 dark:border-blue-900/40 dark:bg-blue-950/40 dark:text-blue-200">
            <div className="flex items-center gap-2">
              <Loader2 className="h-4 w-4 animate-spin shrink-0" />
              <span>{liveStage || '处理中…'}</span>
            </div>
            {asrClauses.length > 0 && (
              <div className="mt-1 max-h-40 overflow-y-auto rounded border border-blue-200/60 bg-white/70 p-2 text-xs dark:border-blue-900/50 dark:bg-slate-900/60">
                <div className="mb-1 font-medium text-blue-900/80 dark:text-blue-200">ASR 分句（实时）</div>
                <ul className="space-y-1">
                  {asrClauses.map((c, i) => (
                    <li key={i} className="text-slate-700 dark:text-slate-300">
                      <span className="text-blue-600 dark:text-blue-400">+{c.fragment || '…'}</span>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        )}

        {result && (
          <div className="space-y-4">
            <Card className="border border-slate-200/80 bg-white/90 p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900/80">
              <h3 className="mb-3 text-sm font-semibold text-slate-800 dark:text-slate-100">任务信息</h3>
              <dl className="grid gap-2 text-sm sm:grid-cols-2">
                <div>
                  <dt className="text-slate-500 dark:text-slate-400">任务 ID</dt>
                  <dd className="font-mono text-xs text-slate-800 dark:text-slate-200">{result.id}</dd>
                </div>
                <div>
                  <dt className="text-slate-500 dark:text-slate-400">来源</dt>
                  <dd className="text-slate-800 dark:text-slate-200">{sourceLabel(result.meta.source)}</dd>
                </div>
                <div>
                  <dt className="text-slate-500 dark:text-slate-400">PCM 时长（约）</dt>
                  <dd className="text-slate-800 dark:text-slate-200">{result.meta.pcm_duration_sec.toFixed(1)} 秒</dd>
                </div>
                <div>
                  <dt className="text-slate-500 dark:text-slate-400">PCM 采样率</dt>
                  <dd className="text-slate-800 dark:text-slate-200">
                    {result.meta.pcm_sample_rate_hz != null ? `${result.meta.pcm_sample_rate_hz} Hz（须与 ASR 模型一致）` : '—'}
                  </dd>
                </div>
                <div>
                  <dt className="text-slate-500 dark:text-slate-400">ASR / LLM</dt>
                  <dd className="text-slate-800 dark:text-slate-200">
                    {result.meta.asr_model} · {result.meta.llm_model}
                  </dd>
                </div>
              </dl>
            </Card>

            {asrClauses.length > 0 && (
              <Card className="border border-slate-200/80 bg-white/90 p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900/80">
                <h3 className="mb-2 text-sm font-semibold text-slate-800 dark:text-slate-100">ASR 分句记录</h3>
                <ul className="max-h-48 space-y-1 overflow-y-auto text-xs text-slate-700 dark:text-slate-300">
                  {asrClauses.map((c, i) => (
                    <li key={i}>
                      <span className="font-mono text-slate-500 dark:text-slate-500">[{i + 1}]</span> {c.fragment || '（空）'}
                    </li>
                  ))}
                </ul>
              </Card>
            )}

            <Card className="border border-slate-200/80 bg-white/90 p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900/80">
              <div className="mb-2 flex items-center justify-between gap-2">
                <h3 className="text-sm font-semibold text-slate-800 dark:text-slate-100">转写原文</h3>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    void navigator.clipboard.writeText(result.asr.transcript || '')
                    showAlert('已复制', 'success')
                  }}
                >
                  复制
                </Button>
              </div>
              <pre className="max-h-72 overflow-auto whitespace-pre-wrap rounded-md border border-slate-100 bg-slate-50 p-3 text-xs leading-relaxed text-slate-800 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200">
                {result.asr.transcript || '（空）'}
              </pre>
            </Card>

            <Card className="border border-slate-200/80 bg-white/90 p-5 shadow-sm dark:border-slate-700 dark:bg-slate-900/80">
              <h3 className="mb-2 text-sm font-semibold text-slate-800 dark:text-slate-100">LLM 分析（结构化）</h3>
              <pre className="max-h-96 overflow-auto rounded-md border border-slate-100 bg-slate-50 p-3 text-xs leading-relaxed text-slate-800 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200">
                {formatLLMBlock(result.llm)}
              </pre>
              {result.llm_raw ? (
                <details className="mt-3 text-xs text-slate-500 dark:text-slate-400">
                  <summary className="cursor-pointer select-none font-medium text-slate-600 dark:text-slate-300">原始模型输出</summary>
                  <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap rounded border border-slate-100 bg-white p-2 dark:border-slate-700 dark:bg-slate-900">
                    {result.llm_raw}
                  </pre>
                </details>
              ) : null}
            </Card>

            {exportDoc ? (
              <div className="flex justify-end">
                <Button type="button" variant="primary" leftIcon={<Download className="h-4 w-4" />} onClick={() => downloadExportDoc(exportDoc)}>
                  下载完整 JSON（含原文与分析）
                </Button>
              </div>
            ) : null}
          </div>
        )}
      </div>
    </AdminLayout>
  )
}

export default CallAnalysis

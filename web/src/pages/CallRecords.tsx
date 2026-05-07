import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { AlertCircle, MicOff } from 'lucide-react'
import { Button, Card, Drawer } from '@arco-design/web-react'
import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import {
  getSIPCall,
  listSIPCalls,
  resolveSipRecordingUrl,
  sipAiEndStatusI18nKey,
  type SIPCallDialogTurn,
  type SIPCallRow,
} from '@/api/sipCalls'
import { showAlert } from '@/utils/notification'
import { EllipsisHoverCell } from '@/pages/ContactCenter/EllipsisHoverCell'
import CallAudioPlayer from '@/components/CallAudioPlayer'

const CallRecords = () => {
  const [loading, setLoading] = useState(false)
  const [calls, setCalls] = useState<SIPCallRow[]>([])
  const [callsPage, setCallsPage] = useState(1)
  const [callsTotal, setCallsTotal] = useState(0)
  const [callFilter, setCallFilter] = useState('')
  const [callsSearchNonce, setCallsSearchNonce] = useState(0)
  const [callDetailDrawerId, setCallDetailDrawerId] = useState<number | null>(null)
  const [callDetailDrawerData, setCallDetailDrawerData] = useState<SIPCallRow | null>(null)
  const [callDetailDrawerLoading, setCallDetailDrawerLoading] = useState(false)
  const [callDetailDrawerFailed, setCallDetailDrawerFailed] = useState(false)
  const pageSize = 20

  const loadCalls = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listSIPCalls(callsPage, pageSize, {
        callId: callFilter.trim() || undefined,
      })
      if (res.code === 200 && res.data) {
        setCalls(res.data.list || [])
        setCallsTotal(res.data.total || 0)
      } else {
        showAlert(res.msg || '加载失败', 'error')
      }
    } catch (e: any) {
      showAlert(e?.msg || '加载失败', 'error')
    } finally {
      setLoading(false)
    }
  }, [callsPage, callFilter])

  useEffect(() => {
    void loadCalls()
  }, [loadCalls, callsSearchNonce])

  const fmt = (s?: string) => (s ? new Date(s).toLocaleString() : '—')
  const mapCallState = (state?: string) => {
    const s = (state || '').toLowerCase()
    const map: Record<string, string> = {
      ended: '已结束',
      ringing: '振铃中',
      answered: '已接通',
      failed: '失败',
    }
    return map[s] || state || '—'
  }
  const mapDirection = (dir?: string) => {
    const d = (dir || '').toLowerCase()
    const map: Record<string, string> = {
      inbound: '呼入',
      outbound: '呼出',
    }
    return map[d] || dir || '—'
  }

  const closeCallDetailDrawer = () => {
    setCallDetailDrawerId(null)
    setCallDetailDrawerData(null)
    setCallDetailDrawerLoading(false)
    setCallDetailDrawerFailed(false)
  }

  const openCallDetailDrawer = async (id: number) => {
    setCallDetailDrawerId(id)
    setCallDetailDrawerData(null)
    setCallDetailDrawerLoading(true)
    setCallDetailDrawerFailed(false)
    try {
      const res = await getSIPCall(id)
      if (res.code === 200 && res.data) setCallDetailDrawerData(res.data)
      else setCallDetailDrawerFailed(true)
    } catch {
      setCallDetailDrawerFailed(true)
    } finally {
      setCallDetailDrawerLoading(false)
    }
  }

  const detailField = (label: string, value: ReactNode) => (
    <div className="rounded-md border border-border bg-background/80 p-2.5 text-sm">
      <div className="mb-0.5 text-[11px] text-muted-foreground">{label}</div>
      <div className="break-words font-medium">{value ?? '—'}</div>
    </div>
  )

  const formatTurnMeta = (turn: SIPCallDialogTurn) => {
    const parts: string[] = []
    if (turn.trigger?.trim()) parts.push(`触发: ${turn.trigger.trim()}`)
    if (turn.scriptStepId?.trim()) parts.push(`脚本步骤: ${turn.scriptStepId.trim()}`)
    if (turn.routeIntent?.trim()) parts.push(`路由意图: ${turn.routeIntent.trim()}`)
    return parts.length ? parts.join(' · ') : ''
  }

  const formatTurnTimings = (turn: SIPCallDialogTurn) => {
    const t: string[] = []
    if (turn.llmFirstMs != null && turn.llmFirstMs > 0) t.push(`LLM 首字 ${turn.llmFirstMs}ms`)
    if (turn.llmWallMs != null && turn.llmWallMs > 0) t.push(`LLM 总耗时 ${turn.llmWallMs}ms`)
    if (turn.ttsMs != null && turn.ttsMs > 0) t.push(`TTS ${turn.ttsMs}ms`)
    if (turn.pipelineMs != null && turn.pipelineMs > 0) t.push(`流水线 ${turn.pipelineMs}ms`)
    return t.length ? t.join(' · ') : ''
  }

  const formatTurnProviders = (turn: SIPCallDialogTurn) => {
    const p: string[] = []
    if (turn.asrProvider?.trim()) p.push(`ASR: ${turn.asrProvider.trim()}`)
    if (turn.ttsProvider?.trim()) p.push(`TTS: ${turn.ttsProvider.trim()}`)
    if (turn.llmModel?.trim()) p.push(`模型: ${turn.llmModel.trim()}`)
    return p.length ? p.join(' · ') : ''
  }

  return (
    <BaseLayout title="通话记录" description="云联络中心 / 通话记录">
      <div className="mb-3 flex flex-wrap gap-2 items-center">
        <input
          className="border border-border rounded-md px-3 py-1.5 text-sm bg-background max-w-xs"
          placeholder="Call-ID"
          value={callFilter}
          onChange={(e) => setCallFilter(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              setCallsPage(1)
              setCallsSearchNonce((n) => n + 1)
            }
          }}
        />
        <Button
          type="primary"
          size="small"
          onClick={() => {
            setCallsPage(1)
            setCallsSearchNonce((n) => n + 1)
          }}
        >
          搜索
        </Button>
      </div>
      <p className="text-xs text-muted-foreground max-w-2xl mb-3">
        列表仅展示主被叫号码；原始 SIP From/To 头保存在服务端，可在详情里折叠查看（一般用于排障）。
      </p>
      <Card bordered={false} bodyStyle={{ padding: 0 }}>
        <div className="overflow-x-auto">
          <table className="min-w-[1280px] w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="text-left p-3 whitespace-nowrap">ID</th>
                <th className="text-left p-3 whitespace-nowrap min-w-[180px]">Call-ID</th>
                <th className="text-left p-3 whitespace-nowrap">状态</th>
                <th className="text-left p-3 whitespace-nowrap">Dir</th>
                <th className="text-left p-3 min-w-[140px]">From</th>
                <th className="text-left p-3 min-w-[140px]">To</th>
                <th className="text-left p-3 whitespace-nowrap">Dur(s)</th>
                <th className="text-left p-3 whitespace-nowrap min-w-[120px]">结束方式</th>
                <th className="text-left p-3 whitespace-nowrap">时间</th>
                <th className="text-left p-3 whitespace-nowrap min-w-[72px]">对话轮次</th>
                <th className="text-left p-3 whitespace-nowrap min-w-[72px]">录音</th>
                <th className="text-left p-3 min-w-[120px]">失败原因</th>
                <th className="text-right p-3 whitespace-nowrap min-w-[100px]">操作</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td className="p-6 text-center" colSpan={13}>加载中...</td></tr>
              ) : calls.length === 0 ? (
                <tr><td className="p-6 text-center" colSpan={13}>暂无数据</td></tr>
              ) : (
                calls.map((c) => {
                  const hasRec = Boolean(c.recordingUrl && c.recordingUrl.trim())
                  return (
                    <tr key={c.id} className="border-t border-border align-top">
                      <td className="p-3 whitespace-nowrap">{c.id}</td>
                      <td className="p-3 max-w-[240px] align-top"><EllipsisHoverCell text={c.callId} lines={2} mono /></td>
                      <td className="p-3 whitespace-nowrap">{mapCallState(c.state)}</td>
                      <td className="p-3 whitespace-nowrap">{mapDirection(c.direction)}</td>
                      <td className="p-3 max-w-[160px] align-top whitespace-nowrap">
                        <div className="font-medium">{c.fromNumber?.trim() || '—'}</div>
                      </td>
                      <td className="p-3 max-w-[160px] align-top whitespace-nowrap">
                        <div className="font-medium">{c.toNumber?.trim() || '—'}</div>
                      </td>
                      <td className="p-3 whitespace-nowrap">{c.durationSec ?? '—'}</td>
                      <td className="p-3 text-xs max-w-[140px] align-top"><EllipsisHoverCell text={c.endStatus ? sipAiEndStatusI18nKey(c.endStatus) : '—'} lines={2} /></td>
                      <td className="p-3 whitespace-nowrap text-xs">{fmt(c.endedAt || c.byeAt || c.updatedAt)}</td>
                      <td className="p-3 whitespace-nowrap text-xs">{c.turnCount != null && c.turnCount > 0 ? c.turnCount : '—'}</td>
                      <td className="p-3 whitespace-nowrap text-xs">{hasRec ? <span className="text-primary font-medium">有录音</span> : <span className="text-muted-foreground">—</span>}</td>
                      <td className="p-3 max-w-[200px] align-top"><EllipsisHoverCell text={c.failureReason} lines={2} className="text-xs" /></td>
                      <td className="p-3 text-right whitespace-nowrap">
                        <Button type="outline" size="small" className="text-xs" onClick={() => void openCallDetailDrawer(c.id)}>
                          查看详情
                        </Button>
                      </td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>
        <div className="flex items-center justify-between p-3 border-t border-border text-sm">
          <span className="text-muted-foreground">总计: {callsTotal}</span>
          <div className="flex gap-2">
            <Button type="outline" size="small" disabled={callsPage <= 1} onClick={() => setCallsPage((p) => Math.max(1, p - 1))}>上一页</Button>
            <Button type="outline" size="small" disabled={callsPage * pageSize >= callsTotal} onClick={() => setCallsPage((p) => p + 1)}>下一页</Button>
          </div>
        </div>
      </Card>

      <Drawer
        width={520}
        title={
          <div>
            <div style={{ fontSize: 16, fontWeight: 600 }}>通话详情</div>
            {callDetailDrawerData || calls.find((c) => c.id === callDetailDrawerId) ? (
              <div style={{ marginTop: 4, fontFamily: 'monospace', fontSize: 11, color: 'var(--color-text-3)', wordBreak: 'break-all' }}>
                {(callDetailDrawerData ?? calls.find((c) => c.id === callDetailDrawerId))?.callId}
              </div>
            ) : null}
          </div>
        }
        visible={callDetailDrawerId != null}
        onCancel={closeCallDetailDrawer}
        footer={null}
        closable
      >
        {(() => {
                const d = callDetailDrawerData ?? calls.find((c) => c.id === callDetailDrawerId) ?? null
                if (!d) return <div className="flex flex-1 items-center justify-center p-6">加载中...</div>
                const turns = callDetailDrawerData?.turns
                const recUrlResolved =
                  callDetailDrawerData?.recordingUrl?.trim() && !callDetailDrawerFailed && !callDetailDrawerLoading
                    ? resolveSipRecordingUrl(callDetailDrawerData.recordingUrl)
                    : ''
                return (
                  <div className="space-y-5">
                    <div className="min-h-0 overflow-y-auto space-y-5">
                      <div className="grid grid-cols-2 gap-2 text-xs sm:text-sm">
                        {detailField('ID', d.id)}
                        {detailField('状态', d.state || '—')}
                        {detailField('Dir', d.direction || '—')}
                        {detailField('Codec', d.codec || '—')}
                        {detailField('Payload', d.payloadType != null || d.clockRate ? `${d.payloadType ?? '—'} / ${d.clockRate ?? '—'}` : '—')}
                        {detailField('Dur(s)', d.durationSec ?? '—')}
                        {detailField('结束方式', d.endStatus ? sipAiEndStatusI18nKey(d.endStatus) : '—')}
                        {detailField('对话轮次', d.turnCount != null && d.turnCount > 0 ? d.turnCount : '—')}
                        {detailField('From 号码', d.fromNumber || '—')}
                        {detailField('To 号码', d.toNumber || '—')}
                        {detailField('创建时间', fmt(d.createdAt))}
                        {detailField('INVITE', fmt(d.inviteAt))}
                        {detailField('ACK', fmt(d.ackAt))}
                        {detailField('BYE', fmt(d.byeAt))}
                        {detailField('结束时间', fmt(d.endedAt))}
                        {detailField('BYE 发起方', d.byeInitiator || '—')}
                        {detailField('录音原始字节', d.recordingRawBytes != null && d.recordingRawBytes > 0 ? d.recordingRawBytes : '—')}
                        {detailField('录音 WAV 字节', d.recordingWavBytes != null && d.recordingWavBytes > 0 ? d.recordingWavBytes : '—')}
                        {detailField('转接', [d.hadSipTransfer && 'SIP', d.hadWebSeat && 'WebSeat'].filter(Boolean).join(' · ') || '—')}
                        <div className="col-span-2">{detailField('失败原因', d.failureReason || '—')}</div>
                      </div>

                      <div className="rounded-lg border border-border bg-muted/20 p-3">
                        <p className="mb-2 text-sm font-medium text-foreground">录音</p>
                        {callDetailDrawerLoading ? (
                          <div className="py-8 text-sm text-muted-foreground text-center">录音加载中...</div>
                        ) : callDetailDrawerFailed ? (
                          <div className="flex flex-col items-center gap-3 rounded-md border border-dashed border-destructive/40 bg-destructive/5 px-4 py-8 text-center">
                            <AlertCircle className="h-8 w-8 shrink-0 text-destructive/80" />
                            <p className="text-sm text-foreground">录音加载失败</p>
                            <Button type="outline" size="small" htmlType="button" onClick={() => callDetailDrawerId != null && void openCallDetailDrawer(callDetailDrawerId)}>
                              重试
                            </Button>
                          </div>
                        ) : !callDetailDrawerData?.recordingUrl?.trim() ? (
                          <div className="flex flex-col items-center gap-2 rounded-md border border-dashed border-border bg-background/60 px-4 py-8 text-center">
                            <MicOff className="h-7 w-7 shrink-0 text-muted-foreground" />
                            <p className="text-sm text-muted-foreground">暂无录音</p>
                          </div>
                        ) : (
                          <>
                            <CallAudioPlayer
                              callId={d.callId || `sip-call-${d.id}`}
                              audioUrl={recUrlResolved}
                              hasAudio
                              durationSeconds={callDetailDrawerData?.durationSec != null && callDetailDrawerData.durationSec > 0 ? callDetailDrawerData.durationSec : null}
                            />
                            <a href={recUrlResolved} target="_blank" rel="noreferrer" className="mt-2 inline-block text-xs text-primary underline">
                              打开录音链接
                            </a>
                          </>
                        )}
                      </div>

                      <div>
                        <p className="mb-2 text-sm font-medium">AI 对话详情</p>
                        {callDetailDrawerLoading ? (
                          <p className="text-xs text-muted-foreground">对话加载中...</p>
                        ) : callDetailDrawerFailed ? (
                          <p className="text-xs text-destructive">对话加载失败</p>
                        ) : !turns || turns.length === 0 ? (
                          <p className="text-xs text-muted-foreground">—</p>
                        ) : (
                          <ul className="space-y-3 rounded-md border border-border bg-background/80 p-3">
                            {turns.map((turn, i) => {
                              const meta = formatTurnMeta(turn)
                              const timings = formatTurnTimings(turn)
                              const providers = formatTurnProviders(turn)
                              return (
                              <li key={i} className="space-y-1 border-l-2 border-primary/40 pl-3 text-sm">
                                <div><span className="text-xs text-muted-foreground">用户 </span>{turn.asrText || '—'}</div>
                                <div><span className="text-xs text-muted-foreground">AI </span>{turn.llmText || '—'}</div>
                                {meta ? <div className="text-[11px] text-muted-foreground">{meta}</div> : null}
                                {timings ? <div className="text-[11px] text-muted-foreground">{timings}</div> : null}
                                {providers ? <div className="text-[11px] text-muted-foreground">{providers}</div> : null}
                                {turn.at ? <div className="text-[11px] text-muted-foreground">{fmt(turn.at)}</div> : null}
                              </li>
                              )
                            })}
                          </ul>
                        )}
                      </div>
                    </div>
                  </div>
                )
              })()}
      </Drawer>
    </BaseLayout>
  )
}

export default CallRecords

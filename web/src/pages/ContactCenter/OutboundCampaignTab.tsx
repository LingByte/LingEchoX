import { useEffect, useMemo, useState } from 'react'
import Button from '@/components/UI/Button'
import Modal from '@/components/UI/Modal'
import { showAlert } from '@/utils/notification'
import {
  createOutboundCampaign,
  deleteOutboundCampaign,
  enqueueOutboundCampaignContacts,
  listOutboundCampaignContacts,
  resetOutboundCampaignSuppressedContacts,
  getOutboundCampaignLogs,
  getOutboundCampaignMetrics,
  listOutboundCampaigns,
  listSIPScriptTemplates,
  pauseOutboundCampaign,
  resumeOutboundCampaign,
  startOutboundCampaign,
  stopOutboundCampaign,
  type OutboundCampaignRow,
  type OutboundCampaignLogRow,
  type OutboundCampaignContactRow,
  type OutboundCampaignMetrics,
  type SIPScriptTemplateRow,
} from '@/api/sipContactCenter'

function normCampaignStatus(s?: string): string {
  return String(s || '').trim().toLowerCase()
}

function outboundCampaignActionFlags(status?: string) {
  const raw = normCampaignStatus(status)
  const s = ['running', 'paused', 'draft', 'done'].includes(raw) ? raw : raw === '' ? 'draft' : 'draft'
  const running = s === 'running'
  const paused = s === 'paused'
  const draft = s === 'draft'
  const done = s === 'done'
  return {
    canStartOrResume: draft || paused,
    canPause: running,
    canStop: !done && (running || paused || draft),
    canDelete: !running,
  }
}

export default function OutboundCampaignTab() {
  const [campaigns, setCampaigns] = useState<OutboundCampaignRow[]>([])
  const [campaignsTotal, setCampaignsTotal] = useState(0)
  const [campaignsPage, setCampaignsPage] = useState(1)
  const [campaignsLoading, setCampaignsLoading] = useState(false)
  const [creating, setCreating] = useState(false)
  const [opBusyId, setOpBusyId] = useState<number | null>(null)
  const [deletingId, setDeletingId] = useState<number | null>(null)
  const [submittingContacts, setSubmittingContacts] = useState(false)
  const [resetSuppressedBusy, setResetSuppressedBusy] = useState(false)
  const [contactsLoading, setContactsLoading] = useState(false)
  const [contactsRows, setContactsRows] = useState<OutboundCampaignContactRow[]>([])
  const [metricsLoading, setMetricsLoading] = useState(false)
  const [logsLoading, setLogsLoading] = useState(false)
  const [metrics, setMetrics] = useState<OutboundCampaignMetrics | null>(null)
  const [logs, setLogs] = useState<OutboundCampaignLogRow[]>([])
  const [scripts, setScripts] = useState<SIPScriptTemplateRow[]>([])
  const [selectedScriptId, setSelectedScriptId] = useState('')
  const [createModalOpen, setCreateModalOpen] = useState(false)
  const [detailModalOpen, setDetailModalOpen] = useState(false)
  const [detailCampaignId, setDetailCampaignId] = useState<number | null>(null)
  const [name, setName] = useState('')
  const [contactsText, setContactsText] = useState('1001\n1002')
  const pageSize = 10

  const detailCampaign = useMemo(() => campaigns.find((c) => c.id === detailCampaignId) || null, [campaigns, detailCampaignId])
  const detailActionFlags = useMemo(() => (detailCampaign ? outboundCampaignActionFlags(detailCampaign.status) : null), [detailCampaign])
  const detailIsPaused = detailCampaign ? normCampaignStatus(detailCampaign.status) === 'paused' : false
  const queueView = useMemo(() => {
    const waiting = contactsRows.filter((row) => ['ready', 'retrying'].includes(String(row.status || '').toLowerCase())).slice().sort((a, b) => {
      const ta = a.nextRunAt ? new Date(a.nextRunAt).getTime() : Number.MAX_SAFE_INTEGER
      const tb = b.nextRunAt ? new Date(b.nextRunAt).getTime() : Number.MAX_SAFE_INTEGER
      if (ta !== tb) return ta - tb
      return a.id - b.id
    })
    const positionById = new Map<number, number>()
    waiting.forEach((row, idx) => positionById.set(row.id, idx + 1))
    return { total: contactsRows.length, waiting: waiting.length, dialing: contactsRows.filter((r) => String(r.status || '').toLowerCase() === 'dialing').length, active: contactsRows.filter((r) => ['dialing', 'retrying', 'ready'].includes(String(r.status || '').toLowerCase())).length, positionById }
  }, [contactsRows])

  useEffect(() => {
    void (async () => {
      try {
        const res = await listSIPScriptTemplates(1, 200)
        if (res.code === 200 && res.data?.list) setScripts(res.data.list.filter((x) => x.enabled))
      } catch { setScripts([]) }
    })()
  }, [])

  const loadCampaigns = async () => {
    setCampaignsLoading(true)
    try {
      const res = await listOutboundCampaigns(campaignsPage, pageSize)
      if (res.code === 200 && res.data) {
        setCampaigns(res.data.list || [])
        setCampaignsTotal(res.data.total || 0)
      }
    } catch (e: any) {
      showAlert(e?.msg || '加载失败', 'error')
    } finally { setCampaignsLoading(false) }
  }
  useEffect(() => { void loadCampaigns() }, [campaignsPage])

  const resetCreateForm = () => {
    setName('')
    setSelectedScriptId('')
  }

  const createCampaign = async () => {
    if (!name.trim()) return showAlert('任务名称不能为空', 'error')
    setCreating(true)
    try {
      const selected = scripts.find((s) => String(s.id) === selectedScriptId)
      const scriptSpec = selected?.scriptSpec != null ? (typeof selected.scriptSpec === 'string' ? selected.scriptSpec : JSON.stringify(selected.scriptSpec)) : JSON.stringify({ id: 'followup-v1', version: '2026-04-06', start_id: 'begin', steps: [{ id: 'begin', type: 'say', prompt: '你好，这里是云联络中心回访。', next_id: 'end' }, { id: 'end', type: 'end' }] })
      const res = await createOutboundCampaign({
        name: name.trim(), scenario: 'campaign', media_profile: 'script', script_id: selected?.scriptId || 'followup-v1', script_version: '', script_spec: scriptSpec,
      })
      if (res.code === 200 && res.data?.id) {
        showAlert('创建成功', 'success')
        setCreateModalOpen(false); resetCreateForm(); await loadCampaigns()
      } else showAlert(res.msg || '创建失败', 'error')
    } catch (e: any) {
      showAlert(e?.msg || '创建失败', 'error')
    } finally { setCreating(false) }
  }

  const submitContacts = async () => {
    if (!detailCampaignId) return showAlert('请先选择任务', 'error')
    const contacts = contactsText.split('\n').map((x) => x.trim()).filter(Boolean).map((phone, idx) => ({ phone, priority: Math.max(1, 10 - idx) }))
    if (!contacts.length) return showAlert('请输入联系人号码', 'error')
    setSubmittingContacts(true)
    try {
      const res = await enqueueOutboundCampaignContacts(detailCampaignId, contacts)
      if (res.code === 200) {
        showAlert(`已导入 ${res.data?.accepted || 0} 个联系人`, 'success')
        void refreshLogs(true); void loadContacts(true)
      } else showAlert(res.msg || '导入失败', 'error')
    } catch (e: any) {
      showAlert(e?.msg || '导入失败', 'error')
    } finally { setSubmittingContacts(false) }
  }

  const doCampaignOp = async (campaignId: number, op: 'start' | 'pause' | 'resume' | 'stop') => {
    setOpBusyId(campaignId)
    try {
      const res = op === 'start' ? await startOutboundCampaign(campaignId) : op === 'pause' ? await pauseOutboundCampaign(campaignId) : op === 'stop' ? await stopOutboundCampaign(campaignId) : await resumeOutboundCampaign(campaignId)
      if (res.code === 200) showAlert('操作成功', 'success')
      else showAlert(res.msg || '操作失败', 'error')
      await loadCampaigns()
      if (detailCampaignId === campaignId) void refreshLogs(true)
    } catch (e: any) {
      showAlert(e?.msg || '操作失败', 'error')
    } finally { setOpBusyId(null) }
  }

  const removeCampaign = async (campaign: OutboundCampaignRow) => {
    if (normCampaignStatus(campaign.status) === 'running') return showAlert('运行中的任务不可删除', 'error')
    setDeletingId(campaign.id)
    try {
      const res = await deleteOutboundCampaign(campaign.id)
      if (res.code === 200) {
        showAlert('删除成功', 'success')
        if (detailCampaignId === campaign.id) { setDetailModalOpen(false); setDetailCampaignId(null) }
        await loadCampaigns()
      } else showAlert(res.msg || '删除失败', 'error')
    } catch (e: any) {
      showAlert(e?.msg || '删除失败', 'error')
    } finally { setDeletingId(null) }
  }

  const refreshMetrics = async () => {
    setMetricsLoading(true)
    try {
      const res = await getOutboundCampaignMetrics()
      if (res.code === 200 && res.data) setMetrics(res.data)
      else showAlert(res.msg || '加载失败', 'error')
    } catch (e: any) { showAlert(e?.msg || '加载失败', 'error') } finally { setMetricsLoading(false) }
  }

  const refreshLogs = async (silent = false) => {
    if (!detailCampaignId) { setLogs([]); return }
    if (!silent) setLogsLoading(true)
    try {
      const res = await getOutboundCampaignLogs(detailCampaignId, 120)
      if (res.code === 200 && res.data?.list) setLogs(res.data.list)
      else if (!silent) showAlert(res.msg || '加载失败', 'error')
    } catch (e: any) { if (!silent) showAlert(e?.msg || '加载失败', 'error') } finally { if (!silent) setLogsLoading(false) }
  }
  const loadContacts = async (silent = false) => {
    if (!detailCampaignId) { setContactsRows([]); return }
    if (!silent) setContactsLoading(true)
    try {
      const res = await listOutboundCampaignContacts(detailCampaignId, 1, 500)
      if (res.code === 200 && res.data?.list) setContactsRows(res.data.list)
      else if (!silent) showAlert(res.msg || '加载失败', 'error')
    } catch (e: any) { if (!silent) showAlert(e?.msg || '加载失败', 'error') } finally { if (!silent) setContactsLoading(false) }
  }
  const resetSuppressedContacts = async () => {
    if (!detailCampaignId) return
    setResetSuppressedBusy(true)
    try {
      const res = await resetOutboundCampaignSuppressedContacts(detailCampaignId)
      if (res.code === 200) {
        showAlert(`已重置 ${res.data?.updated || 0} 条 suppressed 联系人`, 'success')
        await loadContacts(true); await refreshLogs(true)
      } else showAlert(res.msg || '操作失败', 'error')
    } catch (e: any) { showAlert(e?.msg || '操作失败', 'error') } finally { setResetSuppressedBusy(false) }
  }
  useEffect(() => { if (!detailModalOpen) return; void refreshLogs(); void loadContacts() }, [detailCampaignId, detailModalOpen])
  useEffect(() => {
    if (!detailCampaignId || !detailModalOpen) return
    const timer = window.setInterval(() => void refreshLogs(true), 3000)
    return () => window.clearInterval(timer)
  }, [detailCampaignId, detailModalOpen])

  const campaignStatusLabel = (raw?: string) => {
    const s = normCampaignStatus(raw)
    if (!s) return '—'
    const map: Record<string, string> = { draft: '草稿', running: '运行中', paused: '已暂停', done: '已完成' }
    return map[s] || raw || s
  }

  return (
    <div className="mt-4 space-y-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground leading-relaxed rounded-lg border border-border bg-muted/30 px-3 py-2.5 flex-1">外呼任务支持创建、导入联系人、启动/暂停/继续/停止、日志与指标查看。</p>
        <Button onClick={() => { resetCreateForm(); setCreateModalOpen(true) }}>新建任务</Button>
      </div>
      <p className="text-xs text-foreground/90 leading-relaxed rounded-lg border border-border bg-primary/5 px-3 py-2.5">暂停可继续，停止后不可继续，只能重建任务。</p>
      <div className="rounded-lg border border-border bg-card p-3 space-y-3">
        <div className="flex items-center justify-between gap-2"><h3 className="text-sm font-semibold">任务列表</h3><Button size="sm" variant="outline" onClick={() => void loadCampaigns()}>刷新</Button></div>
        {campaignsLoading ? <div className="p-4 text-sm text-muted-foreground">加载中...</div> : (
          <div className="max-h-[520px] overflow-auto rounded border border-border">
            <table className="w-full text-xs">
              <thead className="bg-muted/50"><tr><th className="text-left p-2">ID</th><th className="text-left p-2">任务名称</th><th className="text-left p-2">脚本ID</th><th className="text-left p-2">状态</th><th className="text-left p-2">更新时间</th><th className="text-left p-2">操作</th></tr></thead>
              <tbody>
                {campaigns.map((c) => {
                  const flags = outboundCampaignActionFlags(c.status)
                  const busy = opBusyId === c.id
                  const isPaused = normCampaignStatus(c.status) === 'paused'
                  return (
                    <tr key={c.id} className="border-t">
                      <td className="p-2">{c.id}</td><td className="p-2">{c.name}</td><td className="p-2 font-mono">{c.scriptId || '—'}</td><td className="p-2">{campaignStatusLabel(c.status)}</td><td className="p-2">{c.updatedAt ? new Date(c.updatedAt).toLocaleString() : '—'}</td>
                      <td className="p-2"><div className="flex flex-wrap gap-1"><Button size="sm" variant="outline" disabled={busy} onClick={() => { setDetailCampaignId(c.id); setDetailModalOpen(true) }}>详情</Button><Button size="sm" disabled={busy || !flags.canStartOrResume} onClick={() => void doCampaignOp(c.id, isPaused ? 'resume' : 'start')}>{isPaused ? '继续' : '启动'}</Button><Button size="sm" variant="outline" disabled={busy || !flags.canPause} onClick={() => void doCampaignOp(c.id, 'pause')}>暂停</Button><Button size="sm" variant="outline" disabled={busy || !flags.canStop} onClick={() => void doCampaignOp(c.id, 'stop')}>停止</Button><Button size="sm" variant="outline" disabled={deletingId === c.id || !flags.canDelete} onClick={() => void removeCampaign(c)}>删除</Button></div></td>
                    </tr>
                  )
                })}
                {campaigns.length === 0 && <tr><td colSpan={6} className="p-3 text-center text-muted-foreground">暂无数据</td></tr>}
              </tbody>
            </table>
          </div>
        )}
        <div className="flex gap-2"><Button variant="outline" size="sm" disabled={campaignsPage <= 1} onClick={() => setCampaignsPage((p) => Math.max(1, p - 1))}>上一页</Button><Button variant="outline" size="sm" disabled={campaignsPage * pageSize >= campaignsTotal} onClick={() => setCampaignsPage((p) => p + 1)}>下一页</Button></div>
      </div>

      <Modal isOpen={createModalOpen} onClose={() => setCreateModalOpen(false)} title="新建外呼任务" size="lg">
        <div className="space-y-3">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div className="space-y-2 md:col-span-2"><label className="text-xs text-muted-foreground">脚本模板</label><select className="border border-border rounded-md px-3 py-2 bg-background w-full text-sm" value={selectedScriptId} onChange={(e) => setSelectedScriptId(e.target.value)}><option value="">无</option>{scripts.map((s) => <option key={s.id} value={String(s.id)}>{s.name} ({s.scriptId})</option>)}</select></div>
            <div className="space-y-2 md:col-span-2"><label className="text-xs text-muted-foreground">任务名称</label><input className="border border-border rounded-md px-3 py-2 bg-background w-full" value={name} onChange={(e) => setName(e.target.value)} /></div>
          </div>
          <div className="flex justify-end gap-2"><Button variant="outline" onClick={() => setCreateModalOpen(false)} disabled={creating}>取消</Button><Button onClick={() => void createCampaign()} disabled={creating}>{creating ? '创建中...' : '创建'}</Button></div>
        </div>
      </Modal>

      <Modal isOpen={detailModalOpen} onClose={() => setDetailModalOpen(false)} title={detailCampaign ? `任务详情 #${detailCampaign.id} · ${detailCampaign.name} · ${campaignStatusLabel(detailCampaign.status)}` : '任务详情'} size="xl">
        <div className="space-y-3">
          <p className="text-xs text-foreground/90 leading-relaxed rounded-md border border-border bg-muted/20 px-3 py-2">暂停可继续，停止后不可继续，只能重建任务。</p>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" onClick={() => detailCampaignId && void doCampaignOp(detailCampaignId, detailIsPaused ? 'resume' : 'start')} disabled={!detailCampaignId || opBusyId === detailCampaignId || !detailActionFlags?.canStartOrResume}>{detailIsPaused ? '继续' : '启动'}</Button>
            <Button size="sm" variant="outline" onClick={() => detailCampaignId && void doCampaignOp(detailCampaignId, 'pause')} disabled={!detailCampaignId || opBusyId === detailCampaignId || !detailActionFlags?.canPause}>暂停</Button>
            <Button size="sm" variant="outline" onClick={() => detailCampaignId && void doCampaignOp(detailCampaignId, 'stop')} disabled={!detailCampaignId || opBusyId === detailCampaignId || !detailActionFlags?.canStop}>停止</Button>
          </div>
          <div className="space-y-2"><label className="text-xs text-muted-foreground">联系人（每行一个号码）</label><textarea className="border border-border rounded-md px-3 py-2 bg-background w-full h-24 font-mono text-xs" value={contactsText} onChange={(e) => setContactsText(e.target.value)} /><Button size="sm" variant="outline" onClick={() => void submitContacts()} disabled={submittingContacts || !detailCampaignId}>{submittingContacts ? '导入中...' : '导入联系人'}</Button><Button size="sm" variant="outline" onClick={() => void resetSuppressedContacts()} disabled={!detailCampaignId || resetSuppressedBusy}>{resetSuppressedBusy ? '处理中...' : '重置被抑制号码'}</Button></div>
          <div className="rounded-lg border border-border bg-card p-3 space-y-2">
            <div className="flex items-center justify-between"><h3 className="text-sm font-semibold">已导入联系人</h3><Button size="sm" variant="outline" onClick={() => void loadContacts()} disabled={!detailCampaignId || contactsLoading}>{contactsLoading ? '加载中...' : '刷新'}</Button></div>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-2 text-xs"><div className="rounded border border-border p-2">总联系人: {queueView.total}</div><div className="rounded border border-border p-2">队列中: {queueView.waiting}</div><div className="rounded border border-border p-2">拨号中: {queueView.dialing}</div><div className="rounded border border-border p-2">活跃任务: {queueView.active}</div></div>
            <div className="max-h-52 overflow-auto rounded border border-border"><table className="w-full text-xs"><thead className="bg-muted/50"><tr><th className="text-left p-2">号码</th><th className="text-left p-2">队列位置</th><th className="text-left p-2">状态</th><th className="text-left p-2">尝试</th><th className="text-left p-2">失败原因</th><th className="text-left p-2">下次重试</th></tr></thead><tbody>{contactsRows.map((row) => <tr key={row.id} className="border-t"><td className="p-2 font-mono">{row.phone}</td><td className="p-2">{queueView.positionById.get(row.id) || '—'}</td><td className="p-2">{row.status || '—'}</td><td className="p-2">{`${row.attemptCount ?? 0}/${row.maxAttempts ?? 0}`}</td><td className="p-2">{row.failureReason || '—'}</td><td className="p-2">{row.nextRunAt ? new Date(row.nextRunAt).toLocaleString() : '—'}</td></tr>)}{contactsRows.length === 0 && <tr><td colSpan={6} className="p-3 text-center text-muted-foreground">暂无联系人</td></tr>}</tbody></table></div>
          </div>
          <div className="rounded-lg border border-border bg-card p-3 space-y-2">
            <div className="flex items-center justify-between"><h3 className="text-sm font-semibold">全局指标</h3><Button size="sm" variant="outline" onClick={() => void refreshMetrics()} disabled={metricsLoading}>{metricsLoading ? '加载中...' : '刷新'}</Button></div>
            <div className="grid grid-cols-2 md:grid-cols-5 gap-2 text-xs"><div className="rounded border border-border p-2">invited: {metrics?.invited_total ?? 0}</div><div className="rounded border border-border p-2">answered: {metrics?.answered_total ?? 0}</div><div className="rounded border border-border p-2">failed: {metrics?.failed_total ?? 0}</div><div className="rounded border border-border p-2">retrying: {metrics?.retrying_total ?? 0}</div><div className="rounded border border-border p-2">suppressed: {metrics?.suppressed_total ?? 0}</div></div>
          </div>
          <div className="rounded-lg border border-border bg-card p-3 space-y-2">
            <div className="flex items-center justify-between"><h3 className="text-sm font-semibold">执行日志终端</h3><Button size="sm" variant="outline" onClick={() => void refreshLogs()} disabled={!detailCampaignId || logsLoading}>{logsLoading ? '加载中...' : '刷新'}</Button></div>
            <div className="rounded border border-border bg-black text-green-300 text-xs font-mono p-2 h-64 overflow-auto">{!detailCampaignId && <div className="text-zinc-400">请选择任务后查看日志</div>}{detailCampaignId && logs.length === 0 && <div className="text-zinc-400">暂无执行日志</div>}{logs.map((row) => <div key={`${row.type}-${row.id}-${row.at}`} className="leading-5 break-all"><span className="text-zinc-400">[{new Date(row.at).toLocaleString()}]</span>{' '}<span className={row.level === 'error' ? 'text-red-300' : 'text-cyan-300'}>{row.type.toUpperCase()}</span>{' '}{row.phone ? <span className="text-yellow-200">phone={row.phone} </span> : null}{row.callId ? <span className="text-yellow-200">call={row.callId} </span> : null}<span>{row.message}</span></div>)}</div>
          </div>
        </div>
      </Modal>
    </div>
  )
}

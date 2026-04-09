import { useCallback, useEffect, useState } from 'react'
import { Trash2 } from 'lucide-react'
import Button from '@/components/UI/Button'
import Badge from '@/components/UI/Badge'
import { showAlert } from '@/utils/notification'
import { ACD_ROUTE_TYPES, ACD_SIP_SOURCES, ACD_WORK_STATES, createACDPoolTarget, deleteACDPoolTarget, fetchSIPUsersForSelect, listACDPoolTargets, updateACDPoolTarget, type ACDPoolTargetRow, type ACDSipSource, type SIPUserRow } from '@/api/sipContactCenter'
import ConfirmDialog from '@/components/UI/ConfirmDialog'

function acdTrunkGatewayCell(r: ACDPoolTargetRow): string {
  if (r.routeType !== 'sip' || (r.sipSource || '').toLowerCase() !== 'trunk') return '—'
  const h = (r.sipTrunkHost || '').trim()
  if (!h) return '—'
  const p = r.sipTrunkPort != null && r.sipTrunkPort > 0 ? r.sipTrunkPort : 5060
  const base = `${h}:${p}`
  const sig = (r.sipTrunkSignalingAddr || '').trim()
  return sig ? `${base} → ${sig}` : base
}
function acdCallerCell(r: ACDPoolTargetRow): string {
  if (r.routeType !== 'sip') return '—'
  const id = (r.sipCallerId || '').trim()
  if (!id) return '—'
  const d = (r.sipCallerDisplayName || '').trim()
  return d ? `${id} · ${d}` : id
}
type FormState = { name: string; routeType: string; sipSource: ACDSipSource; targetValue: string; sipTrunkHost: string; sipTrunkPort: number; sipTrunkSignalingAddr: string; sipCallerId: string; sipCallerDisplayName: string; weight: number; workState: string }
const defaultForm = (): FormState => ({ name: '', routeType: 'sip', sipSource: 'internal', targetValue: '', sipTrunkHost: '', sipTrunkPort: 5060, sipTrunkSignalingAddr: '', sipCallerId: '', sipCallerDisplayName: '', weight: 10, workState: 'offline' })

export default function ACDPoolTab({ active, refreshNonce = 0 }: { active: boolean; refreshNonce?: number }) {
  const [rows, setRows] = useState<ACDPoolTargetRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [routeTypeFilter, setRouteTypeFilter] = useState('')
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormState>(defaultForm)
  const [saving, setSaving] = useState(false)
  const [sipUsersPick, setSipUsersPick] = useState<SIPUserRow[]>([])
  const [acdDeleteOpen, setAcdDeleteOpen] = useState(false)
  const [acdDeleteId, setAcdDeleteId] = useState<number | null>(null)
  const pageSize = 20
  const load = useCallback(async () => {
    if (!active) return
    setLoading(true)
    try {
      const res = await listACDPoolTargets(page, pageSize, { routeType: routeTypeFilter.trim() || undefined })
      if (res.code === 200 && res.data) { setRows(res.data.list || []); setTotal(res.data.total || 0) }
    } catch (e: any) {
      showAlert(e?.msg || '加载失败', 'error')
    } finally { setLoading(false) }
  }, [active, page, routeTypeFilter])
  useEffect(() => { void load() }, [load, refreshNonce])
  useEffect(() => {
    if (!modalOpen || !active) return
    if (editingId != null && form.routeType !== 'sip') return
    let cancelled = false
    void (async () => { try { const list = await fetchSIPUsersForSelect(500); if (!cancelled) setSipUsersPick(list) } catch { if (!cancelled) setSipUsersPick([]) } })()
    return () => { cancelled = true }
  }, [modalOpen, active, editingId, form.routeType])
  const openCreate = () => { setEditingId(null); setForm(defaultForm()); setModalOpen(true) }
  const openEdit = (r: ACDPoolTargetRow) => {
    const src = (r.sipSource || '').toLowerCase() === 'trunk' ? 'trunk' : 'internal'
    setEditingId(r.id)
    setForm({ name: r.name || '', routeType: r.routeType || 'sip', sipSource: r.routeType === 'web' ? 'internal' : src, targetValue: r.targetValue || '', sipTrunkHost: r.sipTrunkHost || '', sipTrunkPort: r.sipTrunkPort != null && r.sipTrunkPort > 0 ? r.sipTrunkPort : 5060, sipTrunkSignalingAddr: r.sipTrunkSignalingAddr || '', sipCallerId: r.sipCallerId || '', sipCallerDisplayName: r.sipCallerDisplayName || '', weight: r.weight ?? 0, workState: r.workState || 'offline' })
    setModalOpen(true)
  }
  const closeModal = () => { setModalOpen(false); setEditingId(null) }
  const save = async () => {
    setSaving(true)
    try {
      const routeType = editingId == null ? 'sip' : form.routeType
      const tv = routeType === 'sip' ? form.targetValue.trim() : ''
      if (routeType === 'sip' && !tv) return showAlert('SIP 目标不能为空', 'error')
      if (routeType === 'sip' && form.sipSource === 'trunk' && !form.sipTrunkHost.trim()) return showAlert('Trunk Host 不能为空', 'error')
      const trunkPort = Number(form.sipTrunkPort) || 5060
      const body = { name: form.name.trim(), routeType, sipSource: routeType === 'sip' ? form.sipSource : '', targetValue: tv, sipTrunkHost: routeType === 'sip' && form.sipSource === 'trunk' ? form.sipTrunkHost.trim() : '', sipTrunkPort: routeType === 'sip' && form.sipSource === 'trunk' ? trunkPort : 0, sipTrunkSignalingAddr: routeType === 'sip' && form.sipSource === 'trunk' ? form.sipTrunkSignalingAddr.trim() : '', sipCallerId: routeType === 'sip' ? form.sipCallerId.trim() : '', sipCallerDisplayName: routeType === 'sip' ? form.sipCallerDisplayName.trim() : '', weight: Number(form.weight) || 0, workState: form.workState }
      const res = editingId == null ? await createACDPoolTarget(body) : await updateACDPoolTarget(editingId, body)
      if (res.code === 200) { showAlert('保存成功', 'success'); closeModal(); void load() } else showAlert(res.msg || '保存失败', 'error')
    } catch (e: any) {
      showAlert(e?.msg || '保存失败', 'error')
    } finally { setSaving(false) }
  }
  const confirmAcdDelete = async () => {
    if (acdDeleteId == null) return
    const res = await deleteACDPoolTarget(acdDeleteId)
    if (res.code !== 200) throw new Error(res.msg || '删除失败')
    showAlert('删除成功', 'success')
    void load()
  }
  const workStateLabel = (s: string) => ({ offline: '离线', available: '可用', ringing: '振铃中', busy: '忙碌', acw: '话后整理', break: '休息' } as Record<string, string>)[s] || s
  return (
    <div className="mt-4 space-y-3">
      <p className="text-xs text-muted-foreground leading-relaxed rounded-lg border border-border bg-primary/5 px-3 py-2.5">号码池用于来电分配和转接选线，SIP 注册与 ACD 选线独立。</p>
      <div className="flex flex-wrap gap-2 items-end">
        <div className="flex flex-col gap-1">
          <label className="text-xs text-muted-foreground">路由类型</label>
          <select className="border border-border rounded-md px-3 py-1.5 text-sm bg-background w-28" value={routeTypeFilter} onChange={(e) => setRouteTypeFilter(e.target.value)}>
            <option value="">全部</option>{ACD_ROUTE_TYPES.map((rt) => <option key={rt} value={rt}>{rt}</option>)}
          </select>
        </div>
        <Button size="sm" onClick={() => { setPage(1); void load() }}>搜索</Button>
        <Button size="sm" variant="outline" onClick={openCreate}>新增 SIP 目标</Button>
      </div>
      {loading ? <div className="p-4 text-sm text-muted-foreground">加载中...</div> : (
        <div className="overflow-x-auto rounded-lg border border-border bg-card">
          <table className="min-w-[1240px] w-full text-sm">
            <thead className="bg-muted/50">
              <tr><th className="text-left p-3 whitespace-nowrap">ID</th><th className="text-left p-3 whitespace-nowrap">名称</th><th className="text-left p-3 whitespace-nowrap">路由类型</th><th className="text-left p-3 whitespace-nowrap min-w-[100px]">SIP 来源</th><th className="text-left p-3 min-w-[140px]">目标</th><th className="text-left p-3 min-w-[160px]">Trunk 网关</th><th className="text-left p-3 min-w-[120px]">外呼主叫</th><th className="text-left p-3 whitespace-nowrap">权重</th><th className="text-left p-3 min-w-[200px]">状态</th><th className="text-left p-3 whitespace-nowrap text-xs">状态时间</th><th className="text-right p-3 whitespace-nowrap">操作</th></tr>
            </thead>
            <tbody>
              {rows.length === 0 ? <tr><td colSpan={11} className="p-6 text-center text-muted-foreground">暂无数据</td></tr> : rows.map((r) => (
                <tr key={r.id} className="border-t border-border align-top">
                  <td className="p-3 whitespace-nowrap">{r.id}</td><td className="p-3 max-w-[160px] truncate">{r.name || '—'}</td><td className="p-3 whitespace-nowrap">{r.routeType}</td>
                  <td className="p-3 whitespace-nowrap text-xs">{r.routeType === 'sip' ? ((r.sipSource || '').toLowerCase() === 'trunk' ? <Badge variant="outline" size="xs" shape="pill">trunk</Badge> : <Badge variant="secondary" size="xs" shape="pill">internal</Badge>) : '—'}</td>
                  <td className="p-3 font-mono text-xs max-w-[200px] break-all text-muted-foreground">{r.routeType === 'sip' ? r.targetValue || '—' : '—'}</td><td className="p-3 font-mono text-xs max-w-[220px] break-all text-muted-foreground">{acdTrunkGatewayCell(r)}</td><td className="p-3 font-mono text-xs max-w-[180px] break-all text-muted-foreground">{acdCallerCell(r)}</td><td className="p-3 whitespace-nowrap">{r.weight}</td>
                  <td className="p-3 align-top"><div className="flex flex-wrap gap-1.5"><Badge variant="outline" size="xs" shape="pill">{workStateLabel(r.workState)}</Badge></div></td>
                  <td className="p-3 whitespace-nowrap text-xs text-muted-foreground">{r.workStateAt ? new Date(r.workStateAt).toLocaleString() : '—'}</td>
                  <td className="p-3 text-right"><div className="flex flex-wrap items-center justify-end gap-1"><Button variant="outline" size="sm" className="text-xs" onClick={() => openEdit(r)}>编辑</Button><Button variant="outline" size="sm" className="text-xs text-destructive border-destructive/40 hover:bg-destructive/10" onClick={() => { setAcdDeleteId(r.id); setAcdDeleteOpen(true) }}><Trash2 className="h-3.5 w-3.5 sm:mr-1" /><span className="hidden sm:inline">删除</span></Button></div></td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="flex items-center justify-between p-3 border-t border-border text-sm"><span className="text-muted-foreground">总计: {total}</span><div className="flex gap-2"><Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>上一页</Button><Button variant="outline" size="sm" disabled={page * pageSize >= total} onClick={() => setPage((p) => p + 1)}>下一页</Button></div></div>
        </div>
      )}
      {modalOpen && (
        <div className="fixed inset-0 z-[110] flex items-center justify-center p-4">
          <button type="button" className="absolute inset-0 bg-black/50" aria-label="关闭" onClick={closeModal} />
          <div className="relative z-[111] w-full max-w-xl rounded-lg border border-border bg-card p-5 shadow-xl space-y-4 max-h-[90vh] overflow-y-auto">
            <h3 className="text-lg font-semibold">{editingId == null ? '新增 SIP 目标' : '编辑目标'}</h3>
            <div className="space-y-3 text-sm">
              <div className="flex flex-col gap-1"><label className="text-xs text-muted-foreground">名称</label><input className="border border-border rounded-md px-3 py-2 bg-background" value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} /></div>
              {(editingId == null || form.routeType === 'sip') && (
                <>
                  <div className="flex flex-col gap-1"><label className="text-xs text-muted-foreground">SIP 来源</label><select className="border border-border rounded-md px-3 py-2 bg-background text-sm" value={form.sipSource} onChange={(e) => setForm((f) => ({ ...f, sipSource: e.target.value as ACDSipSource, targetValue: '', sipTrunkHost: '', sipTrunkPort: 5060, sipTrunkSignalingAddr: '' }))}>{ACD_SIP_SOURCES.map((s) => <option key={s} value={s}>{s}</option>)}</select></div>
                  {form.sipSource === 'internal' ? (
                    <div className="flex flex-col gap-1"><label className="text-xs text-muted-foreground">选择已注册 SIP 用户</label><select className="border border-border rounded-md px-3 py-2 bg-background font-mono text-xs" value={form.targetValue} onChange={(e) => setForm((f) => ({ ...f, targetValue: e.target.value }))}><option value="">请选择</option>{sipUsersPick.map((u) => <option key={u.id} value={u.username}>{u.username}@{u.domain}{u.online ? ' · 在线' : ''}</option>)}</select></div>
                  ) : (
                    <>
                      <div className="flex flex-col gap-1"><label className="text-xs text-muted-foreground">Trunk 拨号目标</label><input className="border border-border rounded-md px-3 py-2 bg-background font-mono text-xs" value={form.targetValue} onChange={(e) => setForm((f) => ({ ...f, targetValue: e.target.value }))} /></div>
                      <div className="flex flex-col gap-1"><label className="text-xs text-muted-foreground">Trunk Host</label><input className="border border-border rounded-md px-3 py-2 bg-background font-mono text-xs" value={form.sipTrunkHost} onChange={(e) => setForm((f) => ({ ...f, sipTrunkHost: e.target.value }))} /></div>
                      <div className="flex flex-col gap-1"><label className="text-xs text-muted-foreground">Trunk Port</label><input type="number" min={1} max={65535} className="border border-border rounded-md px-3 py-2 bg-background font-mono text-xs" value={form.sipTrunkPort} onChange={(e) => setForm((f) => ({ ...f, sipTrunkPort: parseInt(e.target.value, 10) || 5060 }))} /></div>
                    </>
                  )}
                </>
              )}
              <div className="flex flex-col gap-1"><label className="text-xs text-muted-foreground">权重</label><input type="number" className="border border-border rounded-md px-3 py-2 bg-background" value={form.weight} onChange={(e) => setForm((f) => ({ ...f, weight: parseInt(e.target.value, 10) || 0 }))} /></div>
              <div className="flex flex-col gap-1"><label className="text-xs text-muted-foreground">工作状态</label><select className="border border-border rounded-md px-3 py-2 bg-background" value={form.workState} onChange={(e) => setForm((f) => ({ ...f, workState: e.target.value }))}>{ACD_WORK_STATES.map((ws) => <option key={ws} value={ws}>{workStateLabel(ws)}</option>)}</select></div>
            </div>
            <div className="flex justify-end gap-2 pt-2"><Button variant="outline" type="button" onClick={closeModal} disabled={saving}>取消</Button><Button type="button" onClick={() => void save()} disabled={saving}>{saving ? '保存中...' : '保存'}</Button></div>
          </div>
        </div>
      )}
      <ConfirmDialog
        isOpen={acdDeleteOpen}
        onClose={() => { setAcdDeleteOpen(false); setAcdDeleteId(null) }}
        onConfirm={confirmAcdDelete}
        title="确认删除号码池目标"
        message="删除后不可恢复，确认继续吗？"
        confirmText="确认删除"
        cancelText="取消"
        variant="danger"
      />
    </div>
  )
}

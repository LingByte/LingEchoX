import { useCallback, useEffect, useState } from 'react'
import { Trash2 } from 'lucide-react'
import Button from '@/components/UI/Button'
import Badge from '@/components/UI/Badge'
import { showAlert } from '@/utils/notification'
import { ACD_ROUTE_TYPES, ACD_WORK_STATES, createACDPoolTarget, deleteACDPoolTarget, listACDPoolTargets, updateACDPoolTarget, type ACDPoolTargetRow } from '@/api/sipContactCenter'
import ConfirmDialog from '@/components/UI/ConfirmDialog'

type FormState = { name: string; routeType: string; targetValue: string; weight: number; workState: string }
const defaultForm = (): FormState => ({ name: '', routeType: 'sip', targetValue: '', weight: 10, workState: 'offline' })

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
  const [acdDeleteOpen, setAcdDeleteOpen] = useState(false)
  const [acdDeleteId, setAcdDeleteId] = useState<number | null>(null)
  const [acdDeleteLoading, setAcdDeleteLoading] = useState(false)
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
  const openCreate = () => { setEditingId(null); setForm(defaultForm()); setModalOpen(true) }
  const openEdit = (r: ACDPoolTargetRow) => {
    setEditingId(r.id)
    setForm({ name: r.name || '', routeType: r.routeType || 'sip', targetValue: r.targetValue || '', weight: r.weight ?? 0, workState: r.workState || 'offline' })
    setModalOpen(true)
  }
  const closeModal = () => { setModalOpen(false); setEditingId(null) }
  const save = async () => {
    setSaving(true)
    try {
      const routeType = editingId == null ? 'sip' : form.routeType
      const tv = routeType === 'sip' ? form.targetValue.trim() : ''
      if (routeType === 'sip' && !tv) return showAlert('SIP 目标不能为空', 'error')
      const body = { name: form.name.trim(), routeType, sipSource: '', targetValue: tv, sipTrunkHost: '', sipTrunkPort: 0, sipTrunkSignalingAddr: '', sipCallerId: '', sipCallerDisplayName: '', weight: Number(form.weight) || 0, workState: form.workState }
      const res = editingId == null ? await createACDPoolTarget(body) : await updateACDPoolTarget(editingId, body)
      if (res.code === 200) { showAlert('保存成功', 'success'); closeModal(); void load() } else showAlert(res.msg || '保存失败', 'error')
    } catch (e: any) {
      showAlert(e?.msg || '保存失败', 'error')
    } finally { setSaving(false) }
  }
  const confirmAcdDelete = async () => {
    if (acdDeleteId == null) return
    setAcdDeleteLoading(true)
    try {
      const res = await deleteACDPoolTarget(acdDeleteId)
      if (res.code !== 200) {
        showAlert(res.msg || '删除失败', 'error')
        return
      }
      showAlert('删除成功', 'success')
      setAcdDeleteOpen(false)
      setAcdDeleteId(null)
      void load()
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '删除失败', 'error')
    } finally {
      setAcdDeleteLoading(false)
    }
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
          <table className="min-w-[760px] w-full text-sm">
            <thead className="bg-muted/50">
              <tr><th className="text-left p-3 whitespace-nowrap">ID</th><th className="text-left p-3 whitespace-nowrap">名称</th><th className="text-left p-3 min-w-[180px]">呼叫号码</th><th className="text-left p-3 whitespace-nowrap">权重</th><th className="text-left p-3 min-w-[200px]">状态</th><th className="text-left p-3 whitespace-nowrap text-xs">状态时间</th><th className="text-right p-3 whitespace-nowrap">操作</th></tr>
            </thead>
            <tbody>
              {rows.length === 0 ? <tr><td colSpan={7} className="p-6 text-center text-muted-foreground">暂无数据</td></tr> : rows.map((r) => (
                <tr key={r.id} className="border-t border-border align-top">
                  <td className="p-3 whitespace-nowrap">{r.id}</td><td className="p-3 max-w-[200px] truncate">{r.name || '—'}</td>
                  <td className="p-3 font-mono text-xs max-w-[260px] break-all text-muted-foreground">{r.routeType === 'sip' ? r.targetValue || '—' : '—'}</td><td className="p-3 whitespace-nowrap">{r.weight}</td>
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
                  <div className="flex flex-col gap-1"><label className="text-xs text-muted-foreground">呼叫电话号</label><input className="border border-border rounded-md px-3 py-2 bg-background font-mono text-sm" placeholder="例如 10086 或 13800138000" value={form.targetValue} onChange={(e) => setForm((f) => ({ ...f, targetValue: e.target.value }))} /></div>
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
        onClose={() => { if (!acdDeleteLoading) { setAcdDeleteOpen(false); setAcdDeleteId(null) } }}
        onConfirm={() => { void confirmAcdDelete() }}
        title="确认删除号码池目标"
        message="删除后不可恢复，确认继续吗？"
        confirmText="确认删除"
        cancelText="取消"
        variant="danger"
        loading={acdDeleteLoading}
      />
    </div>
  )
}

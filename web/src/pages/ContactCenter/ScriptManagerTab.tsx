import { useCallback, useEffect, useState } from 'react'
import Button from '@/components/UI/Button'
import { showAlert } from '@/utils/notification'
import { createSIPScriptTemplate, deleteSIPScriptTemplate, listSIPScriptTemplates, updateSIPScriptTemplate, type SIPScriptTemplateRow } from '@/api/sipContactCenter'
import ConfirmDialog from '@/components/UI/ConfirmDialog'

type FormState = { name: string; description: string; enabled: boolean; scriptSpec: string }
const defaultScriptSpec = `{
  "id": "followup-v1",
  "version": "2026-04-06",
  "start_id": "begin",
  "steps": [
    { "id": "begin", "type": "say", "prompt": "你好，这里是云联络中心回访。", "next_id": "end" },
    { "id": "end", "type": "end" }
  ]
}`
const defaultForm = (): FormState => ({ name: '', description: '', enabled: true, scriptSpec: defaultScriptSpec })

export default function ScriptManagerTab() {
  const [rows, setRows] = useState<SIPScriptTemplateRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<SIPScriptTemplateRow | null>(null)
  const [form, setForm] = useState<FormState>(defaultForm)
  const [scriptDeleteOpen, setScriptDeleteOpen] = useState(false)
  const [scriptDeleteId, setScriptDeleteId] = useState<number | null>(null)
  const pageSize = 20
  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listSIPScriptTemplates(page, pageSize)
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total || 0)
      }
    } catch (e: any) {
      showAlert(e?.msg || '加载失败', 'error')
    } finally {
      setLoading(false)
    }
  }, [page])
  useEffect(() => { void load() }, [load])
  const openCreate = () => { setEditing(null); setForm(defaultForm()); setModalOpen(true) }
  const openEdit = (row: SIPScriptTemplateRow) => {
    setEditing(row)
    setForm({ name: row.name || '', description: row.description || '', enabled: !!row.enabled, scriptSpec: typeof row.scriptSpec === 'string' ? row.scriptSpec : JSON.stringify(row.scriptSpec || {}, null, 2) })
    setModalOpen(true)
  }
  const save = async () => {
    if (!form.name.trim()) return showAlert('脚本名称不能为空', 'error')
    setSaving(true)
    try {
      const body = { name: form.name.trim(), description: form.description.trim(), enabled: form.enabled, scriptSpec: form.scriptSpec.trim() }
      const res = editing ? await updateSIPScriptTemplate(editing.id, body) : await createSIPScriptTemplate(body)
      if (res.code === 200) { showAlert('保存成功', 'success'); setModalOpen(false); setEditing(null); await load() }
      else showAlert(res.msg || '保存失败', 'error')
    } catch (e: any) {
      showAlert(e?.msg || '保存失败', 'error')
    } finally { setSaving(false) }
  }
  const confirmScriptDelete = async () => {
    if (scriptDeleteId == null) return
    const res = await deleteSIPScriptTemplate(scriptDeleteId)
    if (res.code !== 200) throw new Error(res.msg || '删除失败')
    showAlert('删除成功', 'success')
    await load()
  }
  return (
    <div className="mt-4 space-y-3">
      <p className="text-xs text-muted-foreground leading-relaxed rounded-lg border border-border bg-muted/30 px-3 py-2.5">脚本模板用于驱动外呼流程。支持创建、编辑与启停。</p>
      <div className="flex gap-2">
        <Button size="sm" variant="outline" onClick={openCreate}>新建脚本</Button>
        <Button size="sm" onClick={() => void load()}>刷新</Button>
      </div>
      {loading ? <div className="p-4 text-sm text-muted-foreground">加载中...</div> : (
        <div className="overflow-x-auto rounded-lg border border-border bg-card">
          <table className="min-w-[920px] w-full text-sm">
            <thead className="bg-muted/50"><tr><th className="text-left p-3">ID</th><th className="text-left p-3">名称</th><th className="text-left p-3">脚本ID</th><th className="text-left p-3">启用</th><th className="text-left p-3">更新时间</th><th className="text-right p-3">操作</th></tr></thead>
            <tbody>
              {rows.length === 0 ? <tr><td colSpan={6} className="p-6 text-center text-muted-foreground">暂无数据</td></tr> : rows.map((r) => (
                <tr key={r.id} className="border-t border-border">
                  <td className="p-3">{r.id}</td><td className="p-3">{r.name}</td><td className="p-3 font-mono text-xs">{r.scriptId}</td><td className="p-3">{r.enabled ? '已启用' : '已停用'}</td><td className="p-3 text-xs">{r.updatedAt ? new Date(r.updatedAt).toLocaleString() : '—'}</td>
                  <td className="p-3 text-right space-x-2"><Button variant="outline" size="sm" onClick={() => openEdit(r)}>编辑</Button><Button variant="outline" size="sm" onClick={() => { setScriptDeleteId(r.id); setScriptDeleteOpen(true) }}>删除</Button></td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="flex items-center justify-between p-3 border-t border-border text-sm">
            <span className="text-muted-foreground">总计: {total}</span>
            <div className="flex gap-2"><Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>上一页</Button><Button variant="outline" size="sm" disabled={page * pageSize >= total} onClick={() => setPage((p) => p + 1)}>下一页</Button></div>
          </div>
        </div>
      )}
      {modalOpen && (
        <div className="fixed inset-0 z-[120] flex items-center justify-center p-4">
          <button type="button" className="absolute inset-0 bg-black/50" aria-label="关闭" onClick={() => setModalOpen(false)} />
          <div className="relative z-[121] w-full max-w-2xl rounded-lg border border-border bg-card p-5 shadow-xl space-y-3">
            <h3 className="text-lg font-semibold">{editing ? '编辑脚本' : '新建脚本'}</h3>
            <input className="border border-border rounded-md px-3 py-2 bg-background w-full" placeholder="脚本名称" value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} />
            <textarea className="border border-border rounded-md px-3 py-2 bg-background w-full h-20" placeholder="描述" value={form.description} onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))} />
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.enabled} onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))} />启用</label>
            <textarea className="border border-border rounded-md px-3 py-2 bg-background w-full h-64 font-mono text-xs" value={form.scriptSpec} onChange={(e) => setForm((f) => ({ ...f, scriptSpec: e.target.value }))} />
            <div className="flex justify-end gap-2"><Button variant="outline" onClick={() => setModalOpen(false)} disabled={saving}>取消</Button><Button onClick={() => void save()} disabled={saving}>{saving ? '保存中...' : '保存'}</Button></div>
          </div>
        </div>
      )}
      <ConfirmDialog
        isOpen={scriptDeleteOpen}
        onClose={() => { setScriptDeleteOpen(false); setScriptDeleteId(null) }}
        onConfirm={confirmScriptDelete}
        title="确认删除脚本"
        message="删除后不可恢复，确认继续吗？"
        confirmText="确认删除"
        cancelText="取消"
        variant="danger"
      />
    </div>
  )
}

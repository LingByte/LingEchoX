import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Checkbox, Input, Modal, Space, Typography } from '@arco-design/web-react'

const TextArea = Input.TextArea
import { showAlert } from '@/utils/notification'
import { deleteSIPScriptTemplate, listSIPScriptTemplates, updateSIPScriptTemplate, type SIPScriptTemplateRow } from '@/api/sipScripts'
import ScriptSpecEditor from '@/pages/ContactCenter/ScriptSpecEditor'
import { parseHybridScriptDraft } from '@/pages/ContactCenter/scriptSpecTypes'

type FormState = { name: string; description: string; enabled: boolean; scriptSpec: string }

export default function ScriptManagerTab() {
  const navigate = useNavigate()
  const [rows, setRows] = useState<SIPScriptTemplateRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<SIPScriptTemplateRow | null>(null)
  const [form, setForm] = useState<FormState>({
    name: '',
    description: '',
    enabled: true,
    scriptSpec: '{}',
  })
  const [scriptDeleteOpen, setScriptDeleteOpen] = useState(false)
  const [scriptDeleteId, setScriptDeleteId] = useState<number | null>(null)
  const [scriptDeleteLoading, setScriptDeleteLoading] = useState(false)
  const pageSize = 20

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listSIPScriptTemplates(page, pageSize)
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total || 0)
      }
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '加载失败', 'error')
    } finally {
      setLoading(false)
    }
  }, [page])

  useEffect(() => {
    void load()
  }, [load])

  const openEdit = (row: SIPScriptTemplateRow) => {
    setEditing(row)
    setForm({
      name: row.name || '',
      description: row.description || '',
      enabled: !!row.enabled,
      scriptSpec: typeof row.scriptSpec === 'string' ? row.scriptSpec : JSON.stringify(row.scriptSpec || {}, null, 2),
    })
    setModalOpen(true)
  }

  const save = async () => {
    if (!editing) return showAlert('请从列表选择要编辑的脚本', 'error')
    if (!form.name.trim()) return showAlert('脚本名称不能为空', 'error')
    const check = parseHybridScriptDraft(form.scriptSpec.trim())
    if (!check.ok) return showAlert(`脚本内容有误：${check.error}`, 'error')
    setSaving(true)
    try {
      const body = { name: form.name.trim(), description: form.description.trim(), enabled: form.enabled, scriptSpec: form.scriptSpec.trim() }
      const res = await updateSIPScriptTemplate(editing.id, body)
      if (res.code === 200) {
        showAlert('保存成功', 'success')
        setModalOpen(false)
        setEditing(null)
        await load()
      } else showAlert(res.msg || '保存失败', 'error')
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '保存失败', 'error')
    } finally {
      setSaving(false)
    }
  }

  const confirmScriptDelete = async () => {
    if (scriptDeleteId == null) return
    setScriptDeleteLoading(true)
    try {
      const res = await deleteSIPScriptTemplate(scriptDeleteId)
      if (res.code !== 200) {
        showAlert(res.msg || '删除失败', 'error')
        return
      }
      showAlert('删除成功', 'success')
      setScriptDeleteOpen(false)
      setScriptDeleteId(null)
      await load()
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '删除失败', 'error')
    } finally {
      setScriptDeleteLoading(false)
    }
  }

  return (
    <div className="mt-4 space-y-3">
      <Typography.Paragraph style={{ margin: 0, fontSize: 12 }} className="rounded-lg border px-3 py-2.5">
        脚本模板用于驱动外呼流程。可使用可视化步骤编排（播报 → 倾听 → 分支），无需手写 JSON；也可在「JSON 源码」中微调。
      </Typography.Paragraph>
      <Space>
        <Button type="outline" size="small" onClick={() => navigate('/script-manager/new')}>新建脚本</Button>
        <Button type="primary" size="small" onClick={() => void load()}>刷新</Button>
      </Space>
      {loading ? <div className="p-4 text-sm text-muted-foreground">加载中...</div> : (
        <div className="overflow-x-auto rounded-lg border border-border bg-card">
          <table className="min-w-[920px] w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="text-left p-3">ID</th>
                <th className="text-left p-3">名称</th>
                <th className="text-left p-3">脚本ID</th>
                <th className="text-left p-3">启用</th>
                <th className="text-left p-3">更新时间</th>
                <th className="text-right p-3">操作</th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 ? (
                <tr><td colSpan={6} className="p-6 text-center text-muted-foreground">暂无数据</td></tr>
              ) : rows.map((r) => (
                <tr key={r.id} className="border-t border-border">
                  <td className="p-3">{r.id}</td>
                  <td className="p-3">{r.name}</td>
                  <td className="p-3 font-mono text-xs">{r.scriptId}</td>
                  <td className="p-3">{r.enabled ? '已启用' : '已停用'}</td>
                  <td className="p-3 text-xs">{r.updatedAt ? new Date(r.updatedAt).toLocaleString() : '—'}</td>
                  <td className="p-3 text-right">
                    <Space>
                      <Button type="outline" size="small" onClick={() => openEdit(r)}>编辑</Button>
                      <Button type="outline" status="danger" size="small" onClick={() => { setScriptDeleteId(r.id); setScriptDeleteOpen(true) }}>删除</Button>
                    </Space>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="flex items-center justify-between p-3 border-t border-border text-sm">
            <span className="text-muted-foreground">总计: {total}</span>
            <Space>
              <Button size="small" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>上一页</Button>
              <Button size="small" disabled={page * pageSize >= total} onClick={() => setPage((p) => p + 1)}>下一页</Button>
            </Space>
          </div>
        </div>
      )}
      <Modal
        title="编辑脚本"
        visible={modalOpen}
        style={{ width: 960 }}
        onCancel={() => setModalOpen(false)}
        footer={null}
      >
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          <Input placeholder="脚本名称" value={form.name} onChange={(v) => setForm((f) => ({ ...f, name: v }))} />
          <TextArea placeholder="描述" autoSize={{ minRows: 2 }} value={form.description} onChange={(v) => setForm((f) => ({ ...f, description: v }))} />
          <Checkbox checked={form.enabled} onChange={(c) => setForm((f) => ({ ...f, enabled: !!c }))}>启用</Checkbox>
          <ScriptSpecEditor value={form.scriptSpec} onChange={(scriptSpec) => setForm((f) => ({ ...f, scriptSpec }))} />
          <Space style={{ justifyContent: 'flex-end', width: '100%' }}>
            <Button type="outline" onClick={() => setModalOpen(false)} disabled={saving}>取消</Button>
            <Button type="primary" onClick={() => void save()} disabled={saving}>{saving ? '保存中...' : '保存'}</Button>
          </Space>
        </Space>
      </Modal>

      <Modal
        title="确认删除脚本"
        visible={scriptDeleteOpen}
        onOk={() => void confirmScriptDelete()}
        onCancel={() => { setScriptDeleteOpen(false); setScriptDeleteId(null) }}
        okText="确认删除"
        okButtonProps={{ status: 'danger', loading: scriptDeleteLoading }}
      >
        <Typography.Text>删除后不可恢复，确认继续吗？</Typography.Text>
      </Modal>
    </div>
  )
}

import { useCallback, useEffect, useState } from 'react'
import { Button, Input, Modal, Select, Space, Tag, Typography } from '@arco-design/web-react'
import { IconDelete } from '@arco-design/web-react/icon'
import { showAlert } from '@/utils/notification'
import {
  ACD_ROUTE_TYPES,
  ACD_WORK_STATES,
  createACDPoolTarget,
  deleteACDPoolTarget,
  listACDPoolTargets,
  updateACDPoolTarget,
  type ACDPoolTargetRow,
} from '@/api/acdPool'

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
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total || 0)
      }
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '加载失败', 'error')
    } finally {
      setLoading(false)
    }
  }, [active, page, routeTypeFilter])

  useEffect(() => {
    void load()
  }, [load, refreshNonce])

  const openCreate = () => {
    setEditingId(null)
    setForm(defaultForm())
    setModalOpen(true)
  }
  const openEdit = (r: ACDPoolTargetRow) => {
    setEditingId(r.id)
    setForm({
      name: r.name || '',
      routeType: r.routeType || 'sip',
      targetValue: r.targetValue || '',
      weight: r.weight ?? 0,
      workState: r.workState || 'offline',
    })
    setModalOpen(true)
  }
  const closeModal = () => {
    setModalOpen(false)
    setEditingId(null)
  }

  const save = async () => {
    setSaving(true)
    try {
      const routeType = editingId == null ? 'sip' : form.routeType
      const tv = routeType === 'sip' ? form.targetValue.trim() : ''
      if (routeType === 'sip' && !tv) {
        showAlert('SIP 目标不能为空', 'error')
        return
      }
      const body = {
        name: form.name.trim(),
        routeType,
        sipSource: '',
        targetValue: tv,
        sipTrunkHost: '',
        sipTrunkPort: 0,
        sipTrunkSignalingAddr: '',
        sipCallerId: '',
        sipCallerDisplayName: '',
        weight: Number(form.weight) || 0,
        workState: form.workState,
      }
      const res = editingId == null ? await createACDPoolTarget(body) : await updateACDPoolTarget(editingId, body)
      if (res.code === 200) {
        showAlert('保存成功', 'success')
        closeModal()
        void load()
      } else showAlert(res.msg || '保存失败', 'error')
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '保存失败', 'error')
    } finally {
      setSaving(false)
    }
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

  const workStateLabel = (s: string) =>
    ({ offline: '离线', available: '可用', ringing: '振铃中', busy: '忙碌', acw: '话后整理', break: '休息' } as Record<string, string>)[s] || s

  return (
    <div className="mt-4 space-y-3">
      <Typography.Paragraph style={{ margin: 0, fontSize: 12 }} className="rounded-lg border px-3 py-2.5">
        号码池用于来电分配和转接选线，SIP 注册与 ACD 选线独立。
      </Typography.Paragraph>
      <Space wrap align="end">
        <Space direction="vertical" size={4}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>路由类型</Typography.Text>
          <Select
            style={{ width: 112 }}
            value={routeTypeFilter === '' ? undefined : routeTypeFilter}
            placeholder="全部"
            allowClear
            onChange={(v) => setRouteTypeFilter((v as string) ?? '')}
            options={ACD_ROUTE_TYPES.map((rt) => ({ label: rt, value: rt }))}
          />
        </Space>
        <Button type="primary" size="small" onClick={() => { setPage(1); void load() }}>搜索</Button>
        <Button type="outline" size="small" onClick={openCreate}>新增 SIP 目标</Button>
      </Space>

      {loading ? (
        <div className="p-4 text-sm text-muted-foreground">加载中...</div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border bg-card">
          <table className="min-w-[760px] w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="text-left p-3 whitespace-nowrap">ID</th>
                <th className="text-left p-3 whitespace-nowrap">名称</th>
                <th className="text-left p-3 min-w-[180px]">呼叫号码</th>
                <th className="text-left p-3 whitespace-nowrap">权重</th>
                <th className="text-left p-3 min-w-[200px]">状态</th>
                <th className="text-left p-3 whitespace-nowrap text-xs">状态时间</th>
                <th className="text-right p-3 whitespace-nowrap">操作</th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 ? (
                <tr><td colSpan={7} className="p-6 text-center text-muted-foreground">暂无数据</td></tr>
              ) : rows.map((r) => (
                <tr key={r.id} className="border-t border-border align-top">
                  <td className="p-3 whitespace-nowrap">{r.id}</td>
                  <td className="p-3 max-w-[200px] truncate">{r.name || '—'}</td>
                  <td className="p-3 font-mono text-xs max-w-[260px] break-all text-muted-foreground">{r.routeType === 'sip' ? r.targetValue || '—' : '—'}</td>
                  <td className="p-3 whitespace-nowrap">{r.weight}</td>
                  <td className="p-3 align-top"><Tag size="small">{workStateLabel(r.workState)}</Tag></td>
                  <td className="p-3 whitespace-nowrap text-xs text-muted-foreground">{r.workStateAt ? new Date(r.workStateAt).toLocaleString() : '—'}</td>
                  <td className="p-3 text-right">
                    <Space>
                      <Button type="outline" size="small" onClick={() => openEdit(r)}>编辑</Button>
                      <Button type="outline" status="danger" size="small" icon={<IconDelete />} onClick={() => { setAcdDeleteId(r.id); setAcdDeleteOpen(true) }}>删除</Button>
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
        title={editingId == null ? '新增 SIP 目标' : '编辑目标'}
        visible={modalOpen}
        onCancel={closeModal}
        onOk={() => void save()}
        okText={saving ? '保存中...' : '保存'}
        confirmLoading={saving}
        style={{ width: 520 }}
      >
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          <div>
            <Typography.Text style={{ fontSize: 12 }}>名称</Typography.Text>
            <Input value={form.name} onChange={(v) => setForm((f) => ({ ...f, name: v }))} />
          </div>
          {(editingId == null || form.routeType === 'sip') && (
            <div>
              <Typography.Text style={{ fontSize: 12 }}>呼叫电话号</Typography.Text>
              <Input placeholder="例如 10086 或 13800138000" value={form.targetValue} onChange={(v) => setForm((f) => ({ ...f, targetValue: v }))} />
            </div>
          )}
          <div>
            <Typography.Text style={{ fontSize: 12 }}>权重</Typography.Text>
            <Input type="number" value={String(form.weight)} onChange={(v) => setForm((f) => ({ ...f, weight: parseInt(v, 10) || 0 }))} />
          </div>
          <div>
            <Typography.Text style={{ fontSize: 12 }}>工作状态</Typography.Text>
            <Select
              value={form.workState}
              onChange={(v) => setForm((f) => ({ ...f, workState: v as string }))}
              options={ACD_WORK_STATES.map((ws) => ({ label: workStateLabel(ws), value: ws }))}
            />
          </div>
        </Space>
      </Modal>

      <Modal
        title="确认删除号码池目标"
        visible={acdDeleteOpen}
        onOk={() => void confirmAcdDelete()}
        onCancel={() => { if (!acdDeleteLoading) { setAcdDeleteOpen(false); setAcdDeleteId(null) } }}
        okText="确认删除"
        okButtonProps={{ status: 'danger', loading: acdDeleteLoading }}
      >
        <Typography.Text>删除后不可恢复，确认继续吗？</Typography.Text>
      </Modal>
    </div>
  )
}

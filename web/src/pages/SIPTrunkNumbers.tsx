import { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Checkbox,
  Drawer,
  Input,
  Select,
  Space,
  Typography,
} from '@arco-design/web-react'
import { IconDelete } from '@arco-design/web-react/icon'
import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import {
  createTrunkNumber,
  deleteTrunkNumber,
  fetchTrunksForSelect,
  listTrunkNumbers,
  updateTrunkNumber,
  type TrunkNumberRow,
  type TrunkRow,
} from '@/api/trunks'
import { showAlert } from '@/utils/notification'

function toRFC3339OrUndefined(isoLocal: string): string | undefined {
  const s = isoLocal.trim()
  if (!s) return undefined
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return undefined
  return d.toISOString()
}

type FormState = {
  trunkId: string
  number: string
  prefix: string
  description: string
  direction: string
  status: string
  concurrent: string
  callInConcurrent: string
  isTransferRelay: boolean
  effectiveTime: string
  expirationTime: string
  providerId: string
}

const defaultForm = (): FormState => ({
  trunkId: '',
  number: '',
  prefix: '',
  description: '',
  direction: '',
  status: '',
  concurrent: '0',
  callInConcurrent: '0',
  isTransferRelay: false,
  effectiveTime: '',
  expirationTime: '',
  providerId: '0',
})

const SIPTrunkNumbers = () => {
  const [trunks, setTrunks] = useState<TrunkRow[]>([])
  const [rows, setRows] = useState<TrunkNumberRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [trunkFilter, setTrunkFilter] = useState('')
  const [numberQ, setNumberQ] = useState('')
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormState>(defaultForm)
  const [saving, setSaving] = useState(false)
  const [delOpen, setDelOpen] = useState(false)
  const [delId, setDelId] = useState<number | null>(null)
  const [delLoading, setDelLoading] = useState(false)
  const pageSize = 20

  useEffect(() => {
    void (async () => {
      try {
        const list = await fetchTrunksForSelect()
        setTrunks(list)
      } catch {
        setTrunks([])
      }
    })()
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const tid = trunkFilter ? parseInt(trunkFilter, 10) : 0
      const res = await listTrunkNumbers(page, pageSize, {
        trunkId: Number.isFinite(tid) && tid > 0 ? tid : undefined,
        number: numberQ.trim() || undefined,
      })
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total || 0)
      } else showAlert(res.msg || '加载失败', 'error')
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '加载失败', 'error')
    } finally {
      setLoading(false)
    }
  }, [page, trunkFilter, numberQ])

  useEffect(() => {
    void load()
  }, [load])

  const trunkLabel = (id: number) => trunks.find((t) => t.id === id)?.name || `线路 #${id}`

  const openCreate = () => {
    setEditingId(null)
    setForm({
      ...defaultForm(),
      trunkId: trunkFilter || (trunks[0]?.id != null ? String(trunks[0].id) : ''),
    })
    setModalOpen(true)
  }

  const openEdit = (r: TrunkNumberRow) => {
    setEditingId(r.id)
    const eff = r.effectiveTime ? new Date(r.effectiveTime).toISOString().slice(0, 16) : ''
    const exp = r.expirationTime ? new Date(r.expirationTime).toISOString().slice(0, 16) : ''
    setForm({
      trunkId: String(r.trunkId),
      number: r.number || '',
      prefix: r.prefix || '',
      description: r.description || '',
      direction: r.direction || '',
      status: r.status || '',
      concurrent: String(r.concurrent ?? 0),
      callInConcurrent: String(r.callInConcurrent ?? 0),
      isTransferRelay: !!r.isTransferRelay,
      effectiveTime: eff,
      expirationTime: exp,
      providerId: String(r.providerId ?? 0),
    })
    setModalOpen(true)
  }

  const closeModal = () => {
    setModalOpen(false)
    setEditingId(null)
  }

  const save = async () => {
    const trunkId = parseInt(form.trunkId, 10)
    const num = form.number.trim()
    if (!Number.isFinite(trunkId) || trunkId <= 0) {
      showAlert('请选择所属线路', 'error')
      return
    }
    if (!num) {
      showAlert('号码不能为空', 'error')
      return
    }
    const conc = parseInt(form.concurrent, 10) || 0
    const cin = parseInt(form.callInConcurrent, 10) || 0
    const pid = parseInt(form.providerId, 10) || 0
    const eff = toRFC3339OrUndefined(form.effectiveTime)
    const exp = toRFC3339OrUndefined(form.expirationTime)
    const body = {
      trunkId,
      number: num,
      prefix: form.prefix.trim(),
      description: form.description.trim(),
      direction: form.direction.trim(),
      status: form.status.trim(),
      concurrent: conc,
      callInConcurrent: cin,
      isTransferRelay: form.isTransferRelay,
      effectiveTime: eff ?? null,
      expirationTime: exp ?? null,
      providerId: pid,
    }
    setSaving(true)
    try {
      const res = editingId == null ? await createTrunkNumber(body) : await updateTrunkNumber(editingId, body)
      if (res.code === 200) {
        showAlert('保存成功', 'success')
        closeModal()
        void load()
      } else showAlert(res.msg || '保存失败', 'error')
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '保存失败', 'error')
    } finally {
      setSaving(false)
    }
  }

  const confirmDelete = async () => {
    if (delId == null) return
    setDelLoading(true)
    try {
      const res = await deleteTrunkNumber(delId)
      if (res.code !== 200) {
        showAlert(res.msg || '删除失败', 'error')
        return
      }
      showAlert('删除成功', 'success')
      setDelOpen(false)
      setDelId(null)
      void load()
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '删除失败', 'error')
    } finally {
      setDelLoading(false)
    }
  }

  const trunkOptions = trunks.map((t) => ({ label: t.name || `ID ${t.id}`, value: String(t.id) }))

  return (
    <BaseLayout title="中继号码" description="云联络中心 / 中继号码">
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Typography.Paragraph style={{ margin: 0, fontSize: 12, padding: '10px 12px', background: 'var(--color-fill-2)', borderRadius: 8 }}>
          维护每条中继线路下的外显 / 入局号码；生效时间请使用本地时间选择（将按 RFC3339 提交）。
        </Typography.Paragraph>
        <Space wrap align="end">
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>所属线路</Typography.Text>
            <Select
              placeholder="全部线路"
              allowClear
              style={{ width: 220 }}
              value={trunkFilter === '' ? undefined : trunkFilter}
              onChange={(v) => setTrunkFilter((v as string) ?? '')}
              options={trunkOptions}
            />
          </Space>
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>号码</Typography.Text>
            <Input allowClear placeholder="模糊搜索" style={{ width: 180 }} value={numberQ} onChange={setNumberQ} />
          </Space>
          <Button type="primary" onClick={() => { setPage(1); void load() }}>搜索</Button>
          <Button type="outline" onClick={openCreate} disabled={!trunks.length}>新增号码</Button>
        </Space>

        <Card bordered={false}>
          {loading ? (
            <Typography.Text type="secondary">加载中...</Typography.Text>
          ) : (
            <>
              <div style={{ overflowX: 'auto' }}>
                <table style={{ minWidth: 960, width: '100%', fontSize: 13 }}>
                  <thead style={{ background: 'var(--color-fill-2)' }}>
                    <tr>
                      <th style={{ textAlign: 'left', padding: 12 }}>ID</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>线路</th>
                      <th style={{ textAlign: 'left', padding: 12, fontFamily: 'monospace', fontSize: 12 }}>号码</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>用途</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>状态</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>呼出并发</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>呼入并发</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>转人工中继</th>
                      <th style={{ textAlign: 'right', padding: 12 }}>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.length === 0 ? (
                      <tr><td colSpan={9} style={{ padding: 24, textAlign: 'center' }}>暂无数据</td></tr>
                    ) : rows.map((r) => (
                      <tr key={r.id} style={{ borderTop: '1px solid var(--color-border)' }}>
                        <td style={{ padding: 12 }}>{r.id}</td>
                        <td style={{ padding: 12, maxWidth: 140 }}>{trunkLabel(r.trunkId)}</td>
                        <td style={{ padding: 12, fontFamily: 'monospace', fontSize: 12 }}>{r.number}</td>
                        <td style={{ padding: 12, fontSize: 12 }}>{r.direction || '—'}</td>
                        <td style={{ padding: 12, fontSize: 12 }}>{r.status || '—'}</td>
                        <td style={{ padding: 12 }}>{r.concurrent ?? '—'}</td>
                        <td style={{ padding: 12 }}>{r.callInConcurrent ?? '—'}</td>
                        <td style={{ padding: 12, fontSize: 12 }}>{r.isTransferRelay ? '是' : '否'}</td>
                        <td style={{ padding: 12, textAlign: 'right' }}>
                          <Space>
                            <Button type="outline" size="small" onClick={() => openEdit(r)}>编辑</Button>
                            <Button type="outline" status="danger" size="small" icon={<IconDelete />} onClick={() => { setDelId(r.id); setDelOpen(true) }}>删除</Button>
                          </Space>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 12, paddingTop: 12, borderTop: '1px solid var(--color-border)' }}>
                <Typography.Text type="secondary">总计: {total}</Typography.Text>
                <Space>
                  <Button size="small" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>上一页</Button>
                  <Button size="small" disabled={page * pageSize >= total} onClick={() => setPage((p) => p + 1)}>下一页</Button>
                </Space>
              </div>
            </>
          )}
        </Card>

        <Drawer
          title={editingId == null ? '新增中继号码' : '编辑中继号码'}
          visible={modalOpen}
          placement="right"
          width={640}
          onCancel={closeModal}
          footer={
            <Space>
              <Button onClick={closeModal} disabled={saving}>取消</Button>
              <Button type="primary" loading={saving} onClick={() => void save()}>
                {saving ? '保存中...' : '保存'}
              </Button>
            </Space>
          }
        >
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>所属线路 *</Typography.Text>
              <Select
                placeholder="请选择"
                value={form.trunkId || undefined}
                onChange={(v) => setForm((f) => ({ ...f, trunkId: v ?? '' }))}
                options={trunkOptions}
              />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>号码 *</Typography.Text>
              <Input value={form.number} onChange={(v) => setForm((f) => ({ ...f, number: v }))} />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>前缀</Typography.Text>
              <Input value={form.prefix} onChange={(v) => setForm((f) => ({ ...f, prefix: v }))} />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>备注</Typography.Text>
              <Input value={form.description} onChange={(v) => setForm((f) => ({ ...f, description: v }))} />
            </div>
            <Space style={{ width: '100%' }} size={12}>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>呼叫用途</Typography.Text>
                <Input value={form.direction} onChange={(v) => setForm((f) => ({ ...f, direction: v }))} />
              </div>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>状态</Typography.Text>
                <Input value={form.status} onChange={(v) => setForm((f) => ({ ...f, status: v }))} />
              </div>
            </Space>
            <Space style={{ width: '100%' }} size={12}>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>呼出并发</Typography.Text>
                <Input type="number" value={form.concurrent} onChange={(v) => setForm((f) => ({ ...f, concurrent: v }))} />
              </div>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>呼入并发</Typography.Text>
                <Input type="number" value={form.callInConcurrent} onChange={(v) => setForm((f) => ({ ...f, callInConcurrent: v }))} />
              </div>
            </Space>
            <Checkbox checked={form.isTransferRelay} onChange={(c) => setForm((f) => ({ ...f, isTransferRelay: !!c }))}>转人工中继号码</Checkbox>
            <Space style={{ width: '100%' }} size={12}>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>生效时间（可选）</Typography.Text>
                <Input type="datetime-local" value={form.effectiveTime} onChange={(v) => setForm((f) => ({ ...f, effectiveTime: v }))} />
              </div>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>失效时间（可选）</Typography.Text>
                <Input type="datetime-local" value={form.expirationTime} onChange={(v) => setForm((f) => ({ ...f, expirationTime: v }))} />
              </div>
            </Space>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>供应商 ID</Typography.Text>
              <Input type="number" value={form.providerId} onChange={(v) => setForm((f) => ({ ...f, providerId: v }))} />
            </div>
          </Space>
        </Drawer>

        <Drawer
          title="确认删除中继号码"
          visible={delOpen}
          placement="right"
          width={420}
          onCancel={() => { if (!delLoading) { setDelOpen(false); setDelId(null) } }}
          footer={
            <Space>
              <Button onClick={() => { if (!delLoading) { setDelOpen(false); setDelId(null) } }} disabled={delLoading}>
                取消
              </Button>
              <Button status="danger" loading={delLoading} onClick={() => void confirmDelete()}>
                确认删除
              </Button>
            </Space>
          }
        >
          <Typography.Text>删除后不可恢复（软删除），确认继续吗？</Typography.Text>
        </Drawer>
      </Space>
    </BaseLayout>
  )
}

export default SIPTrunkNumbers

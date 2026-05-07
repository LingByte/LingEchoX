import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Drawer, Input, Space, Typography } from '@arco-design/web-react'
import { IconDelete } from '@arco-design/web-react/icon'
import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import { createTrunk, deleteTrunk, listTrunks, updateTrunk, type TrunkRow } from '@/api/trunks'
import { showAlert } from '@/utils/notification'

type FormState = { name: string; description: string; prefix: string; local_addr: string; providerId: string }
const defaultForm = (): FormState => ({ name: '', description: '', prefix: '', local_addr: '', providerId: '0' })

const SIPTrunks = () => {
  const [rows, setRows] = useState<TrunkRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [nameQ, setNameQ] = useState('')
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormState>(defaultForm)
  const [saving, setSaving] = useState(false)
  const [delOpen, setDelOpen] = useState(false)
  const [delId, setDelId] = useState<number | null>(null)
  const [delLoading, setDelLoading] = useState(false)
  const pageSize = 20

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listTrunks(page, pageSize, { name: nameQ.trim() || undefined })
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total || 0)
      } else showAlert(res.msg || '加载失败', 'error')
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '加载失败', 'error')
    } finally {
      setLoading(false)
    }
  }, [page, nameQ])

  useEffect(() => {
    void load()
  }, [load])

  const openCreate = () => {
    setEditingId(null)
    setForm(defaultForm())
    setModalOpen(true)
  }
  const openEdit = (r: TrunkRow) => {
    setEditingId(r.id)
    setForm({
      name: r.name || '',
      description: r.description || '',
      prefix: r.prefix || '',
      local_addr: r.local_addr || '',
      providerId: String(r.providerId ?? 0),
    })
    setModalOpen(true)
  }
  const closeModal = () => {
    setModalOpen(false)
    setEditingId(null)
  }

  const save = async () => {
    const name = form.name.trim()
    if (!name) {
      showAlert('线路名称不能为空', 'error')
      return
    }
    const pid = parseInt(form.providerId, 10)
    const body = {
      name,
      description: form.description.trim(),
      prefix: form.prefix.trim(),
      local_addr: form.local_addr.trim(),
      providerId: Number.isFinite(pid) ? pid : 0,
    }
    setSaving(true)
    try {
      const res = editingId == null ? await createTrunk(body) : await updateTrunk(editingId, body)
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
      const res = await deleteTrunk(delId)
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

  return (
    <BaseLayout title="中继线路" description="云联络中心 / 中继线路">
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Typography.Paragraph style={{ margin: 0, fontSize: 12, padding: '10px 12px', background: 'var(--color-fill-2)', borderRadius: 8 }}>
          管理中继网关线路信息；号码资源请在「中继号码」页面维护。
        </Typography.Paragraph>
        <Space wrap>
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>名称</Typography.Text>
            <Input allowClear placeholder="模糊搜索" style={{ width: 200 }} value={nameQ} onChange={setNameQ} />
          </Space>
          <Button type="primary" onClick={() => { setPage(1); void load() }}>搜索</Button>
          <Button type="outline" onClick={openCreate}>新增线路</Button>
        </Space>

        <Card bordered={false}>
          {loading ? (
            <Typography.Text type="secondary">加载中...</Typography.Text>
          ) : (
            <>
              <div style={{ overflowX: 'auto' }}>
                <table style={{ minWidth: 720, width: '100%', fontSize: 13 }}>
                  <thead style={{ background: 'var(--color-fill-2)' }}>
                    <tr>
                      <th style={{ textAlign: 'left', padding: 12 }}>ID</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>名称</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>前缀</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>本端地址</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>供应商 ID</th>
                      <th style={{ textAlign: 'right', padding: 12 }}>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.length === 0 ? (
                      <tr><td colSpan={6} style={{ padding: 24, textAlign: 'center', color: 'var(--color-text-3)' }}>暂无数据</td></tr>
                    ) : rows.map((r) => (
                      <tr key={r.id} style={{ borderTop: '1px solid var(--color-border)' }}>
                        <td style={{ padding: 12 }}>{r.id}</td>
                        <td style={{ padding: 12, maxWidth: 200 }}>
                          <div style={{ fontWeight: 500 }}>{r.name || '—'}</div>
                          {r.description ? <div style={{ fontSize: 12, color: 'var(--color-text-3)' }}>{r.description}</div> : null}
                        </td>
                        <td style={{ padding: 12, fontFamily: 'monospace', fontSize: 12 }}>{r.prefix || '—'}</td>
                        <td style={{ padding: 12, fontFamily: 'monospace', fontSize: 12, wordBreak: 'break-all', maxWidth: 220 }}>{r.local_addr || '—'}</td>
                        <td style={{ padding: 12 }}>{r.providerId ?? '—'}</td>
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
          title={editingId == null ? '新增中继线路' : '编辑中继线路'}
          visible={modalOpen}
          placement="right"
          width={520}
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
              <Typography.Text style={{ fontSize: 12 }}>线路名称 *</Typography.Text>
              <Input value={form.name} onChange={(v) => setForm((f) => ({ ...f, name: v }))} />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>备注</Typography.Text>
              <Input value={form.description} onChange={(v) => setForm((f) => ({ ...f, description: v }))} />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>前缀</Typography.Text>
              <Input value={form.prefix} onChange={(v) => setForm((f) => ({ ...f, prefix: v }))} />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>本端地址</Typography.Text>
              <Input value={form.local_addr} onChange={(v) => setForm((f) => ({ ...f, local_addr: v }))} />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>供应商 ID</Typography.Text>
              <Input type="number" value={form.providerId} onChange={(v) => setForm((f) => ({ ...f, providerId: v }))} />
            </div>
          </Space>
        </Drawer>

        <Drawer
          title="确认删除中继线路"
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
          <Typography.Text>将同时软删除其下中继号码，确认继续吗？</Typography.Text>
        </Drawer>
      </Space>
    </BaseLayout>
  )
}

export default SIPTrunks

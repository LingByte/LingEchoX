import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Drawer, Input, Space, Typography } from '@arco-design/web-react'
import { IconDelete } from '@arco-design/web-react/icon'
import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import { TableIdCell } from '@/components/TableIdCell'
import { createTrunk, deleteTrunk, listTrunks, updateTrunk, type TrunkRow } from '@/api/trunks'
import { showAlert } from '@/utils/notification'
import { useTranslation } from '@/i18n'
import { FieldLabel } from '@/components/Form/FieldLabel'

type FormState = { name: string; description: string; prefix: string; local_addr: string }
const defaultForm = (): FormState => ({ name: '', description: '', prefix: '', local_addr: '' })

const SIPTrunks = () => {
  const { t } = useTranslation()
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
      } else showAlert(res.msg || t('common.loadFailed'), 'error')
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.loadFailed'), 'error')
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
      showAlert(t('sipTrunks.nameRequired'), 'error')
      return
    }
    const body = {
      name,
      description: form.description.trim(),
      prefix: form.prefix.trim(),
      local_addr: form.local_addr.trim(),
    }
    setSaving(true)
    try {
      const res = editingId == null ? await createTrunk(body) : await updateTrunk(editingId, body)
      if (res.code === 200) {
        showAlert(t('common.saveSuccess'), 'success')
        closeModal()
        void load()
      } else showAlert(res.msg || t('common.saveFailed'), 'error')
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.saveFailed'), 'error')
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
        showAlert(res.msg || t('common.deleteFailed'), 'error')
        return
      }
      showAlert(t('common.deleteSuccess'), 'success')
      setDelOpen(false)
      setDelId(null)
      void load()
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.deleteFailed'), 'error')
    } finally {
      setDelLoading(false)
    }
  }

  return (
    <BaseLayout title={t('pages.sipTrunks.title')} description={t('pages.sipTrunks.description')}>
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Space wrap>
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>{t('sipTrunks.nameLabel')}</Typography.Text>
            <Input allowClear placeholder={t('sipTrunks.searchFuzzy')} style={{ width: 200 }} value={nameQ} onChange={setNameQ} />
          </Space>
          <Button type="primary" onClick={() => { setPage(1); void load() }}>{t('common.search')}</Button>
          <Button type="outline" onClick={openCreate}>{t('sipTrunks.addTrunk')}</Button>
        </Space>

        <Card bordered={false}>
          {loading ? (
            <Typography.Text type="secondary">{t('common.loading')}</Typography.Text>
          ) : (
            <>
              <div style={{ overflowX: 'auto' }}>
                <table style={{ minWidth: 720, width: '100%', fontSize: 13 }}>
                  <thead style={{ background: 'var(--color-fill-2)' }}>
                    <tr>
                      <th style={{ textAlign: 'left', padding: 12 }}>ID</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>{t('sipTrunks.nameLabel')}</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>{t('sipTrunks.colPrefix')}</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>{t('sipTrunks.colGateway')}</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>{t('sipTrunks.colProvider')}</th>
                      <th style={{ textAlign: 'right', padding: 12 }}>{t('common.actions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.length === 0 ? (
                      <tr><td colSpan={6} style={{ padding: 24, textAlign: 'center', color: 'var(--color-text-3)' }}>{t('common.noData')}</td></tr>
                    ) : rows.map((r) => (
                      <tr key={r.id} style={{ borderTop: '1px solid var(--color-border)' }}>
                        <td style={{ padding: 12 }}><TableIdCell id={r.id} /></td>
                        <td style={{ padding: 12, maxWidth: 200 }}>
                          <div style={{ fontWeight: 500 }}>{r.name || '—'}</div>
                          {r.description ? <div style={{ fontSize: 12, color: 'var(--color-text-3)' }}>{r.description}</div> : null}
                        </td>
                        <td style={{ padding: 12, fontFamily: 'monospace', fontSize: 12 }}>{r.prefix || '—'}</td>
                        <td style={{ padding: 12, fontFamily: 'monospace', fontSize: 12, wordBreak: 'break-all', maxWidth: 220 }}>{r.local_addr || '—'}</td>
                        <td style={{ padding: 12, fontFamily: 'monospace', fontSize: 12, wordBreak: 'break-all', maxWidth: 240 }}>{r.providerCode || '—'}</td>
                        <td style={{ padding: 12, textAlign: 'right' }}>
                          <Space>
                            <Button type="outline" size="small" onClick={() => openEdit(r)}>{t('common.edit')}</Button>
                            <Button type="outline" status="danger" size="small" icon={<IconDelete />} onClick={() => { setDelId(r.id); setDelOpen(true) }}>{t('common.delete')}</Button>
                          </Space>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 12, paddingTop: 12, borderTop: '1px solid var(--color-border)' }}>
                <Typography.Text type="secondary">{t('common.total')}: {total}</Typography.Text>
                <Space>
                  <Button size="small" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>{t('common.previous')}</Button>
                  <Button size="small" disabled={page * pageSize >= total} onClick={() => setPage((p) => p + 1)}>{t('common.next')}</Button>
                </Space>
              </div>
            </>
          )}
        </Card>

        <Drawer
          title={editingId == null ? t('sipTrunks.drawerCreate') : t('sipTrunks.drawerEdit')}
          visible={modalOpen}
          placement="right"
          width={520}
          onCancel={closeModal}
          footer={
            <Space>
              <Button onClick={closeModal} disabled={saving}>{t('common.cancel')}</Button>
              <Button type="primary" loading={saving} onClick={() => void save()}>
                {saving ? t('common.saving') : t('common.save')}
              </Button>
            </Space>
          }
        >
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            <div>
              <FieldLabel label="线路名称" required hint="中继线路在管理端的显示名称。" />
              <Input value={form.name} onChange={(v) => setForm((f) => ({ ...f, name: v }))} />
            </div>
            <div>
              <FieldLabel label="备注" hint="运维备注，仅管理端展示。" />
              <Input value={form.description} onChange={(v) => setForm((f) => ({ ...f, description: v }))} />
            </div>
            <div>
              <FieldLabel label="前缀" hint="可选；部分网关外呼拨号前缀。" />
              <Input value={form.prefix} onChange={(v) => setForm((f) => ({ ...f, prefix: v }))} />
            </div>
            <div>
              <FieldLabel
                label="网关地址 (host:port)"
                hint="外呼 / 转呼 INVITE 的下一跳 SIP 网关地址（如 183.213.19.195:50400），取代原 SIP_TRANSFER_HOST + SIP_TRANSFER_PORT。主叫号码与显示名在「中继号码」页按每条号码配置。"
              />
              <Input
                placeholder="例如 183.213.19.195:50400"
                value={form.local_addr}
                onChange={(v) => setForm((f) => ({ ...f, local_addr: v }))}
              />
            </div>
          </Space>
        </Drawer>

        <Drawer
          title={t('sipTrunks.deleteTitle')}
          visible={delOpen}
          placement="right"
          width={420}
          onCancel={() => { if (!delLoading) { setDelOpen(false); setDelId(null) } }}
          footer={
            <Space>
              <Button onClick={() => { if (!delLoading) { setDelOpen(false); setDelId(null) } }} disabled={delLoading}>
                {t('common.cancel')}
              </Button>
              <Button status="danger" loading={delLoading} onClick={() => void confirmDelete()}>
                {t('common.confirmDelete')}
              </Button>
            </Space>
          }
        >
          <Typography.Text>{t('sipTrunks.deleteBody')}</Typography.Text>
        </Drawer>
      </Space>
    </BaseLayout>
  )
}

export default SIPTrunks

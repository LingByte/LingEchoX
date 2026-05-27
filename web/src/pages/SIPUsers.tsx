import { useEffect, useState } from 'react'
import { Button, Card, Modal, Space, Typography } from '@arco-design/web-react'
import { IconEye, IconDelete } from '@arco-design/web-react/icon'
import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import { TableIdCell } from '@/components/TableIdCell'
import { listSIPUsers, deleteSIPUser, type SIPUserRow } from '@/api/sipUsers'
import { showAlert } from '@/utils/notification'
import { useTranslation } from '@/i18n'

const SIPUsers = () => {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [rows, setRows] = useState<SIPUserRow[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [detailOpen, setDetailOpen] = useState(false)
  const [current, setCurrent] = useState<SIPUserRow | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteId, setDeleteId] = useState<number | null>(null)
  const [deleteLoading, setDeleteLoading] = useState(false)
  const pageSize = 20

  const load = async () => {
    setLoading(true)
    try {
      const res = await listSIPUsers(page, pageSize)
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total || 0)
      } else {
        showAlert(res.msg || '加载失败', 'error')
      }
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '加载失败', 'error')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [page])

  const fmt = (s?: string) => (s ? new Date(s).toLocaleString() : '—')
  const signalAddr = (u: SIPUserRow) =>
    u.remoteIp ? `${u.remoteIp}${u.remotePort != null ? `:${u.remotePort}` : ''}` : '—'
  const onlineLabel = (online?: boolean) => (online ? t('sipUsers.online') : t('sipUsers.offline'))

  const openDetail = (u: SIPUserRow) => {
    setCurrent(u)
    setDetailOpen(true)
  }

  const openDelete = (id: number) => {
    setDeleteId(id)
    setDeleteOpen(true)
  }

  const confirmDelete = async () => {
    if (deleteId == null) return
    setDeleteLoading(true)
    try {
      const res = await deleteSIPUser(deleteId)
      if (res.code !== 200) {
        showAlert(res.msg || '删除失败', 'error')
        return
      }
      showAlert('删除成功', 'success')
      await load()
      setDeleteOpen(false)
      setDeleteId(null)
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '删除失败', 'error')
    } finally {
      setDeleteLoading(false)
    }
  }

  return (
    <BaseLayout title={t('pages.sipUsers.title')} description={t('pages.sipUsers.description')}>
      <Card bordered={false}>
        <div style={{ overflowX: 'auto' }}>
          <table style={{ minWidth: 860, width: '100%', fontSize: 13 }}>
            <thead style={{ background: 'var(--color-fill-2)' }}>
              <tr>
                <th style={{ textAlign: 'left', padding: 12 }}>ID</th>
                <th style={{ textAlign: 'left', padding: 12 }}>AOR</th>
                <th style={{ textAlign: 'left', padding: 12 }}>状态</th>
                <th style={{ textAlign: 'left', padding: 12 }}>注册失效</th>
                <th style={{ textAlign: 'left', padding: 12 }}>最后活跃</th>
                <th style={{ textAlign: 'right', padding: 12 }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td style={{ padding: 24, textAlign: 'center' }} colSpan={6}>{t('common.loading')}</td></tr>
              ) : rows.length === 0 ? (
                <tr><td style={{ padding: 24, textAlign: 'center' }} colSpan={6}>{t('common.noData')}</td></tr>
              ) : rows.map((u) => (
                <tr key={u.id} style={{ borderTop: '1px solid var(--color-border)' }}>
                  <td style={{ padding: 12 }}><TableIdCell id={u.id} /></td>
                  <td style={{ padding: 12, wordBreak: 'break-all' }}>{u.username}@{u.domain}</td>
                  <td style={{ padding: 12 }}>{onlineLabel(u.online)}</td>
                  <td style={{ padding: 12 }}>{fmt(u.expiresAt)}</td>
                  <td style={{ padding: 12 }}>{fmt(u.lastSeenAt)}</td>
                  <td style={{ padding: 12, textAlign: 'right', whiteSpace: 'nowrap' }}>
                    <Space>
                      <Button type="outline" size="small" icon={<IconEye />} onClick={() => openDetail(u)}>{t('common.detail')}</Button>
                      <Button type="outline" status="danger" size="small" icon={<IconDelete />} onClick={() => openDelete(u.id)}>{t('common.delete')}</Button>
                    </Space>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 12, borderTop: '1px solid var(--color-border)' }}>
          <Typography.Text type="secondary">{t('common.total')}: {total}</Typography.Text>
          <Space>
            <Button size="small" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>{t('common.previous')}</Button>
            <Button size="small" disabled={page * pageSize >= total} onClick={() => setPage((p) => p + 1)}>{t('common.next')}</Button>
          </Space>
        </div>
      </Card>

      <Modal
        title={t('sipUsers.userDetail')}
        visible={detailOpen}
        onCancel={() => { setDetailOpen(false); setCurrent(null) }}
        footer={null}
        style={{ width: 720 }}
      >
        {current && (
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, fontSize: 13 }}>
            <div style={{ border: '1px solid var(--color-border)', borderRadius: 4, padding: 12 }}>
              <div style={{ fontSize: 12, color: 'var(--color-text-3)', marginBottom: 4 }}>ID</div>
              <div style={{ fontWeight: 500, maxWidth: '100%' }}><TableIdCell id={current.id} /></div>
            </div>
            <div style={{ border: '1px solid var(--color-border)', borderRadius: 4, padding: 12 }}>
              <div style={{ fontSize: 12, color: 'var(--color-text-3)', marginBottom: 4 }}>状态</div>
              <div style={{ fontWeight: 500 }}>{current.online ? 'online' : 'offline'}</div>
            </div>
            <div style={{ gridColumn: '1 / -1', border: '1px solid var(--color-border)', borderRadius: 4, padding: 12 }}>
              <div style={{ fontSize: 12, color: 'var(--color-text-3)', marginBottom: 4 }}>AOR</div>
              <div style={{ fontWeight: 500, wordBreak: 'break-all' }}>{current.username}@{current.domain}</div>
            </div>
            <div style={{ gridColumn: '1 / -1', border: '1px solid var(--color-border)', borderRadius: 4, padding: 12 }}>
              <div style={{ fontSize: 12, color: 'var(--color-text-3)', marginBottom: 4 }}>Contact</div>
              <div style={{ fontWeight: 500, wordBreak: 'break-all' }}>{current.contactUri || '—'}</div>
            </div>
            <div style={{ border: '1px solid var(--color-border)', borderRadius: 4, padding: 12 }}>
              <div style={{ fontSize: 12, color: 'var(--color-text-3)', marginBottom: 4 }}>信令地址</div>
              <div style={{ fontWeight: 500, wordBreak: 'break-all' }}>{signalAddr(current)}</div>
            </div>
            <div style={{ border: '1px solid var(--color-border)', borderRadius: 4, padding: 12 }}>
              <div style={{ fontSize: 12, color: 'var(--color-text-3)', marginBottom: 4 }}>UA</div>
              <div style={{ fontWeight: 500, wordBreak: 'break-all' }}>{current.userAgent || '—'}</div>
            </div>
            <div style={{ border: '1px solid var(--color-border)', borderRadius: 4, padding: 12 }}>
              <div style={{ fontSize: 12, color: 'var(--color-text-3)', marginBottom: 4 }}>注册失效</div>
              <div style={{ fontWeight: 500 }}>{fmt(current.expiresAt)}</div>
            </div>
            <div style={{ border: '1px solid var(--color-border)', borderRadius: 4, padding: 12 }}>
              <div style={{ fontSize: 12, color: 'var(--color-text-3)', marginBottom: 4 }}>最后活跃</div>
              <div style={{ fontWeight: 500 }}>{fmt(current.lastSeenAt)}</div>
            </div>
            <div style={{ gridColumn: '1 / -1', border: '1px solid var(--color-border)', borderRadius: 4, padding: 12 }}>
              <div style={{ fontSize: 12, color: 'var(--color-text-3)', marginBottom: 4 }}>创建时间</div>
              <div style={{ fontWeight: 500 }}>{fmt(current.createdAt)}</div>
            </div>
          </div>
        )}
      </Modal>

      <Modal
        title={t('sipUsers.deleteTitle')}
        visible={deleteOpen}
        onOk={() => void confirmDelete()}
        onCancel={() => { setDeleteOpen(false); setDeleteId(null) }}
        okText={t('common.confirmDelete')}
        cancelText={t('common.cancel')}
        okButtonProps={{ status: 'danger', loading: deleteLoading }}
      >
        <Typography.Text>{t('sipUsers.deleteBody')}</Typography.Text>
      </Modal>
    </BaseLayout>
  )
}

export default SIPUsers

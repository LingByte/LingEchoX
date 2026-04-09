import { useEffect, useState } from 'react'
import { Eye, Trash2 } from 'lucide-react'
import AdminLayout from '@/components/Layout/AdminLayout'
import Card from '@/components/UI/Card'
import Button from '@/components/UI/Button'
import { listSIPUsers, deleteSIPUser, type SIPUserRow } from '@/api/sipContactCenter'
import { showAlert } from '@/utils/notification'
import Modal from '@/components/UI/Modal'
import ConfirmDialog from '@/components/UI/ConfirmDialog'

const SIPUsers = () => {
  const [loading, setLoading] = useState(false)
  const [rows, setRows] = useState<SIPUserRow[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [detailOpen, setDetailOpen] = useState(false)
  const [current, setCurrent] = useState<SIPUserRow | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteId, setDeleteId] = useState<number | null>(null)
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
    } catch (e: any) {
      showAlert(e?.msg || '加载失败', 'error')
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
  const onlineLabel = (online?: boolean) => (online ? '在线' : '离线')

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
    try {
      await deleteSIPUser(deleteId)
      showAlert('删除成功', 'success')
      await load()
    } catch (e: any) {
      showAlert(e?.msg || '删除失败', 'error')
      throw e
    } finally {
      setDeleteOpen(false)
      setDeleteId(null)
    }
  }

  return (
    <AdminLayout title="SIP 用户" description="云联络中心 / SIP 用户">
      <Card className="p-0 overflow-hidden">
        <div className="overflow-x-auto">
          <table className="min-w-[860px] w-full text-sm">
            <thead className="bg-slate-100 dark:bg-slate-800">
              <tr>
                <th className="text-left p-3">ID</th>
                <th className="text-left p-3">AOR</th>
                <th className="text-left p-3">状态</th>
                <th className="text-left p-3">注册失效</th>
                <th className="text-left p-3">最后活跃</th>
                <th className="text-right p-3">操作</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td className="p-6 text-center" colSpan={6}>加载中...</td></tr>
              ) : rows.length === 0 ? (
                <tr><td className="p-6 text-center" colSpan={6}>暂无数据</td></tr>
              ) : rows.map((u) => (
                <tr key={u.id} className="border-t border-slate-200 dark:border-slate-700">
                  <td className="p-3">{u.id}</td>
                  <td className="p-3 break-all">{u.username}@{u.domain}</td>
                  <td className="p-3">{onlineLabel(u.online)}</td>
                  <td className="p-3">{fmt(u.expiresAt)}</td>
                  <td className="p-3">{fmt(u.lastSeenAt)}</td>
                  <td className="p-3 text-right space-x-2 whitespace-nowrap">
                    <Button
                      variant="outline"
                      size="sm"
                      leftIcon={<Eye className="w-4 h-4" />}
                      className="!rounded-full !px-4"
                      onClick={() => openDetail(u)}
                    >
                      详情
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      leftIcon={<Trash2 className="w-4 h-4" />}
                      className="!rounded-full !px-4 border-red-300 text-red-600 hover:bg-red-50 dark:border-red-700 dark:text-red-400 dark:hover:bg-red-950/40"
                      onClick={() => openDelete(u.id)}
                    >
                      删除
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="flex items-center justify-between p-3 border-t border-slate-200 dark:border-slate-700">
          <span className="text-sm text-slate-500">总计: {total}</span>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>上一页</Button>
            <Button variant="outline" size="sm" disabled={page * pageSize >= total} onClick={() => setPage((p) => p + 1)}>下一页</Button>
          </div>
        </div>
      </Card>

      <Modal
        isOpen={detailOpen}
        onClose={() => {
          setDetailOpen(false)
          setCurrent(null)
        }}
        title="SIP 用户详情"
        size="lg"
      >
        {current && (
          <div className="grid grid-cols-2 gap-3 text-sm">
            <div className="rounded-md border border-slate-200 dark:border-slate-700 p-3">
              <div className="text-xs text-slate-500 mb-1">ID</div>
              <div className="font-medium">{current.id}</div>
            </div>
            <div className="rounded-md border border-slate-200 dark:border-slate-700 p-3">
              <div className="text-xs text-slate-500 mb-1">状态</div>
              <div className="font-medium">{current.online ? 'online' : 'offline'}</div>
            </div>
            <div className="rounded-md border border-slate-200 dark:border-slate-700 p-3 col-span-2">
              <div className="text-xs text-slate-500 mb-1">AOR</div>
              <div className="font-medium break-all">{current.username}@{current.domain}</div>
            </div>
            <div className="rounded-md border border-slate-200 dark:border-slate-700 p-3 col-span-2">
              <div className="text-xs text-slate-500 mb-1">Contact</div>
              <div className="font-medium break-all">{current.contactUri || '—'}</div>
            </div>
            <div className="rounded-md border border-slate-200 dark:border-slate-700 p-3">
              <div className="text-xs text-slate-500 mb-1">信令地址</div>
              <div className="font-medium break-all">{signalAddr(current)}</div>
            </div>
            <div className="rounded-md border border-slate-200 dark:border-slate-700 p-3">
              <div className="text-xs text-slate-500 mb-1">UA</div>
              <div className="font-medium break-all">{current.userAgent || '—'}</div>
            </div>
            <div className="rounded-md border border-slate-200 dark:border-slate-700 p-3">
              <div className="text-xs text-slate-500 mb-1">注册失效</div>
              <div className="font-medium">{fmt(current.expiresAt)}</div>
            </div>
            <div className="rounded-md border border-slate-200 dark:border-slate-700 p-3">
              <div className="text-xs text-slate-500 mb-1">最后活跃</div>
              <div className="font-medium">{fmt(current.lastSeenAt)}</div>
            </div>
            <div className="rounded-md border border-slate-200 dark:border-slate-700 p-3 col-span-2">
              <div className="text-xs text-slate-500 mb-1">创建时间</div>
              <div className="font-medium">{fmt(current.createdAt)}</div>
            </div>
          </div>
        )}
      </Modal>

      <ConfirmDialog
        isOpen={deleteOpen}
        onClose={() => {
          setDeleteOpen(false)
          setDeleteId(null)
        }}
        onConfirm={confirmDelete}
        title="确认删除 SIP 用户"
        message="删除后不可恢复，确认继续吗？"
        confirmText="确认删除"
        cancelText="取消"
        variant="danger"
      />
    </AdminLayout>
  )
}

export default SIPUsers

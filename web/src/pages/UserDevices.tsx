import { useState, useEffect } from 'react'
import { Smartphone, Monitor, Tablet, Globe, Trash2, Shield, ShieldCheck, Edit2, Eye } from 'lucide-react'
import AdminLayout from '@/components/Layout/AdminLayout'
import Card from '@/components/UI/Card'
import Button from '@/components/UI/Button'
import Input from '@/components/UI/Input'
import Modal from '@/components/UI/Modal'
import ConfirmDialog from '@/components/UI/ConfirmDialog'
import { getUserDevices, deleteUserDevice, trustUserDevice, getUserDevice, updateUserDevice, type UserDevice } from '@/services/adminApi'
import { showAlert } from '@/utils/notification'
import { usePagination } from '@/hooks/usePagination'

const UserDevices = () => {
  const [devices, setDevices] = useState<UserDevice[]>([])
  const [loading, setLoading] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<UserDevice | null>(null)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [editTarget, setEditTarget] = useState<UserDevice | null>(null)
  const [editDeviceName, setEditDeviceName] = useState('')
  const [showEditModal, setShowEditModal] = useState(false)
  const [viewTarget, setViewTarget] = useState<UserDevice | null>(null)
  const [showViewModal, setShowViewModal] = useState(false)

  const {
    currentPage,
    totalPages,
    pageData,
    totalItems,
    goToPage
  } = usePagination({
    data: devices,
    pageSize: 10
  })

  useEffect(() => {
    fetchDevices()
  }, [])

  const fetchDevices = async () => {
    try {
      setLoading(true)
      const data = await getUserDevices()
      setDevices(data.devices)
    } catch (error: any) {
      showAlert('获取设备列表失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return

    try {
      setLoading(true)
      await deleteUserDevice(deleteTarget.deviceId)
      showAlert('删除设备成功', 'success')
      setShowDeleteConfirm(false)
      setDeleteTarget(null)
      await fetchDevices()
    } catch (error: any) {
      showAlert('删除设备失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const handleTrust = async (device: UserDevice) => {
    try {
      setLoading(true)
      await trustUserDevice(device.deviceId)
      showAlert('信任设备成功', 'success')
      await fetchDevices()
    } catch (error: any) {
      showAlert('信任设备失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const handleEdit = (device: UserDevice) => {
    setEditTarget(device)
    setEditDeviceName(device.deviceName)
    setShowEditModal(true)
  }

  const handleSaveEdit = async () => {
    if (!editTarget) return

    try {
      setLoading(true)
      await updateUserDevice(editTarget.deviceId, { deviceName: editDeviceName })
      showAlert('更新设备成功', 'success')
      setShowEditModal(false)
      setEditTarget(null)
      await fetchDevices()
    } catch (error: any) {
      showAlert('更新设备失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const handleView = async (device: UserDevice) => {
    try {
      setLoading(true)
      const data = await getUserDevice(device.deviceId)
      setViewTarget(data.device)
      setShowViewModal(true)
    } catch (error: any) {
      showAlert('获取设备详情失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const getDeviceIcon = (type: string) => {
    switch (type) {
      case 'mobile':
        return <Smartphone className="w-5 h-5" />
      case 'tablet':
        return <Tablet className="w-5 h-5" />
      case 'desktop':
        return <Monitor className="w-5 h-5" />
      default:
        return <Globe className="w-5 h-5" />
    }
  }

  const renderDeviceItem = (device: UserDevice) => (
    <Card>
      <div className="p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="flex-1">
            <div className="flex items-center gap-3 mb-2">
              <div className="text-blue-600 dark:text-blue-400">
                {getDeviceIcon(device.deviceType)}
              </div>
              <div>
                <h3 className="font-semibold text-slate-900 dark:text-white">
                  {device.deviceName}
                </h3>
                <p className="text-sm text-slate-500 dark:text-slate-400">
                  {device.os} · {device.browser}
                </p>
              </div>
              {device.isTrusted ? (
                <span className="px-2 py-1 text-xs font-medium bg-green-100 dark:bg-green-900/20 text-green-700 dark:text-green-400 rounded flex items-center gap-1">
                  <ShieldCheck className="w-3 h-3" />
                  已信任
                </span>
              ) : (
                <span className="px-2 py-1 text-xs font-medium bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-400 rounded">
                  未信任
                </span>
              )}
            </div>
            <div className="mt-3 space-y-1 text-sm text-slate-600 dark:text-slate-400">
              <div>IP地址: {device.ipAddress}</div>
              <div>位置: {device.location}</div>
              <div>最后使用: {formatDate(device.lastUsedAt)}</div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => handleView(device)}
              leftIcon={<Eye className="w-4 h-4" />}
            >
              查看
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => handleEdit(device)}
              leftIcon={<Edit2 className="w-4 h-4" />}
            >
              编辑
            </Button>
            {!device.isTrusted && (
              <Button
                size="sm"
                variant="ghost"
                onClick={() => handleTrust(device)}
                leftIcon={<Shield className="w-4 h-4" />}
              >
                信任
              </Button>
            )}
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setDeleteTarget(device)
                setShowDeleteConfirm(true)
              }}
              className="text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300"
              leftIcon={<Trash2 className="w-4 h-4" />}
            >
              删除
            </Button>
          </div>
        </div>
      </div>
    </Card>
  )

  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleString('zh-CN')
  }

  return (
    <AdminLayout title="我的设备" description="管理您的登录设备">
      <div className="space-y-6">
        {loading && devices.length === 0 ? (
          <Card>
            <div className="p-12 text-center">
              <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
              <p className="mt-4 text-slate-500 dark:text-slate-400">加载中...</p>
            </div>
          </Card>
        ) : devices.length === 0 ? (
          <Card>
            <div className="p-12 text-center">
              <Smartphone className="w-16 h-16 mx-auto mb-4 text-slate-400 dark:text-slate-500" />
              <p className="text-slate-500 dark:text-slate-400">暂无设备记录</p>
            </div>
          </Card>
        ) : (
          <>
            <div className="space-y-4">
              {pageData.map((device) => renderDeviceItem(device))}
            </div>

            {totalPages > 1 && (
              <div className="flex items-center justify-between">
                <div className="text-sm text-slate-600 dark:text-slate-400">
                  共 {totalItems} 条记录
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => goToPage(currentPage - 1)}
                    disabled={currentPage === 1}
                  >
                    上一页
                  </Button>
                  <span className="text-sm text-slate-600 dark:text-slate-400">
                    第 {currentPage} 页 / 共 {totalPages} 页
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => goToPage(currentPage + 1)}
                    disabled={currentPage >= totalPages}
                  >
                    下一页
                  </Button>
                </div>
              </div>
            )}
          </>
        )}

        {/* 编辑设备弹窗 */}
        <Modal
          isOpen={showEditModal}
          onClose={() => {
            setShowEditModal(false)
            setEditTarget(null)
            setEditDeviceName('')
          }}
          title="编辑设备"
          size="md"
        >
          <div className="space-y-4">
            <Input
              label="设备名称"
              placeholder="请输入设备名称"
              value={editDeviceName}
              onChange={(e) => setEditDeviceName(e.target.value)}
              required
            />
            <div className="flex justify-end gap-2 pt-4">
              <Button
                variant="ghost"
                onClick={() => {
                  setShowEditModal(false)
                  setEditTarget(null)
                  setEditDeviceName('')
                }}
              >
                取消
              </Button>
              <Button
                variant="primary"
                onClick={handleSaveEdit}
                loading={loading}
              >
                保存
              </Button>
            </div>
          </div>
        </Modal>

        {/* 查看设备详情弹窗 */}
        <Modal
          isOpen={showViewModal}
          onClose={() => {
            setShowViewModal(false)
            setViewTarget(null)
          }}
          title="设备详情"
          size="lg"
        >
          {viewTarget && (
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="text-sm font-medium text-slate-700 dark:text-slate-300">设备名称</label>
                  <p className="mt-1 text-sm text-slate-900 dark:text-white">{viewTarget.deviceName}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-slate-700 dark:text-slate-300">设备类型</label>
                  <p className="mt-1 text-sm text-slate-900 dark:text-white">{viewTarget.deviceType}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-slate-700 dark:text-slate-300">操作系统</label>
                  <p className="mt-1 text-sm text-slate-900 dark:text-white">{viewTarget.os}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-slate-700 dark:text-slate-300">浏览器</label>
                  <p className="mt-1 text-sm text-slate-900 dark:text-white">{viewTarget.browser}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-slate-700 dark:text-slate-300">IP地址</label>
                  <p className="mt-1 text-sm text-slate-900 dark:text-white">{viewTarget.ipAddress}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-slate-700 dark:text-slate-300">位置</label>
                  <p className="mt-1 text-sm text-slate-900 dark:text-white">{viewTarget.location}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-slate-700 dark:text-slate-300">设备ID</label>
                  <p className="mt-1 text-sm text-slate-900 dark:text-white font-mono">{viewTarget.deviceId}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-slate-700 dark:text-slate-300">状态</label>
                  <p className="mt-1">
                    {viewTarget.isTrusted ? (
                      <span className="px-2 py-1 text-xs font-medium bg-green-100 dark:bg-green-900/20 text-green-700 dark:text-green-400 rounded">
                        已信任
                      </span>
                    ) : (
                      <span className="px-2 py-1 text-xs font-medium bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-400 rounded">
                        未信任
                      </span>
                    )}
                  </p>
                </div>
                <div>
                  <label className="text-sm font-medium text-slate-700 dark:text-slate-300">最后使用</label>
                  <p className="mt-1 text-sm text-slate-900 dark:text-white">{formatDate(viewTarget.lastUsedAt)}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-slate-700 dark:text-slate-300">创建时间</label>
                  <p className="mt-1 text-sm text-slate-900 dark:text-white">{formatDate(viewTarget.createdAt)}</p>
                </div>
              </div>
              <div>
                <label className="text-sm font-medium text-slate-700 dark:text-slate-300">User-Agent</label>
                <p className="mt-1 text-xs text-slate-600 dark:text-slate-400 font-mono break-all bg-slate-100 dark:bg-slate-800 p-2 rounded">
                  {viewTarget.userAgent}
                </p>
              </div>
            </div>
          )}
        </Modal>

        <ConfirmDialog
          isOpen={showDeleteConfirm}
          onClose={() => {
            setShowDeleteConfirm(false)
            setDeleteTarget(null)
          }}
          onConfirm={handleDelete}
          title="删除设备"
          message={`确定要删除设备 "${deleteTarget?.deviceName}" 吗？`}
          confirmText="删除"
          cancelText="取消"
          variant="danger"
        />
      </div>
    </AdminLayout>
  )
}

export default UserDevices

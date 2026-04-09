import { useState, useEffect } from 'react'
import { Lock, Unlock, Search, AlertCircle } from 'lucide-react'
import AdminLayout from '@/components/Layout/AdminLayout'
import Card from '@/components/UI/Card'
import Button from '@/components/UI/Button'
import Input from '@/components/UI/Input'
import ConfirmDialog from '@/components/UI/ConfirmDialog'
import { getAccountLocks, unlockAccount, type AccountLock } from '@/services/adminApi'
import { showAlert } from '@/utils/notification'

const AccountLocks = () => {
  const [locks, setLocks] = useState<AccountLock[]>([])
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [filters, setFilters] = useState({
    user_id: '',
    email: '',
    is_active: '',
  })
  const [unlockTarget, setUnlockTarget] = useState<AccountLock | null>(null)
  const [showUnlockConfirm, setShowUnlockConfirm] = useState(false)

  useEffect(() => {
    fetchLocks()
  }, [page, filters])

  const fetchLocks = async () => {
    try {
      setLoading(true)
      const params: any = { page, page_size: pageSize }
      if (filters.user_id) params.user_id = parseInt(filters.user_id)
      if (filters.email) params.email = filters.email
      if (filters.is_active) params.is_active = filters.is_active === 'true'
      
      const data = await getAccountLocks(params)
      setLocks(data.locks)
      setTotal(data.total)
    } catch (error: any) {
      showAlert('获取账号锁定记录失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const handleUnlock = async () => {
    if (!unlockTarget) return

    try {
      setLoading(true)
      await unlockAccount(unlockTarget.id)
      showAlert('解锁账号成功', 'success')
      setShowUnlockConfirm(false)
      setUnlockTarget(null)
      await fetchLocks()
    } catch (error: any) {
      showAlert('解锁账号失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleString('zh-CN')
  }

  const isLocked = (lock: AccountLock) => {
    return lock.isActive && new Date(lock.unlockAt) > new Date()
  }

  return (
    <AdminLayout title="账号锁定" description="管理账号锁定记录">
      <div className="space-y-6">
        {/* 筛选区域 */}
        <Card>
          <div className="p-4 space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <Input
                label="用户ID"
                placeholder="输入用户ID"
                value={filters.user_id}
                onChange={(e) => setFilters({ ...filters, user_id: e.target.value })}
              />
              <Input
                label="邮箱"
                placeholder="输入邮箱"
                value={filters.email}
                onChange={(e) => setFilters({ ...filters, email: e.target.value })}
              />
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                  状态
                </label>
                <select
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-800 text-slate-900 dark:text-white"
                  value={filters.is_active}
                  onChange={(e) => setFilters({ ...filters, is_active: e.target.value })}
                >
                  <option value="">全部</option>
                  <option value="true">已锁定</option>
                  <option value="false">已解锁</option>
                </select>
              </div>
            </div>
            <Button
              variant="primary"
              onClick={() => {
                setPage(1)
                fetchLocks()
              }}
              leftIcon={<Search className="w-4 h-4" />}
            >
              搜索
            </Button>
          </div>
        </Card>

        {/* 锁定记录列表 */}
        <Card>
          <div className="p-4">
            {loading ? (
              <div className="text-center py-12">
                <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
                <p className="mt-4 text-slate-500 dark:text-slate-400">加载中...</p>
              </div>
            ) : locks.length === 0 ? (
              <div className="text-center py-12">
                <Lock className="w-16 h-16 mx-auto mb-4 text-slate-400 dark:text-slate-500" />
                <p className="text-slate-500 dark:text-slate-400">暂无锁定记录</p>
              </div>
            ) : (
              <>
                <div className="space-y-3">
                  {locks.map((lock) => (
                    <div
                      key={lock.id}
                      className="p-4 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800/50 transition-colors"
                    >
                      <div className="flex items-start justify-between gap-4">
                        <div className="flex-1 space-y-2">
                          <div className="flex items-center gap-2">
                            <Lock className="w-5 h-5 text-red-600 dark:text-red-400" />
                            <span className="font-semibold text-slate-900 dark:text-white">
                              {lock.email}
                            </span>
                            {isLocked(lock) ? (
                              <span className="px-2 py-0.5 text-xs bg-red-100 dark:bg-red-900/20 text-red-700 dark:text-red-400 rounded flex items-center gap-1">
                                <AlertCircle className="w-3 h-3" />
                                已锁定
                              </span>
                            ) : (
                              <span className="px-2 py-0.5 text-xs bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-400 rounded">
                                已解锁
                              </span>
                            )}
                          </div>
                          <div className="text-sm text-slate-600 dark:text-slate-400 space-y-1">
                            <div>用户ID: {lock.userId}</div>
                            <div>IP地址: {lock.ipAddress}</div>
                            <div>失败次数: {lock.failedAttempts}</div>
                            <div>锁定原因: {lock.reason}</div>
                            <div>锁定时间: {formatDate(lock.lockedAt)}</div>
                            <div>解锁时间: {formatDate(lock.unlockAt)}</div>
                          </div>
                        </div>
                        {isLocked(lock) && (
                          <Button
                            size="sm"
                            variant="primary"
                            onClick={() => {
                              setUnlockTarget(lock)
                              setShowUnlockConfirm(true)
                            }}
                            leftIcon={<Unlock className="w-4 h-4" />}
                          >
                            解锁
                          </Button>
                        )}
                      </div>
                    </div>
                  ))}
                </div>

                {/* 分页 */}
                <div className="mt-6 flex items-center justify-between">
                  <div className="text-sm text-slate-600 dark:text-slate-400">
                    共 {total} 条记录
                  </div>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setPage(page - 1)}
                      disabled={page === 1}
                    >
                      上一页
                    </Button>
                    <span className="text-sm text-slate-600 dark:text-slate-400">
                      第 {page} 页 / 共 {Math.ceil(total / pageSize)} 页
                    </span>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setPage(page + 1)}
                      disabled={page >= Math.ceil(total / pageSize)}
                    >
                      下一页
                    </Button>
                  </div>
                </div>
              </>
            )}
          </div>
        </Card>

        <ConfirmDialog
          isOpen={showUnlockConfirm}
          onClose={() => {
            setShowUnlockConfirm(false)
            setUnlockTarget(null)
          }}
          onConfirm={handleUnlock}
          title="解锁账号"
          message={`确定要解锁账号 "${unlockTarget?.email}" 吗？`}
          confirmText="解锁"
          cancelText="取消"
        />
      </div>
    </AdminLayout>
  )
}

export default AccountLocks

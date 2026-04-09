import { useState, useEffect } from 'react'
import { Search, FileText, Calendar, User, Globe, Monitor } from 'lucide-react'
import AdminLayout from '@/components/Layout/AdminLayout'
import Card from '@/components/UI/Card'
import Button from '@/components/UI/Button'
import Input from '@/components/UI/Input'
import { getOperationLogs, type OperationLog } from '@/services/adminApi'
import { showAlert } from '@/utils/notification'

const OperationLogs = () => {
  const [logs, setLogs] = useState<OperationLog[]>([])
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [filters, setFilters] = useState({
    user_id: '',
    action: '',
    target: '',
  })

  useEffect(() => {
    fetchLogs()
  }, [page, filters])

  const fetchLogs = async () => {
    try {
      setLoading(true)
      const params: any = { page, page_size: pageSize }
      if (filters.user_id) params.user_id = parseInt(filters.user_id)
      if (filters.action) params.action = filters.action
      if (filters.target) params.target = filters.target
      
      const data = await getOperationLogs(params)
      setLogs(data.logs)
      setTotal(data.total)
    } catch (error: any) {
      showAlert('获取操作日志失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const renderLogItem = (log: OperationLog) => (
    <div key={log.id} className="p-4 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800/50 transition-colors">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 space-y-2">
          <div className="flex items-center gap-2">
            <span className="font-semibold text-slate-900 dark:text-white">{log.details}</span>
            <span className="px-2 py-0.5 text-xs bg-blue-100 dark:bg-blue-900/20 text-blue-700 dark:text-blue-400 rounded">
              {log.request_method}
            </span>
          </div>
          <div className="flex items-center gap-4 text-sm text-slate-600 dark:text-slate-400">
            <span className="flex items-center gap-1">
              <User className="w-4 h-4" />
              {log.username} (ID: {log.user_id})
            </span>
            <span className="flex items-center gap-1">
              <Monitor className="w-4 h-4" />
              {log.device} - {log.browser}
            </span>
            <span className="flex items-center gap-1">
              <Globe className="w-4 h-4" />
              {log.location}
            </span>
            <span className="flex items-center gap-1">
              <Calendar className="w-4 h-4" />
              {formatDate(log.created_at)}
            </span>
          </div>
          <div className="text-xs text-slate-500 dark:text-slate-500">
            <div>路径: {log.target}</div>
            <div>IP: {log.ip_address}</div>
          </div>
        </div>
      </div>
    </div>
  )

  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleString('zh-CN')
  }

  return (
    <AdminLayout title="操作日志" description="查看系统操作日志记录">
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
                label="操作类型"
                placeholder="输入操作类型"
                value={filters.action}
                onChange={(e) => setFilters({ ...filters, action: e.target.value })}
              />
              <Input
                label="操作目标"
                placeholder="输入操作目标"
                value={filters.target}
                onChange={(e) => setFilters({ ...filters, target: e.target.value })}
              />
            </div>
            <Button
              variant="primary"
              onClick={() => {
                setPage(1)
                fetchLogs()
              }}
              leftIcon={<Search className="w-4 h-4" />}
            >
              搜索
            </Button>
          </div>
        </Card>

        {/* 日志列表 */}
        <Card>
          <div className="p-4">
            {loading ? (
              <div className="text-center py-12">
                <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
                <p className="mt-4 text-slate-500 dark:text-slate-400">加载中...</p>
              </div>
            ) : logs.length === 0 ? (
              <div className="text-center py-12">
                <FileText className="w-16 h-16 mx-auto mb-4 text-slate-400 dark:text-slate-500" />
                <p className="text-slate-500 dark:text-slate-400">暂无操作日志</p>
              </div>
            ) : (
              <>
                <div className="space-y-3">
                  {logs.map((log) => renderLogItem(log))}
                </div>

                {/* 分页 */}
                {Math.ceil(total / pageSize) > 1 && (
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
                )}
              </>
            )}
          </div>
        </Card>
      </div>
    </AdminLayout>
  )
}

export default OperationLogs

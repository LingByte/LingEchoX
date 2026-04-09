import { useState, useEffect } from 'react'
import { Search, FileText, Calendar, User, Shield, AlertCircle, CheckCircle, Clock, Power, X, ExternalLink } from 'lucide-react'
import AdminLayout from '@/components/Layout/AdminLayout'
import Card from '@/components/UI/Card'
import Button from '@/components/UI/Button'
import Input from '@/components/UI/Input'
import { getFileCensorRecords, updateCensorConfig, type FileCensorRecord } from '@/services/adminApi'
import { useSiteConfig } from '@/contexts/SiteConfigContext'
import { showAlert } from '@/utils/notification'

const FileCensorRecords = () => {
  const { config: siteConfig, refresh: refreshSiteConfig } = useSiteConfig()
  const [records, setRecords] = useState<FileCensorRecord[]>([])
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [censorEnabled, setCensorEnabled] = useState(false)
  const [togglingCensor, setTogglingCensor] = useState(false)
  const [selectedRecord, setSelectedRecord] = useState<FileCensorRecord | null>(null)
  const [showDetailModal, setShowDetailModal] = useState(false)
  const [filters, setFilters] = useState({
    user_id: '',
    censor_type: '',
    provider: '',
    status: '',
    result: '',
  })

  useEffect(() => {
    // Initialize censor status from site config
    if (siteConfig?.CENSOR_ENABLED !== undefined) {
      setCensorEnabled(siteConfig.CENSOR_ENABLED)
    }
    fetchRecords()
  }, [page, filters, siteConfig])

  const toggleCensor = async () => {
    try {
      setTogglingCensor(true)
      const newStatus = !censorEnabled
      await updateCensorConfig({
        key: 'CENSOR_ENABLED',
        value: newStatus ? 'true' : 'false',
      })
      setCensorEnabled(newStatus)
      // Refresh site config to update the status globally
      await refreshSiteConfig()
      showAlert(newStatus ? '文件审核已启用' : '文件审核已禁用', 'success')
    } catch (error: any) {
      showAlert('更新审核状态失败', 'error', error?.msg || error?.message)
    } finally {
      setTogglingCensor(false)
    }
  }

  const fetchRecords = async () => {
    try {
      setLoading(true)
      const params: any = { page, page_size: pageSize }
      if (filters.user_id) params.user_id = parseInt(filters.user_id)
      if (filters.censor_type) params.censor_type = filters.censor_type
      if (filters.provider) params.provider = filters.provider
      if (filters.status) params.status = filters.status
      if (filters.result) params.result = filters.result
      
      const data = await getFileCensorRecords(params)
      setRecords(data.records)
      setTotal(data.total)
    } catch (error: any) {
      showAlert('获取审核记录失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const getStatusBadge = (status: string) => {
    const statusMap: Record<string, { bg: string; text: string; label: string }> = {
      pending: { bg: 'bg-yellow-100 dark:bg-yellow-900/20', text: 'text-yellow-700 dark:text-yellow-400', label: '待审核' },
      processing: { bg: 'bg-blue-100 dark:bg-blue-900/20', text: 'text-blue-700 dark:text-blue-400', label: '审核中' },
      completed: { bg: 'bg-green-100 dark:bg-green-900/20', text: 'text-green-700 dark:text-green-400', label: '已完成' },
      failed: { bg: 'bg-red-100 dark:bg-red-900/20', text: 'text-red-700 dark:text-red-400', label: '失败' },
    }
    const config = statusMap[status] || statusMap.pending
    return (
      <span className={`px-2 py-0.5 text-xs rounded ${config.bg} ${config.text}`}>
        {config.label}
      </span>
    )
  }

  const getResultBadge = (result: string) => {
    const resultMap: Record<string, { bg: string; text: string; label: string; icon: any }> = {
      pass: { bg: 'bg-green-100 dark:bg-green-900/20', text: 'text-green-700 dark:text-green-400', label: '通过', icon: CheckCircle },
      review: { bg: 'bg-orange-100 dark:bg-orange-900/20', text: 'text-orange-700 dark:text-orange-400', label: '需审核', icon: Clock },
      block: { bg: 'bg-red-100 dark:bg-red-900/20', text: 'text-red-700 dark:text-red-400', label: '违规', icon: AlertCircle },
    }
    const config = resultMap[result] || resultMap.pass
    const Icon = config.icon
    return (
      <span className={`px-2 py-0.5 text-xs rounded flex items-center gap-1 w-fit ${config.bg} ${config.text}`}>
        <Icon className="w-3 h-3" />
        {config.label}
      </span>
    )
  }

  const renderRecord = (record: FileCensorRecord) => (
    <div key={record.id} className="p-4 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800/50 transition-colors">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 space-y-2">
          <div className="flex items-center gap-2">
            <span className="font-semibold text-slate-900 dark:text-white truncate">{record.fileName}</span>
            {getStatusBadge(record.status)}
            {getResultBadge(record.result)}
          </div>
          <div className="flex items-center gap-4 text-sm text-slate-600 dark:text-slate-400 flex-wrap">
            <span className="flex items-center gap-1">
              <Shield className="w-4 h-4" />
              {record.censorType.toUpperCase()} - {record.provider}
            </span>
            <span className="flex items-center gap-1">
              <User className="w-4 h-4" />
              用户ID: {record.userId}
            </span>
            <span className="flex items-center gap-1">
              <Calendar className="w-4 h-4" />
              {formatDate(record.submittedAt)}
            </span>
          </div>
          <div className="text-xs text-slate-500 dark:text-slate-500 space-y-1">
            {record.label && <div>标签: {record.label}</div>}
            {record.suggestion && <div>建议: {record.suggestion}</div>}
            {record.score > 0 && <div>置信度: {(record.score * 100).toFixed(2)}%</div>}
            {record.processTime > 0 && <div>处理耗时: {record.processTime}ms</div>}
            {record.errorMessage && <div className="text-red-600 dark:text-red-400">错误: {record.errorMessage}</div>}
          </div>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            setSelectedRecord(record)
            setShowDetailModal(true)
          }}
        >
          查看详情
        </Button>
      </div>
    </div>
  )

  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleString('zh-CN')
  }

  return (
    <AdminLayout title="文件审核记录" description="查看文件内容审核记录">
      <div className="space-y-6">
        {/* 控制区域 */}
        <Card>
          <div className="p-4 flex items-center justify-between">
            <div>
              <h3 className="font-semibold text-slate-900 dark:text-white">文件审核状态</h3>
              <p className="text-sm text-slate-600 dark:text-slate-400 mt-1">
                当前状态: {censorEnabled ? '已启用' : '已禁用'}
              </p>
            </div>
            <Button
              variant={censorEnabled ? 'destructive' : 'primary'}
              onClick={toggleCensor}
              disabled={togglingCensor}
              leftIcon={<Power className="w-4 h-4" />}
            >
              {togglingCensor ? '更新中...' : censorEnabled ? '禁用审核' : '启用审核'}
            </Button>
          </div>
        </Card>

        {/* 筛选区域 */}
        <Card>
          <div className="p-4 space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
              <Input
                label="用户ID"
                placeholder="输入用户ID"
                value={filters.user_id}
                onChange={(e) => setFilters({ ...filters, user_id: e.target.value })}
              />
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">审核类型</label>
                <select
                  value={filters.censor_type}
                  onChange={(e) => setFilters({ ...filters, censor_type: e.target.value })}
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-800 text-slate-900 dark:text-white"
                >
                  <option value="">全部</option>
                  <option value="text">文本</option>
                  <option value="image">图片</option>
                  <option value="audio">音频</option>
                  <option value="video">视频</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">服务商</label>
                <select
                  value={filters.provider}
                  onChange={(e) => setFilters({ ...filters, provider: e.target.value })}
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-800 text-slate-900 dark:text-white"
                >
                  <option value="">全部</option>
                  <option value="qiniu">七牛云</option>
                  <option value="qcloud">腾讯云</option>
                  <option value="aliyun">阿里云</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">审核状态</label>
                <select
                  value={filters.status}
                  onChange={(e) => setFilters({ ...filters, status: e.target.value })}
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-800 text-slate-900 dark:text-white"
                >
                  <option value="">全部</option>
                  <option value="pending">待审核</option>
                  <option value="processing">审核中</option>
                  <option value="completed">已完成</option>
                  <option value="failed">失败</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">审核结果</label>
                <select
                  value={filters.result}
                  onChange={(e) => setFilters({ ...filters, result: e.target.value })}
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-800 text-slate-900 dark:text-white"
                >
                  <option value="">全部</option>
                  <option value="pass">通过</option>
                  <option value="review">需审核</option>
                  <option value="block">违规</option>
                </select>
              </div>
            </div>
            <Button
              variant="primary"
              onClick={() => {
                setPage(1)
                fetchRecords()
              }}
              leftIcon={<Search className="w-4 h-4" />}
            >
              搜索
            </Button>
          </div>
        </Card>

        {/* 记录列表 */}
        <Card>
          <div className="p-4">
            {loading ? (
              <div className="text-center py-12">
                <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
                <p className="mt-4 text-slate-500 dark:text-slate-400">加载中...</p>
              </div>
            ) : records.length === 0 ? (
              <div className="text-center py-12">
                <FileText className="w-16 h-16 mx-auto mb-4 text-slate-400 dark:text-slate-500" />
                <p className="text-slate-500 dark:text-slate-400">暂无审核记录</p>
              </div>
            ) : (
              <>
                <div className="space-y-3">
                  {records.map((record) => renderRecord(record))}
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

      {/* 详情模态框 */}
      {showDetailModal && selectedRecord && (
        <div className="fixed inset-0 bg-black/50 dark:bg-black/70 flex items-center justify-center z-50 p-4">
          <Card className="w-full max-w-2xl max-h-[90vh] overflow-y-auto">
            <div className="p-6">
              <div className="flex items-center justify-between mb-6">
                <h2 className="text-xl font-semibold text-slate-900 dark:text-white">审核记录详情</h2>
                <button
                  onClick={() => setShowDetailModal(false)}
                  className="text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200"
                >
                  <X className="w-5 h-5" />
                </button>
              </div>

              <div className="space-y-6">
                {/* 基本信息 */}
                <div>
                  <h3 className="font-semibold text-slate-900 dark:text-white mb-3">基本信息</h3>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">文件名</p>
                      <p className="text-sm font-medium text-slate-900 dark:text-white mt-1 break-all">{selectedRecord.fileName}</p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">用户ID</p>
                      <p className="text-sm font-medium text-slate-900 dark:text-white mt-1">{selectedRecord.userId}</p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">审核类型</p>
                      <p className="text-sm font-medium text-slate-900 dark:text-white mt-1">{selectedRecord.censorType.toUpperCase()}</p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">服务商</p>
                      <p className="text-sm font-medium text-slate-900 dark:text-white mt-1">{selectedRecord.provider}</p>
                    </div>
                  </div>
                </div>

                {/* 文件URL */}
                <div>
                  <h3 className="font-semibold text-slate-900 dark:text-white mb-3">文件URL</h3>
                  <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded flex items-center justify-between gap-2">
                    <p className="text-sm text-slate-600 dark:text-slate-400 break-all flex-1">{selectedRecord.fileUrl}</p>
                    <a
                      href={selectedRecord.fileUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="flex-shrink-0 text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
                    >
                      <ExternalLink className="w-4 h-4" />
                    </a>
                  </div>
                </div>

                {/* 审核结果 */}
                <div>
                  <h3 className="font-semibold text-slate-900 dark:text-white mb-3">审核结果</h3>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">状态</p>
                      <div className="mt-1">{getStatusBadge(selectedRecord.status)}</div>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">结果</p>
                      <div className="mt-1">{getResultBadge(selectedRecord.result)}</div>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">标签</p>
                      <p className="text-sm font-medium text-slate-900 dark:text-white mt-1">{selectedRecord.label || '-'}</p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">置信度</p>
                      <p className="text-sm font-medium text-slate-900 dark:text-white mt-1">
                        {selectedRecord.score > 0 ? `${(selectedRecord.score * 100).toFixed(2)}%` : '-'}
                      </p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">建议</p>
                      <p className="text-sm font-medium text-slate-900 dark:text-white mt-1">{selectedRecord.suggestion || '-'}</p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">处理耗时</p>
                      <p className="text-sm font-medium text-slate-900 dark:text-white mt-1">
                        {selectedRecord.processTime > 0 ? `${selectedRecord.processTime}ms` : '-'}
                      </p>
                    </div>
                  </div>
                </div>

                {/* 详细信息 */}
                {selectedRecord.details && (
                  <div>
                    <h3 className="font-semibold text-slate-900 dark:text-white mb-3">详细审核信息</h3>
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <pre className="text-xs text-slate-600 dark:text-slate-400 overflow-auto max-h-64">
                        {JSON.stringify(selectedRecord.details, null, 2)}
                      </pre>
                    </div>
                  </div>
                )}

                {/* 错误信息 */}
                {selectedRecord.errorMessage && (
                  <div>
                    <h3 className="font-semibold text-red-600 dark:text-red-400 mb-3">错误信息</h3>
                    <div className="bg-red-50 dark:bg-red-900/20 p-3 rounded border border-red-200 dark:border-red-800">
                      <p className="text-sm text-red-700 dark:text-red-300">{selectedRecord.errorMessage}</p>
                    </div>
                  </div>
                )}

                {/* 时间信息 */}
                <div>
                  <h3 className="font-semibold text-slate-900 dark:text-white mb-3">时间信息</h3>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">提交时间</p>
                      <p className="text-sm font-medium text-slate-900 dark:text-white mt-1">{formatDate(selectedRecord.submittedAt)}</p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-800/50 p-3 rounded">
                      <p className="text-xs text-slate-600 dark:text-slate-400">完成时间</p>
                      <p className="text-sm font-medium text-slate-900 dark:text-white mt-1">
                        {selectedRecord.completedAt ? formatDate(selectedRecord.completedAt) : '-'}
                      </p>
                    </div>
                  </div>
                </div>

                {/* 关闭按钮 */}
                <div className="flex justify-end gap-2 pt-4 border-t border-slate-200 dark:border-slate-700">
                  <Button
                    variant="ghost"
                    onClick={() => setShowDetailModal(false)}
                  >
                    关闭
                  </Button>
                </div>
              </div>
            </div>
          </Card>
        </div>
      )}
    </AdminLayout>
  )
}

export default FileCensorRecords

import { useState, useEffect } from 'react'
import { Key, Plus, Trash2, Copy, Download, AlertCircle, Ban, Unlock } from 'lucide-react'
import AdminLayout from '@/components/Layout/AdminLayout'
import Card from '@/components/UI/Card'
import Button from '@/components/UI/Button'
import Input from '@/components/UI/Input'
import Modal from '@/components/UI/Modal'
import ConfirmDialog from '@/components/UI/ConfirmDialog'
import {
  getAPIKeys,
  createAPIKey,
  deleteAPIKey,
  type APIKey,
  type CreateAPIKeyRequest,
  unbanAPIKey, banAPIKey
} from '@/services/adminApi'
import { showAlert } from '@/utils/notification'

const ApiKeys = () => {
  const [keys, setKeys] = useState<APIKey[]>([])
  const [loading, setLoading] = useState(false)
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<APIKey | null>(null)
  const [newKeyName, setNewKeyName] = useState('')
  const [createdKey, setCreatedKey] = useState<{ apiKey: string; apiSecret: string; name: string } | null>(null)

  useEffect(() => {
    fetchKeys()
  }, [])

  const fetchKeys = async () => {
    try {
      setLoading(true)
      const data = await getAPIKeys()
      setKeys(data)
    } catch (error: any) {
      showAlert('获取密钥列表失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async () => {
    if (!newKeyName.trim()) {
      showAlert('请输入密钥名称', 'error')
      return
    }

    try {
      setLoading(true)
      const data: CreateAPIKeyRequest = {
        name: newKeyName.trim(),
      }
      const result = await createAPIKey(data)
      setCreatedKey({
        apiKey: result.apiKey,
        apiSecret: result.apiSecret,
        name: result.name,
      })
      setNewKeyName('')
      setShowCreateModal(false)
      await fetchKeys()
    } catch (error: any) {
      showAlert('创建密钥失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return

    try {
      setLoading(true)
      await deleteAPIKey(deleteTarget.id)
      showAlert('删除密钥成功', 'success')
      setShowDeleteConfirm(false)
      setDeleteTarget(null)
      await fetchKeys()
    } catch (error: any) {
      showAlert('删除密钥失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const handleBan = async (key: APIKey) => {
    try {
      setLoading(true)
      await banAPIKey(key.id)
      showAlert('封禁密钥成功', 'success')
      await fetchKeys()
    } catch (error: any) {
      showAlert('封禁密钥失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const handleUnban = async (key: APIKey) => {
    try {
      setLoading(true)
      await unbanAPIKey(key.id)
      showAlert('解封密钥成功', 'success')
      await fetchKeys()
    } catch (error: any) {
      showAlert('解封密钥失败', 'error', error?.msg || error?.message)
    } finally {
      setLoading(false)
    }
  }

  const handleCopy = (text: string, label: string) => {
    navigator.clipboard.writeText(text).then(() => {
      showAlert(`${label}已复制到剪贴板`, 'success')
    }).catch(() => {
      showAlert('复制失败', 'error')
    })
  }

  const handleExport = () => {
    if (!createdKey) return

    const content = `API Key: ${createdKey.apiKey}\nAPI Secret: ${createdKey.apiSecret}\n名称: ${createdKey.name}\n创建时间: ${new Date().toLocaleString('zh-CN')}\n\n⚠️ 重要提示：\n- API Secret 仅在创建时显示一次，请妥善保管\n- 请勿将密钥泄露给他人\n- 建议将密钥保存在安全的地方`
    
    const blob = new Blob([content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `api-key-${createdKey.apiKey.substring(0, 10)}.txt`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    
    showAlert('密钥已导出', 'success')
  }

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return '-'
    return new Date(dateStr).toLocaleString('zh-CN')
  }

  return (
    <AdminLayout title="API 密钥管理" description="管理您的 API 密钥，用于访问 API 服务">
      <div className="space-y-6">
        {/* 创建密钥按钮 */}
        <div className="flex justify-between items-center">
          <Button
            variant="primary"
            onClick={() => setShowCreateModal(true)}
            leftIcon={<Plus className="w-4 h-4" />}
          >
            创建密钥
          </Button>
        </div>

        {/* 密钥列表 */}
        {loading && keys.length === 0 ? (
          <Card>
            <div className="p-2 text-center">
              <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
              <p className="mt-4 text-slate-500 dark:text-slate-400">加载中...</p>
            </div>
          </Card>
        ) : keys.length === 0 ? (
          <Card>
            <div className="p-2 text-center">
              <Key className="w-16 h-16 mx-auto mb-4 text-slate-400 dark:text-slate-500" />
              <p className="text-slate-500 dark:text-slate-400">
                还没有创建任何密钥，点击上方按钮创建第一个密钥
              </p>
            </div>
          </Card>
        ) : (
          <div className="grid gap-3">
            {keys.map((key) => (
              <Card key={key.id}>
                <div className="p-2">
                  <div className="flex items-center justify-between gap-3">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <Key className="w-5 h-5 text-blue-600 dark:text-blue-400 flex-shrink-0" />
                        <h3 className="text-sm font-semibold text-slate-900 dark:text-white truncate">
                          {key.name}
                        </h3>
                        {key.isBanned ? (
                          <span className="px-1.5 py-0.5 text-xs font-medium bg-red-100 dark:bg-red-900/20 text-red-700 dark:text-red-400 rounded flex-shrink-0">
                            已封禁
                          </span>
                        ) : key.isActive ? (
                          <span className="px-1.5 py-0.5 text-xs font-medium bg-green-100 dark:bg-green-900/20 text-green-700 dark:text-green-400 rounded flex-shrink-0">
                            激活
                          </span>
                        ) : (
                          <span className="px-1.5 py-0.5 text-xs font-medium bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-400 rounded flex-shrink-0">
                            已禁用
                          </span>
                        )}
                      </div>
                      
                      <div className="space-y-1.5 mt-1">
                        <div>
                          <label className="text-xs text-slate-500 dark:text-slate-400 mb-0.5 block">
                            API Key
                          </label>
                          <div className="flex items-center gap-1.5">
                            <code className="flex-1 px-2 py-1 bg-slate-100 dark:bg-slate-800 rounded text-xs font-mono text-slate-900 dark:text-white truncate">
                              {key.apiKey}
                            </code>
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => handleCopy(key.apiKey, 'API Key')}
                              className="p-1.5 h-auto flex-shrink-0"
                            >
                              <Copy className="w-3.5 h-3.5" />
                            </Button>
                          </div>
                        </div>

                        <div>
                          <label className="text-xs text-slate-500 dark:text-slate-400 mb-0.5 block">
                            API Secret
                          </label>
                          <div className="px-2 py-1 bg-slate-100 dark:bg-slate-800 rounded">
                            <p className="text-xs text-slate-500 dark:text-slate-400 italic">
                              API Secret 仅在创建时显示一次，之后无法查看
                            </p>
                          </div>
                        </div>
                      </div>

                      <div className="mt-2 flex items-center gap-3 text-xs text-slate-500 dark:text-slate-400">
                        <span className="truncate">创建: {formatDate(key.createdAt)}</span>
                        {key.lastUsedAt && (
                          <span className="truncate">使用: {formatDate(key.lastUsedAt)}</span>
                        )}
                        {key.expiresAt && (
                          <span className="truncate">过期: {formatDate(key.expiresAt)}</span>
                        )}
                      </div>
                    </div>

                    <div className="flex items-center gap-1 flex-shrink-0">
                      {key.isBanned ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => handleUnban(key)}
                          className="text-green-600 dark:text-green-400 hover:text-green-700 dark:hover:text-green-300 p-1.5 h-auto"
                          title="解封"
                        >
                          <Unlock className="w-4 h-4" />
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => handleBan(key)}
                          className="text-orange-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300 p-1.5 h-auto"
                          title="封禁"
                        >
                          <Ban className="w-4 h-4" />
                        </Button>
                      )}
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => {
                          setDeleteTarget(key)
                          setShowDeleteConfirm(true)
                        }}
                        className="text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300 p-1.5 h-auto"
                        title="删除"
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    </div>
                  </div>
                </div>
              </Card>
            ))}
          </div>
        )}

        {/* 创建密钥弹窗 */}
        <Modal
          isOpen={showCreateModal}
          onClose={() => {
            setShowCreateModal(false)
            setNewKeyName('')
          }}
          title="创建 API 密钥"
          size="md"
        >
          <div className="space-y-4">
            <Input
              label="密钥名称"
              placeholder="请输入密钥名称（如：生产环境、测试环境）"
              value={newKeyName}
              onChange={(e) => setNewKeyName(e.target.value)}
              required
            />
            <div className="flex justify-end gap-2 pt-4">
              <Button
                variant="ghost"
                onClick={() => {
                  setShowCreateModal(false)
                  setNewKeyName('')
                }}
              >
                取消
              </Button>
              <Button
                variant="primary"
                onClick={handleCreate}
                loading={loading}
              >
                创建
              </Button>
            </div>
          </div>
        </Modal>

        {/* 密钥创建成功弹窗 */}
        <Modal
          isOpen={createdKey !== null}
          onClose={() => setCreatedKey(null)}
          title="密钥创建成功"
          size="lg"
          closeOnOverlayClick={false}
        >
          {createdKey && (
            <div className="space-y-4">
              <div className="p-4 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
                <div className="flex items-start gap-2">
                  <AlertCircle className="w-5 h-5 text-yellow-600 dark:text-yellow-400 mt-0.5 flex-shrink-0" />
                  <div className="flex-1">
                    <p className="text-sm font-medium text-yellow-800 dark:text-yellow-300 mb-1">
                      重要提示
                    </p>
                    <p className="text-xs text-yellow-700 dark:text-yellow-400">
                      API Secret 仅在创建时显示一次，请立即保存。关闭此窗口后将无法再次查看。
                    </p>
                  </div>
                </div>
              </div>

              <div className="space-y-3">
                <div>
                  <label className="text-xs text-slate-500 dark:text-slate-400 mb-1 block">
                    密钥名称
                  </label>
                  <div className="px-3 py-2 bg-slate-100 dark:bg-slate-800 rounded-lg">
                    <p className="text-sm font-medium text-slate-900 dark:text-white">
                      {createdKey.name}
                    </p>
                  </div>
                </div>

                <div>
                  <label className="text-xs text-slate-500 dark:text-slate-400 mb-1 block">
                    API Key
                  </label>
                  <div className="flex items-center gap-2">
                    <code className="flex-1 px-3 py-2 bg-slate-100 dark:bg-slate-800 rounded-lg text-sm font-mono text-slate-900 dark:text-white break-all">
                      {createdKey.apiKey}
                    </code>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => handleCopy(createdKey.apiKey, 'API Key')}
                      leftIcon={<Copy className="w-4 h-4" />}
                    >
                      复制
                    </Button>
                  </div>
                </div>

                <div>
                  <label className="text-xs text-slate-500 dark:text-slate-400 mb-1 block">
                    API Secret
                  </label>
                  <div className="flex items-center gap-2">
                    <code className="flex-1 px-3 py-2 bg-slate-100 dark:bg-slate-800 rounded-lg text-sm font-mono text-slate-900 dark:text-white break-all">
                      {createdKey.apiSecret}
                    </code>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => handleCopy(createdKey.apiSecret, 'API Secret')}
                      leftIcon={<Copy className="w-4 h-4" />}
                    >
                      复制
                    </Button>
                  </div>
                </div>
              </div>

              <div className="flex justify-end gap-2 pt-4 border-t border-slate-200 dark:border-slate-700">
                <Button
                  variant="ghost"
                  onClick={handleExport}
                  leftIcon={<Download className="w-4 h-4" />}
                >
                  导出密钥
                </Button>
                <Button
                  variant="primary"
                  onClick={() => setCreatedKey(null)}
                >
                  我已保存
                </Button>
              </div>
            </div>
          )}
        </Modal>

        {/* 删除确认弹窗 */}
        <ConfirmDialog
          isOpen={showDeleteConfirm}
          onClose={() => {
            setShowDeleteConfirm(false)
            setDeleteTarget(null)
          }}
          onConfirm={handleDelete}
          title="删除密钥"
          message={`确定要删除密钥 "${deleteTarget?.name}" 吗？此操作不可恢复。`}
          confirmText="删除"
          cancelText="取消"
          variant="danger"
        />
      </div>
    </AdminLayout>
  )
}

export default ApiKeys

import { useState, useRef, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Upload, X, FileText, Clock, CheckCircle2, AlertCircle, Trash2, Play, Pause } from 'lucide-react'
import AdminLayout from '@/components/Layout/AdminLayout'
import Card from '@/components/UI/Card'
import Button from '@/components/UI/Button'
import { showAlert } from '@/utils/notification'
import { post } from '@/utils/request'
import { getApiBaseURL } from '@/config/apiConfig'

interface UploadLog {
  id: string
  timestamp: Date
  level: 'info' | 'success' | 'warning' | 'error'
  message: string
  data?: any
}

interface UploadTask {
  id: string
  file: File
  status: 'pending' | 'uploading' | 'success' | 'error' | 'paused'
  progress: number
  speed: number // bytes per second
  startTime?: number
  endTime?: number
  duration?: number // 上传耗时（毫秒）
  uploadedBytes: number
  totalBytes: number
  logs: UploadLog[]
  error?: string
  result?: any
}

const DebugUpload = () => {
  const [tasks, setTasks] = useState<UploadTask[]>([])
  const [selectedTask, setSelectedTask] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const abortControllersRef = useRef<Map<string, AbortController>>(new Map())
  const [compressEnabled, setCompressEnabled] = useState(false)
  const [compressQuality, setCompressQuality] = useState(100)
  const [watermarkEnabled, setWatermarkEnabled] = useState(false)
  const [watermarkText, setWatermarkText] = useState('')
  const [watermarkPosition, setWatermarkPosition] = useState('bottom-right')

  const addLog = (taskId: string, level: UploadLog['level'], message: string, data?: any) => {
    setTasks(prev => prev.map(task => {
      if (task.id === taskId) {
        return {
          ...task,
          logs: [
            ...task.logs,
            {
              id: `${Date.now()}-${Math.random()}`,
              timestamp: new Date(),
              level,
              message,
              data
            }
          ]
        }
      }
      return task
    }))
  }

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`
  }

  const formatDuration = (ms: number) => {
    if (ms < 1000) return `${ms}ms`
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
    const minutes = Math.floor(ms / 60000)
    const seconds = Math.floor((ms % 60000) / 1000)
    return `${minutes}m ${seconds}s`
  }

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files
    if (!files || files.length === 0) return

    Array.from(files).forEach(file => {
      const taskId = `${Date.now()}-${Math.random()}`
      const newTask: UploadTask = {
        id: taskId,
        file,
        status: 'pending',
        progress: 0,
        speed: 0,
        uploadedBytes: 0,
        totalBytes: file.size,
        logs: [{
          id: `${Date.now()}-${Math.random()}`,
          timestamp: new Date(),
          level: 'info',
          message: `文件已选择: ${file.name} (${formatBytes(file.size)})`
        }]
      }
      setTasks(prev => [...prev, newTask])
    })

    // 重置文件输入
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }

  const uploadFile = async (taskId: string) => {
    const task = tasks.find(t => t.id === taskId)
    if (!task) return

    // 创建 AbortController
    const abortController = new AbortController()
    abortControllersRef.current.set(taskId, abortController)

    setTasks(prev => prev.map(t => {
      if (t.id === taskId) {
        return {
          ...t,
          status: 'uploading' as const,
          startTime: Date.now()
        }
      }
      return t
    }))

    addLog(taskId, 'info', '开始上传...', { filename: task.file.name, size: task.totalBytes })

    try {
      const formData = new FormData()
      formData.append('file', task.file)
      formData.append('key', `debug/${Date.now()}_${task.file.name}`)
      formData.append('bucket', 'default')
      
      // 添加压缩参数
      if (compressEnabled) {
        formData.append('compress', 'true')
        formData.append('quality', compressQuality.toString())
      }
      
      // 添加水印参数
      if (watermarkEnabled && watermarkText) {
        formData.append('watermark', 'true')
        formData.append('watermarkText', watermarkText)
        formData.append('watermarkPosition', watermarkPosition)
      }

      const xhr = new XMLHttpRequest()

      // 监听上传进度
      xhr.upload.addEventListener('progress', (e) => {
        if (e.lengthComputable) {
          const progress = Math.round((e.loaded / e.total) * 100)
          const uploadedBytes = e.loaded
          const elapsed = Date.now() - (task.startTime || Date.now())
          const speed = elapsed > 0 ? (uploadedBytes / elapsed) * 1000 : 0

          setTasks(prev => prev.map(t => {
            if (t.id === taskId) {
              return {
                ...t,
                progress,
                uploadedBytes,
                speed
              }
            }
            return t
          }))

          addLog(taskId, 'info', `上传进度: ${progress}% (${formatBytes(uploadedBytes)} / ${formatBytes(e.total)})`, {
            progress,
            uploaded: uploadedBytes,
            total: e.total,
            speed: formatBytes(speed) + '/s'
          })
        }
      })

      // 监听响应
      xhr.addEventListener('load', () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            const response = JSON.parse(xhr.responseText)
            // 检查响应格式：可能是 {code, msg, data} 或直接是 data
            const resultData = response.data || response
            
            // 使用 setTasks 的回调形式获取最新的任务状态
            setTasks(prev => {
              const currentTask = prev.find(t => t.id === taskId)
              if (!currentTask) return prev
              
              // 计算上传耗时和最终速度
              const endTime = Date.now()
              const duration = currentTask.startTime ? endTime - currentTask.startTime : 0
              
              // 从后端返回的 metrics 中提取信息，或计算最终速度
              const metrics = resultData?.metrics || {}
              let finalSpeed = 0
              if (metrics.upload_speed_mbps) {
                // 后端返回的是 MB/s，转换为 bytes/s
                finalSpeed = parseFloat(metrics.upload_speed_mbps) * 1024 * 1024
              } else if (duration > 0) {
                // 计算平均速度：总大小 / 耗时（毫秒）* 1000（转换为秒）
                finalSpeed = (currentTask.totalBytes / duration) * 1000
              }
              
              // 添加成功日志，包含时间和速度信息
              const durationText = duration > 0 ? `${(duration / 1000).toFixed(2)}秒` : '未知'
              const speedText = finalSpeed > 0 ? formatBytes(finalSpeed) + '/s' : '未知'
              addLog(taskId, 'success', `上传成功！耗时: ${durationText}, 平均速度: ${speedText}`, {
                ...resultData,
                duration,
                durationText,
                speed: finalSpeed,
                speedText
              })
              
              return prev.map(t => {
                if (t.id === taskId) {
                  return {
                    ...t,
                    status: 'success' as const,
                    progress: 100,
                    uploadedBytes: t.totalBytes,
                    endTime,
                    duration,
                    speed: finalSpeed, // 使用最终计算的速度
                    result: resultData
                  }
                }
                return t
              })
            })
            
            showAlert('上传成功', 'success')
          } catch (error) {
            console.error('解析响应失败:', error, xhr.responseText)
            throw new Error('解析响应失败')
          }
        } else {
          throw new Error(`HTTP ${xhr.status}: ${xhr.statusText}`)
        }
      })

      xhr.addEventListener('error', () => {
        throw new Error('网络错误')
      })

      xhr.addEventListener('abort', () => {
        setTasks(prev => prev.map(t => {
          if (t.id === taskId) {
            return {
              ...t,
              status: 'paused' as const
            }
          }
          return t
        }))
        addLog(taskId, 'warning', '上传已暂停')
      })

      // 发送请求
      xhr.open('POST', `${getApiBaseURL()}/debug/upload`)
      
      // 添加认证token到请求头
      const token = localStorage.getItem('auth_token')
      if (token) {
        xhr.setRequestHeader('Authorization', `Bearer ${token}`)
      }
      
      xhr.send(formData)

      // 等待上传完成或取消
      await new Promise<void>((resolve, reject) => {
        xhr.addEventListener('loadend', () => {
          if (xhr.status >= 200 && xhr.status < 300) {
            resolve()
          } else {
            reject(new Error(`HTTP ${xhr.status}`))
          }
        })
        xhr.addEventListener('error', () => reject(new Error('Network error')))
        xhr.addEventListener('abort', () => resolve())
      })

    } catch (error: any) {
      setTasks(prev => prev.map(t => {
        if (t.id === taskId) {
          return {
            ...t,
            status: 'error' as const,
            error: error.message || '上传失败'
          }
        }
        return t
      }))
      addLog(taskId, 'error', `上传失败: ${error.message || '未知错误'}`, { error: error.message })
      showAlert(`上传失败: ${error.message}`, 'error')
    } finally {
      abortControllersRef.current.delete(taskId)
    }
  }

  const pauseUpload = (taskId: string) => {
    const controller = abortControllersRef.current.get(taskId)
    if (controller) {
      controller.abort()
    }
  }

  const resumeUpload = (taskId: string) => {
    uploadFile(taskId)
  }

  const removeTask = (taskId: string) => {
    pauseUpload(taskId)
    setTasks(prev => prev.filter(t => t.id !== taskId))
    if (selectedTask === taskId) {
      setSelectedTask(null)
    }
  }

  const selectedTaskData = tasks.find(t => t.id === selectedTask)

  return (
    <AdminLayout title="上传调试工具" description="调试文件上传接口，查看详细日志和上传进度">
      <div className="space-y-6">
        {/* 上传区域 */}
        <Card>
          <div className="p-6">
            <div className="flex items-center justify-between mb-4">
              <div>
                <h3 className="text-lg font-semibold text-slate-900 dark:text-white">文件上传</h3>
                <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
                  选择文件进行上传调试，查看详细的上传日志和进度
                </p>
              </div>
              <Button
                variant="primary"
                onClick={() => fileInputRef.current?.click()}
                leftIcon={<Upload className="w-4 h-4" />}
              >
                选择文件
              </Button>
            </div>
            
            {/* 图片处理选项 */}
            <div className="mt-4 p-4 border border-slate-200 dark:border-slate-700 rounded-lg space-y-4 bg-slate-50 dark:bg-slate-800/50">
              <h4 className="text-sm font-semibold text-slate-900 dark:text-white">
                图片处理选项（仅对图片文件生效）
              </h4>
              
              {/* 压缩选项 */}
              <div className="flex items-center gap-3 p-3 border border-slate-200 dark:border-slate-700 rounded-lg bg-white dark:bg-slate-800">
                <input
                  type="checkbox"
                  id="compress-enabled"
                  checked={compressEnabled}
                  onChange={(e) => setCompressEnabled(e.target.checked)}
                  className="w-4 h-4"
                />
                <label htmlFor="compress-enabled" className="text-sm text-slate-700 dark:text-slate-300 cursor-pointer flex-1">
                  压缩图片
                </label>
                {compressEnabled && (
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-slate-500">质量:</span>
                    <input
                      type="range"
                      min="1"
                      max="100"
                      value={compressQuality}
                      onChange={(e) => setCompressQuality(parseInt(e.target.value))}
                      className="w-24"
                    />
                    <span className="text-xs text-slate-500 w-8">{compressQuality}</span>
                  </div>
                )}
              </div>
              
              {/* 水印选项 */}
              <div className="p-3 border border-slate-200 dark:border-slate-700 rounded-lg bg-white dark:bg-slate-800 space-y-3">
                <div className="flex items-center gap-3">
                  <input
                    type="checkbox"
                    id="watermark-enabled"
                    checked={watermarkEnabled}
                    onChange={(e) => setWatermarkEnabled(e.target.checked)}
                    className="w-4 h-4"
                  />
                  <label htmlFor="watermark-enabled" className="text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
                    添加水印
                  </label>
                </div>
                {watermarkEnabled && (
                  <div className="ml-7 space-y-2">
                    <div>
                      <label className="block text-xs text-slate-600 dark:text-slate-400 mb-1">
                        水印文字
                      </label>
                      <input
                        type="text"
                        value={watermarkText}
                        onChange={(e) => setWatermarkText(e.target.value)}
                        placeholder="请输入水印文字"
                        className="w-full px-3 py-2 text-sm border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-800 text-slate-900 dark:text-white"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-slate-600 dark:text-slate-400 mb-1">
                        位置
                      </label>
                      <select
                        value={watermarkPosition}
                        onChange={(e) => setWatermarkPosition(e.target.value)}
                        className="w-full px-3 py-2 text-sm border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-800 text-slate-900 dark:text-white"
                      >
                        <option value="top-left">左上</option>
                        <option value="top-right">右上</option>
                        <option value="bottom-left">左下</option>
                        <option value="bottom-right">右下</option>
                        <option value="center">居中</option>
                      </select>
                    </div>
                  </div>
                )}
              </div>
            </div>
            
            <input
              ref={fileInputRef}
              type="file"
              multiple
              className="hidden"
              onChange={handleFileSelect}
            />
          </div>
        </Card>

        {/* 任务列表 */}
        {tasks.length > 0 && (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* 左侧：任务列表 */}
            <Card>
              <div className="p-6">
                <h3 className="text-lg font-semibold text-slate-900 dark:text-white mb-4">
                  上传任务 ({tasks.length})
                </h3>
                <div className="space-y-3">
                  {tasks.map(task => (
                    <motion.div
                      key={task.id}
                      initial={{ opacity: 0, y: 10 }}
                      animate={{ opacity: 1, y: 0 }}
                      className={`
                        p-4 rounded-lg border cursor-pointer transition-all
                        ${selectedTask === task.id
                          ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20'
                          : 'border-slate-200 dark:border-slate-700 hover:border-slate-300 dark:hover:border-slate-600'
                        }
                      `}
                      onClick={() => setSelectedTask(task.id)}
                    >
                      <div className="flex items-start justify-between">
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 mb-2">
                            <FileText className="w-4 h-4 text-slate-500 dark:text-slate-400 flex-shrink-0" />
                            <p className="text-sm font-medium text-slate-900 dark:text-white truncate">
                              {task.file.name}
                            </p>
                          </div>
                          <p className="text-xs text-slate-500 dark:text-slate-400 mb-2">
                            {formatBytes(task.totalBytes)}
                          </p>
                          {/* 进度条 */}
                          <div className="w-full bg-slate-200 dark:bg-slate-700 rounded-full h-2 mb-2">
                            <div
                              className={`
                                h-2 rounded-full transition-all
                                ${task.status === 'success' ? 'bg-green-500' :
                                  task.status === 'error' ? 'bg-red-500' :
                                  task.status === 'uploading' ? 'bg-blue-500' :
                                  'bg-slate-400'
                                }
                              `}
                              style={{ width: `${task.progress}%` }}
                            />
                          </div>
                          <div className="flex items-center justify-between text-xs">
                            <span className={`
                              ${task.status === 'success' ? 'text-green-600 dark:text-green-400' :
                                task.status === 'error' ? 'text-red-600 dark:text-red-400' :
                                task.status === 'uploading' ? 'text-blue-600 dark:text-blue-400' :
                                'text-slate-500 dark:text-slate-400'
                              }
                            `}>
                              {task.status === 'pending' && '等待中'}
                              {task.status === 'uploading' && `上传中 ${task.progress}%`}
                              {task.status === 'success' && '完成'}
                              {task.status === 'error' && '失败'}
                              {task.status === 'paused' && '已暂停'}
                            </span>
                            {(task.status === 'uploading' || task.status === 'success') && task.speed > 0 && (
                              <span className="text-slate-500 dark:text-slate-400">
                                {formatBytes(task.speed)}/s
                              </span>
                            )}
                            {task.status === 'success' && task.duration !== undefined && task.duration > 0 && (
                              <span className="text-slate-500 dark:text-slate-400 ml-2">
                                · {(task.duration / 1000).toFixed(2)}s
                              </span>
                            )}
                          </div>
                        </div>
                        <div className="flex items-center gap-2 ml-2">
                          {task.status === 'pending' && (
                            <Button
                              size="sm"
                              variant="primary"
                              onClick={(e) => {
                                e.stopPropagation()
                                uploadFile(task.id)
                              }}
                              leftIcon={<Play className="w-3 h-3" />}
                            >
                              开始
                            </Button>
                          )}
                          {task.status === 'uploading' && (
                            <Button
                              size="sm"
                              variant="secondary"
                              onClick={(e) => {
                                e.stopPropagation()
                                pauseUpload(task.id)
                              }}
                              leftIcon={<Pause className="w-3 h-3" />}
                            >
                              暂停
                            </Button>
                          )}
                          {task.status === 'paused' && (
                            <Button
                              size="sm"
                              variant="primary"
                              onClick={(e) => {
                                e.stopPropagation()
                                resumeUpload(task.id)
                              }}
                              leftIcon={<Play className="w-3 h-3" />}
                            >
                              继续
                            </Button>
                          )}
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={(e) => {
                              e.stopPropagation()
                              removeTask(task.id)
                            }}
                            leftIcon={<Trash2 className="w-3 h-3" />}
                          >
                            删除
                          </Button>
                        </div>
                      </div>
                    </motion.div>
                  ))}
                </div>
              </div>
            </Card>

            {/* 右侧：日志详情 */}
            <Card>
              <div className="p-6">
                <h3 className="text-lg font-semibold text-slate-900 dark:text-white mb-4">
                  上传日志
                </h3>
                {selectedTaskData ? (
                  <div className="space-y-4">
                    <div className="p-4 bg-slate-50 dark:bg-slate-800 rounded-lg">
                      <div className="grid grid-cols-2 gap-4 text-sm">
                        <div>
                          <p className="text-slate-500 dark:text-slate-400">文件名</p>
                          <p className="font-medium text-slate-900 dark:text-white mt-1">
                            {selectedTaskData.file.name}
                          </p>
                        </div>
                        <div>
                          <p className="text-slate-500 dark:text-slate-400">文件大小</p>
                          <p className="font-medium text-slate-900 dark:text-white mt-1">
                            {formatBytes(selectedTaskData.totalBytes)}
                          </p>
                        </div>
                        <div>
                          <p className="text-slate-500 dark:text-slate-400">上传进度</p>
                          <p className="font-medium text-slate-900 dark:text-white mt-1">
                            {selectedTaskData.progress}%
                          </p>
                        </div>
                        <div>
                          <p className="text-slate-500 dark:text-slate-400">上传速度</p>
                          <p className="font-medium text-slate-900 dark:text-white mt-1">
                            {selectedTaskData.speed > 0 ? formatBytes(selectedTaskData.speed) + '/s' : '-'}
                          </p>
                        </div>
                        {selectedTaskData.duration !== undefined && (
                          <div>
                            <p className="text-slate-500 dark:text-slate-400">消耗时间</p>
                            <p className="font-medium text-slate-900 dark:text-white mt-1">
                              {selectedTaskData.duration > 0 
                                ? `${(selectedTaskData.duration / 1000).toFixed(2)}秒`
                                : '-'}
                            </p>
                          </div>
                        )}
                        {selectedTaskData.result?.metrics && (
                          <div>
                            <p className="text-slate-500 dark:text-slate-400">后端统计</p>
                            <p className="font-medium text-slate-900 dark:text-white mt-1 text-xs">
                              {selectedTaskData.result.metrics.upload_duration || '-'}
                            </p>
                          </div>
                        )}
                      </div>
                    </div>

                    {/* 日志列表 */}
                    <div className="space-y-2 max-h-96 overflow-y-auto">
                      {selectedTaskData.logs.map(log => (
                        <div
                          key={log.id}
                          className={`
                            p-3 rounded-lg text-sm border
                            ${log.level === 'success' ? 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800' :
                              log.level === 'error' ? 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800' :
                              log.level === 'warning' ? 'bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800' :
                              'bg-slate-50 dark:bg-slate-800 border-slate-200 dark:border-slate-700'
                            }
                          `}
                        >
                          <div className="flex items-start gap-2">
                            {log.level === 'success' && <CheckCircle2 className="w-4 h-4 text-green-600 dark:text-green-400 flex-shrink-0 mt-0.5" />}
                            {log.level === 'error' && <AlertCircle className="w-4 h-4 text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" />}
                            {log.level === 'warning' && <AlertCircle className="w-4 h-4 text-yellow-600 dark:text-yellow-400 flex-shrink-0 mt-0.5" />}
                            {log.level === 'info' && <Clock className="w-4 h-4 text-blue-600 dark:text-blue-400 flex-shrink-0 mt-0.5" />}
                            <div className="flex-1 min-w-0">
                              <div className="flex items-center gap-2 mb-1">
                                <span className="text-xs text-slate-500 dark:text-slate-400">
                                  {log.timestamp.toLocaleTimeString()}
                                </span>
                                <span className={`
                                  text-xs font-medium px-2 py-0.5 rounded
                                  ${log.level === 'success' ? 'bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300' :
                                    log.level === 'error' ? 'bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300' :
                                    log.level === 'warning' ? 'bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300' :
                                    'bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300'
                                  }
                                `}>
                                  {log.level.toUpperCase()}
                                </span>
                              </div>
                              <p className="text-slate-900 dark:text-white">
                                {log.message}
                              </p>
                              {log.data && (
                                <pre className="mt-2 text-xs bg-slate-100 dark:bg-slate-900 p-2 rounded overflow-x-auto">
                                  {JSON.stringify(log.data, null, 2)}
                                </pre>
                              )}
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>

                    {/* 结果展示 */}
                    {selectedTaskData.result && (
                      <div className="p-4 bg-green-50 dark:bg-green-900/20 rounded-lg border border-green-200 dark:border-green-800">
                        <p className="text-sm font-medium text-green-900 dark:text-green-100 mb-2">
                          上传结果
                        </p>
                        <pre className="text-xs bg-white dark:bg-slate-900 p-3 rounded overflow-x-auto">
                          {JSON.stringify(selectedTaskData.result, null, 2)}
                        </pre>
                      </div>
                    )}

                    {/* 错误信息 */}
                    {selectedTaskData.error && (
                      <div className="p-4 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800">
                        <p className="text-sm font-medium text-red-900 dark:text-red-100 mb-2">
                          错误信息
                        </p>
                        <p className="text-sm text-red-700 dark:text-red-300">
                          {selectedTaskData.error}
                        </p>
                      </div>
                    )}
                  </div>
                ) : (
                  <div className="text-center py-12 text-slate-500 dark:text-slate-400">
                    <FileText className="w-12 h-12 mx-auto mb-4 opacity-50" />
                    <p>请选择一个任务查看详细日志</p>
                  </div>
                )}
              </div>
            </Card>
          </div>
        )}

        {tasks.length === 0 && (
          <Card>
            <div className="p-12 text-center">
              <Upload className="w-16 h-16 mx-auto mb-4 text-slate-400 dark:text-slate-500" />
              <p className="text-slate-500 dark:text-slate-400">
                还没有上传任务，点击上方按钮选择文件开始上传
              </p>
            </div>
          </Card>
        )}
      </div>
    </AdminLayout>
  )
}

export default DebugUpload

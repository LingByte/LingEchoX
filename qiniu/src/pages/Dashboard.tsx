import { motion } from 'framer-motion'
import { useMemo, useEffect, useState, useRef } from 'react'
import { Activity, ArrowUpRight, ArrowDownRight, Network, Clock, HardDrive, AlertCircle, CheckCircle2, Upload, Download, Zap, Calendar } from 'lucide-react'
import AdminLayout from '@/components/Layout/AdminLayout'
import Card from '@/components/UI/Card'
import Button from '@/components/UI/Button'
import { cn } from '@/utils/cn'
import ReactECharts from 'echarts-for-react'
import { useThemeStore } from '@/stores/themeStore'
import { getDashboardStats, DashboardStats } from '@/services/adminApi'

const StatCard = ({ 
  title, 
  value, 
  change, 
  trend, 
  icon: Icon 
}: { 
  title: string
  value: string | number
  change?: string
  trend?: 'up' | 'down'
  icon: React.ComponentType<{ className?: string }>
}) => (
  <motion.div
    initial={{ opacity: 0, y: 20 }}
    animate={{ opacity: 1, y: 0 }}
    className="bg-white dark:bg-slate-900 rounded-xl p-6 border border-slate-200 dark:border-slate-800 shadow-sm hover:shadow-md transition-shadow"
  >
    <div className="flex items-center justify-between">
      <div className="flex-1">
        <p className="text-sm font-medium text-slate-600 dark:text-slate-400 mb-1">
          {title}
        </p>
        <p className="text-2xl font-bold text-slate-900 dark:text-white">
          {value}
        </p>
        {change && (
          <div className={cn(
            "flex items-center gap-1 mt-2 text-sm",
            trend === 'up' ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'
          )}>
            {trend === 'up' ? (
              <ArrowUpRight className="w-4 h-4" />
            ) : (
              <ArrowDownRight className="w-4 h-4" />
            )}
            <span>{change}</span>
          </div>
        )}
      </div>
      <div className="w-12 h-12 rounded-lg bg-blue-100 dark:bg-blue-900/20 flex items-center justify-center">
        <Icon className="w-6 h-6 text-blue-600 dark:text-blue-400" />
      </div>
    </div>
  </motion.div>
)

const Dashboard = () => {
  const { isDark } = useThemeStore()
  const [statsData, setStatsData] = useState<DashboardStats | null>(null)
  const [rangeType, setRangeType] = useState<'today' | 'yesterday' | 'week' | 'month' | 'custom'>('week')
  const [customStartDate, setCustomStartDate] = useState('')
  const [customEndDate, setCustomEndDate] = useState('')
  const chartRef = useRef<any>(null)
  
  // 获取今天的日期字符串
  const today = new Date().toISOString().split('T')[0]

  const fetchStats = async (range: string, startDate?: string, endDate?: string) => {
    try {
      const params: any = { range }
      if (range === 'custom' && startDate && endDate) {
        params.start_date = startDate
        params.end_date = endDate
      }
      const data = await getDashboardStats(params)
      setStatsData(data)
    } catch (error) {
      console.error('获取统计数据失败:', error)
    }
  }

  useEffect(() => {
    fetchStats(rangeType)
  }, [rangeType])

  useEffect(() => {
    // 监听窗口 resize 事件，重新渲染图表
    const handleResize = () => {
      if (chartRef.current) {
        chartRef.current.getEchartsInstance().resize()
      }
    }

    window.addEventListener('resize', handleResize)
    // 初次加载时延迟触发 resize，确保容器尺寸已确定
    const timer = setTimeout(() => {
      handleResize()
    }, 100)

    return () => {
      window.removeEventListener('resize', handleResize)
      clearTimeout(timer)
    }
  }, [])

  const handleCustomDateSubmit = () => {
    if (customStartDate && customEndDate) {
      setRangeType('custom')
      fetchStats('custom', customStartDate, customEndDate)
    }
  }

  // 深色模式下的文本颜色
  const textColor = isDark ? '#e2e8f0' : '#64748b'
  const axisLineColor = isDark ? '#475569' : '#e2e8f0'
  const splitLineColor = isDark ? '#334155' : '#f1f5f9'

  // 格式化带宽大小
  const formatBandwidth = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
  }

  // 格式化存储大小
  const formatStorage = (bytes: number): string => {
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
  }

  const stats = useMemo(() => {
    if (!statsData?.summary) {
      return [
        { title: 'API调用次数', value: '0', change: undefined, trend: undefined, icon: Activity },
        { title: '带宽消耗', value: '0 B', change: undefined, trend: undefined, icon: Network },
        { title: '平均延迟', value: '0 ms', change: undefined, trend: undefined, icon: Clock },
        { title: '存储使用', value: '0 GB', change: undefined, trend: undefined, icon: HardDrive },
        { title: '错误率', value: '0%', change: undefined, trend: undefined, icon: AlertCircle },
        { title: '成功率', value: '100%', change: undefined, trend: undefined, icon: CheckCircle2 },
        { title: '文件上传', value: '0', change: undefined, trend: undefined, icon: Upload },
        { title: '文件下载', value: '0', change: undefined, trend: undefined, icon: Download },
        { title: '并发请求', value: '0', change: undefined, trend: undefined, icon: Zap },
      ]
    }
    
    const summary = statsData.summary
    const apiCalls = summary.apiCalls ?? 0
    const bandwidth = summary.bandwidth ?? 0
    const latency = summary.avgLatency ?? 0
    const storageUsed = summary.storageUsed ?? 0
    const errorRate = summary.errorRate ?? 0
    const successRate = summary.successRate ?? 100
    const uploads = summary.uploads ?? 0
    const downloads = summary.downloads ?? 0
    const concurrentRequests = summary.concurrentRequests ?? 0
    
    return [
      {
        title: 'API调用次数',
        value: apiCalls.toLocaleString(),
        change: undefined,
        trend: undefined,
        icon: Activity,
      },
      {
        title: '带宽消耗',
        value: formatBandwidth(bandwidth),
        change: undefined,
        trend: undefined,
        icon: Network,
      },
      {
        title: '平均延迟',
        value: `${latency.toFixed(0)} ms`,
        change: undefined,
        trend: undefined,
        icon: Clock,
      },
      {
        title: '存储使用',
        value: formatStorage(storageUsed),
        change: undefined,
        trend: undefined,
        icon: HardDrive,
      },
      {
        title: '错误率',
        value: `${errorRate.toFixed(2)}%`,
        change: undefined,
        trend: undefined,
        icon: AlertCircle,
      },
      {
        title: '成功率',
        value: `${successRate.toFixed(2)}%`,
        change: undefined,
        trend: undefined,
        icon: CheckCircle2,
      },
      {
        title: '文件上传',
        value: uploads.toLocaleString(),
        change: undefined,
        trend: undefined,
        icon: Upload,
      },
      {
        title: '文件下载',
        value: downloads.toLocaleString(),
        change: undefined,
        trend: undefined,
        icon: Download,
      },
      {
        title: '并发请求',
        value: concurrentRequests.toLocaleString(),
        change: undefined,
        trend: undefined,
        icon: Zap,
      },
    ]
  }, [statsData?.summary])

  // API 指标趋势折线图配置（按天显示）
  const metricsTrendOption = useMemo(() => {
    if (!statsData?.daily || statsData.daily.length === 0) {
      return {
        tooltip: { trigger: 'axis' },
        legend: { data: [] },
        xAxis: { type: 'category', data: [] },
        yAxis: [{ type: 'value' }],
        series: []
      }
    }

    const dailyData = statsData.daily
    const dates = dailyData.map(d => d.date)
    
    const apiCallsData = dailyData.map(d => d.apiCalls)
    const bandwidthData = dailyData.map(d => (d.bandwidth || 0) / (1024 * 1024)) // 转换为MB
    const latencyData = dailyData.map(d => d.avgLatency)
    const errorRateData = dailyData.map(d => d.errorRate)
    const successRateData = dailyData.map(d => d.successRate)
    const uploadsData = dailyData.map(d => d.uploads)
    const downloadsData = dailyData.map(d => d.downloads)
    
    return {
      tooltip: {
        trigger: 'axis',
        backgroundColor: isDark ? 'rgba(0, 0, 0, 0.9)' : 'rgba(0, 0, 0, 0.8)',
        borderColor: 'transparent',
        textStyle: {
          color: '#fff'
        },
        formatter: (params: any) => {
          let result = `<div style="margin-bottom: 4px; font-weight: bold;">${params[0].axisValue}</div>`
          params.forEach((param: any) => {
            let value = param.value
            if (param.seriesName.includes('错误率') || param.seriesName.includes('成功率')) {
              value = value.toFixed(2) + '%'
            } else if (param.seriesName.includes('带宽')) {
              value = value.toFixed(2) + ' MB'
            } else if (param.seriesName.includes('延迟')) {
              value = value.toFixed(0) + ' ms'
            } else {
              value = value.toLocaleString()
            }
            result += `<div style="margin: 4px 0;">
              <span style="display: inline-block; width: 10px; height: 10px; border-radius: 50%; background: ${param.color}; margin-right: 8px;"></span>
              ${param.seriesName}: <span style="font-weight: bold;">${value}</span>
            </div>`
          })
          return result
        }
      },
      legend: {
        data: ['API调用次数', '带宽消耗 (MB)', '平均延迟 (ms)', '错误率 (%)', '成功率 (%)', '文件上传', '文件下载'],
        textStyle: {
          color: textColor
        },
        top: 20,
        itemGap: 15,
        type: 'scroll',
        orient: 'horizontal'
      },
      grid: {
        left: '70px',
        right: '80px',
        bottom: '50px',
        top: '100px',
        containLabel: false
      },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: dates,
        axisLine: {
          lineStyle: {
            color: axisLineColor
          }
        },
        axisLabel: {
          color: textColor
        }
      },
      yAxis: [
        {
          type: 'value',
          name: 'API调用/文件数',
          position: 'left',
          nameTextStyle: {
            color: textColor,
            padding: [0, 0, 0, 0]
          },
          axisLine: {
            lineStyle: {
              color: axisLineColor
            },
            show: true
          },
          axisLabel: {
            color: textColor,
            formatter: '{value}',
            margin: 8
          },
          splitLine: {
            lineStyle: {
              color: splitLineColor,
              type: 'dashed'
            }
          }
        },
        {
          type: 'value',
          name: '带宽 (MB) / 延迟 (ms)',
          position: 'right',
          nameTextStyle: {
            color: textColor,
            padding: [0, 0, 0, 0]
          },
          axisLine: {
            lineStyle: {
              color: axisLineColor
            },
            show: true
          },
          axisLabel: {
            color: textColor,
            formatter: '{value}',
            margin: 8
          },
          splitLine: {
            show: false
          }
        },
        {
          type: 'value',
          name: '百分比 (%)',
          position: 'right',
          offset: 60,
          nameTextStyle: {
            color: textColor,
            padding: [0, 0, 0, 0]
          },
          axisLine: {
            lineStyle: {
              color: axisLineColor
            },
            show: true
          },
          axisLabel: {
            color: textColor,
            formatter: '{value}%',
            margin: 8
          },
          splitLine: {
            show: false
          },
          min: 0,
          max: 100
        }
      ],
      series: [
        {
          name: 'API调用次数',
          type: 'line',
          smooth: true,
          data: apiCallsData,
          yAxisIndex: 0,
          lineStyle: {
            color: '#3b82f6',
            width: 3
          },
          itemStyle: {
            color: '#3b82f6'
          },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: 'rgba(59, 130, 246, 0.3)' },
                { offset: 1, color: 'rgba(59, 130, 246, 0.05)' }
              ]
            }
          }
        },
        {
          name: '带宽消耗 (MB)',
          type: 'line',
          smooth: true,
          data: bandwidthData,
          yAxisIndex: 1,
          lineStyle: {
            color: '#10b981',
            width: 3
          },
          itemStyle: {
            color: '#10b981'
          }
        },
        {
          name: '平均延迟 (ms)',
          type: 'line',
          smooth: true,
          data: latencyData,
          yAxisIndex: 1,
          lineStyle: {
            color: '#f59e0b',
            width: 3
          },
          itemStyle: {
            color: '#f59e0b'
          }
        },
        {
          name: '错误率 (%)',
          type: 'line',
          smooth: true,
          data: errorRateData,
          yAxisIndex: 2,
          lineStyle: {
            color: '#ef4444',
            width: 2,
            type: 'dashed'
          },
          itemStyle: {
            color: '#ef4444'
          }
        },
        {
          name: '成功率 (%)',
          type: 'line',
          smooth: true,
          data: successRateData,
          yAxisIndex: 2,
          lineStyle: {
            color: '#10b981',
            width: 2,
            type: 'dashed'
          },
          itemStyle: {
            color: '#10b981'
          }
        },
        {
          name: '文件上传',
          type: 'line',
          smooth: true,
          data: uploadsData,
          yAxisIndex: 0,
          lineStyle: {
            color: '#8b5cf6',
            width: 2
          },
          itemStyle: {
            color: '#8b5cf6'
          }
        },
        {
          name: '文件下载',
          type: 'line',
          smooth: true,
          data: downloadsData,
          yAxisIndex: 0,
          lineStyle: {
            color: '#ec4899',
            width: 2
          },
          itemStyle: {
            color: '#ec4899'
          }
        }
      ]
    }
  }, [isDark, textColor, axisLineColor, splitLineColor, statsData?.daily])


  return (
    <AdminLayout
      title="仪表板"
      description=""
    >
      <div className="space-y-6 animate-in fade-in-0 slide-in-from-bottom-4 duration-500">
        {/* 时间范围选择 */}
        <Card className="p-4">
          <div className="flex flex-col md:flex-row gap-4 items-start md:items-end">
            <div className="flex gap-2 flex-wrap">
              <Button
                variant={rangeType === 'today' ? 'primary' : 'ghost'}
                size="sm"
                onClick={() => setRangeType('today')}
              >
                今天
              </Button>
              <Button
                variant={rangeType === 'yesterday' ? 'primary' : 'ghost'}
                size="sm"
                onClick={() => setRangeType('yesterday')}
              >
                昨天
              </Button>
              <Button
                variant={rangeType === 'week' ? 'primary' : 'ghost'}
                size="sm"
                onClick={() => setRangeType('week')}
              >
                最近7天
              </Button>
              <Button
                variant={rangeType === 'month' ? 'primary' : 'ghost'}
                size="sm"
                onClick={() => setRangeType('month')}
              >
                最近30天
              </Button>
            </div>
            
            {/* 自定义日期范围 */}
            <div className="flex gap-2 flex-wrap items-end flex-1">
              <div className="flex gap-2 items-end">
                <div>
                  <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">开始日期</label>
                  <input
                    type="date"
                    value={customStartDate}
                    onChange={(e) => setCustomStartDate(e.target.value)}
                    max={today}
                    className="px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-800 text-slate-900 dark:text-white text-sm"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">结束日期</label>
                  <input
                    type="date"
                    value={customEndDate}
                    onChange={(e) => setCustomEndDate(e.target.value)}
                    max={today}
                    className="px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-800 text-slate-900 dark:text-white text-sm"
                  />
                </div>
                <Button
                  variant={rangeType === 'custom' ? 'primary' : 'ghost'}
                  size="sm"
                  onClick={handleCustomDateSubmit}
                  disabled={!customStartDate || !customEndDate}
                  leftIcon={<Calendar className="w-4 h-4" />}
                >
                  查询
                </Button>
              </div>
            </div>
          </div>
        </Card>

        {/* 统计卡片 */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
          {stats.map((stat, index) => (
            <motion.div
              key={stat.title}
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: index * 0.05 }}
            >
              <StatCard {...stat} />
            </motion.div>
          ))}
        </div>

        {/* API 指标趋势图表 */}
        <Card className="p-6">
          <h3 className="text-lg font-semibold text-slate-900 dark:text-white mb-4">
            API 指标趋势
          </h3>
          <div className="w-full overflow-x-auto" style={{ maxWidth: '100%' }}>
            <div style={{ minWidth: statsData?.daily && statsData.daily.length > 10 ? `${statsData.daily.length * 80}px` : '100%' }}>
              <ReactECharts
                ref={chartRef}
                option={metricsTrendOption}
                style={{ height: '400px', width: '100%', minHeight: '400px' }}
                opts={{ renderer: 'svg' }}
                notMerge={true}
                lazyUpdate={false}
              />
            </div>
          </div>
        </Card>
      </div>
    </AdminLayout>
  )
}

export default Dashboard

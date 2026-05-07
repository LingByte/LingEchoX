import { useEffect, useMemo, useState, type CSSProperties } from 'react'
import { Button, Card, Grid, Progress, Space, Spin, Statistic, Tag, Typography } from '@arco-design/web-react'
import { VChart } from '@visactor/react-vchart'
import { initVChartArcoTheme } from '@visactor/vchart-arco-theme'
import { ArrowRight, Headphones, LayoutDashboard } from 'lucide-react'
import { Link } from 'react-router-dom'
import BaseLayout from '@/components/Layout/BaseLayout'
import { useAuthStore } from '@/stores/authStore'
import { listSIPCalls } from '@/api/sipCalls'
import type { SIPCallRow } from '@/api/sipCalls'
import { getOutboundCampaignMetrics } from '@/api/outboundCampaigns'
import type { OutboundCampaignMetrics } from '@/api/outboundCampaigns'

const { Row, Col } = Grid

let arcoVChartThemeRegistered = false
function ensureArcoVChartTheme() {
  if (!arcoVChartThemeRegistered) {
    initVChartArcoTheme()
    arcoVChartThemeRegistered = true
  }
}

function aggregateCalls(list: SIPCallRow[], totalFromApi: number) {
  let inbound = 0
  let outbound = 0
  let otherDir = 0
  let ended = 0
  let withRecording = 0
  let durationSum = 0
  let durationN = 0
  const dayBuckets: Record<string, number> = {}

  for (const c of list) {
    const dir = (c.direction || '').toLowerCase()
    if (dir === 'inbound') inbound++
    else if (dir === 'outbound') outbound++
    else if (dir) otherDir++

    if ((c.state || '').toLowerCase() === 'ended') ended++
    if (c.recordingUrl?.trim()) withRecording++

    const ds = c.durationSec
    if (ds != null && ds > 0) {
      durationSum += ds
      durationN++
    }

    const ts = c.endedAt || c.byeAt || c.updatedAt || ''
    const day = typeof ts === 'string' && ts.length >= 10 ? ts.slice(0, 10) : ''
    if (day) dayBuckets[day] = (dayBuckets[day] || 0) + 1
  }

  const avgDur = durationN > 0 ? Math.round(durationSum / durationN) : 0
  const dirTotal = inbound + outbound + otherDir
  const sampleNote =
    list.length > 0 && totalFromApi > list.length
      ? `方向与七日分布按当前样本（最近 ${list.length} 条）估算；「通话总数」为服务端全量计数。`
      : list.length > 0
        ? '方向与七日分布基于当前列表样本。'
        : ''

  return {
    inbound,
    outbound,
    otherDir,
    ended,
    withRecording,
    avgDur,
    dayBuckets,
    dirTotal,
    sampleNote,
    totalFromApi,
  }
}

function last7DayLabels(): string[] {
  const out: string[] = []
  const today = new Date()
  for (let i = 6; i >= 0; i--) {
    const d = new Date(today)
    d.setDate(d.getDate() - i)
    out.push(d.toISOString().slice(0, 10))
  }
  return out
}

export default function Overview() {
  const user = useAuthStore((s) => s.user)
  const isPlatform = Boolean(user?.isPlatformAdmin || user?.principal === 'platform')
  const tenantName = String(user?.tenantName || '贵司')
  const displayName = String(user?.displayName || user?.username || user?.email || '管理员')

  const [loading, setLoading] = useState(true)
  const [callsList, setCallsList] = useState<SIPCallRow[]>([])
  const [callsTotal, setCallsTotal] = useState(0)
  const [callsErr, setCallsErr] = useState<string | null>(null)
  const [campaignMetrics, setCampaignMetrics] = useState<OutboundCampaignMetrics | null>(null)

  useEffect(() => {
    ensureArcoVChartTheme()
  }, [])

  useEffect(() => {
    let cancelled = false
    void (async () => {
      setLoading(true)
      setCallsErr(null)
      try {
        const callsRes = await listSIPCalls(1, 100)
        if (cancelled) return
        if (callsRes.code === 200 && callsRes.data) {
          setCallsList(callsRes.data.list || [])
          setCallsTotal(callsRes.data.total ?? 0)
        } else {
          setCallsErr(callsRes.msg || '通话统计加载失败')
          setCallsList([])
          setCallsTotal(0)
        }

        if (!isPlatform) {
          const cm = await getOutboundCampaignMetrics()
          if (!cancelled && cm.code === 200 && cm.data) setCampaignMetrics(cm.data)
          else if (!cancelled) setCampaignMetrics(null)
        } else {
          setCampaignMetrics(null)
        }
      } catch {
        if (!cancelled) {
          setCallsErr('通话统计加载失败')
          setCallsList([])
          setCallsTotal(0)
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [isPlatform])

  const agg = useMemo(() => aggregateCalls(callsList, callsTotal), [callsList, callsTotal])
  const weekLabels = useMemo(() => last7DayLabels(), [])
  const weekCounts = weekLabels.map((d) => agg.dayBuckets[d] || 0)

  const weekTrendSpec = useMemo(
    () => ({
      type: 'line',
      height: 260,
      padding: { left: 12, right: 12, top: 20, bottom: 24 },
      data: [
        {
          id: 'weekTrend',
          values: weekLabels.map((label, i) => ({
            day: label.slice(5),
            count: weekCounts[i],
          })),
        },
      ],
      xField: 'day',
      yField: 'count',
      point: { visible: true },
      line: { style: { curveType: 'monotone' } },
      tooltip: { visible: true },
      legends: { visible: false },
      axes: [
        { orient: 'bottom' },
        { orient: 'left', title: { visible: false }, min: 0 },
      ],
    }),
    [weekLabels, weekCounts],
  )

  const directionPieSpec = useMemo(() => {
    const parts: { type: string; value: number }[] = []
    if (agg.inbound > 0) parts.push({ type: '呼入', value: agg.inbound })
    if (agg.outbound > 0) parts.push({ type: '呼出', value: agg.outbound })
    if (agg.otherDir > 0) parts.push({ type: '其他', value: agg.otherDir })
    return {
      type: 'pie',
      height: 260,
      padding: 12,
      data: [{ id: 'dir', values: parts.length ? parts : [{ type: '暂无', value: 1 }] }],
      categoryField: 'type',
      valueField: 'value',
      legends: { visible: true, orient: 'bottom' },
      tooltip: { visible: true },
      pie: {
        style: {
          stroke: '#fff',
          lineWidth: 2,
        },
      },
    }
  }, [agg.inbound, agg.outbound, agg.otherDir])

  const heroOutlineBtnStyle: CSSProperties = {
    borderColor: 'rgba(255,255,255,0.35)',
    color: '#e0e7ff',
    display: 'inline-flex',
    alignItems: 'center',
    gap: 8,
    flexWrap: 'nowrap',
  }

  return (
    <BaseLayout hideHeader>
      <div
        style={{
          borderRadius: 16,
          padding: '28px 28px 32px',
          marginBottom: 20,
          position: 'relative',
          overflow: 'hidden',
          background: 'linear-gradient(125deg, #0f172a 0%, #1e1b4b 42%, #312e81 78%, #4c1d95 100%)',
          boxShadow: '0 24px 80px rgba(79, 70, 229, 0.25)',
        }}
      >
        <div
          aria-hidden
          style={{
            position: 'absolute',
            width: 420,
            height: 420,
            right: '-120px',
            top: '-140px',
            background: 'radial-gradient(circle, rgba(167,139,250,0.45) 0%, transparent 65%)',
            pointerEvents: 'none',
          }}
        />
        <div
          aria-hidden
          style={{
            position: 'absolute',
            width: 280,
            height: 280,
            left: '-40px',
            bottom: '-100px',
            background: 'radial-gradient(circle, rgba(56,189,248,0.35) 0%, transparent 70%)',
            pointerEvents: 'none',
          }}
        />
        <Row gutter={24} align="center">
          <Col xs={24} lg={14}>
            <Tag color="arcoblue" style={{ marginBottom: 12, border: 'none', background: 'rgba(255,255,255,0.12)', color: '#e0e7ff' }}>
              {isPlatform ? '平台控制台' : '企业空间'}
            </Tag>
            <Typography.Title heading={3} style={{ color: '#fff', marginTop: 0, marginBottom: 8 }}>
              欢迎回来，{displayName}
            </Typography.Title>
            <Typography.Paragraph style={{ color: 'rgba(226,232,240,0.92)', fontSize: 15, marginBottom: 16, maxWidth: 520 }}>
              {isPlatform
                ? '您正以平台管理员身份运维中继与租户资源。下方统计来自当前账号可见的通话样本（最近一页）。'
                : `您已进入「${tenantName}」控制台；下方展示近期通话概况与外呼任务累计指标。`}
            </Typography.Paragraph>
            <Space wrap size={12}>
              {!isPlatform && (
                <Link to="/profile">
                  <Button type="primary" style={{ background: '#fff', color: '#312e81', border: 'none' }}>
                    账号与安全
                  </Button>
                </Link>
              )}
              <Link to={isPlatform ? '/sip-users' : '/web-agents'}>
                <Button type="outline" style={heroOutlineBtnStyle}>
                  {isPlatform ? (
                    <LayoutDashboard size={18} strokeWidth={2} className="shrink-0" />
                  ) : (
                    <Headphones size={18} strokeWidth={2} className="shrink-0" />
                  )}
                  <span style={{ whiteSpace: 'nowrap' }}>{isPlatform ? '管理 SIP 用户' : '打开 Web 坐席'}</span>
                  <ArrowRight size={16} className="shrink-0 opacity-90" />
                </Button>
              </Link>
              <Link to="/call-records">
                <Button type="outline" style={heroOutlineBtnStyle}>
                  <span style={{ whiteSpace: 'nowrap' }}>通话记录</span>
                  <ArrowRight size={16} className="shrink-0 opacity-90" />
                </Button>
              </Link>
            </Space>
          </Col>
          <Col xs={24} lg={10}>
            <div
              style={{
                background: 'rgba(15,23,42,0.35)',
                backdropFilter: 'blur(12px)',
                borderRadius: 12,
                padding: '20px 22px',
                border: '1px solid rgba(255,255,255,0.12)',
              }}
            >
              <Typography.Text style={{ color: 'rgba(226,232,240,0.75)', fontSize: 12 }}>当前上下文</Typography.Text>
              <div style={{ marginTop: 10, display: 'flex', flexDirection: 'column', gap: 8 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center' }}>
                  <span style={{ color: '#cbd5e1', fontSize: 13 }}>组织</span>
                  <span style={{ color: '#fff', fontWeight: 600, textAlign: 'right' }}>{isPlatform ? '全平台' : tenantName}</span>
                </div>
              </div>
            </div>
          </Col>
        </Row>
      </div>

      <Spin loading={loading} style={{ width: '100%' }}>
        {callsErr ? (
          <Card bordered={false} style={{ borderRadius: 12, marginBottom: 16 }}>
            <Typography.Text type="error">{callsErr}</Typography.Text>
          </Card>
        ) : null}

        <Typography.Title heading={6} style={{ margin: '0 0 12px' }}>
          通话概况
        </Typography.Title>
        <Typography.Paragraph style={{ margin: '0 0 14px', fontSize: 12, color: 'var(--color-text-3)' }}>
          {agg.sampleNote || '暂无样本说明。'}
        </Typography.Paragraph>

        <Row gutter={[16, 16]}>
          <Col xs={24} sm={12} lg={6}>
            <Card bordered={false} style={{ borderRadius: 12, height: '100%' }}>
              <Statistic title="通话总数（库内）" value={agg.totalFromApi} groupSeparator />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card bordered={false} style={{ borderRadius: 12, height: '100%' }}>
              <Statistic title="样本中已结束" value={agg.ended} groupSeparator />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card bordered={false} style={{ borderRadius: 12, height: '100%' }}>
              <Statistic title="样本中带录音" value={agg.withRecording} groupSeparator />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card bordered={false} style={{ borderRadius: 12, height: '100%' }}>
              <Statistic title="样本平均时长(s)" value={agg.avgDur} groupSeparator />
            </Card>
          </Col>
        </Row>

        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col xs={24} lg={12}>
            <Card bordered={false} style={{ borderRadius: 12, height: '100%' }} title="方向占比（样本 · VisActor / Arco 主题）">
              {agg.dirTotal === 0 ? (
                <Typography.Text type="secondary">暂无方向数据</Typography.Text>
              ) : (
                <Space direction="vertical" style={{ width: '100%' }} size={12}>
                  <div style={{ width: '100%', overflow: 'hidden' }}>
                    <VChart spec={directionPieSpec as any} />
                  </div>
                  <Space direction="vertical" style={{ width: '100%' }} size={12}>
                    <div>
                      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6, fontSize: 13 }}>
                        <span>呼入</span>
                        <span style={{ color: 'var(--color-text-3)' }}>
                          {agg.inbound} · {Math.round((agg.inbound / agg.dirTotal) * 100)}%
                        </span>
                      </div>
                      <Progress percent={(agg.inbound / agg.dirTotal) * 100} showText={false} color="#6366f1" />
                    </div>
                    <div>
                      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6, fontSize: 13 }}>
                        <span>呼出</span>
                        <span style={{ color: 'var(--color-text-3)' }}>
                          {agg.outbound} · {Math.round((agg.outbound / agg.dirTotal) * 100)}%
                        </span>
                      </div>
                      <Progress percent={(agg.outbound / agg.dirTotal) * 100} showText={false} color="#0ea5e9" />
                    </div>
                  </Space>
                </Space>
              )}
            </Card>
          </Col>
          <Col xs={24} lg={12}>
            <Card bordered={false} style={{ borderRadius: 12, height: '100%' }} title="近 7 日样本趋势（按结束/更新时间）">
              <div style={{ width: '100%', overflow: 'hidden' }}>
                <VChart spec={weekTrendSpec as any} />
              </div>
            </Card>
          </Col>
        </Row>

        {!isPlatform && campaignMetrics ? (
          <>
            <Typography.Title heading={6} style={{ margin: '24px 0 12px' }}>
              外呼任务（累计）
            </Typography.Title>
            <Row gutter={[16, 16]}>
              <Col xs={12} sm={8} lg={4}>
                <Card bordered={false} style={{ borderRadius: 12 }}>
                  <Statistic title="已邀约拨号" value={campaignMetrics.invited_total} groupSeparator />
                </Card>
              </Col>
              <Col xs={12} sm={8} lg={4}>
                <Card bordered={false} style={{ borderRadius: 12 }}>
                  <Statistic title="接通联系人" value={campaignMetrics.answered_total} groupSeparator />
                </Card>
              </Col>
              <Col xs={12} sm={8} lg={4}>
                <Card bordered={false} style={{ borderRadius: 12 }}>
                  <Statistic title="失败/用尽" value={campaignMetrics.failed_total} groupSeparator />
                </Card>
              </Col>
              <Col xs={12} sm={8} lg={4}>
                <Card bordered={false} style={{ borderRadius: 12 }}>
                  <Statistic title="重试中" value={campaignMetrics.retrying_total} groupSeparator />
                </Card>
              </Col>
              <Col xs={12} sm={8} lg={4}>
                <Card bordered={false} style={{ borderRadius: 12 }}>
                  <Statistic title="抑制" value={campaignMetrics.suppressed_total} groupSeparator />
                </Card>
              </Col>
            </Row>
          </>
        ) : null}

        {!isPlatform && (
          <Card bordered={false} style={{ marginTop: 20, borderRadius: 12, background: 'var(--color-fill-2)' }}>
            <Typography.Text type="secondary" style={{ fontSize: 13 }}>
              提示：成员菜单受角色与权限控制；企业「管理员」角色与权限目录全量同步，且不可在「角色权限」中修改该项。
            </Typography.Text>
          </Card>
        )}
      </Spin>
    </BaseLayout>
  )
}

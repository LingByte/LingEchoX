import { useEffect, useMemo, useRef, useState } from 'react'
import { Card, Grid, Progress, Spin, Tag, Typography } from '@arco-design/web-react'
import { VChart } from '@visactor/react-vchart'
import { initVChartArcoTheme } from '@visactor/vchart-arco-theme'
import BaseLayout from '@/components/Layout/BaseLayout'
import { useAuthStore } from '@/stores/authStore'
import { useTranslation } from '@/i18n'
import { listSIPCalls } from '@/api/sipCalls'
import type { SIPCallRow } from '@/api/sipCalls'
import { getOutboundCampaignMetrics } from '@/api/outboundCampaigns'
import type { OutboundCampaignMetrics } from '@/api/outboundCampaigns'

const { Row, Col } = Grid

/** 抵消 BaseLayout main 的 padding，占满可视区域 */
const PAGE_SHELL: React.CSSProperties = {
  margin: '-24px -24px -40px -24px',
  padding: '20px 24px 24px',
  height: '100vh',
  maxHeight: '100vh',
  boxSizing: 'border-box',
  display: 'flex',
  flexDirection: 'column',
  gap: 12,
  overflow: 'hidden',
}

let arcoVChartThemeRegistered = false
function ensureArcoVChartTheme() {
  if (!arcoVChartThemeRegistered) {
    initVChartArcoTheme()
    arcoVChartThemeRegistered = true
  }
}

type SampleNoteKind = 'empty' | 'list' | 'paged'

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
  let sampleNoteKind: SampleNoteKind = 'empty'
  if (list.length > 0 && totalFromApi > list.length) sampleNoteKind = 'paged'
  else if (list.length > 0) sampleNoteKind = 'list'

  return {
    inbound,
    outbound,
    otherDir,
    ended,
    withRecording,
    avgDur,
    dayBuckets,
    dirTotal,
    sampleNoteKind,
    sampleCount: list.length,
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

function MetricTile({ label, value }: { label: string; value: number }) {
  return (
    <div
      style={{
        padding: '8px 10px',
        borderRadius: 8,
        background: 'var(--color-fill-2)',
        minWidth: 0,
      }}
    >
      <div
        style={{
          fontSize: 11,
          color: 'var(--color-text-3)',
          lineHeight: 1.3,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
        title={label}
      >
        {label}
      </div>
      <div style={{ fontSize: 18, fontWeight: 600, lineHeight: 1.35, marginTop: 2 }}>{value}</div>
    </div>
  )
}

function AutoHeightChart({ spec }: { spec: Record<string, unknown> }) {
  const ref = useRef<HTMLDivElement>(null)
  const [height, setHeight] = useState(180)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver((entries) => {
      const h = entries[0]?.contentRect.height ?? 180
      setHeight(Math.max(120, Math.min(220, Math.floor(h))))
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  return (
    <div ref={ref} style={{ flex: 1, minHeight: 120, maxHeight: 220, width: '100%', height: '100%' }}>
      <VChart spec={{ ...spec, height } as any} />
    </div>
  )
}

export default function Overview() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const isPlatform = Boolean(user?.isPlatformAdmin || user?.principal === 'platform')
  const tenantName = String(user?.tenantName || t('common.yourOrg'))
  const displayName = String(
    user?.displayName || user?.username || user?.email || t('common.adminFallback'),
  )

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
          setCallsErr(callsRes.msg || t('overview.callsLoadFailed'))
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
          setCallsErr(t('overview.callsLoadFailed'))
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
  }, [isPlatform, t])

  const agg = useMemo(() => aggregateCalls(callsList, callsTotal), [callsList, callsTotal])
  const weekLabels = useMemo(() => last7DayLabels(), [])
  const weekCounts = weekLabels.map((d) => agg.dayBuckets[d] || 0)

  const metricTiles = useMemo(() => {
    const callTiles = [
      { label: t('overview.statTotalCalls'), value: agg.totalFromApi },
      { label: t('overview.statEndedInSample'), value: agg.ended },
      { label: t('overview.statWithRecording'), value: agg.withRecording },
      { label: t('overview.statAvgDuration'), value: agg.avgDur },
    ]
    if (isPlatform || !campaignMetrics) return callTiles
    return [
      ...callTiles,
      { label: t('overview.campaignInvited'), value: campaignMetrics.invited_total },
      { label: t('overview.campaignAnswered'), value: campaignMetrics.answered_total },
      { label: t('overview.campaignFailed'), value: campaignMetrics.failed_total },
      { label: t('overview.campaignRetrying'), value: campaignMetrics.retrying_total },
      { label: t('overview.campaignSuppressed'), value: campaignMetrics.suppressed_total },
    ]
  }, [agg, campaignMetrics, isPlatform, t])

  const weekTrendSpec = useMemo(
    () => ({
      type: 'line',
      padding: { left: 8, right: 8, top: 8, bottom: 20 },
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
      point: { visible: true, style: { size: 4 } },
      line: { style: { curveType: 'monotone', lineWidth: 2 } },
      tooltip: { visible: true },
      legends: { visible: false },
      axes: [{ orient: 'bottom' }, { orient: 'left', title: { visible: false }, min: 0 }],
    }),
    [weekLabels, weekCounts],
  )

  const directionRows = [
    { key: 'inbound', label: t('overview.inbound'), count: agg.inbound, color: '#6366f1' },
    { key: 'outbound', label: t('overview.outbound'), count: agg.outbound, color: '#0ea5e9' },
    ...(agg.otherDir > 0
      ? [{ key: 'other', label: t('overview.other'), count: agg.otherDir, color: '#94a3b8' }]
      : []),
  ]

  return (
    <BaseLayout hideHeader>
      <div style={PAGE_SHELL}>
        <div
          style={{
            flexShrink: 0,
            minHeight: 142,
            borderRadius: 12,
            padding: '22px 24px',
            position: 'relative',
            overflow: 'hidden',
            background: 'linear-gradient(125deg, #0f172a 0%, #1e1b4b 42%, #312e81 78%, #4c1d95 100%)',
          }}
        >
          <Row gutter={20} align="center">
            <Col flex={1} style={{ minWidth: 0 }}>
              <Tag
                color="arcoblue"
                size="small"
                style={{ marginBottom: 10, border: 'none', background: 'rgba(255,255,255,0.12)', color: '#e0e7ff' }}
              >
                {isPlatform ? t('overview.platformConsole') : t('overview.tenantSpace')}
              </Tag>
              <Typography.Title heading={4} style={{ color: '#fff', margin: 0, lineHeight: 1.35 }}>
                {t('overview.welcomeBack', { name: displayName })}
              </Typography.Title>
              <Typography.Paragraph
                style={{
                  color: 'rgba(226,232,240,0.85)',
                  fontSize: 13,
                  margin: '8px 0 0',
                  lineHeight: 1.5,
                }}
              >
                {isPlatform ? t('overview.heroPlatform') : t('overview.heroTenant', { tenant: tenantName })}
              </Typography.Paragraph>
            </Col>
            <Col flex="none" style={{ textAlign: 'right' }}>
              <Typography.Text style={{ color: 'rgba(226,232,240,0.7)', fontSize: 12 }}>
                {t('overview.organization')}
              </Typography.Text>
              <div style={{ color: '#fff', fontWeight: 600, fontSize: 14, marginTop: 6, maxWidth: 220 }}>
                {isPlatform ? t('overview.allPlatform') : tenantName}
              </div>
            </Col>
          </Row>
        </div>

        <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', gap: 12, position: 'relative' }}>
          {loading ? (
            <div
              style={{
                position: 'absolute',
                inset: 0,
                zIndex: 2,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                background: 'color-mix(in srgb, var(--color-bg-1) 55%, transparent)',
                borderRadius: 12,
              }}
            >
              <Spin />
            </div>
          ) : null}

          {callsErr ? (
            <Typography.Text type="error" style={{ fontSize: 12, flexShrink: 0 }}>
              {callsErr}
            </Typography.Text>
          ) : null}

          <Card
            bordered={false}
            style={{ borderRadius: 12, flexShrink: 0 }}
            title={t('overview.metricsPanel')}
            bodyStyle={{ padding: '10px 12px' }}
          >
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fill, minmax(108px, 1fr))',
                gap: 8,
              }}
            >
              {metricTiles.map((m) => (
                <MetricTile key={m.label} label={m.label} value={m.value} />
              ))}
            </div>
          </Card>

          <div
            style={{
              flex: '0 0 auto',
              marginTop: 'auto',
              minHeight: 0,
              maxHeight: 'min(34vh, 260px)',
              display: 'flex',
              gap: 12,
              overflow: 'hidden',
            }}
          >
            <Card
              bordered={false}
              style={{
                borderRadius: 12,
                width: 200,
                flexShrink: 0,
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
              }}
              title={t('overview.directionShare')}
              bodyStyle={{
                padding: '10px 14px',
                flex: 1,
                minHeight: 0,
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center',
              }}
            >
              {agg.dirTotal === 0 ? (
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {t('overview.noDirectionData')}
                </Typography.Text>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                  {directionRows.map((row) => (
                    <div key={row.key}>
                      <div
                        style={{
                          display: 'flex',
                          justifyContent: 'space-between',
                          marginBottom: 4,
                          fontSize: 12,
                          gap: 4,
                        }}
                      >
                        <span>{row.label}</span>
                        <span style={{ color: 'var(--color-text-3)', whiteSpace: 'nowrap' }}>
                          {row.count} · {Math.round((row.count / agg.dirTotal) * 100)}%
                        </span>
                      </div>
                      <Progress
                        percent={(row.count / agg.dirTotal) * 100}
                        showText={false}
                        color={row.color}
                        size="small"
                      />
                    </div>
                  ))}
                </div>
              )}
            </Card>

            <Card
              bordered={false}
              style={{
                borderRadius: 12,
                flex: 1,
                minWidth: 0,
                minHeight: 0,
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
              }}
              title={t('overview.weekTrend')}
              bodyStyle={{
                padding: '6px 10px 10px',
                flex: 1,
                minHeight: 0,
                display: 'flex',
                flexDirection: 'column',
                overflow: 'hidden',
              }}
            >
              <AutoHeightChart spec={weekTrendSpec} />
            </Card>
          </div>
        </div>
      </div>
    </BaseLayout>
  )
}

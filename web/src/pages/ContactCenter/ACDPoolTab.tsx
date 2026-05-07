import { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Checkbox,
  Input,
  Modal,
  Select,
  Space,
  Tag,
  TimePicker,
  Typography,
} from '@arco-design/web-react'
import { IconDelete, IconPlus } from '@arco-design/web-react/icon'
import dayjs, { type Dayjs } from 'dayjs'
import { showAlert } from '@/utils/notification'
import {
  ACD_ROUTE_TYPES,
  ACD_WORK_STATES,
  createACDPoolTarget,
  deleteACDPoolTarget,
  listACDPoolTargets,
  updateACDPoolTarget,
  type ACDPoolTargetRow,
} from '@/api/acdPool'

/** Matches backend acd_shift_schedule: weekdays 0=Sun..6=Sat; empty weekdays = all days */
type ShiftSegment = { weekdays: number[]; start: string; end: string }

const WEEKDAY_OPTS = [
  { label: '日', value: 0 },
  { label: '一', value: 1 },
  { label: '二', value: 2 },
  { label: '三', value: 3 },
  { label: '四', value: 4 },
  { label: '五', value: 5 },
  { label: '六', value: 6 },
]

function hmToDayjs(hm: string): Dayjs {
  const m = /^(\d{1,2}):(\d{2})$/.exec(hm.trim())
  if (!m) return dayjs().hour(9).minute(0).second(0).millisecond(0)
  const h = Math.min(23, Math.max(0, parseInt(m[1]!, 10)))
  const min = Math.min(59, Math.max(0, parseInt(m[2]!, 10)))
  return dayjs().hour(h).minute(min).second(0).millisecond(0)
}

function dayjsToHm(d: Dayjs | undefined | null): string {
  if (!d || !d.isValid()) return '09:00'
  return d.format('HH:mm')
}

function validHm(s: string): boolean {
  return /^([01]?\d|2[0-3]):([0-5]\d)$/.test(s.trim())
}

function parseShiftScheduleJSON(raw: string): ShiftSegment[] {
  const t = raw.trim()
  if (!t) return []
  try {
    const arr = JSON.parse(t) as unknown
    if (!Array.isArray(arr)) return []
    const out: ShiftSegment[] = []
    for (const item of arr) {
      if (!item || typeof item !== 'object') continue
      const o = item as Record<string, unknown>
      const wd = o.weekdays
      let weekdays: number[] = []
      if (Array.isArray(wd)) {
        weekdays = wd.filter((n): n is number => typeof n === 'number' && n >= 0 && n <= 6)
      }
      const start = typeof o.start === 'string' ? o.start : '09:00'
      const end = typeof o.end === 'string' ? o.end : '18:00'
      out.push({ weekdays, start, end })
    }
    return out
  } catch {
    return []
  }
}

function serializeShiftSchedule(segments: ShiftSegment[]): string {
  if (!segments.length) return ''
  return JSON.stringify(
    segments.map((s) => ({
      weekdays: [...new Set(s.weekdays)].sort((a, b) => a - b),
      start: s.start.trim(),
      end: s.end.trim(),
    })),
  )
}

type FormState = {
  name: string
  routeType: string
  targetValue: string
  weight: number
  workState: string
  shiftSegments: ShiftSegment[]
}
const defaultForm = (): FormState => ({
  name: '',
  routeType: 'sip',
  targetValue: '',
  weight: 10,
  workState: 'offline',
  shiftSegments: [],
})

const defaultShiftSegment = (): ShiftSegment => ({
  weekdays: [1, 2, 3, 4, 5],
  start: '09:00',
  end: '18:00',
})

export default function ACDPoolTab({ active, refreshNonce = 0 }: { active: boolean; refreshNonce?: number }) {
  const [rows, setRows] = useState<ACDPoolTargetRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [routeTypeFilter, setRouteTypeFilter] = useState('')
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormState>(defaultForm)
  const [saving, setSaving] = useState(false)
  const [acdDeleteOpen, setAcdDeleteOpen] = useState(false)
  const [acdDeleteId, setAcdDeleteId] = useState<number | null>(null)
  const [acdDeleteLoading, setAcdDeleteLoading] = useState(false)
  const pageSize = 20

  const load = useCallback(async () => {
    if (!active) return
    setLoading(true)
    try {
      const res = await listACDPoolTargets(page, pageSize, { routeType: routeTypeFilter.trim() || undefined })
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total || 0)
      }
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '加载失败', 'error')
    } finally {
      setLoading(false)
    }
  }, [active, page, routeTypeFilter])

  useEffect(() => {
    void load()
  }, [load, refreshNonce])

  const openCreate = () => {
    setEditingId(null)
    setForm(defaultForm())
    setModalOpen(true)
  }
  const openEdit = (r: ACDPoolTargetRow) => {
    setEditingId(r.id)
    setForm({
      name: r.name || '',
      routeType: r.routeType || 'sip',
      targetValue: r.targetValue || '',
      weight: r.weight ?? 0,
      workState: r.workState || 'offline',
      shiftSegments: parseShiftScheduleJSON(r.shiftSchedule ?? ''),
    })
    setModalOpen(true)
  }
  const closeModal = () => {
    setModalOpen(false)
    setEditingId(null)
  }

  const save = async () => {
    setSaving(true)
    try {
      const routeType = editingId == null ? 'sip' : form.routeType
      const tv = routeType === 'sip' ? form.targetValue.trim() : ''
      if (routeType === 'sip' && !tv) {
        showAlert('SIP 目标不能为空', 'error')
        return
      }
      const segs = form.shiftSegments
      for (let i = 0; i < segs.length; i++) {
        const s = segs[i]!
        if (!validHm(s.start) || !validHm(s.end)) {
          showAlert(`第 ${i + 1} 段班次时间须为 HH:mm（00:00–23:59）`, 'error')
          return
        }
      }
      const shiftTrim = serializeShiftSchedule(segs)
      const body = {
        name: form.name.trim(),
        routeType,
        sipSource: '',
        targetValue: tv,
        sipTrunkHost: '',
        sipTrunkPort: 0,
        sipTrunkSignalingAddr: '',
        sipCallerId: '',
        sipCallerDisplayName: '',
        weight: Number(form.weight) || 0,
        workState: form.workState,
        shiftSchedule: shiftTrim,
      }
      const res = editingId == null ? await createACDPoolTarget(body) : await updateACDPoolTarget(editingId, body)
      if (res.code === 200) {
        showAlert('保存成功', 'success')
        closeModal()
        void load()
      } else showAlert(res.msg || '保存失败', 'error')
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '保存失败', 'error')
    } finally {
      setSaving(false)
    }
  }

  const confirmAcdDelete = async () => {
    if (acdDeleteId == null) return
    setAcdDeleteLoading(true)
    try {
      const res = await deleteACDPoolTarget(acdDeleteId)
      if (res.code !== 200) {
        showAlert(res.msg || '删除失败', 'error')
        return
      }
      showAlert('删除成功', 'success')
      setAcdDeleteOpen(false)
      setAcdDeleteId(null)
      void load()
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '删除失败', 'error')
    } finally {
      setAcdDeleteLoading(false)
    }
  }

  const workStateLabel = (s: string) =>
    ({ offline: '离线', available: '可用', ringing: '振铃中', busy: '忙碌', acw: '话后整理', break: '休息' } as Record<string, string>)[s] || s

  const shiftScheduleSummary = (json?: string) => {
    if (!json?.trim()) return '全天'
    try {
      const a = JSON.parse(json) as unknown
      return Array.isArray(a) && a.length ? `已设 ${a.length} 段` : '全天'
    } catch {
      return '格式异常'
    }
  }

  return (
    <div className="mt-4 space-y-3">
      <Typography.Paragraph style={{ margin: 0, fontSize: 12 }} className="rounded-lg border px-3 py-2.5">
        号码池用于来电分配和转接选线，SIP 注册与 ACD 选线独立。
      </Typography.Paragraph>
      <Space wrap align="end">
        <Space direction="vertical" size={4}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>路由类型</Typography.Text>
          <Select
            style={{ width: 112 }}
            value={routeTypeFilter === '' ? undefined : routeTypeFilter}
            placeholder="全部"
            allowClear
            onChange={(v) => setRouteTypeFilter((v as string) ?? '')}
            options={ACD_ROUTE_TYPES.map((rt) => ({ label: rt, value: rt }))}
          />
        </Space>
        <Button type="primary" size="small" onClick={() => { setPage(1); void load() }}>搜索</Button>
        <Button type="outline" size="small" onClick={openCreate}>新增 SIP 目标</Button>
      </Space>

      {loading ? (
        <div className="p-4 text-sm text-muted-foreground">加载中...</div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border bg-card">
          <table className="min-w-[760px] w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="text-left p-3 whitespace-nowrap">ID</th>
                <th className="text-left p-3 whitespace-nowrap">名称</th>
                <th className="text-left p-3 min-w-[180px]">呼叫号码</th>
                <th className="text-left p-3 whitespace-nowrap">权重</th>
                <th className="text-left p-3 whitespace-nowrap">班次</th>
                <th className="text-left p-3 min-w-[200px]">状态</th>
                <th className="text-left p-3 whitespace-nowrap text-xs">状态时间</th>
                <th className="text-right p-3 whitespace-nowrap">操作</th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 ? (
                <tr><td colSpan={8} className="p-6 text-center text-muted-foreground">暂无数据</td></tr>
              ) : rows.map((r) => (
                <tr key={r.id} className="border-t border-border align-top">
                  <td className="p-3 whitespace-nowrap">{r.id}</td>
                  <td className="p-3 max-w-[200px] truncate">{r.name || '—'}</td>
                  <td className="p-3 font-mono text-xs max-w-[260px] break-all text-muted-foreground">{r.routeType === 'sip' ? r.targetValue || '—' : '—'}</td>
                  <td className="p-3 whitespace-nowrap">{r.weight}</td>
                  <td className="p-3 whitespace-nowrap text-xs text-muted-foreground">{shiftScheduleSummary(r.shiftSchedule)}</td>
                  <td className="p-3 align-top"><Tag size="small">{workStateLabel(r.workState)}</Tag></td>
                  <td className="p-3 whitespace-nowrap text-xs text-muted-foreground">{r.workStateAt ? new Date(r.workStateAt).toLocaleString() : '—'}</td>
                  <td className="p-3 text-right">
                    <Space>
                      <Button type="outline" size="small" onClick={() => openEdit(r)}>编辑</Button>
                      <Button type="outline" status="danger" size="small" icon={<IconDelete />} onClick={() => { setAcdDeleteId(r.id); setAcdDeleteOpen(true) }}>删除</Button>
                    </Space>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="flex items-center justify-between p-3 border-t border-border text-sm">
            <span className="text-muted-foreground">总计: {total}</span>
            <Space>
              <Button size="small" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>上一页</Button>
              <Button size="small" disabled={page * pageSize >= total} onClick={() => setPage((p) => p + 1)}>下一页</Button>
            </Space>
          </div>
        </div>
      )}

      <Modal
        title={editingId == null ? '新增 SIP 目标' : '编辑目标'}
        visible={modalOpen}
        onCancel={closeModal}
        onOk={() => void save()}
        okText={saving ? '保存中...' : '保存'}
        confirmLoading={saving}
        style={{ width: 620 }}
      >
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          <div>
            <Typography.Text style={{ fontSize: 12 }}>名称</Typography.Text>
            <Input value={form.name} onChange={(v) => setForm((f) => ({ ...f, name: v }))} />
          </div>
          {(editingId == null || form.routeType === 'sip') && (
            <div>
              <Typography.Text style={{ fontSize: 12 }}>呼叫电话号</Typography.Text>
              <Input placeholder="例如 10086 或 13800138000" value={form.targetValue} onChange={(v) => setForm((f) => ({ ...f, targetValue: v }))} />
            </div>
          )}
          <div>
            <Typography.Text style={{ fontSize: 12 }}>权重</Typography.Text>
            <Input type="number" value={String(form.weight)} onChange={(v) => setForm((f) => ({ ...f, weight: parseInt(v, 10) || 0 }))} />
          </div>
          <div>
            <Typography.Text style={{ fontSize: 12 }}>工作状态</Typography.Text>
            <Select
              value={form.workState}
              onChange={(v) => setForm((f) => ({ ...f, workState: v as string }))}
              options={ACD_WORK_STATES.map((ws) => ({ label: workStateLabel(ws), value: ws }))}
            />
          </div>
          <div>
            <Typography.Text style={{ fontSize: 12 }}>接线班次（可选）</Typography.Text>
            <Typography.Paragraph style={{ margin: '4px 0 8px', fontSize: 11 }} type="secondary">
              不添加时段表示全天可接线。未勾选任何星期表示全周有效。跨午夜时段（如 22:00–06:00）已支持。判断时区由服务端 ACD_SHIFT_TIMEZONE 决定。
            </Typography.Paragraph>
            {form.shiftSegments.length === 0 ? (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>当前：全天</Typography.Text>
            ) : (
              <Space direction="vertical" style={{ width: '100%' }} size={10}>
                {form.shiftSegments.map((seg, i) => (
                  <div
                    key={i}
                    className="rounded-md border border-border bg-muted/30 p-3 space-y-2"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <Typography.Text style={{ fontSize: 12 }}>时段 {i + 1}</Typography.Text>
                      <Space size={6}>
                        <Button
                          type="text"
                          size="mini"
                          onClick={() =>
                            setForm((f) => ({
                              ...f,
                              shiftSegments: f.shiftSegments.map((s, j) =>
                                j === i ? { ...s, weekdays: [1, 2, 3, 4, 5] } : s,
                              ),
                            }))
                          }
                        >
                          填入工作日
                        </Button>
                        <Button
                          type="text"
                          size="mini"
                          status="danger"
                          icon={<IconDelete />}
                          onClick={() =>
                            setForm((f) => ({
                              ...f,
                              shiftSegments: f.shiftSegments.filter((_, j) => j !== i),
                            }))
                          }
                        >
                          删除
                        </Button>
                      </Space>
                    </div>
                    <div>
                      <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginBottom: 6 }}>
                        星期（留空=全周）
                      </Typography.Text>
                      <Checkbox.Group
                        value={seg.weekdays}
                        onChange={(v) =>
                          setForm((f) => ({
                            ...f,
                            shiftSegments: f.shiftSegments.map((s, j) =>
                              j === i ? { ...s, weekdays: (v as number[]).slice().sort((a, b) => a - b) } : s,
                            ),
                          }))
                        }
                        options={WEEKDAY_OPTS}
                      />
                    </div>
                    <Space wrap align="center" size={12}>
                      <Space size={6}>
                        <Typography.Text style={{ fontSize: 12 }}>开始</Typography.Text>
                        <TimePicker
                          format="HH:mm"
                          disableConfirm
                          style={{ width: 112 }}
                          value={hmToDayjs(seg.start)}
                          onChange={(vs, d) => {
                            const hm =
                              d?.isValid?.() === true
                                ? dayjsToHm(d)
                                : vs && validHm(vs)
                                  ? vs.trim()
                                  : seg.start
                            setForm((f) => ({
                              ...f,
                              shiftSegments: f.shiftSegments.map((s, j) =>
                                j === i ? { ...s, start: hm } : s,
                              ),
                            }))
                          }}
                        />
                      </Space>
                      <Space size={6}>
                        <Typography.Text style={{ fontSize: 12 }}>结束</Typography.Text>
                        <TimePicker
                          format="HH:mm"
                          disableConfirm
                          style={{ width: 112 }}
                          value={hmToDayjs(seg.end)}
                          onChange={(vs, d) => {
                            const hm =
                              d?.isValid?.() === true
                                ? dayjsToHm(d)
                                : vs && validHm(vs)
                                  ? vs.trim()
                                  : seg.end
                            setForm((f) => ({
                              ...f,
                              shiftSegments: f.shiftSegments.map((s, j) =>
                                j === i ? { ...s, end: hm } : s,
                              ),
                            }))
                          }}
                        />
                      </Space>
                    </Space>
                  </div>
                ))}
              </Space>
            )}
            <Button
              type="outline"
              size="small"
              style={{ marginTop: 10 }}
              icon={<IconPlus />}
              onClick={() =>
                setForm((f) => ({
                  ...f,
                  shiftSegments: [...f.shiftSegments, defaultShiftSegment()],
                }))
              }
            >
              添加时段
            </Button>
          </div>
        </Space>
      </Modal>

      <Modal
        title="确认删除号码池目标"
        visible={acdDeleteOpen}
        onOk={() => void confirmAcdDelete()}
        onCancel={() => { if (!acdDeleteLoading) { setAcdDeleteOpen(false); setAcdDeleteId(null) } }}
        okText="确认删除"
        okButtonProps={{ status: 'danger', loading: acdDeleteLoading }}
      >
        <Typography.Text>删除后不可恢复，确认继续吗？</Typography.Text>
      </Modal>
    </div>
  )
}

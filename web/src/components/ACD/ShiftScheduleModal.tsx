import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Button,
  Checkbox,
  Input,
  Modal,
  Radio,
  Space,
  TimePicker,
  Typography,
} from '@arco-design/web-react'
import { IconCopy, IconPlus } from '@arco-design/web-react/icon'
import dayjs, { type Dayjs } from 'dayjs'
import { showAlert } from '@/utils/notification'
import {
  applyTimeTypePreset,
  bulkApplyTimeRange,
  copyDaySchedule,
  dayGridToSegments,
  defaultTimeSlot,
  emptyDayGrid,
  parseShiftScheduleJSON,
  readShiftPresets,
  segmentsToDayGrid,
  serializeShiftSchedule,
  validHm,
  writeShiftPresets,
  type DaySchedule,
  type ShiftPreset,
  type ShiftSegment,
  type ShiftTimeType,
  type TimeSlot,
} from '@/utils/shiftSchedule'

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

const TIME_TYPE_OPTS: { value: ShiftTimeType; label: string; disabled?: boolean }[] = [
  { value: 'all', label: '全部时间' },
  { value: 'mon_fri', label: '周一到周五' },
  { value: 'week', label: '星期' },
  { value: 'holiday', label: '法定节假日', disabled: true },
  { value: 'workday', label: '工作日' },
  { value: 'custom', label: '自定义' },
]

type Props = {
  visible: boolean
  value: string
  onCancel: () => void
  onConfirm: (serialized: string) => void
}

export function ShiftScheduleModal({ visible, value, onCancel, onConfirm }: Props) {
  const [timeType, setTimeType] = useState<ShiftTimeType>('all')
  const [days, setDays] = useState<DaySchedule[]>(() => emptyDayGrid())
  const [bulkStart, setBulkStart] = useState('09:00')
  const [bulkEnd, setBulkEnd] = useState('18:00')
  const [presets, setPresets] = useState<ShiftPreset[]>(() => readShiftPresets())
  const [selectedPresetId, setSelectedPresetId] = useState<string | undefined>()
  const [presetManageOpen, setPresetManageOpen] = useState(false)
  const [newPresetName, setNewPresetName] = useState('')

  const resetFromValue = useCallback((raw: string) => {
    const segs = parseShiftScheduleJSON(raw)
    const { days: grid, timeType: inferred } = segmentsToDayGrid(segs)
    setDays(grid)
    setTimeType(inferred)
    if (segs.length === 1) {
      setBulkStart(segs[0]!.start)
      setBulkEnd(segs[0]!.end)
    }
  }, [])

  useEffect(() => {
    if (visible) resetFromValue(value)
  }, [visible, value, resetFromValue])

  const presetOptions = useMemo(
    () => presets.map((p) => ({ label: p.name, value: p.id })),
    [presets],
  )

  const applyPreset = (id: string | undefined) => {
    setSelectedPresetId(id)
    if (!id) return
    const hit = presets.find((p) => p.id === id)
    if (!hit) return
    const { days: grid, timeType: inferred } = segmentsToDayGrid(hit.segments)
    setDays(grid)
    setTimeType(inferred)
  }

  const saveCurrentAsPreset = () => {
    const name = newPresetName.trim()
    if (!name) {
      showAlert('请输入策略名称', 'warning')
      return
    }
    const segs = dayGridToSegments(days)
    const id = `preset-${Date.now()}`
    const next = [...presets, { id, name, segments: segs }]
    setPresets(next)
    writeShiftPresets(next)
    setSelectedPresetId(id)
    setNewPresetName('')
    showAlert('策略已保存', 'success')
  }

  const deletePreset = (id: string) => {
    const next = presets.filter((p) => p.id !== id)
    setPresets(next)
    writeShiftPresets(next)
    if (selectedPresetId === id) setSelectedPresetId(undefined)
  }

  const onTimeTypeChange = (t: ShiftTimeType) => {
    if (t === 'holiday') {
      showAlert('法定节假日需对接假日库，当前请使用「自定义」按天配置', 'info')
      return
    }
    setTimeType(t)
    setDays((prev) => applyTimeTypePreset(t, prev))
  }

  const updateDay = (weekday: number, patch: Partial<DaySchedule>) => {
    setDays((prev) => prev.map((d) => (d.weekday === weekday ? { ...d, ...patch } : d)))
    if (timeType !== 'custom' && timeType !== 'week') setTimeType('custom')
  }

  const updateSlot = (weekday: number, slotIdx: number, patch: Partial<TimeSlot>) => {
    setDays((prev) =>
      prev.map((d) => {
        if (d.weekday !== weekday) return d
        const slots = d.slots.map((s, i) => (i === slotIdx ? { ...s, ...patch } : s))
        return { ...d, slots }
      }),
    )
    if (timeType !== 'custom' && timeType !== 'week') setTimeType('custom')
  }

  const addSlot = (weekday: number) => {
    setDays((prev) =>
      prev.map((d) =>
        d.weekday === weekday
          ? { ...d, enabled: true, slots: [...d.slots, defaultTimeSlot()] }
          : d,
      ),
    )
    setTimeType('custom')
  }

  const copyFromPrev = (idx: number) => {
    if (idx <= 0) return
    setDays((prev) => {
      const from = prev[idx - 1]!
      const to = prev[idx]!
      return prev.map((d, i) => (i === idx ? copyDaySchedule(from, to) : d))
    })
    setTimeType('custom')
  }

  const handleConfirm = () => {
    if (timeType === 'all') {
      onConfirm('')
      return
    }
    const segs = dayGridToSegments(days)
    if (!segs.length) {
      showAlert('请至少勾选一天并填写有效时段', 'warning')
      return
    }
    for (let i = 0; i < segs.length; i++) {
      const s = segs[i]!
      if (!validHm(s.start) || !validHm(s.end)) {
        showAlert(`第 ${i + 1} 段班次时间须为 HH:mm`, 'error')
        return
      }
    }
    onConfirm(serializeShiftSchedule(segs))
  }

  const handleBulkApply = () => {
    if (!validHm(bulkStart) || !validHm(bulkEnd)) {
      showAlert('一键应用时间须为 HH:mm', 'warning')
      return
    }
    setDays((prev) => bulkApplyTimeRange(prev, bulkStart, bulkEnd, false))
    setTimeType('custom')
  }

  return (
    <>
      <Modal
        title="座席时间策略"
        visible={visible}
        onCancel={onCancel}
        style={{ width: 760 }}
        autoFocus={false}
        focusLock
        footer={
          <Space>
            <Button onClick={onCancel}>取消</Button>
            <Button type="primary" onClick={handleConfirm}>
              确定
            </Button>
          </Space>
        }
      >
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-3">
            <Typography.Text style={{ fontSize: 13, whiteSpace: 'nowrap' }}>选择策略</Typography.Text>
            <select
              className="h-8 min-w-[200px] rounded border border-border bg-background px-2 text-sm"
              value={selectedPresetId ?? ''}
              onChange={(e) => applyPreset(e.target.value || undefined)}
            >
              <option value="">请选择策略</option>
              {presetOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
            <Button type="outline" size="small" onClick={() => setPresetManageOpen(true)}>
              策略管理
            </Button>
          </div>

          <div>
            <Typography.Text style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>时间类型</Typography.Text>
            <Radio.Group
              type="button"
              value={timeType}
              onChange={(v) => onTimeTypeChange(v as ShiftTimeType)}
            >
              {TIME_TYPE_OPTS.map((o) => (
                <Radio key={o.value} value={o.value} disabled={o.disabled}>
                  {o.label}
                </Radio>
              ))}
            </Radio.Group>
          </div>

          {timeType !== 'all' && (
            <>
              <Typography.Paragraph style={{ margin: 0, fontSize: 12 }} type="secondary">
                <Typography.Text type="error">*</Typography.Text> 指定时间{' '}
                <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                  指定时间须勾选才可生效
                </Typography.Text>
              </Typography.Paragraph>

              <div className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-muted/20 px-3 py-2.5">
                <Typography.Text style={{ fontSize: 12 }}>一键应用</Typography.Text>
                <TimePicker
                  format="HH:mm"
                  disableConfirm
                  style={{ width: 108 }}
                  value={hmToDayjs(bulkStart)}
                  onChange={(_, d) => setBulkStart(d?.isValid?.() ? dayjsToHm(d) : bulkStart)}
                />
                <Typography.Text type="secondary">~</Typography.Text>
                <TimePicker
                  format="HH:mm"
                  disableConfirm
                  style={{ width: 108 }}
                  value={hmToDayjs(bulkEnd)}
                  onChange={(_, d) => setBulkEnd(d?.isValid?.() ? dayjsToHm(d) : bulkEnd)}
                />
                <Button type="primary" size="small" onClick={handleBulkApply}>
                  应用到全部星期
                </Button>
              </div>

              <div className="rounded-md border border-border overflow-hidden">
                {days.map((day, idx) => (
                  <div
                    key={day.weekday}
                    className="flex flex-wrap items-center gap-2 border-b border-border px-3 py-2.5 last:border-b-0 bg-card"
                  >
                    <Button
                      type="text"
                      size="mini"
                      icon={<IconCopy />}
                      disabled={idx === 0}
                      onClick={() => copyFromPrev(idx)}
                    >
                      复制
                    </Button>
                    <Checkbox
                      checked={day.enabled}
                      onChange={(checked) => updateDay(day.weekday, { enabled: checked })}
                    >
                      {day.label}
                    </Checkbox>
                    {day.slots.map((slot, slotIdx) => (
                      <Space key={slotIdx} size={6} align="center">
                        <TimePicker
                          format="HH:mm"
                          disableConfirm
                          style={{ width: 108 }}
                          disabled={!day.enabled}
                          value={hmToDayjs(slot.start)}
                          onChange={(_, d) =>
                            updateSlot(day.weekday, slotIdx, {
                              start: d?.isValid?.() ? dayjsToHm(d) : slot.start,
                            })
                          }
                        />
                        <Typography.Text type="secondary">~</Typography.Text>
                        <TimePicker
                          format="HH:mm"
                          disableConfirm
                          style={{ width: 108 }}
                          disabled={!day.enabled}
                          value={hmToDayjs(slot.end)}
                          onChange={(_, d) =>
                            updateSlot(day.weekday, slotIdx, {
                              end: d?.isValid?.() ? dayjsToHm(d) : slot.end,
                            })
                          }
                        />
                      </Space>
                    ))}
                    <Button
                      type="text"
                      size="mini"
                      icon={<IconPlus />}
                      disabled={!day.enabled}
                      onClick={() => addSlot(day.weekday)}
                    />
                  </div>
                ))}
              </div>
            </>
          )}

          {timeType === 'all' && (
            <Typography.Paragraph style={{ margin: 0, fontSize: 12 }} type="secondary">
              全部时间：不限制接线时段，坐席在班次外不会被自动标记离线。
            </Typography.Paragraph>
          )}

          <Typography.Paragraph style={{ margin: 0, fontSize: 11 }} type="secondary">
            跨午夜时段（如 22:00–06:00）已支持。判断时区由服务端 ACD_SHIFT_TIMEZONE 决定。
          </Typography.Paragraph>
        </div>
      </Modal>

      <Modal
        title="策略管理"
        visible={presetManageOpen}
        onCancel={() => setPresetManageOpen(false)}
        style={{ width: 480 }}
        footer={
          <Button onClick={() => setPresetManageOpen(false)}>关闭</Button>
        }
      >
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          <Space style={{ width: '100%' }}>
            <Input
              placeholder="新策略名称"
              value={newPresetName}
              onChange={setNewPresetName}
              style={{ flex: 1 }}
            />
            <Button type="primary" onClick={saveCurrentAsPreset}>
              保存当前配置
            </Button>
          </Space>
          {presets.length === 0 ? (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              暂无已保存策略，配置好时段后点击「保存当前配置」。
            </Typography.Text>
          ) : (
            presets.map((p) => (
              <div
                key={p.id}
                className="flex items-center justify-between rounded border border-border px-3 py-2"
              >
                <Typography.Text>{p.name}</Typography.Text>
                <Space>
                  <Button
                    type="text"
                    size="mini"
                    onClick={() => {
                      applyPreset(p.id)
                      setPresetManageOpen(false)
                    }}
                  >
                    应用
                  </Button>
                  <Button type="text" size="mini" status="danger" onClick={() => deletePreset(p.id)}>
                    删除
                  </Button>
                </Space>
              </div>
            ))
          )}
        </Space>
      </Modal>
    </>
  )
}

export type { ShiftSegment }

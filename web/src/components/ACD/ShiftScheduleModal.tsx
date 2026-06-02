import { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Checkbox,
  Modal,
  Radio,
  Space,
  TimePicker,
  Typography,
} from '@arco-design/web-react'
import { IconDelete, IconPlus } from '@arco-design/web-react/icon'
import dayjs, { type Dayjs } from 'dayjs'
import { showAlert } from '@/utils/notification'
import {
  applyTimeTypePreset,
  buildScheduleSegments,
  bulkApplyTimeRange,
  defaultHolidaySlots,
  defaultTimeSlot,
  emptyDayGrid,
  parseShiftScheduleJSON,
  segmentsToScheduleView,
  serializeShiftSchedule,
  TIME_TYPE_LABELS,
  validHm,
  type DaySchedule,
  type ShiftSegment,
  type ShiftTimeType,
  type TimeSlot,
} from '@/utils/shiftSchedule'
import './ShiftScheduleModal.css'

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

const TIME_TYPE_OPTS: { value: ShiftTimeType; label: string }[] = (
  ['all', 'mon_fri', 'workday', 'holiday', 'custom'] as ShiftTimeType[]
).map((value) => ({ value, label: TIME_TYPE_LABELS[value] }))

const TYPE_HINTS: Partial<Record<ShiftTimeType, string>> = {
  mon_fri: '周一至周五每天同一时段接线（不含法定节假日规则）。',
  workday: '按国务院工作日历：含调休补班日，排除法定节假日休息。',
  holiday: '仅在法定节假日及调休放假日内接线（如国庆、春节等）。',
  custom: '按星期自由勾选，可配置多段时段。',
}

type Props = {
  visible: boolean
  value: string
  onCancel: () => void
  onConfirm: (serialized: string) => void
}

function SlotLine({
  slot,
  disabled,
  onChange,
  onRemove,
  showRemove,
}: {
  slot: TimeSlot
  disabled?: boolean
  onChange: (patch: Partial<TimeSlot>) => void
  onRemove?: () => void
  showRemove?: boolean
}) {
  return (
    <div className="shift-schedule__slot-line">
      <TimePicker
        format="HH:mm"
        disableConfirm
        className="shift-schedule__time"
        disabled={disabled}
        value={hmToDayjs(slot.start)}
        onChange={(_, d) => onChange({ start: d?.isValid?.() ? dayjsToHm(d) : slot.start })}
      />
      <span className="shift-schedule__tilde">至</span>
      <TimePicker
        format="HH:mm"
        disableConfirm
        className="shift-schedule__time"
        disabled={disabled}
        value={hmToDayjs(slot.end)}
        onChange={(_, d) => onChange({ end: d?.isValid?.() ? dayjsToHm(d) : slot.end })}
      />
      <span className="shift-schedule__slot-actions">
        {showRemove && onRemove && (
          <Button
            type="text"
            size="mini"
            status="danger"
            icon={<IconDelete />}
            disabled={disabled}
            aria-label="删除时段"
            onClick={onRemove}
          />
        )}
      </span>
    </div>
  )
}

export function ShiftScheduleModal({ visible, value, onCancel, onConfirm }: Props) {
  const [timeType, setTimeType] = useState<ShiftTimeType>('all')
  const [days, setDays] = useState<DaySchedule[]>(() => emptyDayGrid())
  const [holidaySlots, setHolidaySlots] = useState<TimeSlot[]>(() => defaultHolidaySlots())
  const [bulkStart, setBulkStart] = useState('09:00')
  const [bulkEnd, setBulkEnd] = useState('18:00')

  const resetFromValue = useCallback((raw: string) => {
    const segs = parseShiftScheduleJSON(raw)
    const view = segmentsToScheduleView(segs)
    setDays(view.days)
    setTimeType(view.timeType)
    setHolidaySlots(view.holidaySlots)
    if (segs.length === 1) {
      setBulkStart(segs[0]!.start)
      setBulkEnd(segs[0]!.end)
    }
  }, [])

  useEffect(() => {
    if (visible) resetFromValue(value)
  }, [visible, value, resetFromValue])

  const onTimeTypeChange = (t: ShiftTimeType) => {
    setTimeType(t)
    setDays((prev) => applyTimeTypePreset(t, prev))
    if (t === 'holiday' && holidaySlots.length === 0) {
      setHolidaySlots(defaultHolidaySlots())
    }
  }

  const updateDay = (weekday: number, patch: Partial<DaySchedule>) => {
    setDays((prev) => prev.map((d) => (d.weekday === weekday ? { ...d, ...patch } : d)))
    if (timeType !== 'custom') setTimeType('custom')
  }

  const updateSlot = (weekday: number, slotIdx: number, patch: Partial<TimeSlot>) => {
    setDays((prev) =>
      prev.map((d) => {
        if (d.weekday !== weekday) return d
        const slots = d.slots.map((s, i) => (i === slotIdx ? { ...s, ...patch } : s))
        return { ...d, slots }
      }),
    )
    if (timeType !== 'custom') setTimeType('custom')
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

  const removeSlot = (weekday: number, slotIdx: number) => {
    setDays((prev) =>
      prev.map((d) => {
        if (d.weekday !== weekday) return d
        if (d.slots.length <= 1) return d
        const slots = d.slots.filter((_, i) => i !== slotIdx)
        return { ...d, slots: slots.length ? slots : [defaultTimeSlot()] }
      }),
    )
    setTimeType('custom')
  }

  const updateHolidaySlot = (slotIdx: number, patch: Partial<TimeSlot>) => {
    setHolidaySlots((prev) => prev.map((s, i) => (i === slotIdx ? { ...s, ...patch } : s)))
  }

  const addHolidaySlot = () => {
    setHolidaySlots((prev) => [...prev, defaultTimeSlot()])
  }

  const removeHolidaySlot = (slotIdx: number) => {
    setHolidaySlots((prev) => {
      if (prev.length <= 1) return prev
      const next = prev.filter((_, i) => i !== slotIdx)
      return next.length ? next : defaultHolidaySlots()
    })
  }

  const handleConfirm = () => {
    if (timeType === 'all') {
      onConfirm('')
      return
    }
    const segs = buildScheduleSegments(timeType, days, holidaySlots)
    if (!segs.length) {
      showAlert(
        timeType === 'holiday' ? '请至少添加一段法定节假日接线时段' : '请至少勾选一天并填写有效时段',
        'warning',
      )
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
    if (timeType === 'holiday') {
      setHolidaySlots([{ start: bulkStart, end: bulkEnd }])
      return
    }
    setDays((prev) => bulkApplyTimeRange(prev, bulkStart, bulkEnd, false))
    setTimeType('custom')
  }

  const showWeekGrid = timeType !== 'all' && timeType !== 'holiday'
  const showHolidayPanel = timeType === 'holiday'

  return (
    <Modal
      title="座席时间策略"
      visible={visible}
      onCancel={onCancel}
      className="shift-schedule-modal"
      style={{ width: 640, maxHeight: 'calc(100vh - 48px)' }}
      alignCenter
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
      <div className="shift-schedule">
        <div className="shift-schedule__section">
          <Typography.Text className="shift-schedule__label">时间类型</Typography.Text>
          <Radio.Group
            type="button"
            size="small"
            value={timeType}
            onChange={(v) => onTimeTypeChange(v as ShiftTimeType)}
          >
            {TIME_TYPE_OPTS.map((o) => (
              <Radio key={o.value} value={o.value}>
                {o.label}
              </Radio>
            ))}
          </Radio.Group>
        </div>

        {timeType === 'all' ? (
          <Typography.Text className="shift-schedule__hint">
            全部时间：不限制接线时段，坐席在班次外不会被自动标记离线。
          </Typography.Text>
        ) : (
          <>
            {TYPE_HINTS[timeType] && (
              <Typography.Text className="shift-schedule__hint">{TYPE_HINTS[timeType]}</Typography.Text>
            )}

            <div className="shift-schedule__bulk">
              <span className="shift-schedule__bulk-title">一键应用</span>
              <div className="shift-schedule__slot-line">
                <TimePicker
                  format="HH:mm"
                  disableConfirm
                  className="shift-schedule__time"
                  value={hmToDayjs(bulkStart)}
                  onChange={(_, d) => setBulkStart(d?.isValid?.() ? dayjsToHm(d) : bulkStart)}
                />
                <span className="shift-schedule__tilde">至</span>
                <TimePicker
                  format="HH:mm"
                  disableConfirm
                  className="shift-schedule__time"
                  value={hmToDayjs(bulkEnd)}
                  onChange={(_, d) => setBulkEnd(d?.isValid?.() ? dayjsToHm(d) : bulkEnd)}
                />
                <span className="shift-schedule__slot-actions" />
              </div>
              <Button type="primary" size="small" onClick={handleBulkApply}>
                {showHolidayPanel ? '应用到节假日' : '应用到全部星期'}
              </Button>
            </div>

            {showHolidayPanel && (
              <div className="shift-schedule__holiday-panel">
                <div className="shift-schedule__grid-head shift-schedule__grid-head--holiday">
                  <span>法定节假日接线时段</span>
                  <span />
                </div>
                <div className="shift-schedule__holiday-slots">
                  {holidaySlots.map((slot, slotIdx) => (
                    <div key={slotIdx} className="shift-schedule__holiday-row">
                      <SlotLine
                        slot={slot}
                        onChange={(patch) => updateHolidaySlot(slotIdx, patch)}
                        showRemove={holidaySlots.length > 1}
                        onRemove={() => removeHolidaySlot(slotIdx)}
                      />
                    </div>
                  ))}
                  <Button type="outline" size="small" icon={<IconPlus />} onClick={addHolidaySlot}>
                    添加时段
                  </Button>
                </div>
              </div>
            )}

            {showWeekGrid && (
              <>
                <div className="shift-schedule__grid-head" aria-hidden>
                  <span>星期</span>
                  <span>时段（可多段，如午休前后）</span>
                  <span />
                </div>

                <div className="shift-schedule__days">
                  {days.map((day) => (
                    <div
                      key={day.weekday}
                      className={`shift-schedule__day${day.enabled ? '' : ' shift-schedule__day--off'}`}
                    >
                      <div className="shift-schedule__day-label">
                        <Checkbox
                          checked={day.enabled}
                          onChange={(checked) => updateDay(day.weekday, { enabled: checked })}
                        >
                          {day.label}
                        </Checkbox>
                      </div>
                      <div className="shift-schedule__slots">
                        {day.slots.map((slot, slotIdx) => (
                          <SlotLine
                            key={slotIdx}
                            slot={slot}
                            disabled={!day.enabled}
                            onChange={(patch) => updateSlot(day.weekday, slotIdx, patch)}
                            showRemove={day.slots.length > 1}
                            onRemove={() => removeSlot(day.weekday, slotIdx)}
                          />
                        ))}
                      </div>
                      <div className="shift-schedule__day-add">
                        <Button
                          type="outline"
                          size="mini"
                          icon={<IconPlus />}
                          disabled={!day.enabled}
                          onClick={() => addSlot(day.weekday)}
                        >
                          时段
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              </>
            )}
          </>
        )}
      </div>
    </Modal>
  )
}

export type { ShiftSegment }

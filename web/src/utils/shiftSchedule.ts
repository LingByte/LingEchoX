/** Matches backend acd_shift_schedule: weekdays 0=Sun..6=Sat; optional calendar scope */
export type ShiftCalendarScope = '' | 'holiday' | 'workday' | 'weekend'

export type ShiftSegment = {
  weekdays: number[]
  start: string
  end: string
  calendar?: ShiftCalendarScope
}

export type TimeSlot = { start: string; end: string }

export type DaySchedule = {
  weekday: number
  label: string
  enabled: boolean
  slots: TimeSlot[]
}

export type ShiftTimeType =
  | 'all'
  | 'mon_fri'
  | 'holiday'
  | 'workday'
  | 'custom'

export const TIME_TYPE_LABELS: Record<ShiftTimeType, string> = {
  all: '全部时间',
  mon_fri: '周一到周五',
  holiday: '法定节假日',
  workday: '工作日',
  custom: '自定义',
}

export const WEEKDAY_ROWS: { weekday: number; label: string }[] = [
  { weekday: 1, label: '星期一' },
  { weekday: 2, label: '星期二' },
  { weekday: 3, label: '星期三' },
  { weekday: 4, label: '星期四' },
  { weekday: 5, label: '星期五' },
  { weekday: 6, label: '星期六' },
  { weekday: 0, label: '星期日' },
]

export function defaultTimeSlot(): TimeSlot {
  return { start: '09:00', end: '18:00' }
}

export function emptyDayGrid(): DaySchedule[] {
  return WEEKDAY_ROWS.map(({ weekday, label }) => ({
    weekday,
    label,
    enabled: false,
    slots: [defaultTimeSlot()],
  }))
}

export function defaultHolidaySlots(): TimeSlot[] {
  return [defaultTimeSlot()]
}

export function validHm(s: string): boolean {
  return /^([01]?\d|2[0-3]):([0-5]\d)$/.test(s.trim())
}

function normCalendar(raw: unknown): ShiftCalendarScope {
  const s = String(raw ?? '').trim().toLowerCase()
  if (s === 'holiday' || s === 'workday' || s === 'weekend') return s
  return ''
}

export function parseShiftScheduleJSON(raw: string): ShiftSegment[] {
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
      const calendar = normCalendar(o.calendar)
      out.push({ weekdays, start, end, ...(calendar ? { calendar } : {}) })
    }
    return out
  } catch {
    return []
  }
}

export function serializeShiftSchedule(segments: ShiftSegment[]): string {
  if (!segments.length) return ''
  return JSON.stringify(
    segments.map((s) => {
      const row: Record<string, unknown> = {
        weekdays: [...new Set(s.weekdays)].sort((a, b) => a - b),
        start: s.start.trim(),
        end: s.end.trim(),
      }
      if (s.calendar) row.calendar = s.calendar
      return row
    }),
  )
}

export type ShiftScheduleView = {
  days: DaySchedule[]
  timeType: ShiftTimeType
  holidaySlots: TimeSlot[]
}

export function segmentsToScheduleView(segments: ShiftSegment[]): ShiftScheduleView {
  if (segments.length === 0) {
    return { days: emptyDayGrid(), timeType: 'all', holidaySlots: defaultHolidaySlots() }
  }

  const holidayOnly = segments.every((s) => s.calendar === 'holiday')
  if (holidayOnly) {
    const slots = segments
      .filter((s) => validHm(s.start) && validHm(s.end))
      .map((s) => ({ start: s.start.trim(), end: s.end.trim() }))
    return {
      days: emptyDayGrid(),
      timeType: 'holiday',
      holidaySlots: slots.length ? slots : defaultHolidaySlots(),
    }
  }

  const slotMap = new Map<number, TimeSlot[]>()
  for (const wd of [0, 1, 2, 3, 4, 5, 6]) slotMap.set(wd, [])

  for (const seg of segments) {
    if (seg.calendar === 'holiday') continue
    const targetDays = seg.weekdays.length === 0 ? [0, 1, 2, 3, 4, 5, 6] : seg.weekdays
    const slot = { start: seg.start.trim(), end: seg.end.trim() }
    for (const wd of targetDays) {
      slotMap.get(wd)?.push({ ...slot })
    }
  }

  const days = WEEKDAY_ROWS.map(({ weekday, label }) => {
    const slots = slotMap.get(weekday) ?? []
    return {
      weekday,
      label,
      enabled: slots.length > 0,
      slots: slots.length > 0 ? slots : [defaultTimeSlot()],
    }
  })

  return {
    days,
    timeType: inferTimeType(days, segments),
    holidaySlots: defaultHolidaySlots(),
  }
}

/** @deprecated use segmentsToScheduleView */
export function segmentsToDayGrid(segments: ShiftSegment[]): { days: DaySchedule[]; timeType: ShiftTimeType } {
  const v = segmentsToScheduleView(segments)
  return { days: v.days, timeType: v.timeType }
}

function inferTimeType(days: DaySchedule[], segments: ShiftSegment[]): ShiftTimeType {
  if (segments.every((s) => s.calendar === 'holiday')) return 'holiday'
  if (segments.some((s) => s.calendar === 'workday')) return 'workday'

  const enabled = days.filter((d) => d.enabled)
  if (enabled.length === 0) return 'custom'

  const monFri = enabled.every((d) => d.weekday >= 1 && d.weekday <= 5) && enabled.length === 5
  const sameSlot =
    enabled.length > 0 &&
    enabled.every(
      (d) =>
        d.slots.length === 1 &&
        d.slots[0]!.start === enabled[0]!.slots[0]!.start &&
        d.slots[0]!.end === enabled[0]!.slots[0]!.end,
    )

  if (monFri && sameSlot && enabled[0]!.slots[0]!.start === '09:00' && enabled[0]!.slots[0]!.end === '18:00') {
    return 'mon_fri'
  }
  return 'custom'
}

export function buildScheduleSegments(
  timeType: ShiftTimeType,
  days: DaySchedule[],
  holidaySlots: TimeSlot[],
): ShiftSegment[] {
  switch (timeType) {
    case 'all':
      return []
    case 'holiday': {
      const out: ShiftSegment[] = []
      for (const slot of holidaySlots) {
        if (!validHm(slot.start) || !validHm(slot.end)) continue
        out.push({
          weekdays: [],
          start: slot.start.trim(),
          end: slot.end.trim(),
          calendar: 'holiday',
        })
      }
      return out
    }
    case 'workday':
      return dayGridToSegments(days).map((s) => ({ ...s, calendar: 'workday' }))
    default:
      return dayGridToSegments(days)
  }
}

export function dayGridToSegments(days: DaySchedule[]): ShiftSegment[] {
  const raw: ShiftSegment[] = []
  for (const day of days) {
    if (!day.enabled) continue
    for (const slot of day.slots) {
      if (!validHm(slot.start) || !validHm(slot.end)) continue
      raw.push({ weekdays: [day.weekday], start: slot.start.trim(), end: slot.end.trim() })
    }
  }
  return mergeSegments(raw)
}

function mergeSegments(segs: ShiftSegment[]): ShiftSegment[] {
  const groups = new Map<string, number[]>()
  for (const s of segs) {
    const cal = s.calendar ?? ''
    const key = `${cal}|${s.start}|${s.end}`
    const list = groups.get(key) ?? []
    for (const wd of s.weekdays) {
      if (!list.includes(wd)) list.push(wd)
    }
    groups.set(key, list)
  }
  const out: ShiftSegment[] = []
  for (const [key, wds] of groups) {
    const parts = key.split('|')
    const cal = parts[0] as ShiftCalendarScope
    const start = parts[1]!
    const end = parts[2]!
    const seg: ShiftSegment = { weekdays: wds.sort((a, b) => a - b), start, end }
    if (cal) seg.calendar = cal
    out.push(seg)
  }
  return out
}

export function applyTimeTypePreset(type: ShiftTimeType, days: DaySchedule[]): DaySchedule[] {
  switch (type) {
    case 'all':
      return emptyDayGrid()
    case 'mon_fri':
      return days.map((d) => ({
        ...d,
        enabled: d.weekday >= 1 && d.weekday <= 5,
        slots: [defaultTimeSlot()],
      }))
    case 'workday':
      return days.map((d) => ({
        ...d,
        enabled: d.weekday >= 1 && d.weekday <= 5,
        slots: [defaultTimeSlot()],
      }))
    case 'custom':
      return days.map((d) => ({
        ...d,
        enabled: true,
        slots: d.slots.length ? d.slots : [defaultTimeSlot()],
      }))
    case 'holiday':
      return emptyDayGrid()
    default:
      return days
  }
}

export function bulkApplyTimeRange(
  days: DaySchedule[],
  start: string,
  end: string,
  onlyChecked: boolean,
): DaySchedule[] {
  return days.map((d) => {
    if (onlyChecked && !d.enabled) return d
    return { ...d, enabled: true, slots: [{ start, end }] }
  })
}

export function shiftScheduleSummary(json?: string): string {
  if (!json?.trim()) return '全天'
  try {
    const segs = parseShiftScheduleJSON(json)
    if (!segs.length) return '全天'
    if (segs.every((s) => s.calendar === 'holiday')) {
      return `法定节假日 · ${segs.length} 段`
    }
    if (segs.some((s) => s.calendar === 'workday')) {
      const n = segs.filter((s) => s.calendar === 'workday').length
      return `工作日（含调休）· ${n} 段`
    }
    const { days } = segmentsToScheduleView(segs)
    const enabled = days.filter((d) => d.enabled).length
    const slots = days.reduce((n, d) => n + (d.enabled ? d.slots.length : 0), 0)
    const tt = inferTimeType(days, segs)
    if (tt === 'mon_fri') return `周一至周五 · ${slots} 段`
    return `${enabled} 天 · ${slots} 段`
  } catch {
    return '格式异常'
  }
}

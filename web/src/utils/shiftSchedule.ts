/** Matches backend acd_shift_schedule: weekdays 0=Sun..6=Sat; empty weekdays = all days */
export type ShiftSegment = { weekdays: number[]; start: string; end: string }

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
  | 'week'
  | 'holiday'
  | 'workday'
  | 'custom'

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

export function validHm(s: string): boolean {
  return /^([01]?\d|2[0-3]):([0-5]\d)$/.test(s.trim())
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
      out.push({ weekdays, start, end })
    }
    return out
  } catch {
    return []
  }
}

export function serializeShiftSchedule(segments: ShiftSegment[]): string {
  if (!segments.length) return ''
  return JSON.stringify(
    segments.map((s) => ({
      weekdays: [...new Set(s.weekdays)].sort((a, b) => a - b),
      start: s.start.trim(),
      end: s.end.trim(),
    })),
  )
}

export function segmentsToDayGrid(segments: ShiftSegment[]): { days: DaySchedule[]; timeType: ShiftTimeType } {
  const slotMap = new Map<number, TimeSlot[]>()
  for (const wd of [0, 1, 2, 3, 4, 5, 6]) slotMap.set(wd, [])

  if (segments.length === 0) {
    return { days: emptyDayGrid(), timeType: 'all' }
  }

  for (const seg of segments) {
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

  return { days, timeType: inferTimeType(days, segments) }
}

function inferTimeType(days: DaySchedule[], segments: ShiftSegment[]): ShiftTimeType {
  if (segments.length === 0) return 'all'

  const enabled = days.filter((d) => d.enabled)
  if (enabled.length === 0) return 'all'

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
  if (monFri && sameSlot) return 'workday'
  if (enabled.length === 7 && enabled.every((d) => d.slots.length === 1)) return 'week'
  return 'custom'
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
    const key = `${s.start}|${s.end}`
    const list = groups.get(key) ?? []
    for (const wd of s.weekdays) {
      if (!list.includes(wd)) list.push(wd)
    }
    groups.set(key, list)
  }
  const out: ShiftSegment[] = []
  for (const [key, wds] of groups) {
    const [start, end] = key.split('|')
    out.push({ weekdays: wds.sort((a, b) => a - b), start: start!, end: end! })
  }
  return out
}

export function applyTimeTypePreset(type: ShiftTimeType, days: DaySchedule[]): DaySchedule[] {
  switch (type) {
    case 'all':
      return emptyDayGrid()
    case 'mon_fri':
    case 'workday':
      return days.map((d) => ({
        ...d,
        enabled: d.weekday >= 1 && d.weekday <= 5,
        slots: [defaultTimeSlot()],
      }))
    case 'week':
    case 'custom':
      return days.map((d) => ({
        ...d,
        enabled: true,
        slots: d.slots.length ? d.slots : [defaultTimeSlot()],
      }))
    case 'holiday':
      return days
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

export function copyDaySchedule(from: DaySchedule, to: DaySchedule): DaySchedule {
  return {
    ...to,
    enabled: from.enabled,
    slots: from.slots.map((s) => ({ ...s })),
  }
}

export function shiftScheduleSummary(json?: string): string {
  if (!json?.trim()) return '全天'
  try {
    const segs = parseShiftScheduleJSON(json)
    if (!segs.length) return '全天'
    const { days } = segmentsToDayGrid(segs)
    const enabled = days.filter((d) => d.enabled).length
    const slots = days.reduce((n, d) => n + (d.enabled ? d.slots.length : 0), 0)
    return `${enabled} 天 · ${slots} 段`
  } catch {
    return '格式异常'
  }
}

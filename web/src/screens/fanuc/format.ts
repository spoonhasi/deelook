// How a Fanuc 0i-F/30i HMI writes values on its screens.
import type { Envelope } from '../../lib/types'

export function ok(e: Envelope | undefined): e is Envelope & { status: 0 } {
  return !!e && e.status === 0
}

export function num(e: Envelope | undefined): number | null {
  return ok(e) && typeof e.value === 'number' ? e.value : null
}

export function str(e: Envelope | undefined): string {
  return ok(e) && typeof e.value === 'string' ? e.value : ''
}

/** The i-th number of a range read (axis=1-3 and the like). */
export function numAt(e: Envelope | undefined, i: number): number | null {
  if (!ok(e) || !Array.isArray(e.value)) return null
  const v = e.value[i]
  return typeof v === 'number' ? v : null
}

export function strArray(e: Envelope | undefined): (string | null)[] {
  return ok(e) && Array.isArray(e.value) ? (e.value as (string | null)[]) : []
}

/** Axis names to draw (the position boxes have room for five). */
export function axesOf(v: Record<string, Envelope>): string[] {
  return strArray(v.axisName)
    .slice(0, 5)
    .map((n) => n ?? '')
}

/** Range reads carry no unit, so take it from the G20/G21 modal. */
export function unitOf(v: Record<string, Envelope>): string {
  return strArray(v.gModal).includes('G20') ? 'inch' : 'mm'
}

export function bool(e: Envelope | undefined): boolean {
  return ok(e) && e.value === true
}

/** An axis still has distance to go: the screen marks it with "*". */
export function moving(dtg: Envelope | undefined, i: number): boolean {
  const d = numAt(dtg, i)
  return d !== null && d !== 0
}

/** Bit `bit` of byte `byte` of a PMC byte list (plcValueList, uint8). */
export function pmcBit(e: Envelope | undefined, byte: number, bit: number): boolean | null {
  const b = numAt(e, byte)
  return b === null ? null : ((b >> bit) & 1) === 1
}

/** Position: 3 decimals in mm and degrees, 4 in inch (IS-B increment system). */
export function position(v: number | null, unit: unknown): string {
  if (v === null) return ''
  return v.toFixed(unit === 'inch' ? 4 : 3)
}

export function integer(v: number | null): string {
  return v === null ? '' : String(Math.round(v))
}

/** O number as the header shows it: "O0100". */
export function programNumber(name: string): string {
  return name || 'O0000'
}

/** Sequence number as the header shows it: "N00030". */
export function sequenceNumber(n: number | null): string {
  return 'N' + String(n ?? 0).padStart(5, '0')
}

/**
 * Aux modal (F M H D T S): blank when nothing is commanded. deemesh gives 0
 * for "not commanded" on Fanuc, and the panel leaves those fields empty.
 */
export function auxValue(v: number | null): string {
  if (v === null || v === 0) return ''
  return Number.isInteger(v) ? String(v) : String(+v.toFixed(4))
}

/** Elapsed time as "1H32M11S" / "0H 0M56S" (H, M, S drawn in another colour). */
export function duration(sec: number | null): { h: string; m: string; s: string } | null {
  if (sec === null) return null
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  return { h: String(h), m: String(m).padStart(2, ' '), s: String(s).padStart(2, ' ') }
}

/** "2026-10-14T03:53:41" → ["03", "53", "41"] (machine clock, no time zone). */
export function clock(iso: string): string[] {
  const t = iso.split('T')[1]
  return t ? t.split(':').slice(0, 3) : []
}

/** Mode field of the status line (operateMode). */
export function modeText(v: number | null): string {
  switch (v) {
    case 0: return 'JOG'
    case 1: return 'MDI'
    case 2: return 'MEM'
    case 6: return 'EDIT'
    case 7: return 'HND'
    case 8: return 'TJOG'
    case 9: return 'THND'
    case 10: return 'INC'
    case 11: return 'REF'
    case 12: return 'RMT'
    default: return '****'
  }
}

/** Automatic-operation field of the status line (executionStatus). */
export function runText(v: number | null): string {
  switch (v) {
    case 1: return 'STOP'
    case 2: return 'HOLD'
    case 3: return 'STRT'
    case 4: return 'MSTR'
    default: return '****'
  }
}

/** Axis-motion field of the status line (motionStatus). */
export function motionText(v: number | null): string {
  switch (v) {
    case 1: return 'MTN'
    case 2: return 'DWL'
    default: return '***'
  }
}

/**
 * Alarm field of the status line (alarmStatus + its desc), drawn white on
 * red. NC Guide showed "ALM" steadily while PS0010 was up.
 */
export function alarmText(v: number | null, desc: unknown): string {
  if (v === 2) return 'ALM'
  if (v === 1) return typeof desc === 'string' && /battery/i.test(desc) ? 'BAT' : 'ALM'
  return ''
}

/** Program text as the PROG screen shows it: each block followed by ";" (EOB). */
export function programLines(e: Envelope | undefined): string[] {
  const s = str(e)
  return s ? s.split('\n').map((l) => l + ';') : []
}

/** Work coordinate system number on the WORK page: EXT is 00, G54 01 ... */
export const WORK_SYSTEMS = ['EXT', 'G54', 'G55', 'G56', 'G57', 'G58', 'G59']

export interface AlarmItem {
  code: string
  message: string
  category: string
  severity: string
}

export function alarmItems(e: Envelope | undefined): AlarmItem[] {
  return ok(e) && Array.isArray(e.value) ? (e.value as AlarmItem[]) : []
}

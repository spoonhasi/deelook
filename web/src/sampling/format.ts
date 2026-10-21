// How the sampling pages write states, times, sizes and setups.
import type { Config } from './api'

export const STATE: Record<string, string> = {
  idle: '대기',
  starting: '시작하는 중',
  waiting: '시작 블록 대기',
  running: '수집 중',
  stopping: '종료하는 중',
  stopped: '종료됨',
  completed: '완료 (점 수 채움)',
  failed: '실패',
  interrupted: '중단됨 (서버 종료)',
}

export function duration(from?: string, to?: string | number): string {
  if (!from) return ''
  const end = typeof to === 'number' ? to : to ? Date.parse(to) : Date.now()
  let s = Math.max(0, Math.round((end - Date.parse(from)) / 1000))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  s %= 60
  return h ? `${h}시간 ${m}분 ${s}초` : m ? `${m}분 ${s}초` : `${s}초`
}
export const time = (iso?: string) => (iso ? new Date(iso).toLocaleString('ko-KR', { hour12: false }) : '')
export const clock = (iso: string) => new Date(iso).toLocaleTimeString('ko-KR', { hour12: false })
export const fmt = (n: number) => Math.round(n).toLocaleString('ko-KR')
export function bytes(samples: number[] = []): string {
  return size(samples.reduce((a, n) => a + n, 0) * 2)
}
// units of 1024, as Windows shows disk sizes
export function size(b: number): string {
  const u = [
    ['TB', 2 ** 40],
    ['GB', 2 ** 30],
    ['MB', 2 ** 20],
    ['KB', 2 ** 10],
  ] as const
  for (const [name, n] of u) if (b >= n) return `${(b / n).toFixed(b / n < 10 ? 1 : 0)} ${name}`
  return `${b} B`
}
export function seconds(us: number): string {
  const s = us / 1e6
  return s >= 60 ? `${(s / 60).toFixed(1)}분` : `${s.toFixed(s < 10 ? 2 : 1)}초`
}
export function how(c: Config): string {
  const parts = [c.mode === 'extended' ? '확장' : '기본', `${c.servoPeriodUs / 1000} ms`]
  parts.push(c.points ? `${fmt(c.points)}점` : '종료까지')
  if (c.startSeq) parts.push(`N${c.startSeq}부터`)
  if (c.trigger?.seq) parts.push(`책갈피 N${c.trigger.seq}`)
  if (c.trigger?.signal) parts.push(`책갈피 ${c.trigger.signal}${c.trigger.up ? '↑' : ''}${c.trigger.down ? '↓' : ''}`)
  return parts.join(' · ')
}

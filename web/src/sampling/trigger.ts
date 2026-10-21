import { TRG } from './api'

/** Trigger bits (the bookmarks) as the CNC reported them, in words; unknown bits as hex. */
export function trgText(bits: number): string {
  const out: string[] = []
  if (bits & TRG.seq) out.push('블록 N(SEQ)')
  if (bits & TRG.up) out.push('0→1(P_UP)')
  if (bits & TRG.down) out.push('1→0(P_DW)')
  if (bits & TRG.ended) out.push('수집 끝(STSM)')
  const rest = bits & ~(TRG.seq | TRG.up | TRG.down | TRG.ended)
  if (rest) out.push(`기타 0x${rest.toString(16).padStart(4, '0')}`)
  return `${out.join(' · ')} [0x${bits.toString(16).padStart(4, '0')}]`
}

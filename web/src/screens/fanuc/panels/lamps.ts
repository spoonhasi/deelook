// Lamps of the machine operator panel. With the "ncguide" profile they are
// the PMC outputs of NC Guide's standard panel ladder (Y0-Y8, found by
// pressing each key and watching the bytes). Without it, the lamps deemesh
// can tell machine-independently are lit and the rest stay dark.
import type { Envelope } from '../../../lib/types'
import * as f from '../format'

export type Lamp =
  | 'auto' | 'edit' | 'mdi' | 'remote' | 'ref' | 'jog' | 'inc' | 'handle'
  | 'sbk' | 'bdt' | 'opt' | 'teach' | 'x1' | 'x10' | 'x100' | 'x1000'
  | 'axX' | 'axY' | 'axZ' | 'ax4' | 'ax5' | 'ax6'
  | 'restart' | 'mlk' | 'dry' | 'plus' | 'rapid' | 'minus'
  | 'cycleStop' | 'cycleStart' | 'prgStop' | 'spCw' | 'spStop' | 'spCcw'

// [byte, bit] of Y0..Y8. X1-X1000, + / - and the spindle keys have no lamp
// output in that ladder.
const NCGUIDE: Partial<Record<Lamp, [number, number]>> = {
  auto: [0, 0], edit: [0, 1], mdi: [0, 2], remote: [0, 3],
  sbk: [0, 4], bdt: [0, 5], opt: [0, 6], teach: [0, 7],
  restart: [1, 0], mlk: [1, 1], dry: [1, 2],
  cycleStop: [2, 0], cycleStart: [2, 1], prgStop: [2, 2],
  ref: [2, 4], jog: [2, 5], inc: [2, 6], handle: [2, 7],
  axX: [5, 4], axY: [5, 5], axZ: [5, 6],
  ax4: [6, 0], ax5: [6, 1], ax6: [6, 2], rapid: [6, 5],
}

export function lampOn(lamp: Lamp, v: Record<string, Envelope>): boolean {
  if (f.str(v.lampProfile) === 'ncguide') {
    const at = NCGUIDE[lamp]
    return at ? f.pmcBit(v.pmcY, at[0], at[1]) === true : false
  }
  const mode = f.num(v.mode)
  const run = f.num(v.run)
  switch (lamp) {
    case 'auto': return mode === 2
    case 'edit': return mode === 6
    case 'mdi': return mode === 1
    case 'remote': return mode === 12
    case 'ref': return mode === 11
    case 'jog': return mode === 0 || mode === 8
    case 'inc': return mode === 10
    case 'handle': return mode === 7 || mode === 9
    case 'teach': return mode === 8 || mode === 9
    case 'sbk': return f.bool(v.singleBlock)
    case 'bdt': return f.bool(v.blockSkip)
    case 'mlk': return f.bool(v.machineLock)
    case 'dry': return f.bool(v.dryRun)
    case 'cycleStart': return run === 3
    case 'cycleStop': return run === 2
    default: return false
  }
}

// Which screen the web HMI shows, driven by the MDI function keys, the soft
// keys and the PAGE keys, as on the control (measured on NC Guide 0i-F).
// This only changes what the browser shows; nothing is sent to the machine.

export type Fn = 'pos' | 'prog' | 'ofs' | 'system' | 'message' | 'graph'
export type PosPage = 'abs' | 'rel' | 'all'

export interface SoftKey {
  label: string[] // one or two lines of six cells
  color?: string
  pressed?: boolean
  onpress?: () => void
}

interface PageDef {
  id: string
  label: string[]
  color?: string
  title: string // header (full screen) or function-box title (split screen)
  full?: boolean // uses the whole display instead of the right half
  supported?: boolean
}

const GREEN = '#008000'

const PAGES: Record<Fn, PageDef[]> = {
  pos: [
    { id: 'abs', label: ['ABSOLU', 'TE'], title: 'ACTUAL POSITION', full: true, supported: true },
    { id: 'rel', label: ['RELATI', 'VE'], title: 'ACTUAL POSITION', full: true, supported: true },
    { id: 'all', label: [' ALL'], title: 'ACTUAL POSITION', full: true, supported: true },
  ],
  prog: [
    { id: 'program', label: ['PROGRA', 'M'], title: 'PROGRAM', supported: true },
    { id: 'folder', label: ['FOLDER'], title: 'FOLDER' },
    { id: 'next', label: [' NEXT'], title: 'NEXT PROGRAM' },
    { id: 'check', label: ['CHECK'], title: 'PROGRAM CHECK' },
  ],
  ofs: [
    { id: 'offset', label: ['OFFSET'], title: 'OFFSET', full: true, supported: true },
    { id: 'setting', label: ['SETTIN', 'G'], title: 'SETTING (HANDY)' },
    { id: 'work', label: [' WORK'], title: 'WORK COORDINATES', supported: true },
  ],
  system: [
    { id: 'parameter', label: ['PARAME', 'TER'], title: 'PARAMETER' },
    { id: 'diagnosis', label: ['DIAGNO', 'SIS'], title: 'DIAGNOSIS' },
    { id: 'servo', label: ['SERVO', 'GUIDE'], color: GREEN, title: 'SERVO GUIDE' },
    { id: 'config', label: ['SYSTEM'], color: GREEN, title: 'SYSTEM CONFIG' },
  ],
  message: [
    { id: 'alarm', label: ['ALARM'], title: 'ALARM MESSAGE', full: true, supported: true },
    { id: 'message', label: ['MESSAG', 'E'], title: 'OPERATOR MESSAGE', supported: true },
    { id: 'history', label: ['HISTRY'], title: 'ALARM HISTORY', full: true },
  ],
  graph: [
    { id: 'param', label: [' DRAW', 'PARAM'], title: 'GRAPHIC PARAMETER(DYNAMIC GRAPHIC)' },
    { id: 'path', label: [' PATH', ' EXEC'], title: 'GRAPHIC PARAMETER(DYNAMIC GRAPHIC)' },
    { id: 'anime', label: ['ANIME', ' EXEC'], title: 'GRAPHIC PARAMETER(DYNAMIC GRAPHIC)' },
    { id: 'toolpos', label: [' TOOL', ' POS'], title: 'GRAPHIC PARAMETER(DYNAMIC GRAPHIC)' },
  ],
}

// the fifth right-hand soft key and the narrow key after it, per function
const OPRT: Record<Fn, { oprt?: string; plus: boolean }> = {
  pos: { oprt: GREEN, plus: false },
  prog: { oprt: GREEN, plus: true },
  ofs: { oprt: GREEN, plus: true },
  system: { oprt: GREEN, plus: true },
  message: { plus: true },
  graph: { oprt: GREEN, plus: false },
}

export const OFFSET_ROWS = 16
export const WORK_PAGES = 2

export class FanucNav {
  fn = $state<Fn>('pos')
  posPage = $state<PosPage>('abs')
  page = $state<Record<Fn, string>>({
    pos: 'abs',
    prog: 'program',
    ofs: 'offset',
    system: 'parameter',
    message: 'alarm',
    graph: 'param',
  })
  offsetPage = $state(1)
  offsetPages = $state(1) // known from the machine's tool offset count
  workPage = $state(1)

  current = $derived.by((): PageDef => {
    const id = this.fn === 'pos' ? this.posPage : this.page[this.fn]
    return PAGES[this.fn].find((p) => p.id === id) ?? PAGES[this.fn][0]
  })

  /** Whole display (POS, OFFSET, ALARM...) or position on the left half. */
  full = $derived(!!this.current.full)

  /** What the server has to read for this screen (see internal/screens/fanuc.go). */
  view = $derived.by((): { view: string; page?: number } => {
    const id = this.current.id
    if (this.fn === 'prog' && id === 'program') return { view: 'prog' }
    if (this.fn === 'ofs' && id === 'offset') return { view: 'offset', page: this.offsetPage }
    if (this.fn === 'ofs' && id === 'work') return { view: 'work' }
    if (this.fn === 'message') return { view: id === 'message' ? 'message' : 'alarm' }
    return { view: 'pos' }
  })

  /** An MDI function key. CUSTOM1/2 and HELP have no screen here. */
  functionKey(fn: Fn) {
    this.fn = fn
  }

  /** The control switches to the alarm screen when an alarm comes up. */
  alarmRaised() {
    this.fn = 'message'
    this.page.message = 'alarm'
  }

  pageKey(dir: 1 | -1) {
    if (this.fn === 'ofs' && this.current.id === 'offset') {
      this.offsetPage = Math.min(Math.max(this.offsetPage + dir, 1), this.offsetPages)
    } else if (this.fn === 'ofs' && this.current.id === 'work') {
      this.workPage = Math.min(Math.max(this.workPage + dir, 1), WORK_PAGES)
    }
  }

  /** The 12 soft keys: [0] left end, [1..5] left half, [6..10] right half, [11] right end. */
  softKeys = $derived.by((): SoftKey[] => {
    const keys: SoftKey[] = Array.from({ length: 12 }, () => ({ label: [] }))
    const posKeys = PAGES.pos.map((p) => ({
      label: p.label,
      pressed: this.posPage === p.id,
      onpress: () => (this.posPage = p.id as PosPage),
    }))
    if (this.fn === 'pos') {
      posKeys.forEach((k, i) => (keys[6 + i] = k))
    } else {
      if (!this.full) posKeys.forEach((k, i) => (keys[1 + i] = k))
      PAGES[this.fn].forEach((p, i) => {
        keys[6 + i] = {
          label: p.label,
          color: p.color,
          pressed: this.current.id === p.id,
          onpress: () => (this.page[this.fn] = p.id),
        }
      })
    }
    const o = OPRT[this.fn]
    if (o.oprt) keys[10] = { label: ['(OPRT)'], color: o.oprt }
    if (o.plus) keys[11] = { label: ['+'] }
    return keys
  })
}

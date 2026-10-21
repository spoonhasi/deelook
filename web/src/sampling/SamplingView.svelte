<script lang="ts">
  // Time-series collection with FOCAS2 cnc_sdt*: every option the calls
  // offer, so the page shows what the CNC's sampler can do.
  //   basic     cnc_sdtsetchnl/startsmpl/readsmpl/endsmpl/cancelsmpl: servo
  //             and spindle data, until stopped or N points, at once or from
  //             block N; cnc_sdtread1shot for present values
  //   extended  cnc_sdtsetchnl2/startsmpl2/readsmpl2/endsmpl2: also PMC
  //             signals and CNC information, and trigger marks
  import { onMount } from 'svelte'
  import {
    api,
    type ChannelConfig,
    type Config,
    type DataType,
    type Kind,
    type Library,
    type Live,
    type MachineInfo,
    type Mode,
    type HubState,
    type OneShotResult,
    type SessionInfo,
    type Storage,
    type Status,
    type TypedKind,
    type Types,
  } from './api'
  import BitChart from './BitChart.svelte'
  import LiveChart from './LiveChart.svelte'
  import SessionViewer from './SessionViewer.svelte'
  import Reference from './Reference.svelte'
  import HelpButton from '../lib/HelpButton.svelte'
  import Modal from '../lib/Modal.svelte'
  import { explainError, PMC_KINDS } from './reference'
  import { trgText } from './trigger'

  const MAX_CHANNELS = 8
  const PMC_UNITS = [1, 2, 3, 4, 5, 9]
  const KIND: Record<Kind, string> = {
    servo: '서보',
    spindle: '주축',
    pmcbit: 'PMC 비트',
    pmcbyte: 'PMC 바이트',
    pmcword: 'PMC 워드',
    cnc: 'CNC 정보',
  }
  const BASIC_KINDS: Kind[] = ['servo', 'spindle']
  const EXT_KINDS: Kind[] = ['servo', 'spindle', 'pmcbit', 'pmcbyte', 'pmcword', 'cnc']

  let types = $state<Types>({ servo: [], spindle: [], cnc: [], pmcKinds: 'GFYXARTKCDMNEZ' })
  let machines = $state<MachineInfo[]>([])
  let loaded = $state(false) // the machine list came
  let hub = $state<HubState>({ ok: true })
  let library = $state<Library | null>(null)
  let machineId = $state('')
  let statuses = $state<Status[]>([])
  let sessions = $state<SessionInfo[]>([])
  let live = $state<Live>({ channels: [], events: [] })
  let busy = $state(false)
  let message = $state('')
  let now = $state(Date.now())
  let viewing = $state<SessionInfo | null>(null) // recorded session drawn in full

  // the form; turned into a Config on start
  let form = $state({
    mode: 'basic' as Mode,
    servoPeriodUs: 1000,
    fixed: false,
    points: 5000,
    path: 1,
    waitBlock: false,
    startSeq: 100,
    trgSeqOn: false,
    trgSeq: 100,
    trgSigOn: true,
    trgSignal: 'Y0.4',
    trgUnit: 1,
    trgUp: true,
    trgDown: true,
    channels: [] as ChannelConfig[],
  })

  const machine = $derived(machines.find((m) => m.machine === machineId))

  // the machines come from two lists, one shown at a time: the hub's and
  // those added here by address. The list and the machine picked in each are
  // kept in this browser.
  type Source = 'hub' | 'local'
  let source = $state<Source>('hub')
  const hubMachines = $derived(machines.filter((m) => m.source !== 'local'))
  const localMachines = $derived(machines.filter((m) => m.source === 'local'))
  const shown = $derived(source === 'hub' ? hubMachines : localMachines)
  const PICK_KEY = 'deelook.sampling.pick'
  let picked: Record<Source, string> = { hub: '', local: '' }
  function loadPick(): { source?: Source; hub?: string; local?: string } {
    try {
      return JSON.parse(localStorage.getItem(PICK_KEY) ?? '{}')
    } catch {
      return {}
    }
  }
  function savePick() {
    try {
      localStorage.setItem(PICK_KEY, JSON.stringify({ source, ...picked }))
    } catch {
      // private window: remembered for this page only
    }
  }
  /** Shows a list and the machine last picked in it (or its first). */
  async function selectSource(src: Source) {
    source = src
    const list = src === 'hub' ? hubMachines : localMachines
    const id = list.find((m) => m.machine === picked[src])?.machine ?? list[0]?.machine ?? ''
    if (id) {
      await selectMachine(id)
    } else {
      machineId = ''
      viewing = null
      sessions = []
      live = { channels: [], events: [] }
      shot = null
      savePick()
    }
  }
  // machines that have not sampled since deelook started are not listed: idle
  const status = $derived<Status>(statuses.find((s) => s.machine === machineId) ?? { machine: machineId, state: 'idle' })
  const active = $derived(!!status && ['starting', 'waiting', 'running', 'stopping'].includes(status.state))
  const ext = $derived(form.mode === 'extended')
  const extOk = $derived(!!library?.extended)
  // without the FOCAS2 library nothing reaches the CNC: the page still shows
  // the machines, recorded sessions and the reference
  const libOk = $derived(library?.available !== false)

  const typed = (k: Kind): k is TypedKind => k === 'servo' || k === 'spindle' || k === 'cnc'
  const typeOf = (kind: Kind, t: number) => (typed(kind) ? types[kind].find((d) => d.type === t) : undefined)
  // when a machine's axis names are not known: numbers, as many as FOCAS allows
  const generic = (prefix: string, n: number) => Array.from({ length: n }, (_, i) => `${prefix}${i + 1}`)
  const axisNames = (kind: Kind) =>
    kind === 'servo'
      ? machine?.axes?.length ? machine.axes : generic('축', 32)
      : machine?.spindles?.length ? machine.spindles : generic('S', 8)
  const axisName = (kind: Kind, axis: number) => axisNames(kind)[axis - 1] ?? `${kind === 'servo' ? '축' : 'S'}${axis}`
  const pathCount = $derived(machine?.paths || 10)

  function labelOf(c: ChannelConfig): string {
    switch (c.kind) {
      case 'servo':
      case 'spindle':
        return `${axisName(c.kind, c.axis)} ${typeOf(c.kind, c.type)?.code ?? c.type}`
      case 'cnc':
        return `P${c.axis} ${typeOf('cnc', c.type)?.code ?? c.type}`
      case 'pmcbit':
        return 'PMC'
      default:
        return `${(c.signals ?? []).join(' ')} ${c.kind === 'pmcbyte' ? 'byte' : 'word'}`
    }
  }
  function unitOf(c: ChannelConfig): string {
    if (c.kind === 'pmcbyte') return 'PMC 바이트 0-255'
    if (c.kind === 'pmcword') return 'PMC 워드 0-65535'
    if (c.kind === 'pmcbit') return 'PMC 비트'
    return typeOf(c.kind, c.type)?.unit ?? ''
  }

  function newChannel(kind: Kind = 'servo'): ChannelConfig {
    const c: ChannelConfig = { kind, axis: 1, type: 0, shift: 0, label: '' }
    setKind(c, kind)
    return c
  }

  function setKind(c: ChannelConfig, kind: Kind) {
    c.kind = kind
    c.axis = 1
    c.shift = 0
    delete c.signals
    delete c.unit
    if (typed(kind)) {
      const code = { servo: 'TCMD', spindle: 'SPEED', cnc: 'NNO' }[kind]
      const t = types[kind].find((d) => d.code === code) ?? types[kind][0]
      c.type = t?.type ?? 0
      c.shift = t?.shift ?? 0
    } else {
      c.type = 0
      c.signals = kind === 'pmcbit' ? ['Y0.4'] : ['R100']
      c.unit = 1
    }
  }
  /** Kinds to offer: the mode's, plus the channel's own if the mode lacks it. */
  // every kind in both modes (those of the extended mode off in basic), so
  // the menu keeps its width and switching the mode moves nothing
  const kindOff = (k: Kind) => !ext && !BASIC_KINDS.includes(k)

  /** Why a channel of kind cannot be added now ('' if it can). */
  function canAdd(kind: Kind): string {
    if (locked) return '수집 중에는 바꿀 수 없음'
    if (form.channels.length >= MAX_CHANNELS) return '채널은 8개까지'
    if (!ext && !BASIC_KINDS.includes(kind)) return '확장 방식에서만 (cnc_sdtsetchnl2)'
    return ''
  }
  function addFromReference(kind: Kind, type: number) {
    const c = newChannel(kind)
    setType(c, type)
    form.channels.push(c)
  }

  function setType(c: ChannelConfig, t: number) {
    c.type = t
    c.shift = typeOf(c.kind, t)?.shift ?? 0
  }
  function setSignals(c: ChannelConfig, text: string) {
    c.signals = text
      .split(/[\s,]+/)
      .map((s) => s.trim().toUpperCase())
      .filter(Boolean)
  }

  function toConfig(): Config {
    const f = form
    return {
      mode: f.mode,
      servoPeriodUs: f.servoPeriodUs,
      points: f.fixed ? f.points : 0,
      path: f.path,
      startSeq: !ext && f.waitBlock ? f.startSeq : 0,
      trigger: {
        seq: ext && f.trgSeqOn ? f.trgSeq : 0,
        signal: ext && f.trgSigOn ? f.trgSignal.trim().toUpperCase() : '',
        unit: f.trgUnit,
        up: f.trgUp,
        down: f.trgDown,
      },
      channels: f.channels.map((c) => ({ ...c, label: labelOf(c) })),
    }
  }

  function fromConfig(c?: Config) {
    if (!c?.channels?.length) {
      form.channels = [newChannel()]
      return
    }
    form.mode = c.mode === 'extended' && extOk ? 'extended' : 'basic'
    form.servoPeriodUs = c.servoPeriodUs || 1000
    form.fixed = !!c.points
    if (c.points) form.points = c.points
    form.path = c.path || 1
    form.waitBlock = !!c.startSeq
    if (c.startSeq) form.startSeq = c.startSeq
    if (c.trigger) {
      form.trgSeqOn = !!c.trigger.seq
      if (c.trigger.seq) form.trgSeq = c.trigger.seq
      form.trgSigOn = !!c.trigger.signal
      if (c.trigger.signal) form.trgSignal = c.trigger.signal
      form.trgUnit = c.trigger.unit || 1
      form.trgUp = c.trigger.up
      form.trgDown = c.trigger.down
    }
    form.channels = c.channels
      .filter((ch) => form.mode === 'extended' || BASIC_KINDS.includes(ch.kind))
      .map((ch) => ({ ...ch, signals: ch.signals ? [...ch.signals] : undefined }))
    if (!form.channels.length) form.channels = [newChannel()]
  }

  // what is wrong with the form, in words; the server checks again
  // an extended sampling with a number of points asks the CNC for this many
  // more and drops them: they take the 0s the CNC ends PMC and CNC
  // information channels with (endPad on the server)
  const END_PAD = 16
  const BIT = /^[GFYXARTKCDMNEZ]\d{1,5}\.[0-7]$/
  const WORD = /^[GFYXARTKCDMNEZ]\d{1,5}$/
  const problems = $derived.by(() => {
    const p: string[] = []
    const f = form
    const kinds = f.channels.map((c) => c.kind)
    if (!f.channels.length) p.push('채널을 하나 이상 넣으세요.')
    if (ext && !extOk) p.push('이 FOCAS2 라이브러리에는 cnc_sdt*2 함수가 없어 확장 방식을 쓸 수 없습니다.')
    if (!ext && kinds.some((k) => !BASIC_KINDS.includes(k)))
      p.push('PMC 신호·CNC 정보 채널은 확장 방식(cnc_sdtsetchnl2)에서만 쓸 수 있습니다.')
    if (f.fixed && !(f.points >= 8)) p.push('점 수는 8 이상이어야 합니다.')
    if (f.servoPeriodUs === 250 && kinds.includes('servo') && kinds.includes('spindle') && f.fixed)
      p.push('0.25 ms 서보와 주축을 함께 모을 때는 점 수를 정할 수 없습니다(CNC 제약). "종료할 때까지"를 쓰세요.')
    if (kinds.filter((k) => k === 'pmcbit').length > 1) p.push('PMC 비트 신호는 한 채널에 모아 넣습니다(최대 16개).')
    f.channels.forEach((c, i) => {
      if (!c.kind.startsWith('pmc')) return
      const sig = c.signals ?? []
      const max = c.kind === 'pmcbit' ? 16 : 1
      if (!sig.length || sig.length > max) p.push(`채널 ${i + 1}: PMC 주소를 ${max === 1 ? '1개' : '1~16개'} 넣으세요.`)
      const re = c.kind === 'pmcbit' ? BIT : WORD
      const bad = sig.filter((s) => !re.test(s))
      if (bad.length) p.push(`채널 ${i + 1}: 주소 형식이 틀립니다 (${bad.join(', ')}) — ${c.kind === 'pmcbit' ? '예: Y0.4' : '예: R100'}`)
    })
    const bitCh = f.channels.find((c) => c.kind === 'pmcbit')
    if (bitCh && kinds.some((k) => k === 'pmcbyte' || k === 'pmcword') && (bitCh.signals ?? []).length > 8)
      p.push('PMC 바이트·워드 채널이 있으면 비트 채널의 신호는 8개까지입니다.')
    if (ext && f.trgSigOn && PMC_KINDS.some((k) => !k.trigger && f.trgSignal.trim().toUpperCase().startsWith(k.letter)))
      p.push('T(타이머)·C(카운터)·Z 주소는 책갈피 신호로 쓸 수 없습니다.')
    if (ext && f.trgSigOn) {
      if (!BIT.test(f.trgSignal.trim().toUpperCase())) p.push('책갈피 신호는 R9091.6 같은 PMC 비트 주소여야 합니다.')
      if (!f.trgUp && !f.trgDown) p.push('책갈피를 남길 변화(0→1, 1→0)를 하나 이상 고르세요.')
    }
    if (!ext && f.waitBlock && !(f.startSeq > 0)) p.push('시작 블록 번호를 넣으세요.')
    if (ext && f.trgSeqOn && !(f.trgSeq > 0)) p.push('책갈피 블록 번호를 넣으세요.')
    return p
  })

  // read1shot works on servo and spindle channels only
  const shotChannels = $derived(form.channels.filter((c) => BASIC_KINDS.includes(c.kind)))

  const SHIFT_TIP =
    '오른쪽 시프트 n (0~15): CNC가 값을 2ⁿ으로 나눠 16비트(-32768~32767)에 담아 보냅니다.\n' +
    '데이터를 고르면 FANUC 권장값으로 바뀝니다 (대부분 0, 속도 지령 9, 속도 피드백 3).\n' +
    '크게 하면 큰 값도 넘치지 않지만 거칠어지고, 작게 하면 세밀하지만 큰 값이 넘쳐 엉뚱한 값이 됩니다.'

  /** What a channel's data is, for its menu's tooltip (the table is in the reference below). */
  function typeTip(d: DataType | undefined, shift: number): string {
    if (!d) return ''
    const lines = [`${d.nameKo} (${d.name})`]
    for (const x of d.units) lines.push(`단위: ${x.raw}${x.means ? ` = ${x.means}` : ''}${x.when ? ` (${x.when})` : ''}`)
    if (d.note) lines.push(d.note)
    if (shift) lines.push(`시프트 ${shift}: 받은 값 × ${2 ** shift} = 위 단위의 값`)
    return lines.join('\n')
  }

  /** A PMC address as FOCAS2 takes it, PMC_DATA: R9091.6 is {unittype 1, adr 5 (R), bit 6, no 9091}. */
  function pmcData(addr: string, unit: number): string {
    const m = /^([A-Z])(\d+)(?:\.([0-7]))?$/.exec(addr.trim().toUpperCase())
    const k = m ? types.pmcKinds.indexOf(m[1]) : -1
    if (!m || k < 0) return addr
    return `{unittype ${unit}, adr ${k} (${m[1]}), bit ${m[3] ?? 0}, no ${Number(m[2])}}`
  }

  /** The bookmark example: a PMC system relay the PMC itself flips every 0.5 s. */
  function bookmarkExample() {
    Object.assign(form, { trgSigOn: true, trgSignal: 'R9091.6', trgUnit: 1, trgUp: true, trgDown: true })
  }

  /** The FOCAS2 calls this setup makes, with its arguments. */
  const calls = $derived.by(() => {
    const f = form
    const n = f.channels.length
    const points = f.fixed ? f.points : -1
    if (!ext)
      return [
        ['cnc_allclibhndl3', '연결 (deemesh와 별도 핸들)'],
        ['cnc_sdtsetchnl', `채널 ${n}개, datanum ${points}${points < 0 ? ' (종료할 때까지)' : ' 점'}`],
        ['cnc_sdtstartsmpl', `path ${f.path}, seqno ${f.waitBlock ? f.startSeq : 0}${f.waitBlock ? ' (그 블록이 실행되면 시작)' : ' (즉시)'}`],
        ['cnc_sdtreadsmpl', '반복 — stat -1 시작 대기 / 1 수집 중 / 0 끝'],
        points > 0
          ? ['cnc_sdtendsmpl', 'CNC가 다 모은 뒤 (stat 0), 그 전에 수집 종료 버튼을 누르면 cnc_sdtcancelsmpl']
          : ['cnc_sdtcancelsmpl', '수집 종료 버튼'],
        ['cnc_freelibhndl', ''],
      ]
    const cond = (f.trgSeqOn ? 1 : 0) | (f.trgSigOn && f.trgUp ? 2 : 0) | (f.trgSigOn && f.trgDown ? 4 : 0)
    return [
      ['cnc_allclibhndl3', '연결 (deemesh와 별도 핸들)'],
      [
        'cnc_sdtsetchnl2',
        `채널 ${n}개 (IDBSDTCHAN2: PMC 주소 io[16]), datanum ` +
          (points < 0 ? `${points} (종료할 때까지)` : `${points + END_PAD} (${points}점 + 끝의 0을 버릴 ${END_PAD}점)`) +
          (f.channels.some((c) => c.kind === 'pmcbit') ? ' — PMC 비트 채널은 맨 뒤로 보냄' : ''),
      ],
      [
        'cnc_sdtstartsmpl2',
        `path ${f.path}, condition 0x${cond.toString(16).padStart(4, '0')}` +
          (cond ? `, TRG_DATA {seq_no ${f.trgSeqOn ? f.trgSeq : 0}, pmc_trg ${f.trgSigOn ? pmcData(f.trgSignal, f.trgUnit) : '-'}}` : ' (책갈피 없음)'),
      ],
      ['cnc_sdtreadsmpl2', '반복 — trg 비트: SEQ / P_UP / P_DW 표시, STSM = 다 모음'],
      [
        'cnc_sdtendsmpl2',
        (points > 0 ? 'STSM(다 모음) 뒤 또는 ' : '') + '수집 종료 버튼, 읽기 실패 뒤에도 반드시 (안 하면 CNC 샘플러가 멈춤)',
      ],
      ['cnc_freelibhndl', ''],
    ]
  })

  async function loadSessions() {
    if (!machineId) return
    try {
      ;[sessions, storage] = await Promise.all([api.sessions(machineId), api.storage().catch(() => storage)])
    } catch (e) {
      message = String((e as Error).message)
    }
  }

  // the data folder's disk, refreshed with the session list
  let storage = $state<Storage | null>(null)
  const lowDisk = $derived(!!storage && storage.minFree > 0 && storage.free > 0 && storage.free < storage.minFree)

  async function deleteSession(s: SessionInfo) {
    if (!confirm(`세션 #${s.id} (${s.file}, ${size(s.size)})을 지울까요? 파일이 지워지고 되돌릴 수 없습니다.`)) return
    try {
      await api.deleteSession(s.id)
      if (viewing?.id === s.id) viewing = null
      await loadSessions()
    } catch (e) {
      message = String((e as Error).message)
    }
  }

  async function loadLive() {
    try {
      live = await api.live(machineId)
    } catch {
      // keep the last picture
    }
  }

  async function selectMachine(id: string) {
    machineId = id
    const m = machines.find((x) => x.machine === id)
    if (m) {
      source = m.source === 'local' ? 'local' : 'hub'
      picked[source] = id
      savePick()
    }
    viewing = null
    live = { channels: [], events: [] }
    shot = null
    await loadSessions()
    fromConfig(sessions.find((s) => s.config?.channels?.length)?.config)
    if (statuses.find((s) => s.machine === id)?.session) void loadLive()
  }

  async function loadMachines() {
    const m = await api.machines()
    machines = m.machines
    hub = m.hub
    library = m.library ?? null
    loaded = true
  }

  onMount(async () => {
    try {
      const [t] = await Promise.all([api.types(), loadMachines()])
      types = t
      statuses = await api.status().catch(() => [])
      const p = loadPick()
      picked = { hub: p.hub ?? '', local: p.local ?? '' }
      // the list used last time, unless it is empty and the other is not
      let src: Source = p.source === 'local' ? 'local' : 'hub'
      if (!(src === 'hub' ? hubMachines : localMachines).length && (src === 'hub' ? localMachines : hubMachines).length)
        src = src === 'hub' ? 'local' : 'hub'
      await selectSource(src)
    } catch (e) {
      message = String((e as Error).message)
    }
  })

  // machines added here by address (the add and change dialogs), and axis
  // names read over FOCAS2. Addresses go to the server and never come back:
  // the change dialog takes a new one, it cannot show the current one.
  let adding = $state({ name: '', host: '', port: 8193 })
  let editing = $state<{ name: string; host: string; port: number | null }>({ name: '', host: '', port: null })
  let addOpen = $state(false)
  let editOpen = $state(false)
  let manageBusy = $state(false)
  let manageMsg = $state('')
  /** Runs a machine change and reloads the list; false when it failed (manageMsg says why). */
  async function manage(what: () => Promise<string | void>): Promise<boolean> {
    manageBusy = true
    manageMsg = ''
    try {
      const id = await what()
      await loadMachines()
      if (id) await selectMachine(id)
      return true
    } catch (e) {
      manageMsg = String((e as Error).message)
      return false
    } finally {
      manageBusy = false
    }
  }
  function openAdd() {
    adding = { name: '', host: '', port: 8193 }
    manageMsg = ''
    addOpen = true
  }
  function openEdit() {
    editing = { name: machine?.name ?? '', host: '', port: null }
    manageMsg = ''
    editOpen = true
  }
  async function addMachine() {
    const ok = await manage(async () => (await api.addMachine(adding.name, adding.host, Number(adding.port))).machine)
    if (ok) addOpen = false
  }
  async function saveMachine() {
    const id = machineId
    const ok = await manage(async () => {
      await api.updateMachine(id, editing.name, editing.host, Number(editing.port) || 0)
      return id
    })
    if (ok) editOpen = false
  }
  async function deleteMachine() {
    if (!machine || !confirm(`${machine.name} (${machine.machine})을(를) 목록에서 지울까요?\n이 기계로 기록한 세션 파일은 지워지지 않습니다.`))
      return
    const gone = machineId
    const ok = await manage(async () => {
      await api.deleteMachine(gone)
      machines = machines.filter((m) => m.machine !== gone)
      await selectSource('local') // the next machine added here, or none
    })
    if (ok) editOpen = false
  }
  const readNames = () =>
    manage(async () => {
      await api.readNames(machineId)
    })

  // state of every machine, every second
  $effect(() => {
    let stop = false
    const tick = async () => {
      try {
        statuses = await api.status()
      } catch {
        // the server restarted or the network dropped; try again
      }
      now = Date.now()
      if (!stop) timer = setTimeout(tick, 1000)
    }
    let timer = setTimeout(tick, 0)
    return () => {
      stop = true
      clearTimeout(timer)
    }
  })

  // the newest samples while this machine is sampling
  $effect(() => {
    const id = machineId
    if (!id || !active) return
    let stop = false
    const tick = async () => {
      await loadLive()
      if (!stop) timer = setTimeout(tick, 500)
    }
    let timer = setTimeout(tick, 0)
    return () => {
      stop = true
      clearTimeout(timer)
    }
  })

  // a session that ended (the CNC finished, another browser, a failure):
  // its last picture and the session list
  let lastState = ''
  $effect(() => {
    const st = status?.state ?? ''
    if (lastState && st !== lastState && ['stopped', 'completed', 'failed'].includes(st)) {
      void loadLive()
      // a fixed-length capture the CNC finished: show all of it
      const id = status?.session
      void loadSessions().then(() => {
        if (st === 'completed') viewing = sessions.find((s) => s.id === id) ?? viewing
      })
    }
    lastState = st
  })

  async function start() {
    busy = true
    message = ''
    shotRepeat = false
    try {
      await api.start(machineId, toConfig())
      statuses = await api.status()
      await loadSessions()
    } catch (e) {
      message = String((e as Error).message)
      statuses = await api.status().catch(() => statuses)
    } finally {
      busy = false
    }
  }

  async function stop() {
    busy = true
    message = ''
    try {
      await api.stop(machineId)
      statuses = await api.status()
      await loadSessions()
      await loadLive()
    } catch (e) {
      message = String((e as Error).message)
    } finally {
      busy = false
    }
  }

  // cnc_sdtread1shot: once, or every second while the box is ticked
  let shot = $state<(OneShotResult & { labels: string[]; units: string[] }) | null>(null)
  let shotError = $state('')
  let shotBusy = $state(false)
  let shotRepeat = $state(false)
  async function readOnce() {
    if (shotBusy) return
    shotBusy = true
    try {
      const chs = shotChannels.map((c) => ({ ...c, label: labelOf(c) }))
      const r = await api.oneshot(machineId, form.servoPeriodUs, chs)
      shot = { ...r, labels: chs.map((c) => c.label), units: chs.map(unitOf) }
      shotError = ''
    } catch (e) {
      shotError = String((e as Error).message)
      shotRepeat = false
    } finally {
      shotBusy = false
    }
  }
  $effect(() => {
    if (!shotRepeat || active) return
    let stop = false
    const tick = async () => {
      await readOnce()
      if (!stop && shotRepeat) timer = setTimeout(tick, 1000)
    }
    let timer = setTimeout(tick, 0)
    return () => {
      stop = true
      clearTimeout(timer)
    }
  })

  const STATE: Record<string, string> = {
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

  function duration(from?: string, to?: string | number): string {
    if (!from) return ''
    const end = typeof to === 'number' ? to : to ? Date.parse(to) : Date.now()
    let s = Math.max(0, Math.round((end - Date.parse(from)) / 1000))
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    s %= 60
    return h ? `${h}시간 ${m}분 ${s}초` : m ? `${m}분 ${s}초` : `${s}초`
  }
  const time = (iso?: string) => (iso ? new Date(iso).toLocaleString('ko-KR', { hour12: false }) : '')
  const clock = (iso: string) => new Date(iso).toLocaleTimeString('ko-KR', { hour12: false })
  const fmt = (n: number) => Math.round(n).toLocaleString('ko-KR')
  function bytes(samples: number[] = []): string {
    return size(samples.reduce((a, n) => a + n, 0) * 2)
  }
  // units of 1024, as Windows shows disk sizes
  function size(b: number): string {
    const u = [
      ['TB', 2 ** 40],
      ['GB', 2 ** 30],
      ['MB', 2 ** 20],
      ['KB', 2 ** 10],
    ] as const
    for (const [name, n] of u) if (b >= n) return `${(b / n).toFixed(b / n < 10 ? 1 : 0)} ${name}`
    return `${b} B`
  }
  function seconds(us: number): string {
    const s = us / 1e6
    return s >= 60 ? `${(s / 60).toFixed(1)}분` : `${s.toFixed(s < 10 ? 2 : 1)}초`
  }
  function how(c: Config): string {
    const parts = [c.mode === 'extended' ? '확장' : '기본', `${c.servoPeriodUs / 1000} ms`]
    parts.push(c.points ? `${fmt(c.points)}점` : '종료까지')
    if (c.startSeq) parts.push(`N${c.startSeq}부터`)
    if (c.trigger?.seq) parts.push(`책갈피 N${c.trigger.seq}`)
    if (c.trigger?.signal) parts.push(`책갈피 ${c.trigger.signal}${c.trigger.up ? '↑' : ''}${c.trigger.down ? '↓' : ''}`)
    return parts.join(' · ')
  }
  const progress = $derived(
    status?.config?.points && status.samples?.length ? Math.min(1, status.samples[0] / status.config.points) : null,
  )
  const locked = $derived(active || busy)
</script>

{#snippet err(text: string)}
  <p class="error">{text}{#if explainError(text)}<br /><span class="explain">→ {explainError(text)}</span>{/if}</p>
{/snippet}

<div class="viz-root page">
  <div class="cols">
    <section class="card setup">
      <h2>수집 설정</h2>
      {#if !libOk && library}
        <div class="nolib">
          <p><b>FOCAS2 라이브러리가 없어 시계열 수집을 쓸 수 없습니다.</b> HMI 화면은 deemesh로 그대로 됩니다.</p>
          <p>
            고속 샘플링(<code>cnc_sdt*</code>)은 FANUC의 FOCAS2 라이브러리로 CNC에 직접 연결합니다. FANUC의 파일이라 deelook에 들어 있지
            않으니, 구해서 넣고 deelook을 다시 시작하세요.
          </p>
          <ul>
            {#if library.windows}
              <li><code>Fwlib64.dll</code>과 <code>fwlibe64.dll</code>을 <code>deelook.exe</code> 옆에 둡니다 (이더넷 연결은 이 둘이면 됨).</li>
            {:else}
              <li>32비트 <code>libfwlib32.so</code>를 deelook 실행 파일 옆에 둡니다 (32비트 libc·libstdc++ 필요).</li>
            {/if}
            <li>다른 곳에 있으면 <code>deelook.toml</code>에 <code>fwlib = '…'</code>로 그 파일을 지정합니다.</li>
            <li>
              시험한 판과 해시: <a href="https://github.com/spoonhasi/deelook/blob/main/fwlib/README.md" target="_blank" rel="noreferrer">fwlib/README.md</a>
            </li>
          </ul>
          <p class="hint">찾은 곳: {library.path}<br />{library.error}</p>
        </div>
      {/if}
      {#if !hub.ok}
        <p class="warn">
          ⚠ deemesh-hub에 연결할 수 없습니다. {hub.syncedAt ? `마지막으로 받은 기계 목록(${time(hub.syncedAt)})과` : ''} 여기서 직접 추가한
          기계로 수집합니다. 수집은 deelook이 CNC에 직접 연결하므로 허브 없이도 됩니다.
          <br /><span class="hint">{hub.error}</span>
        </p>
      {/if}
      <div class="row">
        <span>목록</span>
        <div class="seg" role="radiogroup" aria-label="기계 목록">
          <label class:on={source === 'hub'}>
            <input type="radio" name="machine-source" checked={source === 'hub'} onchange={() => selectSource('hub')} disabled={busy} />deemesh-hub
            <small>{hubMachines.length}대</small>
          </label>
          <label class:on={source === 'local'}>
            <input type="radio" name="machine-source" checked={source === 'local'} onchange={() => selectSource('local')} disabled={busy} />직접 추가
            <small>{localMachines.length}대</small>
          </label>
        </div>
        <HelpButton title="기계 목록: deemesh-hub와 직접 추가">
          <ul>
            <li><b>deemesh-hub</b>: 허브에 <code>nc_focas2_fanuc</code>로 등록된 기계 전부입니다(따로 고르지 않음). 허브에서 목록을
              받을 때마다 저장해 두므로, 허브에 연결이 안 돼도 마지막으로 받은 목록으로 수집할 수 있습니다. 허브에서 빠진 기계는 다음에
              목록을 받을 때 같이 빠집니다.</li>
            <li><b>직접 추가</b>: 허브에 없는 FANUC 기계를 기계 옆 "추가"에서 이름·IP·FOCAS2 포트로 넣은 것입니다(번호
              <code>L1</code>, <code>L2</code>…). 추가할 때 연결해서 축 이름을 읽으므로 기계가 켜져 있어야 합니다.</li>
            <li>둘 다 <b>데이터 폴더의 <code>deelook.db</code></b>에 IP·포트와 함께 저장됩니다. 설정 파일 <code>deelook.toml</code>에는
              들어가지 않고, 넘겨주는 세션 파일에도 IP·포트는 없습니다.</li>
            <li>어느 목록이든 수집은 deelook이 CNC에 직접 FOCAS2로 연결해서 합니다. 허브는 기계 목록과 축 이름을 알려 줄 뿐입니다.</li>
            <li>고른 목록과 목록마다 고른 기계는 이 브라우저에 기억됩니다.</li>
          </ul>
        </HelpButton>
      </div>
      <div class="row">
        <span>기계</span>
        <select class="machine" value={machineId} onchange={(e) => selectMachine(e.currentTarget.value)} disabled={busy || !shown.length} aria-label="기계">
          {#if !shown.length}<option value="">{source === 'hub' ? '허브에 FANUC 기계 없음' : '직접 추가한 기계 없음'}</option>{/if}
          {#each shown as m (m.machine)}
            <option value={m.machine}>{m.name || '?'} ({source === 'local' ? m.machine : `machine ${m.machine}`})</option>
          {/each}
        </select>
        <!-- for the machines added here only (the hub's are named and placed on
             the hub); hidden rather than left out for the hub's list, so the row
             keeps its height and the menu its width when the list changes -->
        <button type="button" class:unseen={source !== 'local'} aria-hidden={source !== 'local'} tabindex={source === 'local' ? 0 : -1}
          onclick={openAdd} disabled={busy || source !== 'local'} title="허브에 없는 FANUC 기계를 IP로 추가">추가</button>
        <button type="button" class:unseen={source !== 'local'} aria-hidden={source !== 'local'} tabindex={source === 'local' ? 0 : -1}
          onclick={openEdit} disabled={busy || source !== 'local' || !machine} title="이름·주소 바꾸기, 축 이름 읽기, 지우기">변경</button>
      </div>
      {#if machine?.error}<p class="error">{machine.error}</p>{/if}
      {#if machine?.note}
        <p class="hint">
          {machine.note === 'axis names not known' ? '축 이름을 모릅니다' : machine.note} — 축은 번호로 고르거나
          <button type="button" class="link" onclick={readNames} disabled={manageBusy || active || !libOk}>축 이름을 CNC에서 읽기</button>
          {#if manageMsg && !addOpen && !editOpen}<br /><span class="error">{manageMsg}</span>{/if}
        </p>
      {/if}
      {#if loaded && !shown.length}
        <p class="warn">
          {#if source === 'hub'}
            deemesh-hub에서 받은 FANUC(<code>nc_focas2_fanuc</code>) 기계가 없습니다. 허브에 없는 기계는 기계 옆 "추가"로 IP를 넣어
            추가하세요.
          {:else}
            직접 추가한 기계가 없습니다. 기계 옆 "추가"로 이름·IP·FOCAS2 포트를 넣어 추가하세요.
          {/if}
        </p>
      {/if}
      <Modal title="기계 직접 추가" bind:open={addOpen}>
        <div class="mform">
          <label><span>이름</span><input type="text" bind:value={adding.name} placeholder="비우면 L1 같은 번호" /></label>
          <label><span>IP 주소</span><input type="text" bind:value={adding.host} placeholder="예: 192.0.2.10" /></label>
          <label><span>FOCAS2 포트</span><input type="number" min="1" max="65535" bind:value={adding.port} /></label>
        </div>
        <p class="hint">
          허브에 없는 FANUC 기계를 넣습니다. 추가할 때 그 주소로 연결해서 축 이름을 읽으므로 CNC가 켜져 있어야 합니다. 이름은 화면과
          세션 파일 이름에 쓰이니 IP를 넣지 마세요. IP·포트는 데이터 폴더의 <code>deelook.db</code>에만 저장되고 화면에는 다시 나오지
          않습니다.
        </p>
        {#if manageMsg}{@render err(manageMsg)}{/if}
        <div class="actions">
          <button class="primary" onclick={addMachine} disabled={manageBusy || !adding.host.trim() || !libOk}>{manageBusy ? '연결하는 중…' : '추가'}</button>
          <button onclick={() => (addOpen = false)}>취소</button>
        </div>
      </Modal>

      <Modal title={machine ? `${machine.name} (${machine.source === 'local' ? machine.machine : `machine ${machine.machine}`})` : '기계'}
        bind:open={editOpen}>
        {#if machine?.source === 'local'}
          <div class="mform">
            <label><span>이름</span><input type="text" bind:value={editing.name} /></label>
            <label><span>새 IP 주소</span><input type="text" bind:value={editing.host} placeholder="그대로" /></label>
            <label><span>새 FOCAS2 포트</span><input type="number" min="1" max="65535" bind:value={editing.port} placeholder="그대로" /></label>
          </div>
          <p class="hint">
            지금 IP·포트는 보이지 않습니다(deelook은 기계 주소를 브라우저에 내보내지 않음). 바꿀 때만 새로 넣으세요. 주소를 바꾸면 그
            주소로 연결해서 축 이름을 다시 읽습니다. 이름만 바꾸면 연결하지 않습니다.
          </p>
          <div class="actions">
            <button class="primary" onclick={saveMachine}
              disabled={manageBusy || (!editing.name.trim() && !editing.host.trim() && !editing.port) || (!!editing.host.trim() && !libOk)}>
              {manageBusy ? '저장하는 중…' : '저장'}</button>
            <button onclick={() => (editOpen = false)}>닫기</button>
          </div>
        {:else if machine}
          <p class="hint">
            deemesh-hub의 기계입니다. 이름과 주소는 허브에서 정하니, 바꾸려면 허브의 기계 설정에서 바꾸세요. 허브가 답하면 축 이름도
            허브에서 받습니다.
          </p>
        {/if}
        {#if machine}
          <h4>축 이름</h4>
          <p class="hint">
            {machine.axes.length ? `서보: ${machine.axes.join(' ')}` : '서보: 모름'}
            {machine.spindles.length ? ` · 주축: ${machine.spindles.join(' ')}` : ''}{machine.paths ? ` · 계통 ${machine.paths}개` : ''}
          </p>
          <div class="row">
            <button onclick={readNames} disabled={manageBusy || active || !libOk}>축 이름을 CNC에서 읽기</button>
            <span class="hint">FOCAS2 cnc_rdaxisname·cnc_rdspdlname으로 직접 읽어 저장</span>
          </div>
          {#if machine.source === 'local'}
            <div class="row">
              <button class="danger" onclick={deleteMachine} disabled={manageBusy || active}>이 기계 지우기</button>
              <span class="hint">목록에서만 지웁니다. 세션 파일은 남습니다.</span>
            </div>
          {/if}
          {#if manageMsg && editOpen}{@render err(manageMsg)}{/if}
        {/if}
      </Modal>

      <div class="row">
        <span>방식</span>
        <div class="seg" role="radiogroup" aria-label="수집 방식">
          <label class:on={!ext}><input type="radio" bind:group={form.mode} value="basic" disabled={locked} />기본 <small>cnc_sdt*</small></label>
          <label class:on={ext} title={extOk ? '' : '이 라이브러리에 cnc_sdt*2가 없습니다'}>
            <input type="radio" bind:group={form.mode} value="extended" disabled={locked || !extOk} />확장 <small>cnc_sdt*2</small>
          </label>
        </div>
        <HelpButton title="기본 방식과 확장 방식">
          <table>
            <thead><tr><th></th><th>기본 <code>cnc_sdt*</code></th><th>확장 <code>cnc_sdt*2</code></th></tr></thead>
            <tbody>
              <tr><th>모으는 것</th><td>서보·주축 데이터</td><td>서보·주축 데이터에 더해 <b>PMC 신호</b>(비트·바이트·워드)와 <b>CNC 정보</b>(O·N 번호)를 같은 시간축으로</td></tr>
              <tr><th>시작</th><td>즉시, 또는 지정한 블록(N 번호)이 실행될 때</td><td>늘 즉시</td></tr>
              <tr><th>책갈피</th><td>없음</td><td>블록이 실행되거나 PMC 신호가 바뀐 순간을 CNC가 표시 (수집은 멈추지 않음)</td></tr>
              <tr><th>길이</th><td colspan="2">종료할 때까지, 또는 정한 점 수만큼</td></tr>
              <tr><th>되는 곳</th><td>0i-D, 0i-F, 30i-A, 30i-B, Power Motion i-A · Windows, Linux</td><td>0i-F, 30i-B · Windows만 (Linux 32비트 라이브러리에는 이 함수가 없음)</td></tr>
            </tbody>
          </table>
          <p>
            0i-F·30i-B를 Windows에서 쓰고 블록 N에서 시작할 필요가 없으면, 확장 방식이 서보·주축 데이터를 똑같이 모으면서 PMC 신호,
            O·N 번호, 책갈피까지 줍니다. 기본 방식은 확장 함수가 없는 기종·라이브러리, 블록 N 시작, 그리고 확장 방식의 함정(PMC
            채널 순서, 실패 뒤 정리 등)이 필요 없는 단순한 수집에 씁니다.
          </p>
        </HelpButton>
      </div>
      {#if library?.available && !extOk}
        <p class="hint">확장 방식: 이 라이브러리({library.version})에는 cnc_sdt*2 함수가 없습니다 (리눅스 32비트 J555 라이브러리가 그렇습니다).</p>
      {/if}

      <label class="row">
        <span>서보 주기</span>
        <select bind:value={form.servoPeriodUs} disabled={locked}>
          <option value={1000}>1 ms (1 kHz)</option>
          <option value={250}>0.25 ms (4 kHz)</option>
        </select>
        <span class="hint">주축은 항상 1 ms, CNC 정보는 4 ms마다 갱신</span>
      </label>

      <div class="row">
        <span>길이</span>
        <label class="opt"><input type="radio" bind:group={form.fixed} value={false} disabled={locked} />종료할 때까지</label>
        <label class="opt"><input type="radio" bind:group={form.fixed} value={true} disabled={locked} />점 수</label>
        {#if form.fixed}
          <input type="number" class="wide" min="8" step="1000" bind:value={form.points} disabled={locked} />
          <span class="hint">채널당 = {seconds(form.points * form.servoPeriodUs)}{#if form.servoPeriodUs === 250} (0.25 ms 채널){/if}</span>
        {/if}
      </div>

      <label class="row">
        <span>계통</span>
        <select bind:value={form.path} disabled={locked}>
          {#each Array.from({ length: pathCount }, (_, i) => i + 1) as p (p)}<option value={p}>path {p}</option>{/each}
        </select>
        <span class="hint">{ext ? '책갈피 블록 번호를 볼 계통' : '시작 블록 번호를 볼 계통'}</span>
      </label>

      <div class="row">
        <span>시작</span>
        {#if !ext}
          <label class="opt"><input type="radio" bind:group={form.waitBlock} value={false} disabled={locked} />즉시</label>
          <label class="opt"><input type="radio" bind:group={form.waitBlock} value={true} disabled={locked} />블록 N</label>
          {#if form.waitBlock}
            <input type="number" class="wide" min="1" bind:value={form.startSeq} disabled={locked} />
            <span class="hint">이 시퀀스 번호가 실행되면 CNC가 수집을 시작</span>
          {/if}
        {:else}
          <!-- cnc_sdtstartsmpl2 has no start block: it starts at once; the
               same two choices as in basic, so the row keeps its size -->
          <label class="opt"><input type="radio" name="start-ext" checked disabled />즉시</label>
          <label class="opt" title="블록 N에서 시작은 기본 방식에서만"><input type="radio" name="start-ext" disabled />블록 N</label>
          <span class="hint">기본 방식에서만</span>
        {/if}
      </div>
      <!-- in both modes (off in basic), so switching the mode does not move the page -->
      <div class="row">
        <span>책갈피</span>
        <label class="opt"><input type="checkbox" bind:checked={form.trgSeqOn} disabled={locked || !ext} />블록 N</label>
        {#if form.trgSeqOn}<input type="number" class="wide" min="1" bind:value={form.trgSeq} disabled={locked || !ext} />{/if}
        {#if !ext}<span class="hint">확장 방식에서만</span>{/if}
        <HelpButton title="책갈피 (FANUC 이름: 트리거)">
          <p>
            책갈피는 수집을 <b>시작하거나 멈추지 않습니다.</b> 수집은 "수집 시작"부터 "수집 종료"(또는 정한 점 수)까지 그대로
            이어지고, 조건이 맞는 순간 CNC가 그 자리에 표시를 남깁니다. 차트에는 점선, CSV에는 <code>trigger</code> 열로 나옵니다.
            긴 수집에서 보고 싶은 순간을 바로 찾는 데 씁니다.
          </p>
          <ul>
            <li><b>블록 N</b>: 고른 계통에서 그 시퀀스 번호의 블록이 실행되는 순간.</li>
            <li><b>PMC 신호</b>: 그 비트가 0→1(상승)이나 1→0(하강)으로 바뀌는 순간. 둘 다 고르면 바뀔 때마다. 1로 머물러 있는
              동안에는 남지 않고, 시작할 때 이미 1인 신호에도 남지 않습니다(31i 벤치·NC Guide에서 확인).</li>
            <li>T(타이머)·C(카운터)·Z 주소는 쓸 수 없습니다.</li>
          </ul>
          <p>
            CNC가 찍는 이유: deelook은 네트워크 너머에서 데이터를 묶어서 받아 오므로 신호가 바뀐 정확한 샘플을 알 수 없습니다.
            CNC는 그 순간을 1 ms 단위로 같은 시간축에 찍어 줍니다.
          </p>
          <p>
            시험해 보기: <b>R9091.6</b>은 PMC가 0.5초마다 저절로 0과 1을 바꾸는 시스템 릴레이라, 기계를 건드리지 않아도 책갈피가
            0.5초마다 남습니다. PMC 신호 줄의 "예: R9091.6"을 누르면 채워집니다.
          </p>
          <p>기본 방식에는 책갈피가 없고, 대신 블록 N에서 수집을 시작할 수 있습니다.</p>
        </HelpButton>
      </div>
      <div class="row">
        <span></span>
        <label class="opt"><input type="checkbox" bind:checked={form.trgSigOn} disabled={locked || !ext} />PMC 신호</label>
        {#if form.trgSigOn}
          <input type="text" class="addr" bind:value={form.trgSignal} placeholder="R9091.6" disabled={locked || !ext} aria-label="책갈피 PMC 주소" />
          <select bind:value={form.trgUnit} disabled={locked || !ext} aria-label="책갈피 PMC 경로">
            {#each PMC_UNITS as u (u)}<option value={u}>PMC{u}</option>{/each}
          </select>
          <label class="opt"><input type="checkbox" bind:checked={form.trgUp} disabled={locked || !ext} />상승 (0→1)</label>
          <label class="opt"><input type="checkbox" bind:checked={form.trgDown} disabled={locked || !ext} />하강 (1→0)</label>
        {/if}
        <button type="button" class="link" onclick={bookmarkExample} disabled={locked || !ext}
          title="PMC가 0.5초마다 저절로 바꾸는 신호: 기계를 건드리지 않고 책갈피 시험">예: R9091.6</button>
      </div>

      <ol class="channels">
        {#each form.channels as c, i (i)}
          {@const d = typeOf(c.kind, c.type)}
          <li>
            <div class="ch">
              <span class="no">{i + 1}</span>
              <select value={c.kind} onchange={(e) => setKind(c, e.currentTarget.value as Kind)} disabled={locked} aria-label="채널 {i + 1} 종류">
                {#each EXT_KINDS as k (k)}
                  <option value={k} disabled={kindOff(k) && k !== c.kind}>{KIND[k]}</option>
                {/each}
              </select>
              {#if c.kind === 'servo' || c.kind === 'spindle'}
                <select bind:value={c.axis} disabled={locked} aria-label="채널 {i + 1} {c.kind === 'servo' ? '축' : '주축'}">
                  {#each axisNames(c.kind) as name, a (a)}
                    <option value={a + 1}>{name}</option>
                  {/each}
                  {#if !axisNames(c.kind).length}<option value={1}>{axisName(c.kind, 1)}</option>{/if}
                </select>
              {:else if c.kind === 'cnc'}
                <select bind:value={c.axis} disabled={locked} aria-label="채널 {i + 1} 계통">
                  {#each Array.from({ length: pathCount }, (_, p) => p + 1) as p (p)}<option value={p}>path {p}</option>{/each}
                </select>
              {:else}
                <input type="text" class="addr grow" value={(c.signals ?? []).join(', ')} disabled={locked}
                  placeholder={c.kind === 'pmcbit' ? 'R9091.6, R9091.5' : 'R100'}
                  aria-label="채널 {i + 1} PMC 주소" onchange={(e) => setSignals(c, e.currentTarget.value)} />
                <select bind:value={c.unit} disabled={locked} aria-label="채널 {i + 1} PMC 경로">
                  {#each PMC_UNITS as u (u)}<option value={u}>PMC{u}</option>{/each}
                </select>
              {/if}
              {#if typed(c.kind)}
                <select class="grow type" value={c.type} onchange={(e) => setType(c, Number(e.currentTarget.value))} disabled={locked}
                  aria-label="채널 {i + 1} 데이터" title={typeTip(d, c.shift)}>
                  {#each types[c.kind] as t (t.type)}
                    <option value={t.type}>{t.type}. {t.code} — {t.nameKo}</option>
                  {/each}
                </select>
              {/if}
              {#if c.kind === 'servo' || c.kind === 'spindle'}
                <label class="opt shift" title={SHIFT_TIP}>시프트
                  <input type="number" min="0" max="15" bind:value={c.shift} disabled={locked} aria-label="채널 {i + 1} 시프트" /></label>
              {/if}
              <button class="icon" title="채널 삭제" aria-label="채널 {i + 1} 삭제" disabled={locked || form.channels.length === 1}
                onclick={() => form.channels.splice(i, 1)}>✕</button>
            </div>
          </li>
        {/each}
      </ol>
      <div class="row add">
        <button class="ghost cmd" disabled={locked || form.channels.length >= MAX_CHANNELS} onclick={() => form.channels.push(newChannel())}>+ 채널 추가</button>
        <!-- in both modes (off in basic), like the bookmarks -->
        <button class="ghost cmd" disabled={!ext || locked || form.channels.length >= MAX_CHANNELS || form.channels.some((c) => c.kind === 'pmcbit')}
          title={ext ? '' : '확장 방식에서만'} onclick={() => form.channels.push(newChannel('pmcbit'))}>+ PMC 비트</button>
        <button class="ghost cmd" disabled={!ext || locked || form.channels.length >= MAX_CHANNELS}
          title={ext ? '' : '확장 방식에서만'} onclick={() => form.channels.push(newChannel('cnc'))}>+ N 번호</button>
        <HelpButton title="채널">
          <ul>
            <li>최대 8채널. 서보는 DSP(2축)당 4채널 정도, 주축은 앰프당 2채널까지입니다. 넘으면 CNC가 <code>EW_DATA</code>로
              거부합니다(31i 벤치에서 X1 7채널 + Z1 1채널이 거부됨).</li>
            <li>값은 CNC가 주는 16비트 원시값(-32768~32767)이고, 단위는 데이터마다 다릅니다(데이터 메뉴에 마우스를 올리면 나옴,
              전체 표는 아래 안내).</li>
            <li><b>시프트</b>: CNC 안의 값은 16비트보다 큰 경우가 있어서, CNC가 2<sup>n</sup>으로 나눈 뒤(오른쪽으로 n비트 밀어서)
              16비트에 담아 보냅니다. 예를 들어 속도 지령은 16,777,216이 3750 min⁻¹이라 그대로는 16비트에 안 들어가므로 시프트 9(÷512)로
              받으면 32,768이 3750 min⁻¹입니다. 데이터를 고르면 FANUC 레퍼런스의 권장값으로 바뀌고(대부분 0, 속도 지령 9, 속도 피드백
              3) 보통은 그대로 두면 됩니다. 크게 하면 넘치지 않지만 값이 거칠어지고, 작게 하면 세밀하지만 큰 값이 넘쳐 엉뚱한 값이
              됩니다.</li>
            <li>데이터 종류는 서보 0~46번, 주축 0~21번입니다. 31i 벤치·NC Guide는 모든 종류를 받았고, 그 기계에 없는 장치의
              종류는 거부되지 않고 0으로 옵니다. 되는지는 값이 의미 있게 나오는지로 봅니다.</li>
            <li>확장 방식에서는 PMC 비트(16개까지 한 채널, 하나만), PMC 바이트·워드, CNC 정보(O·N 번호) 채널도 됩니다. PMC 비트
              채널은 deelook이 CNC에 맨 마지막 채널로 보냅니다(앞에 두면 뒤 채널 데이터가 밀려 섞임).</li>
            <li>CNC 정보(O·N 번호)는 4 ms마다 갱신되고, 수집을 시작하면 다음 블록이 시작될 때까지 0입니다.</li>
          </ul>
        </HelpButton>
      </div>

      <div class="actions">
        <button class="primary cmd" onclick={start} disabled={busy || active || !machineId || !!machine?.error || problems.length > 0 || !libOk || lowDisk}>수집 시작</button>
        <button class="danger cmd" onclick={stop} disabled={busy || !active}>수집 종료</button>
        <HelpButton title="이 설정이 부르는 FOCAS2 함수" label="FOCAS2 함수">
          <ol class="calls">
            {#each calls as [fn, what], i (i)}<li><code>{fn}</code> {what}</li>{/each}
          </ol>
          <p class="hint">
            설정 전에 남아 있는 세션을 정리하려고 cnc_sdtcancelsmpl{ext ? ', cnc_sdtendsmpl2' : ''}, cnc_sdtclrchnl도 부릅니다.
            실제로 부른 기록과 결과는 오른쪽 상태 칸의 "FOCAS2 호출 기록"에 있습니다.
          </p>
        </HelpButton>
        <!-- why it cannot start, in this row: one line however many reasons -->
        {#if problems.length}
          <HelpButton title="수집을 시작할 수 없는 이유" warn
            label={'⚠ ' + problems[0] + (problems.length > 1 ? ` (외 ${problems.length - 1}건)` : '')}>
            <ul>{#each problems as p (p)}<li>{p}</li>{/each}</ul>
          </HelpButton>
        {/if}
      </div>
      {#if message}{@render err(message)}{/if}


      <h3>현재값 한 번 읽기 <small>cnc_sdtread1shot</small></h3>
      <p class="hint">수집 없이 위 채널(서보·주축만)의 지금 값을 읽습니다: 연결 → cnc_sdtsetchnl → cnc_sdtread1shot → cnc_sdtclrchnl → 해제.</p>
      <div class="row">
        <button onclick={readOnce} disabled={active || busy || shotBusy || !shotChannels.length || !!machine?.error || !libOk}>읽기</button>
        <label class="opt"><input type="checkbox" bind:checked={shotRepeat} disabled={active || busy || !shotChannels.length} />1초마다</label>
        {#if shot}<span class="hint">{clock(shot.at)}</span>{/if}
      </div>
      {#if shotError}{@render err(shotError)}{/if}
      {#if shot}
        <table class="shot">
          <tbody>
            {#each shot.values as v, i (i)}
              <tr><td>{shot.labels[i]}</td><td class="num">{v}</td><td class="hint">{shot.units[i]}</td></tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </section>

    <section class="card state">
      <h2>상태</h2>
      {#if status}
        <dl>
          <dt>상태</dt>
          <dd>
            <span class="badge {status.state}">{STATE[status.state] ?? status.state}</span>
            {#if status.state === 'waiting'}<span class="hint"> N{status.config?.startSeq} 블록이 실행되면 시작합니다</span>{/if}
          </dd>
          {#if status.session}
            {@const c = status.config}
            <dt>세션</dt>
            <dd>#{status.session}{#if c} · {how(c)}{/if}</dd>
            <dt>시작</dt>
            <dd>{time(status.startedAt)}{#if status.firstDataAt && c?.startSeq} · 첫 데이터 {clock(status.firstDataAt)}{/if}</dd>
            <dt>{active ? '경과' : '길이'}</dt>
            <dd>{duration(status.startedAt, active ? now : status.endedAt)}</dd>
            <dt>샘플</dt>
            <dd>
              {fmt(status.samples?.[0] ?? 0)}{#if c?.points} / {fmt(c.points)}{/if} 점/채널 · {bytes(status.samples)}
              {#if progress !== null}
                <div class="bar" role="progressbar" aria-valuenow={Math.round(progress * 100)} aria-valuemin="0" aria-valuemax="100">
                  <div style="width: {progress * 100}%"></div>
                </div>
              {/if}
            </dd>
            {#if status.reads?.calls}
              {@const r = status.reads}
              <dt>읽기</dt>
              <dd>
                {fmt(r.calls)}회 · 1회에 {fmt(r.lastPoints[0] ?? 0)}점 (최대 {fmt(r.maxPoints)}) · 평균 {r.avgMs.toFixed(0)} ms (최대 {r.maxMs.toFixed(0)})
                {#if active}<br />읽는 속도 {fmt(r.rate)}점/초 · CNC {fmt(r.expected)}점/초{/if}
              </dd>
            {/if}
          {/if}
        </dl>
        {#if status.reads?.behind && active}
          <p class="warn">
            ⚠ 읽기가 CNC 샘플링보다 느립니다. 한 번에 읽히는 양(약 1.3 KB)이 정해져 있어 채널이 많거나 0.25 ms이거나 네트워크가 느리면
            밀립니다. {status.config?.points
              ? 'CNC가 점 수를 다 모으면 끝나므로, 그때 못 읽은 점은 사라집니다.'
              : '계속되면 CNC 버퍼가 넘쳐(EW_BUFFER) 수집이 멈춥니다.'}
          </p>
        {/if}
        {#if status.error}{@render err(status.error)}{/if}
      {/if}
      {#if library?.available}<p class="hint">FOCAS2 라이브러리 {library.version}{library.extended ? ' · cnc_sdt*2 있음' : ' · cnc_sdt*2 없음'}</p>
      {:else if library}<p class="hint">FOCAS2 라이브러리 없음 — 수집할 수 없음</p>{/if}

      {#if live.channels.length}
        <div class="charts">
          {#each live.channels as ch, i (i)}
            {@const c = status?.config?.channels[i]}
            {#if ch.kind === 'pmcbit'}
              <BitChart {ch} events={live.events} />
            {:else}
              <LiveChart {ch} unit={c ? unitOf(c) : ''} events={live.events} />
            {/if}
          {/each}
        </div>
      {:else if active}
        <p class="hint">데이터를 기다리는 중…</p>
      {/if}

      {#if status?.events?.length}
        <h3>책갈피 <small>cnc_sdtreadsmpl2 trg</small></h3>
        <div class="scroll short">
          <table>
            <thead><tr><th>시점 (첫 샘플부터)</th><th>받은 시각</th><th>비트</th></tr></thead>
            <tbody>
              {#each [...status.events].reverse() as e (e.timeUs + ':' + e.bits)}
                <tr><td class="num">{(e.timeUs / 1000).toFixed(0)} ms</td><td>{clock(e.at)}</td><td>{trgText(e.bits)}</td></tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}

      {#if status?.calls?.length}
        <details class="calls">
          <summary>FOCAS2 호출 기록 (읽기 반복 제외)</summary>
          <div class="scroll short">
            <table>
              <tbody>
                {#each status.calls as k, i (i)}
                  <tr class:bad={k.result !== 'EW_OK' && !k.cleanup}>
                    <td>{clock(k.at)}</td><td><code>{k.fn}</code></td><td class="num">{k.ms.toFixed(1)} ms</td>
                    <td>
                      {k.result}{#if k.cleanup}<span class="hint"> · 설정 전 정리</span>{/if}
                      {#if k.result !== 'EW_OK'}<br /><span class="hint">{explainError(k.result)}</span>{/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </details>
      {/if}
    </section>
  </div>

  <Reference {types} {canAdd} onadd={addFromReference} />

  {#if viewing}
    <SessionViewer session={viewing} {unitOf} onclose={() => (viewing = null)} />
  {/if}

  <section class="card">
    <h2>기록된 세션</h2>
    {#if storage}
      <p class="hint storage" class:warn={lowDisk}>
        수집 한 건 = 파일 하나 (넘겨줄 때는 "DB"로 받은 그 파일만) · 세션 파일 {storage.sessions}개 {size(storage.used)}
        {#if storage.total}· 남은 공간 {size(storage.free)} / {size(storage.total)}{/if}
        {#if storage.minFree}· {size(storage.minFree)}보다 적으면 수집을 멈춥니다{/if}
        {#if lowDisk}<br />⚠ 남은 공간이 기준보다 적어 새 수집을 시작할 수 없습니다. 지난 세션을 지우거나 deelook.toml의 minfree를 바꾸세요.{/if}
      </p>
    {/if}
    {#if sessions.length}
      <div class="scroll">
        <table class="sessions">
          <thead>
            <tr><th>#</th><th>시작</th><th>길이</th><th>방식</th><th>채널</th><th>샘플/채널</th><th>책갈피</th><th>크기</th><th>상태</th><th></th></tr>
          </thead>
          <tbody>
            {#each sessions as s (s.id)}
              <tr>
                <td>{s.id}</td>
                <td>{time(s.startedAt)}</td>
                <td>{s.status === 'running' ? '수집 중' : duration(s.startedAt, s.endedAt)}</td>
                <td class="labels">{how(s.config)}</td>
                <td class="labels">{s.config.channels.map((c) => (c.kind === 'pmcbit' ? (c.signals ?? []).join(' ') : c.label)).join(', ')}</td>
                <td>{s.samples?.length ? fmt(s.samples[0]) : ''}</td>
                <td>{s.events || ''}</td>
                <td class="num">{size(s.size)}</td>
                <td><span class="badge {s.status}" title={s.error}>{STATE[s.status] ?? s.status}</span></td>
                <td class="links">
                  <button class="link" onclick={() => (viewing = s)}>보기</button>
                  {#if s.status !== 'running'}
                    <a href={api.csvUrl(s.id)} download title="표로 (엑셀은 104만 행까지)">CSV</a>
                    <a href={api.dbUrl(s.id)} download title="이 세션의 SQLite 파일 그대로: {s.file}">DB</a>
                    <button class="link danger" onclick={() => deleteSession(s)}>삭제</button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="hint">아직 기록이 없습니다.</p>
    {/if}
  </section>
</div>

<style>
  .viz-root {
    color-scheme: dark;
    --surface-1: #1a1a19;
    --surface-2: #262625;
    --text-primary: #ffffff;
    --text-secondary: #c3c2b7;
    --grid: #3a3a38;
    --series-1: #3987e5;
    --mark: #e3a008;
  }
  .page {
    display: flex;
    flex-direction: column;
    gap: 12px;
    width: min(100%, 1400px);
    margin: 0 auto;
  }
  .cols {
    display: grid;
    grid-template-columns: minmax(0, 5fr) minmax(0, 6fr);
    gap: 12px;
  }
  @media (max-width: 900px) {
    .cols {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  .card {
    background: #252525;
    border: 1px solid #333;
    border-radius: 6px;
    padding: 12px 14px;
    min-width: 0;
  }
  h2 {
    margin: 0 0 10px;
    font-size: 15px;
    color: var(--text-primary);
  }
  h3 {
    margin: 16px 0 4px;
    font-size: 14px;
    color: var(--text-primary);
  }
  h3 small,
  .seg small {
    font-weight: 400;
    color: var(--text-secondary);
    font-family: ui-monospace, monospace;
    font-size: 11px;
  }
  p {
    margin: 8px 0;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
    flex-wrap: wrap;
  }
  .row > span:first-child {
    width: 72px;
    color: var(--text-secondary);
  }
  .row.add {
    margin-top: 6px;
  }
  .opt {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 13px;
  }
  .seg {
    display: inline-flex;
    border: 1px solid #444;
    border-radius: 4px;
    overflow: hidden;
  }
  .seg label {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 12px;
    cursor: pointer;
    font-size: 13px;
  }
  .seg label.on {
    background: #1f3b63;
    color: var(--text-primary);
  }
  .seg input {
    margin: 0;
  }
  select,
  input {
    font: inherit;
    color: var(--text-primary);
    background: #1a1a19;
    border: 1px solid #444;
    border-radius: 4px;
    padding: 3px 6px;
    max-width: 100%;
  }
  input[type='radio'],
  input[type='checkbox'] {
    accent-color: #3987e5;
  }
  /* menus, fields and buttons share one height, so the controls of a row line up */
  select,
  input:not([type='radio']):not([type='checkbox']),
  button:not(.link) {
    box-sizing: border-box;
    height: 32px;
  }
  .shift {
    font-size: 12px;
    color: var(--text-secondary);
  }
  input[type='number'] {
    width: 52px;
  }
  input[type='number'].wide {
    width: 90px;
  }
  input.addr {
    width: 120px;
    text-transform: uppercase;
  }
  .channels {
    list-style: none;
    margin: 10px 0 0;
    padding: 0;
  }
  .channels li {
    border-bottom: 1px solid #2f2f2e;
    padding: 6px 0;
  }
  .ch {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }
  .ch .no {
    width: 14px;
    color: var(--text-secondary);
    font-size: 12px;
  }
  .ch .grow {
    flex: 1 1 140px;
    min-width: 0;
  }
  .ch .type {
    flex-basis: 220px;
  }
  /* one width for the channel and start/stop buttons, so they line up */
  button.cmd {
    width: 7.5em;
    padding-left: 0;
    padding-right: 0;
    text-align: center;
  }
  .num {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .shot {
    max-width: 420px;
  }
  .shot td:first-child {
    color: var(--text-primary);
  }
  .scroll {
    overflow-x: auto;
  }
  .scroll.short {
    max-height: 220px;
    overflow-y: auto;
  }
  .labels {
    color: var(--text-secondary);
  }
  button {
    font: inherit;
    border-radius: 4px;
    padding: 5px 14px;
    cursor: pointer;
    border: 1px solid #555;
    background: #2d2d2d;
    color: var(--text-primary);
  }
  button:disabled {
    opacity: 0.45;
    cursor: default;
  }
  button.primary {
    background: #1f6feb;
    border-color: #1f6feb;
  }
  button.danger {
    background: #b42318;
    border-color: #b42318;
  }
  button.icon {
    padding: 2px 8px;
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 10px;
    min-width: 0; /* the warning shortens with "…" instead of wrapping */
  }
  .actions > button {
    flex: none;
  }
  ol.calls {
    margin: 6px 0;
    padding-left: 22px;
  }
  ol.calls li {
    margin: 4px 0;
  }
  .hint {
    color: var(--text-secondary);
    font-size: 12px;
  }
  .error {
    color: #ff8a80;
    font-size: 13px;
    white-space: pre-wrap;
  }
  .unseen {
    visibility: hidden;
  }
  /* the machine menu gives way, so add and change stay on its right */
  select.machine {
    flex: 1 1 0;
    min-width: 5em;
    max-width: 360px;
  }
  .mform {
    display: grid;
    gap: 8px;
  }
  .mform label {
    display: grid;
    grid-template-columns: 110px minmax(0, 1fr);
    align-items: center;
    gap: 8px;
  }
  .mform input {
    width: 100%;
    box-sizing: border-box;
  }
  h4 {
    margin: 14px 0 4px;
    font-size: 14px;
    color: var(--text-primary);
  }
  .nolib {
    border: 1px solid #9a6700;
    background: #2b2416;
    border-radius: 6px;
    padding: 8px 12px;
    margin: 0 0 12px;
    font-size: 13px;
  }
  .nolib p {
    margin: 4px 0;
  }
  .nolib ul {
    margin: 4px 0;
    padding-left: 18px;
  }
  .explain {
    color: var(--text-secondary);
  }
  .warn {
    color: #ffb77c;
    font-size: 13px;
  }
  dl {
    display: grid;
    grid-template-columns: 64px 1fr;
    gap: 4px 10px;
    margin: 0 0 8px;
    font-size: 13px;
  }
  dt {
    color: var(--text-secondary);
  }
  dd {
    margin: 0;
  }
  .bar {
    height: 6px;
    margin-top: 4px;
    background: #1a1a19;
    border-radius: 3px;
    overflow: hidden;
    max-width: 320px;
  }
  .bar > div {
    height: 100%;
    background: var(--series-1);
  }
  .badge {
    display: inline-block;
    padding: 1px 8px;
    border-radius: 10px;
    font-size: 12px;
    border: 1px solid #555;
  }
  /* state badges carry their word, never colour alone */
  .badge.running,
  .badge.starting,
  .badge.waiting,
  .badge.completed {
    border-color: #3fb950;
    color: #7ee787;
  }
  .badge.failed,
  .badge.interrupted {
    border-color: #f0883e;
    color: #ffb77c;
  }
  .charts {
    display: grid;
    gap: 10px;
    margin-top: 10px;
  }
  details.calls {
    margin-top: 12px;
    font-size: 13px;
  }
  details.calls summary {
    cursor: pointer;
    color: var(--text-primary);
  }
  td.links {
    white-space: nowrap;
  }
  td.links a {
    margin-right: 8px;
  }
  button.link.danger {
    color: #ff8a80;
  }
  .storage.warn {
    color: #ffb77c;
  }
  button.link {
    border: none;
    background: none;
    color: #79b8ff;
    padding: 0 8px 0 0;
    font-size: 13px;
  }
  tr.bad td {
    color: #ffb77c;
  }
  a {
    color: #79b8ff;
  }
  code {
    background: #1a1a19;
    padding: 0 4px;
    border-radius: 3px;
  }
</style>

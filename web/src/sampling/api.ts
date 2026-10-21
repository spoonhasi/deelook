// The sampling API of the deelook server (internal/sampling/http.go).

export type Mode = 'basic' | 'extended'
export type Kind = 'servo' | 'spindle' | 'pmcbit' | 'pmcbyte' | 'pmcword' | 'cnc'
export type TypedKind = 'servo' | 'spindle' | 'cnc'

export interface Unit {
  raw: string
  means: string
  when?: string
}

export interface DataType {
  type: number
  code: string
  name: string // the reference's English name
  nameKo: string
  units: Unit[] // what the raw value means with shift 0
  unit: string // units in one line
  shift: number // right shift the reference suggests
  note?: string
}

export interface Types {
  servo: DataType[]
  spindle: DataType[]
  cnc: DataType[]
  pmcKinds: string
}

export interface ChannelConfig {
  kind: Kind
  axis: number // servo axis 1..32 / spindle 1..8 (logical across paths); cnc: path
  type: number
  shift: number
  signals?: string[] // pmc*: addresses like Y0.4 (bit) or R100 (byte/word)
  unit?: number // pmc*: PMC path
  label: string
}

export interface Trigger {
  seq: number // block number, 0 = none
  signal: string // PMC bit like Y0.4, '' = none
  unit: number
  up: boolean
  down: boolean
}

export interface Config {
  mode: Mode
  servoPeriodUs: number // 1000 or 250
  points: number // 0 = until stopped (basic only)
  path: number
  startSeq: number // basic: start when this block runs
  trigger: Trigger // extended
  channels: ChannelConfig[]
}

export interface Library {
  available: boolean // false: no FOCAS2 library, no sampling (error says why, path is where it looked)
  path: string
  version?: string
  extended?: boolean // has cnc_sdt*2
  error?: string
  windows: boolean // the server runs on Windows
}

export interface MachineInfo {
  machine: string // hub machine_id, or L1, L2... for machines added in deelook
  name: string
  source: 'hub' | 'local'
  axes: string[] // empty when not known
  spindles: string[]
  paths: number // 0 when not known
  error?: string // cannot sample
  note?: string // can sample, something is missing
}

export interface HubState {
  ok: boolean
  error?: string
  syncedAt?: string // when the hub's machine list was last received
}

export type State = 'idle' | 'starting' | 'waiting' | 'running' | 'stopping' | 'stopped' | 'completed' | 'failed'

export interface Event {
  timeUs: number
  bits: number
  at: string
}

export interface Call {
  at: string
  fn: string
  ms: number
  result: string
  cleanup?: boolean // tidying before setup: a refusal is expected
}

export interface ReadStats {
  calls: number
  lastPoints: number[]
  maxPoints: number
  avgMs: number
  maxMs: number
  rate: number
  expected: number
  behind: boolean
}

export interface Status {
  machine: string
  name?: string
  state: State
  error?: string
  session?: number
  startedAt?: string
  firstDataAt?: string
  endedAt?: string
  config?: Config
  samples?: number[]
  reads?: ReadStats
  events?: Event[]
  calls?: Call[]
}

export interface SessionInfo {
  id: number
  machine: string
  machineName: string
  startedAt: string
  endedAt?: string
  status: 'running' | 'stopped' | 'completed' | 'failed' | 'interrupted'
  error?: string
  config: Config
  samples: number[]
  events: number
  file: string // the session's SQLite file name
  size: number // its size in bytes
}

/** The data folder's disk: free space, the session files, the minimum before sampling stops. */
export interface Storage {
  dir: string
  free: number
  total: number
  used: number
  sessions: number
  minFree: number
}

export interface LiveChannel {
  label: string
  kind: Kind
  signals?: string[]
  periodUs: number
  bucketUs: number
  endUs: number
  min: number[] // pmcbit: AND of the words in the bucket
  max: number[] // pmcbit: OR
}

export interface Live {
  channels: LiveChannel[]
  events: Event[]
}

export interface View {
  fromUs: number
  toUs: number
  totalUs: number
  channels: LiveChannel[]
  events: Event[]
}

export interface OneShotResult {
  at: string
  values: number[]
  calls: Call[]
}

// cnc_sdtstartsmpl2 / cnc_sdtreadsmpl2 trigger bits
export const TRG = { seq: 1, up: 2, down: 4, ended: 0x8000 }

async function get<T>(url: string): Promise<T> {
  const r = await fetch(url)
  if (!r.ok) throw new Error((await r.text()).trim() || `HTTP ${r.status}`)
  return (await r.json()) as T
}

/** POST with a JSON body: the server refuses anything else (no cross-site form posts). */
async function post<T>(url: string, body: unknown = {}, method = 'POST'): Promise<T> {
  const r = await fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
  if (!r.ok) throw new Error((await r.text()).trim() || `HTTP ${r.status}`)
  return (await r.json()) as T
}

const m = (id: string) => `/api/sampling/${encodeURIComponent(id)}`

export const api = {
  types: () => get<Types>('/api/sampling/types'),
  machines: () => get<{ library?: Library; hub: HubState; machines: MachineInfo[] }>('/api/sampling/machines'),
  addMachine: (name: string, host: string, port: number) =>
    post<{ machine: string }>('/api/sampling/machines', { name, host, port }),
  /** What the page stored under key in deelook.db, null when nothing yet. */
  ui: <T>(key: string) => get<T | null>(`/api/ui/${encodeURIComponent(key)}`),
  /** Stores value under key; keepalive lets it finish while the page is being left. */
  setUI: async (key: string, value: unknown) => {
    await fetch(`/api/ui/${encodeURIComponent(key)}`, {
      method: 'PUT',
      keepalive: true,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(value),
    })
  },
  /** Renames a machine added here; a host or port (empty, 0: as it is) moves it there. */
  updateMachine: (id: string, name: string, host: string, port: number) =>
    post<{ ok: boolean }>(`/api/sampling/machines/${encodeURIComponent(id)}`, { name, host, port }, 'PATCH'),
  deleteMachine: (id: string) => post<{ ok: boolean }>(`/api/sampling/machines/${encodeURIComponent(id)}`, {}, 'DELETE'),
  readNames: (id: string) => post<{ ok: boolean }>(`/api/sampling/machines/${encodeURIComponent(id)}/names`),
  status: () => get<Status[]>('/api/sampling/status'),
  start: (id: string, cfg: Config) => post<Status>(`${m(id)}/start`, cfg),
  stop: (id: string) => post<Status>(`${m(id)}/stop`),
  oneshot: (id: string, servoPeriodUs: number, channels: ChannelConfig[]) =>
    post<OneShotResult>(`${m(id)}/oneshot`, { servoPeriodUs, channels }),
  live: (id: string) => get<Live>(`${m(id)}/live?window=3s&buckets=600`),
  sessions: (id: string) => get<SessionInfo[]>(`/api/sampling/sessions?machine=${encodeURIComponent(id)}&limit=50`),
  view: (id: number, fromUs = 0, toUs = 0) =>
    get<View>(`/api/sampling/sessions/${id}/view?from=${Math.round(fromUs)}&to=${Math.round(toUs)}&buckets=600`),
  csvUrl: (id: number) => `/api/sampling/sessions/${id}/csv`,
  dbUrl: (id: number) => `/api/sampling/sessions/${id}/db`,
  deleteSession: (id: number) => post<{ ok: boolean }>(`/api/sampling/sessions/${id}`, {}, 'DELETE'),
  storage: () => get<Storage>('/api/sampling/storage'),
}

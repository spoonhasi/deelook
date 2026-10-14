// What the deelook server sends on every poll (internal/live.Snapshot).

/** One deemesh answer: status 0 with a value, or a negative status with an error. */
export interface Envelope {
  status: number
  value?: unknown
  error?: string
  desc?: unknown
  unit?: unknown
}

export interface Snapshot {
  screen: string
  machine: string
  at: string
  /** false: the server could not reach the hub; values are the last ones it had */
  ok: boolean
  error?: string
  values?: Record<string, Envelope>
}

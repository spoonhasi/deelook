import type { Snapshot } from './types'

/**
 * Follows one view's snapshots from the deelook server (Server-Sent Events).
 * The browser reconnects by itself after a network drop; when the server
 * refuses the stream (unknown machine, hub down) the reason is fetched for
 * display and the stream is retried a few seconds later.
 */
export class ScreenStream {
  snapshot = $state<Snapshot | null>(null)
  /** Why no stream is open right now; empty while snapshots arrive. */
  problem = $state('')
  /** Date.now() of the last snapshot. */
  receivedAt = $state(0)

  #url: string
  #es: EventSource | null = null
  #retry: ReturnType<typeof setTimeout> | undefined
  #closed = false

  /**
   * `previous` is shown until this view's first snapshot arrives, so a view
   * switch keeps the header and status line on screen.
   */
  constructor(screen: string, params: Record<string, string>, previous: Snapshot | null = null) {
    const q = new URLSearchParams(Object.entries(params).filter(([, v]) => v !== ''))
    this.#url = `/api/screens/${screen}/stream?${q}`
    this.snapshot = previous
    this.receivedAt = Date.now()
    this.#open()
  }

  #open() {
    const es = new EventSource(this.#url)
    this.#es = es
    es.onmessage = (e) => {
      this.snapshot = JSON.parse(e.data) as Snapshot
      this.receivedAt = Date.now()
      this.problem = ''
    }
    es.onerror = () => {
      if (es.readyState !== EventSource.CLOSED) {
        this.problem = 'deelook 서버에 다시 연결하는 중'
        return
      }
      void this.#explain()
      if (!this.#closed) this.#retry = setTimeout(() => this.#open(), 5000)
    }
  }

  // EventSource hides the HTTP error body, so ask once more to show it.
  async #explain() {
    try {
      const r = await fetch(this.#url, { signal: AbortSignal.timeout(3000) })
      this.problem = r.ok ? 'deelook 서버에 다시 연결하는 중' : (await r.text()).trim() || `HTTP ${r.status}`
    } catch {
      this.problem = 'deelook 서버에 연결할 수 없습니다'
    }
  }

  close() {
    this.#closed = true
    this.#es?.close()
    clearTimeout(this.#retry)
  }
}

<script lang="ts">
  // Recorded sessions, which keep piling up: a fixed-height box with a line
  // per day (how many, how large), opened to that day's sessions a part at
  // a time; filters by period, state and machine; many at once deleted or
  // downloaded as one zip; and old ones cleared by age.
  import { untrack } from 'svelte'
  import Modal from '../lib/Modal.svelte'
  import { api, type DaySummary, type SessionInfo, type SessionQuery } from './api'
  import { duration, fmt, how, size, STATE } from './format'

  let {
    machineId,
    version,
    viewingId = null,
    onview,
    onchange,
  }: {
    machineId: string
    version: number // bumped when sessions start or end
    viewingId?: number | null
    onview: (s: SessionInfo) => void
    onchange: (deleted: number[] | null) => void // null: some, not known which
  } = $props()

  const PART = 100 // sessions of a day per request
  const MAX_ZIP = 2000

  type Period = 'today' | '7d' | '30d' | 'all'
  type Kind = 'all' | 'ok' | 'bad'
  let period = $state<Period>('all')
  let kind = $state<Kind>('all')
  let allMachines = $state(false)
  let days = $state<DaySummary[]>([])
  let total = $state({ count: 0, size: 0 })
  let open = $state<Record<string, boolean>>({})
  let loaded = $state<Record<string, { list: SessionInfo[]; more: boolean }>>({})
  let selected = $state<Record<number, { size: number; day: string }>>({})
  let msg = $state('')
  let busy = $state(false)

  const STATUSES: Record<Kind, string[]> = { all: [], ok: ['stopped', 'completed'], bad: ['failed', 'interrupted'] }

  function baseQuery(): SessionQuery {
    const q: SessionQuery = { machine: allMachines ? '' : machineId, status: STATUSES[kind] }
    if (period !== 'all') {
      const d = new Date()
      d.setHours(0, 0, 0, 0)
      d.setDate(d.getDate() - ({ today: 0, '7d': 6, '30d': 29 } as const)[period])
      q.since = d.toISOString()
    }
    return q
  }
  /** A day's bounds where the page is. */
  function dayRange(day: string) {
    const [y, m, d] = day.split('-').map(Number)
    return { since: new Date(y, m - 1, d).toISOString(), until: new Date(y, m - 1, d + 1).toISOString() }
  }
  function dayLabel(day: string): string {
    const [y, m, d] = day.split('-').map(Number)
    const opt: Intl.DateTimeFormatOptions = { month: 'long', day: 'numeric', weekday: 'short' }
    if (y !== new Date().getFullYear()) opt.year = 'numeric'
    return new Date(y, m - 1, d).toLocaleDateString('ko-KR', opt)
  }
  /** HH:MM:SS where the page is: one short line (the day is in the day row). */
  function clockOf(iso: string): string {
    const d = new Date(iso)
    return [d.getHours(), d.getMinutes(), d.getSeconds()].map((n) => String(n).padStart(2, '0')).join(':')
  }

  async function loadDay(day: string, more = false) {
    const cur = loaded[day]
    const before = more && cur?.list.length ? cur.list[cur.list.length - 1].id : undefined
    const list = await api.sessionList({ ...baseQuery(), ...dayRange(day), before, limit: PART + 1 })
    loaded[day] = { list: [...(more && cur ? cur.list : []), ...list.slice(0, PART)], more: list.length > PART }
  }

  async function reload() {
    msg = ''
    if (!machineId && !allMachines) {
      days = []
      total = { count: 0, size: 0 }
      return
    }
    try {
      const r = await api.sessionDays(baseQuery())
      days = r.days
      total = { count: r.count, size: r.size }
      if (!days.some((d) => open[d.day]) && days.length) open[days[0].day] = true // the newest day, at first
      loaded = {}
      await Promise.all(days.filter((d) => open[d.day]).map((d) => loadDay(d.day)))
      // what was selected and is gone (deleted elsewhere) stays out
      const present = new Set(Object.values(loaded).flatMap((x) => x.list.map((s) => s.id)))
      for (const id of Object.keys(selected).map(Number)) {
        if (!present.has(id) && open[selected[id].day]) delete selected[id]
      }
    } catch (e) {
      msg = String((e as Error).message)
    }
  }
  $effect(() => {
    void [machineId, version, period, kind, allMachines]
    untrack(() => void reload())
  })

  async function toggle(day: string) {
    open[day] = !open[day]
    if (open[day] && !loaded[day]) await loadDay(day).catch((e) => (msg = String(e.message)))
  }

  // selection: by session, or all of a day
  const chosen = $derived(Object.keys(selected).map(Number))
  const chosenSize = $derived(Object.values(selected).reduce((a, x) => a + x.size, 0))
  function pick(s: SessionInfo, day: string, on: boolean) {
    if (on) selected[s.id] = { size: s.size, day }
    else delete selected[s.id]
  }
  function inDay(day: string) {
    return Object.values(selected).filter((x) => x.day === day).length
  }
  function selectable(day: DaySummary) {
    const l = loaded[day.day]
    return l && !l.more ? l.list.filter((s) => s.status !== 'running').length : day.count
  }
  async function pickDay(day: DaySummary, on: boolean) {
    if (!on) {
      for (const [id, x] of Object.entries(selected)) if (x.day === day.day) delete selected[Number(id)]
      return
    }
    try {
      let list = loaded[day.day]?.list ?? []
      if (!loaded[day.day] || loaded[day.day].more) {
        list = await api.sessionList({ ...baseQuery(), ...dayRange(day.day), limit: 5000 })
      }
      for (const s of list) if (s.status !== 'running') selected[s.id] = { size: s.size, day: day.day }
    } catch (e) {
      msg = String((e as Error).message)
    }
  }

  async function remove(ids: number[], what: string) {
    if (!ids.length || !confirm(`${what}을(를) 지울까요? 파일이 지워지고 되돌릴 수 없습니다.`)) return
    busy = true
    try {
      const r = await api.deleteSessions({ ids })
      for (const id of ids) delete selected[id]
      const failed = Object.entries(r.failed ?? {})
      msg = failed.length ? `${r.count}건 지움 · 못 지운 ${failed.length}건: ${failed.map(([id, e]) => `#${id} ${e}`).join(', ')}` : ''
      onchange(ids)
      await reload()
    } catch (e) {
      msg = String((e as Error).message)
    } finally {
      busy = false
    }
  }
  function download(ids: number[]) {
    if (ids.length > MAX_ZIP) {
      msg = `한 번에 ${fmt(MAX_ZIP)}건까지 받을 수 있습니다.`
      return
    }
    const a = document.createElement('a')
    a.href = api.zipUrl([...ids].sort((x, y) => x - y))
    a.download = ''
    a.click()
  }

  // clearing by age: what it would take first, then the delete
  let cleanOpen = $state(false)
  let cleanDays = $state(30)
  let cleanAll = $state(false)
  let cleanPreview = $state<{ count: number; size: number } | null>(null)
  let cleanMsg = $state('')
  function cutoff(daysOld: number): string {
    const d = new Date()
    d.setHours(0, 0, 0, 0)
    d.setDate(d.getDate() - daysOld)
    return d.toISOString()
  }
  function cleanBody(dryRun: boolean) {
    return { machine: cleanAll ? '' : machineId, until: cutoff(Math.max(0, Math.floor(cleanDays))), dryRun }
  }
  $effect(() => {
    if (!cleanOpen) return
    void [cleanDays, cleanAll]
    cleanPreview = null
    cleanMsg = ''
    const t = setTimeout(async () => {
      try {
        cleanPreview = await api.deleteSessions(cleanBody(true))
      } catch (e) {
        cleanMsg = String((e as Error).message)
      }
    }, 250)
    return () => clearTimeout(t)
  })
  async function clean() {
    if (!cleanPreview?.count) return
    if (!confirm(`${fmt(cleanPreview.count)}건 (${size(cleanPreview.size)})을 지울까요? 되돌릴 수 없습니다.`)) return
    busy = true
    try {
      const r = await api.deleteSessions(cleanBody(false))
      cleanMsg = `${fmt(r.count)}건 지움 (${size(r.size)})`
      selected = {}
      onchange(null)
      await reload()
      cleanPreview = await api.deleteSessions(cleanBody(true))
    } catch (e) {
      cleanMsg = String((e as Error).message)
    } finally {
      busy = false
    }
  }

  const cols = $derived(allMachines ? 12 : 11)
</script>

<div class="bar">
  <label>기간
    <select bind:value={period}>
      <option value="today">오늘</option>
      <option value="7d">최근 7일</option>
      <option value="30d">최근 30일</option>
      <option value="all">전체</option>
    </select>
  </label>
  <label>상태
    <select bind:value={kind}>
      <option value="all">전체</option>
      <option value="ok">정상 끝남</option>
      <option value="bad">실패·중단</option>
    </select>
  </label>
  <label class="check"><input type="checkbox" bind:checked={allMachines} />모든 기계</label>
  <span class="hint">{fmt(total.count)}건 · {size(total.size)}</span>
  <button type="button" class="right" onclick={() => (cleanOpen = true)} disabled={busy}>오래된 세션 정리</button>
</div>
<div class="bar">
  <span class="hint">선택 {fmt(chosen.length)}건{chosen.length ? ` · ${size(chosenSize)}` : ''}</span>
  <button type="button" onclick={() => download(chosen)} disabled={!chosen.length || busy}>DB 받기 (zip)</button>
  <button type="button" class="danger" onclick={() => remove(chosen, `선택한 ${fmt(chosen.length)}건 (${size(chosenSize)})`)}
    disabled={!chosen.length || busy}>지우기</button>
  <button type="button" onclick={() => (selected = {})} disabled={!chosen.length}>선택 해제</button>
</div>
{#if msg}<p class="msg">{msg}</p>{/if}

<div class="list">
  <table>
    <thead>
      <tr>
        <th></th><th>#</th><th>시작</th>{#if allMachines}<th>기계</th>{/if}<th>길이</th><th>방식</th><th>채널</th>
        <th class="num">샘플/채널</th><th class="num">책갈피</th><th class="num">크기</th><th>상태</th><th></th>
      </tr>
    </thead>
    <tbody>
      {#each days as d (d.day)}
        {@const n = inDay(d.day)}
        <tr class="day">
          <td>
            <input type="checkbox" aria-label="{dayLabel(d.day)} 모두 선택" checked={n > 0 && n >= selectable(d)}
              indeterminate={n > 0 && n < selectable(d)} onchange={(e) => pickDay(d, e.currentTarget.checked)} />
          </td>
          <td colspan={cols - 1}>
            <button type="button" class="toggle" onclick={() => toggle(d.day)} aria-expanded={!!open[d.day]}>
              <span class="arrow">{open[d.day] ? '▼' : '▶'}</span> {dayLabel(d.day)}
            </button>
            <span class="hint">{fmt(d.count)}건 · {size(d.size)}</span>
          </td>
        </tr>
        {#if open[d.day]}
          {#each loaded[d.day]?.list ?? [] as s (s.id)}
            <tr class:viewing={s.id === viewingId}>
              <td>
                <input type="checkbox" aria-label="세션 {s.id} 선택" disabled={s.status === 'running'} checked={!!selected[s.id]}
                  onchange={(e) => pick(s, d.day, e.currentTarget.checked)} />
              </td>
              <td class="num">{s.id}</td>
              <td class="num">{clockOf(s.startedAt)}</td>
              {#if allMachines}<td>{s.machineName}</td>{/if}
              <td>{s.status === 'running' ? '수집 중' : duration(s.startedAt, s.endedAt)}</td>
              <td class="labels">{how(s.config)}</td>
              <td class="labels chans">{s.config.channels.map((c) => (c.kind === 'pmcbit' ? (c.signals ?? []).join(' ') : c.label)).join(', ')}</td>
              <td class="num">{s.samples?.length ? fmt(s.samples[0]) : ''}</td>
              <td class="num">{s.events || ''}</td>
              <td class="num">{size(s.size)}</td>
              <td><span class="badge {s.status}" title={s.error}>{STATE[s.status] ?? s.status}</span></td>
              <td class="links">
                <button type="button" class="link" onclick={() => onview(s)}>보기</button>
                {#if s.status !== 'running'}
                  <a href={api.csvUrl(s.id)} download title="표로 (엑셀은 104만 행까지)">CSV</a>
                  <a href={api.dbUrl(s.id)} download title="이 세션의 SQLite 파일 그대로: {s.file}">DB</a>
                  <button type="button" class="link danger" onclick={() => remove([s.id], `세션 #${s.id} (${s.file}, ${size(s.size)})`)}>삭제</button>
                {/if}
              </td>
            </tr>
          {/each}
          {#if !loaded[d.day]}
            <tr><td></td><td colspan={cols - 1} class="hint">불러오는 중…</td></tr>
          {:else if loaded[d.day].more}
            <tr>
              <td></td>
              <td colspan={cols - 1}>
                <button type="button" class="link" onclick={() => loadDay(d.day, true)}>
                  더 보기 (남은 {fmt(d.count - loaded[d.day].list.length)}건)
                </button>
              </td>
            </tr>
          {/if}
        {/if}
      {:else}
        <tr><td></td><td colspan={cols - 1} class="hint">{total.count ? '' : '기록된 세션이 없습니다.'}</td></tr>
      {/each}
    </tbody>
  </table>
</div>

<Modal title="오래된 세션 정리" bind:open={cleanOpen}>
  <div class="clean">
    <label><input type="number" min="0" max="36500" bind:value={cleanDays} /> 일보다 오래된 세션을</label>
    <div class="seg">
      <label><input type="radio" name="clean-scope" checked={!cleanAll} onchange={() => (cleanAll = false)} /> 이 기계만</label>
      <label><input type="radio" name="clean-scope" checked={cleanAll} onchange={() => (cleanAll = true)} /> 모든 기계</label>
    </div>
  </div>
  <p>
    {#if cleanPreview}
      지울 세션: <b>{fmt(cleanPreview.count)}건 · {size(cleanPreview.size)}</b>
      <span class="hint">(시작한 날이 {Math.max(0, Math.floor(cleanDays))}일 전보다 앞선 것, 수집 중인 것은 빼고)</span>
    {:else}
      <span class="hint">세는 중…</span>
    {/if}
  </p>
  {#if cleanMsg}<p class="msg">{cleanMsg}</p>{/if}
  <div class="actions">
    <button type="button" class="danger" onclick={clean} disabled={busy || !cleanPreview?.count}>지우기</button>
    <button type="button" onclick={() => (cleanOpen = false)}>닫기</button>
  </div>
  <p class="hint">세션 파일이 지워지고 되돌릴 수 없습니다. 넘겨줄 것은 먼저 "DB 받기"로 받아 두세요.</p>
</Modal>

<style>
  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 12px;
    margin: 6px 0;
    font-size: 13px;
  }
  .bar label {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .bar .right {
    margin-left: auto;
  }
  .hint {
    color: var(--text-secondary);
    font-size: 12px;
  }
  .msg {
    color: #ffb77c;
    font-size: 12px;
    margin: 4px 0;
  }
  select,
  input[type='number'] {
    box-sizing: border-box;
    height: 30px;
    font: inherit;
    color: var(--text-primary);
    background: #1a1a19;
    border: 1px solid #444;
    border-radius: 4px;
    padding: 2px 6px;
  }
  input[type='number'] {
    width: 70px;
  }
  input[type='checkbox'],
  input[type='radio'] {
    accent-color: #3987e5;
  }
  button {
    box-sizing: border-box;
    height: 30px;
    padding: 0 12px;
    font: inherit;
    font-size: 13px;
    color: var(--text-primary);
    background: #2d2d2d;
    border: 1px solid #555;
    border-radius: 4px;
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.45;
    cursor: default;
  }
  button.danger {
    border-color: #8b3a3a;
    color: #ff8a80;
  }
  button.link,
  button.toggle {
    height: auto;
    border: none;
    background: none;
    padding: 0 8px 0 0;
  }
  button.link {
    color: #79b8ff;
  }
  button.link.danger {
    color: #ff8a80;
  }
  button.toggle {
    color: var(--text-primary);
    font-weight: 600;
  }
  .arrow {
    display: inline-block;
    width: 1em;
    color: var(--text-secondary);
    font-size: 11px;
  }
  /* a fixed height: the page does not grow with the sessions */
  .list {
    max-height: 460px;
    overflow: auto;
    border: 1px solid #333;
    border-radius: 4px;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  th {
    position: sticky;
    top: 0;
    z-index: 1;
    background: #2a2a29;
    color: var(--text-primary);
    font-weight: 600;
    text-align: left;
    white-space: nowrap;
  }
  th,
  td {
    padding: 4px 8px;
    border-bottom: 1px solid #2c2c2b;
    vertical-align: middle;
  }
  tr.day td {
    background: #232322;
  }
  tr.viewing td {
    background: #1d2a3a;
  }
  .num {
    text-align: right;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  .labels {
    color: var(--text-secondary);
  }
  .chans {
    max-width: 260px;
  }
  td.links {
    white-space: nowrap;
  }
  td.links a {
    color: #79b8ff;
    margin-right: 8px;
  }
  .badge {
    display: inline-block;
    padding: 1px 8px;
    border-radius: 10px;
    font-size: 12px;
    border: 1px solid #555;
    white-space: nowrap;
  }
  /* state badges carry their word, never colour alone */
  .badge.running,
  .badge.completed {
    border-color: #3fb950;
    color: #7ee787;
  }
  .badge.failed,
  .badge.interrupted {
    border-color: #f0883e;
    color: #ffb77c;
  }
  .clean {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px 16px;
  }
  .clean .seg {
    display: inline-flex;
    gap: 12px;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
</style>

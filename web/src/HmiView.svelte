<script lang="ts">
  import { untrack } from 'svelte'
  import { address } from './lib/address.svelte'
  import { ScreenStream } from './lib/stream.svelte'
  import * as f from './screens/fanuc/format'
  import { FanucNav, OFFSET_ROWS } from './screens/fanuc/nav.svelte'
  import MainPanel from './screens/fanuc/panels/MainPanel.svelte'
  import MdiPanel from './screens/fanuc/panels/MdiPanel.svelte'
  import SubPanel from './screens/fanuc/panels/SubPanel.svelte'
  import Screen from './screens/fanuc/Screen.svelte'

  // ?machine=<machine_id or machine_name>, chosen in the footer too; the
  // server picks its default (setting machine) otherwise
  const machine = $derived(address.machine)
  const nav = new FanucNav()

  // one stream per view and machine: switching screens re-opens it with
  // what the new screen needs, keeping the last values on screen meanwhile;
  // another machine starts from nothing
  let stream = $state<ScreenStream | null>(null)
  let streamFor: string | null = null // the machine the stream is for
  $effect(() => {
    const { view, page } = nav.view
    const m = machine
    const same = m === streamFor
    const previous = same ? untrack(() => stream?.snapshot ?? null) : null
    if (!same) lastAlarm = null
    streamFor = m
    const s = new ScreenStream('fanuc', { machine: m, view, page: page ? String(page) : '' }, previous)
    stream = s
    return () => s.close()
  })

  const snap = $derived(stream?.snapshot ?? null)
  const values = $derived(snap?.values ?? {})

  $effect(() => {
    const n = f.num(values.offsetCount)
    if (n) nav.offsetPages = Math.max(1, Math.ceil(n / OFFSET_ROWS))
  })

  // like the control (NC Guide did), jump to the alarm screen when an alarm
  // comes up; an alarm already up when the page opens does not jump
  let lastAlarm: number | null = null
  $effect(() => {
    const a = f.num(values.alarm)
    if (a === null) return
    if (a === 2 && lastAlarm !== null && lastAlarm !== 2) untrack(() => nav.alarmRaised())
    lastAlarm = a
  })

  // the hub's machines: names for the footer, and those this screen is
  // drawn for to choose from
  type HubMachine = { machine_id: number; machine_name: string; screen?: string }
  let names = $state<Record<string, string>>({})
  let choices = $state<{ id: string; name: string }[]>([])
  fetch('/api/machines')
    .then((r) => (r.ok ? r.json() : null))
    .then((d: { machines: HubMachine[] } | null) => {
      for (const m of d?.machines ?? []) names[String(m.machine_id)] = m.machine_name
      choices = (d?.machines ?? [])
        .filter((m) => m.screen === 'fanuc')
        .map((m) => ({ id: String(m.machine_id), name: m.machine_name }))
    })
    .catch(() => {})

  // the machine on screen: the one the values are from, else the one asked for
  const shown = $derived(snap?.machine ?? choices.find((c) => c.id === machine || c.name === machine)?.id ?? '')
  function choose(id: string) {
    if (id) address.go('/hmi', address.withMachine(id))
  }

  let now = $state(Date.now())
  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 1000)
    return () => clearInterval(t)
  })

  // What is wrong with the values on screen, if anything. The screen keeps
  // showing the last values; this line says they are not current.
  const notice = $derived.by(() => {
    if (stream?.problem) return stream.problem
    if (!snap) return '연결 중…'
    if (!snap.ok) return `deemesh 허브 응답 없음 — 마지막 값 표시 중 (${snap.error})`
    const link = values.clock
    if (link && link.status !== 0) return `CNC 통신 오류 ${link.status}: ${link.error ?? ''}`
    if (now - (stream?.receivedAt ?? 0) > 5000) return '업데이트 지연 — 마지막 값 표시 중'
    return ''
  })

  const title = $derived(snap ? `${names[snap.machine] ?? ''} (machine ${snap.machine})` : '')

  // the machine panels can be hidden (small screens); the MDI keys stay,
  // since they switch screens
  let panels = $state(loadPanels())
  function loadPanels(): boolean {
    try {
      return localStorage.getItem('deelook.panels') !== 'off'
    } catch {
      return true
    }
  }
  function togglePanels() {
    panels = !panels
    try {
      localStorage.setItem('deelook.panels', panels ? 'on' : 'off')
    } catch {
      // private window: keep the choice for this page only
    }
  }
</script>

<div class="view">
  <div class="hmi" class:panels>
    <div class="col left">
      <div class="cnc" class:stale={!!notice}>
        <Screen {values} {nav} />
      </div>
      {#if panels}<div class="main-panel"><MainPanel {values} /></div>{/if}
    </div>
    <div class="col right">
      {#if panels}<div class="sub-panel"><SubPanel {values} /></div>{/if}
      <div class="mdi-panel">
        <MdiPanel onfunction={(fn) => nav.functionKey(fn)} onpage={(d) => nav.pageKey(d)} />
      </div>
    </div>
  </div>
  <footer>
    <span class="dot" class:bad={!!notice}></span>
    {#if choices.length}
      <select aria-label="기계" value={shown} onchange={(e) => choose(e.currentTarget.value)}>
        {#if !shown}<option value="" disabled>기계 고르기</option>{/if}
        {#if shown && !choices.some((c) => c.id === shown)}<option value={shown}>{title}</option>{/if}
        {#each choices as c (c.id)}<option value={c.id}>{c.name} (machine {c.id})</option>{/each}
      </select>
    {:else}
      <span>{title}</span>
    {/if}
    {#if notice}<span class="notice">{notice}</span>{/if}
    <button type="button" onclick={togglePanels}>{panels ? '조작반 숨기기' : '조작반 보이기'}</button>
  </footer>
</div>

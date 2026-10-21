<script lang="ts">
  // The deemesh-hub address at the top right of the app's pages: where the
  // HMI screens read their values (sampling works without it). Changing it
  // applies at once and is written into deelook.toml; no address runs
  // deelook without deemesh.
  import { onMount } from 'svelte'
  import Modal from './Modal.svelte'

  type HubInfo = { url: string; ok: boolean; machines: number; error?: string; warning?: string }
  let info = $state<HubInfo | null>(null)
  let open = $state(false)
  let draft = $state('')
  let busy = $state(false)
  let msg = $state('')
  let changed = false // the pages read the machines again when the dialog closes

  async function load() {
    try {
      const r = await fetch('/api/hub')
      if (r.ok) info = await r.json()
    } catch {
      // deelook restarting: keep what was shown
    }
  }
  onMount(() => {
    void load()
    const t = setInterval(load, 30000)
    return () => clearInterval(t)
  })
  $effect(() => {
    if (!open && changed) location.reload()
  })

  function edit() {
    draft = info?.url ?? ''
    msg = ''
    open = true
    void load()
  }
  async function save(url: string) {
    busy = true
    msg = ''
    try {
      const r = await fetch('/api/hub', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url }),
      })
      if (!r.ok) {
        msg = (await r.text()).trim()
        return
      }
      info = (await r.json()) as HubInfo
      changed = true
      draft = info.url
      // stay open when there is something to read: it did not answer, or it is not saved
      if (info.warning) msg = info.warning
      else if (info.url && !info.ok) msg = `저장했습니다. 그런데 지금은 연결되지 않습니다: ${info.error ?? ''}`
      else open = false
    } catch (e) {
      msg = String((e as Error).message)
    } finally {
      busy = false
    }
  }
  const statusText = $derived(
    !info
      ? '확인하는 중…'
      : !info.url
        ? '쓰지 않음 (주소 없음)'
        : info.ok
          ? `연결됨 · 기계 ${info.machines}대`
          : `연결 안 됨: ${info.error ?? ''}`,
  )
</script>

<button type="button" class="hub" onclick={edit} title="deemesh-hub 주소: {statusText}">
  <span class="dot" class:ok={info?.ok} class:off={!!info && !info.url}></span>
  deemesh-hub <span class="addr">{info ? (info.url ? info.url.replace(/^https?:\/\//, '') : '없음') : '…'}</span>
</button>
<Modal title="deemesh-hub 주소" bind:open>
  <p class="state">
    <span class="dot" class:ok={info?.ok} class:off={!!info && !info.url}></span>
    {statusText}
  </p>
  <label class="field">
    <span>주소</span>
    <input type="text" bind:value={draft} placeholder="http://127.0.0.1:8080" spellcheck="false" />
  </label>
  <p class="hint">
    HMI 화면은 값을 전부 deemesh-hub에서 받으므로 허브가 있어야 합니다. 시계열 수집은 허브 없이도 됩니다(기계를 IP로 직접
    추가). 바꾸면 바로 적용되고 <code>deelook.toml</code>의 <code>hub</code>도 바뀝니다. 주소를 지우면 deemesh 없이 씁니다.
  </p>
  {#if msg}<p class="msg">{msg}</p>{/if}
  <div class="actions">
    <button class="primary" onclick={() => save(draft)} disabled={busy || !draft.trim()}>{busy ? '확인하는 중…' : '저장'}</button>
    <button onclick={() => save('')} disabled={busy || !info?.url}>주소 지우기 (deemesh 안 씀)</button>
    <button onclick={() => (open = false)}>닫기</button>
  </div>
</Modal>

<style>
  .hub {
    margin-left: auto;
    align-self: center;
    margin-bottom: 4px;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    max-width: 50vw;
    padding: 3px 10px;
    border: 1px solid #444;
    border-radius: 12px;
    background: #252525;
    color: #c8c8c8;
    font: inherit;
    font-size: 12px;
    white-space: nowrap;
    cursor: pointer;
  }
  .hub:hover {
    border-color: #777;
    color: #fff;
  }
  .addr {
    overflow: hidden;
    text-overflow: ellipsis;
    color: #9a9a9a;
  }
  @media (max-width: 560px) {
    .addr {
      display: none;
    }
  }
  .dot {
    flex: none;
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #f0883e; /* not answering */
  }
  .dot.ok {
    background: #3fb950;
  }
  .dot.off {
    background: #777;
  }
  .state {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0 0 12px;
    overflow-wrap: anywhere;
  }
  .field {
    display: grid;
    grid-template-columns: 50px minmax(0, 1fr);
    align-items: center;
    gap: 8px;
  }
  input {
    box-sizing: border-box;
    width: 100%;
    height: 32px;
    padding: 3px 8px;
    border: 1px solid #444;
    border-radius: 4px;
    background: #1a1a19;
    color: #fff;
    font: inherit;
  }
  .hint {
    color: #b0b0b0;
    font-size: 12px;
    line-height: 1.6;
  }
  .hint code {
    font-family: ui-monospace, Consolas, monospace;
    background: #2f2f2f;
    padding: 0 4px;
    border-radius: 3px;
  }
  .msg {
    color: #ffb77c;
    font-size: 13px;
    overflow-wrap: anywhere;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .actions button {
    box-sizing: border-box;
    height: 32px;
    padding: 0 14px;
    border: 1px solid #555;
    border-radius: 4px;
    background: #2d2d2d;
    color: #fff;
    font: inherit;
    cursor: pointer;
  }
  .actions button.primary {
    background: #1f6feb;
    border-color: #1f6feb;
  }
  .actions button:disabled {
    opacity: 0.45;
    cursor: default;
  }
</style>

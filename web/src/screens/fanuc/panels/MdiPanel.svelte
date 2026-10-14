<script lang="ts">
  // MDI keyboard of NC Guide. The function keys and PAGE keys change what
  // this browser shows; the rest are drawn but do nothing (deelook is read
  // only and never sends a key to the machine).
  import type { Fn } from '../nav.svelte'

  interface Props {
    onfunction: (fn: Fn) => void
    onpage: (dir: 1 | -1) => void
  }
  let { onfunction, onpage }: Props = $props()

  interface Key {
    x: number
    y: number
    w?: number
    label: string // main legend ("/" splits lines)
    alt?: string // small legend top-left
    peach?: boolean
    big?: boolean
    action?: () => void
  }
  const fnKey = (x: number, label: string, fn: Fn): Key => ({ x, y: 41, label, peach: true, action: () => onfunction(fn) })
  const row = (y: number, x0: number, chars: string, alts: string[]): Key[] =>
    [...chars].map((c, i) => ({ x: x0 + 57 * i, y, label: c, alt: alts[i], big: true }))

  const keys: Key[] = [
    { x: 18, y: 41, label: 'RESET' },
    { x: 106, y: 41, label: 'HELP', peach: true },
    fnKey(189, 'POS', 'pos'),
    fnKey(246, 'PROG', 'prog'),
    fnKey(302, 'OFS/SET', 'ofs'),
    fnKey(359, 'MESSAGE', 'message'),
    fnKey(416, 'GRAPH', 'graph'),
    { x: 473, y: 41, label: 'CUSTOM1', peach: true },
    { x: 530, y: 41, label: 'CUSTOM2', peach: true },
    fnKey(721, 'SYSTEM', 'system'),
    ...row(109, 18, 'QWERTYUIOP', ['!', '@', '#', '$', '%', '&', '*', '(', ')', '\\¥']),
    ...row(171, 49, 'ASDFGHJKL', ['', '~', '-', ';', ':', '"', "'", '[', ']']),
    { x: 18, y: 233, label: 'ABC/abc', peach: true },
    ...row(233, 75, 'ZXCVBNM', ['', '', '<', '>', '/', '?', 'SP']),
    { x: 473, y: 233, label: 'EOB' },
    { x: 530, y: 233, label: '⇧/SHIFT' },
    ...row(109, 607, '789', []),
    ...row(171, 607, '456', []),
    ...row(233, 607, '123', []),
    ...row(295, 607, '−0.', ['+', '−', ',']),
    { x: 473, y: 295, w: 101, label: 'SPECIAL' },
    { x: 18, y: 311, label: 'CTRL', peach: true },
    { x: 75, y: 311, label: 'AUX', peach: true },
    { x: 163, y: 311, label: '↑/PAGE', peach: true, action: () => onpage(-1) },
    { x: 163, y: 368, label: 'PAGE/↓', peach: true, action: () => onpage(1) },
    { x: 277, y: 311, label: '↑', big: true },
    { x: 220, y: 342, label: '←', big: true },
    { x: 334, y: 342, label: '→', big: true },
    { x: 277, y: 368, label: '↓', big: true },
    { x: 18, y: 368, label: 'ALT', peach: true },
    { x: 75, y: 368, label: 'TAB', peach: true },
    { x: 416, y: 368, label: 'ALTER', peach: true },
    { x: 473, y: 368, label: 'INSERT', peach: true },
    { x: 530, y: 368, label: 'DELETE', peach: true },
    { x: 607, y: 368, label: 'CAN', peach: true },
    { x: 664, y: 368, w: 106, label: 'INPUT', peach: true },
  ]
</script>

{#snippet face(key: Key)}
  {@const w = key.w ?? 50}
  {@const lines = key.label.split('/')}
  <rect x={key.x} y={key.y} width={w} height="51" rx="3" fill={key.peach ? '#fde1bf' : '#fffffd'} stroke="#2e2f33" />
  {#if key.alt}<text x={key.x + 5} y={key.y + 14} class="alt">{key.alt}</text>{/if}
  {#if key.big}
    <text x={key.x + w / 2 + (key.alt ? 4 : 0)} y={key.y + 38} class="big">{key.label}</text>
  {:else}
    {#each lines as line, n (n)}
      <text x={key.x + w / 2} y={key.y + 30 + (n - (lines.length - 1) / 2) * 13} class="small">{line}</text>
    {/each}
  {/if}
{/snippet}

<svg viewBox="8 32 772 394" xmlns="http://www.w3.org/2000/svg" aria-label="MDI">
  <rect x="8" y="32" width="772" height="394" rx="4" fill="#5c5d63" />
  {#each keys as key, i (i)}
    {#if key.action}
      <g class="key active" role="button" tabindex="0" aria-label={key.label.replace('/', ' ')} onclick={key.action}
        onkeydown={(e) => e.key === 'Enter' && key.action?.()}>
        {@render face(key)}
      </g>
    {:else}
      <g class="key">{@render face(key)}</g>
    {/if}
  {/each}
</svg>

<style>
  svg {
    display: block;
    width: 100%;
    height: auto;
    user-select: none;
  }
  text {
    font-family: Arial, Helvetica, sans-serif;
    fill: #111;
    text-anchor: middle;
  }
  .big {
    font-size: 26px;
  }
  .small {
    font-size: 10px;
    font-weight: 700;
  }
  .alt {
    font-size: 10px;
    text-anchor: start;
  }
  .active {
    cursor: pointer;
  }
  .active:hover rect {
    stroke: #ffd36b;
    stroke-width: 2;
  }
  .active:active {
    transform: translate(1px, 1px);
  }
  .active:focus {
    outline: none;
  }
</style>

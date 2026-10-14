<script lang="ts">
  // Machine operator panel ("Main Panel" of NC Guide): 5 x 11 keys, each
  // with its lamp above. The keys only show state; deelook never presses them.
  import type { Envelope } from '../../../lib/types'
  import { lampOn, type Lamp } from './lamps'

  let { values }: { values: Record<string, Envelope> } = $props()

  interface Key {
    label: string[]
    big?: boolean
    lamp?: Lamp
  }
  const cols = [22, 76, 130, 184, 253, 307, 361, 415, 485, 538, 592]
  const rows = [60, 137, 214, 291, 368]
  const k = (label: string, lamp?: Lamp, big = false): Key => ({ label: label ? label.split('/') : [], lamp, big })
  const keys: Key[][] = [
    [k('AUTO', 'auto'), k('EDIT', 'edit'), k('MDI', 'mdi'), k('REMOTE', 'remote'), k('REF', 'ref'), k('JOG', 'jog'), k('INC', 'inc'), k('HANDLE', 'handle'), k(''), k(''), k('')],
    [k('SINGLE/BLOCK', 'sbk'), k('BLOCK/SKIP', 'bdt'), k('OPT/STOP', 'opt'), k('TEACH', 'teach'), k('X  1', 'x1'), k('X10', 'x10'), k('X100', 'x100'), k('X1000', 'x1000'), k('X', 'axX', true), k('Y', 'axY', true), k('Z', 'axZ', true)],
    [k('RESTART', 'restart'), k('MC/LOCK', 'mlk'), k('DRY/RUN', 'dry'), k(''), k(''), k(''), k(''), k(''), k('4', 'ax4', true), k('5', 'ax5', true), k('6', 'ax6', true)],
    [k(''), k(''), k(''), k(''), k(''), k(''), k(''), k(''), k('+', 'plus', true), k('RAPID', 'rapid'), k('−', 'minus', true)],
    [k('CYCLE/STOP', 'cycleStop'), k('CYCLE/START', 'cycleStart'), k('PRG/STOP', 'prgStop'), k(''), k(''), k(''), k(''), k(''), k('SPDL/CW', 'spCw'), k('SPDL/STOP', 'spStop'), k('SPDL/CCW', 'spCcw')],
  ]
</script>

<svg viewBox="10 36 652 396" xmlns="http://www.w3.org/2000/svg" aria-label="Main Panel (read only)">
  <title>Main Panel — 상태 표시 전용, 누를 수 없음</title>
  <rect x="10" y="36" width="652" height="396" rx="4" fill="#63676a" />
  {#each keys as row, r (r)}
    {#each row as key, c (c)}
      {@const x = cols[c]}
      {@const y = rows[r]}
      {@const on = key.lamp ? lampOn(key.lamp, values) : false}
      <circle cx={x + 24} cy={y - 10} r="3.2" fill={on ? '#dcff00' : '#272b2e'} class:lit={on} />
      <rect {x} {y} width="49" height="52" rx="3" fill="#fbfcf9" stroke="#3d4043" />
      {#if key.big}
        <text x={x + 24.5} y={y + 35} class="big">{key.label[0]}</text>
      {:else}
        {#each key.label as line, i (i)}
          <text x={x + 24.5} y={y + 29 + (i - (key.label.length - 1) / 2) * 11}>{line}</text>
        {/each}
      {/if}
    {/each}
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
    font-size: 9.5px;
    font-weight: 700;
    text-anchor: middle;
    fill: #111;
  }
  text.big {
    font-size: 26px;
    font-weight: 400;
  }
  .lit {
    filter: drop-shadow(0 0 2px #dcff00);
  }
</style>

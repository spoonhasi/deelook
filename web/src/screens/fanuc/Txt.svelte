<script lang="ts">
  // Text on the HMI's character grid. The browser font is squeezed or
  // stretched so each character takes exactly one cell, as on the CNC:
  // small cells are 8 px wide, large ones 24 px.
  interface Props {
    x: number
    y: number // baseline
    text?: string
    /** Coloured pieces instead of text, e.g. [['1', black], ['H', blue]]. */
    segs?: [string, string][]
    fill?: string
    big?: boolean
    pitch?: number
    anchor?: 'start' | 'middle' | 'end'
    blink?: boolean
  }
  let { x, y, text = '', segs, fill = '#000000', big = false, pitch, anchor = 'start', blink = false }: Props = $props()

  const content = $derived(segs ? segs.map((s) => s[0]).join('') : text)
  const cell = $derived(pitch ?? (big ? 24 : 8))
</script>

{#if content}
  <text {x} {y} {fill} class:big class:blink text-anchor={anchor} textLength={content.length * cell} lengthAdjust="spacingAndGlyphs"
    >{#if segs}{#each segs as [t, f], i (i)}<tspan fill={f}>{t}</tspan>{/each}{:else}{text}{/if}</text
  >
{/if}

<style>
  text {
    font-family: 'JetBrains Mono', monospace;
    font-size: 12.5px;
    font-weight: 700;
    font-variant-numeric: slashed-zero;
    white-space: pre;
  }
  text.big {
    font-size: 31.5px;
    font-weight: 600;
  }
  .blink {
    animation: blink 1s steps(1) infinite;
  }
  @keyframes blink {
    50% {
      opacity: 0;
    }
  }
</style>

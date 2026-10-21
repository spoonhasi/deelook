<script lang="ts">
  // A modal dialog the page opens and closes through `open`: forms such as
  // adding or changing a machine. Esc, the close button or a click beside
  // the box closes it (and sets open back to false).
  import type { Snippet } from 'svelte'

  let { title, open = $bindable(false), children }: { title: string; open?: boolean; children: Snippet } = $props()
  let dialog: HTMLDialogElement

  $effect(() => {
    if (open && !dialog.open) dialog.showModal()
    else if (!open && dialog.open) dialog.close()
  })

  function beside(e: MouseEvent) {
    if (e.target === dialog) open = false // the backdrop: the box fills the dialog
  }
</script>

<dialog bind:this={dialog} onclick={beside} onclose={() => (open = false)} aria-label={title}>
  <div class="box">
    <header>
      <h3>{title}</h3>
      <button type="button" class="close" aria-label="닫기" onclick={() => (open = false)}>✕</button>
    </header>
    {#if open}{@render children()}{/if}
  </div>
</dialog>

<style>
  dialog {
    width: min(560px, calc(100vw - 32px));
    max-height: 85vh;
    padding: 0;
    border: 1px solid #444;
    border-radius: 8px;
    background: #252525;
    color: #d8d8d8;
    overflow: auto;
  }
  dialog::backdrop {
    background: rgba(0, 0, 0, 0.6);
  }
  .box {
    padding: 14px 18px 18px;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 10px;
  }
  h3 {
    margin: 0;
    font-size: 16px;
    color: #fff;
  }
  .close {
    border: none;
    background: none;
    color: #aaa;
    font-size: 16px;
    padding: 4px 8px;
    cursor: pointer;
  }
  .close:hover {
    color: #fff;
  }
</style>

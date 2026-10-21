<script lang="ts">
  // A "?" button that opens an explanation in a modal dialog: long
  // explanations stay out of the form, so switching an option does not
  // move the page. Esc, the close button or a click beside the box closes it.
  import type { Snippet } from 'svelte'

  let { title, children }: { title: string; children: Snippet } = $props()
  let dialog: HTMLDialogElement

  function beside(e: MouseEvent) {
    if (e.target === dialog) dialog.close() // the backdrop: the box fills the dialog
  }
</script>

<button type="button" class="help" aria-label="{title} 설명" title="설명" onclick={() => dialog.showModal()}>?</button>
<dialog bind:this={dialog} onclick={beside} aria-label={title}>
  <div class="box">
    <header>
      <h3>{title}</h3>
      <button type="button" class="close" aria-label="닫기" onclick={() => dialog.close()}>✕</button>
    </header>
    <div class="body">{@render children()}</div>
  </div>
</dialog>

<style>
  .help {
    flex: none;
    width: 20px;
    height: 20px;
    padding: 0;
    margin-left: 4px;
    border: 1px solid #666;
    border-radius: 50%;
    background: #2d2d2d;
    color: #c8c8c8;
    font: 600 12px/1 system-ui, sans-serif;
    cursor: pointer;
    vertical-align: middle;
  }
  .help:hover,
  .help:focus-visible {
    color: #fff;
    border-color: #aaa;
  }
  dialog {
    width: min(680px, calc(100vw - 32px));
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
    margin-bottom: 6px;
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
  .body {
    font-size: 14px;
    line-height: 1.6;
  }
  .body :global(p) {
    margin: 8px 0;
  }
  .body :global(ul) {
    margin: 8px 0;
    padding-left: 20px;
  }
  .body :global(table) {
    border-collapse: collapse;
    margin: 8px 0;
    font-size: 13px;
  }
  .body :global(th),
  .body :global(td) {
    border: 1px solid #3a3a3a;
    padding: 4px 8px;
    text-align: left;
    vertical-align: top;
  }
  .body :global(th) {
    background: #2d2d2d;
    color: #eee;
  }
  .body :global(code) {
    font-family: ui-monospace, Consolas, monospace;
    font-size: 0.92em;
    background: #2f2f2f;
    padding: 0 4px;
    border-radius: 3px;
  }
</style>

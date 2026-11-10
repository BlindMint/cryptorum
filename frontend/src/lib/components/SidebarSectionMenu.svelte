<script lang="ts">
	let { label, createLabel, oncreate, onadd }: { label: string; createLabel: string; oncreate: () => void; onadd: () => void } = $props();
	let menu = $state<HTMLDetailsElement>();
</script>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape' && menu?.open) { menu.open = false; menu.querySelector('summary')?.focus(); } }} onpointerdown={(e) => { if (menu && !e.composedPath().includes(menu)) menu.open = false; }} />
<details bind:this={menu} class="relative">
	<summary class="flex h-7 w-6 cursor-pointer list-none items-center justify-center rounded text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-base)] focus-visible:outline-2 focus-visible:outline-[var(--color-primary-500)]" aria-label="Add to {label}" title="Add to {label}">
		<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v12m6-6H6"/></svg>
	</summary>
	<div class="floating-surface absolute right-0 top-full z-[85] mt-1 w-44 rounded-md border border-[var(--color-surface-border)] p-1">
		<button type="button" class="w-full rounded px-2 py-1.5 text-left text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]" onclick={() => { if (menu) menu.open = false; oncreate(); }}>{createLabel}</button>
		<button type="button" class="w-full rounded px-2 py-1.5 text-left text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]" onclick={() => { if (menu) menu.open = false; onadd(); }}>Add divider</button>
	</div>
</details>

<style>summary::-webkit-details-marker { display: none; }</style>

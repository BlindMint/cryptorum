<script lang="ts">
	import { onMount, tick } from 'svelte';
	import type { SidebarDivider } from '$lib/utils/sidebar-dividers';
	let { divider, first, last, startEditing = false, onrename, onmove, onremove, dragging = false }: {
		divider: SidebarDivider; first: boolean; last: boolean; startEditing?: boolean;
		dragging?: boolean;
		onrename: (label: string) => void; onmove: (direction: -1 | 1) => void; onremove: () => void;
	} = $props();
	let editing = $state(false);
	let label = $state('');
	let input = $state<HTMLInputElement>();
	let menu = $state<HTMLDetailsElement>();
	async function rename() {
		if (menu) menu.open = false;
		label = divider.label;
		editing = true;
		await tick();
		input?.focus();
		input?.select();
	}
	async function save(e: SubmitEvent) {
		e.preventDefault();
		onrename(label.trim());
		editing = false;
		await tick();
		menu?.querySelector('summary')?.focus();
	}
	onMount(() => { if (startEditing) void rename(); });
</script>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape' && menu?.open) { menu.open = false; menu.querySelector('summary')?.focus(); } }} onpointerdown={(e) => { if (menu && !e.composedPath().includes(menu)) menu.open = false; }} />
<div class="sidebar-divider relative mx-2.5 my-2" class:dragging class:compact={!divider.label && !editing} data-divider-id={divider.id}
	role="listitem" class:can-drag={!editing}>
	{#if editing}
		<form onsubmit={save} class="flex gap-1">
			<input bind:this={input} bind:value={label} aria-label="Divider label" placeholder="Label (optional)" maxlength="80"
				class="min-w-0 flex-1 rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-2 py-1 text-xs text-[var(--color-surface-text)] focus:outline-2 focus:outline-[var(--color-primary-500)]"
				onkeydown={(e) => { if (e.key === 'Escape') { e.stopPropagation(); editing = false; } }} />
			<button type="submit" class="accent-action rounded px-2 py-1 text-xs">Save</button>
		</form>
	{:else}
		<div class="divider-row flex min-h-6 items-center gap-2">
			<span class="h-px min-w-2 flex-1 bg-[var(--color-surface-border)]" aria-hidden="true"></span>
			{#if divider.label}<span class="max-w-[75%] truncate text-[11px] font-medium text-[var(--color-surface-text-muted)]" title={divider.label}>{divider.label}</span>{/if}
			{#if divider.label}<span class="h-px min-w-2 flex-1 bg-[var(--color-surface-border)]" aria-hidden="true"></span>{/if}
			<details bind:this={menu}>
				<summary class="divider-actions flex h-6 w-5 cursor-pointer list-none items-center justify-center rounded text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-base)] focus-visible:outline-2 focus-visible:outline-[var(--color-primary-500)]" aria-label="Actions for {divider.label || 'divider'}" title="Divider actions">⋯</summary>
				<div class="floating-surface absolute right-0 top-full z-[85] w-36 rounded-md border border-[var(--color-surface-border)] p-1 text-xs text-[var(--color-surface-text)]">
					<button type="button" onclick={rename}>Rename</button>
					<button type="button" disabled={first} onclick={() => { if (menu) menu.open = false; onmove(-1); menu?.querySelector('summary')?.focus(); }}>Move up</button>
					<button type="button" disabled={last} onclick={() => { if (menu) menu.open = false; onmove(1); menu?.querySelector('summary')?.focus(); }}>Move down</button>
					<button type="button" onclick={onremove}>Remove divider</button>
				</div>
			</details>
		</div>
	{/if}
</div>

<style>
	.compact { margin-top: 0; margin-bottom: 0; }
	.compact .divider-row { height: 12px; min-height: 12px; }
	.compact details { position: absolute; right: 0; top: 50%; transform: translateY(-50%); }
	.compact .divider-actions { background: var(--color-surface-overlay); }
	.can-drag { cursor: grab; user-select: none; }
	.dragging { opacity: 0.5; cursor: grabbing; }
	summary::-webkit-details-marker { display: none; }
	details button { display: block; width: 100%; padding: 6px 8px; border-radius: 4px; text-align: left; }
	details button:hover { background: var(--color-surface-base); }
	details button:disabled { opacity: 0.4; cursor: default; }
	@media (hover: hover) {
		.divider-actions { opacity: 0; }
		.sidebar-divider:hover .divider-actions, .sidebar-divider:focus-within .divider-actions { opacity: 1; }
	}
</style>

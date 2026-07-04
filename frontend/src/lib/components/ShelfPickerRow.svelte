<script lang="ts">
	import { parseLibraryIcon } from '$lib/utils/library-icons';

	interface Shelf {
		id: number;
		name: string;
		icon?: string;
		is_magic?: number;
		book_count?: number;
	}

	interface Props {
		shelf: Shelf;
		disabled?: boolean;
		actionLabel?: string;
		onSelect?: (shelf: Shelf) => void;
	}

	let {
		shelf,
		disabled = false,
		actionLabel = 'Add',
		onSelect
	}: Props = $props();

	let parsedIcon = $derived(parseLibraryIcon(shelf.icon || (shelf.is_magic === 1 ? 'sparkles' : 'bookmark')));
</script>

<button
	type="button"
	onclick={() => onSelect?.(shelf)}
	disabled={disabled}
	class="group flex w-full items-center justify-between gap-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-4 py-3 text-left transition-all duration-200 ease-out hover:-translate-y-0.5 hover:border-[var(--color-primary-500)]/45 hover:bg-[var(--color-surface-overlay)] hover:shadow-sm disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:border-[var(--color-surface-border)] disabled:hover:shadow-none"
>
	<span class="flex min-w-0 items-center gap-3">
		<span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition-transform duration-200 ease-out {shelf.is_magic === 1 ? 'bg-purple-500/15 text-purple-300' : 'bg-[var(--color-primary-500)]/15 text-[var(--color-primary-400)]'} group-hover:scale-105">
			{#if parsedIcon?.svg}
				<span class="shelf-picker-icon h-4 w-4">{@html parsedIcon.svg}</span>
			{:else}
				<span class="text-xs font-semibold uppercase">{(parsedIcon?.name || shelf.name || '?').slice(0, 1)}</span>
			{/if}
		</span>
		<span class="min-w-0">
			<span class="block truncate text-sm font-medium text-[var(--color-surface-text)]">{shelf.name}</span>
			<span class="block text-xs text-[var(--color-surface-text-muted)]">
				{shelf.book_count ?? 0} books{#if shelf.is_magic === 1} · Magic{:else} · Manual{/if}
			</span>
		</span>
	</span>
	<span class="shrink-0 text-xs font-medium text-[var(--color-primary-400)] transition-transform duration-200 ease-out group-hover:translate-x-0.5">{actionLabel}</span>
</button>

<style>
	.shelf-picker-icon :global(svg) {
		display: block;
		height: 100%;
		width: 100%;
		overflow: visible;
	}
</style>

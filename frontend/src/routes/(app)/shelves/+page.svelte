<script lang="ts">
	import { onMount } from 'svelte';
	import { parseLibraryIcon } from '$lib/utils/library-icons';

	interface Shelf {
		id: number;
		name: string;
		icon: string;
		is_magic: number;
		book_count: number;
		sort_order?: number;
	}

	let shelves = $state<Shelf[]>([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			const res = await fetch('/api/shelves', { cache: 'no-store' });
			if (res.ok) shelves = await res.json();
		} catch (e) {
			console.error('Failed to fetch shelves:', e);
		} finally {
			loading = false;
		}
	});
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold text-[var(--color-surface-text)]">Shelves</h1>
			<p class="text-[var(--color-surface-text-muted)] mt-1">Organize your books into collections</p>
		</div>
		<div class="flex flex-wrap gap-3">
			<a
				href="/shelves/new"
				class="inline-flex items-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-4 py-2 font-medium text-[var(--color-surface-text)] transition-colors hover:border-[var(--color-primary-500)]/45 hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-primary-400)]"
			>
				<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
				</svg>
				Create Shelf
			</a>
			<a
				href="/shelves/new?magic=true"
				class="accent-action inline-flex items-center rounded-lg px-4 py-2 font-medium transition-colors"
			>
				<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"></path>
				</svg>
				Create Magic Shelf
			</a>
		</div>
	</div>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if shelves.length === 0}
		<div class="text-center py-16 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
			<svg class="w-16 h-16 text-[var(--color-primary-400)] mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
			</svg>
			<h3 class="text-lg font-medium text-[var(--color-surface-text)] mb-2">No shelves yet</h3>
			<p class="text-[var(--color-surface-text-muted)]">Create shelves to organize your books</p>
		</div>
	{:else}
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each shelves as shelf}
				{@const parsedIcon = parseLibraryIcon(shelf.icon || (shelf.is_magic === 1 ? 'sparkles' : 'bookmark'))}
				<a href="/shelves/{shelf.id}" class="group relative overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5 transition-colors hover:border-[var(--color-primary-500)]/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]">
					{#if shelf.is_magic === 1}
						<div class="absolute top-3 right-3 flex items-center space-x-1 rounded-full border border-purple-400/30 bg-purple-500/15 px-2 py-1 text-xs font-medium text-purple-300">
							<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"></path>
							</svg>
							<span>Magic</span>
						</div>
					{/if}
					<div class="flex items-center gap-4 pr-7">
						<div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg {shelf.is_magic === 1 ? 'bg-purple-500/15 text-purple-300' : 'bg-[var(--color-primary-500)]/15 text-[var(--color-primary-400)]'}">
							{#if parsedIcon?.svg}
								<div class="shelf-icon h-6 w-6">{@html parsedIcon.svg}</div>
							{:else}
								<span class="text-sm font-semibold uppercase">{(parsedIcon?.name || shelf.name || '?').slice(0, 1)}</span>
							{/if}
						</div>
						<div class="min-w-0 flex-1">
							<h3 class="truncate font-semibold text-[var(--color-surface-text)] transition-colors group-hover:text-[var(--color-primary-400)]">{shelf.name}</h3>
							<p class="mt-0.5 text-sm text-[var(--color-surface-text-muted)]">{shelf.book_count} books</p>
							<p class="mt-1 text-xs font-medium uppercase tracking-wide {shelf.is_magic === 1 ? 'text-purple-300' : 'text-[var(--color-primary-400)]'}">
								{shelf.is_magic === 1 ? 'Smart rules' : 'Manual'}
							</p>
						</div>
						<svg class="absolute right-4 top-1/2 h-5 w-5 -translate-y-1/2 text-[var(--color-surface-text-muted)] opacity-0 transition-all group-hover:translate-x-0.5 group-hover:text-[var(--color-primary-400)] group-hover:opacity-100" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
						</svg>
					</div>
				</a>
			{/each}
		</div>
	{/if}
</div>

<style>
	.shelf-icon :global(svg) {
		height: 100%;
		width: 100%;
	}
</style>

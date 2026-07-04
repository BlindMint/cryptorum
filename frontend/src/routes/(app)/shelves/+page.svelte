<script lang="ts">
	import { onMount } from 'svelte';
	import ShelfModal from '$lib/components/ShelfModal.svelte';
	import { lenientSearchMatch } from '$lib/utils/search';
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
	let showShelfModal = $state(false);
	let createMagicShelf = $state(false);
	let searchQuery = $state('');
	let sortBy = $state<'custom' | 'name' | 'count' | 'type'>('custom');

	onMount(async () => {
		await fetchShelves();
	});

	async function fetchShelves() {
		loading = true;
		try {
			const res = await fetch('/api/shelves', { cache: 'no-store' });
			if (res.ok) shelves = await res.json();
		} catch (e) {
			console.error('Failed to fetch shelves:', e);
		} finally {
			loading = false;
		}
	}

	function openCreateShelf(magic: boolean) {
		createMagicShelf = magic;
		showShelfModal = true;
	}

	async function handleShelfSaved() {
		showShelfModal = false;
		await fetchShelves();
		await (window as any).refreshSidebar?.();
	}

	function getVisibleShelves() {
		const query = searchQuery.trim();
		const filtered = shelves.filter(shelf => lenientSearchMatch(query, shelf.name));
		return [...filtered].sort((a, b) => {
			if (sortBy === 'count') {
				return b.book_count - a.book_count || a.name.localeCompare(b.name);
			}
			if (sortBy === 'type') {
				return a.is_magic - b.is_magic || a.name.localeCompare(b.name);
			}
			if (sortBy === 'name') {
				return a.name.localeCompare(b.name);
			}
			return (a.sort_order ?? 0) - (b.sort_order ?? 0) || a.name.localeCompare(b.name);
		});
	}

	function getShelfCountLabel(): string {
		if (shelves.length === 0) return 'Organize your books into collections';
		if (!searchQuery.trim()) return `${shelves.length} shelves`;
		return `${getVisibleShelves().length} of ${shelves.length} shelves`;
	}
</script>

<div class="catalog-page-shell space-y-6">
	<div class="catalog-page-header">
		<div class="catalog-page-header-row">
			<div class="catalog-page-title-row">
				<h1 class="catalog-page-title">Shelves</h1>
				<p class="catalog-page-count">{getShelfCountLabel()}</p>
			</div>

			<div class="catalog-page-actions">
				<button
					type="button"
					onclick={() => openCreateShelf(false)}
					class="catalog-page-action catalog-page-action-secondary"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
					</svg>
					Create Shelf
				</button>
				<button
					type="button"
					onclick={() => openCreateShelf(true)}
					class="accent-action catalog-page-action"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"></path>
					</svg>
					Create Magic Shelf
				</button>
			</div>
		</div>

		{#if shelves.length > 0}
			<div class="catalog-page-controls">
				<input
					type="search"
					bind:value={searchQuery}
					placeholder="Search shelves"
					class="catalog-page-control"
				>
				<select
					bind:value={sortBy}
					class="catalog-page-control"
				>
					<option value="custom">Sidebar order</option>
					<option value="name">Sort by name</option>
					<option value="count">Sort by count</option>
					<option value="type">Sort by type</option>
				</select>
			</div>
		{/if}
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
	{:else if getVisibleShelves().length === 0}
		<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-12 text-center text-[var(--color-surface-text-muted)]">
			No shelves match your search
		</div>
	{:else}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2 2xl:grid-cols-3">
			{#each getVisibleShelves() as shelf}
				{@const parsedIcon = parseLibraryIcon(shelf.icon || (shelf.is_magic === 1 ? 'sparkles' : 'bookmark'))}
				<a href="/shelves/{shelf.id}" class="group relative flex items-center overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5 transition-colors hover:border-[var(--color-primary-500)]/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]">
					<div class="flex min-w-0 w-full items-center gap-4 pr-7">
						<div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg {shelf.is_magic === 1 ? 'bg-purple-500/15 text-purple-300' : 'bg-[var(--color-primary-500)]/15 text-[var(--color-primary-400)]'}">
							{#if parsedIcon?.svg}
								<div class="shelf-icon h-6 w-6">{@html parsedIcon.svg}</div>
							{:else}
								<span class="text-sm font-semibold uppercase">{(parsedIcon?.name || shelf.name || '?').slice(0, 1)}</span>
							{/if}
						</div>
						<div class="min-w-0 flex-1">
							<h3 class="line-clamp-2 break-words font-semibold leading-snug text-[var(--color-surface-text)] transition-colors group-hover:text-[var(--color-primary-400)]">{shelf.name}</h3>
							<p class="mt-0.5 text-sm text-[var(--color-surface-text-muted)]">{shelf.book_count} books</p>
							<p class="mt-1 inline-flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide {shelf.is_magic === 1 ? 'text-purple-300' : 'text-[var(--color-primary-400)]'}">
								{#if shelf.is_magic === 1}
									<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"></path>
									</svg>
									<span>Magic</span>
								{:else}
									<span>Manual</span>
								{/if}
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

<ShelfModal
	open={showShelfModal}
	mode="create"
	initialMagic={createMagicShelf}
	onClose={() => showShelfModal = false}
	onSaved={handleShelfSaved}
/>

<style>
	.shelf-icon :global(svg) {
		height: 100%;
		width: 100%;
	}
</style>

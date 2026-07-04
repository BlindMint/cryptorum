<script lang="ts">
	import { onMount } from 'svelte';
	import LibraryModal from '$lib/components/LibraryModal.svelte';
	import { lenientSearchMatch } from '$lib/utils/search';
	import { parseLibraryIcon } from '$lib/utils/library-icons';

	interface Library {
		id: number;
		name: string;
		icon: string;
		book_count: number;
		exclude_from_suggestions?: boolean;
		comic_spread_fallback?: string;
		is_importing?: boolean;
		sort_order?: number;
	}

	let libraries = $state<Library[]>([]);
	let loading = $state(true);
	let showLibraryModal = $state(false);
	let searchQuery = $state('');
	let sortBy = $state<'name' | 'count'>('name');

	onMount(async () => {
		await fetchLibraries();
	});

	async function fetchLibraries() {
		loading = true;
		try {
			const res = await fetch('/api/libraries', { cache: 'no-store' });
			if (res.ok) {
				const data = await res.json();
				if (Array.isArray(data)) libraries = data;
			}
		} catch (e) {
			console.error('Failed to fetch libraries:', e);
		} finally {
			loading = false;
		}
	}

	function getVisibleLibraries() {
		const query = searchQuery.trim();
		const filtered = libraries.filter(library => lenientSearchMatch(query, library.name));
		return [...filtered].sort((a, b) => {
			if (sortBy === 'count') {
				return b.book_count - a.book_count || a.name.localeCompare(b.name);
			}
			return a.name.localeCompare(b.name);
		});
	}

	function getLibraryCountLabel(): string {
		if (libraries.length === 0) return 'Add a library to get started';
		if (!searchQuery.trim()) return `${libraries.length} libraries`;
		return `${getVisibleLibraries().length} of ${libraries.length} libraries`;
	}

	async function handleLibrarySaved() {
		showLibraryModal = false;
		await fetchLibraries();
		await (window as any).refreshSidebar?.();
	}

	function libraryStatusLabel(library: Library): string | null {
		if (library.is_importing) return 'Scanning';
		if (library.exclude_from_suggestions) return 'Hidden from suggestions';
		return null;
	}
</script>

<div class="catalog-page-shell space-y-6">
	<div class="catalog-page-header">
		<div class="catalog-page-header-row">
			<div class="catalog-page-title-row">
				<h1 class="catalog-page-title">Libraries</h1>
				<p class="catalog-page-count">{getLibraryCountLabel()}</p>
			</div>

			<button
				type="button"
				onclick={() => showLibraryModal = true}
				class="accent-action catalog-page-action shrink-0"
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v12m6-6H6"></path>
				</svg>
				Add Library
			</button>
		</div>

		{#if libraries.length > 0}
			<div class="catalog-page-controls">
				<div class="catalog-search-field">
					<svg class="catalog-search-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-4.35-4.35M10.5 18a7.5 7.5 0 110-15 7.5 7.5 0 010 15z"></path>
					</svg>
					<input
						type="search"
						bind:value={searchQuery}
						placeholder="Search libraries"
						class="catalog-page-control catalog-search-control"
					>
				</div>
				<select
					bind:value={sortBy}
					class="catalog-page-control"
				>
					<option value="name">Sort by name</option>
					<option value="count">Sort by count</option>
				</select>
			</div>
		{/if}
	</div>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="h-12 w-12 animate-spin rounded-full border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if libraries.length === 0}
		<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-16 text-center">
			<svg class="mx-auto mb-4 h-24 w-24 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
			</svg>
			<p class="text-[var(--color-surface-text-muted)]">No libraries found</p>
		</div>
	{:else}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2 2xl:grid-cols-3">
			{#each getVisibleLibraries() as library}
				{@const parsedIcon = parseLibraryIcon(library.icon)}
				<a
					href="/library?library={library.id}"
					class="flex flex-col justify-center overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4 transition-colors hover:border-[var(--color-primary-500)]/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
				>
					<div class="flex min-w-0 w-full items-center gap-3">
						<div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary-500)]/15 text-[var(--color-primary-400)]">
							{#if parsedIcon?.svg}
								<div class="library-icon h-6 w-6">{@html parsedIcon.svg}</div>
							{:else}
								<svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
								</svg>
							{/if}
						</div>
						<div class="min-w-0 flex-1">
							<h3 class="line-clamp-2 break-words text-base font-semibold leading-snug text-[var(--color-surface-text)]">{library.name}</h3>
							<p class="text-sm text-[var(--color-surface-text-muted)]">{library.book_count} books</p>
						</div>
					</div>
					{#if libraryStatusLabel(library)}
						<div class="mt-3 inline-flex rounded-full border border-[var(--color-primary-500)]/35 bg-[var(--color-primary-500)]/10 px-2.5 py-1 text-xs font-medium text-[var(--color-primary-400)]">
							{libraryStatusLabel(library)}
						</div>
					{/if}
				</a>
			{/each}
			{#if getVisibleLibraries().length === 0}
				<div class="col-span-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-12 text-center text-[var(--color-surface-text-muted)]">
					No libraries match your search
				</div>
			{/if}
		</div>
	{/if}
</div>

<LibraryModal
	open={showLibraryModal}
	library={null}
	onClose={() => showLibraryModal = false}
	onSaved={handleLibrarySaved}
/>

<style>
	.library-icon :global(svg) {
		height: 100%;
		width: 100%;
	}
</style>

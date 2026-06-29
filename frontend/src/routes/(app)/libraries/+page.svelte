<script lang="ts">
	import { onMount } from 'svelte';
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

	function libraryStatusLabel(library: Library): string {
		if (library.is_importing) return 'Scanning';
		if (library.exclude_from_suggestions) return 'Hidden from suggestions';
		return 'Library';
	}
</script>

<div class="space-y-6">
	<div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
		<div>
			<h1 class="text-2xl font-bold text-[var(--color-surface-text)]">Libraries</h1>
			{#if libraries.length > 0}
				<p class="mt-1 text-[var(--color-surface-text-muted)]">{libraries.length} libraries</p>
			{/if}
		</div>
		<div class="flex flex-col gap-3 sm:flex-row">
			<input
				type="text"
				bind:value={searchQuery}
				placeholder="Search libraries"
				class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)]"
			>
			<select
				bind:value={sortBy}
				class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)]"
			>
				<option value="name">Sort by name</option>
				<option value="count">Sort by count</option>
			</select>
		</div>
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
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
			{#each getVisibleLibraries() as library}
				{@const parsedIcon = parseLibraryIcon(library.icon)}
				<a
					href="/library?library={library.id}"
					class="overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4 transition-colors hover:border-[var(--color-primary-500)]/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
				>
					<div class="mb-2 flex min-w-0 items-center gap-3">
						<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-[var(--color-primary-500)]/20 text-[var(--color-primary-400)]">
							{#if parsedIcon?.svg}
								<div class="library-icon h-5 w-5">{@html parsedIcon.svg}</div>
							{:else}
								<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
								</svg>
							{/if}
						</div>
						<div class="min-w-0 flex-1">
							<h3 class="truncate text-lg font-semibold text-[var(--color-surface-text)]">{library.name}</h3>
							<p class="text-sm text-[var(--color-surface-text-muted)]">{library.book_count} books</p>
						</div>
					</div>
					<div class="flex items-center justify-between gap-3 text-xs text-[var(--color-surface-text-muted)]">
						<span>{libraryStatusLabel(library)}</span>
						<span class="font-medium text-[var(--color-primary-400)]">Open</span>
					</div>
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

<style>
	.library-icon :global(svg) {
		height: 100%;
		width: 100%;
	}
</style>

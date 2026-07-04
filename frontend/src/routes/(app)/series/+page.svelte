<script lang="ts">
	import { onMount } from 'svelte';
	import { lenientSearchMatch } from '$lib/utils/search';

	let series = $state<any[]>([]);
	let loading = $state(true);
	let searchQuery = $state('');
	let sortBy = $state<'name' | 'count'>('name');

	onMount(async () => {
		await fetchSeries();
	});

	async function fetchSeries() {
		loading = true;
		try {
			const res = await fetch('/api/series');
			if (res.ok) {
				const data = await res.json();
				if (Array.isArray(data)) series = data;
			}
		} catch (e) {
			console.error('Failed to fetch series:', e);
		} finally {
			loading = false;
		}
	}

	function getVisibleSeries() {
		const query = searchQuery.trim();
		const filtered = series.filter(item => lenientSearchMatch(query, item.name));
		return [...filtered].sort((a, b) => {
			if (sortBy === 'count') {
				return b.book_count - a.book_count || a.name.localeCompare(b.name);
			}
			return a.name.localeCompare(b.name);
		});
	}

	function getSeriesCountLabel(): string {
		if (series.length === 0) return 'Series from your book metadata';
		if (!searchQuery.trim()) return `${series.length} series`;
		return `${getVisibleSeries().length} of ${series.length} series`;
	}
</script>

<div class="space-y-6">
	<div class="space-y-3">
		<div class="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1">
			<h1 class="text-2xl font-bold text-[var(--color-surface-text)]">Series</h1>
			<p class="whitespace-nowrap text-sm text-[var(--color-surface-text-muted)]">{getSeriesCountLabel()}</p>
		</div>

		<div class="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2">
			<input
				type="search"
				bind:value={searchQuery}
				placeholder="Search series"
				class="min-w-0 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
			>
			<select
				bind:value={sortBy}
				class="min-w-0 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
			>
				<option value="name">Sort by name</option>
				<option value="count">Sort by count</option>
			</select>
		</div>
	</div>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if series.length === 0}
		<div class="text-center py-16 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
			<svg class="w-24 h-24 text-[var(--color-surface-text-muted)] mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
			</svg>
			<p class="text-[var(--color-surface-text-muted)]">No series found</p>
		</div>
	{:else}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2 2xl:grid-cols-3">
			{#each getVisibleSeries() as serie}
				<a
					href="/library?series={encodeURIComponent(serie.name)}&sort=series&sort_dir=asc"
					class="flex items-center overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4 transition-colors hover:border-[var(--color-primary-500)]/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
				>
					<div class="flex min-w-0 w-full items-center gap-3">
						<div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary-500)]/15 text-[var(--color-primary-400)]">
							<svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
							</svg>
						</div>
						<div class="min-w-0 flex-1">
							<h3 class="line-clamp-2 break-words text-base font-semibold leading-snug text-[var(--color-surface-text)]">{serie.name}</h3>
							<p class="text-sm text-[var(--color-surface-text-muted)]">{serie.book_count} books</p>
						</div>
					</div>
				</a>
			{/each}
			{#if getVisibleSeries().length === 0}
				<div class="col-span-full text-center py-12 text-[var(--color-surface-text-muted)] bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
					No series match your search
				</div>
			{/if}
		</div>
	{/if}
</div>

<script lang="ts">
	import { onMount } from 'svelte';

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
		const query = searchQuery.trim().toLowerCase();
		const filtered = series.filter(item => !query || item.name.toLowerCase().includes(query));
		return [...filtered].sort((a, b) => {
			if (sortBy === 'count') {
				return b.book_count - a.book_count || a.name.localeCompare(b.name);
			}
			return a.name.localeCompare(b.name);
		});
	}
</script>

<div class="space-y-6">
	<div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
		<div>
		<h1 class="text-2xl font-bold text-[var(--color-surface-text)]">Series</h1>
		{#if series.length > 0}
			<p class="text-[var(--color-surface-text-muted)] mt-1">{series.length} series</p>
		{/if}
		</div>
		<div class="flex flex-col sm:flex-row gap-3">
			<input
				type="text"
				bind:value={searchQuery}
				placeholder="Search series"
				class="px-3 py-2 rounded-lg bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)]"
			>
			<select
				bind:value={sortBy}
				class="px-3 py-2 rounded-lg bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)]"
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
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
			{#each getVisibleSeries() as serie}
				<a
					href="/library?series={encodeURIComponent(serie.name)}"
					class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] p-4 hover:border-[var(--color-primary-500)]/50 transition-colors overflow-hidden"
				>
					<div class="flex items-center space-x-3 mb-2 min-w-0">
						<svg class="w-8 h-8 text-[var(--color-primary-500)] flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
						</svg>
						<div class="min-w-0 flex-1">
							<h3 class="text-lg font-semibold text-[var(--color-surface-text)] truncate">{serie.name}</h3>
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

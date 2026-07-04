<script lang="ts">
	import { onMount } from 'svelte';
	import { lenientSearchMatch } from '$lib/utils/search';

	let authors = $state<any[]>([]);
	let loading = $state(true);
	let searchQuery = $state('');
	let sortBy = $state<'name' | 'count'>('name');

	onMount(async () => {
		await fetchAuthors();
	});

	async function fetchAuthors() {
		loading = true;
		try {
			const res = await fetch('/api/authors');
			if (res.ok) {
				const data = await res.json();
				if (Array.isArray(data)) authors = data;
			}
		} catch (e) {
			console.error('Failed to fetch authors:', e);
		} finally {
			loading = false;
		}
	}

	function getVisibleAuthors() {
		const query = searchQuery.trim();
		const filtered = authors.filter(author => lenientSearchMatch(query, author.name));
		return [...filtered].sort((a, b) => {
			if (sortBy === 'count') {
				return b.book_count - a.book_count || a.name.localeCompare(b.name);
			}
			return a.name.localeCompare(b.name);
		});
	}

	function getAuthorCountLabel(): string {
		if (authors.length === 0) return 'Authors from your book metadata';
		if (!searchQuery.trim()) return `${authors.length} authors`;
		return `${getVisibleAuthors().length} of ${authors.length} authors`;
	}
</script>

<div class="catalog-page-shell space-y-6">
	<div class="catalog-page-header">
		<div class="catalog-page-title-row">
			<h1 class="catalog-page-title">Authors</h1>
			<p class="catalog-page-count">{getAuthorCountLabel()}</p>
		</div>

		<div class="catalog-page-controls">
			<div class="catalog-search-field">
				<svg class="catalog-search-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-4.35-4.35M10.5 18a7.5 7.5 0 110-15 7.5 7.5 0 010 15z"></path>
				</svg>
				<input
					type="search"
					bind:value={searchQuery}
					placeholder="Search authors"
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
	</div>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if authors.length === 0}
		<div class="text-center py-16 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
			<svg class="w-24 h-24 text-[var(--color-surface-text-muted)] mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"></path>
			</svg>
			<p class="text-[var(--color-surface-text-muted)]">No authors found</p>
		</div>
	{:else}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2 2xl:grid-cols-3">
			{#each getVisibleAuthors() as author}
				<a
					href="/library?author={encodeURIComponent(author.name)}"
					class="flex items-center overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4 transition-colors hover:border-[var(--color-primary-500)]/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
				>
					<div class="flex min-w-0 w-full items-center gap-3">
						<div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary-500)]/15 text-[var(--color-primary-400)]">
							<svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"></path>
							</svg>
						</div>
						<div class="min-w-0 flex-1">
							<h3 class="line-clamp-2 break-words text-base font-semibold leading-snug text-[var(--color-surface-text)]">{author.name}</h3>
							<p class="text-sm text-[var(--color-surface-text-muted)]">{author.book_count} books</p>
						</div>
					</div>
				</a>
			{/each}
			{#if getVisibleAuthors().length === 0}
				<div class="col-span-full text-center py-12 text-[var(--color-surface-text-muted)] bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
					No authors match your search
				</div>
			{/if}
		</div>
	{/if}
</div>

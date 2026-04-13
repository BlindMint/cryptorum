<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';

	interface Props {
		showHeader?: boolean;
	}

	let { showHeader = true }: Props = $props();

	let activeTab = $state('authors');
	let searchQuery = $state('');
	let metadata = $state<any[]>([]);
	let providers = $state<any[]>([]);
	let loading = $state(true);

	const tabs = [
		{ id: 'authors', label: 'Authors', icon: 'user' },
		{ id: 'series', label: 'Series', icon: 'collection' },
		{ id: 'genres', label: 'Genres', icon: 'tag' },
		{ id: 'publishers', label: 'Publishers', icon: 'building' },
		{ id: 'languages', label: 'Languages', icon: 'globe' }
	];

	async function fetchMetadata() {
		loading = true;
		try {
			const res = await fetch(`/api/metadata/${activeTab}`);
			if (res.ok) {
				metadata = await res.json();
			}
		} catch (error) {
			console.error('Failed to fetch metadata:', error);
		} finally {
			loading = false;
		}
	}

	async function fetchProviders() {
		try {
			const res = await fetch('/api/providers');
			if (res.ok) {
				providers = await res.json();
			}
		} catch (error) {
			console.error('Failed to fetch providers:', error);
		}
	}

	$effect(() => {
		activeTab;
		fetchMetadata();
	});

	function getFilteredMetadata() {
		if (!searchQuery) return metadata;
		return metadata.filter(item => item.name.toLowerCase().includes(searchQuery.toLowerCase()));
	}

	function getIconPath(icon: string) {
		const icons = {
			user: 'M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z',
			collection: 'M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10',
			tag: 'M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z',
			building: 'M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4',
			globe: 'M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9'
		};
		return icons[icon as keyof typeof icons] || icons.tag;
	}

	function openFilteredLibrary(item: any) {
		const url = new URL('/library', window.location.origin);
		const key = activeTab === 'authors' ? 'author' : activeTab === 'series' ? 'series' : activeTab === 'genres' ? 'genre' : activeTab === 'publishers' ? 'publisher' : 'language';
		url.searchParams.set(key, item.name);
		goto(url.pathname + url.search);
	}

	onMount(async () => {
		await fetchMetadata();
		await fetchProviders();
	});
</script>

<div class="space-y-6">
	{#if showHeader}
		<div>
			<h1 class="text-2xl font-bold text-[var(--color-surface-text)]">Metadata Manager</h1>
			<p class="text-[var(--color-surface-text-muted)] mt-1">Manage and organize your book metadata</p>
		</div>
	{/if}

	<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4">
		<div class="flex flex-wrap items-center gap-3">
			<span class="text-sm font-medium text-[var(--color-surface-text)]">Metadata Providers</span>
			{#if providers.length > 0}
				{#each providers as provider}
					<span class="inline-flex items-center rounded-full border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-1 text-sm text-[var(--color-surface-text-muted)]">
						{provider.name}
					</span>
				{/each}
			{:else}
				<span class="text-sm text-[var(--color-surface-text-muted)]">No providers loaded.</span>
			{/if}
		</div>
	</div>

	<div class="border-b border-[var(--color-surface-border)]">
		<nav class="flex space-x-8">
			{#each tabs as tab}
				<button
					onclick={() => activeTab = tab.id}
					class="flex items-center space-x-2 py-4 px-1 border-b-2 font-medium text-sm transition-colors {activeTab === tab.id ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:border-[var(--color-surface-border)]'}"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={getIconPath(tab.icon)}></path>
					</svg>
					<span>{tab.label}</span>
				</button>
			{/each}
		</nav>
	</div>

	<div class="flex items-center space-x-4">
		<div class="flex-1 max-w-md">
			<div class="relative">
				<svg class="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
				</svg>
				<input
					type="text"
					placeholder="Search {activeTab}..."
					bind:value={searchQuery}
					class="w-full pl-10 pr-4 py-2 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
				>
			</div>
		</div>
	</div>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else}
		<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] overflow-hidden">
			<div class="overflow-x-auto">
				<table class="w-full">
					<thead class="bg-[var(--color-surface-overlay)] border-b border-[var(--color-surface-border)]">
						<tr>
							<th class="px-6 py-3 text-left text-xs font-medium text-[var(--color-surface-text-muted)] uppercase tracking-wider">Name</th>
							<th class="px-6 py-3 text-left text-xs font-medium text-[var(--color-surface-text-muted)] uppercase tracking-wider">Books</th>
							<th class="px-6 py-3 text-left text-xs font-medium text-[var(--color-surface-text-muted)] uppercase tracking-wider">Actions</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-[var(--color-surface-border)]">
						{#each getFilteredMetadata() as item}
							<tr class="hover:bg-[var(--color-surface-overlay)]">
								<td class="px-6 py-4 whitespace-nowrap">
									<div class="flex items-center">
										<svg class="w-5 h-5 text-[var(--color-primary-500)] mr-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={getIconPath(activeTab === 'authors' ? 'user' : activeTab === 'series' ? 'collection' : 'tag')}></path>
										</svg>
										<div>
											<div class="text-sm font-medium text-[var(--color-surface-text)]">{item.name}</div>
										</div>
									</div>
								</td>
								<td class="px-6 py-4 whitespace-nowrap text-sm text-[var(--color-surface-text-muted)]">
									{item.book_count} books
								</td>
								<td class="px-6 py-4 whitespace-nowrap text-sm font-medium space-x-2">
									<button onclick={() => openFilteredLibrary(item)} class="text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">
										View Books
									</button>
								</td>
							</tr>
						{:else}
							<tr>
								<td colspan="3" class="px-6 py-12 text-center text-[var(--color-surface-text-muted)]">
									No {activeTab} found
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}
</div>

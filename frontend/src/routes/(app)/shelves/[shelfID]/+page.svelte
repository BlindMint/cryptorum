<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { showFormatOnCover, getFormatColor } from '$lib/stores';

	let shelf = $state<any>(null);
	let books = $state<any[]>([]);
	let loading = $state(true);
	let formatOnCover = $state(true);

	$effect(() => {
		const unsub = showFormatOnCover.subscribe((value: boolean) => formatOnCover = value);
		return unsub;
	});

	onMount(async () => {
		showFormatOnCover.init();
		await fetchShelfBooks();
	});

	async function fetchShelfBooks() {
		loading = true;
		const shelfId = $page.params.shelfID;
		try {
			const [shelfRes, booksRes] = await Promise.all([
				fetch(`/api/shelves/${shelfId}`),
				fetch(`/api/shelves/${shelfId}/books`)
			]);

			if (shelfRes.ok) shelf = await shelfRes.json();
			if (booksRes.ok) books = await booksRes.json();
		} catch (e) {
			console.error('Failed to fetch shelf:', e);
		} finally {
			loading = false;
		}
	}

	function parseAuthors(authorsJson: string): string {
		try {
			const arr = JSON.parse(authorsJson);
			return Array.isArray(arr) ? arr.join(', ') : authorsJson;
		} catch {
			return authorsJson;
		}
	}

	function formatDate(timestamp: number) {
		return new Date(timestamp * 1000).toLocaleDateString();
	}
</script>

<div class="space-y-6">
	<a href="/shelves" class="inline-flex items-center text-slate-400 hover:text-white transition-colors">
		<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
		</svg>
		Back to Shelves
	</a>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-amber-500"></div>
		</div>
	{:else if shelf}
		<div>
			<h1 class="text-2xl font-bold text-white">{shelf.name}</h1>
			<p class="text-slate-400 mt-1">{books.length} books</p>
		</div>

		{#if books.length === 0}
			<div class="text-center py-16 bg-slate-800 rounded-lg border border-slate-700">
				<svg class="w-16 h-16 text-slate-600 mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
				</svg>
				<h3 class="text-lg font-medium text-white mb-2">No books on this shelf</h3>
				<p class="text-slate-400">Add books to this shelf from the book detail page</p>
			</div>
			{:else}
			<div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
				{#each books as book}
					<a href="/book/{book.id}" class="block group">
						<div class="relative aspect-[2/3] bg-slate-800 rounded-lg overflow-hidden mb-2">
							{#if book.cover_path}
								<img src="/api/covers/{book.id}" alt={book.title} class="w-full h-full object-cover group-hover:scale-105 transition-transform">
							{:else}
								<div class="w-full h-full flex items-center justify-center">
									<svg class="w-12 h-12 text-slate-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
									</svg>
									</div>
								{/if}
								{#if formatOnCover && book.format}
									{@const formatColor = getFormatColor(book.format)}
									<div
										class="absolute bottom-2 left-2 z-10 px-1.5 py-0.5 rounded text-[10px] font-medium uppercase border border-black/20 shadow-[0_1px_2px_rgba(0,0,0,0.35)]"
										style="background-color: {formatColor.bg}; color: {formatColor.text};"
									>
										{book.format}
									</div>
								{/if}
						</div>
						<h3 class="text-sm font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
						{#if book.authors && book.authors !== '[]'}
							<p class="text-xs text-[var(--color-surface-text-muted)] truncate">{parseAuthors(book.authors)}</p>
						{/if}
					</a>
				{/each}
			</div>
		{/if}
	{:else}
		<div class="text-center py-16 bg-slate-800 rounded-lg border border-slate-700">
			<p class="text-slate-400">Shelf not found</p>
		</div>
	{/if}
</div>

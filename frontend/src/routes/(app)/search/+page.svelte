<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';

  let query = $state($page.url.searchParams.get('q') || '');
  let results = $state<any[]>([]);
  let loading = $state(false);
  let viewMode = $state('grid');
  let gridSize = $state(6);

  function parseAuthors(authors: string): string {
  	try {
  		const parsed = JSON.parse(authors);
  		return Array.isArray(parsed) ? parsed.join(', ') : authors;
  	} catch {
  		return authors;
  	}
  }

	onMount(() => {
		if (query) search();
	});

	async function search() {
		if (!query.trim()) {
			results = [];
			return;
		}

		loading = true;
		try {
			const res = await fetch(`/api/search?q=${encodeURIComponent(query)}`);
			results = await res.json();
		} catch (e) {
			console.error('Search failed:', e);
		} finally {
			loading = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			search();
		}
	}
</script>

<div class="space-y-6">
	<div class="max-w-2xl mx-auto">
		<h1 class="text-2xl font-bold text-[var(--color-surface-text)] mb-6">Search</h1>
		<div class="flex gap-3">
			<input
				type="text"
				bind:value={query}
				onkeydown={handleKeydown}
				placeholder="Search books by title, author, or description..."
				class="flex-1 px-4 py-3 border border-[var(--color-surface-border)] rounded-lg bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:ring-2 focus:ring-[var(--color-primary-500)] focus:border-transparent"
			>
			<button
				onclick={search}
				disabled={loading}
				class="px-6 py-3 bg-[var(--color-primary-500)] text-white rounded-lg hover:bg-[var(--color-primary-600)] disabled:opacity-50 transition-colors"
			>
				{loading ? 'Searching...' : 'Search'}
			</button>
		</div>
	</div>

  {#if results.length > 0}
  		<div class="max-w-7xl mx-auto">
  			<div class="flex items-center justify-between mb-6">
  				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">
  					{results.length} result{results.length === 1 ? '' : 's'} found
  				</h2>

  				<!-- View Toggle -->
  				<div class="flex items-center space-x-2">
  					<button
  						onclick={() => viewMode = 'grid'}
  						class="p-2 rounded-lg {viewMode === 'grid' ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'} transition-colors"
  						aria-label="Grid view"
  					>
  						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
  							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z"></path>
  						</svg>
  					</button>
  					<button
  						onclick={() => viewMode = 'list'}
  						class="p-2 rounded-lg {viewMode === 'list' ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'} transition-colors"
  						aria-label="List view"
  					>
  						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
  							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16"></path>
  						</svg>
  					</button>

  					{#if viewMode === 'grid'}
  						<div class="flex items-center space-x-2">
  							<span class="text-sm text-[var(--color-surface-text-muted)]">Grid size:</span>
  							<input
  								type="range"
  								min="3"
  								max="8"
  								bind:value={gridSize}
  								class="w-20"
  							>
  							<span class="text-sm text-[var(--color-surface-text)] w-6">{gridSize}</span>
  						</div>
  					{/if}
  				</div>
  			</div>

  			<div class={viewMode === 'grid' ? `grid gap-4 grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-${Math.min(gridSize, 6)} xl:grid-cols-${Math.min(gridSize, 8)}` : 'space-y-4'}>
				{#each results as book}
					{#if viewMode === 'grid'}
						<a href="/book/{book.id}" class="block group">
							<div class="relative">
								<BookCoverFrame
									src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
									alt={book.title}
									mode="contain"
									frameClass="aspect-[2/3] mb-2"
									imageClass="group-hover:scale-105 transition-transform"
									placeholderSize="md"
								/>
								{#if book.status === 'reading' || book.status === 'finished'}
									<span class="absolute top-1 right-1 z-10 w-2.5 h-2.5 rounded-full bg-[var(--color-primary-500)]"></span>
								{/if}
								{#if book.opened && book.percent > 0}
									<div class="absolute bottom-1.5 left-0 right-0 z-10 h-1 bg-slate-700">
										<div class="h-full bg-[var(--color-primary-500)] transition-all duration-300" style="width: {book.percent}%"></div>
									</div>
								{/if}
							</div>
							<h3 class="text-sm font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
							{#if book.authors && book.authors !== '[]'}
								<p class="text-xs text-[var(--color-surface-text-muted)] truncate">{parseAuthors(book.authors)}</p>
							{/if}
						</a>
					{:else}
						<!-- List view -->
						<a href="/book/{book.id}" class="block bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] p-4 hover:border-[var(--color-primary-500)]/50 transition-colors">
							<div class="flex items-center space-x-4">
								<BookCoverFrame
									src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
									alt={book.title}
									mode="contain"
									frameClass="w-12 h-16 flex-shrink-0"
									imageClass="object-cover"
									placeholderSize="sm"
								/>
								<div class="flex-1 min-w-0">
									<div class="flex items-center space-x-2 mb-1">
										<h3 class="text-lg font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
  										{#if book.status === 'reading' || book.status === 'finished'}
  											<span class="w-2.5 h-2.5 rounded-full {book.status === 'reading' ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-primary-500)]'} flex-shrink-0"></span>
  										{/if}
  									</div>
  									{#if book.authors && book.authors !== '[]'}
  										<p class="text-sm text-[var(--color-surface-text-muted)] mb-1">{parseAuthors(book.authors)}</p>
  									{/if}
  									{#if book.status === 'reading' && book.percent > 0}
  										<div class="w-full bg-[var(--color-surface-700)] rounded-full h-1.5">
  											<div class="h-full bg-[var(--color-primary-500)] rounded-full transition-all duration-300" style="width: {book.percent}%"></div>
  										</div>
  									{/if}
  								</div>
  							</div>
  						</a>
  					{/if}
  				{/each}
  			</div>
  		</div>
	{:else if query && !loading}
		<div class="text-center py-12">
			<svg class="w-16 h-16 text-[var(--color-primary-400)] mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
			</svg>
			<p class="text-[var(--color-surface-text-muted)]">No results found for "{query}"</p>
		</div>
	{:else if !query}
		<div class="text-center py-12">
			<svg class="w-16 h-16 text-[var(--color-primary-400)] mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
			</svg>
			<p class="text-[var(--color-surface-text-muted)]">Enter a search term to find books</p>
		</div>
	{/if}
</div>

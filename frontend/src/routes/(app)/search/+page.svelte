<script lang="ts">
	import { page } from '$app/stores';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import MetadataLookupModal from '$lib/components/MetadataLookupModal.svelte';
	import BulkMetadataReviewModal from '$lib/components/BulkMetadataReviewModal.svelte';

	let query = $state($page.url.searchParams.get('q') || '');
	let libraryFilter = $state($page.url.searchParams.get('library') || '');
	let libraryName = $state('');
	let results = $state<any[]>([]);
	let loading = $state(false);
	let viewMode = $state('grid');
	let gridSize = $state(6);
	let selectedBooks = $state<Set<number>>(new Set());
	let showShelfPicker = $state(false);
	let shelves = $state<any[]>([]);
	let actionInProgress = $state(false);
	let showMetadataLookup = $state(false);
	let showMetadataMenu = $state(false);
	let metadataLookupQueueing = $state(false);
	let metadataLookupJob = $state<any | null>(null);
	let showBulkMetadataReview = $state(false);
	let longPressTimer: number | null = null;
	let longPressThreshold = 500;
	let suppressNextClickBookId: number | null = null;
	let lastUrlSearch = '';

	let bulkSelectMode = $derived(selectedBooks.size > 0);
	let gridStyle = $derived(viewMode === 'grid'
		? `grid-template-columns: repeat(${gridSize}, minmax(0, 1fr))`
		: '');

	function parseAuthors(authors: string): string {
		try {
			const parsed = JSON.parse(authors);
			return Array.isArray(parsed) ? parsed.join(', ') : authors;
		} catch {
			return authors;
		}
	}

	async function fetchLibraryName() {
		if (!libraryFilter) {
			libraryName = '';
			return;
		}

		try {
			const res = await fetch(`/api/libraries/${libraryFilter}`);
			if (res.ok) {
				const data = await res.json();
				libraryName = data.name || '';
			}
		} catch (error) {
			console.error('Failed to fetch library name:', error);
		}
	}

	function buildSearchUrl(): string {
		const params = new URLSearchParams();
		params.set('q', query.trim());
		if (libraryFilter) params.set('library_id', libraryFilter);
		return `/api/search?${params.toString()}`;
	}

	$effect(() => {
		const currentSearch = $page.url.search;
		if (currentSearch === lastUrlSearch) return;
		lastUrlSearch = currentSearch;

		query = $page.url.searchParams.get('q') || '';
		libraryFilter = $page.url.searchParams.get('library') || '';
		if (libraryFilter) {
			fetchLibraryName();
		} else {
			libraryName = '';
		}

		if (query) {
			search();
		} else {
			results = [];
			deselectAll();
		}
	});

	async function search() {
		const trimmed = query.trim();
		if (!trimmed) {
			results = [];
			selectedBooks = new Set();
			return;
		}

		loading = true;
		selectedBooks = new Set();
		showMetadataMenu = false;
		try {
			const res = await fetch(buildSearchUrl());
			if (res.ok) {
				results = await res.json();
			} else {
				results = [];
			}
		} catch (error) {
			console.error('Search failed:', error);
		} finally {
			loading = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			search();
		}
	}

	function toggleBookSelection(bookId: number, event?: Event) {
		if (event) {
			event.preventDefault();
			event.stopPropagation();
		}
		const next = new Set(selectedBooks);
		if (next.has(bookId)) {
			next.delete(bookId);
		} else {
			next.add(bookId);
		}
		selectedBooks = next;
	}

	function handleBookClick(event: MouseEvent) {
		const bookId = Number((event.currentTarget as HTMLElement).dataset.bookId);
		if (suppressNextClickBookId === bookId) {
			event.preventDefault();
			event.stopPropagation();
			suppressNextClickBookId = null;
			return;
		}
		if (bulkSelectMode) {
			event.preventDefault();
			event.stopPropagation();
			toggleBookSelection(bookId);
		}
	}

	function handleBookKeydown(event: KeyboardEvent) {
		if (event.key !== 'Enter' && event.key !== ' ') return;
		event.preventDefault();
		handleBookClick(event as unknown as MouseEvent);
	}

	function handleMouseDown(event: MouseEvent) {
		if (typeof window === 'undefined' || 'ontouchstart' in window) return;
		const bookId = Number((event.currentTarget as HTMLElement).dataset.bookId);
		longPressTimer = window.setTimeout(() => {
			suppressNextClickBookId = bookId;
			toggleBookSelection(bookId);
			longPressTimer = null;
		}, longPressThreshold);
	}

	function handleMouseUp() {
		if (longPressTimer) {
			clearTimeout(longPressTimer);
			longPressTimer = null;
		}
	}

	function handleTouchStart(event: TouchEvent) {
		const bookId = Number((event.currentTarget as HTMLElement).dataset.bookId);
		longPressTimer = window.setTimeout(() => {
			suppressNextClickBookId = bookId;
			toggleBookSelection(bookId);
			longPressTimer = null;
		}, longPressThreshold);
	}

	function handleTouchEnd() {
		if (longPressTimer) {
			clearTimeout(longPressTimer);
			longPressTimer = null;
		}
	}

	function selectAllPage() {
		selectedBooks = new Set(results.map((book) => book.id));
	}

	function deselectAll() {
		selectedBooks = new Set();
	}

	async function fetchShelves() {
		try {
			const res = await fetch('/api/shelves');
			if (res.ok) {
				shelves = await res.json();
			}
		} catch (error) {
			console.error('Failed to fetch shelves:', error);
		}
	}

	async function addToShelf(shelfId: number) {
		actionInProgress = true;
		try {
			const res = await fetch(`/api/shelves/${shelfId}/books/bulk`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ book_ids: Array.from(selectedBooks) })
			});

			if (res.ok) {
				showShelfPicker = false;
				deselectAll();
			} else {
				console.error('Failed to add books to shelf');
			}
		} catch (error) {
			console.error('Failed to add books to shelf:', error);
		} finally {
			actionInProgress = false;
		}
	}

	async function deleteSelectedBooks() {
		if (selectedBooks.size === 0) return;
		if (!confirm(`Delete ${selectedBooks.size} book(s)? This cannot be undone.`)) return;

		actionInProgress = true;
		try {
			const res = await fetch('/api/books/bulk-delete', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ book_ids: Array.from(selectedBooks) })
			});

			if (res.ok) {
				await search();
				deselectAll();
			} else {
				console.error('Failed to delete books');
			}
		} catch (error) {
			console.error('Failed to delete books:', error);
		} finally {
			actionInProgress = false;
		}
	}

	function openShelfPicker() {
		fetchShelves();
		showShelfPicker = true;
	}

	function openMetadataLookup() {
		if (selectedBooks.size === 0) return;
		showMetadataMenu = false;
		showMetadataLookup = true;
	}

	async function queueBulkMetadataLookup() {
		if (selectedBooks.size === 0 || metadataLookupQueueing) return;
		showMetadataMenu = false;
		metadataLookupQueueing = true;
		try {
			const res = await fetch('/api/jobs/metadata-lookup', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ book_ids: Array.from(selectedBooks) })
			});
			if (!res.ok) {
				throw new Error(await res.text());
			}
			metadataLookupJob = await res.json();
			showBulkMetadataReview = true;
		} catch (error) {
			console.error('Failed to queue metadata lookup:', error);
		} finally {
			metadataLookupQueueing = false;
		}
	}

	async function refreshAfterMetadataLookup() {
		await search();
		showMetadataLookup = false;
	}
</script>

<div class="space-y-6">
	<div class="max-w-2xl mx-auto">
		<h1 class="text-2xl font-bold text-[var(--color-surface-text)] mb-2">Search</h1>
		{#if libraryName}
			<p class="mb-4 text-sm text-[var(--color-surface-text-muted)]">Scoped to {libraryName}</p>
		{/if}
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
			<div class="flex items-center justify-between mb-6 gap-4 flex-wrap">
				<div>
					<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">
						{results.length} result{results.length === 1 ? '' : 's'} found
					</h2>
				</div>

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

			<div class={viewMode === 'grid' ? 'grid gap-4 grid-cols-2 sm:grid-cols-3 md:grid-cols-4' : 'space-y-4'} style={viewMode === 'grid' ? gridStyle : ''}>
				{#each results as book}
					{#if viewMode === 'grid'}
						<div
							class="relative group {selectedBooks.has(book.id) ? 'ring-2 ring-[var(--color-primary-500)] ring-offset-2 ring-offset-[var(--color-surface-base)] rounded-lg' : ''}"
							data-book-id={book.id}
							onclick={handleBookClick}
							onmousedown={handleMouseDown}
							onmouseup={handleMouseUp}
							ontouchstart={handleTouchStart}
							ontouchend={handleTouchEnd}
							onkeydown={handleBookKeydown}
							role="button"
							tabindex="0"
						>
							<a href="/book/{book.id}" class="block">
								<div class="relative">
									<BookCoverFrame
										src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
										alt={book.title}
										mode="cover"
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
							<button
								onclick={(event) => toggleBookSelection(book.id, event)}
								class="absolute top-2 left-2 z-20 w-6 h-6 rounded border-2 transition-all opacity-0 group-hover:opacity-100 {bulkSelectMode ? 'opacity-100' : ''} {selectedBooks.has(book.id) ? 'bg-[var(--color-primary-500)] border-[var(--color-primary-500)]' : 'bg-[var(--color-surface-800)]/90 border-[var(--color-surface-400)]'} flex items-center justify-center"
								aria-label={selectedBooks.has(book.id) ? 'Deselect book' : 'Select book'}
							>
								{#if selectedBooks.has(book.id)}
									<svg class="w-4 h-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7"></path>
									</svg>
								{/if}
							</button>
						</div>
					{:else}
						<div
							class="relative group"
							data-book-id={book.id}
							onclick={handleBookClick}
							onmousedown={handleMouseDown}
							onmouseup={handleMouseUp}
							ontouchstart={handleTouchStart}
							ontouchend={handleTouchEnd}
							onkeydown={handleBookKeydown}
							role="button"
							tabindex="0"
						>
							<a href="/book/{book.id}" class="block bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] {selectedBooks.has(book.id) ? 'border-[var(--color-primary-500)]' : ''} p-4 hover:border-[var(--color-primary-500)]/50 transition-colors">
								<div class="flex items-center space-x-4">
									<BookCoverFrame
										src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
										alt={book.title}
										mode="cover"
										frameClass="w-12 h-16 flex-shrink-0"
										imageClass="object-cover"
										placeholderSize="sm"
									/>
									<div class="flex-1 min-w-0">
										<div class="flex items-center space-x-2 mb-1">
											<h3 class="text-lg font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
											{#if book.status === 'reading' || book.status === 'finished'}
												<span class="w-2.5 h-2.5 rounded-full bg-[var(--color-primary-500)] flex-shrink-0"></span>
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
							<button
								onclick={(event) => toggleBookSelection(book.id, event)}
								class="absolute top-1/2 -translate-y-1/2 left-3 z-20 w-6 h-6 rounded border-2 transition-all opacity-0 group-hover:opacity-100 {bulkSelectMode ? 'opacity-100' : ''} {selectedBooks.has(book.id) ? 'bg-[var(--color-primary-500)] border-[var(--color-primary-500)]' : 'bg-[var(--color-surface-800)]/90 border-[var(--color-surface-400)]'} flex items-center justify-center"
								aria-label={selectedBooks.has(book.id) ? 'Deselect book' : 'Select book'}
							>
								{#if selectedBooks.has(book.id)}
									<svg class="w-4 h-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7"></path>
									</svg>
								{/if}
							</button>
						</div>
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

{#if selectedBooks.size > 0}
	<div class="fixed bottom-0 left-0 right-0 z-50 animate-slide-up">
		<div class="bg-[var(--color-surface-overlay)] backdrop-blur-lg border-t border-[var(--color-surface-border)] shadow-2xl">
			<div class="max-w-7xl mx-auto px-4 py-3">
				<div class="flex items-center justify-between gap-4 flex-wrap">
					<div class="flex items-center gap-4 flex-wrap">
						<span class="text-[var(--color-surface-text)] font-medium">
							{selectedBooks.size} selected
						</span>
						<div class="flex items-center gap-2">
							<button
								onclick={selectAllPage}
								class="px-3 py-1.5 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] transition-colors"
							>
								Select All on Page
							</button>
							<button
								onclick={deselectAll}
								class="px-3 py-1.5 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] transition-colors"
							>
								Deselect
							</button>
						</div>
					</div>
					<div class="flex items-center gap-2">
						<div class="relative">
							<button
								onclick={() => showMetadataMenu = !showMetadataMenu}
								disabled={selectedBooks.size === 0 || metadataLookupQueueing}
								class="px-4 py-2 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] font-medium transition-colors disabled:opacity-50 flex items-center gap-2"
							>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
								</svg>
								<span>{metadataLookupQueueing ? 'Queueing...' : 'Metadata'}</span>
							</button>
							{#if showMetadataMenu}
								<div class="absolute bottom-full right-0 mb-2 w-72 overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
									<button
										type="button"
										class="block w-full px-4 py-3 text-left text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
										onclick={openMetadataLookup}
									>
										<div class="font-medium">Lookup selected books</div>
										<div class="mt-0.5 text-xs text-[var(--color-surface-text-muted)]">Review and search one book at a time.</div>
									</button>
									<button
										type="button"
										class="block w-full border-t border-[var(--color-surface-border)] px-4 py-3 text-left text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
										onclick={queueBulkMetadataLookup}
									>
										<div class="font-medium">Queue bulk metadata lookup</div>
										<div class="mt-0.5 text-xs text-[var(--color-surface-text-muted)]">Find the top match for every selected book.</div>
									</button>
								</div>
							{/if}
						</div>
						<button
							onclick={openShelfPicker}
							disabled={actionInProgress}
							class="px-4 py-2 text-sm rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white font-medium transition-colors disabled:opacity-50 flex items-center gap-2"
						>
							<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
							</svg>
							<span>Add to Shelf</span>
						</button>
						<button
							onclick={deleteSelectedBooks}
							disabled={actionInProgress}
							class="px-4 py-2 text-sm rounded-lg bg-red-500 hover:bg-red-600 text-white font-medium transition-colors disabled:opacity-50 flex items-center gap-2"
						>
							{#if actionInProgress}
								<svg class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
									<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
									<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
								</svg>
							{:else}
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
								</svg>
							{/if}
							<span>Delete</span>
						</button>
					</div>
				</div>
			</div>
		</div>
	</div>
{/if}

{#if showShelfPicker}
	<div class="fixed inset-0 z-[60] flex items-center justify-center">
		<button type="button" class="absolute inset-0 bg-black/60" aria-label="Close shelf picker" onclick={() => showShelfPicker = false}></button>
		<div class="relative bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] w-full max-w-md max-h-[80vh] overflow-hidden shadow-2xl">
			<div class="px-6 py-4 border-b border-[var(--color-surface-border)]">
				<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Add to Shelf</h3>
				<p class="text-sm text-[var(--color-surface-text-muted)] mt-1">Add {selectedBooks.size} book(s) to shelf</p>
			</div>
			<div class="p-4 max-h-64 overflow-y-auto">
				{#if shelves.length === 0}
					<p class="text-center text-[var(--color-surface-text-muted)] py-4">No shelves yet. Create one first.</p>
				{:else}
					<div class="space-y-2">
						{#each shelves as shelf}
							<button
								onclick={() => addToShelf(shelf.id)}
								disabled={actionInProgress}
								class="w-full flex items-center space-x-3 px-4 py-3 rounded-lg bg-[var(--color-surface-base)] hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] transition-colors disabled:opacity-50"
							>
								<svg class="w-5 h-5 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
								</svg>
								<span class="flex-1 text-left">{shelf.name}</span>
								<span class="text-sm text-[var(--color-surface-text-muted)]">{shelf.book_count} books</span>
							</button>
						{/each}
					</div>
				{/if}
			</div>
			<div class="px-6 py-4 border-t border-[var(--color-surface-border)]">
				<a href="/shelves/new" class="block w-full text-center px-4 py-2 text-sm rounded-lg border border-dashed border-[var(--color-surface-border)] text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] transition-colors">
					+ Create New Shelf
				</a>
			</div>
		</div>
	</div>
{/if}

{#if showMetadataLookup}
	<MetadataLookupModal
		bookIds={Array.from(selectedBooks)}
		title="Lookup Selected Books"
		onClose={() => showMetadataLookup = false}
		onApplied={refreshAfterMetadataLookup}
	/>
{/if}

{#if showBulkMetadataReview && metadataLookupJob?.id}
	<BulkMetadataReviewModal
		jobId={metadataLookupJob.id}
		initialJob={metadataLookupJob}
		onClose={() => showBulkMetadataReview = false}
		onApplied={async () => search()}
	/>
{/if}

<style>
	@keyframes slide-up {
		from {
			transform: translateY(100%);
		}
		to {
			transform: translateY(0);
		}
	}

	.animate-slide-up {
		animation: slide-up 0.2s ease-out;
	}
</style>

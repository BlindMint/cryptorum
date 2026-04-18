<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import MetadataLookupModal from '$lib/components/MetadataLookupModal.svelte';
	import BulkMetadataReviewModal from '$lib/components/BulkMetadataReviewModal.svelte';
	import { showFormatOnCover, getFormatColor } from '$lib/stores';

	let shelf = $state<any>(null);
	let books = $state<any[]>([]);
	let loading = $state(true);
	let formatOnCover = $state(true);
	let selectedBooks = $state<Set<number>>(new Set());
	let showMetadataLookup = $state(false);
	let showMetadataMenu = $state(false);
	let metadataLookupQueueing = $state(false);
	let metadataLookupJob = $state<any | null>(null);
	let showBulkMetadataReview = $state(false);
	let actionInProgress = $state(false);
	let longPressTimer: number | null = null;
	let longPressThreshold = 500;
	let suppressNextClickBookId: number | null = null;

	let bulkSelectMode = $derived(selectedBooks.size > 0);

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
		} catch (error) {
			console.error('Failed to fetch shelf:', error);
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
		selectedBooks = new Set(books.map((book) => book.id));
	}

	function deselectAll() {
		selectedBooks = new Set();
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

	function openMetadataLookup() {
		if (selectedBooks.size === 0) return;
		showMetadataMenu = false;
		showMetadataLookup = true;
	}

	async function refreshAfterMetadataLookup() {
		await fetchShelfBooks();
		showMetadataLookup = false;
	}

	async function removeSelectedFromShelf() {
		if (selectedBooks.size === 0 || !shelf) return;
		if (!confirm(`Remove ${selectedBooks.size} book(s) from this shelf?`)) return;

		actionInProgress = true;
		try {
			const res = await fetch(`/api/shelves/${shelf.id}/books/bulk`, {
				method: 'DELETE',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ book_ids: Array.from(selectedBooks) })
			});

			if (res.ok) {
				await fetchShelfBooks();
				deselectAll();
			} else {
				console.error('Failed to remove books from shelf');
			}
		} catch (error) {
			console.error('Failed to remove books from shelf:', error);
		} finally {
			actionInProgress = false;
		}
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
				{/each}
			</div>
		{/if}
	{:else}
		<div class="text-center py-16 bg-slate-800 rounded-lg border border-slate-700">
			<p class="text-slate-400">Shelf not found</p>
		</div>
	{/if}
</div>

{#if selectedBooks.size > 0}
	<div class="fixed bottom-0 left-0 right-0 z-50 animate-slide-up">
		<div class="bg-[var(--color-surface-overlay)] backdrop-blur-lg border-t border-[var(--color-surface-border)] shadow-2xl">
			<div class="max-w-7xl mx-auto px-4 py-3">
				<div class="flex items-center justify-between gap-4 flex-wrap">
					<div class="flex items-center gap-4 flex-wrap">
						<span class="text-[var(--color-surface-text)] font-medium">{selectedBooks.size} selected</span>
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
							onclick={removeSelectedFromShelf}
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
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 6h14M10 6V4h4v2m-5 4v6m4-6v6M6 6l1 14h10l1-14"></path>
								</svg>
							{/if}
							<span>Remove from Shelf</span>
						</button>
					</div>
				</div>
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
		onApplied={async () => fetchShelfBooks()}
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

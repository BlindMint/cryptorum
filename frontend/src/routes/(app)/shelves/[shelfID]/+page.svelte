<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import BulkMetadataReviewModal from '$lib/components/BulkMetadataReviewModal.svelte';
	import BulkMetadataEditModal from '$lib/components/BulkMetadataEditModal.svelte';
	import BulkMetadataLookupConfirmModal from '$lib/components/BulkMetadataLookupConfirmModal.svelte';
	import { appActivity, showFormatOnCover, getFormatColor } from '$lib/stores';
	import {
		getBookDetailContextUrl,
		getInlineMetadataEditUrl,
		startMetadataEditContextSession,
		startMetadataEditSession
	} from '$lib/utils/metadata-edit-session';
	import { confirmBulkAction } from '$lib/utils/bulk-confirm';
	import { restoreRouteScrollPosition, saveRouteScrollPosition } from '$lib/utils/scroll-position';

	let shelf = $state<any>(null);
	let books = $state<any[]>([]);
	let loading = $state(true);
	let formatOnCover = $state(true);
	let selectedBooks = $state<Set<number>>(new Set());
	let showBulkMetadataEdit = $state(false);
	let bulkMetadataEditBookIds = $state<number[]>([]);
	let showMetadataMenu = $state(false);
	let showBulkMetadataLookupConfirm = $state(false);
	let metadataLookupQueueing = $state(false);
	let metadataLookupJob = $state<any | null>(null);
	let showBulkMetadataReview = $state(false);
	let actionInProgress = $state(false);
	let longPressTimer: number | null = null;
	let longPressThreshold = 500;
	let suppressNextClickBookId: number | null = null;
	let longPressTouchStart: { x: number; y: number } | null = null;
	const LONG_PRESS_MOVE_TOLERANCE = 10;
	let bulkSelectionAnchorId = $state<number | null>(null);

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
			restoreRouteScrollPosition();
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

	function getVisibleBookIds(): number[] {
		return books.map((book) => book.id);
	}

	function openBookDetailFromList(event: MouseEvent, bookId: number) {
		saveRouteScrollPosition();
		if (bulkSelectMode || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) return;
		event.preventDefault();
		const visibleIds = getVisibleBookIds();
		const session = startMetadataEditContextSession(visibleIds, `${$page.url.pathname}${$page.url.search}`, {
			totalCount: visibleIds.length
		});
		if (!session) {
			goto(`/book/${bookId}`);
			return;
		}
		goto(getBookDetailContextUrl(bookId, session, Math.max(0, visibleIds.indexOf(bookId))));
	}

	function toggleBookSelection(bookId: number, event?: MouseEvent) {
		if (event) {
			event.preventDefault();
			event.stopPropagation();
		}
		const visibleIds = getVisibleBookIds();
		if (event?.shiftKey && bulkSelectionAnchorId !== null) {
			const anchorIndex = visibleIds.indexOf(bulkSelectionAnchorId);
			const targetIndex = visibleIds.indexOf(bookId);
			if (anchorIndex !== -1 && targetIndex !== -1) {
				const next = new Set(selectedBooks);
				const shouldSelect = !selectedBooks.has(bookId);
				const [start, end] = anchorIndex < targetIndex ? [anchorIndex, targetIndex] : [targetIndex, anchorIndex];
				for (const id of visibleIds.slice(start, end + 1)) {
					if (shouldSelect) next.add(id);
					else next.delete(id);
				}
				selectedBooks = next;
				return;
			}
		}
		const next = new Set(selectedBooks);
		if (next.has(bookId)) {
			next.delete(bookId);
		} else {
			next.add(bookId);
		}
		selectedBooks = next;
		bulkSelectionAnchorId = bookId;
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
			toggleBookSelection(bookId, event);
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

	function clearLongPressTimer() {
		if (longPressTimer) {
			clearTimeout(longPressTimer);
			longPressTimer = null;
		}
	}

	function handleMouseUp() {
		clearLongPressTimer();
	}

	function handleTouchStart(event: TouchEvent) {
		const bookId = Number((event.currentTarget as HTMLElement).dataset.bookId);
		const touch = event.touches[0];
		longPressTouchStart = touch ? { x: touch.clientX, y: touch.clientY } : null;
		longPressTimer = window.setTimeout(() => {
			suppressNextClickBookId = bookId;
			toggleBookSelection(bookId);
			longPressTimer = null;
		}, longPressThreshold);
	}

	function handleTouchMove(event: TouchEvent) {
		if (!longPressTimer || !longPressTouchStart) return;
		const touch = event.touches[0];
		if (!touch) return;
		const deltaX = Math.abs(touch.clientX - longPressTouchStart.x);
		const deltaY = Math.abs(touch.clientY - longPressTouchStart.y);
		if (deltaX > LONG_PRESS_MOVE_TOLERANCE || deltaY > LONG_PRESS_MOVE_TOLERANCE) {
			clearLongPressTimer();
			longPressTouchStart = null;
		}
	}

	function handleTouchEnd() {
		clearLongPressTimer();
		longPressTouchStart = null;
	}

	function selectAllPage() {
		selectedBooks = new Set(books.map((book) => book.id));
	}

	function deselectAll() {
		selectedBooks = new Set();
		bulkSelectionAnchorId = null;
	}

	async function queueBulkMetadataLookup() {
		if (selectedBooks.size === 0 || metadataLookupQueueing) return;
		if (!confirmBulkAction({ action: 'queue metadata lookup for', count: selectedBooks.size })) return;
		showMetadataMenu = false;
		showBulkMetadataLookupConfirm = false;
		metadataLookupQueueing = true;
		const selectedCount = selectedBooks.size;
		const pendingJob = appActivity.startPendingJob({
			job_type: 'metadata_lookup',
			title: `Bulk metadata lookup (${selectedCount} books)`,
			total_items: selectedCount
		});
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
			appActivity.confirmPendingJob(pendingJob, metadataLookupJob);
			await appActivity.refresh();
			showBulkMetadataReview = true;
		} catch (error) {
			console.error('Failed to queue metadata lookup:', error);
			appActivity.failPendingJob(pendingJob, 'Unable to queue metadata lookup.');
		} finally {
			metadataLookupQueueing = false;
		}
	}

	function openSequentialMetadataEdit() {
		if (selectedBooks.size === 0) return;
		showMetadataMenu = false;
		const ids = Array.from(selectedBooks);
		const session = startMetadataEditSession(ids, `${$page.url.pathname}${$page.url.search}`);
		if (!session) return;
		goto(getInlineMetadataEditUrl(ids[0], session, 0));
	}

	function openBulkMetadataEdit() {
		if (selectedBooks.size === 0) return;
		bulkMetadataEditBookIds = Array.from(selectedBooks);
		showMetadataMenu = false;
		showBulkMetadataEdit = true;
	}

	function openBulkMetadataLookupConfirm() {
		if (selectedBooks.size === 0) return;
		showMetadataMenu = false;
		showBulkMetadataLookupConfirm = true;
	}

	async function removeSelectedFromShelf() {
		if (selectedBooks.size === 0 || !shelf) return;
		if (!confirmBulkAction({ action: 'remove {count} books from this shelf', count: selectedBooks.size, destructive: true, alwaysConfirm: true })) return;

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
						onmouseleave={handleMouseUp}
						ontouchstart={handleTouchStart}
						ontouchmove={handleTouchMove}
						ontouchend={handleTouchEnd}
						ontouchcancel={handleTouchEnd}
						onkeydown={handleBookKeydown}
						role="button"
						tabindex="0"
					>
						<a href="/book/{book.id}" class="block" onclick={(event) => openBookDetailFromList(event, book.id)}>
							<div class="relative mb-2 overflow-hidden rounded-lg">
								<BookCoverFrame
									src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
									alt={book.title}
									format={book.format}
									mode="cover"
									frameClass="aspect-[2/3]"
									imageClass="group-hover:scale-105 transition-transform"
									placeholderSize="md"
								/>
								{#if formatOnCover && book.format}
									{@const formatColor = getFormatColor(book.format)}
									<div
										class="absolute right-2 top-2 z-10 px-1.5 py-0.5 rounded text-[10px] font-medium uppercase border border-black/20 shadow-[0_1px_2px_rgba(0,0,0,0.35)]"
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
								class="px-3 py-1.5 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]"
							>
								Select All on Page
							</button>
							<button
								onclick={deselectAll}
								class="px-3 py-1.5 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]"
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
								class="px-4 py-2 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] font-medium transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none flex items-center gap-2"
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
										onclick={openSequentialMetadataEdit}
									>
										<div class="font-medium">Edit selected one by one</div>
										<div class="mt-0.5 text-xs text-[var(--color-surface-text-muted)]">Open the full editor and move through this selection.</div>
									</button>
									<button
										type="button"
										class="block w-full border-t border-[var(--color-surface-border)] px-4 py-3 text-left text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
										onclick={openBulkMetadataEdit}
									>
										<div class="font-medium">Edit metadata in bulk</div>
										<div class="mt-0.5 text-xs text-[var(--color-surface-text-muted)]">Apply shared metadata changes to this selection.</div>
									</button>
									<button
										type="button"
										class="block w-full border-t border-[var(--color-surface-border)] px-4 py-3 text-left text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
										onclick={openBulkMetadataLookupConfirm}
									>
										<div class="font-medium">Bulk metadata lookup</div>
										<div class="mt-0.5 text-xs text-[var(--color-surface-text-muted)]">Find top matches, then review before applying.</div>
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

{#if showBulkMetadataLookupConfirm}
	<BulkMetadataLookupConfirmModal
		count={selectedBooks.size}
		queueing={metadataLookupQueueing}
		onCancel={() => showBulkMetadataLookupConfirm = false}
		onProceed={queueBulkMetadataLookup}
	/>
{/if}

{#if showBulkMetadataEdit}
	<BulkMetadataEditModal
		bookIds={bulkMetadataEditBookIds}
		onClose={() => { showBulkMetadataEdit = false; bulkMetadataEditBookIds = []; }}
		onSaved={() => fetchShelfBooks()}
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

<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import BookCoverProgressChip from '$lib/components/BookCoverProgressChip.svelte';
	import BulkMetadataReviewModal from '$lib/components/BulkMetadataReviewModal.svelte';
	import BulkMetadataEditModal from '$lib/components/BulkMetadataEditModal.svelte';
	import ShelfModal from '$lib/components/ShelfModal.svelte';
	import ShelfBookPickerModal from '$lib/components/ShelfBookPickerModal.svelte';
	import { gridScale, showFormatOnCover, showProgressChipOnCover, getFormatColor } from '$lib/stores';
	import { getCoverThumbUrl, getLibraryCoverThumbSize } from '$lib/utils/covers';
	import { bookHasCoverProgress } from '$lib/utils/cover-progress';
	import { getBookReaderHref } from '$lib/utils/book-formats';
	import {
		DEFAULT_GRID_SCALE,
		getResponsiveGridLayout,
		getResponsiveGridStyle,
		GRID_SCALE_STEP,
		MAX_GRID_SCALE,
		MIN_GRID_SCALE
	} from '$lib/utils/responsive-grid';
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
	let progressChipOnCover = $state(true);
	let selectedBooks = $state<Set<number>>(new Set());
	let showBulkMetadataEdit = $state(false);
	let bulkMetadataEditBookIds = $state<number[]>([]);
	let bulkMetadataLookupBookIds = $state<number[]>([]);
	let showMetadataMenu = $state(false);
	let metadataLookupJob = $state<any | null>(null);
	let showBulkMetadataReview = $state(false);
	let showEditShelfModal = $state(false);
	let showAddBooksModal = $state(false);
	let actionInProgress = $state(false);
	let deletingShelf = $state(false);
	let shelfSearch = $state('');
	let viewMode = $state<'grid' | 'list'>('grid');
	let localGridScale = $state(DEFAULT_GRID_SCALE);
	let shelfSortBy = $state<'title' | 'authors' | 'series' | 'added_at' | 'last_read' | 'status' | 'format'>('title');
	let shelfSortDir = $state<'asc' | 'desc'>('asc');
	let showSettingsMenu = $state(false);
	let showSortMenu = $state(false);
	let shelfGridContainer = $state<HTMLDivElement | null>(null);
	let shelfGridWidth = $state(0);
	let longPressTimer: number | null = null;
	let longPressThreshold = 500;
	let activeShelfId = $state('');
	let shelfRequestToken = 0;
	let suppressNextClickBookId: number | null = null;
	let longPressTouchStart: { x: number; y: number } | null = null;
	const LONG_PRESS_MOVE_TOLERANCE = 10;
	const SHELF_SUMMARY_CACHE_KEY = 'cryptorumShelfSummaries';
	let bulkSelectionAnchorId = $state<number | null>(null);

	let bulkSelectMode = $derived(selectedBooks.size > 0);
	let visibleBooks = $derived(getVisibleBooks());
	let existingShelfBookIds = $derived(new Set(books.map((book) => Number(book.id))));
	let estimatedGridWidth = $derived(shelfGridWidth > 0 ? shelfGridWidth : typeof window === 'undefined' ? 1920 : window.innerWidth);
	let responsiveGridLayout = $derived(getResponsiveGridLayout(estimatedGridWidth, localGridScale));
	let responsiveGridColumns = $derived(responsiveGridLayout.columns);
	let gridStyle = $derived(viewMode === 'grid' ? getResponsiveGridStyle(responsiveGridLayout) : '');
	let shelfCoverThumbSize = $derived(getLibraryCoverThumbSize(responsiveGridColumns));
	const shelfSortOptions: Array<{ value: typeof shelfSortBy; label: string }> = [
		{ value: 'title', label: 'Title' },
		{ value: 'authors', label: 'Author' },
		{ value: 'series', label: 'Series' },
		{ value: 'added_at', label: 'Added' },
		{ value: 'last_read', label: 'Last Read' },
		{ value: 'status', label: 'Status' },
		{ value: 'format', label: 'Format' }
	];

	$effect(() => {
		const unsub = showFormatOnCover.subscribe((value: boolean) => formatOnCover = value);
		return unsub;
	});

	$effect(() => {
		const unsub = showProgressChipOnCover.subscribe((value: boolean) => progressChipOnCover = value);
		return unsub;
	});

	$effect(() => {
		const unsub = gridScale.subscribe((value: number) => localGridScale = value);
		return unsub;
	});

	$effect(() => {
		const element = shelfGridContainer;
		if (!element || typeof ResizeObserver === 'undefined') return;

		const updateWidth = () => {
			shelfGridWidth = element.clientWidth;
		};
		const observer = new ResizeObserver(updateWidth);
		updateWidth();
		observer.observe(element);
		return () => observer.disconnect();
	});

	onMount(async () => {
		gridScale.init();
		showFormatOnCover.init();
		showProgressChipOnCover.init();
	});

	function updateGridScale(value: number) {
		gridScale.set(value);
	}

	$effect(() => {
		const shelfId = $page.params.shelfID;
		if (!shelfId || shelfId === activeShelfId) return;

		activeShelfId = shelfId;
		resetShelfRouteState();
		if (!shelf) {
			seedShelfFromCache(shelfId);
		}
		void fetchShelfBooks(shelfId);
	});

	function resetShelfRouteState() {
		clearLongPressTimer();
		selectedBooks = new Set();
		bulkSelectionAnchorId = null;
		shelfSearch = '';
		showMetadataMenu = false;
		showBulkMetadataEdit = false;
		showBulkMetadataReview = false;
		showEditShelfModal = false;
		showAddBooksModal = false;
		showSettingsMenu = false;
		showSortMenu = false;
		bulkMetadataEditBookIds = [];
		bulkMetadataLookupBookIds = [];
		metadataLookupJob = null;
		deletingShelf = false;
		actionInProgress = false;
		suppressNextClickBookId = null;
		longPressTouchStart = null;
	}

	async function fetchShelfBooks(shelfId = $page.params.shelfID) {
		const requestToken = ++shelfRequestToken;
		loading = true;
		try {
			const [shelfRes, booksRes] = await Promise.all([
				fetch(`/api/shelves/${shelfId}`),
				fetch(`/api/shelves/${shelfId}/books`)
			]);

			const nextShelf = shelfRes.ok ? await shelfRes.json() : null;
			const nextBooks = booksRes.ok ? await booksRes.json() : [];

			if (requestToken !== shelfRequestToken) return;

			shelf = nextShelf;
			books = nextBooks;
			if (nextShelf) {
				cacheShelfSummary(nextShelf);
			}
			applyShelfSortDefaults(nextShelf);
		} catch (error) {
			if (requestToken !== shelfRequestToken) return;
			console.error('Failed to fetch shelf:', error);
			shelf = null;
			books = [];
		} finally {
			if (requestToken === shelfRequestToken) {
				loading = false;
				restoreRouteScrollPosition();
			}
		}
	}

	function seedShelfFromCache(shelfId: string) {
		try {
			const cached = JSON.parse(sessionStorage.getItem(SHELF_SUMMARY_CACHE_KEY) || '[]') as any[];
			const cachedShelf = Array.isArray(cached) ? cached.find((item) => String(item.id) === shelfId) : null;
			if (cachedShelf) {
				shelf = cachedShelf;
			}
		} catch {
			// Ignore cache failures; fetchShelfBooks will load the shelf.
		}
	}

	function cacheShelfSummary(nextShelf: any) {
		try {
			const cached = JSON.parse(sessionStorage.getItem(SHELF_SUMMARY_CACHE_KEY) || '[]') as any[];
			const shelves = Array.isArray(cached) ? cached.filter((item) => String(item.id) !== String(nextShelf.id)) : [];
			sessionStorage.setItem(SHELF_SUMMARY_CACHE_KEY, JSON.stringify([...shelves, nextShelf]));
		} catch {
			// Ignore storage failures; this is only a render cache.
		}
	}

	function applyShelfSortDefaults(nextShelf: any) {
		const allowedSorts = new Set(['title', 'authors', 'series', 'added_at', 'last_read', 'status', 'format']);
		const nextSortBy = String(nextShelf?.sort_by || 'title');
		shelfSortBy = allowedSorts.has(nextSortBy) ? nextSortBy as typeof shelfSortBy : 'title';
		shelfSortDir = nextShelf?.sort_dir === 'desc' ? 'desc' : 'asc';
	}

	function parseAuthors(authorsJson: string): string {
		try {
			const arr = JSON.parse(authorsJson);
			return Array.isArray(arr) ? arr.join(', ') : authorsJson;
		} catch {
			return authorsJson;
		}
	}

	function getVisibleBooks() {
		const query = shelfSearch.trim().toLowerCase();
		const filtered = !query ? books : books.filter((book) => {
			const text = [
				book.title || '',
				parseAuthors(book.authors || ''),
				book.format || '',
				book.status || ''
			].join(' ').toLowerCase();
			return query.split(/\s+/).filter(Boolean).every((token) => text.includes(token));
		});
		return [...filtered].sort((a, b) => compareShelfBooks(a, b));
	}

	function compareShelfBooks(a: any, b: any): number {
		let comparison = 0;
		if (shelfSortBy === 'authors') {
			comparison = parseAuthors(a.authors || '').localeCompare(parseAuthors(b.authors || ''), undefined, { sensitivity: 'base' });
		} else if (shelfSortBy === 'series') {
			comparison = String(a.series || '').localeCompare(String(b.series || ''), undefined, { numeric: true, sensitivity: 'base' });
			if (comparison === 0) {
				comparison = Number(a.series_number || 0) - Number(b.series_number || 0);
			}
		} else if (shelfSortBy === 'added_at') {
			comparison = Number(a.added_at || 0) - Number(b.added_at || 0);
		} else if (shelfSortBy === 'last_read') {
			comparison = Number(a.last_read_at || 0) - Number(b.last_read_at || 0);
		} else if (shelfSortBy === 'status') {
			comparison = String(a.status || '').localeCompare(String(b.status || ''), undefined, { sensitivity: 'base' });
		} else if (shelfSortBy === 'format') {
			comparison = String(a.format || '').localeCompare(String(b.format || ''), undefined, { sensitivity: 'base' });
		} else {
			comparison = String(a.title || '').localeCompare(String(b.title || ''), undefined, { sensitivity: 'base' });
		}
		if (comparison === 0) {
			comparison = String(a.title || '').localeCompare(String(b.title || ''), undefined, { sensitivity: 'base' });
		}
		return shelfSortDir === 'desc' ? -comparison : comparison;
	}

	function shelfSortLabel() {
		switch (shelfSortBy) {
			case 'authors':
				return 'Author';
			case 'series':
				return 'Series';
			case 'added_at':
				return 'Added';
			case 'last_read':
				return 'Last Read';
			case 'status':
				return 'Status';
			case 'format':
				return 'Format';
			default:
				return 'Title';
		}
	}

	function toggleSortDirection() {
		shelfSortDir = shelfSortDir === 'asc' ? 'desc' : 'asc';
	}

	function setShelfSort(value: typeof shelfSortBy) {
		shelfSortBy = value;
		showSortMenu = false;
	}

	function sortDirectionLabel() {
		return shelfSortDir === 'asc' ? 'Ascending' : 'Descending';
	}

	function sortDirectionArrow() {
		return shelfSortDir === 'asc' ? '↑' : '↓';
	}

	function toggleFormatOnCover() {
		formatOnCover = !formatOnCover;
		showFormatOnCover.set(formatOnCover);
	}

	function toggleProgressChipOnCover() {
		progressChipOnCover = !progressChipOnCover;
		showProgressChipOnCover.set(progressChipOnCover);
	}

	function getReaderUrl(book: any): string {
		const shelfPath = shelf?.id ? `/shelves/${shelf.id}` : '/shelves';
		return getBookReaderHref(book.id, getBookFormat(book), shelfPath, book.resume_file_id);
	}

	async function deleteShelf() {
		if (!shelf?.id || deletingShelf) return;
		if (!confirm(`Delete shelf "${shelf.name}"? Books will remain in your libraries.`)) return;

		deletingShelf = true;
		try {
			const res = await fetch(`/api/shelves/${shelf.id}`, { method: 'DELETE' });
			if (res.ok) {
				await (window as any).refreshSidebar?.();
				goto('/shelves');
			} else {
				console.error('Failed to delete shelf');
			}
		} catch (error) {
			console.error('Failed to delete shelf:', error);
		} finally {
			deletingShelf = false;
		}
	}

	function getStatusLabel(status: string): string {
		switch (status) {
			case 'reading':
				return 'Currently Reading';
			case 'finished':
				return 'Already Read';
			default:
				return 'Unread';
		}
	}

	function getBookDetailHref(bookId: number): string {
		return `/book/${bookId}`;
	}

	function getBookCoverSrc(book: any): string | null {
		return book.cover_path ? getCoverThumbUrl(book.id, shelfCoverThumbSize, book.cover_updated_on) : null;
	}

	function getBookAuthors(book: any): string {
		return book.authors && book.authors !== '[]' ? parseAuthors(book.authors) : '';
	}

	function getBookTitle(book: any): string {
		return book.title || 'Untitled';
	}

	function getBookFormat(book: any): string {
		return book.format || '';
	}

	function getShelfTypeLabel(): string {
		return shelf?.is_magic === 1 ? 'Magic' : 'Manual';
	}

	function getShelfTypeClass(): string {
		return shelf?.is_magic === 1 ? 'text-purple-300' : 'text-[var(--color-primary-400)]';
	}

	function clearShelfSearch() {
		shelfSearch = '';
	}

	function getSearchCountLabel(): string {
		if (!shelfSearch.trim()) return `${books.length} books`;
		return `${visibleBooks.length} of ${books.length} books`;
	}

	function getVisibleBookIds(): number[] {
		return visibleBooks.map((book) => book.id);
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
		selectedBooks = new Set(visibleBooks.map((book) => book.id));
	}

	function deselectAll() {
		selectedBooks = new Set();
		bulkSelectionAnchorId = null;
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

	function openBulkMetadataLookup() {
		if (selectedBooks.size === 0) return;
		showMetadataMenu = false;
		metadataLookupJob = null;
		bulkMetadataLookupBookIds = Array.from(selectedBooks);
		showBulkMetadataReview = true;
	}

	async function removeSelectedFromShelf() {
		if (selectedBooks.size === 0 || !shelf) return;
		if (shelf.is_magic === 1) return;
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

	async function handleShelfSaved() {
		showEditShelfModal = false;
		await fetchShelfBooks();
		await (window as any).refreshSidebar?.();
	}

	async function handleBooksAdded() {
		showAddBooksModal = false;
		await fetchShelfBooks();
		await (window as any).refreshSidebar?.();
	}
</script>

<div class="catalog-page-shell space-y-6">
	{#if shelf}
		<div class="catalog-page-header">
			<div class="catalog-page-header-row">
				<div class="catalog-page-title-row">
					<h1 class="catalog-page-title break-words">{shelf.name}</h1>
					<p class="catalog-page-count">{getSearchCountLabel()}</p>
					<span class="inline-flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide {getShelfTypeClass()}">
						{#if shelf.is_magic === 1}
							<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"></path>
							</svg>
						{/if}
						<span>{getShelfTypeLabel()}</span>
					</span>
				</div>
				<div class="catalog-page-actions">
					{#if shelf.is_magic !== 1}
						<button
							type="button"
							onclick={() => showAddBooksModal = true}
							class="accent-action catalog-page-action"
						>
							<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v12m6-6H6"></path>
							</svg>
							Add Books
						</button>
					{/if}
					<button
						type="button"
						onclick={() => showEditShelfModal = true}
						class="catalog-page-action catalog-page-action-secondary"
					>
						<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m16.862 4.487 1.687-1.688a1.875 1.875 0 1 1 2.652 2.652L10.582 16.07a4.5 4.5 0 0 1-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 0 1 1.13-1.897l8.932-8.931Z"></path>
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19.5 7.125 16.875 4.5"></path>
						</svg>
						Edit Shelf
					</button>
					<button
						type="button"
						onclick={deleteShelf}
						disabled={deletingShelf}
						class="catalog-page-action border border-red-500/35 bg-red-500/10 text-red-300 hover:border-red-400/60 hover:bg-red-500/15 disabled:opacity-50"
					>
						<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 7h12m-9 4v6m6-6v6M9 7V4h6v3m-9 0 1 13h10l1-13"></path>
						</svg>
						{deletingShelf ? 'Deleting...' : 'Delete'}
					</button>
				</div>
			</div>

			<div class="grid min-w-0 grid-cols-[minmax(0,1fr)_2.5rem_auto] items-center gap-2">
				<div class="relative min-w-0">
					<svg class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-4.35-4.35M10.5 18a7.5 7.5 0 110-15 7.5 7.5 0 010 15z"></path>
					</svg>
					<input
						type="search"
						bind:value={shelfSearch}
						placeholder="Search this shelf"
						class="h-10 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-2 pl-9 pr-9 text-sm text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
					>
					{#if shelfSearch}
						<button
							type="button"
							onclick={clearShelfSearch}
							class="absolute right-2 top-1/2 inline-flex h-6 w-6 -translate-y-1/2 items-center justify-center rounded text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
							aria-label="Clear shelf search"
						>
							<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
								<path d="M18 6 6 18"></path>
								<path d="m6 6 12 12"></path>
							</svg>
						</button>
					{/if}
				</div>

			<div class="relative">
				{#if showSettingsMenu}
					<button
						type="button"
						class="fixed inset-0 z-20"
						aria-label="Close shelf settings menu"
						onclick={() => showSettingsMenu = false}
					></button>
				{/if}
				<button
					type="button"
					onclick={() => showSettingsMenu = !showSettingsMenu}
					aria-label="Shelf settings"
					class="inline-flex h-10 w-10 items-center justify-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-overlay)]"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 0 0 2.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 0 0 1.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 0 0-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 0 0-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 0 0-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 0 0-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 0 0 1.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
					</svg>
				</button>
				{#if showSettingsMenu}
					<div class="absolute right-0 top-full z-40 mt-2 w-56 max-w-[calc(100vw-1.5rem)] overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-3 shadow-lg">
						<div class="px-4 pb-3 border-b border-[var(--color-surface-border)]">
							<div class="mb-2 text-sm font-medium text-[var(--color-surface-text)]">View</div>
							<div class="grid grid-cols-2 overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-1">
								<button
									type="button"
									onclick={() => viewMode = 'grid'}
									class="rounded-md px-2 py-1.5 text-sm font-medium transition-colors {viewMode === 'grid' ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-400)] ring-1 ring-inset ring-[var(--color-primary-500)]/45' : 'text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]'}"
									aria-pressed={viewMode === 'grid'}
								>
									Grid
								</button>
								<button
									type="button"
									onclick={() => viewMode = 'list'}
									class="rounded-md px-2 py-1.5 text-sm font-medium transition-colors {viewMode === 'list' ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-400)] ring-1 ring-inset ring-[var(--color-primary-500)]/45' : 'text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]'}"
									aria-pressed={viewMode === 'list'}
								>
									List
								</button>
							</div>
						</div>
						{#if viewMode === 'grid'}
							<div class="px-4 py-3 border-b border-[var(--color-surface-border)]">
								<div class="mb-2 flex items-center justify-between gap-3">
									<label class="text-sm font-medium text-[var(--color-surface-text)]" for="shelf-grid-scale">Grid Scale</label>
									<span class="text-xs text-[var(--color-surface-text-muted)]">{responsiveGridColumns} cols</span>
								</div>
								<div class="flex items-center space-x-2">
									<input
										id="shelf-grid-scale"
										type="range"
										min={MIN_GRID_SCALE}
										max={MAX_GRID_SCALE}
										step={GRID_SCALE_STEP}
										value={localGridScale}
										oninput={(event) => updateGridScale(Number(event.currentTarget.value))}
										class="flex-1 h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer slider"
									>
									<span class="text-sm text-[var(--color-surface-text)] w-12 text-right">{localGridScale}%</span>
								</div>
							</div>
						{/if}
						<button
							type="button"
							onclick={toggleFormatOnCover}
							class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)]"
						>
							<span>Show Format on Cover</span>
							<span class="relative h-6 w-10 rounded-full transition-colors {formatOnCover ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-surface-600)]'}">
								<span class="absolute top-1 h-4 w-4 rounded-full bg-white transition-transform {formatOnCover ? 'left-5' : 'left-1'}"></span>
							</span>
						</button>
						<button
							type="button"
							onclick={toggleProgressChipOnCover}
							class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)]"
						>
							<span>Show Progress Chip</span>
							<span class="relative h-6 w-10 rounded-full transition-colors {progressChipOnCover ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-surface-600)]'}">
								<span class="absolute top-1 h-4 w-4 rounded-full bg-white transition-transform {progressChipOnCover ? 'left-5' : 'left-1'}"></span>
							</span>
						</button>
					</div>
				{/if}
			</div>

			<div class="relative">
				{#if showSortMenu}
					<button
						type="button"
						class="fixed inset-0 z-20"
						aria-label="Close shelf sort menu"
						onclick={() => showSortMenu = false}
					></button>
				{/if}
				<div class="inline-flex h-10 overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
					<button
						type="button"
						onclick={() => showSortMenu = !showSortMenu}
						aria-label="Sort shelf books by {shelfSortLabel()}"
						class="inline-flex min-w-0 items-center px-3 text-sm font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-overlay)]"
					>
						<span class="hidden min-w-0 truncate sm:inline">{shelfSortLabel()}</span>
						<svg class="h-4 w-4 sm:ml-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
						</svg>
					</button>
					<button
						type="button"
						onclick={toggleSortDirection}
						class="inline-flex w-10 items-center justify-center border-l border-[var(--color-surface-border)] text-[var(--color-primary-400)] transition-colors hover:bg-[var(--color-surface-overlay)]"
						title={sortDirectionLabel()}
						aria-label={shelfSortDir === 'asc' ? `Sort ${shelfSortLabel()} ascending` : `Sort ${shelfSortLabel()} descending`}
					>
						{sortDirectionArrow()}
					</button>
				</div>
				{#if showSortMenu}
					<div class="absolute right-0 top-full z-40 mt-2 w-48 overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-1 shadow-lg">
						{#each shelfSortOptions as option}
							<button
								type="button"
								onclick={() => setShelfSort(option.value)}
								class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)] {shelfSortBy === option.value ? 'text-[var(--color-primary-400)]' : ''}"
							>
								<span>{option.label}</span>
								{#if shelfSortBy === option.value}
									<span class="text-xs">✓</span>
								{/if}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		</div>
		</div>

		{#if selectedBooks.size > 0}
			<div class="-mt-3 text-sm font-medium text-[var(--color-surface-text-muted)]">{selectedBooks.size} selected</div>
		{/if}

		{#if loading && books.length === 0}
			<div class="flex justify-center py-12">
				<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
			</div>
		{:else if books.length === 0}
			<div class="text-center py-16 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
				<svg class="w-16 h-16 text-[var(--color-surface-text-muted)] mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
				</svg>
				<h3 class="text-lg font-medium text-[var(--color-surface-text)] mb-2">No books on this shelf</h3>
				<p class="text-[var(--color-surface-text-muted)]">
					{shelf.is_magic === 1 ? 'No books currently match these smart rules.' : 'Use Add Books to build this shelf.'}
				</p>
			</div>
		{:else if visibleBooks.length === 0}
			<div class="text-center py-16 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
				<h3 class="text-lg font-medium text-[var(--color-surface-text)] mb-2">No books match your shelf search</h3>
				<p class="text-[var(--color-surface-text-muted)]">Clear the shelf search to see all books on this shelf.</p>
			</div>
		{:else if viewMode === 'list'}
			<div class="space-y-2">
				{#each visibleBooks as book}
					<div
						class="group relative rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] transition-all duration-200 hover:border-[var(--color-primary-500)]/40 hover:bg-[var(--color-surface-700)]/60 {selectedBooks.has(book.id) ? 'ring-2 ring-[var(--color-primary-500)] ring-offset-2 ring-offset-[var(--color-surface-base)]' : ''}"
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
						<a href={getBookDetailHref(book.id)} class="flex items-center gap-4 p-3" onclick={(event) => openBookDetailFromList(event, book.id)}>
							<div class="relative w-14 shrink-0 overflow-hidden rounded-md sm:w-16">
								<BookCoverFrame
									src={getBookCoverSrc(book)}
									alt={getBookTitle(book)}
									format={getBookFormat(book)}
									mode="cover"
									frameClass="aspect-[2/3]"
									imageClass="group-hover:scale-105 transition-transform"
									placeholderSize="sm"
								/>
							</div>
							<div class="min-w-0 flex-1">
								<h3 class="truncate text-sm font-semibold text-[var(--color-surface-text)] sm:text-base">{getBookTitle(book)}</h3>
								{#if getBookAuthors(book)}
									<p class="truncate text-sm text-[var(--color-surface-text-muted)]">{getBookAuthors(book)}</p>
								{/if}
								<div class="mt-2 flex flex-wrap items-center gap-2">
									<span class="rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-2 py-0.5 text-xs text-[var(--color-surface-text-muted)]">
										{getStatusLabel(book.status)}
									</span>
									{#if getBookFormat(book)}
										{@const formatColor = getFormatColor(getBookFormat(book))}
										<span
											class="rounded-md border border-black/20 px-2 py-0.5 text-xs font-medium uppercase"
											style="background-color: {formatColor.bg}; color: {formatColor.text};"
										>
											{getBookFormat(book)}
										</span>
									{/if}
								</div>
							</div>
						</a>
						<button
							type="button"
							onclick={(event) => toggleBookSelection(book.id, event)}
							class="absolute right-3 top-3 z-20 flex h-7 w-7 items-center justify-center rounded border-2 transition-all {selectedBooks.has(book.id) ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]' : 'border-[var(--color-surface-400)] bg-[var(--color-surface-800)]/90 opacity-0 group-hover:opacity-100'} {bulkSelectMode ? 'opacity-100' : ''}"
							aria-label={selectedBooks.has(book.id) ? 'Deselect book' : 'Select book'}
						>
							{#if selectedBooks.has(book.id)}
								<svg class="h-4 w-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7"></path>
								</svg>
							{/if}
						</button>
					</div>
				{/each}
			</div>
		{:else}
			<div bind:this={shelfGridContainer} class="grid" style={gridStyle}>
				{#each visibleBooks as book}
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
						<div class="block">
							<div class="relative mb-2 overflow-hidden rounded-lg">
								<a href={getBookDetailHref(book.id)} class="block" onclick={(event) => openBookDetailFromList(event, book.id)} aria-label="View details for {getBookTitle(book)}">
									<BookCoverFrame
										src={getBookCoverSrc(book)}
										alt={getBookTitle(book)}
										format={getBookFormat(book)}
										mode="cover"
										frameClass="aspect-[2/3]"
										imageClass="group-hover:scale-105 transition-transform"
										placeholderSize="md"
									/>
									{#if bookHasCoverProgress(book)}
										<div class="absolute bottom-0 left-0 right-0 z-10 h-1 bg-slate-700">
											<div class="h-full bg-[var(--color-primary-500)] transition-all duration-300" style="width: {book.percent}%"></div>
										</div>
									{/if}
									{#if formatOnCover && getBookFormat(book)}
										{@const formatColor = getFormatColor(getBookFormat(book))}
										<div
											class="absolute right-2 top-2 z-10 px-1.5 py-0.5 rounded text-[10px] font-medium uppercase border border-black/20 shadow-[0_1px_2px_rgba(0,0,0,0.35)]"
											style="background-color: {formatColor.bg}; color: {formatColor.text};"
										>
											{getBookFormat(book)}
										</div>
									{/if}
									{#if book.status === 'reading' || book.status === 'finished'}
										<span class="absolute bottom-2 right-2 z-20 h-3 w-3 rounded-full border border-black/30 {book.status === 'reading' ? 'bg-blue-500' : 'bg-emerald-500'} shadow-[0_1px_2px_rgba(0,0,0,0.35)]"></span>
									{/if}
								</a>
								{#if progressChipOnCover && bookHasCoverProgress(book)}
									<BookCoverProgressChip
										href={getReaderUrl(book)}
										percent={book.percent}
										title={getBookTitle(book)}
										format={getBookFormat(book)}
										disabled={bulkSelectMode}
									/>
								{/if}
							</div>
							<a href={getBookDetailHref(book.id)} class="block" onclick={(event) => openBookDetailFromList(event, book.id)}>
								<h3 class="text-sm font-medium text-[var(--color-surface-text)] truncate">{getBookTitle(book)}</h3>
								{#if getBookAuthors(book)}
									<p class="text-xs text-[var(--color-surface-text-muted)] truncate">{getBookAuthors(book)}</p>
								{/if}
							</a>
						</div>
						<button
							type="button"
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
		<div class="text-center py-16 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
			<p class="text-[var(--color-surface-text-muted)]">Shelf not found</p>
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
								disabled={selectedBooks.size === 0}
								class="px-4 py-2 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] font-medium transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none flex items-center gap-2"
							>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
								</svg>
								<span>Metadata</span>
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
										onclick={openBulkMetadataLookup}
									>
										<div class="font-medium">Bulk metadata lookup</div>
										<div class="mt-0.5 text-xs text-[var(--color-surface-text-muted)]">Find top matches, then review before applying.</div>
									</button>
								</div>
							{/if}
						</div>
						{#if shelf?.is_magic !== 1}
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
						{/if}
					</div>
				</div>
			</div>
		</div>
	</div>
{:else if loading}
	<div class="flex justify-center py-12">
		<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
	</div>
{/if}

{#if showBulkMetadataEdit}
	<BulkMetadataEditModal
		bookIds={bulkMetadataEditBookIds}
		onClose={() => { showBulkMetadataEdit = false; bulkMetadataEditBookIds = []; }}
		onSaved={() => fetchShelfBooks()}
	/>
{/if}

{#if showBulkMetadataReview && (metadataLookupJob?.id || bulkMetadataLookupBookIds.length > 0)}
	<BulkMetadataReviewModal
		jobId={metadataLookupJob?.id ?? null}
		bookIds={bulkMetadataLookupBookIds}
		initialJob={metadataLookupJob}
		onClose={() => { showBulkMetadataReview = false; bulkMetadataLookupBookIds = []; metadataLookupJob = null; }}
		onApplied={async () => fetchShelfBooks()}
	/>
{/if}

<ShelfModal
	open={showEditShelfModal}
	mode="edit"
	shelf={shelf}
	onClose={() => showEditShelfModal = false}
	onSaved={handleShelfSaved}
/>

<ShelfBookPickerModal
	open={showAddBooksModal}
	shelf={shelf}
	existingBookIds={existingShelfBookIds}
	onClose={() => showAddBooksModal = false}
	onAdded={handleBooksAdded}
/>

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

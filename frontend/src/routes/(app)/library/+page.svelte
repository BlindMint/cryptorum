<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { beforeNavigate, goto } from '$app/navigation';
	import { appActivity, filterPanelWidth, gridScale, showFormatOnCover, getFormatColor } from '$lib/stores';
	import { confirmBulkAction } from '$lib/utils/bulk-confirm';
	import { getCoverThumbUrl, getLibraryCoverThumbSize } from '$lib/utils/covers';
	import { getBookReaderHref, isAudioFormat } from '$lib/utils/book-formats';
	import { DEFAULT_GRID_SCALE, getResponsiveGridLayout, getResponsiveGridStyle, GRID_SCALE_STEP, MAX_GRID_SCALE, MIN_GRID_SCALE } from '$lib/utils/responsive-grid';
	import {
		getBookDetailContextUrl,
		getInlineMetadataEditUrl,
		startMetadataEditContextSession,
		startMetadataEditSession
	} from '$lib/utils/metadata-edit-session';
	import { restoreRouteScrollPosition, saveRouteScrollPosition } from '$lib/utils/scroll-position';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import FilterPanel from '$lib/components/FilterPanel.svelte';
	import BulkMetadataReviewModal from '$lib/components/BulkMetadataReviewModal.svelte';
	import BulkMetadataEditModal from '$lib/components/BulkMetadataEditModal.svelte';
	import ShelfPickerRow from '$lib/components/ShelfPickerRow.svelte';
	import ShelfModal from '$lib/components/ShelfModal.svelte';
	import LibraryModal from '$lib/components/LibraryModal.svelte';

		type FilterMode = 'AND' | 'OR' | 'NOT';
		const COMPACT_TOOLBAR_WIDTH = 720;

		let books = $state<any[]>([]);
  let loading = $state(true);
  let loadingMore = $state(false);
  let scanning = $state(false);
  let scanMessage = $state('');
  let libraryName = $state('');
  let totalBooks = $state(0);
  let serverHasMore = $state(false);
	let hasMore = $derived(serverHasMore);

	// Display controls
	let viewMode = $state('grid');
	let localGridScale = $state(DEFAULT_GRID_SCALE);
	let sortBy = $state(getSortByForUrl($page.url));
	let sortDir = $state<'asc' | 'desc'>(getSortDirForUrl($page.url));
	let estimatedGridWidth = $derived(typeof window === 'undefined' ? 1920 : window.innerWidth);
	let responsiveGridLayout = $derived(getResponsiveGridLayout(estimatedGridWidth, localGridScale));
	let responsiveGridColumns = $derived(responsiveGridLayout.columns);
	let gridStyle = $derived(viewMode === 'grid' ? getResponsiveGridStyle(responsiveGridLayout) : '');
		let libraryCoverThumbSize = $derived(getLibraryCoverThumbSize(responsiveGridColumns));
		let showSettingsMenu = $state(false);
		let formatOnCover = $state(true);
		let activeBookActions = $state<number | null>(null);
		let toolbarContainer = $state<HTMLDivElement | null>(null);
		let toolbarWidth = $state(0);
		let compactToolbar = $derived(toolbarWidth > 0 && toolbarWidth < COMPACT_TOOLBAR_WIDTH);

	$effect(() => {
		const unsub = gridScale.subscribe((v: number) => localGridScale = v);
		return unsub;
	});

		$effect(() => {
			const unsub = showFormatOnCover.subscribe((v: boolean) => formatOnCover = v);
			return unsub;
		});

		$effect(() => {
			const element = toolbarContainer;
			if (!element || typeof ResizeObserver === 'undefined') return;

			const updateWidth = () => {
				toolbarWidth = element.clientWidth;
			};
			const observer = new ResizeObserver(updateWidth);
			updateWidth();
			observer.observe(element);
			return () => observer.disconnect();
		});

	function updateGridScale(value: number) {
		gridScale.set(value);
	}

	function toggleFormatOnCover() {
		formatOnCover = !formatOnCover;
		showFormatOnCover.set(formatOnCover);
	}

	// Filter state
	let showFilterPanel = $state(false);
	let filterPanelBackdrop = $state(false);
	let showSortMenu = $state(false);
	let availableAuthors = $state<any[]>([]);
	let availableSeries = $state<any[]>([]);
	let availableTags = $state<any[]>([]);
	let availableFormats = $state<any[]>([]);

	// Bulk selection state
	let selectedBooks = $state<Set<number>>(new Set());
	let showBulkPanel = $state(false);
	let showBulkMetadataEdit = $state(false);
	let bulkMetadataEditBookIds = $state<number[]>([]);
	let bulkMetadataLookupBookIds = $state<number[]>([]);
	let showMetadataMenu = $state(false);
	let metadataLookupJob = $state<any | null>(null);
	let showBulkMetadataReview = $state(false);
	let showShelfPicker = $state(false);
	let showCreateShelfModal = $state(false);
	let showLibraryModal = $state(false);
	let editingLibrary = $state<any | null>(null);
	let shelves = $state<any[]>([]);
	let actionInProgress = $state(false);
	let selectAllMode = $state<'none' | 'page' | 'filtered'>('none');
	let bulkSelectMode = $derived(selectedBooks.size > 0);
	let manualShelves = $derived(shelves.filter((shelf) => shelf.is_magic !== 1));
	let bulkSelectionAnchorId = $state<number | null>(null);
	let bulkSelectionRestored = false;

	// Long press state for mobile
	let longPressTimer: number | null = null;
	let longPressThreshold = 500; // ms
	let longPressActivated = $state(false);
	let suppressNextClickBookId: number | null = null;
	let longPressTouchStart: { x: number; y: number } | null = null;
	const LONG_PRESS_MOVE_TOLERANCE = 10;

	let libraryFilter = $derived($page.url.searchParams.get('library') || '');
	let librarySearch = $state('');
	let currentOffset = $state(0);
	const BATCH_SIZE = 80;
	const BACKGROUND_REFRESH_ACTIVE_INTERVAL_MS = 3000;
	const BACKGROUND_REFRESH_IDLE_INTERVAL_MS = 30000;
	const MAX_REFRESH_LIMIT = 200;
	const BULK_SELECTION_STORAGE_KEY = 'cryptorumLibraryBulkSelection';
	const FILTER_PANEL_PARAM_KEYS = ['author', 'series', 'genre', 'genre_mode', 'tags', 'tag_mode', 'format', 'status', 'filter_mode', 'value_filter_mode'];
	const LATIN_CONFUSABLE_SORT_MAP: Record<string, string> = {
		'Α': 'A', 'А': 'A', 'Β': 'B', 'В': 'B', 'Ε': 'E', 'Е': 'E', 'Ζ': 'Z',
		'Η': 'H', 'Н': 'H', 'Ι': 'I', 'І': 'I', 'Κ': 'K', 'К': 'K', 'Μ': 'M',
		'М': 'M', 'Ν': 'N', 'Ο': 'O', 'О': 'O', 'Ρ': 'P', 'Р': 'P', 'Τ': 'T',
		'Т': 'T', 'Υ': 'Y', 'У': 'Y', 'Χ': 'X', 'Х': 'X', 'Ϲ': 'C', 'С': 'C',
		'α': 'a', 'а': 'a', 'β': 'b', 'в': 'b', 'ε': 'e', 'е': 'e', 'η': 'h',
		'н': 'h', 'ι': 'i', 'і': 'i', 'κ': 'k', 'к': 'k', 'μ': 'm', 'м': 'm',
		'ο': 'o', 'о': 'o', 'ρ': 'p', 'р': 'p', 'τ': 't', 'т': 't', 'υ': 'y',
		'у': 'y', 'χ': 'x', 'х': 'x', 'ϲ': 'c', 'с': 'c'
	};
	const LATIN_CONFUSABLE_SORT_PATTERN = /[ΑАΒВΕЕΖΗНΙІΚКΜМΝΟОΡРΤТΥУΧХϹСαаβвεеηнιіκкμмοоρрτтυуχхϲс]/g;
	let backgroundRefreshing = false;
	let backgroundRefreshTimer: ReturnType<typeof setTimeout> | null = null;
	let searchUpdateTimer: ReturnType<typeof setTimeout> | null = null;
	let activeBulkSelectionScope = '';

	function getQueryValues(params: URLSearchParams, key: string, splitComma: boolean = false): string[] {
		const values = params.getAll(key);
		const source = values.length > 0 ? values : [params.get(key) || ''];
		const cleaned: string[] = [];

		for (const raw of source) {
			if (splitComma) {
				cleaned.push(...raw.split(',').map((value) => value.trim()).filter(Boolean));
			} else {
				const value = raw.trim();
				if (value) cleaned.push(value);
			}
		}

		return Array.from(new Set(cleaned));
	}

	function setQueryValues(params: URLSearchParams, key: string, values: string[]) {
		params.delete(key);
		for (const value of values) {
			const trimmed = value.trim();
			if (trimmed) params.append(key, trimmed);
		}
	}

	function getSortByForUrl(url: URL): string {
		const explicitSort = url.searchParams.get('sort');
		if (explicitSort) return explicitSort;
		return getQueryValues(url.searchParams, 'series').length === 1 ? 'series' : 'title';
	}

	function getSortDirForUrl(url: URL): 'asc' | 'desc' {
		const explicitSortDir = url.searchParams.get('sort_dir');
		if (explicitSortDir) return explicitSortDir === 'desc' ? 'desc' : 'asc';
		return 'asc';
	}

	function getFilterMode(): FilterMode {
		const mode = ($page.url.searchParams.get('filter_mode') || 'AND').toUpperCase();
		return mode === 'OR' || mode === 'NOT' ? mode : 'AND';
	}

	function getValueFilterMode(): FilterMode {
		const mode = ($page.url.searchParams.get('value_filter_mode') || 'OR').toUpperCase();
		return mode === 'AND' || mode === 'NOT' ? mode : 'OR';
	}

	function navigateWithFilters(url: URL, replaceState: boolean = false, openFilters: boolean = true) {
		showFilterPanel = openFilters;
		goto(url.pathname + url.search, {
			replaceState,
			noScroll: true,
			keepFocus: true
		});
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
		} catch (e) {
			console.error('Failed to fetch library name:', e);
		}
	}

	function buildBooksUrl(offset: number = 0, limit: number = BATCH_SIZE, includeTotal: boolean = true): string {
		const params = $page.url.searchParams;
		const authors = getQueryValues(params, 'author');
		const series = getQueryValues(params, 'series');
		const genres = getQueryValues(params, 'genre');
		const tags = getQueryValues(params, 'tags');
		const formats = getQueryValues(params, 'format');
		const statuses = getQueryValues(params, 'status');
		const filterMode = getFilterMode();
		const valueFilterMode = getValueFilterMode();

		const queryParams = new URLSearchParams();
		queryParams.set('limit', String(limit));
		queryParams.set('offset', String(offset));
		if (!includeTotal) queryParams.set('include_total', 'false');

		if (libraryFilter) queryParams.set('library_id', libraryFilter);
		const q = $page.url.searchParams.get('q');
		if (q) queryParams.set('q', q);
		for (const author of authors) queryParams.append('author', author);
		for (const seriesName of series) queryParams.append('series', seriesName);
		for (const genre of genres) queryParams.append('genre', genre);
		for (const tag of tags) queryParams.append('tags', tag);
		for (const format of formats) queryParams.append('format', format);
		for (const status of statuses) queryParams.append('status', status);
		if (filterMode !== 'AND') queryParams.set('filter_mode', filterMode);
		if (valueFilterMode !== 'OR') queryParams.set('value_filter_mode', valueFilterMode);
		queryParams.set('sort', sortBy);
		queryParams.set('sort_dir', sortDir);

		return '/api/books?' + queryParams.toString();
	}

	function buildFilterOptionsUrl(): string {
		const params = $page.url.searchParams;
		const queryParams = new URLSearchParams();
		if (libraryFilter) queryParams.set('library_id', libraryFilter);
		const q = params.get('q')?.trim();
		if (q) queryParams.set('q', q);
		const query = queryParams.toString();
		return query ? `/api/filter-options?${query}` : '/api/filter-options';
	}

	function buildBookNavigationUrl(): string | undefined {
		if (sortBy === 'random') return undefined;
		const booksUrl = buildBooksUrl(0, BATCH_SIZE, true);
		const query = booksUrl.split('?')[1] || '';
		const params = new URLSearchParams(query);
		params.delete('limit');
		params.delete('offset');
		params.delete('include_total');
		return `/api/books/navigation?${params.toString()}`;
	}

	async function fetchBooks(reset: boolean = true) {
		if (reset) {
			loading = true;
			currentOffset = 0;
		} else {
			loadingMore = true;
		}

		try {
			const url = buildBooksUrl(reset ? 0 : currentOffset, BATCH_SIZE, reset);
			const res = await fetch(url);
			if (res.ok) {
				const data = await res.json();
				if (data.books) {
					if (typeof data.total === 'number') {
						totalBooks = data.total;
					}
					serverHasMore = typeof data.has_more === 'boolean'
						? data.has_more
						: books.length + data.books.length < totalBooks;
					if (reset) {
						books = data.books;
					} else {
						books = [...books, ...data.books];
					}
					currentOffset = books.length;
					sortBooks();
				}
			}
		} catch (e) {
			console.error('Failed to fetch books:', e);
		} finally {
			loading = false;
			loadingMore = false;
			// Re-setup observer after content changes
			setTimeout(() => {
				if (loadMoreTrigger && hasMore) {
					setupObserver();
				}
				if (reset) {
					restoreRouteScrollPosition();
				}
			}, 200);
		}
	}

	async function refreshBooksInBackground() {
		if (loading || loadingMore || backgroundRefreshing) return;
		backgroundRefreshing = true;

		try {
			const previousBooks = books;
			const loadedAllKnownBooks = previousBooks.length >= totalBooks;
			const refreshLimit = Math.min(
				MAX_REFRESH_LIMIT,
				Math.max(BATCH_SIZE, previousBooks.length + (loadedAllKnownBooks ? BATCH_SIZE : 0))
			);
			const res = await fetch(buildBooksUrl(0, refreshLimit, false), { cache: 'no-store' });
			if (res.ok) {
				const data = await res.json();
				if (data.books) {
					const refreshedBooks = data.books;
					if (previousBooks.length > refreshedBooks.length) {
						const refreshedIDs = new Set(refreshedBooks.map((book: any) => book.id));
						const existingTail = previousBooks.filter((book) => !refreshedIDs.has(book.id));
						books = [...refreshedBooks, ...existingTail].slice(0, Math.max(refreshedBooks.length, previousBooks.length));
					} else {
						books = refreshedBooks;
					}
					if (typeof data.total === 'number') {
						totalBooks = data.total;
					}
					if (typeof data.has_more === 'boolean') {
						serverHasMore = data.has_more;
					}
					currentOffset = books.length;
					sortBooks();
				}
			}
		} catch (e) {
			console.error('Failed to refresh books:', e);
		} finally {
			backgroundRefreshing = false;
		}
	}

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		await fetchBooks(false);
	}

	function scheduleBackgroundRefresh(delay?: number) {
		if (backgroundRefreshTimer !== null) {
			window.clearTimeout(backgroundRefreshTimer);
		}

		const nextDelay = delay ?? (
			scanning || scanMessage
				? BACKGROUND_REFRESH_ACTIVE_INTERVAL_MS
				: BACKGROUND_REFRESH_IDLE_INTERVAL_MS
		);

		backgroundRefreshTimer = window.setTimeout(async () => {
			if (document.visibilityState === 'visible') {
				await refreshBooksInBackground();
			}
			scheduleBackgroundRefresh();
		}, nextDelay);
	}

	function handleVisibilityChange() {
		if (document.visibilityState === 'visible') {
			scheduleBackgroundRefresh(500);
		}
	}

	async function fetchFilterOptions() {
		try {
			const res = await fetch(buildFilterOptionsUrl());
			if (!res.ok) return;
			const data = await res.json();
			availableAuthors = data.authors ?? [];
			availableSeries = data.series ?? [];
			availableTags = data.tags ?? [];
			availableFormats = data.formats ?? [];
		} catch (e) {
			console.error('Failed to fetch filter options:', e);
		}
	}

	$effect(() => {
		const filter = libraryFilter;
		if (filter !== undefined) {
			fetchLibraryName();
		}
	});

	$effect(() => {
		// Re-fetch when URL params change
		$page.url.search;
		const nextSelectionScope = currentSelectionScopeKey();
		if (bulkSelectionRestored && activeBulkSelectionScope && activeBulkSelectionScope !== nextSelectionScope) {
			clearBulkSelectionState();
		}
		activeBulkSelectionScope = nextSelectionScope;
		librarySearch = $page.url.searchParams.get('q') || '';
		sortBy = getSortByForUrl($page.url);
		sortDir = getSortDirForUrl($page.url);
		fetchBooks(true);
		fetchFilterOptions();
	});

	$effect(() => {
		sortBy;
		sortDir;
		if (books.length > 0) {
			sortBooks();
		}
	});

	$effect(() => {
		showBulkPanel = selectedBooks.size > 0;
		persistBulkSelectionState();
	});

	function selectionScopeKeyForUrl(url: URL) {
		const libraryId = url.searchParams.get('library') || '';
		return libraryId ? `library:${libraryId}` : 'library:all';
	}

	function currentSelectionScopeKey() {
		return selectionScopeKeyForUrl($page.url);
	}

	function currentRouteKey() {
		return typeof window === 'undefined' ? '' : `${window.location.pathname}${window.location.search}`;
	}

	function clearBulkSelectionState() {
		selectedBooks = new Set();
		bulkSelectionAnchorId = null;
		selectAllMode = 'none';
		showMetadataMenu = false;
		sessionStorage.removeItem(BULK_SELECTION_STORAGE_KEY);
	}

	beforeNavigate(({ to }) => {
		if (selectedBooks.size === 0) return;
		if (!to?.url) {
			clearBulkSelectionState();
			return;
		}
		const targetPath = to.url.pathname;
		const isBookWorkflow = /^\/book\/\d+/.test(targetPath);
		const isSameLibraryScope = targetPath === '/library' && selectionScopeKeyForUrl(to.url) === currentSelectionScopeKey();
		if (!isBookWorkflow && !isSameLibraryScope) {
			clearBulkSelectionState();
		}
	});

	function persistBulkSelectionState() {
		if (typeof window === 'undefined') return;
		if (!bulkSelectionRestored) return;
		if (selectedBooks.size === 0) {
			sessionStorage.removeItem(BULK_SELECTION_STORAGE_KEY);
			return;
		}
		sessionStorage.setItem(BULK_SELECTION_STORAGE_KEY, JSON.stringify({
			scope: currentSelectionScopeKey(),
			sourcePath: currentRouteKey(),
			selectedIds: Array.from(selectedBooks),
			selectAllMode,
			timestamp: Date.now()
		}));
	}

	function restoreBulkSelectionState() {
		if (typeof window === 'undefined') return;
		try {
			const raw = sessionStorage.getItem(BULK_SELECTION_STORAGE_KEY);
			if (!raw) {
				bulkSelectionRestored = true;
				activeBulkSelectionScope = currentSelectionScopeKey();
				return;
			}
			const state = JSON.parse(raw);
			const stateScope = typeof state.scope === 'string' ? state.scope : state.route;
			if (stateScope !== currentSelectionScopeKey()) {
				bulkSelectionRestored = true;
				activeBulkSelectionScope = currentSelectionScopeKey();
				return;
			}
			const selectedIds = Array.isArray(state.selectedIds) ? state.selectedIds.map(Number).filter(Number.isFinite) : [];
			selectedBooks = new Set(selectedIds);
			selectAllMode = state.selectAllMode === 'filtered' || state.selectAllMode === 'page' ? state.selectAllMode : 'none';
			showBulkPanel = selectedIds.length > 0;
		} catch {
			sessionStorage.removeItem(BULK_SELECTION_STORAGE_KEY);
		} finally {
			bulkSelectionRestored = true;
			activeBulkSelectionScope = currentSelectionScopeKey();
		}
	}

	let loadMoreTrigger = $state<HTMLDivElement | null>(null);
	let observer: IntersectionObserver | null = null;
	const LOAD_MORE_ROOT_MARGIN = '1400px 0px';

	function getLoadMoreRoot(): Element | null {
		return loadMoreTrigger?.closest('main') ?? null;
	}

	function setupObserver() {
		if (observer) {
			observer.disconnect();
			observer = null;
		}

		if (loadMoreTrigger && hasMore) {
			const root = getLoadMoreRoot();
			observer = new IntersectionObserver(
				(entries) => {
					const entry = entries[0];
					if (entry.isIntersecting && hasMore && !loadingMore) {
						loadMore();
					}
				},
				{ root, threshold: 0, rootMargin: LOAD_MORE_ROOT_MARGIN }
			);
			observer.observe(loadMoreTrigger);
		}
	}

	$effect(() => {
		// Re-setup observer when trigger element exists, books change, or loading state changes
		if (loadMoreTrigger && books.length > 0) {
			// Small delay to ensure DOM has updated
			setTimeout(() => setupObserver(), 100);
		}
	});



	onMount(() => {
		restoreBulkSelectionState();
		fetchLibraryName();
		fetchBooks(true);
		fetchFilterOptions();
		gridScale.init();
		showFormatOnCover.init();
		scheduleBackgroundRefresh(BACKGROUND_REFRESH_IDLE_INTERVAL_MS);
		document.addEventListener('visibilitychange', handleVisibilityChange);
		const filterPanelBackdropMedia = window.matchMedia('(max-width: 1023px)');
		const updateFilterPanelBackdrop = (event: MediaQueryListEvent | MediaQueryList) => {
			filterPanelBackdrop = event.matches;
		};
		updateFilterPanelBackdrop(filterPanelBackdropMedia);
		filterPanelBackdropMedia.addEventListener('change', updateFilterPanelBackdrop);
		return () => {
			filterPanelBackdropMedia.removeEventListener('change', updateFilterPanelBackdrop);
			if (backgroundRefreshTimer !== null) {
				window.clearTimeout(backgroundRefreshTimer);
			}
			document.removeEventListener('visibilitychange', handleVisibilityChange);
			if (searchUpdateTimer) clearTimeout(searchUpdateTimer);
			if (observer) observer.disconnect();
		};
	});

	async function scanLibrary(targetLibrary: any | null = null) {
		const targetLibraryId = targetLibrary?.id ? String(targetLibrary.id) : libraryFilter;
		const targetLibraryName = targetLibrary?.name || libraryName || 'Library';
		scanning = true;
		scanMessage = 'Scanning...';
		const pendingJob = appActivity.startPendingJob({
			job_type: 'library_scan',
			title: targetLibraryId ? `Scan library: ${targetLibraryName}` : 'Scan libraries',
			payload: targetLibraryId ? { library_id: Number(targetLibraryId), library_name: targetLibraryName } : {}
		});
		try {
			const res = targetLibraryId
				? await fetch(`/api/libraries/${targetLibraryId}/scan`, { method: 'POST' })
				: await fetch('/api/scan', { method: 'POST' });
			const data = await res.json().catch(() => ({}));
			if (res.ok) {
				const jobIds = targetLibraryId
					? [data.job_id].filter(Boolean)
					: [...(data.queued_jobs ?? []), ...(data.existing_jobs ?? [])];
				if (jobIds.length > 0) {
					appActivity.confirmPendingJob(pendingJob, jobIds);
				} else {
					appActivity.removePendingJob(pendingJob);
				}
				await appActivity.refresh();
				scanMessage = 'Scan started. Updating automatically...';
				setTimeout(async () => {
					await fetchBooks(true);
					scanMessage = '';
					scanning = false;
				}, 5000);
			} else {
				appActivity.failPendingJob(pendingJob, 'Unable to queue library scan.');
				scanMessage = `Scan failed: ${data.error || 'Unknown error'}`;
				scanning = false;
			}
		} catch (e) {
			appActivity.failPendingJob(pendingJob, 'Unable to queue library scan.');
			scanMessage = 'Scan failed. Check console for details.';
			scanning = false;
		}
	}

	async function openEditLibraryModal() {
		if (!libraryFilter) return;
		try {
			const response = await fetch(`/api/libraries/${libraryFilter}`, { cache: 'no-store' });
			editingLibrary = response.ok
				? await response.json()
				: { id: Number(libraryFilter), name: libraryName || 'Library', book_count: totalBooks };
			showLibraryModal = true;
		} catch (error) {
			console.error('Failed to load library for editing:', error);
			editingLibrary = { id: Number(libraryFilter), name: libraryName || 'Library', book_count: totalBooks };
			showLibraryModal = true;
		}
	}

	function closeLibraryModal() {
		showLibraryModal = false;
		editingLibrary = null;
	}

	async function handleLibrarySaved(result: { library: any; isEditing: boolean; foldersChanged: boolean }) {
		closeLibraryModal();
		await fetchLibraryName();
		await fetchBooks(true);
		await fetchFilterOptions();
		if ((window as any).refreshSidebar) {
			(window as any).refreshSidebar();
		}
		if (result.isEditing && result.foldersChanged && confirm('Library folders changed. Scan this library now?')) {
			await scanLibrary(result.library);
		}
	}

	async function regenerateLibraryCovers(library: any, mode: 'all' | 'missing') {
		if (!library?.id) return;
		if (!confirmBulkAction({
			action: mode === 'all' ? 'regenerate covers for' : 'regenerate missing covers for',
			count: library.book_count || totalBooks
		})) return;

		const pendingJob = appActivity.startPendingJob({
			job_type: 'cover_regenerate',
			title: mode === 'all' ? `Regenerate covers for ${library.name}` : `Regenerate missing covers for ${library.name}`,
			payload: { mode, library_id: library.id, library_name: library.name }
		});
		try {
			const response = await fetch('/api/settings/book-covers/regenerate', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ mode, library_id: library.id })
			});
			if (!response.ok) {
				appActivity.failPendingJob(pendingJob, 'Unable to queue cover regeneration.');
				console.error('Failed to queue library cover regeneration:', await response.text());
				return;
			}
			const job = await response.json().catch(() => null);
			if (job?.id) {
				appActivity.confirmPendingJob(pendingJob, job);
			} else {
				appActivity.confirmPendingJob(pendingJob);
			}
			await appActivity.refresh();
		} catch (error) {
			console.error('Failed to queue library cover regeneration:', error);
			appActivity.failPendingJob(pendingJob, 'Unable to queue cover regeneration.');
		}
	}

	function formatDate(timestamp: number) {
		return new Date(timestamp * 1000).toLocaleDateString();
	}

	function parseAuthors(authorsJson: string): string {
		try {
			const arr = JSON.parse(authorsJson);
			return Array.isArray(arr) ? arr.join(', ') : authorsJson;
		} catch {
			return authorsJson;
		}
	}

	function sortKey(value: string | undefined | null): string {
		return String(value || '')
			.trim()
			.replace(/^[\s\p{P}\p{S}]+/u, '')
			.replace(LATIN_CONFUSABLE_SORT_PATTERN, (char) => LATIN_CONFUSABLE_SORT_MAP[char] ?? char)
			.toLocaleLowerCase();
	}

	function sortBooks() {
		books.sort((a, b) => {
			let comparison = 0;
			switch (sortBy) {
				case 'title':
					comparison = sortKey(a.title).localeCompare(sortKey(b.title));
					break;
				case 'authors':
					comparison = sortKey(parseAuthors(a.authors)).localeCompare(sortKey(parseAuthors(b.authors)));
					break;
				case 'added_at':
					comparison = (a.added_at || 0) - (b.added_at || 0);
					break;
				case 'last_read':
					comparison = (a.last_read_at || a.added_at || 0) - (b.last_read_at || b.added_at || 0);
					break;
				case 'series':
					return compareSeriesBooks(a, b);
				default:
					comparison = 0;
			}
			return sortDir === 'desc' ? -comparison : comparison;
		});
	}

	function compareSeriesBooks(a: any, b: any) {
		const aSeries = sortKey(a.series);
		const bSeries = sortKey(b.series);
		if (!aSeries && bSeries) return 1;
		if (aSeries && !bSeries) return -1;

		const seriesComparison = aSeries.localeCompare(bSeries);
		if (seriesComparison !== 0) return sortDir === 'desc' ? -seriesComparison : seriesComparison;

		const aNumber = Number(a.series_number || 0);
		const bNumber = Number(b.series_number || 0);
		if (aNumber && !bNumber) return -1;
		if (!aNumber && bNumber) return 1;
		if (aNumber !== bNumber) return sortDir === 'desc' ? bNumber - aNumber : aNumber - bNumber;
		return sortKey(a.title).localeCompare(sortKey(b.title));
	}

	const sortOptions = [
		{ value: 'title', label: 'Title' },
		{ value: 'authors', label: 'Author' },
		{ value: 'series', label: 'Series' },
		{ value: 'added_at', label: 'Date Added' },
		{ value: 'last_read', label: 'Last Read' }
	];

	function currentSortLabel() {
		return sortOptions.find((option) => option.value === sortBy)?.label ?? 'Title';
	}

	function sortDirectionLabel() {
		return sortDir === 'asc' ? 'Ascending' : 'Descending';
	}

	function sortDirectionArrow() {
		return sortDir === 'asc' ? '↑' : '↓';
	}

	function setSort(value: string) {
		sortBy = value;
		showSortMenu = false;
		const url = new URL(window.location.href);
		url.searchParams.set('sort', sortBy);
		url.searchParams.set('sort_dir', sortDir);
		navigateWithFilters(url, true, false);
	}

	function toggleSortDirection() {
		sortDir = sortDir === 'asc' ? 'desc' : 'asc';
		const url = new URL(window.location.href);
		url.searchParams.set('sort', sortBy);
		url.searchParams.set('sort_dir', sortDir);
		navigateWithFilters(url, true, false);
	}

	function updateScopedSearch(value: string) {
		librarySearch = value;
		if (searchUpdateTimer) clearTimeout(searchUpdateTimer);
		searchUpdateTimer = setTimeout(() => {
			const url = new URL(window.location.href);
			const query = librarySearch.trim();
			if (query) {
				url.searchParams.set('q', query);
			} else {
				url.searchParams.delete('q');
			}
			navigateWithFilters(url, true, false);
		}, 300);
	}

	function clearScopedSearch() {
		if (searchUpdateTimer) clearTimeout(searchUpdateTimer);
		librarySearch = '';
		const url = new URL(window.location.href);
		url.searchParams.delete('q');
		navigateWithFilters(url, true, false);
	}

	function statusDot(status: string) {
		switch (status) {
			case 'reading': return 'bg-blue-500';
			case 'finished': return 'bg-emerald-500';
			default: return '';
		}
	}

	function getVisibleBookIds(): number[] {
		return books.map((book) => book.id);
	}

	function openBookDetailFromList(event: MouseEvent, bookId: number) {
		saveRouteScrollPosition();
		persistBulkSelectionState();
		event.stopPropagation();
		if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) return;
		event.preventDefault();
		const visibleIds = getVisibleBookIds();
		const navigationUrl = buildBookNavigationUrl();
		const session = startMetadataEditContextSession(visibleIds, `${$page.url.pathname}${$page.url.search}`, {
			navigationUrl,
			totalCount: navigationUrl ? totalBooks : visibleIds.length
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
				updateSelectAllMode();
				return;
			}
		}
		const newSet = new Set(selectedBooks);
		if (newSet.has(bookId)) {
			newSet.delete(bookId);
		} else {
			newSet.add(bookId);
		}
		selectedBooks = newSet;
		bulkSelectionAnchorId = bookId;
		updateSelectAllMode();
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
		// If not in bulk mode, let the link handle navigation normally
    }

	function getReaderUrl(book: any): string {
		return getBookReaderHref(book.id, book.format, '/library');
	}

	function getReaderActionLabel(book: any): string {
		const title = book.title || 'book';
		return isAudioFormat(book.format) ? `Play audio for ${title}` : `Read ${title}`;
	}

	function isTouchLike(): boolean {
		return typeof window !== 'undefined' && window.matchMedia('(hover: none), (pointer: coarse)').matches;
	}

	function handleCoverTap(event: MouseEvent, bookId: number) {
		if (!isTouchLike() || bulkSelectMode) return;
		event.preventDefault();
		event.stopPropagation();
		activeBookActions = activeBookActions === bookId ? null : bookId;
	}

	function handleCoverKeydown(event: KeyboardEvent, bookId: number) {
		if (event.key !== 'Enter' && event.key !== ' ') return;
		event.preventDefault();
		activeBookActions = activeBookActions === bookId ? null : bookId;
	}

    function handleBookKeydown(event: KeyboardEvent) {
		if (event.key !== 'Enter' && event.key !== ' ') return;
		event.preventDefault();
		handleBookClick(event as unknown as MouseEvent);
    }

	function clearLongPressTimer() {
		if (longPressTimer) {
			clearTimeout(longPressTimer);
			longPressTimer = null;
		}
	}

	function handleMouseDown(event: MouseEvent) {
		if ('ontouchstart' in window) return;
		const bookId = Number((event.currentTarget as HTMLElement).dataset.bookId);
		longPressTimer = window.setTimeout(() => {
			suppressNextClickBookId = bookId;
			toggleBookSelection(bookId);
			longPressTimer = null;
		}, longPressThreshold);
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

	function updateSelectAllMode() {
		if (selectedBooks.size === 0) {
			selectAllMode = 'none';
		} else if (selectedBooks.size === books.length && books.length < totalBooks) {
			selectAllMode = 'page'; // Selected all on page but not all in filtered set
		} else if (selectedBooks.size === totalBooks) {
			selectAllMode = 'filtered'; // Selected all in filtered set
		} else {
			selectAllMode = 'page'; // Partial selection on page
		}
	}

	function selectAllPage() {
		selectedBooks = new Set([...selectedBooks, ...books.map(b => b.id)]);
		selectAllMode = 'page';
	}

	function selectAllFiltered() {
		// For filter-based selection, we just track that we're in "filtered" mode
		// The actual selection happens via the filter-based bulk API
		selectAllMode = 'filtered';
		// Select all currently loaded books too for UI purposes
		selectedBooks = new Set(books.map(b => b.id));
	}

	function deselectAll() {
		selectedBooks = new Set();
		bulkSelectionAnchorId = null;
		selectAllMode = 'none';
	}

	function isBookSelected(bookId: number): boolean {
		return selectedBooks.has(bookId);
	}

	async function fetchShelves() {
		try {
			const res = await fetch('/api/shelves', { cache: 'no-store' });
			if (res.ok) {
				shelves = await res.json();
			}
		} catch (e) {
			console.error('Failed to fetch shelves:', e);
		}
	}

	function getCurrentFilterParams() {
		return {
			library_id: libraryFilter || undefined,
			author: getQueryValues($page.url.searchParams, 'author'),
			series: getQueryValues($page.url.searchParams, 'series'),
			genre: getQueryValues($page.url.searchParams, 'genre'),
			tags: getQueryValues($page.url.searchParams, 'tags'),
			format: getQueryValues($page.url.searchParams, 'format'),
			status: getQueryValues($page.url.searchParams, 'status'),
			q: $page.url.searchParams.get('q') || undefined,
			filter_mode: getFilterMode(),
			value_filter_mode: getValueFilterMode()
		};
	}

	async function addToShelf(shelfId: number) {
		const count = getSelectionCount();
		if (!confirmBulkAction({ action: 'add {count} books to this shelf', count })) return;
		actionInProgress = true;
		try {
			let res;
			if (selectAllMode === 'filtered') {
				// Use filter-based endpoint
				res = await fetch(`/api/shelves/${shelfId}/books/bulk-by-filter`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(getCurrentFilterParams())
				});
			} else {
				// Use individual book IDs
				res = await fetch(`/api/shelves/${shelfId}/books/bulk`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ book_ids: Array.from(selectedBooks) })
				});
			}

			if (res.ok) {
				showShelfPicker = false;
				deselectAll();
				await fetchBooks(true);
			} else {
				console.error('Failed to add books to shelf');
			}
		} catch (e) {
			console.error('Failed to add books to shelf:', e);
		} finally {
			actionInProgress = false;
		}
	}

	async function deleteSelectedBooks() {
		const count = selectAllMode === 'filtered' ? totalBooks : selectedBooks.size;
		if (!confirmBulkAction({ action: 'delete', count, destructive: true, alwaysConfirm: true })) return;

		actionInProgress = true;
		try {
			let res;
			if (selectAllMode === 'filtered') {
				// Use filter-based endpoint
				res = await fetch('/api/books/bulk-delete-by-filter', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(getCurrentFilterParams())
				});
			} else {
				// Use individual book IDs
				res = await fetch('/api/books/bulk-delete', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ book_ids: Array.from(selectedBooks) })
				});
			}

			if (res.ok) {
				await fetchBooks(true);
				deselectAll();
			} else {
				console.error('Failed to delete books');
			}
		} catch (e) {
			console.error('Failed to delete books:', e);
		} finally {
			actionInProgress = false;
		}
	}

	function openShelfPicker() {
		fetchShelves();
		showShelfPicker = true;
	}

	async function handleShelfCreatedFromPicker() {
		showCreateShelfModal = false;
		await fetchShelves();
		await (window as any).refreshSidebar?.();
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
		bulkMetadataEditBookIds = selectAllMode === 'filtered' ? [] : Array.from(selectedBooks);
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

	function getActiveFilters(): { key: string; value: string; label: string }[] {
		const filters: { key: string; value: string; label: string }[] = [];
		const params = $page.url.searchParams;

		for (const author of getQueryValues(params, 'author')) {
			filters.push({ key: 'author', value: author, label: `Author: ${author}` });
		}
		for (const seriesName of getQueryValues(params, 'series')) {
			filters.push({ key: 'series', value: seriesName, label: `Series: ${seriesName}` });
		}
		for (const genre of getQueryValues(params, 'genre')) {
			filters.push({ key: 'genre', value: genre, label: `Tag: ${genre}` });
		}
		for (const tag of getQueryValues(params, 'tags')) {
			filters.push({ key: 'tags', value: tag, label: `Tag: ${tag}` });
		}
		for (const format of getQueryValues(params, 'format')) {
			filters.push({ key: 'format', value: format, label: `Format: ${format.toUpperCase()}` });
		}
		for (const status of getQueryValues(params, 'status')) {
			filters.push({ key: 'status', value: status, label: `Status: ${status}` });
		}
		const search = params.get('q')?.trim();
		if (search) {
			filters.push({ key: 'q', value: search, label: `Search: ${search}` });
		}

		return filters;
	}

	function removeFilter(key: string, value: string) {
		const url = new URL($page.url);
		if (key === 'genre' || key === 'tags') {
			const values = getQueryValues(url.searchParams, key).filter((item) => item !== value);
			setQueryValues(url.searchParams, key, values);
		} else {
			const values = getQueryValues(url.searchParams, key).filter((item) => item !== value);
			url.searchParams.delete(key);
			for (const item of values) url.searchParams.append(key, item);
		}
		navigateWithFilters(url, false, false);
	}

	function clearAllFilters() {
		const url = new URL($page.url);
		for (const key of FILTER_PANEL_PARAM_KEYS) url.searchParams.delete(key);
		url.searchParams.delete('q');
		navigateWithFilters(url, false, false);
	}

	function hasFilterPanelFilters(): boolean {
		return FILTER_PANEL_PARAM_KEYS.some((key) => $page.url.searchParams.has(key));
	}

	function clearFilterPanelFilters() {
		const url = new URL($page.url);
		for (const key of FILTER_PANEL_PARAM_KEYS) url.searchParams.delete(key);
		navigateWithFilters(url, true, false);
	}

	function toggleRepeatedFilter(key: string, value: string) {
		const url = new URL($page.url);
		const values = getQueryValues(url.searchParams, key);
		const nextValues = values.includes(value)
			? values.filter((item) => item !== value)
			: [...values, value];

		url.searchParams.delete(key);
		for (const item of nextValues) url.searchParams.append(key, item);
		navigateWithFilters(url);
	}

	function setFilterMode(mode: FilterMode) {
		const url = new URL($page.url);
		if (mode === 'AND') {
			url.searchParams.delete('filter_mode');
		} else {
			url.searchParams.set('filter_mode', mode);
		}
		navigateWithFilters(url, true);
	}

	function setValueFilterMode(mode: FilterMode) {
		const url = new URL($page.url);
		if (mode === 'OR') {
			url.searchParams.delete('value_filter_mode');
		} else {
			url.searchParams.set('value_filter_mode', mode);
		}
		navigateWithFilters(url, true);
	}

	function applyAuthorFilter(authorName: string) {
		toggleRepeatedFilter('author', authorName);
	}

	function applySeriesFilter(seriesName: string) {
		toggleRepeatedFilter('series', seriesName);
	}

	function toggleTagSelection(tagName: string) {
		const url = new URL($page.url);
		const tagList = getQueryValues(url.searchParams, 'tags');
		const newTags = tagList.includes(tagName)
			? tagList.filter((tag) => tag !== tagName)
			: [...tagList, tagName];

		setQueryValues(url.searchParams, 'tags', newTags);
		if (newTags.length === 0) {
			url.searchParams.delete('tag_mode');
		} else {
			url.searchParams.delete('tag_mode');
		}
		navigateWithFilters(url);
	}

	function isTagSelected(tagName: string): boolean {
		return getQueryValues($page.url.searchParams, 'tags').includes(tagName);
	}

	function isFormatSelected(format: string): boolean {
		return getQueryValues($page.url.searchParams, 'format').includes(format);
	}

	function isAuthorSelected(authorName: string): boolean {
		return getQueryValues($page.url.searchParams, 'author').includes(authorName);
	}

	function isSeriesSelected(seriesName: string): boolean {
		return getQueryValues($page.url.searchParams, 'series').includes(seriesName);
	}

	function isStatusSelected(status: string): boolean {
		return getQueryValues($page.url.searchParams, 'status').includes(status);
	}

	function applyStatusFilter(status: string) {
		toggleRepeatedFilter('status', status);
	}

	function applyFormatFilter(format: string) {
		toggleRepeatedFilter('format', format);
	}

	function getSelectionCount(): number {
		if (selectAllMode === 'filtered') {
			return totalBooks;
		}
		return selectedBooks.size;
	}

	function getVisibleSelectedCount(): number {
		if (selectAllMode === 'filtered') {
			return Math.min(books.length, totalBooks);
		}
		const visibleIds = new Set(books.map((book) => book.id));
		return Array.from(selectedBooks).filter((bookId) => visibleIds.has(bookId)).length;
	}

	function getHiddenSelectedCount(): number {
		return Math.max(0, getSelectionCount() - getVisibleSelectedCount());
	}
  </script>

<div class="pb-20" style={`--filter-panel-offset: ${$filterPanelWidth + 24}px;`}>
		<div class="sticky top-0 z-30 px-3 py-3 sm:px-6 sm:py-4 bg-[var(--color-surface-base)]/95 backdrop-blur border-b border-[var(--color-surface-border)] shadow-[0_1px_0_rgba(255,255,255,0.04)] {showFilterPanel ? 'lg:pr-[var(--filter-panel-offset)]' : ''}">
				<div bind:this={toolbarContainer} class="space-y-3">
				<div class="flex min-w-0 items-start justify-between gap-3">
					<div class="min-w-0">
						<div class="flex min-w-0 items-baseline gap-2 sm:gap-3">
							<h1 class="truncate text-xl font-bold text-[var(--color-surface-text)] sm:text-2xl">{libraryFilter ? libraryName || 'Library' : 'All Books'}</h1>
							{#if totalBooks > 0}
								<p class="whitespace-nowrap text-sm text-[var(--color-surface-text-muted)]">{totalBooks} books</p>
							{/if}
						</div>
						{#if scanMessage}
							<div class="mt-2 inline-flex items-center gap-2 rounded-lg border border-[var(--color-primary-500)]/40 bg-[var(--color-primary-500)]/15 px-3 py-1.5 text-sm text-[var(--color-primary-300)]">
								{scanMessage}
							</div>
						{/if}
					</div>
					{#if libraryFilter}
						<button
							type="button"
							onclick={openEditLibraryModal}
							class="inline-flex h-9 shrink-0 items-center gap-2 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 text-sm font-medium text-[var(--color-surface-text)] transition-colors hover:border-[var(--color-primary-500)]/50 hover:text-[var(--color-primary-400)]"
							title="Edit Library"
						>
							<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m16.862 4.487 1.687-1.688a1.875 1.875 0 1 1 2.652 2.652L10.582 16.07a4.5 4.5 0 0 1-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 0 1 1.13-1.897l8.932-8.931Z"></path>
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19.5 7.125 16.875 4.5"></path>
							</svg>
							<span class="hidden sm:inline">Edit Library</span>
						</button>
					{/if}
				</div>

				<div class="grid min-w-0 grid-cols-[minmax(0,1fr)_2.5rem_auto_auto] items-center gap-2">
						<div class="relative col-start-2 row-start-1">
					{#if showSettingsMenu}
						<button
							type="button"
							class="fixed inset-0 z-20"
							aria-label="Close settings menu"
							onclick={() => showSettingsMenu = false}
						></button>
					{/if}
					<button
						onclick={() => showSettingsMenu = !showSettingsMenu}
						aria-label="Library settings"
						class="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-[var(--color-surface-overlay)] hover:bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] font-medium transition-colors"
					>
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
						</svg>
						</button>
						{#if showSettingsMenu}
							<div class="absolute right-0 top-full z-40 mt-2 w-56 max-w-[calc(100vw-1.5rem)] max-h-[70vh] overflow-y-auto rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-3 shadow-lg">
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
											<label class="text-sm font-medium text-[var(--color-surface-text)]" for="library-grid-scale">Grid Scale</label>
										<span class="text-xs text-[var(--color-surface-text-muted)]">{responsiveGridColumns} cols</span>
									</div>
									<div class="flex items-center space-x-2">
										<input
											id="library-grid-scale"
											type="range"
											min={MIN_GRID_SCALE}
											max={MAX_GRID_SCALE}
											step={GRID_SCALE_STEP}
											value={localGridScale}
											oninput={(e) => updateGridScale(Number(e.currentTarget.value))}
											class="flex-1 h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer slider"
										>
										<span class="text-sm text-[var(--color-surface-text)] w-12 text-right">{localGridScale}%</span>
									</div>
								</div>
							{/if}
							<button
								onclick={toggleFormatOnCover}
								class="w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] flex items-center justify-between"
							>
								<span>Show Format on Cover</span>
								<span class="w-10 h-6 rounded-full transition-colors {formatOnCover ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-surface-600)]'} relative">
									<span class="absolute top-1 w-4 h-4 bg-white rounded-full transition-transform {formatOnCover ? 'left-5' : 'left-1'}"></span>
								</span>
							</button>
							<div class="border-t border-[var(--color-surface-border)] mt-1 pt-1">
								<button
									onclick={() => scanLibrary()}
									disabled={scanning}
									class="group w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] flex items-center disabled:opacity-50"
								>
									{#if scanning}
										<svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
											<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
											<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
										</svg>
										Scanning...
									{:else}
										<svg class="w-4 h-4 mr-2 transition-transform duration-200 ease-out group-hover:-rotate-45" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
										</svg>
										{libraryFilter ? 'Scan Library' : 'Scan All Libraries'}
									{/if}
								</button>
							</div>
						</div>
					{/if}
				</div>

					<div class="relative col-start-1 row-start-1 min-w-0">
					<svg class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-4.35-4.35M10.5 18a7.5 7.5 0 110-15 7.5 7.5 0 010 15z"></path>
					</svg>
					<input
						type="search"
						value={librarySearch}
						oninput={(event) => updateScopedSearch(event.currentTarget.value)}
						placeholder="Search current results"
						aria-label="Search current library results"
						class="h-10 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-2 pl-9 text-sm text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:border-[var(--color-primary-500)] focus:outline-none {librarySearch ? 'pr-9' : 'pr-3'}"
					/>
					{#if librarySearch}
						<button
							type="button"
							onclick={clearScopedSearch}
							class="absolute right-2 top-1/2 flex h-6 w-6 -translate-y-1/2 items-center justify-center rounded text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]"
							aria-label="Clear library search"
						>
							<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"></path>
							</svg>
						</button>
					{/if}
				</div>

					<div class="relative col-start-3 row-start-1">
					{#if showSortMenu}
						<button
							type="button"
							class="fixed inset-0 z-20"
							aria-label="Close sort menu"
							onclick={() => showSortMenu = false}
						></button>
					{/if}
					<div class="inline-flex h-10 overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] {compactToolbar ? 'max-w-[5.5rem]' : 'max-w-32 sm:max-w-40 md:max-w-48'}">
						<button
							onclick={() => showSortMenu = !showSortMenu}
							aria-label="Sort books by {currentSortLabel()}"
							class="inline-flex min-w-0 items-center text-[var(--color-surface-text)] font-medium transition-colors hover:bg-[var(--color-surface-700)] {compactToolbar ? 'px-2' : 'px-3 sm:px-4'}"
						>
							<span class={compactToolbar ? 'hidden' : 'hidden min-w-0 truncate text-sm sm:inline'}>{currentSortLabel()}</span>
							<svg class="w-4 h-4 {compactToolbar ? '' : 'sm:ml-2'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
							</svg>
						</button>
						<button
							type="button"
							onclick={toggleSortDirection}
							class="inline-flex w-10 items-center justify-center border-l border-[var(--color-surface-border)] text-[var(--color-primary-400)] transition-colors hover:bg-[var(--color-surface-700)]"
							aria-label="Toggle sort direction"
							title={sortDirectionLabel()}
						>
							{sortDirectionArrow()}
						</button>
					</div>
					{#if showSortMenu}
						<div class="absolute right-0 top-full mt-2 w-48 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded-lg shadow-lg z-40 py-1">
							{#each sortOptions as option}
								<button
									onclick={() => setSort(option.value)}
									class="flex w-full items-center justify-between px-4 py-2 text-left hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] {sortBy === option.value ? 'text-[var(--color-primary-400)]' : ''}"
								>
									<span>{option.label}</span>
									{#if sortBy === option.value}
										<span class="text-xs">✓</span>
									{/if}
								</button>
							{/each}
						</div>
					{/if}
				</div>

					<button
						onclick={() => showFilterPanel = !showFilterPanel}
						class="relative col-start-4 row-start-1 inline-flex h-10 items-center rounded-lg bg-[var(--color-surface-overlay)] hover:bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] font-medium transition-colors {compactToolbar ? 'w-10 justify-center px-0 hover:text-[var(--color-primary-400)]' : 'px-3 sm:px-4'}"
					>
						<svg class="w-4 h-4 {compactToolbar ? '' : 'mr-2'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z"></path>
						</svg>
						<span class={compactToolbar ? 'hidden' : 'hidden sm:inline'}>Filter</span>
						{#if getActiveFilters().length > 0}
							<span class={compactToolbar
								? 'absolute -right-1 -top-1 min-w-5 rounded-full bg-[var(--color-primary-500)] px-1.5 py-0.5 text-center text-[10px] leading-none text-white'
								: 'ml-2 px-2 py-0.5 text-xs rounded-full bg-[var(--color-primary-500)] text-white'}
							>
								{getActiveFilters().length}
							</span>
						{/if}
				</button>

			</div>
		</div>

		{#if getActiveFilters().length > 0}
			<div class="mt-3 flex items-center flex-wrap gap-2">
				<span class="text-sm text-[var(--color-surface-text-muted)]">Active filters:</span>
				{#each getActiveFilters() as filter}
					<button
						onclick={() => removeFilter(filter.key, filter.value)}
						class="active-filter-chip inline-flex items-center rounded-full px-3 py-1 text-sm transition-colors"
					>
						{filter.label}
						<svg class="w-3 h-3 ml-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
						</svg>
					</button>
				{/each}
				<button
					onclick={clearAllFilters}
					class="text-sm text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] underline"
				>
					Clear all
				</button>
			</div>
		{/if}
	</div>
	<FilterPanel
		authors={availableAuthors}
		series={availableSeries}
		tags={availableTags}
		formats={availableFormats}
		filterMode={getFilterMode()}
		valueFilterMode={getValueFilterMode()}
		hasActiveFilters={hasFilterPanelFilters()}
		onClose={() => showFilterPanel = false}
		onClear={clearFilterPanelFilters}
		onSetFilterMode={setFilterMode}
		onSetValueFilterMode={setValueFilterMode}
		onToggleAuthor={applyAuthorFilter}
		onToggleSeries={applySeriesFilter}
		onToggleTag={toggleTagSelection}
		onToggleFormat={applyFormatFilter}
		onToggleStatus={applyStatusFilter}
		isAuthorSelected={isAuthorSelected}
		isSeriesSelected={isSeriesSelected}
		isTagSelected={isTagSelected}
		isFormatSelected={isFormatSelected}
		isStatusSelected={isStatusSelected}
		open={showFilterPanel}
		showBackdrop={filterPanelBackdrop}
	/>
	<div class="px-6 pt-6 {showFilterPanel ? 'lg:pr-[var(--filter-panel-offset)]' : ''}">
	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if books.length === 0}
		<div class="text-center py-16 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
			<svg class="w-16 h-16 text-[var(--color-primary-400)] mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
			</svg>
			{#if getActiveFilters().length > 0}
				<h3 class="text-lg font-medium text-[var(--color-surface-text)] mb-2">No books match your filters</h3>
				<p class="text-[var(--color-surface-text-muted)] mb-4">Try adjusting or clearing your filters</p>
				<button onclick={clearAllFilters} class="px-4 py-2 bg-[var(--color-primary-500)] text-white rounded-lg hover:bg-[var(--color-primary-600)]">
					Clear Filters
				</button>
			{:else}
				<h3 class="text-lg font-medium text-[var(--color-surface-text)] mb-2">
					{libraryFilter ? `No books in ${libraryName || 'library'}` : 'No books in library'}
				</h3>
				<p class="text-[var(--color-surface-text-muted)] mb-4">
					{libraryFilter ? 'Scan to find books in this library' : 'Configure your library paths in settings, then scan.'}
				</p>
				{#if libraryFilter}
					<button onclick={() => scanLibrary()} disabled={scanning} class="accent-action rounded-lg px-4 py-2 font-medium transition-colors">
						Scan Now
					</button>
				{/if}
			{/if}
		</div>
	{:else}
		<div class={viewMode === 'grid' ? 'grid' : 'space-y-4'} style={gridStyle}>
			{#each books as book}
				{#if viewMode === 'grid'}
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
						onkeydown={handleBookKeydown}
						role="button"
						tabindex="0"
					>
					<div class="block">
							<div class="relative mb-2 overflow-hidden rounded-lg" role="button" tabindex="0" onclick={(event) => handleCoverTap(event, book.id)} onkeydown={(event) => handleCoverKeydown(event, book.id)}>
								<BookCoverFrame
									src={book.cover_path ? getCoverThumbUrl(book.id, libraryCoverThumbSize, book.cover_updated_on) : null}
									alt={book.title}
									format={book.format}
									mode="cover"
									frameClass="aspect-[2/3]"
									imageClass="group-hover:scale-105 transition-transform"
									placeholderSize="md"
								/>
								{#if book.opened && book.percent > 0}
									<div class="absolute bottom-0 left-0 right-0 z-10 h-1 bg-slate-700">
										<div class="h-full bg-[var(--color-primary-500)] transition-all duration-300" style="width: {book.percent}%"></div>
									</div>
								{/if}
								{#if formatOnCover && book.format}
									{@const formatColor = getFormatColor(book.format)}
									<div
										class="absolute right-2 top-2 z-10 px-1.5 py-0.5 rounded text-[10px] font-medium uppercase border border-black/20 shadow-[0_1px_2px_rgba(0,0,0,0.35)]"
										style="background-color: {formatColor.bg}; color: {formatColor.text};"
									>
										{book.format}
									</div>
								{/if}
								{#if book.status === 'reading' || book.status === 'finished'}
									<span class="absolute bottom-2 right-2 z-20 h-3 w-3 rounded-full border border-black/30 {statusDot(book.status)} shadow-[0_1px_2px_rgba(0,0,0,0.35)]"></span>
								{/if}
								<div class="cover-action-overlay {activeBookActions === book.id ? 'is-active' : ''}">
									<a href="/book/{book.id}" class="cover-action-button top-action" aria-label="View details for {book.title || 'book'}" onclick={(event) => openBookDetailFromList(event, book.id)}>
										<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 11v5m0-8h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
										</svg>
									</a>
									{#if bulkSelectMode}
										<button type="button" class="cover-action-button bottom-action opacity-45 cursor-not-allowed" aria-label="Opening is disabled during bulk selection" onclick={(event) => event.stopPropagation()}>
											<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
												<path d="M8 5.5v13l10-6.5-10-6.5z"></path>
											</svg>
										</button>
									{:else}
										<a href={getReaderUrl(book)} class="cover-action-button bottom-action" aria-label={getReaderActionLabel(book)} onclick={(event) => event.stopPropagation()}>
											<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
												<path d="M8 5.5v13l10-6.5-10-6.5z"></path>
											</svg>
										</a>
									{/if}
								</div>
							</div>
							<a href="/book/{book.id}" class="block" onclick={(event) => openBookDetailFromList(event, book.id)}>
								<h3 class="text-sm font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
							{#if book.authors && book.authors !== '[]'}
								<p class="text-xs text-[var(--color-surface-text-muted)] truncate">{parseAuthors(book.authors)}</p>
							{/if}
						</a>
						</div>
						<!-- Checkbox for selection - visible in bulk mode or on hover -->
						<button
							onclick={(e) => toggleBookSelection(book.id, e)}
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
					<!-- List view -->
					<div
						class="relative group"
						data-book-id={book.id}
						onclick={handleBookClick}
						onmousedown={handleMouseDown}
						onmouseup={handleMouseUp}
						onmouseleave={handleMouseUp}
						ontouchstart={handleTouchStart}
						ontouchmove={handleTouchMove}
						ontouchend={handleTouchEnd}
						onkeydown={handleBookKeydown}
						role="button"
						tabindex="0"
					>
						<a href="/book/{book.id}" class="block bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] {selectedBooks.has(book.id) ? 'border-[var(--color-primary-500)]' : ''} p-4 hover:border-[var(--color-primary-500)]/50 transition-colors" onclick={(event) => openBookDetailFromList(event, book.id)}>
							<div class="flex items-center space-x-4">
								<BookCoverFrame
									src={book.cover_path ? getCoverThumbUrl(book.id, 'small', book.cover_updated_on) : null}
									alt={book.title}
									format={book.format}
									mode="cover"
									frameClass="w-12 h-16 flex-shrink-0"
									imageClass="object-cover"
									placeholderSize="sm"
								/>
								<div class="flex-1 min-w-0">
									<div class="flex items-center space-x-2 mb-1">
										<h3 class="text-lg font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
										{#if book.status === 'reading' || book.status === 'finished'}
											<span class="w-2.5 h-2.5 rounded-full {statusDot(book.status)} flex-shrink-0"></span>
										{/if}
									</div>
									{#if book.authors && book.authors !== '[]'}
										<p class="text-sm text-[var(--color-surface-text-muted)] mb-1">{parseAuthors(book.authors)}</p>
									{/if}
									{#if book.status === 'reading' && book.percent > 0}
										<div class="w-full bg-[var(--color-surface-700)] rounded-full h-1.5 mb-1">
											<div class="bg-[var(--color-primary-500)] h-1.5 rounded-full transition-all duration-300" style="width: {book.percent}%"></div>
										</div>
										<p class="text-xs text-[var(--color-surface-text-muted)]">{Math.round(book.percent)}% complete</p>
									{:else if book.status === 'finished'}
										<p class="text-xs text-[var(--color-primary-500)]">Finished</p>
									{/if}
								</div>
							</div>
						</a>
						<!-- Checkbox for selection - visible in bulk mode or on hover -->
						<button
							onclick={(e) => toggleBookSelection(book.id, e)}
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

		<!-- Infinite scroll trigger -->
		<div bind:this={loadMoreTrigger} class="h-10 flex items-center justify-center">
			{#if loadingMore}
				<svg class="animate-spin h-6 w-6 text-[var(--color-primary-500)]" fill="none" viewBox="0 0 24 24">
					<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
					<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
				</svg>
			{:else if hasMore}
				<div class="flex flex-col items-center space-y-2">
					<span class="text-[var(--color-surface-text-muted)] text-sm">Scroll for more books</span>
					<button
						onclick={loadMore}
						class="px-4 py-2 bg-[var(--color-primary-500)] text-white rounded-lg hover:bg-[var(--color-primary-600)] transition-colors text-sm"
					>
						Load More Books
					</button>
				</div>
			{:else}
				<span class="text-[var(--color-surface-text-muted)] text-sm">All books loaded</span>
			{/if}
		</div>
	{/if}
	</div>
 </div>

 <!-- Bulk Actions Panel -->
 {#if showBulkPanel}
	<div class="fixed bottom-0 left-0 right-0 z-50 animate-slide-up">
		<div class="bg-[var(--color-surface-overlay)] backdrop-blur-lg border-t border-[var(--color-surface-border)] shadow-2xl">
			<div class="max-w-7xl mx-auto px-4 py-3">
				<div class="flex items-center justify-between gap-4">
					<div class="flex items-center space-x-4">
						<span class="text-[var(--color-surface-text)] font-medium">
							{getSelectionCount()} selected
							{#if selectAllMode === 'filtered'}
								<span class="text-xs text-[var(--color-surface-text-muted)]">(all {totalBooks} in filter)</span>
							{:else if getHiddenSelectedCount() > 0}
								<span class="text-xs text-[var(--color-surface-text-muted)]">({getVisibleSelectedCount()} visible / {getHiddenSelectedCount()} hidden by filters)</span>
							{/if}
						</span>
						<div class="flex items-center space-x-2">
							<button
								onclick={selectAllPage}
								class="px-3 py-1.5 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]"
							>
								Select All on Page
							</button>
							{#if totalBooks > 0 && selectAllMode !== 'filtered'}
								<button
									onclick={selectAllFiltered}
									class="accent-action rounded-lg px-3 py-1.5 text-sm transition-colors"
								>
									Select All {totalBooks} Books
								</button>
							{/if}
							<button
								onclick={deselectAll}
								class="px-3 py-1.5 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]"
							>
								Deselect
							</button>
						</div>
					</div>
					<div class="flex items-center space-x-2">
						<div class="relative">
							<button
								onclick={() => showMetadataMenu = !showMetadataMenu}
								disabled={selectedBooks.size === 0}
								class="px-4 py-2 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] font-medium transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none flex items-center space-x-2"
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
							<button
								onclick={openShelfPicker}
								disabled={actionInProgress}
								class="accent-action flex items-center space-x-2 rounded-lg px-4 py-2 text-sm font-medium transition-colors"
							>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
								</svg>
							<span>Add to Shelf</span>
						</button>
						<button
							onclick={deleteSelectedBooks}
							disabled={actionInProgress}
							class="px-4 py-2 text-sm rounded-lg bg-red-500 hover:bg-red-600 text-white font-medium transition-colors disabled:opacity-50 flex items-center space-x-2"
						>
							{#if actionInProgress}
								<svg class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
									<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
									<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
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

 <!-- Shelf Picker Modal -->
 {#if showShelfPicker}
	<div class="fixed inset-0 z-[60] flex items-center justify-center">
		<button type="button" class="absolute inset-0 bg-black/60" aria-label="Close shelf picker" onclick={() => showShelfPicker = false}></button>
		<div class="relative bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] w-full max-w-md max-h-[80vh] overflow-hidden shadow-2xl">
			<div class="px-6 py-4 border-b border-[var(--color-surface-border)]">
				<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Add to Shelf</h3>
				<p class="text-sm text-[var(--color-surface-text-muted)] mt-1">Add {getSelectionCount()} book(s) to shelf</p>
			</div>
			<div class="p-4 max-h-64 overflow-y-auto">
				{#if manualShelves.length === 0}
					<p class="text-center text-[var(--color-surface-text-muted)] py-4">No manual shelves yet. Create one first.</p>
				{:else}
					<div class="space-y-2">
						{#each manualShelves as shelf}
							<ShelfPickerRow
								{shelf}
								disabled={actionInProgress}
								onSelect={(selectedShelf) => addToShelf(selectedShelf.id)}
							/>
						{/each}
					</div>
				{/if}
			</div>
			<div class="px-6 py-4 border-t border-[var(--color-surface-border)]">
				<button
					type="button"
					onclick={() => showCreateShelfModal = true}
					class="block w-full text-center px-4 py-2 text-sm rounded-lg border border-dashed border-[var(--color-surface-border)] text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] transition-colors"
				>
					+ Create New Shelf
				</button>
			</div>
		</div>
	</div>
	{/if}

	<ShelfModal
		open={showCreateShelfModal}
		mode="create"
		initialMagic={false}
		allowMagicToggle={false}
		onClose={() => showCreateShelfModal = false}
		onSaved={handleShelfCreatedFromPicker}
	/>

	{#if showBulkMetadataEdit}
		<BulkMetadataEditModal
			bookIds={bulkMetadataEditBookIds}
			filterParams={selectAllMode === 'filtered' ? getCurrentFilterParams() : null}
			selectionCount={getSelectionCount()}
			onClose={() => { showBulkMetadataEdit = false; bulkMetadataEditBookIds = []; }}
			onSaved={() => fetchBooks(true)}
		/>
	{/if}

	{#if showBulkMetadataReview && (metadataLookupJob?.id || bulkMetadataLookupBookIds.length > 0)}
		<BulkMetadataReviewModal
			jobId={metadataLookupJob?.id ?? null}
			bookIds={bulkMetadataLookupBookIds}
			initialJob={metadataLookupJob}
			onClose={() => { showBulkMetadataReview = false; bulkMetadataLookupBookIds = []; metadataLookupJob = null; }}
			onApplied={async () => fetchBooks(true)}
		/>
	{/if}

	<LibraryModal
		open={showLibraryModal}
		library={editingLibrary}
		onClose={closeLibraryModal}
		onSaved={handleLibrarySaved}
		onScan={(library) => scanLibrary(library)}
		onRegenerateCovers={(library, mode) => regenerateLibraryCovers(library, mode)}
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

	.cover-action-overlay {
		position: absolute;
		inset: 0;
		z-index: 20;
		opacity: 0;
		pointer-events: none;
		transition: opacity 160ms ease;
	}

	@media (hover: hover) and (pointer: fine) {
		.group:hover .cover-action-overlay {
			opacity: 1;
			pointer-events: auto;
		}
	}

	.cover-action-overlay.is-active {
		opacity: 1;
		pointer-events: auto;
	}

	.cover-action-button {
		position: absolute;
		left: 50%;
		display: inline-flex;
		width: 2.5rem !important;
		height: 2.5rem;
		min-width: 2.5rem;
		padding: 0;
		transform: translateX(-50%);
		align-items: center;
		justify-content: center;
		border-radius: 9999px;
		border: 1px solid var(--color-surface-text-muted);
		background: color-mix(in srgb, var(--color-surface-base) 88%, transparent);
		color: var(--color-surface-text-muted);
		box-shadow: 0 10px 28px rgba(0, 0, 0, 0.38);
		backdrop-filter: blur(8px);
		line-height: 1;
		transition: transform 160ms ease, border-color 160ms ease, color 160ms ease, box-shadow 160ms ease;
	}

	.cover-action-button:hover {
		transform: translateX(-50%) scale(1.06);
		border-color: var(--color-surface-text);
		color: var(--color-surface-text);
		box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-surface-text) 28%, transparent), 0 0 18px color-mix(in srgb, var(--color-surface-text) 32%, transparent), 0 10px 28px rgba(0, 0, 0, 0.38);
	}

	.cover-action-button.top-action {
		top: 18%;
	}

	.cover-action-button.bottom-action {
		bottom: 18%;
	}
 </style>

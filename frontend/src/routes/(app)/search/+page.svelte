<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onDestroy, onMount } from 'svelte';
	import { filterPanelWidth, gridScale, showFormatOnCover, getFormatColor } from '$lib/stores';
	import { restoreRouteScrollPosition, saveRouteScrollPosition } from '$lib/utils/scroll-position';
	import { confirmBulkAction } from '$lib/utils/bulk-confirm';
	import { getBookReaderHref, isAudioFormat } from '$lib/utils/book-formats';
	import { DEFAULT_GRID_SCALE, getResponsiveGridColumns, GRID_SCALE_STEP, MAX_GRID_SCALE, MIN_GRID_SCALE, observeElementWidth } from '$lib/utils/responsive-grid';
	import {
		getBookDetailContextUrl,
		getInlineMetadataEditUrl,
		startMetadataEditContextSession,
		startMetadataEditSession
	} from '$lib/utils/metadata-edit-session';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import FilterPanel from '$lib/components/FilterPanel.svelte';
	import BulkMetadataReviewModal from '$lib/components/BulkMetadataReviewModal.svelte';
	import BulkMetadataEditModal from '$lib/components/BulkMetadataEditModal.svelte';

	type FilterMode = 'AND' | 'OR' | 'NOT';
	type SearchResponse = {
		results?: any[];
		total?: number;
		offset?: number;
		limit?: number;
		has_more?: boolean;
	};

	const BATCH_SIZE = 50;
	const FILTER_PANEL_PARAM_KEYS = ['author', 'series', 'genre', 'genre_mode', 'tags', 'tag_mode', 'format', 'status', 'filter_mode', 'value_filter_mode'];
	const sortOptions = [
		{ value: 'relevance', label: 'Relevance' },
		{ value: 'title', label: 'Title' },
		{ value: 'authors', label: 'Author' },
		{ value: 'series', label: 'Series' },
		{ value: 'added_at', label: 'Date Added' },
		{ value: 'last_read', label: 'Last Read' }
	];

	let query = $state($page.url.searchParams.get('q') || '');
	let libraryFilter = $state($page.url.searchParams.get('library') || '');
	let libraryName = $state('');
	let results = $state<any[]>([]);
	let loading = $state(false);
	let loadingMore = $state(false);
	let totalResults = $state(0);
	let serverHasMore = $state(false);
	let currentOffset = $state(0);
	let viewMode = $state('grid');
	let localGridScale = $state(DEFAULT_GRID_SCALE);
	let gridContainerWidth = $state(0);
	let gridMeasureElement = $state<HTMLDivElement | null>(null);
	let sortBy = $state($page.url.searchParams.get('sort') || 'relevance');
	let sortDir = $state<'asc' | 'desc'>($page.url.searchParams.get('sort_dir') === 'desc' ? 'desc' : 'asc');
	let showSortMenu = $state(false);
	let showDisplayMenu = $state(false);
	let showFilterPanel = $state(false);
	let filterPanelBackdrop = $state(false);
	let availableAuthors = $state<any[]>([]);
	let availableSeries = $state<any[]>([]);
	let availableTags = $state<any[]>([]);
	let availableFormats = $state<any[]>([]);
	let selectedBooks = $state<Set<number>>(new Set());
	let showShelfPicker = $state(false);
	let shelves = $state<any[]>([]);
	let actionInProgress = $state(false);
	let showBulkMetadataEdit = $state(false);
	let bulkMetadataEditBookIds = $state<number[]>([]);
	let bulkMetadataLookupBookIds = $state<number[]>([]);
	let showMetadataMenu = $state(false);
	let metadataLookupJob = $state<any | null>(null);
	let showBulkMetadataReview = $state(false);
	let longPressTimer: number | null = null;
	let longPressThreshold = 500;
	let suppressNextClickBookId: number | null = null;
	let longPressTouchStart: { x: number; y: number } | null = null;
	const LONG_PRESS_MOVE_TOLERANCE = 10;
	let lastUrlSearch = '';
	let bulkSelectionAnchorId = $state<number | null>(null);
	let loadMoreTrigger = $state<HTMLDivElement | null>(null);
	let observer: IntersectionObserver | null = null;
	let formatOnCover = $state(true);
	let activeBookActions = $state<number | null>(null);

	let bulkSelectMode = $derived(selectedBooks.size > 0);
	let hasMore = $derived(serverHasMore);
	let responsiveGridColumns = $derived(getResponsiveGridColumns(gridContainerWidth, localGridScale));
	let gridStyle = $derived(viewMode === 'grid'
		? `grid-template-columns: repeat(${responsiveGridColumns}, minmax(0, 1fr))`
		: '');

	function updateGridScale(value: number) {
		gridScale.set(value);
	}

	function parseAuthors(authors: string): string {
		try {
			const parsed = JSON.parse(authors);
			return Array.isArray(parsed) ? parsed.join(', ') : authors;
		} catch {
			return authors;
		}
	}

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

	function getFilterMode(): FilterMode {
		const mode = ($page.url.searchParams.get('filter_mode') || 'AND').toUpperCase();
		return mode === 'OR' || mode === 'NOT' ? mode : 'AND';
	}

	function getValueFilterMode(): FilterMode {
		const mode = ($page.url.searchParams.get('value_filter_mode') || 'OR').toUpperCase();
		return mode === 'AND' || mode === 'NOT' ? mode : 'OR';
	}

	function navigateWithSearchParams(url: URL, replaceState: boolean = false) {
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
		} catch (error) {
			console.error('Failed to fetch library name:', error);
		}
	}

	function buildSearchUrl(offset: number = 0, limit: number = BATCH_SIZE): string {
		const params = new URLSearchParams();
		params.set('q', query.trim());
		params.set('offset', String(offset));
		params.set('limit', String(limit));
		if (libraryFilter) params.set('library_id', libraryFilter);
		for (const author of getQueryValues($page.url.searchParams, 'author')) params.append('author', author);
		for (const seriesName of getQueryValues($page.url.searchParams, 'series')) params.append('series', seriesName);
		const genres = getQueryValues($page.url.searchParams, 'genre', true);
		const tags = getQueryValues($page.url.searchParams, 'tags', true);
		if (genres.length > 0) params.set('genre', genres.join(','));
		if (tags.length > 0) params.set('tags', tags.join(','));
		for (const format of getQueryValues($page.url.searchParams, 'format')) params.append('format', format);
		for (const status of getQueryValues($page.url.searchParams, 'status')) params.append('status', status);
		if (getFilterMode() !== 'AND') params.set('filter_mode', getFilterMode());
		if (getValueFilterMode() !== 'OR') params.set('value_filter_mode', getValueFilterMode());
		params.set('sort', sortBy);
		params.set('sort_dir', sortDir);
		return `/api/search?${params.toString()}`;
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

	$effect(() => {
		const currentSearch = $page.url.search;
		if (currentSearch === lastUrlSearch) return;
		lastUrlSearch = currentSearch;

		query = $page.url.searchParams.get('q') || '';
		libraryFilter = $page.url.searchParams.get('library') || '';
		sortBy = $page.url.searchParams.get('sort') || 'relevance';
		sortDir = $page.url.searchParams.get('sort_dir') === 'desc' ? 'desc' : 'asc';
		if (libraryFilter) {
			fetchLibraryName();
		} else {
			libraryName = '';
		}
		void fetchFilterOptions();

		if (query) {
			search(true);
		} else {
			results = [];
			totalResults = 0;
			currentOffset = 0;
			serverHasMore = false;
			if (observer) observer.disconnect();
			deselectAll();
		}
	});

	onMount(() => {
		gridScale.init();
		showFormatOnCover.init();
		void fetchFilterOptions();
		const filterPanelBackdropMedia = window.matchMedia('(max-width: 1023px)');
		const updateFilterPanelBackdrop = (event: MediaQueryListEvent | MediaQueryList) => {
			filterPanelBackdrop = event.matches;
		};
		updateFilterPanelBackdrop(filterPanelBackdropMedia);
		filterPanelBackdropMedia.addEventListener('change', updateFilterPanelBackdrop);

		return () => {
			filterPanelBackdropMedia.removeEventListener('change', updateFilterPanelBackdrop);
		};
	});

	$effect(() => {
		const unsub = gridScale.subscribe((value: number) => localGridScale = value);
		return unsub;
	});

	$effect(() => {
		const unsub = showFormatOnCover.subscribe((value: boolean) => formatOnCover = value);
		return unsub;
	});

	$effect(() => {
		if (!gridMeasureElement) return;
		return observeElementWidth(gridMeasureElement, (width) => gridContainerWidth = width);
	});

	$effect(() => {
		if (loadMoreTrigger && results.length > 0 && hasMore) {
			setTimeout(() => setupObserver(), 100);
		}
	});

	onDestroy(() => {
		if (observer) {
			observer.disconnect();
			observer = null;
		}
	});

	async function search(reset: boolean = true) {
		const trimmed = query.trim();
		if (!trimmed) {
			results = [];
			totalResults = 0;
			currentOffset = 0;
			serverHasMore = false;
			if (observer) observer.disconnect();
			selectedBooks = new Set();
			return;
		}

		if (reset) {
			loading = true;
			loadingMore = false;
			currentOffset = 0;
			selectedBooks = new Set();
			showMetadataMenu = false;
			if (observer) observer.disconnect();
		} else {
			if (loading || loadingMore || !hasMore) return;
			loadingMore = true;
		}
		try {
			const res = await fetch(buildSearchUrl(reset ? 0 : currentOffset));
			if (res.ok) {
				const data: SearchResponse | any[] = await res.json();
				const pageResults = Array.isArray(data) ? data : data.results ?? [];
				if (reset) {
					results = pageResults;
				} else {
					results = [...results, ...pageResults];
				}
				totalResults = Array.isArray(data) ? results.length : data.total ?? results.length;
				serverHasMore = Array.isArray(data)
					? false
					: typeof data.has_more === 'boolean'
						? data.has_more
						: results.length < totalResults;
				currentOffset = results.length;
			} else {
				if (reset) {
					results = [];
					totalResults = 0;
					currentOffset = 0;
					serverHasMore = false;
				}
			}
		} catch (error) {
			console.error('Search failed:', error);
		} finally {
			if (reset) {
				loading = false;
				restoreRouteScrollPosition();
			} else {
				loadingMore = false;
			}
			setTimeout(() => {
				if (loadMoreTrigger && hasMore) setupObserver();
			}, 200);
		}
	}

	function loadMore() {
		void search(false);
	}

	function setupObserver() {
		if (observer) observer.disconnect();
		if (loadMoreTrigger && hasMore) {
			observer = new IntersectionObserver(
				(entries) => {
					if (entries[0].isIntersecting && !loadingMore && !loading) {
						loadMore();
					}
				},
				{ rootMargin: '200px' }
			);
			observer.observe(loadMoreTrigger);
		}
	}

	function submitSearch() {
		const url = new URL($page.url);
		if (query.trim()) {
			url.searchParams.set('q', query.trim());
		} else {
			url.searchParams.delete('q');
		}
		url.searchParams.set('sort', sortBy);
		url.searchParams.set('sort_dir', sortDir);
		const nextSearch = url.search;
		if (nextSearch === $page.url.search) {
			void search(true);
		} else {
			navigateWithSearchParams(url);
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			submitSearch();
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
		} catch (error) {
			console.error('Failed to fetch filter options:', error);
		}
	}

	function currentSortLabel() {
		return sortOptions.find((option) => option.value === sortBy)?.label ?? 'Relevance';
	}

	function sortDirectionLabel() {
		return sortDir === 'asc' ? 'Ascending' : 'Descending';
	}

	function sortDirectionArrow() {
		return sortDir === 'asc' ? '↑' : '↓';
	}

	function updateSortParams(nextSortBy = sortBy, nextSortDir = sortDir) {
		const url = new URL($page.url);
		url.searchParams.set('sort', nextSortBy);
		url.searchParams.set('sort_dir', nextSortDir);
		navigateWithSearchParams(url);
	}

	function setSort(value: string) {
		sortBy = value;
		showSortMenu = false;
		updateSortParams(value, sortDir);
	}

	function toggleSortDirection() {
		const nextDir = sortDir === 'asc' ? 'desc' : 'asc';
		sortDir = nextDir;
		updateSortParams(sortBy, nextDir);
	}

	function getActiveFilters(): { key: string; value: string; label: string }[] {
		const filters: { key: string; value: string; label: string }[] = [];
		const params = $page.url.searchParams;

		for (const author of getQueryValues(params, 'author')) filters.push({ key: 'author', value: author, label: `Author: ${author}` });
		for (const seriesName of getQueryValues(params, 'series')) filters.push({ key: 'series', value: seriesName, label: `Series: ${seriesName}` });
		for (const genre of getQueryValues(params, 'genre', true)) filters.push({ key: 'genre', value: genre, label: `Tag: ${genre}` });
		for (const tag of getQueryValues(params, 'tags', true)) filters.push({ key: 'tags', value: tag, label: `Tag: ${tag}` });
		for (const format of getQueryValues(params, 'format')) filters.push({ key: 'format', value: format, label: `Format: ${format.toUpperCase()}` });
		for (const status of getQueryValues(params, 'status')) filters.push({ key: 'status', value: status, label: `Status: ${status}` });

		return filters;
	}

	function removeFilter(key: string, value: string) {
		const url = new URL($page.url);
		if (key === 'genre' || key === 'tags') {
			const values = getQueryValues(url.searchParams, key, true).filter((item) => item !== value);
			url.searchParams.delete(key);
			if (values.length > 0) url.searchParams.set(key, values.join(','));
		} else {
			const values = getQueryValues(url.searchParams, key).filter((item) => item !== value);
			url.searchParams.delete(key);
			for (const item of values) url.searchParams.append(key, item);
		}
		navigateWithSearchParams(url);
	}

	function clearAllFilters() {
		const url = new URL($page.url);
		for (const key of FILTER_PANEL_PARAM_KEYS) url.searchParams.delete(key);
		navigateWithSearchParams(url);
	}

	function hasFilterPanelFilters(): boolean {
		return FILTER_PANEL_PARAM_KEYS.some((key) => $page.url.searchParams.has(key));
	}

	function clearFilterPanelFilters() {
		const url = new URL($page.url);
		for (const key of FILTER_PANEL_PARAM_KEYS) url.searchParams.delete(key);
		navigateWithSearchParams(url, true);
	}

	function toggleRepeatedFilter(key: string, value: string) {
		const url = new URL($page.url);
		const values = getQueryValues(url.searchParams, key);
		const nextValues = values.includes(value)
			? values.filter((item) => item !== value)
			: [...values, value];

		url.searchParams.delete(key);
		for (const item of nextValues) url.searchParams.append(key, item);
		navigateWithSearchParams(url);
	}

	function setFilterMode(mode: FilterMode) {
		const url = new URL($page.url);
		if (mode === 'AND') {
			url.searchParams.delete('filter_mode');
		} else {
			url.searchParams.set('filter_mode', mode);
		}
		navigateWithSearchParams(url, true);
	}

	function setValueFilterMode(mode: FilterMode) {
		const url = new URL($page.url);
		if (mode === 'OR') {
			url.searchParams.delete('value_filter_mode');
		} else {
			url.searchParams.set('value_filter_mode', mode);
		}
		navigateWithSearchParams(url, true);
	}

	function toggleTagSelection(tagName: string) {
		const url = new URL($page.url);
		const tagList = getQueryValues(url.searchParams, 'tags', true);
		const newTags = tagList.includes(tagName)
			? tagList.filter((tag) => tag !== tagName)
			: [...tagList, tagName];

		url.searchParams.delete('tags');
		if (newTags.length > 0) url.searchParams.set('tags', newTags.join(','));
		url.searchParams.delete('tag_mode');
		navigateWithSearchParams(url);
	}

	function isAuthorSelected(authorName: string): boolean {
		return getQueryValues($page.url.searchParams, 'author').includes(authorName);
	}

	function isSeriesSelected(seriesName: string): boolean {
		return getQueryValues($page.url.searchParams, 'series').includes(seriesName);
	}

	function isTagSelected(tagName: string): boolean {
		return getQueryValues($page.url.searchParams, 'tags', true).includes(tagName);
	}

	function isStatusSelected(status: string): boolean {
		return getQueryValues($page.url.searchParams, 'status').includes(status);
	}

	function isFormatSelected(format: string): boolean {
		return getQueryValues($page.url.searchParams, 'format').includes(format);
	}

	function getVisibleBookIds(): number[] {
		return results.map((book) => book.id);
	}

	function openBookDetailFromList(event: MouseEvent, bookId: number) {
		saveRouteScrollPosition();
		event.stopPropagation();
		if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) return;
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

	function getReaderUrl(book: any): string {
		return getBookReaderHref(book.id, book.format, '/search');
	}

	function getReaderActionLabel(book: any): string {
		const title = book.title || 'book';
		return isAudioFormat(book.format) ? `Play audio for ${title}` : `Read ${title}`;
	}

	function statusDot(status: string) {
		switch (status) {
			case 'reading': return 'bg-blue-500';
			case 'finished': return 'bg-emerald-500';
			default: return '';
		}
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
		selectedBooks = new Set(results.map((book) => book.id));
	}

	function deselectAll() {
		selectedBooks = new Set();
		bulkSelectionAnchorId = null;
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
		if (!confirmBulkAction({ action: 'add {count} books to this shelf', count: selectedBooks.size })) return;
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
		if (!confirmBulkAction({ action: 'delete', count: selectedBooks.size, destructive: true, alwaysConfirm: true })) return;

		actionInProgress = true;
		try {
			const res = await fetch('/api/books/bulk-delete', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ book_ids: Array.from(selectedBooks) })
			});

			if (res.ok) {
				await search(true);
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

</script>

<div class="space-y-6 transition-all duration-300 {showFilterPanel ? 'lg:pr-[var(--filter-panel-offset)]' : ''}" style={`--filter-panel-offset: ${$filterPanelWidth + 24}px;`}>
	<div class="max-w-2xl mx-auto">
		<h1 class="text-2xl font-bold text-[var(--color-surface-text)] mb-2">Search</h1>
		{#if libraryName}
			<p class="mb-4 text-sm text-[var(--color-surface-text-muted)]">Scoped to {libraryName}</p>
		{/if}
		<div class="flex flex-col gap-3 sm:flex-row">
			<input
				type="text"
				bind:value={query}
				onkeydown={handleKeydown}
				placeholder="Search books by title, author, or description..."
				class="min-w-0 flex-1 px-4 py-3 border border-[var(--color-surface-border)] rounded-lg bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:ring-2 focus:ring-[var(--color-primary-500)] focus:border-transparent"
			>
			<button
				onclick={submitSearch}
				disabled={loading}
				class="px-6 py-3 bg-[var(--color-primary-500)] text-white rounded-lg hover:bg-[var(--color-primary-600)] disabled:opacity-50 transition-colors"
			>
				{loading ? 'Searching...' : 'Search'}
			</button>
		</div>
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
		onToggleAuthor={(value) => toggleRepeatedFilter('author', value)}
		onToggleSeries={(value) => toggleRepeatedFilter('series', value)}
		onToggleTag={toggleTagSelection}
		onToggleFormat={(value) => toggleRepeatedFilter('format', value)}
		onToggleStatus={(value) => toggleRepeatedFilter('status', value)}
		isAuthorSelected={isAuthorSelected}
		isSeriesSelected={isSeriesSelected}
		isTagSelected={isTagSelected}
		isFormatSelected={isFormatSelected}
		isStatusSelected={isStatusSelected}
		open={showFilterPanel}
		showBackdrop={filterPanelBackdrop}
	/>

	{#if results.length > 0}
		<div bind:this={gridMeasureElement} class="w-full">
			<div class="mb-6 flex min-w-0 flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
				<div class="min-w-0">
					<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">
						{#if totalResults > results.length}
							{results.length} of {totalResults} result{totalResults === 1 ? '' : 's'} shown
						{:else}
							{results.length} result{results.length === 1 ? '' : 's'} found
						{/if}
					</h2>
				</div>

				<div class="grid grid-cols-[auto_auto_2.5rem_2.5rem] items-center gap-2 lg:justify-end">
					<div class="relative">
						{#if showSortMenu}
							<button
								type="button"
								class="fixed inset-0 z-20"
								aria-label="Close sort menu"
								onclick={() => showSortMenu = false}
							></button>
						{/if}
						<div class="inline-flex h-10 max-w-36 overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] sm:max-w-48">
							<button
								onclick={() => showSortMenu = !showSortMenu}
								aria-label="Sort results"
								class="inline-flex min-w-0 items-center px-3 font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)] sm:px-4"
							>
								<span class="hidden sm:inline">Sort</span>
								<span class="ml-0 hidden min-w-0 truncate text-sm text-[var(--color-surface-text-muted)] sm:ml-2 md:inline">{currentSortLabel()}</span>
								<svg class="ml-2 h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
							<div class="absolute right-0 top-full z-40 mt-2 w-48 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-1 shadow-lg">
								{#each sortOptions as option}
									<button
										onclick={() => setSort(option.value)}
										class="flex w-full items-center justify-between px-4 py-2 text-left text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)] {sortBy === option.value ? 'text-[var(--color-primary-400)]' : ''}"
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
						class="inline-flex h-10 items-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)] sm:px-4"
					>
						<svg class="mr-2 h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z"></path>
						</svg>
						<span class="hidden sm:inline">Filter</span>
						{#if getActiveFilters().length > 0}
							<span class="ml-2 rounded-full bg-[var(--color-primary-500)] px-2 py-0.5 text-xs text-white">
								{getActiveFilters().length}
							</span>
						{/if}
					</button>

					<button
						type="button"
						onclick={() => viewMode = viewMode === 'grid' ? 'list' : 'grid'}
						class="inline-flex h-10 w-10 items-center justify-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)] hover:text-[var(--color-primary-400)]"
						aria-label={viewMode === 'grid' ? 'Switch to list view' : 'Switch to grid view'}
						title={viewMode === 'grid' ? 'Switch to list view' : 'Switch to grid view'}
					>
						{#if viewMode === 'grid'}
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16"></path>
							</svg>
						{:else}
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z"></path>
							</svg>
						{/if}
					</button>

					<div class="relative">
						{#if showDisplayMenu}
							<button
								type="button"
								class="fixed inset-0 z-20"
								aria-label="Close display menu"
								onclick={() => showDisplayMenu = false}
							></button>
						{/if}
						<button
							type="button"
							onclick={() => showDisplayMenu = !showDisplayMenu}
							aria-label="Display settings"
							class="inline-flex h-10 w-10 items-center justify-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)]"
						>
							<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
							</svg>
						</button>
						{#if showDisplayMenu}
							<div class="absolute right-0 top-full z-40 mt-2 w-56 max-w-[calc(100vw-1.5rem)] rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4 shadow-lg">
								{#if viewMode === 'grid'}
									<div>
										<div class="mb-2 flex items-center justify-between gap-3">
											<label class="text-sm font-medium text-[var(--color-surface-text)]" for="search-grid-scale">Grid Scale</label>
											<span class="text-xs text-[var(--color-surface-text-muted)]">{responsiveGridColumns} cols</span>
										</div>
										<div class="flex items-center gap-2">
											<input
												id="search-grid-scale"
												type="range"
												min={MIN_GRID_SCALE}
												max={MAX_GRID_SCALE}
												step={GRID_SCALE_STEP}
												value={localGridScale}
												oninput={(event) => updateGridScale(Number(event.currentTarget.value))}
												class="flex-1"
											>
											<span class="w-12 text-right text-sm text-[var(--color-surface-text)]">{localGridScale}%</span>
										</div>
									</div>
								{:else}
									<p class="text-sm text-[var(--color-surface-text-muted)]">Grid scale is available in grid view.</p>
								{/if}
							</div>
						{/if}
					</div>
				</div>
			</div>

			{#if getActiveFilters().length > 0}
				<div class="mb-6 flex flex-wrap items-center gap-2">
					<span class="text-sm text-[var(--color-surface-text-muted)]">Active filters:</span>
					{#each getActiveFilters() as filter}
						<button
							onclick={() => removeFilter(filter.key, filter.value)}
							class="inline-flex items-center rounded-full border border-[var(--color-primary-500)]/50 bg-[var(--color-primary-500)]/20 px-3 py-1 text-sm text-[var(--color-primary-300)] transition-colors hover:bg-[var(--color-primary-500)]/30"
						>
							{filter.label}
							<svg class="ml-1.5 h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
							</svg>
						</button>
					{/each}
					<button
						onclick={clearAllFilters}
						class="text-sm text-[var(--color-surface-text-muted)] underline hover:text-[var(--color-surface-text)]"
					>
						Clear all
					</button>
				</div>
			{/if}

			<div class={viewMode === 'grid' ? 'grid gap-4' : 'space-y-4'} style={viewMode === 'grid' ? gridStyle : ''}>
				{#each results as book}
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
							ontouchcancel={handleTouchEnd}
							onkeydown={handleBookKeydown}
							role="button"
							tabindex="0"
						>
							<div class="block">
								<div class="relative mb-2 overflow-hidden rounded-lg" role="button" tabindex="0" onclick={(event) => handleCoverTap(event, book.id)} onkeydown={(event) => handleCoverKeydown(event, book.id)}>
									<BookCoverFrame
										src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
										alt={book.title}
										format={book.format}
										mode="cover"
										frameClass="aspect-[2/3]"
										imageClass="group-hover:scale-105 transition-transform"
										placeholderSize="md"
									/>
									{#if book.status === 'reading' || book.status === 'finished'}
										<span class="absolute bottom-2 right-2 z-20 h-3 w-3 rounded-full border border-black/30 {statusDot(book.status)} shadow-[0_1px_2px_rgba(0,0,0,0.35)]"></span>
									{/if}
									{#if book.opened && book.percent > 0}
										<div class="absolute bottom-0 left-0 right-0 z-10 h-1 bg-slate-700">
											<div class="h-full bg-[var(--color-primary-500)] transition-all duration-300" style="width: {book.percent}%"></div>
										</div>
									{/if}
									{#if formatOnCover && book.format}
										{@const formatColor = getFormatColor(book.format)}
										<div
											class="absolute right-2 top-2 z-10 rounded border border-black/20 px-1.5 py-0.5 text-[10px] font-medium uppercase shadow-[0_1px_2px_rgba(0,0,0,0.35)]"
											style="background-color: {formatColor.bg}; color: {formatColor.text};"
										>
											{book.format}
										</div>
									{/if}
									<div class="cover-action-overlay {activeBookActions === book.id ? 'is-active' : ''}">
										<a href="/book/{book.id}" class="cover-action-button top-action" aria-label="View details for {book.title || 'book'}" onclick={(event) => openBookDetailFromList(event, book.id)}>
											<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 11v5m0-8h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
											</svg>
										</a>
										{#if bulkSelectMode}
											<button type="button" class="cover-action-button bottom-action cursor-not-allowed opacity-45" aria-label="Opening is disabled during bulk selection" onclick={(event) => event.stopPropagation()}>
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
									<h3 class="truncate text-sm font-medium text-[var(--color-surface-text)]">{book.title || 'Untitled'}</h3>
									{#if book.authors && book.authors !== '[]'}
										<p class="truncate text-xs text-[var(--color-surface-text-muted)]">{parseAuthors(book.authors)}</p>
									{/if}
								</a>
							</div>
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
							onmouseleave={handleMouseUp}
							ontouchstart={handleTouchStart}
							ontouchmove={handleTouchMove}
							ontouchend={handleTouchEnd}
							ontouchcancel={handleTouchEnd}
							onkeydown={handleBookKeydown}
							role="button"
							tabindex="0"
						>
							<a href="/book/{book.id}" class="block bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] {selectedBooks.has(book.id) ? 'border-[var(--color-primary-500)]' : ''} p-4 hover:border-[var(--color-primary-500)]/50 transition-colors" onclick={(event) => openBookDetailFromList(event, book.id)}>
								<div class="flex items-center space-x-4">
									<BookCoverFrame
										src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
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
													<span class="h-2.5 w-2.5 flex-shrink-0 rounded-full {statusDot(book.status)}"></span>
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
			{#if hasMore}
				<div bind:this={loadMoreTrigger} class="flex justify-center py-8">
					{#if loadingMore}
						<div class="flex items-center gap-2 text-[var(--color-surface-text-muted)]">
							<svg class="h-5 w-5 animate-spin" fill="none" viewBox="0 0 24 24">
								<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
								<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
							</svg>
							<span>Loading more results...</span>
						</div>
					{:else}
						<button
							type="button"
							onclick={loadMore}
							class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-4 py-2 text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)]"
						>
							Load more results
						</button>
					{/if}
				</div>
			{/if}
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
							<button
								onclick={openShelfPicker}
								disabled={actionInProgress}
								class="accent-action flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-colors"
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

{#if showBulkMetadataEdit}
	<BulkMetadataEditModal
		bookIds={bulkMetadataEditBookIds}
		selectionCount={bulkMetadataEditBookIds.length}
		onClose={() => { showBulkMetadataEdit = false; bulkMetadataEditBookIds = []; }}
		onSaved={() => search(true)}
	/>
{/if}

{#if showBulkMetadataReview && (metadataLookupJob?.id || bulkMetadataLookupBookIds.length > 0)}
	<BulkMetadataReviewModal
		jobId={metadataLookupJob?.id ?? null}
		bookIds={bulkMetadataLookupBookIds}
		initialJob={metadataLookupJob}
		onClose={() => { showBulkMetadataReview = false; bulkMetadataLookupBookIds = []; metadataLookupJob = null; }}
		onApplied={async () => search(true)}
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

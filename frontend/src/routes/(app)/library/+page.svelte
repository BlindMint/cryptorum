<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { gridSize, showFormatOnCover, getFormatColor } from '$lib/stores';
	import { getCoverThumbUrl, getLibraryCoverThumbSize } from '$lib/utils/covers';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import MetadataLookupModal from '$lib/components/MetadataLookupModal.svelte';
	import BulkMetadataReviewModal from '$lib/components/BulkMetadataReviewModal.svelte';

	type FilterMode = 'AND' | 'OR' | 'NOT';
	const FILTER_MODES: FilterMode[] = ['AND', 'OR', 'NOT'];

 	let books = $state<any[]>([]);
  let loading = $state(true);
  let loadingMore = $state(false);
  let scanning = $state(false);
  let scanMessage = $state('');
  let libraryName = $state('');
  let totalBooks = $state(0);
  let hasMore = $derived(books.length < totalBooks);

 	// Display controls
 	let viewMode = $state('grid');
  let localGridSize = $state(4);
  let sortBy = $state('added_at');
  let gridStyle = $derived(viewMode === 'grid' ? `grid-template-columns: repeat(${localGridSize}, minmax(0, 1fr))` : '');
  let libraryCoverThumbSize = $derived(getLibraryCoverThumbSize(localGridSize));
  let showSettingsMenu = $state(false);
 	let formatOnCover = $state(true);

 	$effect(() => {
 		const unsub = gridSize.subscribe((v: number) => localGridSize = v);
 		return unsub;
 	});

 	$effect(() => {
 		const unsub = showFormatOnCover.subscribe((v: boolean) => formatOnCover = v);
 		return unsub;
 	});

 	function updateGridSize(value: number) {
 		localGridSize = value;
 		gridSize.set(value);
 	}

 	function toggleFormatOnCover() {
 		formatOnCover = !formatOnCover;
 		showFormatOnCover.set(formatOnCover);
 	}

	// Filter state
	let showFilterPanel = $state(false);
	let showSortMenu = $state(false);
	let filterAuthorsOpen = $state(true);
	let filterSeriesOpen = $state(true);
	let filterGenresOpen = $state(true);
	let filterTagsOpen = $state(true);
	let filterStatusOpen = $state(true);
	let availableAuthors = $state<any[]>([]);
	let availableSeries = $state<any[]>([]);
	let availableGenres = $state<any[]>([]);
	let availableTags = $state<any[]>([]);
	
 	// Bulk selection state
  	let selectedBooks = $state<Set<number>>(new Set());
  	let showBulkPanel = $state(false);
  	let showMetadataLookup = $state(false);
	let showMetadataMenu = $state(false);
	let metadataLookupQueueing = $state(false);
	let metadataLookupJob = $state<any | null>(null);
	let showBulkMetadataReview = $state(false);
  	let showShelfPicker = $state(false);
  	let shelves = $state<any[]>([]);
  	let actionInProgress = $state(false);
  	let selectAllMode = $state<'none' | 'page' | 'filtered'>('none');
  	let bulkSelectMode = $derived(selectedBooks.size > 0);

  	// Long press state for mobile
   	let longPressTimer: number | null = null;
   	let longPressThreshold = 500; // ms
  	let longPressActivated = $state(false);

 	let libraryFilter = $derived($page.url.searchParams.get('library') || '');
 	let currentOffset = $state(0);
 	const BATCH_SIZE = 50;

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

	function getFilterModeIndex(): number {
		return ['AND', 'OR', 'NOT'].indexOf(getFilterMode());
	}

	function navigateWithFilters(url: URL, replaceState: boolean = false) {
		showFilterPanel = true;
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

	function buildBooksUrl(offset: number = 0): string {
		const params = $page.url.searchParams;
		const authors = getQueryValues(params, 'author');
		const series = getQueryValues(params, 'series');
		const genres = getQueryValues(params, 'genre', true);
		const tags = getQueryValues(params, 'tags', true);
		const statuses = getQueryValues(params, 'status');
		const filterMode = getFilterMode();

		const queryParams = new URLSearchParams();
		queryParams.set('limit', String(BATCH_SIZE));
		queryParams.set('offset', String(offset));

		if (libraryFilter) queryParams.set('library_id', libraryFilter);
		for (const author of authors) queryParams.append('author', author);
		for (const seriesName of series) queryParams.append('series', seriesName);
		if (genres.length > 0) queryParams.set('genre', genres.join(','));
		if (tags.length > 0) queryParams.set('tags', tags.join(','));
		for (const status of statuses) queryParams.append('status', status);
		if (filterMode !== 'AND') queryParams.set('filter_mode', filterMode);

		return '/api/books?' + queryParams.toString();
	}

 	async function fetchBooks(reset: boolean = true) {
 		if (reset) {
 			loading = true;
 			currentOffset = 0;
 		} else {
 			loadingMore = true;
 		}

 		try {
 			const url = buildBooksUrl(reset ? 0 : currentOffset);
 			const res = await fetch(url);
 			if (res.ok) {
 				const data = await res.json();
 				if (data.books) {
 					totalBooks = data.total;
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
  			}, 200);
  		}
 	}

 	async function loadMore() {
 		if (loadingMore || !hasMore) return;
 		await fetchBooks(false);
 	}

	async function fetchFilterOptions() {
		try {
			const [authorsRes, seriesRes, genresRes, tagsRes] = await Promise.all([
				fetch('/api/authors'),
				fetch('/api/series'),
				fetch('/api/metadata/genres'),
				fetch('/api/metadata/tags')
			]);
			if (authorsRes.ok) {
				availableAuthors = await authorsRes.json();
			}
			if (seriesRes.ok) {
				availableSeries = await seriesRes.json();
			}
			if (genresRes.ok) {
				availableGenres = await genresRes.json();
			}
			if (tagsRes.ok) {
				availableTags = await tagsRes.json();
			}
		} catch (e) {
			console.error('Failed to fetch filter options:', e);
		}
	}

 	$effect(() => {
 		const filter = libraryFilter;
 		if (filter !== undefined) {
 			fetchLibraryName();
 			fetchBooks(true);
 		}
 	});

	$effect(() => {
		// Re-fetch when URL params change
		$page.url.search;
		fetchBooks(true);
	});

 	$effect(() => {
  		sortBy;
  		if (books.length > 0) {
  			sortBooks();
  		}
  	});

 	$effect(() => {
    		showBulkPanel = selectedBooks.size > 0;
   	});

	let loadMoreTrigger = $state<HTMLDivElement | null>(null);
  	let observer: IntersectionObserver | null = null;

   	function setupObserver() {
   		// Clean up existing observer
   		if (observer) {
   			console.log('Disconnecting existing observer');
   			observer.disconnect();
   			observer = null;
   		}

   		// Only set up observer if we have more content to load
		if (loadMoreTrigger && hasMore) {
   			console.log('Setting up intersection observer, hasMore:', hasMore, 'books count:', books.length);
   			observer = new IntersectionObserver(
   				(entries) => {
   					const entry = entries[0];
   					console.log('Intersection observer triggered:', {
   						isIntersecting: entry.isIntersecting,
   						intersectionRatio: entry.intersectionRatio,
   						hasMore,
   						loadingMore,
   						booksCount: books.length
   					});
   					if (entry.isIntersecting && hasMore && !loadingMore) {
   						console.log('Loading more books...');
   						loadMore();
   					}
   				},
   				{ threshold: 0.3, rootMargin: '150px' }
   			);
			observer.observe(loadMoreTrigger);
   			console.log('Observer connected to trigger element');
   		} else {
   			console.log('Not setting up observer - loadMoreTrigger:', !!loadMoreTrigger, 'hasMore:', hasMore);
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
 		fetchLibraryName();
 		fetchBooks(true);
 		fetchFilterOptions();
 		showFormatOnCover.init();

 		// Set initial grid size based on viewport
 		const updateGridSizeForViewport = () => {
 			const width = window.innerWidth;
 			if (width >= 1024) {
 				localGridSize = 7;
 			} else if (width >= 768) {
 				localGridSize = 5;
 			} else {
 				localGridSize = 4;
 			}
 			gridSize.set(localGridSize);
 		};
 		updateGridSizeForViewport();

 		return () => {
 			if (observer) observer.disconnect();
 		};
 	});

	async function scanLibrary() {
		scanning = true;
		scanMessage = 'Scanning...';
		try {
			const res = libraryFilter
				? await fetch(`/api/libraries/${libraryFilter}/scan`, { method: 'POST' })
				: await fetch('/api/scan', { method: 'POST' });
			const data = await res.json().catch(() => ({}));
			if (res.ok) {
				scanMessage = 'Scan started. Refreshing in 5s...';
				setTimeout(async () => {
 					await fetchBooks(true);
 					scanMessage = '';
 					scanning = false;
 				}, 5000);
 			} else {
 				scanMessage = `Scan failed: ${data.error || 'Unknown error'}`;
 				scanning = false;
 			}
 		} catch (e) {
 			scanMessage = 'Scan failed. Check console for details.';
 			scanning = false;
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

  	function sortBooks() {
  		books.sort((a, b) => {
 			switch (sortBy) {
 				case 'title':
 					return (a.title || '').localeCompare(b.title || '');
 				case 'authors':
 					return parseAuthors(a.authors).localeCompare(parseAuthors(b.authors));
 				case 'added_at':
 					return b.added_at - a.added_at;
 				case 'last_read':
 					if (a.status === 'reading' && b.status !== 'reading') return -1;
 					if (b.status === 'reading' && a.status !== 'reading') return 1;
 					return b.added_at - a.added_at;
 				default:
 					return 0;
 			}
 		});
 	}

 	function statusDot(status: string) {
 		switch (status) {
 			case 'reading': return 'bg-blue-500';
 			case 'finished': return 'bg-emerald-500';
 			default: return '';
 		}
 	}

 	function toggleBookSelection(bookId: number, event?: Event) {
  		if (event) {
  			event.preventDefault();
  			event.stopPropagation();
  		}
  		const newSet = new Set(selectedBooks);
  		if (newSet.has(bookId)) {
  			newSet.delete(bookId);
  		} else {
  			newSet.add(bookId);
  		}
  		selectedBooks = newSet;
  		updateSelectAllMode();
  	}

    function handleBookClick(event: MouseEvent) {
    		const bookId = Number((event.currentTarget as HTMLElement).dataset.bookId);
    		if (bulkSelectMode) {
    			event.preventDefault();
    			event.stopPropagation();
    			toggleBookSelection(bookId);
    		}
    		// If not in bulk mode, let the link handle navigation normally
    }

    function handleBookKeydown(event: KeyboardEvent) {
    		if (event.key !== 'Enter' && event.key !== ' ') return;
    		event.preventDefault();
    		handleBookClick(event as unknown as MouseEvent);
    }

    function handleMouseDown(event: MouseEvent) {
    		if ('ontouchstart' in window) return;
    		const bookId = Number((event.currentTarget as HTMLElement).dataset.bookId);
    		longPressTimer = window.setTimeout(() => {
    			// Prevent the upcoming click event from firing
    			event.stopImmediatePropagation();
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
    			// Prevent the upcoming click event from firing
    			event.stopImmediatePropagation();
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
 		selectedBooks = new Set(books.map(b => b.id));
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
 		selectAllMode = 'none';
 	}

 	function isBookSelected(bookId: number): boolean {
 		return selectedBooks.has(bookId);
 	}

 	async function fetchShelves() {
 		try {
 			const res = await fetch('/api/shelves');
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
			genre: getQueryValues($page.url.searchParams, 'genre', true).join(',') || undefined,
			tags: getQueryValues($page.url.searchParams, 'tags', true).join(',') || undefined,
			status: getQueryValues($page.url.searchParams, 'status'),
			filter_mode: getFilterMode()
		};
	}

 	async function addToShelf(shelfId: number) {
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
 		if (!confirm(`Delete ${count} book(s)? This cannot be undone.`)) return;

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
		await fetchBooks(true);
		showMetadataLookup = false;
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
		for (const genre of getQueryValues(params, 'genre', true)) {
			filters.push({ key: 'genre', value: genre, label: `Genre: ${genre}` });
		}
		for (const tag of getQueryValues(params, 'tags', true)) {
			filters.push({ key: 'tags', value: tag, label: `Tag: ${tag}` });
		}
		for (const status of getQueryValues(params, 'status')) {
			filters.push({ key: 'status', value: status, label: `Status: ${status}` });
		}

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
 		navigateWithFilters(url);
 	}

	function clearAllFilters() {
		const url = new URL($page.url);
		url.searchParams.delete('author');
		url.searchParams.delete('series');
		url.searchParams.delete('genre');
		url.searchParams.delete('genre_mode');
		url.searchParams.delete('tags');
		url.searchParams.delete('tag_mode');
		url.searchParams.delete('status');
		url.searchParams.delete('filter_mode');
		navigateWithFilters(url);
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

  	function applyAuthorFilter(authorName: string) {
		toggleRepeatedFilter('author', authorName);
  	}

	function applySeriesFilter(seriesName: string) {
		toggleRepeatedFilter('series', seriesName);
	}

	function toggleGenreSelection(genreName: string) {
		const url = new URL($page.url);
		const genreList = getQueryValues(url.searchParams, 'genre', true);
		const newGenres = genreList.includes(genreName)
			? genreList.filter((genre) => genre !== genreName)
			: [...genreList, genreName];

		if (newGenres.length === 0) {
			url.searchParams.delete('genre');
			url.searchParams.delete('genre_mode');
		} else {
			url.searchParams.set('genre', newGenres.join(','));
			url.searchParams.delete('genre_mode');
		}
		navigateWithFilters(url);
	}

	function toggleTagSelection(tagName: string) {
		const url = new URL($page.url);
		const tagList = getQueryValues(url.searchParams, 'tags', true);
		const newTags = tagList.includes(tagName)
			? tagList.filter((tag) => tag !== tagName)
			: [...tagList, tagName];

		if (newTags.length === 0) {
			url.searchParams.delete('tags');
			url.searchParams.delete('tag_mode');
		} else {
			url.searchParams.set('tags', newTags.join(','));
			url.searchParams.delete('tag_mode');
		}
		navigateWithFilters(url);
	}

	function isGenreSelected(genreName: string): boolean {
		return getQueryValues($page.url.searchParams, 'genre', true).includes(genreName);
	}

	function isTagSelected(tagName: string): boolean {
		return getQueryValues($page.url.searchParams, 'tags', true).includes(tagName);
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

 	function getSelectionCount(): number {
 		if (selectAllMode === 'filtered') {
 			return totalBooks;
 		}
 		return selectedBooks.size;
 	}
  </script>

<div class="pb-20 transition-all duration-300">
	<div class="sticky top-0 z-30 px-3 py-3 sm:px-6 sm:py-4 bg-[var(--color-surface-base)]/95 backdrop-blur border-b border-[var(--color-surface-border)] shadow-[0_1px_0_rgba(255,255,255,0.04)] transition-all duration-300 {showFilterPanel ? 'lg:pr-[21.5rem]' : ''}">
		<div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between min-w-0">
			<div class="min-w-0 flex-1">
				<div class="flex items-baseline gap-2 sm:gap-3 min-w-0">
					<h1 class="text-xl sm:text-2xl font-bold text-[var(--color-surface-text)] truncate">
						{libraryFilter ? libraryName || 'Library' : 'All Books'}
					</h1>
					{#if totalBooks > 0}
						<p class="text-sm text-[var(--color-surface-text-muted)] whitespace-nowrap">
							{totalBooks} books
						</p>
					{/if}
				</div>
				{#if scanMessage}
					<div class="mt-2 inline-flex items-center gap-2 rounded-lg border border-[var(--color-primary-500)]/40 bg-[var(--color-primary-500)]/15 px-3 py-1.5 text-sm text-[var(--color-primary-300)]">
						{scanMessage}
					</div>
				{/if}
			</div>

			<div class="flex items-center justify-start lg:justify-end gap-2 flex-wrap flex-shrink-0">
				<button
					onclick={() => viewMode = 'grid'}
					class="p-2.5 rounded-lg {viewMode === 'grid' ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'} transition-colors"
					aria-label="Grid view"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z"></path>
					</svg>
				</button>
				<button
					onclick={() => viewMode = 'list'}
					class="p-2.5 rounded-lg {viewMode === 'list' ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'} transition-colors"
					aria-label="List view"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16"></path>
					</svg>
				</button>

				<div class="relative">
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
						class="inline-flex h-10 items-center px-3 sm:px-4 rounded-lg bg-[var(--color-surface-overlay)] hover:bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] font-medium transition-colors"
					>
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
						</svg>
						<svg class="hidden sm:block w-4 h-4 ml-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
						</svg>
					</button>
					{#if showSettingsMenu}
						<div class="absolute right-0 top-full mt-2 w-56 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded-lg shadow-lg z-40 py-3">
							{#if viewMode === 'grid'}
								<div class="px-4 pb-3 border-b border-[var(--color-surface-border)]">
									<label class="text-sm font-medium text-[var(--color-surface-text)] block mb-2" for="library-grid-size">Grid Size</label>
									<div class="flex items-center space-x-2">
										<input
											id="library-grid-size"
											type="range"
											min="2"
											max="12"
											bind:value={localGridSize}
											onchange={(e) => updateGridSize(Number(e.currentTarget.value))}
											class="flex-1 h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer slider"
										>
										<span class="text-sm text-[var(--color-surface-text)] w-6 text-center">{localGridSize}</span>
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
									onclick={scanLibrary}
									disabled={scanning}
									class="w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] flex items-center disabled:opacity-50"
								>
									{#if scanning}
										<svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
											<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
											<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
										</svg>
										Scanning...
									{:else}
										<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
										</svg>
										{libraryFilter ? 'Scan Library' : 'Scan All Libraries'}
									{/if}
								</button>
							</div>
						</div>
					{/if}
				</div>

				<div class="relative">
					{#if showSortMenu}
						<button
							type="button"
							class="fixed inset-0 z-20"
							aria-label="Close sort menu"
							onclick={() => showSortMenu = false}
						></button>
					{/if}
					<button
						onclick={() => showSortMenu = !showSortMenu}
						aria-label="Sort books"
						class="inline-flex h-10 items-center px-3 sm:px-4 rounded-lg bg-[var(--color-surface-overlay)] hover:bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] font-medium transition-colors"
					>
						<span class="hidden sm:inline">Sort</span>
						<svg class="w-4 h-4 ml-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
						</svg>
					</button>
					{#if showSortMenu}
						<div class="absolute right-0 top-full mt-2 w-48 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded-lg shadow-lg z-40 py-1">
							<button
								onclick={() => { sortBy = 'added_at'; showSortMenu = false; }}
								class="w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] {sortBy === 'added_at' ? 'text-[var(--color-primary-400)]' : ''}"
							>
								Date Added
							</button>
							<button
								onclick={() => { sortBy = 'title'; showSortMenu = false; }}
								class="w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] {sortBy === 'title' ? 'text-[var(--color-primary-400)]' : ''}"
							>
								Title
							</button>
							<button
								onclick={() => { sortBy = 'authors'; showSortMenu = false; }}
								class="w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] {sortBy === 'authors' ? 'text-[var(--color-primary-400)]' : ''}"
							>
								Author
							</button>
							<button
								onclick={() => { sortBy = 'last_read'; showSortMenu = false; }}
								class="w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] {sortBy === 'last_read' ? 'text-[var(--color-primary-400)]' : ''}"
							>
								Last Read
							</button>
						</div>
					{/if}
				</div>

				<button
					onclick={() => showFilterPanel = !showFilterPanel}
					class="inline-flex h-10 items-center px-3 sm:px-4 rounded-lg bg-[var(--color-surface-overlay)] hover:bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] font-medium transition-colors"
				>
					<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z"></path>
					</svg>
					<span class="hidden sm:inline">Filter</span>
					{#if getActiveFilters().length > 0}
						<span class="ml-2 px-2 py-0.5 text-xs rounded-full bg-[var(--color-primary-500)] text-white">
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
						class="inline-flex items-center px-3 py-1 rounded-full bg-[var(--color-primary-500)]/20 border border-[var(--color-primary-500)]/50 text-[var(--color-primary-300)] text-sm hover:bg-[var(--color-primary-500)]/30 transition-colors"
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

  <!-- Filter Side Panel (right side, under top bar) -->
  {#if showFilterPanel}
  		<!-- Side Panel - attached to bottom of top bar, no backdrop -->
  		<div class="fixed top-16 right-0 h-[calc(100vh-4rem)] w-80 bg-[var(--color-surface-overlay)] border-l border-[var(--color-surface-border)] z-30 overflow-y-auto shadow-xl transform transition-transform duration-300 ease-out translate-x-0">
  			<div class="sticky top-0 h-[73px] bg-[var(--color-surface-overlay)] border-b border-[var(--color-surface-border)] px-4 flex items-center justify-between gap-3 z-10">
  				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Filters</h2>
				<div class="relative grid grid-cols-3 flex-1 max-w-40 rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-1">
					<span
						class="absolute top-1 bottom-1 left-1 rounded-lg bg-[var(--color-primary-500)] shadow-sm transition-transform duration-200 ease-out"
						style="width: calc((100% - 0.5rem) / 3); transform: translateX({getFilterModeIndex() * 100}%);"
					></span>
					{#each FILTER_MODES as mode}
						<button
							onclick={() => setFilterMode(mode)}
							class="relative z-10 px-2 py-1.5 rounded-lg text-[11px] font-semibold tracking-wide transition-colors {getFilterMode() === mode ? 'text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
						>
							{mode}
						</button>
					{/each}
				</div>
					<button
						onclick={() => showFilterPanel = false}
						aria-label="Close filters"
 						class="p-1 rounded hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] transition-colors"
  				>
  					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
  						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
  					</svg>
  				</button>
  			</div>
 			
 			<div class="p-4 space-y-2">
 				<!-- Author Filter (Accordion) -->
 				<div class="border border-[var(--color-surface-border)] rounded-lg overflow-hidden">
 					<button
 						onclick={() => filterAuthorsOpen = !filterAuthorsOpen}
 						class="w-full flex items-center justify-between px-4 py-3 bg-[var(--color-surface-base)] hover:bg-[var(--color-surface-700)] transition-colors"
 					>
 						<span class="font-medium text-[var(--color-surface-text)]">Author</span>
 						<div class="flex items-center space-x-2">
 							<span class="text-xs text-[var(--color-surface-text-muted)] bg-[var(--color-surface-overlay)] px-2 py-0.5 rounded">{availableAuthors.length}</span>
 							<svg class="w-4 h-4 text-[var(--color-surface-text-muted)] transition-transform {filterAuthorsOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
 								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
 							</svg>
 						</div>
 					</button>
 					{#if filterAuthorsOpen}
 						<div class="max-h-48 overflow-y-auto">
 							{#each availableAuthors.slice(0, 15) as author}
 								<button
 									onclick={() => applyAuthorFilter(author.name)}
 									class="w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] transition-colors flex justify-between items-center {isAuthorSelected(author.name) ? 'bg-[var(--color-primary-500)]/20' : ''}"
 								>
 									<span class="truncate flex items-center">
										{#if isAuthorSelected(author.name)}
											<svg class="w-4 h-4 mr-2 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
											</svg>
										{/if}
										{author.name}
									</span>
 									<span class="text-xs text-[var(--color-surface-text-muted)] ml-2">{author.book_count}</span>
 								</button>
 							{/each}
 							{#if availableAuthors.length === 0}
 								<p class="text-sm text-[var(--color-surface-text-muted)] px-4 py-2">No authors found</p>
 							{/if}
 						</div>
 					{/if}
 				</div>

 				<!-- Series Filter (Accordion) -->
 				<div class="border border-[var(--color-surface-border)] rounded-lg overflow-hidden">
 					<button
 						onclick={() => filterSeriesOpen = !filterSeriesOpen}
 						class="w-full flex items-center justify-between px-4 py-3 bg-[var(--color-surface-base)] hover:bg-[var(--color-surface-700)] transition-colors"
 					>
 						<span class="font-medium text-[var(--color-surface-text)]">Series</span>
 						<div class="flex items-center space-x-2">
 							<span class="text-xs text-[var(--color-surface-text-muted)] bg-[var(--color-surface-overlay)] px-2 py-0.5 rounded">{availableSeries.length}</span>
 							<svg class="w-4 h-4 text-[var(--color-surface-text-muted)] transition-transform {filterSeriesOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
 								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
 							</svg>
 						</div>
 					</button>
 					{#if filterSeriesOpen}
 						<div class="max-h-48 overflow-y-auto">
 							{#each availableSeries.slice(0, 15) as serie}
 								<button
 									onclick={() => applySeriesFilter(serie.name)}
 									class="w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] transition-colors flex justify-between items-center {isSeriesSelected(serie.name) ? 'bg-[var(--color-primary-500)]/20' : ''}"
 								>
 									<span class="truncate flex items-center">
										{#if isSeriesSelected(serie.name)}
											<svg class="w-4 h-4 mr-2 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
											</svg>
										{/if}
										{serie.name}
									</span>
 									<span class="text-xs text-[var(--color-surface-text-muted)] ml-2">{serie.book_count}</span>
 								</button>
 							{/each}
							{#if availableSeries.length === 0}
								<p class="text-sm text-[var(--color-surface-text-muted)] px-4 py-2">No series found</p>
							{/if}
						</div>
					{/if}
				</div>

				<!-- Genre Filter (Accordion) -->
				<div class="border border-[var(--color-surface-border)] rounded-lg overflow-hidden">
					<button
						onclick={() => filterGenresOpen = !filterGenresOpen}
						class="w-full flex items-center justify-between px-4 py-3 bg-[var(--color-surface-base)] hover:bg-[var(--color-surface-700)] transition-colors"
					>
						<span class="font-medium text-[var(--color-surface-text)]">Genre</span>
						<div class="flex items-center space-x-2">
							<span class="text-xs text-[var(--color-surface-text-muted)] bg-[var(--color-surface-overlay)] px-2 py-0.5 rounded">{availableGenres.length}</span>
							<svg class="w-4 h-4 text-[var(--color-surface-text-muted)] transition-transform {filterGenresOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
							</svg>
						</div>
					</button>
					{#if filterGenresOpen}
						<div class="max-h-48 overflow-y-auto">
							{#each availableGenres.slice(0, 15) as genre}
								<button
									onclick={() => toggleGenreSelection(genre.name)}
									class="w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] transition-colors flex justify-between items-center {isGenreSelected(genre.name) ? 'bg-[var(--color-primary-500)]/20' : ''}"
								>
									<span class="truncate flex items-center">
										{#if isGenreSelected(genre.name)}
											<svg class="w-4 h-4 mr-2 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
											</svg>
										{/if}
										{genre.name}
									</span>
									<span class="text-xs text-[var(--color-surface-text-muted)] ml-2">{genre.book_count}</span>
								</button>
							{/each}
							{#if availableGenres.length === 0}
								<p class="text-sm text-[var(--color-surface-text-muted)] px-4 py-2">No genres found</p>
							{/if}
						</div>
					{/if}
				</div>

				<!-- Tags Filter (Accordion) -->
  				<div class="border border-[var(--color-surface-border)] rounded-lg overflow-hidden">
  					<button
  						onclick={() => filterTagsOpen = !filterTagsOpen}
  						class="w-full flex items-center justify-between px-4 py-3 bg-[var(--color-surface-base)] hover:bg-[var(--color-surface-700)] transition-colors"
  					>
  						<span class="font-medium text-[var(--color-surface-text)]">Tags</span>
  						<div class="flex items-center space-x-2">
  							<span class="text-xs text-[var(--color-surface-text-muted)] bg-[var(--color-surface-overlay)] px-2 py-0.5 rounded">{availableTags.length}</span>
  							<svg class="w-4 h-4 text-[var(--color-surface-text-muted)] transition-transform {filterTagsOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
  								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
  							</svg>
  						</div>
  					</button>
  					{#if filterTagsOpen}
  						<div class="max-h-48 overflow-y-auto">
  							{#each availableTags.slice(0, 15) as tag}
  								<button
  									onclick={() => toggleTagSelection(tag.name)}
  									class="w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] transition-colors flex justify-between items-center {isTagSelected(tag.name) ? 'bg-[var(--color-primary-500)]/20' : ''}"
  								>
  									<span class="truncate flex items-center">
  										{#if isTagSelected(tag.name)}
  											<svg class="w-4 h-4 mr-2 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
  												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
  											</svg>
  										{/if}
  										{tag.name}
  									</span>
  									<span class="text-xs text-[var(--color-surface-text-muted)] ml-2">{tag.book_count}</span>
  								</button>
  							{/each}
  							{#if availableTags.length === 0}
  								<p class="text-sm text-[var(--color-surface-text-muted)] px-4 py-2">No tags found</p>
 							{/if}
 						</div>
 					{/if}
 				</div>

 				<!-- Status Filter (Accordion) -->
 				<div class="border border-[var(--color-surface-border)] rounded-lg overflow-hidden">
 					<button
 						onclick={() => filterStatusOpen = !filterStatusOpen}
 						class="w-full flex items-center justify-between px-4 py-3 bg-[var(--color-surface-base)] hover:bg-[var(--color-surface-700)] transition-colors"
 					>
 						<span class="font-medium text-[var(--color-surface-text)]">Reading Status</span>
 						<svg class="w-4 h-4 text-[var(--color-surface-text-muted)] transition-transform {filterStatusOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
 							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
 						</svg>
 					</button>
 					{#if filterStatusOpen}
 						<div class="py-1">
 							{#each [{ value: 'unread', label: 'Unread' }, { value: 'reading', label: 'Reading' }, { value: 'finished', label: 'Finished' }] as statusOption}
 								<button
 									onclick={() => applyStatusFilter(statusOption.value)}
 									class="w-full text-left px-4 py-2 hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] transition-colors flex items-center {isStatusSelected(statusOption.value) ? 'bg-[var(--color-primary-500)]/20' : ''}"
 								>
									{#if isStatusSelected(statusOption.value)}
										<svg class="w-4 h-4 mr-2 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
										</svg>
									{/if}
 									{statusOption.label}
 								</button>
 							{/each}
 						</div>
 					{/if}
 				</div>
 			</div>
 		</div>
 	{/if}
	<div class="px-6 pt-6 transition-all duration-300 {showFilterPanel ? 'lg:pr-[21.5rem]' : ''}">
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
 					<button onclick={scanLibrary} disabled={scanning} class="px-4 py-2 bg-[var(--color-primary-500)] text-white rounded-lg hover:bg-[var(--color-primary-600)]">
 						Scan Now
 					</button>
 				{/if}
 			{/if}
 		</div>
  	{:else}
  		<div class={viewMode === 'grid' ? 'grid gap-4' : 'space-y-4'} style={gridStyle}>
  			{#each books as book}
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
							{#if book.status === 'reading' || book.status === 'finished'}
								<span class="absolute top-1 right-1 z-10 w-2.5 h-2.5 rounded-full {statusDot(book.status)}"></span>
    							{/if}
							<BookCoverFrame
								src={book.cover_path ? getCoverThumbUrl(book.id, libraryCoverThumbSize, book.cover_updated_on) : null}
								alt={book.title}
								mode="cover"
								frameClass="aspect-[2/3] mb-2"
								imageClass="group-hover:scale-105 transition-transform"
								placeholderSize="md"
							/>
  							<h3 class="text-sm font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
    							{#if book.authors && book.authors !== '[]'}
    								<p class="text-xs text-[var(--color-surface-text-muted)] truncate">{parseAuthors(book.authors)}</p>
    							{/if}
    						</a>
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
  						ontouchstart={handleTouchStart}
  						ontouchend={handleTouchEnd}
  						onkeydown={handleBookKeydown}
  						role="button"
  						tabindex="0"
  					>
  						<a href="/book/{book.id}" class="block bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] {selectedBooks.has(book.id) ? 'border-[var(--color-primary-500)]' : ''} p-4 hover:border-[var(--color-primary-500)]/50 transition-colors">
  							<div class="flex items-center space-x-4">
								<BookCoverFrame
									src={book.cover_path ? getCoverThumbUrl(book.id, 'small', book.cover_updated_on) : null}
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
 							{/if}
 						</span>
 						<div class="flex items-center space-x-2">
 							<button
 								onclick={selectAllPage}
 								class="px-3 py-1.5 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] transition-colors"
 							>
 								Select All on Page
 							</button>
 							{#if totalBooks > books.length}
 								<button
 									onclick={selectAllFiltered}
 									class="px-3 py-1.5 text-sm rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white transition-colors"
 								>
 									Select All {totalBooks}
 								</button>
 							{/if}
 							<button
 								onclick={deselectAll}
 								class="px-3 py-1.5 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] transition-colors"
 							>
 								Deselect
 							</button>
 						</div>
					</div>
					<div class="flex items-center space-x-2">
						<div class="relative">
							<button
								onclick={() => showMetadataMenu = !showMetadataMenu}
								disabled={selectedBooks.size === 0 || metadataLookupQueueing}
								class="px-4 py-2 text-sm rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] font-medium transition-colors disabled:opacity-50 flex items-center space-x-2"
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
							class="px-4 py-2 text-sm rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white font-medium transition-colors disabled:opacity-50 flex items-center space-x-2"
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
			onApplied={async () => fetchBooks(true)}
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

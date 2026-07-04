<script lang="ts">
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import { getFormatColor } from '$lib/stores';

	interface Shelf {
		id: number;
		name: string;
	}

	interface Library {
		id: number;
		name: string;
		book_count: number;
	}

	interface Book {
		id: number;
		title: string;
		authors: string;
		cover_path: string;
		format: string;
		library_id: number;
		status: string;
	}

	interface MetadataOption {
		name: string;
		value?: string;
		book_count: number;
	}

	interface Props {
		open: boolean;
		shelf: Shelf | null;
		existingBookIds?: Set<number>;
		onClose?: () => void;
		onAdded?: () => void;
	}

	let {
		open = false,
		shelf = null,
		existingBookIds = new Set<number>(),
		onClose,
		onAdded
	}: Props = $props();

	let libraries = $state<Library[]>([]);
	let books = $state<Book[]>([]);
	let selectedBooks = $state<Set<number>>(new Set());
	let searchQuery = $state('');
	let libraryID = $state('');
	let authorFilter = $state('');
	let seriesFilter = $state('');
	let tagFilter = $state('');
	let statusFilter = $state('');
	let formatFilter = $state('');
	let sortBy = $state('title');
	let sortDir = $state<'asc' | 'desc'>('asc');
	let loading = $state(false);
	let loadingMore = $state(false);
	let adding = $state(false);
	let initializedForOpen = $state(false);
	let totalBooks = $state(0);
	let offset = $state(0);
	let hasMore = $state(false);
	let errorMessage = $state('');
	let searchTimer: ReturnType<typeof setTimeout> | null = null;
	let filterOptions = $state<{ authors: MetadataOption[]; series: MetadataOption[]; tags: MetadataOption[]; }>({
		authors: [],
		series: [],
		tags: []
	});

	const BATCH_SIZE = 60;
	const FILTER_OPTION_LIMIT = 200;
	let hasActiveFilters = $derived(!!(searchQuery.trim() || libraryID || authorFilter || seriesFilter || tagFilter || statusFilter || formatFilter));
	let visibleAuthorOptions = $derived(filterOptions.authors.slice(0, FILTER_OPTION_LIMIT));
	let visibleSeriesOptions = $derived(filterOptions.series.slice(0, FILTER_OPTION_LIMIT));
	let visibleTagOptions = $derived(filterOptions.tags.slice(0, FILTER_OPTION_LIMIT));
	const statusOptions = [
		{ value: '', label: 'Any status' },
		{ value: 'unread', label: 'Unread' },
		{ value: 'reading', label: 'Currently Reading' },
		{ value: 'finished', label: 'Already Read' }
	];
	const formatOptions = [
		{ value: '', label: 'Any format' },
		{ value: 'epub', label: 'EPUB' },
		{ value: 'pdf', label: 'PDF' },
		{ value: 'cbz', label: 'CBZ' },
		{ value: 'cbr', label: 'CBR' },
		{ value: 'mobi', label: 'MOBI' },
		{ value: 'azw3', label: 'AZW3' }
	];

	function parseAuthors(authorsJson: string): string {
		try {
			const arr = JSON.parse(authorsJson);
			return Array.isArray(arr) ? arr.join(', ') : authorsJson;
		} catch {
			return authorsJson;
		}
	}

	function buildBooksUrl(nextOffset = 0, includeTotal = true): string {
		const params = new URLSearchParams();
		params.set('limit', String(BATCH_SIZE));
		params.set('offset', String(nextOffset));
		params.set('sort', sortBy);
		params.set('sort_dir', sortDir);
		if (!includeTotal) params.set('include_total', 'false');
		if (searchQuery.trim()) params.set('q', searchQuery.trim());
		if (libraryID) params.set('library_id', libraryID);
		if (authorFilter) params.append('author', authorFilter);
		if (seriesFilter) params.append('series', seriesFilter);
		if (tagFilter) params.append('tags', tagFilter);
		if (statusFilter) params.set('status', statusFilter);
		if (formatFilter) params.set('format', formatFilter);
		return `/api/books?${params.toString()}`;
	}

	function buildFilterOptionsUrl(): string {
		const params = new URLSearchParams();
		if (searchQuery.trim()) params.set('q', searchQuery.trim());
		if (libraryID) params.set('library_id', libraryID);
		const query = params.toString();
		return query ? `/api/filter-options?${query}` : '/api/filter-options';
	}

	function buildBulkFilterPayload() {
		return {
			library_id: libraryID || undefined,
			q: searchQuery.trim() || undefined,
			author: authorFilter ? [authorFilter] : [],
			series: seriesFilter ? [seriesFilter] : [],
			tags: tagFilter ? [tagFilter] : [],
			format: formatFilter ? [formatFilter] : [],
			status: statusFilter ? [statusFilter] : []
		};
	}

	async function fetchLibraries() {
		try {
			const res = await fetch('/api/libraries', { cache: 'no-store' });
			if (res.ok) libraries = await res.json();
		} catch (e) {
			console.error('Failed to fetch libraries:', e);
		}
	}

	async function fetchFilterOptions() {
		try {
			const res = await fetch(buildFilterOptionsUrl(), { cache: 'no-store' });
			if (!res.ok) return;
			const data = await res.json();
			filterOptions = {
				authors: Array.isArray(data.authors) ? data.authors : [],
				series: Array.isArray(data.series) ? data.series : [],
				tags: Array.isArray(data.tags) ? data.tags : []
			};
		} catch (e) {
			console.error('Failed to fetch filter options:', e);
		}
	}

	async function fetchBooks(reset = true) {
		if (!open) return;
		errorMessage = '';
		if (reset) {
			loading = true;
			offset = 0;
		} else {
			loadingMore = true;
		}

		try {
			const res = await fetch(buildBooksUrl(reset ? 0 : offset, reset), { cache: 'no-store' });
			if (!res.ok) {
				errorMessage = await res.text() || 'Failed to load books';
				return;
			}
			const data = await res.json();
			const nextBooks = data.books || [];
			if (reset) {
				books = nextBooks;
			} else {
				books = [...books, ...nextBooks];
			}
			if (typeof data.total === 'number') totalBooks = data.total;
			hasMore = !!data.has_more;
			offset = books.length;
		} catch (e) {
			console.error('Failed to fetch books:', e);
			errorMessage = 'Failed to load books';
		} finally {
			loading = false;
			loadingMore = false;
		}
	}

	function scheduleSearch() {
		if (searchTimer) window.clearTimeout(searchTimer);
		searchTimer = window.setTimeout(() => {
			void fetchBooks(true);
			void fetchFilterOptions();
		}, 250);
	}

	function refreshBooksAndOptions() {
		void fetchBooks(true);
		void fetchFilterOptions();
	}

	function closeModal() {
		if (adding) return;
		onClose?.();
	}

	function isAlreadyOnShelf(bookId: number): boolean {
		return existingBookIds.has(bookId);
	}

	function isSelected(bookId: number): boolean {
		return selectedBooks.has(bookId);
	}

	function toggleBook(bookId: number) {
		if (isAlreadyOnShelf(bookId)) return;
		const next = new Set(selectedBooks);
		if (next.has(bookId)) next.delete(bookId);
		else next.add(bookId);
		selectedBooks = next;
	}

	function selectVisible() {
		const next = new Set(selectedBooks);
		for (const book of books) {
			if (!isAlreadyOnShelf(book.id)) next.add(book.id);
		}
		selectedBooks = next;
	}

	function clearSelection() {
		selectedBooks = new Set();
	}

	function clearFilters() {
		searchQuery = '';
		libraryID = '';
		authorFilter = '';
		seriesFilter = '';
		tagFilter = '';
		statusFilter = '';
		formatFilter = '';
		refreshBooksAndOptions();
	}

	async function addSelectedBooks() {
		if (!shelf?.id || selectedBooks.size === 0) return;
		adding = true;
		errorMessage = '';
		try {
			const res = await fetch(`/api/shelves/${shelf.id}/books/bulk`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ book_ids: Array.from(selectedBooks) })
			});
			if (!res.ok) {
				errorMessage = await res.text() || 'Failed to add books';
				return;
			}
			selectedBooks = new Set();
			onAdded?.();
			onClose?.();
		} catch (e) {
			console.error('Failed to add books:', e);
			errorMessage = 'Failed to add books';
		} finally {
			adding = false;
		}
	}

	async function addAllMatchingBooks() {
		if (!shelf?.id || totalBooks === 0 || adding) return;
		if (!confirm(`Add all ${totalBooks} matching books to "${shelf.name}"? Books already on this shelf will be skipped.`)) return;
		adding = true;
		errorMessage = '';
		try {
			const res = await fetch(`/api/shelves/${shelf.id}/books/bulk-by-filter`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(buildBulkFilterPayload())
			});
			if (!res.ok) {
				errorMessage = await res.text() || 'Failed to add matching books';
				return;
			}
			selectedBooks = new Set();
			onAdded?.();
			onClose?.();
		} catch (e) {
			console.error('Failed to add matching books:', e);
			errorMessage = 'Failed to add matching books';
		} finally {
			adding = false;
		}
	}

	$effect(() => {
		if (!open) {
			initializedForOpen = false;
			return;
		}
		if (initializedForOpen) return;
		initializedForOpen = true;
		selectedBooks = new Set();
		searchQuery = '';
		libraryID = '';
		authorFilter = '';
		seriesFilter = '';
		tagFilter = '';
		statusFilter = '';
		formatFilter = '';
		sortBy = 'title';
		sortDir = 'asc';
		void fetchLibraries();
		void fetchFilterOptions();
		window.setTimeout(() => fetchBooks(true), 0);
	});
</script>

{#if open && shelf}
	<div
		class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4"
		role="dialog"
		aria-modal="true"
		tabindex="-1"
		onkeydown={(event) => { if (event.key === 'Escape') closeModal(); }}
	>
		<button
			type="button"
			class="absolute inset-0 z-0"
			aria-label="Close add books modal"
			onclick={closeModal}
		></button>

		<div class="relative z-10 flex max-h-[96vh] w-full max-w-6xl flex-col overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl" style="height: min(96vh, 1040px);">
			<div class="flex items-center justify-between gap-4 border-b border-[var(--color-surface-border)] px-6 py-4">
				<div>
					<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Add Books to {shelf.name}</h3>
					<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">
						{selectedBooks.size > 0 ? `${selectedBooks.size} selected` : totalBooks > 0 ? `${totalBooks} books available` : 'Search and select books'}
					</p>
				</div>
				<button
					type="button"
					class="inline-flex h-9 w-9 items-center justify-center rounded-md text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)]"
					aria-label="Close"
					onclick={closeModal}
				>
					<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<path d="M18 6 6 18"></path>
						<path d="m6 6 12 12"></path>
					</svg>
				</button>
			</div>

			<div class="border-b border-[var(--color-surface-border)] p-4">
				<div class="grid gap-3 lg:grid-cols-[minmax(220px,1fr)_180px_160px_140px_170px]">
					<input
						type="search"
						bind:value={searchQuery}
						oninput={scheduleSearch}
						placeholder="Search books"
						class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
					>
					<select
						bind:value={libraryID}
						onchange={refreshBooksAndOptions}
						class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
					>
						<option value="">All libraries</option>
						{#each libraries as library}
							<option value={library.id}>{library.name}</option>
						{/each}
					</select>
					<select
						bind:value={statusFilter}
						onchange={() => fetchBooks(true)}
						class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
					>
						{#each statusOptions as option}
							<option value={option.value}>{option.label}</option>
						{/each}
					</select>
					<select
						bind:value={formatFilter}
						onchange={() => fetchBooks(true)}
						class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
					>
						{#each formatOptions as option}
							<option value={option.value}>{option.label}</option>
						{/each}
					</select>
					<div class="inline-flex overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
						<select
							bind:value={sortBy}
							onchange={() => fetchBooks(true)}
							class="min-w-0 flex-1 bg-transparent px-3 py-2 text-[var(--color-surface-text)] focus:outline-none"
							aria-label="Sort books"
						>
							<option value="title">Title</option>
							<option value="authors">Author</option>
							<option value="series">Series</option>
							<option value="added_at">Added</option>
							<option value="last_read">Last Read</option>
						</select>
						<button
							type="button"
							onclick={() => { sortDir = sortDir === 'asc' ? 'desc' : 'asc'; fetchBooks(true); }}
							class="w-10 border-l border-[var(--color-surface-border)] text-[var(--color-primary-400)] hover:bg-[var(--color-surface-overlay)]"
							aria-label={sortDir === 'asc' ? 'Sort ascending' : 'Sort descending'}
							title={sortDir === 'asc' ? 'Sort ascending' : 'Sort descending'}
						>
							{sortDir === 'asc' ? '↑' : '↓'}
						</button>
					</div>
				</div>
				<div class="mt-3 grid gap-3 lg:grid-cols-[minmax(180px,1fr)_minmax(180px,1fr)_minmax(180px,1fr)_auto]">
					<select
						bind:value={authorFilter}
						onchange={() => fetchBooks(true)}
						class="min-w-0 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
						aria-label="Filter by author"
					>
						<option value="">Any author</option>
						{#each visibleAuthorOptions as author}
							<option value={author.value || author.name}>{author.name} ({author.book_count})</option>
						{/each}
					</select>
					<select
						bind:value={seriesFilter}
						onchange={() => fetchBooks(true)}
						class="min-w-0 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
						aria-label="Filter by series"
					>
						<option value="">Any series</option>
						{#each visibleSeriesOptions as series}
							<option value={series.value || series.name}>{series.name} ({series.book_count})</option>
						{/each}
					</select>
					<select
						bind:value={tagFilter}
						onchange={() => fetchBooks(true)}
						class="min-w-0 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
						aria-label="Filter by tag"
					>
						<option value="">Any tag</option>
						{#each visibleTagOptions as tag}
							<option value={tag.value || tag.name}>{tag.name} ({tag.book_count})</option>
						{/each}
					</select>
					<button
						type="button"
						onclick={clearFilters}
						disabled={!hasActiveFilters}
						class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text-muted)] transition-colors hover:border-[var(--color-primary-500)]/45 hover:text-[var(--color-surface-text)] disabled:cursor-not-allowed disabled:opacity-45"
					>
						Clear Filters
					</button>
				</div>
			</div>

			<div class="relative min-h-0 flex-1 overflow-y-auto p-4" aria-busy={loading}>
				{#if loading && books.length === 0}
					<div class="flex justify-center py-12">
						<div class="h-10 w-10 animate-spin rounded-full border-b-2 border-[var(--color-primary-500)]"></div>
					</div>
				{:else if books.length === 0}
					<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] py-12 text-center text-[var(--color-surface-text-muted)]">
						No books match your current search.
					</div>
				{:else}
					<div class="grid grid-cols-1 gap-2 md:grid-cols-2 xl:grid-cols-3">
						{#each books as book}
							{@const alreadyOnShelf = isAlreadyOnShelf(book.id)}
							{@const selected = isSelected(book.id)}
							<button
								type="button"
								onclick={() => toggleBook(book.id)}
								disabled={alreadyOnShelf}
								class="flex min-w-0 items-center gap-3 rounded-lg border p-2 text-left transition-colors {selected ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-base)] hover:border-[var(--color-primary-500)]/50 hover:bg-[var(--color-surface-overlay)]'} {alreadyOnShelf ? 'cursor-not-allowed opacity-55' : ''}"
							>
								<div class="w-12 shrink-0 overflow-hidden rounded">
									<BookCoverFrame
										src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
										alt={book.title}
										format={book.format}
										mode="cover"
										frameClass="aspect-[2/3]"
										placeholderSize="sm"
									/>
								</div>
								<div class="min-w-0 flex-1">
									<div class="truncate text-sm font-medium text-[var(--color-surface-text)]">{book.title || 'Untitled'}</div>
									{#if book.authors && book.authors !== '[]'}
										<div class="truncate text-xs text-[var(--color-surface-text-muted)]">{parseAuthors(book.authors)}</div>
									{/if}
									<div class="mt-1 flex items-center gap-2">
										{#if book.format}
											{@const formatColor = getFormatColor(book.format)}
											<span
												class="rounded border border-black/20 px-1.5 py-0.5 text-[10px] font-medium uppercase"
												style="background-color: {formatColor.bg}; color: {formatColor.text};"
											>
												{book.format}
											</span>
										{/if}
										{#if alreadyOnShelf}
											<span class="text-xs text-[var(--color-primary-400)]">On shelf</span>
										{/if}
									</div>
								</div>
								<span class="flex h-6 w-6 shrink-0 items-center justify-center rounded border {selected ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)] text-white' : 'border-[var(--color-surface-border)] text-transparent'}">
									<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7"></path>
									</svg>
								</span>
							</button>
						{/each}
					</div>

					{#if hasMore}
						<div class="mt-4 flex justify-center">
							<button
								type="button"
								onclick={() => fetchBooks(false)}
								disabled={loadingMore}
								class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)]/45 hover:bg-[var(--color-surface-overlay)] disabled:opacity-50"
							>
								{loadingMore ? 'Loading...' : 'Load More'}
							</button>
						</div>
					{/if}
				{/if}

				{#if loading && books.length > 0}
					<div class="pointer-events-none absolute inset-0 z-10 flex items-start justify-center bg-[var(--color-surface-overlay)]/55 pt-8 backdrop-blur-[1px]">
						<div class="flex items-center gap-2 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] shadow-lg">
							<div class="h-4 w-4 animate-spin rounded-full border-b-2 border-[var(--color-primary-500)]"></div>
							<span>Updating results...</span>
						</div>
					</div>
				{/if}

				{#if errorMessage}
					<div class="mt-4 rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">{errorMessage}</div>
				{/if}
			</div>

			<div class="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--color-surface-border)] px-6 py-4">
				<div class="flex items-center gap-2">
					<button
						type="button"
						onclick={selectVisible}
						class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]"
					>
						Select Visible
					</button>
					<button
						type="button"
						onclick={addAllMatchingBooks}
						disabled={totalBooks === 0 || adding}
						class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] disabled:cursor-not-allowed disabled:opacity-50"
					>
						Add All Matching
					</button>
					<button
						type="button"
						onclick={clearSelection}
						class="rounded-lg px-3 py-1.5 text-sm text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]"
					>
						Clear
					</button>
				</div>
				<div class="flex items-center gap-3">
					<button
						type="button"
						onclick={closeModal}
						class="rounded-lg px-4 py-2 text-[var(--color-surface-text-muted)] transition-colors hover:text-[var(--color-surface-text)]"
					>
						Cancel
					</button>
					<button
						type="button"
						onclick={addSelectedBooks}
						disabled={selectedBooks.size === 0 || adding}
						class="accent-action rounded-lg px-4 py-2 font-medium transition-colors disabled:opacity-50"
					>
						{adding ? 'Adding...' : `Add ${selectedBooks.size} ${selectedBooks.size === 1 ? 'Book' : 'Books'}`}
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

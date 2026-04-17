<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import AutocompleteInput from '$lib/components/AutocompleteInput.svelte';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import MetadataLookupModal from '$lib/components/MetadataLookupModal.svelte';
	import {
		getBookReaderHref,
		getFormatDisplayLabel,
		getPreferredBookFormat,
		getPreferredTextFormat,
		getReaderRouteKind,
		getSpeedReaderHref,
		uniqueBookFormats
	} from '$lib/utils/book-formats';

	let book = $state<any>(null);
	let files = $state<any[]>([]);
	let loading = $state(true);
	let editing = $state(false);
	let saving = $state(false);
	let activeTab = $state<'similar' | 'sessions' | 'files'>('similar');
	let sessions = $state<any[]>([]);
	let sessionsLoading = $state(false);
	let similarBooks = $state<any[]>([]);
	let similarLoading = $state(false);
	let similarBooksLoaded = $state(false);
	let saveError = $state<string | null>(null);
	let showMetadataLookup = $state(false);
	let showCoverModal = $state(false);
	let showShelfPicker = $state(false);
	let regeneratingCover = $state(false);
	let shelves = $state<any[]>([]);
	let shelfActionInProgress = $state(false);
	let shelfActionMessage = $state('');
	let formatMenuOpen = $state(false);
	let convertMenuFileId = $state<number | null>(null);
	let selectedConvertFormat = $state<'epub' | 'fb2' | 'txt' | 'rtf'>('epub');

	let editForm = $state<any>({});
	let authorsList = $state<string[]>([]);

	let statusOptions = [
		{ value: 'unread', label: 'Unread' },
		{ value: 'reading', label: 'Currently Reading' },
		{ value: 'finished', label: 'Already Read' }
	];

	$effect(() => {
		const bookId = $page.params.bookID;
		if (bookId) {
			sessions = [];
			similarBooks = [];
			similarBooksLoaded = false;
			void fetchBook();
		}
	});

	$effect(() => {
		if (book?.id && activeTab === 'similar' && !similarLoading && !similarBooksLoaded) {
			void fetchSimilarBooks();
		}
	});

	$effect(() => {
		if (showShelfPicker) {
			void fetchShelves();
		}
	});

	async function fetchBook() {
		loading = true;
		formatMenuOpen = false;
		const bookId = $page.params.bookID;
		try {
			const [bookRes, filesRes] = await Promise.all([
				fetch(`/api/books/${bookId}`),
				fetch(`/api/books/${bookId}/files`)
			]);

			if (bookRes.ok) book = await bookRes.json();
			if (filesRes.ok) files = await filesRes.json();
		} catch (e) {
			console.error('Failed to fetch book:', e);
		} finally {
			loading = false;
		}
	}

	async function fetchSessions() {
		sessionsLoading = true;
		const bookId = $page.params.bookID;
		try {
			const res = await fetch(`/api/books/${bookId}/sessions`, {
				cache: 'no-store'
			});
			if (res.ok) {
				sessions = await res.json();
			}
		} catch (e) {
			console.error('Failed to fetch sessions:', e);
		} finally {
			sessionsLoading = false;
		}
	}

	async function deleteSession(sessionId: number) {
		if (!book?.id) return;
		if (!confirm('Delete this reading session?')) return;

		try {
			const res = await fetch(`/api/books/${book.id}/sessions/${sessionId}`, {
				method: 'DELETE'
			});
			if (res.ok) {
				sessions = sessions.filter(session => session.id !== sessionId);
			}
		} catch (e) {
			console.error('Failed to delete session:', e);
		}
	}

	async function fetchSimilarBooks() {
		if (!book?.id || similarLoading || similarBooksLoaded) return;
		similarLoading = true;
		const bookId = $page.params.bookID;
		try {
			const res = await fetch(`/api/books/${bookId}/similar?limit=6`);
			if (res.ok) {
				similarBooks = await res.json();
				similarBooksLoaded = true;
			}
		} catch (e) {
			console.error('Failed to fetch similar books:', e);
		} finally {
			similarLoading = false;
		}
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

	function parseAuthors(authorsJson: string): string[] {
		try {
			const arr = JSON.parse(authorsJson);
			return Array.isArray(arr) ? arr : [authorsJson];
		} catch {
			return [authorsJson];
		}
	}

	function parseJsonArray(jsonStr: string): string[] {
		if (!jsonStr || jsonStr === '[]' || jsonStr === '') return [];
		try {
			const arr = JSON.parse(jsonStr);
			return Array.isArray(arr) ? arr : [];
		} catch {
			return [];
		}
	}

	function normalizeAuthorName(name: string): string {
		if (!name) return name;

		// Remove extra whitespace and normalize
		name = name.trim().replace(/\s+/g, ' ');

		// Check if it's "Last, First" format
		const commaIndex = name.indexOf(',');
		if (commaIndex > 0) {
			const lastName = name.substring(0, commaIndex).trim();
			const firstName = name.substring(commaIndex + 1).trim();
			// Reorder to "First Last"
			return `${firstName} ${lastName}`;
		}

		// Handle formats like "James A. Smith" - keep as is
		return name;
	}

	function matchAuthorNames(name1: string, name2: string): boolean {
		const normalized1 = normalizeAuthorName(name1).toLowerCase().replace(/[^a-z\s]/g, '');
		const normalized2 = normalizeAuthorName(name2).toLowerCase().replace(/[^a-z\s]/g, '');

		// Exact match after normalization
		if (normalized1 === normalized2) return true;

		// Check if one is substring of the other (for partial matches)
		return normalized1.includes(normalized2) || normalized2.includes(normalized1);
	}

	interface HierarchicalPart {
 		text: string;
 		fullPath: string;
 		isParent: boolean;
 	}

 	function parseHierarchicalGenre(genre: string): HierarchicalPart[] {
 		const parts = genre.split('.');
 		const result: HierarchicalPart[] = [];
 		let currentPath = '';

 		for (let i = 0; i < parts.length; i++) {
 			if (i === 0) {
 				currentPath = parts[i];
 			} else {
 				currentPath += '.' + parts[i];
 			}
 			result.push({
 				text: parts[i],
 				fullPath: currentPath,
 				isParent: i < parts.length - 1
 			});
 		}
 		return result;
 	}

 	let hoveredGenrePath = $state<string | null>(null);
 	let hoveredTagPath = $state<string | null>(null);

	function isHierarchyPartActive(hoveredPath: string | null, partPath: string): boolean {
		return hoveredPath === partPath || !!hoveredPath?.startsWith(partPath + '.');
	}

	function isHoveredPathInHierarchy(hoveredPath: string | null, fullPath: string): boolean {
		return hoveredPath === fullPath || !!fullPath.startsWith((hoveredPath || '') + '.');
	}

	function getReadableFormats(): string[] {
		return uniqueBookFormats(files).filter((format) => {
			if (format === 'cb7') return false;
			return getReaderRouteKind(format) !== null;
		});
	}

	function getSpeedReadableFormats(): string[] {
		return uniqueBookFormats(files).filter((format) => format === 'pdf' || getReaderRouteKind(format) === 'epub');
	}

	function getPrimaryReadFormat(): string | null {
		const preferred = getPreferredBookFormat(files);
		const readable = getReadableFormats();
		if (preferred && readable.includes(preferred)) {
			return preferred;
		}
		return readable[0] || null;
	}

	function getPrimarySpeedReadFormat(): string | null {
		const speedReadable = getSpeedReadableFormats();
		const preferredText = getPreferredTextFormat(files);
		if (preferredText && speedReadable.includes(preferredText)) {
			return preferredText;
		}
		if (speedReadable.includes('pdf')) {
			return 'pdf';
		}
		return speedReadable[0] || null;
	}

	const readableFormats = $derived(getReadableFormats());
	const primaryReadFormat = $derived(getPrimaryReadFormat());
	const primarySpeedReadFormat = $derived(getPrimarySpeedReadFormat());

	function formatSize(bytes: number): string {
		if (bytes < 1024) return bytes + ' B';
		if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
		return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
	}

	function getFileName(path: string): string {
		if (!path) return 'file';
		return path.split(/[/\\]/).pop() || path;
	}

	function toggleConvertMenu(fileId: number) {
		convertMenuFileId = convertMenuFileId === fileId ? null : fileId;
	}

	function downloadFile(file: any) {
		window.location.href = `/api/books/${book.id}/files/${file.id}/download`;
	}

	function downloadConvertedFile(file: any) {
		window.location.href = `/api/books/${book.id}/files/${file.id}/convert?format=${encodeURIComponent(selectedConvertFormat)}`;
		convertMenuFileId = null;
	}

	function formatSessionDate(timestamp: number): string {
		return new Date(timestamp * 1000).toLocaleString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	function formatSessionDay(timestamp: number): string {
		return new Date(timestamp * 1000).toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric'
		});
	}

	function formatDuration(start: number, end: number | null): string {
		if (!end) return 'In progress';
		const seconds = end - start;
		if (seconds < 60) return `${seconds}s`;
		const minutes = Math.floor(seconds / 60);
		if (minutes < 60) return `${minutes}m`;
		const hours = Math.floor(minutes / 60);
		const remainingMinutes = minutes % 60;
		return `${hours}h ${remainingMinutes}m`;
	}

	function formatTime(timestamp: number): string {
		return new Date(timestamp * 1000).toLocaleTimeString('en-US', {
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	function getStatusLabel(status: string): string {
		const option = statusOptions.find(s => s.value === status);
		return option ? option.label : status;
	}

	function getReaderTypeLabel(readerType: string): string {
		switch (readerType) {
			case 'normal':
			case 'epub':
				return 'Normal Reader';
			case 'speed':
				return 'Speed Reader';
			case 'pdf':
				return 'PDF Reader';
			case 'comic':
				return 'Comic Reader';
			case 'audio':
				return 'Audio Reader';
			default:
				return readerType || 'Reader';
		}
	}

	function startEditing() {
		editForm = {
			title: book.title || '',
			series: book.series || '',
			series_number: book.series_number || '',
			publisher: book.publisher || '',
			pub_date: book.pub_date || '',
			description: book.description || '',
			rating: Math.round(book.rating || 0),
			status: book.status || 'unread',
			genres: parseJsonArray(book.genres || '[]').join(', '),
			tags: parseJsonArray(book.tags || '[]').join(', '),
			isbn: book.isbn || '',
			asin: book.asin || '',
			language: book.language || '',
			page_count: book.page_count || 0
		};
		authorsList = parseAuthors(book.authors || '[]');
		editing = true;
	}

	function cancelEditing() {
		editing = false;
		editForm = {};
	}

	async function refreshAfterMetadataApply() {
		await fetchBook();
		if (editing) {
			startEditing();
		}
		showMetadataLookup = false;
	}

	function openCoverModal() {
		if (book?.cover_path) {
			showCoverModal = true;
		}
	}

	function openShelfPicker() {
		showShelfPicker = true;
		shelfActionMessage = '';
	}

	async function addBookToShelf(shelfId: number) {
		if (!book?.id || shelfActionInProgress) return;
		shelfActionInProgress = true;
		shelfActionMessage = '';
		try {
			const res = await fetch(`/api/shelves/${shelfId}/books`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ book_id: book.id })
			});
			if (res.ok) {
				shelfActionMessage = 'Added to shelf';
				await fetchShelves();
			} else {
				shelfActionMessage = await res.text() || 'Failed to add book';
			}
		} catch (e) {
			console.error('Failed to add book to shelf:', e);
			shelfActionMessage = 'Failed to add book';
		} finally {
			shelfActionInProgress = false;
		}
	}

	function closeCoverModal() {
		showCoverModal = false;
	}

	function handleCoverModalKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && showCoverModal) {
			closeCoverModal();
		}
	}

	async function regenerateCover() {
		if (!book?.id || regeneratingCover) return;
		regeneratingCover = true;
		try {
			const res = await fetch(`/api/books/${book.id}/cover/regenerate`, {
				method: 'POST'
			});
			if (res.ok) {
				await fetchBook();
			} else {
				console.error('Failed to regenerate cover:', await res.text());
			}
		} catch (e) {
			console.error('Failed to regenerate cover:', e);
		} finally {
			regeneratingCover = false;
		}
	}

	async function saveMetadata() {
		saveError = null;
		saving = true;
		console.log('saveMetadata called, book.id:', book?.id);
		try {
			let authorsArray: string[] = authorsList.map((a: string) => normalizeAuthorName(a.trim())).filter((a: string) => a);

			let genresArray: string[] = [];
			if (typeof editForm.genres === 'string') {
				genresArray = editForm.genres.split(',').map((g: string) => g.trim()).filter((g: string) => g);
			} else if (Array.isArray(editForm.genres)) {
				genresArray = editForm.genres;
			}

			let tagsArray: string[] = [];
			if (typeof editForm.tags === 'string') {
				tagsArray = editForm.tags.split(',').map((t: string) => t.trim()).filter((t: string) => t);
			} else if (Array.isArray(editForm.tags)) {
				tagsArray = editForm.tags;
			}

 			console.log('Sending PUT to /api/books/' + book.id);
 			const res = await fetch(`/api/books/${book.id}`, {
 				method: 'PUT',
 				headers: { 'Content-Type': 'application/json' },
 				body: JSON.stringify({
 					title: editForm.title,
 					authors: authorsArray,
 					series: editForm.series,
 					series_number: editForm.series_number === '' ? 0 : parseFloat(editForm.series_number),
 					publisher: editForm.publisher,
 					pub_date: editForm.pub_date,
 					description: editForm.description,
 					rating: editForm.rating,
 					status: editForm.status,
 					genres: genresArray,
 					tags: tagsArray,
 					isbn: editForm.isbn,
					asin: editForm.asin,
 					language: editForm.language,
 					page_count: editForm.page_count
				})
			});

			console.log('Response status:', res.status);
			if (res.ok) {
				const data = await res.json();
				console.log('Save successful:', data);
				editing = false;
				editForm = {};
				await fetchBook();
			} else {
				const errorText = await res.text();
				console.error('Failed to save metadata:', res.status, errorText);
				saveError = `Failed to save: ${res.status} ${errorText}`;
			}
		} catch (e) {
			console.error('Failed to save metadata:', e);
			saveError = `Error: ${e}`;
		} finally {
			saving = false;
		}
	}

	function navigateWithFilter(field: string, value: string) {
		const url = new URL($page.url);
		url.pathname = '/library';
		if (field === 'author') {
			url.searchParams.set('author', value);
		} else if (field === 'series') {
			url.searchParams.set('series', value);
		} else if (field === 'genre') {
			url.searchParams.set('genre', value);
		} else if (field === 'tags') {
			url.searchParams.set('tags', value);
		} else if (field === 'publisher') {
			url.searchParams.set('publisher', value);
		} else if (field === 'status') {
			url.searchParams.set('status', value);
		} else if (field === 'language') {
			url.searchParams.set('language', value);
		}
		goto(url.pathname + url.search);
	}

	function navigateToLibrary() {
		if (book.library_id) {
			goto(`/library?library=${book.library_id}`);
		}
	}

	function handleTabChange(tab: 'similar' | 'sessions' | 'files') {
		activeTab = tab;
		if (tab === 'sessions') {
			fetchSessions();
		} else if (tab === 'similar') {
			fetchSimilarBooks();
		}
	}

	function setRating(value: number) {
		editForm.rating = value;
	}

	function getStarFill(rating: number, starIndex: number): boolean {
		return starIndex <= rating;
	}
</script>

	<div class="space-y-6 pb-20">
		{#if editing}
			<button onclick={cancelEditing} class="group inline-flex items-center text-[var(--color-surface-text-muted)] transition-colors duration-200 ease-out hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]">
				<svg class="mr-2 h-4 w-4 transition-colors duration-200 ease-out group-hover:text-[var(--color-surface-text)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
				</svg>
				Back to Book Details
			</button>
		{:else}
			<a href="/library" class="group inline-flex items-center text-[var(--color-surface-text-muted)] transition-colors duration-200 ease-out hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]">
				<svg class="mr-2 h-4 w-4 transition-colors duration-200 ease-out group-hover:text-[var(--color-surface-text)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
				</svg>
				Back to Library
			</a>
	{/if}

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if book}
		{#if editing}
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] p-6">
				<div class="flex items-center justify-between mb-4">
					<h3 class="text-lg font-medium text-[var(--color-surface-text)]">Edit Metadata</h3>
					<div class="flex flex-wrap items-center gap-2">
							<button
								onclick={regenerateCover}
								type="button"
								disabled={regeneratingCover}
								class="group inline-flex items-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-0.5 hover:bg-[var(--color-surface-600)] hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
							>
								{#if regeneratingCover}
									<svg class="animate-spin -ml-0.5 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
										<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
										<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
									</svg>
									Regenerating...
								{:else}
									<svg class="mr-2 h-4 w-4 transition-transform duration-200 ease-out group-hover:rotate-45 group-hover:scale-110" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
									</svg>
									Regenerate Cover
								{/if}
							</button>
							<button
								onclick={() => showMetadataLookup = true}
								type="button"
								class="group inline-flex items-center rounded-lg bg-[var(--color-primary-500)] px-3 py-1.5 text-sm text-white transition-all duration-200 ease-out hover:-translate-y-0.5 hover:bg-[var(--color-primary-600)] hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
							>
								<svg class="mr-2 h-4 w-4 transition-transform duration-200 ease-out group-hover:scale-110" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
								</svg>
								Lookup Metadata
							</button>
					</div>
				</div>
				<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
					<div>
						<label class="block text-sm text-[var(--color-surface-text-muted)] mb-1" for="book-title">Title</label>
						<input id="book-title" type="text" bind:value={editForm.title} class="w-full bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
					</div>
					<div>
						<div class="block text-sm text-[var(--color-surface-text-muted)] mb-2">Authors</div>
						<div class="space-y-2">
							{#each authorsList as author, i}
								<div class="flex items-center space-x-2">
									<input
										type="text"
										bind:value={authorsList[i]}
										placeholder="Author name"
										class="flex-1 bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
									/>
										<button
											onclick={() => authorsList.splice(i, 1)}
											class="group rounded-md p-2 text-red-400 transition-all duration-200 ease-out hover:-translate-y-0.5 hover:bg-red-500/10 hover:text-red-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400/40"
											title="Remove author"
										>
											<svg class="h-4 w-4 transition-transform duration-200 ease-out group-hover:scale-110" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
											</svg>
										</button>
									</div>
								{/each}
								<button
									onclick={() => authorsList.push('')}
									class="group inline-flex items-center rounded-lg border border-dashed border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-primary-400)] transition-all duration-200 ease-out hover:-translate-y-0.5 hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-primary-300)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
								>
									<svg class="mr-2 h-4 w-4 transition-transform duration-200 ease-out group-hover:scale-110" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
									</svg>
									Add Author
								</button>
						</div>
					</div>
					<div>
						<label class="block text-sm text-[var(--color-surface-text-muted)] mb-1" for="book-publisher">Publisher</label>
						<input id="book-publisher" type="text" bind:value={editForm.publisher} class="w-full bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
					</div>
					<div>
						<label class="block text-sm text-[var(--color-surface-text-muted)] mb-1" for="book-pub-date">Published Date</label>
						<input id="book-pub-date" type="text" bind:value={editForm.pub_date} placeholder="YYYY-MM-DD" class="w-full bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
					</div>
					<div>
						<label class="block text-sm text-[var(--color-surface-text-muted)] mb-1" for="book-language">Language</label>
						<input id="book-language" type="text" bind:value={editForm.language} class="w-full bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
					</div>
					<div>
						<label class="block text-sm text-[var(--color-surface-text-muted)] mb-1" for="book-isbn">ISBN</label>
						<input id="book-isbn" type="text" bind:value={editForm.isbn} class="w-full bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
					</div>
					<div>
						<label class="block text-sm text-[var(--color-surface-text-muted)] mb-1" for="book-asin">ASIN</label>
						<input id="book-asin" type="text" bind:value={editForm.asin} class="w-full bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
					</div>
					<div>
						<label class="block text-sm text-[var(--color-surface-text-muted)] mb-1" for="book-pages">Pages</label>
						<input id="book-pages" type="number" bind:value={editForm.page_count} min="0" class="w-full bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
					</div>
					<div>
						<div class="block text-sm text-[var(--color-surface-text-muted)] mb-2">Status</div>
						<div class="flex gap-2">
							{#each statusOptions as option}
									<button
										type="button"
										onclick={() => editForm.status = option.value}
										class="flex-1 rounded-lg border px-3 py-2 text-sm font-medium transition-all duration-200 ease-out {editForm.status === option.value ? 'bg-[var(--color-primary-500)] border-[var(--color-primary-500)] text-white shadow-sm' : 'bg-[var(--color-surface-700)] border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:-translate-y-0.5 hover:bg-[var(--color-surface-600)] hover:shadow-sm'}"
									>
										{option.label}
									</button>
								{/each}
						</div>
					</div>
					<div class="md:col-span-2">
						<div class="block text-sm text-[var(--color-surface-text-muted)] mb-1">Rating</div>
						<div class="flex items-center gap-1">
							{#each [1, 2, 3, 4, 5, 6, 7, 8, 9, 10] as star}
									<button
										type="button"
										onclick={() => setRating(star)}
										class="text-2xl transition-all duration-200 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-yellow-400/40 focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] {star <= editForm.rating ? 'text-yellow-400' : 'text-[var(--color-surface-600)]'} hover:-translate-y-0.5 hover:scale-110 hover:text-yellow-400"
									>
										★
									</button>
								{/each}
							<span class="ml-2 text-sm text-[var(--color-surface-text-muted)]">({editForm.rating}/10)</span>
						</div>
					</div>
					<div class="md:col-span-2">
						<div class="block text-sm text-[var(--color-surface-text-muted)] mb-1">Series</div>
						<div class="flex gap-2">
							<input id="book-series" type="text" bind:value={editForm.series} placeholder="Series name" class="flex-1 bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
							<input id="book-series-number" type="number" bind:value={editForm.series_number} min="0" placeholder="#" class="w-20 bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
						</div>
					</div>
					<div class="md:col-span-2">
						<label class="block text-sm text-[var(--color-surface-text-muted)] mb-1" for="book-genres">Genres</label>
						<AutocompleteInput
							id="book-genres"
							bind:value={editForm.genres}
							placeholder="Fiction, Science Fiction.Space Opera, History"
							field="genres"
							onchange={(v) => editForm.genres = v}
						/>
						<p class="text-xs text-[var(--color-surface-text-muted)] mt-1">Comma-separated. Use "Parent.Child" for hierarchies.</p>
					</div>
					<div class="md:col-span-2">
						<label class="block text-sm text-[var(--color-surface-text-muted)] mb-1" for="book-tags">Tags</label>
						<AutocompleteInput
							id="book-tags"
							bind:value={editForm.tags}
							placeholder="Favorite, Classics, Must Read"
							field="tags"
							onchange={(v) => editForm.tags = v}
						/>
						<p class="text-xs text-[var(--color-surface-text-muted)] mt-1">Comma-separated. Use "Parent.Child" for hierarchies.</p>
					</div>
 					<div class="md:col-span-2">
						<label class="block text-sm text-[var(--color-surface-text-muted)] mb-1" for="book-description">Description</label>
						<textarea id="book-description" bind:value={editForm.description} rows="4" class="w-full bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)] resize-y"></textarea>
					</div>
				</div>
					<div class="flex justify-end gap-3 mt-6">
						<button onclick={cancelEditing} disabled={saving} class="rounded-lg bg-[var(--color-surface-700)] px-4 py-2 font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none">Cancel</button>
						<button onclick={saveMetadata} disabled={saving} class="rounded-lg bg-[var(--color-primary-500)] px-4 py-2 font-medium text-white transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-primary-600)] hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none">
							{saving ? 'Saving...' : 'Save Changes'}
						</button>
					</div>
				{#if saveError}
					<div class="mt-4 p-3 bg-red-500/20 border border-red-500/50 rounded-lg text-red-400 text-sm">
						{saveError}
					</div>
				{/if}
			</div>
		{:else}
				<div class="flex items-start justify-between gap-4">
					<div class="flex-1 min-w-0">
						<h1 class="text-2xl xl:text-3xl font-bold text-[var(--color-surface-text)] break-words">{book.title || 'Untitled'}</h1>
								<div class="flex flex-wrap items-center gap-x-2 gap-y-1 mt-3">
								{#each parseAuthors(book.authors || '[]') as author, i}
									{#if i > 0}<span class="text-[var(--color-surface-text-muted)]">,</span>{/if}
									<button
										onclick={() => navigateWithFilter('author', author)}
										class="inline-flex items-center rounded-md px-1.5 py-0.5 text-[var(--color-primary-400)] transition-colors duration-200 ease-out hover:bg-[var(--color-primary-500)]/12 hover:text-[var(--color-primary-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
									>
										{author}
									</button>
								{/each}
						</div>
					</div>
						<button
							onclick={startEditing}
							class="group flex-shrink-0 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] p-2 text-[var(--color-surface-text)] transition-colors duration-200 ease-out hover:bg-[var(--color-surface-600)] hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
							title="Edit Metadata"
						>
							<svg class="h-5 w-5 transition-colors duration-200 ease-out group-hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z"></path>
							</svg>
						</button>
			</div>

				<div class="flex flex-col md:flex-row gap-6 md:items-start">
					<div class="w-full max-w-[13rem] mx-auto md:mx-0 flex-shrink-0 flex flex-col">
							<button
								type="button"
								onclick={openCoverModal}
								class="group block w-full text-left transition-all duration-200 ease-out {book.cover_path ? 'cursor-zoom-in hover:-translate-y-0.5 hover:shadow-lg' : 'cursor-default'} focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
								disabled={!book.cover_path}
								title={book.cover_path ? 'Open cover preview' : undefined}
							>
							<BookCoverFrame
								src={book.cover_path ? `/api/covers/${book.id}` : null}
								alt={book.title}
								mode="contain"
								frameClass="aspect-[2/3] w-full"
								imageClass="transition-transform duration-200 ease-out group-hover:scale-[1.02]"
							/>
						</button>

					<div class="mt-4">
						<div class="flex items-center justify-between mb-2">
							<span class="text-sm text-[var(--color-surface-text-muted)]">Progress</span>
							<span class="text-sm font-medium text-[var(--color-surface-text)]">{Math.round(book.percent || 0)}%</span>
						</div>
						<div class="w-full h-2 bg-[var(--color-surface-700)] rounded-full overflow-hidden">
							<div
								class="h-full bg-[var(--color-primary-500)] rounded-full transition-all duration-300"
								style="width: {book.percent || 0}%"
							></div>
						</div>
					</div>
					<div class="mt-3">
						<div class="flex items-center justify-between mb-2">
							<span class="text-sm text-[var(--color-surface-text-muted)]">Speed Reader</span>
							<span class="text-sm font-medium text-[var(--color-surface-text)]">{Math.round(book.speed_reader_percent || 0)}%</span>
						</div>
						<div class="w-full h-2 bg-[var(--color-surface-700)] rounded-full overflow-hidden">
							<div
								class="h-full bg-[var(--color-primary-500)]/70 rounded-full transition-all duration-300"
								style="width: {book.speed_reader_percent || 0}%"
							></div>
						</div>
					</div>

					<div class="mt-3 mb-4 md:mb-0 flex flex-col gap-2">
						{#if primaryReadFormat}
							<div class="relative">
								<div class="flex w-full overflow-hidden rounded-lg">
										<a
											href={getBookReaderHref(book.id, primaryReadFormat)}
											class="flex min-w-0 flex-1 items-center justify-between gap-3 bg-[var(--color-primary-500)] px-3 py-2 text-sm font-medium text-white transition-colors duration-200 ease-out hover:bg-[var(--color-primary-600)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] sm:px-4"
										>
											<span class="truncate">{book.opened && book.percent > 0 ? 'Continue Reading' : 'Read Now'}</span>
											<span class="text-[10px] uppercase tracking-[0.14em] text-white/80">{getFormatDisplayLabel(primaryReadFormat)}</span>
										</a>
										{#if readableFormats.length > 1}
											<button
												type="button"
												onclick={() => formatMenuOpen = !formatMenuOpen}
												class="inline-flex items-center justify-center border-l border-white/20 bg-[var(--color-primary-500)] px-3 text-white transition-colors duration-200 ease-out hover:bg-[var(--color-primary-600)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
												aria-label="Choose reader format"
												aria-expanded={formatMenuOpen}
											>
												<svg class="h-4 w-4 transition-transform duration-200 ease-out {formatMenuOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
													<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
												</svg>
											</button>
										{/if}
								</div>
								{#if formatMenuOpen && readableFormats.length > 1}
									<div class="absolute right-0 top-full z-20 mt-2 w-56 overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-800)] shadow-lg">
											{#each readableFormats.filter((format) => format !== primaryReadFormat) as format}
												<a
													href={getBookReaderHref(book.id, format)}
													onclick={() => formatMenuOpen = false}
													class="flex items-center justify-between px-3 py-2 text-sm text-[var(--color-surface-text)] transition-colors duration-200 hover:bg-[var(--color-surface-700)]"
												>
												<span>{getFormatDisplayLabel(format)}</span>
												<span class="text-xs uppercase tracking-[0.12em] text-[var(--color-surface-text-muted)]">
													{getReaderRouteKind(format) || 'reader'}
												</span>
											</a>
										{/each}
									</div>
								{/if}
							</div>
						{/if}
							{#if primarySpeedReadFormat}
									<a
										href={getSpeedReaderHref(book.id, primarySpeedReadFormat)}
										class="group flex w-full items-center justify-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-colors duration-200 ease-out hover:border-[var(--color-surface-500)] hover:bg-[var(--color-surface-600)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] sm:px-4"
									>
										<svg class="mr-2 h-4 w-4 transition-colors duration-200 ease-out group-hover:text-[var(--color-primary-300)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
										</svg>
										Speed Read
									</a>
								{/if}
								<button
									type="button"
									onclick={openShelfPicker}
									class="group flex w-full items-center justify-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-colors duration-200 ease-out hover:border-[var(--color-surface-500)] hover:bg-[var(--color-surface-600)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] sm:px-4"
								>
									<svg class="mr-2 h-4 w-4 transition-colors duration-200 ease-out group-hover:text-[var(--color-primary-300)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
									</svg>
									Add to Shelf
								</button>
						</div>
				</div>

				<div class="flex-1 min-w-0">
					<div class="grid grid-cols-2 gap-x-6 gap-y-3">
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Library</dt>
							<dd class="text-sm text-[var(--color-surface-text)]">
								{#if book.library_name}
									<button
										onclick={navigateToLibrary}
											class="text-[var(--color-primary-400)] transition-colors duration-200 ease-out hover:text-[var(--color-primary-300)] hover:underline focus-visible:outline-none focus-visible:underline"
									>
										{book.library_name}
									</button>
								{:else}
									<span class="text-[var(--color-surface-text-muted)]">-</span>
								{/if}
							</dd>
						</div>
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Publisher</dt>
							<dd class="text-sm text-[var(--color-surface-text)]">
								{#if book.publisher}
									<button
										onclick={() => navigateWithFilter('publisher', book.publisher)}
											class="text-[var(--color-primary-400)] transition-colors duration-200 ease-out hover:text-[var(--color-primary-300)] hover:underline focus-visible:outline-none focus-visible:underline"
									>
										{book.publisher}
									</button>
								{:else}
									<span class="text-[var(--color-surface-text-muted)]">-</span>
								{/if}
							</dd>
						</div>
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Published</dt>
							<dd class="text-sm text-[var(--color-surface-text)]">
								{#if book.pub_date}
									<button
										onclick={() => navigateWithFilter('pub_date', book.pub_date)}
											class="text-[var(--color-primary-400)] transition-colors duration-200 ease-out hover:text-[var(--color-primary-300)] hover:underline focus-visible:outline-none focus-visible:underline"
									>
										{book.pub_date}
									</button>
								{:else}
									<span class="text-[var(--color-surface-text-muted)]">-</span>
								{/if}
							</dd>
						</div>
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Language</dt>
							<dd class="text-sm text-[var(--color-surface-text)]">
								{#if book.language}
									<button
										onclick={() => navigateWithFilter('language', book.language)}
											class="text-[var(--color-primary-400)] transition-colors duration-200 ease-out hover:text-[var(--color-primary-300)] hover:underline focus-visible:outline-none focus-visible:underline"
									>
										{book.language}
									</button>
								{:else}
									<span class="text-[var(--color-surface-text-muted)]">-</span>
								{/if}
							</dd>
						</div>
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Pages</dt>
							<dd class="text-sm text-[var(--color-surface-text)]">{book.page_count || '-'}</dd>
						</div>
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">ISBN</dt>
							<dd class="text-sm text-[var(--color-surface-text)] font-mono">{book.isbn || '-'}</dd>
						</div>
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">ASIN</dt>
							<dd class="text-sm text-[var(--color-surface-text)] font-mono">{book.asin || '-'}</dd>
						</div>
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Rating</dt>
							<dd class="text-sm text-[var(--color-surface-text)] flex items-center gap-0.5">
								{#if book.rating}
									{#each [1, 2, 3, 4, 5, 6, 7, 8, 9, 10] as star}
										<span class="{star <= book.rating ? 'text-yellow-400' : 'text-[var(--color-surface-600)]'}">★</span>
									{/each}
									<span class="ml-1 text-[var(--color-surface-text-muted)]">({Math.round(book.rating)}/10)</span>
								{:else}
									<span class="text-[var(--color-surface-text-muted)]">-</span>
								{/if}
							</dd>
						</div>
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Status</dt>
								<dd class="text-sm">
									<button
										onclick={() => navigateWithFilter('status', book.status || 'reading')}
										class="rounded-full px-2 py-0.5 text-xs font-medium transition-colors duration-200 ease-out {book.status === 'reading' ? 'bg-blue-500/20 text-blue-400' : book.status === 'finished' ? 'bg-emerald-500/20 text-emerald-400' : 'bg-[var(--color-surface-700)] text-[var(--color-surface-text)]'} hover:opacity-85 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
									>
										{book.status === 'reading' ? 'Currently Reading' : book.status === 'finished' ? 'Already Read' : 'Unread'}
									</button>
							</dd>
						</div>
						{#if uniqueBookFormats(files).length > 0}
							<div class="flex items-start gap-2 col-span-2">
								<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Formats</dt>
								<dd class="flex flex-wrap gap-2 min-w-0">
									{#each uniqueBookFormats(files) as format}
										<span class="rounded-full border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-2.5 py-0.5 text-xs font-medium uppercase tracking-[0.08em] text-[var(--color-surface-text)]">
											{getFormatDisplayLabel(format)}
										</span>
									{/each}
								</dd>
							</div>
						{/if}
								{#if book.series}
									<div class="flex items-start gap-2 col-span-2">
										<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Series</dt>
										<dd class="text-sm text-[var(--color-surface-text)]">
											<button
												onclick={() => navigateWithFilter('series', book.series)}
												class="text-[var(--color-primary-400)] transition-colors duration-200 ease-out hover:text-[var(--color-primary-300)] hover:underline focus-visible:outline-none focus-visible:underline"
											>
												{book.series}
											</button>
									{#if book.series_number}
										<span class="text-[var(--color-surface-text-muted)]"> #{book.series_number}</span>
									{/if}
								</dd>
							</div>
						{/if}
					</div>

					{#if (book.genres && book.genres !== '[]') || (book.tags && book.tags !== '[]')}
						<div class="mt-6 space-y-4">
							{#if book.genres && book.genres !== '[]'}
								<div class="flex items-start gap-2">
									<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Genres</dt>
									<dd class="flex flex-wrap gap-2 min-w-0">
										{#each parseJsonArray(book.genres) as genre}
											{@const parts = parseHierarchicalGenre(genre)}
											<div class="relative group">
													<div class="inline-flex items-center rounded-full border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-1 text-sm text-[var(--color-surface-text)] transition-colors duration-200 ease-out hover:border-[var(--color-surface-500)] hover:bg-[var(--color-surface-600)]">
														{#each parts as part, i}
															<button
																onclick={() => navigateWithFilter('genre', part.fullPath)}
																class="transition-colors duration-200 ease-out hover:text-[var(--color-primary-300)] hover:underline focus-visible:outline-none focus-visible:underline {isHierarchyPartActive(hoveredGenrePath, part.fullPath) ? 'text-[var(--color-primary-300)]' : 'text-[var(--color-primary-400)]'}"
																onmouseenter={() => hoveredGenrePath = part.fullPath}
																onmouseleave={() => hoveredGenrePath = null}
															>{part.text}</button>{#if i < parts.length - 1}<span class="{isHierarchyPartActive(hoveredGenrePath, parts[i + 1].fullPath) ? 'text-[var(--color-primary-400)]' : 'text-[var(--color-surface-text-muted)]'}">.</span>{/if}
														{/each}
												</div>
												{#if isHoveredPathInHierarchy(hoveredGenrePath, genre)}
													<div class="absolute bottom-full left-0 mb-1 px-2 py-1 bg-[var(--color-surface-800)] text-[var(--color-surface-text)] text-xs rounded shadow-lg whitespace-nowrap z-10">
														Click to filter by: <span class="text-[var(--color-primary-400)]">{hoveredGenrePath}</span>
													</div>
												{/if}
											</div>
										{/each}
									</dd>
								</div>
							{/if}

							{#if book.tags && book.tags !== '[]'}
								<div class="flex items-start gap-2">
									<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Tags</dt>
									<dd class="flex flex-wrap gap-2 min-w-0">
										{#each parseJsonArray(book.tags) as tag}
											{@const parts = parseHierarchicalGenre(tag)}
											<div class="relative group">
													<div class="inline-flex items-center rounded-full border border-[var(--color-primary-500)]/40 bg-[var(--color-primary-500)]/20 px-3 py-1 text-sm text-[var(--color-primary-400)] transition-colors duration-200 ease-out hover:border-[var(--color-primary-500)]/60 hover:bg-[var(--color-primary-500)]/30">
														{#each parts as part, i}
															<button
																onclick={() => navigateWithFilter('tags', part.fullPath)}
																class="transition-colors duration-200 ease-out hover:text-[var(--color-primary-200)] hover:underline focus-visible:outline-none focus-visible:underline {isHierarchyPartActive(hoveredTagPath, part.fullPath) ? 'text-[var(--color-primary-200)]' : 'text-[var(--color-primary-400)]'}"
																onmouseenter={() => hoveredTagPath = part.fullPath}
																onmouseleave={() => hoveredTagPath = null}
															>{part.text}</button>{#if i < parts.length - 1}<span class="{isHierarchyPartActive(hoveredTagPath, parts[i + 1].fullPath) ? 'text-[var(--color-primary-200)]' : 'text-[var(--color-primary-500)]'}">.</span>{/if}
														{/each}
												</div>
												{#if isHoveredPathInHierarchy(hoveredTagPath, tag)}
													<div class="absolute bottom-full left-0 mb-1 px-2 py-1 bg-[var(--color-surface-800)] text-[var(--color-surface-text)] text-xs rounded shadow-lg whitespace-nowrap z-10">
														Click to filter by: <span class="text-[var(--color-primary-400)]">{hoveredTagPath}</span>
													</div>
												{/if}
											</div>
										{/each}
									</dd>
								</div>
							{/if}
						</div>
					{/if}

					{#if book.description}
						<div class="mt-6">
							<p class="text-sm text-[var(--color-surface-text)] whitespace-pre-wrap line-clamp-3">{book.description}</p>
						</div>
					{/if}
				</div>
			</div>

			<div class="border-t border-[var(--color-surface-border)] pt-6">
					<div class="flex gap-6">
						<button
							onclick={() => handleTabChange('similar')}
							class="flex items-center gap-2 -mb-px border-b-2 px-4 py-2 text-sm font-medium transition-all duration-200 ease-out {activeTab === 'similar' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:-translate-y-px hover:border-[var(--color-surface-border)] hover:text-[var(--color-surface-text)]'}"
						>
							Similar Books
						{#if activeTab === 'similar' && similarLoading}
							<svg class="h-4 w-4 animate-spin text-[var(--color-primary-500)]" viewBox="0 0 24 24" fill="none" aria-hidden="true">
								<circle class="opacity-25" cx="12" cy="12" r="9" stroke="currentColor" stroke-width="2"></circle>
								<path class="opacity-75" fill="currentColor" d="M12 3a9 9 0 0 1 9 9h-2.5a6.5 6.5 0 0 0-6.5-6.5V3z"></path>
							</svg>
						{/if}
						</button>
						<button
							onclick={() => handleTabChange('sessions')}
							class="border-b-2 -mb-px px-4 py-2 text-sm font-medium transition-all duration-200 ease-out {activeTab === 'sessions' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:-translate-y-px hover:border-[var(--color-surface-border)] hover:text-[var(--color-surface-text)]'}"
						>
							Reading Sessions
						</button>
						<button
							onclick={() => handleTabChange('files')}
							class="border-b-2 -mb-px px-4 py-2 text-sm font-medium transition-all duration-200 ease-out {activeTab === 'files' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:-translate-y-px hover:border-[var(--color-surface-border)] hover:text-[var(--color-surface-text)]'}"
						>
							Files
						</button>
				</div>

				<div class="mt-4">
					{#if activeTab === 'sessions'}
						{#if sessionsLoading}
							<div class="flex justify-center py-8">
								<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--color-primary-500)]"></div>
							</div>
						{:else if sessions.length === 0}
							<div class="text-center py-8 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
								<p class="text-[var(--color-surface-text-muted)]">No reading sessions yet</p>
							</div>
						{:else}
							<div class="space-y-3">
									{#each sessions as session}
										<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4 transition-all duration-200 ease-out hover:-translate-y-0.5 hover:border-[var(--color-surface-500)] hover:shadow-sm">
											<div class="flex items-start justify-between gap-4">
											<div class="min-w-0">
												<div class="text-sm font-medium text-[var(--color-surface-text)]">{formatSessionDay(session.started_at)}</div>
												<div class="mt-1 text-xs text-[var(--color-surface-text-muted)]">
													{formatTime(session.started_at)} → {session.ended_at ? formatTime(session.ended_at) : 'In progress'}
												</div>
												<div class="mt-2 text-xs text-[var(--color-surface-text-muted)]">
													Duration: <span class="text-[var(--color-surface-text)]">{formatDuration(session.started_at, session.ended_at)}</span>
												</div>
											</div>
											<div class="flex items-center gap-2 flex-shrink-0">
													<span class="px-2 py-1 rounded-full text-xs font-medium bg-[var(--color-primary-500)]/20 text-[var(--color-primary-300)]">
														{getReaderTypeLabel(session.reader_type)}
													</span>
													<button
														onclick={() => deleteSession(session.id)}
														class="group rounded-lg bg-red-500/10 p-2 text-red-400 transition-all duration-200 ease-out hover:-translate-y-0.5 hover:bg-red-500/20 hover:text-red-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400/40"
														title="Delete session"
													>
														<svg class="h-4 w-4 transition-transform duration-200 ease-out group-hover:scale-110" fill="none" stroke="currentColor" viewBox="0 0 24 24">
															<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
														</svg>
													</button>
											</div>
										</div>
									</div>
								{/each}
							</div>
						{/if}
					{:else if activeTab === 'files'}
						<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] overflow-visible">
							{#if files.length === 0}
								<div class="text-center py-8">
									<p class="text-[var(--color-surface-text-muted)]">No files found</p>
								</div>
							{:else}
									<div class="divide-y divide-[var(--color-surface-border)]">
										{#each files as file}
											<div class="group flex items-center justify-between p-4 transition-all duration-200 ease-out hover:bg-[var(--color-surface-700)]/50">
												<div class="flex items-center space-x-3 min-w-0">
													<svg class="h-5 w-5 flex-shrink-0 text-[var(--color-primary-500)] transition-transform duration-200 ease-out group-hover:scale-110" fill="none" stroke="currentColor" viewBox="0 0 24 24">
														<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
													</svg>
												<div class="min-w-0">
													<p class="text-sm font-medium text-[var(--color-surface-text)] truncate">{getFileName(file.path)}</p>
													<div class="mt-1 flex items-center gap-2 text-xs text-[var(--color-surface-text-muted)]">
														<span class="px-2 py-0.5 rounded-full bg-[var(--color-surface-700)] text-[var(--color-surface-text)] uppercase">{file.format}</span>
														<span>{formatSize(file.size)}</span>
													</div>
												</div>
												</div>
												<div class="flex items-center gap-2">
													<button
														onclick={() => downloadFile(file)}
														class="rounded-lg bg-[var(--color-primary-500)] px-3 py-1.5 text-sm font-medium text-white transition-all duration-200 ease-out hover:-translate-y-0.5 hover:bg-[var(--color-primary-600)] hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
													>
														Download
													</button>
													<div class="relative">
														<button
															onclick={() => toggleConvertMenu(file.id)}
															class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-1.5 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-0.5 hover:bg-[var(--color-surface-600)] hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
														>
															Convert
														</button>
													{#if convertMenuFileId === file.id}
														<div class="absolute right-0 mt-2 w-64 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-xl z-20 p-3 space-y-3">
															<div>
																<div class="text-sm font-medium text-[var(--color-surface-text)]">Convert format</div>
																<div class="text-xs text-[var(--color-surface-text-muted)]">Choose a download format before saving.</div>
															</div>
															<label class="block space-y-1">
																<span class="text-xs text-[var(--color-surface-text-muted)]">Format</span>
																<select
																	bind:value={selectedConvertFormat}
																	class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)]"
																>
																	<option value="epub">EPUB</option>
																	<option value="fb2">FB2</option>
																	<option value="txt">TXT</option>
																	<option value="rtf">RTF</option>
																</select>
															</label>
																<div class="flex items-center justify-end gap-2">
																	<button
																		onclick={() => convertMenuFileId = null}
																		class="rounded-lg px-3 py-2 text-sm text-[var(--color-surface-text-muted)] transition-all duration-200 ease-out hover:-translate-y-px hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-overlay)]"
																	>
																		Cancel
																	</button>
																	<button
																		onclick={() => downloadConvertedFile(file)}
																		class="rounded-lg bg-[var(--color-primary-500)] px-3 py-2 text-sm font-medium text-white transition-all duration-200 ease-out hover:-translate-y-0.5 hover:bg-[var(--color-primary-600)] hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-overlay)]"
																	>
																		Download Converted
																	</button>
															</div>
														</div>
													{/if}
												</div>
											</div>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{:else if activeTab === 'similar'}
						{#if similarLoading}
							<div class="flex items-center justify-center gap-3 py-8 text-[var(--color-surface-text-muted)]">
								<svg class="h-5 w-5 animate-spin text-[var(--color-primary-500)]" viewBox="0 0 24 24" fill="none" aria-hidden="true">
									<circle class="opacity-25" cx="12" cy="12" r="9" stroke="currentColor" stroke-width="2"></circle>
									<path class="opacity-75" fill="currentColor" d="M12 3a9 9 0 0 1 9 9h-2.5a6.5 6.5 0 0 0-6.5-6.5V3z"></path>
								</svg>
								<span>Finding similar books...</span>
							</div>
						{:else if similarBooks.length === 0}
							<div class="text-center py-8 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
								<p class="text-[var(--color-surface-text-muted)]">No similar books found</p>
							</div>
						{:else}
							<div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
										{#each similarBooks as similar}
										<a href="/book/{similar.id}" class="group block rounded-xl p-2 transition-all duration-200 ease-out hover:-translate-y-1 hover:bg-[var(--color-surface-overlay)] hover:shadow-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]">
											<BookCoverFrame
												src={similar.cover_path ? `/api/covers/${similar.id}` : null}
												alt={similar.title}
												mode="contain"
												frameClass="aspect-[2/3] w-full mb-2"
												imageClass="transition-all duration-200 ease-out group-hover:scale-[1.02] group-hover:opacity-80"
												placeholderSize="md"
											/>
											<h4 class="truncate text-sm font-medium text-[var(--color-surface-text)] transition-colors duration-200 group-hover:text-[var(--color-primary-400)]">{similar.title}</h4>
											<p class="text-xs text-[var(--color-surface-text-muted)] truncate">{parseAuthors(similar.authors).join(', ')}</p>
										</a>
									{/each}
							</div>
						{/if}
					{:else}
						<div class="text-center py-8 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
							<p class="text-[var(--color-surface-text-muted)]">Similar books coming soon</p>
						</div>
					{/if}
				</div>
			</div>
		{/if}
	{:else}
		<div class="text-center py-16 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
			<p class="text-[var(--color-surface-text-muted)]">Book not found</p>
		</div>
	{/if}
</div>

{#if showMetadataLookup && book?.id}
	<MetadataLookupModal
		bookIds={[book.id]}
		title="Lookup Metadata"
		onClose={() => showMetadataLookup = false}
		onApplied={refreshAfterMetadataApply}
	/>
{/if}

{#if showShelfPicker}
	<div class="fixed inset-0 z-[80] flex items-center justify-center p-4">
		<button
			type="button"
			class="absolute inset-0 bg-black/70"
			aria-label="Close shelf picker"
			onclick={() => showShelfPicker = false}
		></button>
		<div class="relative w-full max-w-md overflow-hidden rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
			<div class="border-b border-[var(--color-surface-border)] px-5 py-4">
				<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Add to Shelf</h3>
				<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">{book.title || 'This book'}</p>
			</div>
			<div class="max-h-[60vh] overflow-y-auto p-4">
							{#if shelves.length === 0}
								<div class="py-6 text-center text-sm text-[var(--color-surface-text-muted)]">
									No shelves yet.
								</div>
							{:else}
								<div class="space-y-2">
									{#each shelves as shelf}
										<button
											type="button"
											onclick={() => addBookToShelf(shelf.id)}
											disabled={shelfActionInProgress}
											class="group flex w-full items-center justify-between rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-4 py-3 text-left transition-all duration-200 ease-out hover:-translate-y-0.5 hover:bg-[var(--color-surface-700)] hover:shadow-sm disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
										>
											<span class="flex min-w-0 items-center gap-3">
												<span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition-transform duration-200 ease-out {shelf.is_magic === 1 ? 'bg-purple-500/20 text-purple-300' : 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-400)]'} group-hover:scale-105">
													<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
														<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
													</svg>
												</span>
												<span class="min-w-0">
													<span class="block truncate text-sm font-medium text-[var(--color-surface-text)]">{shelf.name}</span>
													<span class="block text-xs text-[var(--color-surface-text-muted)]">{shelf.book_count} books{#if shelf.is_magic === 1} · Magic{/if}</span>
												</span>
											</span>
											<span class="text-xs font-medium text-[var(--color-primary-400)] transition-transform duration-200 ease-out group-hover:translate-x-0.5">Add</span>
										</button>
									{/each}
								</div>
							{/if}
			</div>
					<div class="flex items-center justify-between gap-3 border-t border-[var(--color-surface-border)] px-5 py-4">
						{#if shelfActionMessage}
							<p class="text-sm text-[var(--color-surface-text-muted)]">{shelfActionMessage}</p>
						{:else}
							<span class="text-sm text-[var(--color-surface-text-muted)]">Choose a shelf to add this book.</span>
						{/if}
						<div class="flex items-center gap-3">
							<a href="/shelves/new" class="text-sm text-[var(--color-primary-400)] transition-colors duration-200 hover:text-[var(--color-primary-300)] focus-visible:outline-none focus-visible:underline">
								New shelf
							</a>
							<button
								type="button"
								onclick={() => showShelfPicker = false}
								class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-700)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-overlay)]"
							>
								Close
							</button>
						</div>
			</div>
		</div>
	</div>
{/if}

{#if showCoverModal && book}
	<div class="fixed inset-0 z-[140] flex items-center justify-center p-4">
		<button
			type="button"
			class="absolute inset-0 bg-black/75"
			onclick={closeCoverModal}
			aria-label="Close cover preview"
		></button>
		<div class="relative z-[1] w-full max-w-3xl max-h-[90vh] overflow-auto rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
			<div class="flex items-center justify-between gap-4 border-b border-[var(--color-surface-border)] px-4 py-3">
				<div class="min-w-0">
					<p class="text-sm font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Cover Preview'}</p>
					<p class="text-xs text-[var(--color-surface-text-muted)]">Cover preview</p>
				</div>
					<button
						type="button"
						onclick={closeCoverModal}
						class="rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-overlay)]"
					>
						Close
					</button>
			</div>
			<div class="flex justify-center p-4">
				<div class="w-full max-w-[24rem]">
					<BookCoverFrame
						src={book.cover_path ? `/api/covers/${book.id}` : null}
						alt={book.title}
						mode="contain"
						frameClass="aspect-[2/3] w-full"
						imageClass="object-contain p-2"
					/>
				</div>
			</div>
		</div>
	</div>
{/if}

<svelte:window onkeydown={handleCoverModalKeydown} />

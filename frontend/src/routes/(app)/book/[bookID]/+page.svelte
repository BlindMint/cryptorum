<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import AutocompleteInput from '$lib/components/AutocompleteInput.svelte';
	import MetadataLookupModal from '$lib/components/MetadataLookupModal.svelte';

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
	let saveError = $state<string | null>(null);
	let showMetadataLookup = $state(false);
	let convertMenuFileId = $state<number | null>(null);
	let selectedConvertFormat = $state<'epub' | 'fb2' | 'txt' | 'rtf'>('epub');

	let editForm = $state<any>({});
	let authorsList = $state<string[]>([]);

	let statusOptions = [
		{ value: 'unread', label: 'Unread' },
		{ value: 'reading', label: 'Currently Reading' },
		{ value: 'finished', label: 'Already Read' }
	];

	onMount(async () => {
		await fetchBook();
	});

	$effect(() => {
		const bookId = $page.params.bookID;
		if (bookId) {
			fetchBook();
		}
	});

	async function fetchBook() {
		loading = true;
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
		if (similarBooks.length > 0) return;
		similarLoading = true;
		const bookId = $page.params.bookID;
		try {
			const res = await fetch(`/api/books/${bookId}/similar?limit=6`);
			if (res.ok) {
				similarBooks = await res.json();
			}
		} catch (e) {
			console.error('Failed to fetch similar books:', e);
		} finally {
			similarLoading = false;
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

	function getReaderUrl(format: string): string {
		const f = format.toLowerCase();
		if (['mp3', 'm4b', 'm4a', 'opus', 'ogg', 'aac'].includes(f)) return `/reader/audio/${book.id}`;
		if (['pdf'].includes(f)) return `/reader/pdf/${book.id}`;
		if (['cbz', 'cbr', 'cb7', 'cbt'].includes(f)) return `/reader/cbx/${book.id}`;
		return `/reader/epub/${book.id}`;
	}

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
		<button onclick={cancelEditing} class="inline-flex items-center text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] transition-colors">
			<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
			</svg>
			Back to Book Details
		</button>
	{:else}
		<a href="/library" class="inline-flex items-center text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] transition-colors">
			<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
					<button
						onclick={() => showMetadataLookup = true}
						type="button"
						class="inline-flex items-center px-3 py-1.5 text-sm bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white rounded transition-colors"
					>
						<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
						</svg>
						Lookup Metadata
					</button>
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
										class="p-2 text-red-400 hover:text-red-300 transition-colors"
										title="Remove author"
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
										</svg>
									</button>
								</div>
							{/each}
							<button
								onclick={() => authorsList.push('')}
								class="inline-flex items-center px-3 py-2 text-sm text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)] border border-dashed border-[var(--color-surface-border)] rounded hover:border-[var(--color-primary-500)] transition-colors"
							>
								<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
									class="flex-1 px-3 py-2 rounded-lg border text-sm font-medium transition-all {editForm.status === option.value ? 'bg-[var(--color-primary-500)] border-[var(--color-primary-500)] text-white' : 'bg-[var(--color-surface-700)] border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-600)]'}"
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
									class="text-2xl focus:outline-none transition-colors {star <= editForm.rating ? 'text-yellow-400' : 'text-[var(--color-surface-600)]'} hover:text-yellow-400"
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
					<button onclick={cancelEditing} disabled={saving} class="px-4 py-2 rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] disabled:opacity-50 text-[var(--color-surface-text)] font-medium transition-colors">Cancel</button>
					<button onclick={saveMetadata} disabled={saving} class="px-4 py-2 rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] disabled:opacity-50 text-white font-medium transition-colors">
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
									class="text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)] hover:underline"
								>
									{author}
								</button>
							{/each}
						</div>
					</div>
				<button
					onclick={startEditing}
					class="p-2 rounded-lg bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] transition-colors flex-shrink-0"
					title="Edit Metadata"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z"></path>
					</svg>
				</button>
			</div>

			<div class="flex flex-col md:flex-row gap-6">
				<div class="w-full md:w-64 xl:w-72 flex-shrink-0">
					<div class="aspect-[2/3] bg-slate-800 rounded-lg overflow-hidden border border-[var(--color-surface-border)]">
						{#if book.cover_path}
							<img src="/api/covers/{book.id}" alt={book.title} class="w-full h-full object-cover">
						{:else}
							<div class="w-full h-full flex items-center justify-center">
								<svg class="w-12 h-12 text-slate-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
								</svg>
							</div>
						{/if}
					</div>

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

					<div class="mt-4 flex flex-col gap-2">
						<a
							href={getReaderUrl(files[0]?.format || 'epub')}
							class="flex items-center justify-center w-full px-4 py-2.5 bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white font-medium rounded-lg transition-colors text-sm"
						>
							{book.opened && book.percent > 0 ? 'Continue Reading' : 'Read Now'}
						</a>
						{#if files.some(f => ['epub', 'pdf'].includes(f.format?.toLowerCase()))}
							<a
								href="/reader/speed/{book.id}"
								class="flex items-center justify-center w-full px-4 py-2.5 bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] font-medium rounded-lg transition-colors text-sm border border-[var(--color-surface-border)]"
							>
								<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
								</svg>
								Speed Read
							</a>
						{/if}
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
										class="text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)] hover:underline"
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
										class="text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)] hover:underline"
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
										class="text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)] hover:underline"
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
										class="text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)] hover:underline"
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
									class="px-2 py-0.5 rounded-full text-xs font-medium {book.status === 'reading' ? 'bg-blue-500/20 text-blue-400' : book.status === 'finished' ? 'bg-emerald-500/20 text-emerald-400' : 'bg-[var(--color-surface-700)] text-[var(--color-surface-text)]'} hover:opacity-80 transition-opacity"
								>
									{book.status === 'reading' ? 'Currently Reading' : book.status === 'finished' ? 'Already Read' : 'Unread'}
								</button>
							</dd>
						</div>
						{#if book.series}
							<div class="flex items-start gap-2 col-span-2">
								<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Series</dt>
								<dd class="text-sm text-[var(--color-surface-text)]">
									<button
										onclick={() => navigateWithFilter('series', book.series)}
										class="text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)] hover:underline"
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
												<div class="inline-flex items-center rounded-full border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-1 text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-600)]">
													{#each parts as part, i}
														<button
															onclick={() => navigateWithFilter('genre', part.fullPath)}
															class="transition-colors hover:underline {isHierarchyPartActive(hoveredGenrePath, part.fullPath) ? 'text-[var(--color-primary-400)]' : ''}"
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
												<div class="inline-flex items-center rounded-full border border-[var(--color-primary-500)]/40 bg-[var(--color-primary-500)]/20 px-3 py-1 text-sm text-[var(--color-primary-400)] transition-colors hover:bg-[var(--color-primary-500)]/30">
													{#each parts as part, i}
														<button
															onclick={() => navigateWithFilter('tags', part.fullPath)}
															class="transition-colors hover:underline {isHierarchyPartActive(hoveredTagPath, part.fullPath) ? 'text-[var(--color-primary-200)]' : ''}"
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
						class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px flex items-center gap-2 {activeTab === 'similar' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
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
						class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {activeTab === 'sessions' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
					>
						Reading Sessions
					</button>
					<button
						onclick={() => handleTabChange('files')}
						class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {activeTab === 'files' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
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
									<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4 hover:border-[var(--color-surface-500)] transition-colors">
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
													class="p-2 rounded-lg bg-red-500/10 text-red-400 hover:bg-red-500/20 transition-colors"
													title="Delete session"
												>
													<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
										<div class="p-4 flex items-center justify-between hover:bg-[var(--color-surface-700)]/50 transition-colors">
											<div class="flex items-center space-x-3 min-w-0">
												<svg class="w-5 h-5 text-[var(--color-primary-500)] flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
													class="px-3 py-1 text-sm bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white rounded transition-colors"
												>
													Download
												</button>
												<div class="relative">
													<button
														onclick={() => toggleConvertMenu(file.id)}
														class="px-3 py-1 text-sm border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text)] rounded transition-colors"
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
																	class="px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]"
																>
																	Cancel
																</button>
																<button
																	onclick={() => downloadConvertedFile(file)}
																	class="px-3 py-2 text-sm bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white rounded transition-colors"
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
									<a href="/book/{similar.id}" class="group">
										<div class="aspect-[2/3] rounded-lg overflow-hidden bg-[var(--color-surface-700)] mb-2">
											{#if similar.cover_path}
												<img
													src="/api/covers/{similar.id}"
													alt={similar.title}
													class="w-full h-full object-cover group-hover:opacity-80 transition-opacity"
												/>
											{:else}
												<div class="w-full h-full flex items-center justify-center text-[var(--color-surface-text-muted)]">
													<svg class="w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
														<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
													</svg>
												</div>
											{/if}
										</div>
										<h4 class="text-sm font-medium text-[var(--color-surface-text)] truncate group-hover:text-[var(--color-primary-400)] transition-colors">{similar.title}</h4>
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

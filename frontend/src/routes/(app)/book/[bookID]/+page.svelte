<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import AutocompleteInput from '$lib/components/AutocompleteInput.svelte';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import MetadataLookupModal from '$lib/components/MetadataLookupModal.svelte';
	import PathDisplay from '$lib/components/PathDisplay.svelte';
	import ShelfPickerRow from '$lib/components/ShelfPickerRow.svelte';
	import ShelfModal from '$lib/components/ShelfModal.svelte';
	import { showFormatOnCover, getFormatColor } from '$lib/stores';
	import { addMetadataSuggestionsFromPayload, refreshMetadataSuggestions } from '$lib/stores/metadataSuggestions';
	import { getCoverThumbUrl } from '$lib/utils/covers';
	import {
		getBookDetailContextUrl,
		getInlineMetadataEditUrl,
		getMetadataEditSession,
		type MetadataEditSelectionSession
	} from '$lib/utils/metadata-edit-session';
	import {
		buildMetadataPayload,
		comicSpreadFallbackOptions,
		createMetadataEditForm,
		formatSeriesNumber,
		metadataStatusOptions,
		normalizeAuthorName,
		parseAuthors,
		parseJsonArray,
		prepareAuthorRows,
		ratingStars,
		ratingToStars,
		removeAuthorRow,
		setAuthorRowValue,
		type MetadataEditForm
	} from '$lib/utils/metadata-edit';
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
	let navigatingBook = $state(false);
	let refreshingBook = $state(false);
	let editing = $state(false);
	let saving = $state(false);
	let activeTab = $state<'similar' | 'sessions' | 'files'>('similar');
	let bookTabContainer: HTMLDivElement | null = $state(null);
	let bookTabIndicatorStyle = $state('opacity: 0; transform: translateX(0); width: 0;');
	let sessions = $state<any[]>([]);
	let sessionsLoading = $state(false);
	let similarBooks = $state<any[]>([]);
	let similarLoading = $state(false);
	let similarBooksLoaded = $state(false);
	let similarError = $state('');
	let filesError = $state('');
	let saveError = $state<string | null>(null);
	let showMetadataLookup = $state(false);
	let showCoverModal = $state(false);
	let showShelfPicker = $state(false);
	let showCreateShelfModal = $state(false);
	let regeneratingCover = $state(false);
	let shelves = $state<any[]>([]);
	let shelfActionInProgress = $state(false);
	let shelfActionMessage = $state('');
	let formatMenuOpen = $state(false);
	let formatOnCover = $state(true);
	let convertMenuFileId = $state<number | null>(null);
	let selectedConvertFormat = $state<'epub' | 'fb2' | 'txt' | 'rtf'>('epub');
	let statusSaving = $state(false);
	let statusError = $state('');
	let hoveredRating = $state(0);
	let coverFileInput: HTMLInputElement | null = $state(null);
	let coverUploading = $state(false);

	let editForm = $state<MetadataEditForm>(createMetadataEditForm(null));
	let authorsList = $state<string[]>([]);
	let statusOptions = metadataStatusOptions;
	let selectionSession = $state<MetadataEditSelectionSession | null>(null);
	let manualShelves = $derived(shelves.filter((shelf) => shelf.is_magic !== 1));
	let currentBookRouteId: string | null = null;
	let bookRequestToken = 0;
	let exitingInlineMetadataEdit = false;

	type CoverMutationResponse = {
		cover_path?: string;
		cover_source?: string;
		cover_updated_on?: number;
	};

	$effect(() => {
		const unsub = showFormatOnCover.subscribe((value: boolean) => formatOnCover = value);
		return unsub;
	});

	onMount(() => {
		showFormatOnCover.init();
		void updateBookTabIndicator();
		window.addEventListener('resize', updateBookTabIndicator);
		return () => window.removeEventListener('resize', updateBookTabIndicator);
	});

	function loadSelectionSession() {
		const selectionId = $page.url.searchParams.get('selection');
		const contextId = $page.url.searchParams.get('context');
		const sessionId = selectionId || contextId;
		if (sessionId) {
			selectionSession = getMetadataEditSession(sessionId);
			return;
		}

		selectionSession = null;
		if (typeof window === 'undefined' || !document.referrer) return;
		try {
			const referrer = new URL(document.referrer);
			if (referrer.origin !== window.location.origin || !referrer.pathname.startsWith('/reader/')) return;
			const latestSession = getMetadataEditSession();
			const currentBookId = Number($page.params.bookID);
			if (latestSession && Number.isFinite(currentBookId) && latestSession.bookIds.includes(currentBookId)) {
				selectionSession = latestSession;
			}
		} catch {
			// Ignore invalid referrers and leave the page as a normal book detail view.
		}
	}

	function shouldOpenInlineMetadataEdit(): boolean {
		return $page.url.searchParams.get('edit') === 'metadata';
	}

	$effect(() => {
		const bookId = $page.params.bookID;
		if (bookId) {
			loadSelectionSession();
			if (bookId !== currentBookRouteId) {
				currentBookRouteId = bookId;
				void fetchBook({ bookId, mode: book ? 'navigate' : 'initial', resetRelated: true });
			} else if (book && shouldOpenInlineMetadataEdit() && !editing) {
				startEditing();
			}
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

	async function fetchBook(options: { bookId?: string; mode?: 'initial' | 'navigate' | 'quiet'; resetRelated?: boolean } = {}) {
		const mode = options.mode || 'initial';
		const resetRelated = options.resetRelated ?? mode !== 'quiet';
		const requestToken = ++bookRequestToken;
		const showFullLoading = mode === 'initial' && !book;
		if (showFullLoading) loading = true;
		if (mode === 'navigate') navigatingBook = true;
		if (mode === 'quiet') refreshingBook = true;
		formatMenuOpen = false;
		convertMenuFileId = null;
		const bookId = options.bookId || $page.params.bookID;
		try {
			const [bookResult, filesResult] = await Promise.allSettled([
				fetch(`/api/books/${bookId}`, { credentials: 'same-origin' }),
				fetch(`/api/books/${bookId}/files`, { credentials: 'same-origin' })
			]);
			if (requestToken !== bookRequestToken) return;
			const bookRes = bookResult.status === 'fulfilled' ? bookResult.value : null;
			const filesRes = filesResult.status === 'fulfilled' ? filesResult.value : null;

			if (bookRes?.status === 401 || filesRes?.status === 401) {
				await goto('/login');
				return;
			}
			if (bookRes?.ok) {
				book = await bookRes.json();
				if (shouldOpenInlineMetadataEdit() && !exitingInlineMetadataEdit) {
					startEditing();
				} else if (mode !== 'quiet') {
					editing = false;
				}
			} else {
				book = null;
			}
			if (filesRes?.ok) {
				files = await filesRes.json();
				filesError = '';
			} else {
				files = [];
				filesError = 'Unable to check available reader formats.';
			}
		} catch (e) {
			if (requestToken !== bookRequestToken) return;
			console.error('Failed to fetch book:', e);
			filesError = 'Unable to check available reader formats.';
		} finally {
			if (requestToken === bookRequestToken) {
				if (resetRelated) {
					sessions = [];
					sessionsLoading = false;
					similarBooks = [];
					similarBooksLoaded = false;
					similarError = '';
					if (activeTab === 'sessions' && book?.id) {
						void fetchSessions();
					}
				}
				loading = false;
				navigatingBook = false;
				refreshingBook = false;
			}
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
			} else {
				similarBooks = [];
				similarError = res.status === 404 ? '' : 'Unable to load similar books.';
			}
		} catch (e) {
			console.error('Failed to fetch similar books:', e);
			similarBooks = [];
			similarError = 'Unable to load similar books.';
		} finally {
			similarBooksLoaded = true;
			similarLoading = false;
		}
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

	function getCurrentBookDetailUrl(): string {
		return `${$page.url.pathname}${$page.url.search}`;
	}

	const readableFormats = $derived(getReadableFormats());
	const primaryReadFormat = $derived(getPrimaryReadFormat());
	const primarySpeedReadFormat = $derived(getPrimarySpeedReadFormat());
	const isAudioItem = $derived(getReaderRouteKind(primaryReadFormat || getPreferredBookFormat(files) || book?.format) === 'audio');

	function formatSize(bytes: number): string {
		if (bytes < 1024) return bytes + ' B';
		if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
		return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
	}

	function getFileName(path: string): string {
		if (!path) return 'file';
		return path.split(/[/\\]/).pop() || path;
	}

	function getPrimaryReadActionLabel(): string {
		const hasProgress = !!book?.opened && (book.percent || 0) > 0;
		if (isAudioItem) {
			return hasProgress ? 'Continue Listening' : 'Play Audio';
		}
		return hasProgress ? 'Continue Reading' : 'Read Now';
	}

	function getUnavailableReadActionLabel(): string {
		if (loading) return 'Checking reader...';
		return isAudioItem ? 'Audio unavailable' : 'Read Now unavailable';
	}

	function getPrimaryFilePath(): string {
		const preferredFormat = getPreferredBookFormat(files);
		const preferredFile = preferredFormat
			? files.find((file) => file.format === preferredFormat)
			: null;
		return preferredFile?.path || files[0]?.path || '';
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

	function statusChipClass(status: string) {
		const current = book?.status || 'unread';
		if (current === status) {
			if (status === 'reading') return 'border-blue-500/50 bg-blue-500/15 text-blue-300 shadow-sm';
			if (status === 'finished') return 'border-emerald-500/50 bg-emerald-500/15 text-emerald-300 shadow-sm';
			return 'border-[var(--color-primary-500)]/55 bg-[var(--color-primary-500)]/15 text-[var(--color-primary-400)] shadow-sm';
		}
		return 'border-[var(--color-surface-border)] bg-[var(--color-surface-700)] text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-600)] hover:text-[var(--color-surface-text)]';
	}

	function editingStatusChipClass(status: string) {
		if (editForm.status === status) {
			if (status === 'reading') return 'border-blue-500/50 bg-blue-500/15 text-blue-300 shadow-sm';
			if (status === 'finished') return 'border-emerald-500/50 bg-emerald-500/15 text-emerald-300 shadow-sm';
			return 'border-[var(--color-primary-500)]/55 bg-[var(--color-primary-500)]/15 text-[var(--color-primary-400)] shadow-sm';
		}
		return 'border-[var(--color-surface-border)] bg-[var(--color-surface-700)] text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-600)] hover:text-[var(--color-surface-text)]';
	}

	async function updateBookStatus(status: string) {
		if (!book?.id || statusSaving || (book.status || 'unread') === status) return;
		const previousStatus = book.status || 'unread';
		statusSaving = true;
		statusError = '';
		book = { ...book, status };
		try {
			const res = await fetch(`/api/books/${book.id}/status`, {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ status })
			});
			if (!res.ok) {
				throw new Error(await res.text());
			}
		} catch (e) {
			book = { ...book, status: previousStatus };
			statusError = 'Failed to update status.';
			console.error('Failed to update reading status:', e);
		} finally {
			statusSaving = false;
		}
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
		exitingInlineMetadataEdit = false;
		editForm = createMetadataEditForm(book);
		authorsList = prepareAuthorRows(parseAuthors(book.authors || '[]'));
		editing = true;
	}

	function cancelEditing() {
		editing = false;
		editForm = createMetadataEditForm(book);
		authorsList = prepareAuthorRows(parseAuthors(book?.authors || '[]'));
		saveError = null;
	}

	async function refreshAfterMetadataApply() {
		const wasEditing = editing;
		await fetchBook({ mode: 'quiet', resetRelated: false });
		if (wasEditing && book) {
			startEditing();
		}
		showMetadataLookup = false;
	}

	function getSelectionIndex(): number {
		const queryIndex = Number($page.url.searchParams.get('index') || 0);
		if (Number.isFinite(queryIndex) && queryIndex >= 0) return queryIndex;
		if (!selectionSession || !book?.id) return 0;
		return Math.max(0, selectionSession.bookIds.indexOf(book.id));
	}

	function hasSelectionNavigation(): boolean {
		return !!selectionSession && getNavigationTotal() > 1 && (!book?.id || selectionSession.bookIds.includes(book.id) || !!selectionSession.navigationUrl);
	}

	function isSelectionEditSession(): boolean {
		return !!selectionSession && selectionSession.kind !== 'context';
	}

	function getPositionLabel(): string {
		if (!hasSelectionNavigation() || !selectionSession) return '';
		return `${Math.min(getSelectionIndex() + 1, getNavigationTotal())} of ${getNavigationTotal()}`;
	}

	function getNavigationTotal(): number {
		if (!selectionSession) return 0;
		return Number.isFinite(selectionSession.totalCount) && (selectionSession.totalCount || 0) > 0
			? selectionSession.totalCount || 0
			: selectionSession.bookIds.length;
	}

	function getCurrentMetadataPayload() {
		return buildMetadataPayload(editForm, authorsList);
	}

	function getSavedMetadataPayload() {
		return buildMetadataPayload(createMetadataEditForm(book), parseAuthors(book?.authors || '[]'));
	}

	function hasUnsavedMetadataChanges(): boolean {
		if (!editing || !book) return false;
		return JSON.stringify(getCurrentMetadataPayload()) !== JSON.stringify(getSavedMetadataPayload());
	}

	function confirmDiscardUnsavedChanges(): boolean {
		if (!hasUnsavedMetadataChanges()) return true;
		return confirm('Discard unsaved metadata changes?');
	}

	function getBackActionLabel(): string {
		if (editing) {
			return isSelectionEditSession() ? 'Back to Selection' : 'Back to Book Details';
		}
		if (isSelectionEditSession()) return 'Back to Selection';
		const sourcePath = selectionSession?.sourcePath || '';
		if (sourcePath.startsWith('/search')) return 'Back to Results';
		return 'Back to Library';
	}

	function exitInlineMetadataEdit() {
		if (!confirmDiscardUnsavedChanges()) return;
		exitingInlineMetadataEdit = true;
		cancelEditing();
		if (book?.id) {
			goto(getBookDetailContextUrl(book.id, selectionSession, getSelectionIndex()), { replaceState: true });
		}
	}

	async function fetchContextNavigation(offset: number): Promise<{ bookId: number; index: number; total: number } | null> {
		if (!selectionSession?.navigationUrl || !book?.id) return null;
		const url = new URL(selectionSession.navigationUrl, window.location.origin);
		url.searchParams.set('current_id', String(book.id));
		url.searchParams.set('direction', offset < 0 ? 'previous' : 'next');
		const res = await fetch(url.pathname + url.search, { credentials: 'same-origin' });
		if (!res.ok) return null;
		const data = await res.json();
		const bookId = Number(data.book_id || 0);
		const index = Number(data.index);
		const total = Number(data.total);
		if (!Number.isFinite(bookId) || bookId <= 0 || !Number.isFinite(index)) return null;
		if (Number.isFinite(total) && total > 0) {
			selectionSession = { ...selectionSession, totalCount: total };
		}
		return { bookId, index, total };
	}

	async function goToSelectionOffset(offset: number) {
		if (navigatingBook) return;
		if (!hasSelectionNavigation() || !selectionSession || !book?.id) return;
		if (!confirmDiscardUnsavedChanges()) return;
		const nextIndex = getSelectionIndex() + offset;
		const total = getNavigationTotal();
		if (nextIndex < 0 || nextIndex >= total) return;
		let nextBookId = selectionSession.bookIds[nextIndex];
		let resolvedIndex = nextIndex;
		if (!nextBookId && selectionSession.kind === 'context') {
			const resolved = await fetchContextNavigation(offset);
			if (!resolved) return;
			nextBookId = resolved.bookId;
			resolvedIndex = resolved.index;
		}
		const targetUrl = editing
			? getInlineMetadataEditUrl(nextBookId, selectionSession, resolvedIndex)
			: getBookDetailContextUrl(nextBookId, selectionSession, resolvedIndex);
		goto(targetUrl);
	}

	function openCoverModal() {
		if (book?.cover_path) {
			showCoverModal = true;
		}
	}

	function openShelfPicker() {
		showShelfPicker = true;
		shelfActionMessage = '';
		void fetchShelves();
	}

	async function handleShelfCreatedFromPicker() {
		showCreateShelfModal = false;
		await fetchShelves();
		await (window as any).refreshSidebar?.();
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

	function applyCoverMutation(update: CoverMutationResponse) {
		if (!book) return;
		book = {
			...book,
			cover_path: update.cover_path ?? book.cover_path,
			cover_source: update.cover_source ?? book.cover_source,
			cover_updated_on: update.cover_updated_on ?? book.cover_updated_on ?? Math.floor(Date.now() / 1000)
		};
	}

	async function regenerateCover() {
		if (!book?.id || regeneratingCover) return;
		regeneratingCover = true;
		try {
			const res = await fetch(`/api/books/${book.id}/cover/regenerate`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					comic_spread_fallback: editForm.comic_spread_fallback || book.comic_spread_fallback || 'inherit'
				})
			});
			if (res.ok) {
				applyCoverMutation(await res.json());
			} else {
				console.error('Failed to regenerate cover:', await res.text());
			}
		} catch (e) {
			console.error('Failed to regenerate cover:', e);
		} finally {
			regeneratingCover = false;
		}
	}

	function openCoverPicker() {
		coverFileInput?.click();
	}

	async function uploadCustomCover(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file || !book?.id || coverUploading) return;
		coverUploading = true;
		try {
			const formData = new FormData();
			formData.append('cover', file);
			const res = await fetch(`/api/books/${book.id}/cover/custom`, {
				method: 'POST',
				body: formData
			});
			if (res.ok) {
				applyCoverMutation(await res.json());
			} else {
				saveError = await res.text();
			}
		} catch (e) {
			console.error('Failed to upload cover:', e);
			saveError = 'Failed to upload cover.';
		} finally {
			coverUploading = false;
			input.value = '';
		}
	}

	async function resetCustomCover() {
		if (!book?.id || coverUploading) return;
		if (!confirm('Remove the custom cover and restore the imported or generated cover?')) return;
		coverUploading = true;
		try {
			const res = await fetch(`/api/books/${book.id}/cover/custom`, { method: 'DELETE' });
			if (res.ok) {
				applyCoverMutation(await res.json());
			} else {
				saveError = await res.text();
			}
		} catch (e) {
			console.error('Failed to reset cover:', e);
			saveError = 'Failed to reset cover.';
		} finally {
			coverUploading = false;
		}
	}

	async function saveMetadata(stayEditing = false): Promise<boolean> {
		if (!book?.id || saving) return false;
		saveError = null;
		saving = true;
		try {
			const payload = buildMetadataPayload(editForm, authorsList);
			const res = await fetch(`/api/books/${book.id}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(payload)
			});

			if (res.ok) {
				await fetchBook({ mode: 'quiet', resetRelated: false });
				editing = stayEditing && !exitingInlineMetadataEdit;
				if (editing) {
					editForm = createMetadataEditForm(book);
					authorsList = prepareAuthorRows(parseAuthors(book?.authors || '[]'));
				}
				addMetadataSuggestionsFromPayload(payload);
				void refreshMetadataSuggestions();
				return true;
			} else {
				const errorText = await res.text();
				console.error('Failed to save metadata:', res.status, errorText);
				saveError = `Failed to save: ${res.status} ${errorText}`;
				return false;
			}
		} catch (e) {
			console.error('Failed to save metadata:', e);
			saveError = `Error: ${e}`;
			return false;
		} finally {
			saving = false;
		}
	}

	async function applyMetadataEdits(): Promise<boolean> {
		return saveMetadata(true);
	}

	function navigateWithFilter(field: string, value: string) {
		const url = new URL($page.url);
		url.pathname = '/library';
		if (field === 'author') {
			url.searchParams.set('author', value);
		} else if (field === 'series') {
			url.searchParams.set('series', value);
			url.searchParams.set('sort', 'series');
			url.searchParams.set('sort_dir', 'asc');
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

	function goBackToPreviousContext(event: MouseEvent) {
		event.preventDefault();
		if (editing) {
			exitInlineMetadataEdit();
			return;
		}
		if (selectionSession?.sourcePath) {
			goto(selectionSession.sourcePath, { replaceState: true });
			return;
		}
		const currentRoute = `${$page.url.pathname}${$page.url.search}`;
		if (typeof window !== 'undefined') {
			try {
				const stack = JSON.parse(sessionStorage.getItem('cryptorumRouteStack') || '[]') as string[];
				const previousUnique = [...stack].reverse().find((route) =>
					route &&
					route !== currentRoute &&
					!route.startsWith('/reader/') &&
					!/^\/book\/\d+/.test(route)
				);
				if (previousUnique) {
					goto(previousUnique, { replaceState: true });
					return;
				}
			} catch {
				// Fall back to a library route.
			}
		}
		if (book?.library_id) {
			goto(`/library?library=${book.library_id}`);
		} else {
			goto('/library');
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

	async function updateBookTabIndicator() {
		await tick();
		const container = bookTabContainer;
		const activeButton = container?.querySelector('[data-tab-active="true"]') as HTMLElement | null;
		if (!container || !activeButton) {
			bookTabIndicatorStyle = 'opacity: 0; transform: translateX(0); width: 0;';
			return;
		}
		const containerRect = container.getBoundingClientRect();
		const activeRect = activeButton.getBoundingClientRect();
		bookTabIndicatorStyle = `opacity: 1; width: ${activeRect.width}px; transform: translateX(${activeRect.left - containerRect.left + container.scrollLeft}px);`;
	}

	$effect(() => {
		activeTab;
		void updateBookTabIndicator();
	});

	function setRating(value: number) {
		editForm.rating = editForm.rating === value ? 0 : value;
	}

	function getStarFill(rating: number, starIndex: number): boolean {
		return starIndex <= rating;
	}

	function isEditingRatingFilled(star: number): boolean {
		const activeRating = hoveredRating || editForm.rating;
		return star <= activeRating;
	}

	function isBookRatingFilled(star: number): boolean {
		return star <= ratingToStars(book?.rating);
	}

	function addAuthor() {
		authorsList = [...authorsList, ''];
	}

	function removeAuthor(index: number) {
		authorsList = removeAuthorRow(authorsList, index);
	}

	function updateAuthor(index: number, value: string) {
		authorsList = setAuthorRowValue(authorsList, index, value);
	}
</script>

	<div class="space-y-6">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<a href="/library" onclick={goBackToPreviousContext} class="group inline-flex items-center text-[var(--color-surface-text-muted)] transition-colors duration-200 ease-out hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]">
				<svg class="mr-2 h-4 w-4 transition-colors duration-200 ease-out group-hover:text-[var(--color-surface-text)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
				</svg>
				{getBackActionLabel()}
			</a>

			{#if hasSelectionNavigation()}
				<div class="flex items-center gap-2">
					<button
						type="button"
						onclick={() => void goToSelectionOffset(-1)}
						disabled={getSelectionIndex() <= 0 || saving}
						class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-1.5 text-xs font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:translate-y-0"
					>
						Prev
					</button>
					<span class="min-w-12 text-center text-xs font-medium uppercase tracking-[0.12em] text-[var(--color-surface-text-muted)]">{getPositionLabel()}</span>
					<button
						type="button"
						onclick={() => void goToSelectionOffset(1)}
						disabled={!selectionSession || getSelectionIndex() >= getNavigationTotal() - 1 || saving}
						class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-1.5 text-xs font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:translate-y-0"
					>
						Next
					</button>
				</div>
			{/if}
		</div>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if book}
		<section
			class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5 sm:p-6 {navigatingBook ? 'pointer-events-none' : ''}"
			aria-busy={navigatingBook || refreshingBook}
		>
				<div class="flex items-start justify-between gap-4">
						<div class="min-w-0 flex-1">
						{#if editing}
							<input
								id="book-title"
								type="text"
								bind:value={editForm.title}
								placeholder="Untitled book"
								class="w-full rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-2.5 py-1.5 text-2xl font-bold text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)] xl:text-3xl"
							/>
							<div class="mt-1 max-w-xl space-y-2">
								{#each authorsList as author, i}
									<div class="flex max-w-xl items-center gap-2">
										<div class="min-w-0 flex-1">
											<AutocompleteInput
												id={`book-author-${i}`}
												bind:value={authorsList[i]}
												placeholder="Author name"
												field="authors"
												multiple={false}
												onchange={(value) => updateAuthor(i, value)}
											/>
										</div>
										<button
											type="button"
											onclick={() => removeAuthor(i)}
											class="rounded-md p-2 text-red-400 transition-all duration-200 ease-out hover:-translate-y-px hover:bg-red-500/10 hover:text-red-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400/40"
											title="Remove author"
											aria-label="Remove author"
										>
											<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
											</svg>
										</button>
									</div>
								{/each}
								<button
									type="button"
									onclick={addAuthor}
									class="inline-flex items-center rounded-md px-1.5 py-0.5 text-sm text-[var(--color-primary-400)] transition-colors duration-200 ease-out hover:bg-[var(--color-primary-500)]/12 hover:text-[var(--color-primary-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]"
								>
									Add Author
								</button>
								</div>
							{:else}
							<h1 class="break-words text-2xl font-bold text-[var(--color-surface-text)] xl:text-3xl">{book.title || 'Untitled'}</h1>
							<div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1">
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
							{/if}
						</div>
						<div class="flex flex-shrink-0 flex-col items-end gap-2">
							<div class="flex flex-wrap justify-end gap-2">
								{#if editing}
									<button
										onclick={cancelEditing}
										disabled={saving}
										class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:opacity-50"
									>
										Cancel
								</button>
								<button
									onclick={applyMetadataEdits}
									disabled={saving}
									class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:opacity-50"
								>
									{saving ? 'Saving...' : 'Apply'}
								</button>
							{:else}
								<button
									onclick={startEditing}
									class="group rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] p-2 text-[var(--color-surface-text)] transition-colors duration-200 ease-out hover:bg-[var(--color-surface-600)] hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
									title="Edit Metadata"
									aria-label="Edit metadata"
									>
										<svg class="h-5 w-5 transition-colors duration-200 ease-out group-hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z"></path>
										</svg>
									</button>
								{/if}
							</div>
							{#if editing}
								<button
									type="button"
									onclick={() => showMetadataLookup = true}
									class="accent-action rounded-lg px-3 py-2 text-sm font-medium transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
								>
									Lookup Metadata
								</button>
							{/if}
						</div>
					</div>

					<div class="mt-7 flex flex-col gap-6 md:flex-row md:items-start md:gap-8">
					<div class="w-full max-w-[13rem] mx-auto md:mx-0 flex-shrink-0 flex flex-col">
						<input bind:this={coverFileInput} type="file" accept="image/*" class="hidden" onchange={uploadCustomCover} />
						<div class="relative">
							<button
								type="button"
								onclick={openCoverModal}
								class="group block w-full text-left transition-all duration-200 ease-out {book.cover_path ? 'cursor-zoom-in hover:-translate-y-0.5 hover:shadow-lg' : 'cursor-default'} focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
								disabled={!book.cover_path}
								title={book.cover_path ? 'Open cover preview' : undefined}
							>
							<BookCoverFrame
								src={book.cover_path ? getCoverThumbUrl(book.id, 'large', book.cover_updated_on) : null}
								alt={book.title}
								format={book.format}
								mode="contain"
								frameClass="aspect-[2/3] w-full"
								imageClass="transition-transform duration-200 ease-out group-hover:scale-[1.02]"
								loading="eager"
							/>
						</button>
						{#if editing}
							<div class="absolute bottom-2 right-2 z-20 flex gap-1.5">
								{#if book.cover_source === 'custom'}
									<button
										type="button"
										onclick={resetCustomCover}
										disabled={coverUploading}
										class="rounded-full border border-[var(--color-surface-border)] bg-[var(--color-surface-900)]/90 px-2 py-1 text-xs font-semibold text-[var(--color-surface-text)] shadow-lg backdrop-blur transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-800)] disabled:opacity-50 disabled:hover:translate-y-0"
									>
										Reset
									</button>
								{/if}
								<button
									type="button"
									onclick={openCoverPicker}
									disabled={coverUploading}
									class="cover-accent-action rounded-full px-2 py-1 text-xs font-semibold shadow-lg backdrop-blur transition-all duration-200 ease-out hover:-translate-y-px disabled:opacity-50 disabled:hover:translate-y-0"
								>
									{coverUploading ? 'Saving...' : 'Edit'}
								</button>
							</div>
							{/if}
							</div>

						{#if editing}
							<div class="mt-4 space-y-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)]/40 p-3">
								<label class="block text-xs font-semibold uppercase tracking-[0.12em] text-[var(--color-surface-text-muted)]" for="inline-comic-fallback">Wide comic fallback</label>
								<select
									id="inline-comic-fallback"
									bind:value={editForm.comic_spread_fallback}
									class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								>
									{#each comicSpreadFallbackOptions as option}
										<option value={option.value} class="bg-[var(--color-surface-base)] text-[var(--color-surface-text)]">{option.label}</option>
									{/each}
								</select>
								<button
									type="button"
									onclick={regenerateCover}
									disabled={regeneratingCover}
									class="group inline-flex w-full items-center justify-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:opacity-50"
								>
									{#if regeneratingCover}
										<svg class="mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
											<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
											<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
										</svg>
										Regenerating...
									{:else}
										<svg class="mr-2 h-4 w-4 transition-transform duration-200 ease-out group-hover:-rotate-45 group-hover:scale-110" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
										</svg>
										Regenerate Cover
									{/if}
								</button>
							</div>
						{/if}

						<div class="mt-4">
							<div class="flex items-center justify-between mb-2">
								<span class="text-sm text-[var(--color-surface-text-muted)]">{isAudioItem ? 'Listening Progress' : 'Progress'}</span>
							<span class="text-sm font-medium text-[var(--color-surface-text)]">{Math.round(book.percent || 0)}%</span>
						</div>
						<div class="w-full h-2 bg-[var(--color-surface-700)] rounded-full overflow-hidden">
							<div
								class="h-full bg-[var(--color-primary-500)] rounded-full transition-all duration-300"
								style="width: {book.percent || 0}%"
							></div>
						</div>
					</div>
					{#if primarySpeedReadFormat}
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
					{/if}

					<div class="mt-3 mb-4 md:mb-0 flex flex-col gap-2">
						<div class="relative">
							<div class="flex w-full overflow-hidden rounded-lg {primaryReadFormat && !editing ? 'read-action-control' : ''}">
								{#if primaryReadFormat}
									{#if editing}
										<button
											type="button"
											disabled
											class="flex min-w-0 flex-1 cursor-not-allowed items-center justify-between gap-3 bg-[var(--color-surface-700)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text-muted)] opacity-70 sm:px-4"
										>
											<span class="truncate">{getPrimaryReadActionLabel()}</span>
											<span class="text-[10px] uppercase tracking-[0.14em]">{getFormatDisplayLabel(primaryReadFormat)}</span>
										</button>
										{:else}
											<a
												href={getBookReaderHref(book.id, primaryReadFormat, getCurrentBookDetailUrl())}
												class="flex min-w-0 flex-1 items-center justify-between gap-3 px-3 py-2 text-sm font-medium transition-colors duration-200 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] sm:px-4"
										>
											<span class="truncate">{getPrimaryReadActionLabel()}</span>
											<span class="text-[10px] uppercase tracking-[0.14em] text-[var(--color-primary-300)]">{getFormatDisplayLabel(primaryReadFormat)}</span>
										</a>
								{/if}
								{:else}
									<button
										type="button"
										disabled
										class="flex min-w-0 flex-1 items-center justify-between gap-3 bg-[var(--color-surface-700)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text-muted)] sm:px-4"
									>
										<span class="truncate">{getUnavailableReadActionLabel()}</span>
										<span class="text-[10px] uppercase tracking-[0.14em]">{filesError ? 'retry' : 'no reader'}</span>
									</button>
								{/if}
								{#if readableFormats.length > 1}
									<button
										type="button"
										onclick={() => formatMenuOpen = !formatMenuOpen}
										disabled={editing}
										class="read-action-format-toggle inline-flex items-center justify-center px-3 transition-colors duration-200 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:bg-[var(--color-surface-700)] disabled:text-[var(--color-surface-text-muted)]"
										aria-label="Choose reader format"
										aria-expanded={formatMenuOpen}
									>
										<svg class="h-4 w-4 transition-transform duration-200 ease-out {formatMenuOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
										</svg>
									</button>
								{/if}
							</div>
							{#if !primaryReadFormat && !loading}
								<button type="button" onclick={() => void fetchBook({ mode: 'quiet', resetRelated: false })} class="mt-1 text-xs text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">
									Retry reader check
								</button>
							{/if}
							{#if formatMenuOpen && readableFormats.length > 1 && !editing}
								<div class="absolute right-0 top-full z-20 mt-2 w-56 overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-800)] shadow-lg">
									{#each readableFormats.filter((format) => format !== primaryReadFormat) as format}
										<a
											href={getBookReaderHref(book.id, format, getCurrentBookDetailUrl())}
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
							{#if primarySpeedReadFormat}
									{#if editing}
										<button
											type="button"
											disabled
											class="flex w-full cursor-not-allowed items-center justify-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text-muted)] opacity-70 sm:px-4"
										>
											<svg class="mr-2 h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
											</svg>
											Speed Read
										</button>
									{:else}
										<a
											href={getSpeedReaderHref(book.id, primarySpeedReadFormat, getCurrentBookDetailUrl())}
											class="group flex w-full items-center justify-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-colors duration-200 ease-out hover:border-[var(--color-surface-500)] hover:bg-[var(--color-surface-600)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] sm:px-4"
										>
											<svg class="mr-2 h-4 w-4 transition-colors duration-200 ease-out group-hover:text-[var(--color-primary-300)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
											</svg>
											Speed Read
										</a>
									{/if}
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
					<div class="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
						<div class="flex items-start gap-2 sm:col-span-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Status</dt>
							<dd class="flex flex-wrap gap-2 text-sm">
								{#each statusOptions as option}
									<button
										type="button"
										onclick={() => editing ? editForm.status = option.value : updateBookStatus(option.value)}
										disabled={!editing && statusSaving}
										class="rounded-lg border px-3 py-1.5 text-xs font-medium transition-all duration-200 ease-out {editing ? editingStatusChipClass(option.value) : statusChipClass(option.value)} focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:cursor-wait disabled:opacity-70"
										aria-pressed={(editing ? editForm.status : book.status || 'unread') === option.value}
									>
										{option.label}
									</button>
								{/each}
								{#if statusError}
									<span class="basis-full text-xs text-red-400">{statusError}</span>
								{/if}
							</dd>
						</div>
						{#if uniqueBookFormats(files).length > 0}
							<div class="flex items-start gap-2 sm:col-span-2">
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
							<dd class="min-w-0 flex-1 text-sm text-[var(--color-surface-text)]">
								{#if editing}
									<AutocompleteInput
										id="book-publisher"
										bind:value={editForm.publisher}
										field="publishers"
										multiple={false}
										onchange={(value) => editForm.publisher = value}
									/>
								{:else if book.publisher}
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
							<dd class="min-w-0 flex-1 text-sm text-[var(--color-surface-text)]">
								{#if editing}
									<input id="book-pub-date" type="text" bind:value={editForm.pub_date} placeholder="YYYY-MM-DD" class="w-full rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
								{:else if book.pub_date}
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
							<dd class="min-w-0 flex-1 text-sm text-[var(--color-surface-text)]">
								{#if editing}
									<AutocompleteInput
										id="book-language"
										bind:value={editForm.language}
										field="languages"
										multiple={false}
										onchange={(value) => editForm.language = value}
									/>
								{:else if book.language}
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
							<dd class="min-w-0 flex-1 text-sm text-[var(--color-surface-text)]">
								{#if editing}
									<input id="book-pages" type="number" bind:value={editForm.page_count} min="0" class="w-full rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
								{:else}
									{book.page_count || '-'}
								{/if}
							</dd>
						</div>
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">ISBN</dt>
							<dd class="min-w-0 flex-1 text-sm text-[var(--color-surface-text)]">
								{#if editing}
									<input id="book-isbn" type="text" bind:value={editForm.isbn} class="w-full rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 font-mono text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
								{:else}
									<span class="font-mono">{book.isbn || '-'}</span>
								{/if}
							</dd>
						</div>
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">ASIN</dt>
							<dd class="min-w-0 flex-1 text-sm text-[var(--color-surface-text)]">
								{#if editing}
									<input id="book-asin" type="text" bind:value={editForm.asin} class="w-full rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 font-mono text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
								{:else}
									<span class="font-mono">{book.asin || '-'}</span>
								{/if}
							</dd>
						</div>
						<div class="flex items-start gap-2">
							<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Rating</dt>
							<dd class="inline-flex max-w-full items-center gap-0.5 whitespace-nowrap text-sm text-[var(--color-surface-text)]" onmouseleave={() => hoveredRating = 0}>
								{#if editing}
									{#each ratingStars as star}
										<button
											type="button"
											onmouseenter={() => hoveredRating = star}
											onfocus={() => hoveredRating = star}
											onblur={() => hoveredRating = 0}
											onclick={() => setRating(star)}
											class="text-lg transition-all duration-200 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-yellow-400/40 focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] {isEditingRatingFilled(star) ? 'text-yellow-400' : 'text-[var(--color-surface-600)]'} hover:-translate-y-px hover:scale-110"
											aria-label={editForm.rating === star ? 'Clear rating' : `Set rating to ${star} star${star === 1 ? '' : 's'}`}
										>
											&#9733;
										</button>
									{/each}
								{:else if book.rating}
									{#each ratingStars as star}
										<span class="{isBookRatingFilled(star) ? 'text-yellow-400' : 'text-[var(--color-surface-600)]'}">★</span>
									{/each}
								{:else}
									<span class="text-[var(--color-surface-text-muted)]">-</span>
								{/if}
							</dd>
						</div>
						{#if getPrimaryFilePath()}
							<div class="flex items-start gap-2 sm:col-span-2">
								<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Path</dt>
								<dd class="min-w-0 flex-1">
									<PathDisplay path={getPrimaryFilePath()} libraryPaths={book.library_paths || []} />
								</dd>
							</div>
						{/if}
								{#if editing || book.series}
									<div class="flex items-start gap-2 sm:col-span-2">
										<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Series</dt>
										<dd class="min-w-0 flex-1 text-sm text-[var(--color-surface-text)]">
											{#if editing}
												<div class="flex gap-2">
													<div class="min-w-0 flex-1">
														<AutocompleteInput
															id="book-series"
															bind:value={editForm.series}
															placeholder="Series name"
															field="series"
															multiple={false}
															onchange={(value) => editForm.series = value}
														/>
													</div>
													<input id="book-series-number" type="text" bind:value={editForm.series_number} placeholder="#" class="w-24 rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
												</div>
											{:else}
												<button
													onclick={() => navigateWithFilter('series', book.series)}
													class="text-[var(--color-primary-400)] transition-colors duration-200 ease-out hover:text-[var(--color-primary-300)] hover:underline focus-visible:outline-none focus-visible:underline"
												>
													{book.series}
												</button>
												{#if formatSeriesNumber(book)}
													<span class="text-[var(--color-surface-text-muted)]"> #{formatSeriesNumber(book)}</span>
												{/if}
											{/if}
								</dd>
							</div>
						{/if}
					</div>

					{#if editing || (book.tags && book.tags !== '[]')}
						<div class="mt-6 space-y-4">
							<div class="flex items-start gap-2">
								<dt class="text-sm text-[var(--color-surface-text-muted)] w-24 flex-shrink-0">Tags</dt>
								<dd class="min-w-0 flex-1">
									{#if editing}
										<AutocompleteInput
											id="book-tags"
											bind:value={editForm.tags}
											placeholder="Fiction, Science Fiction.Space Opera, Favorite"
											field="tags"
											onchange={(value) => editForm.tags = value}
										/>
										<p class="mt-1 text-xs text-[var(--color-surface-text-muted)]">Comma-separated. Use "Parent.Child" for hierarchies.</p>
									{:else}
										<div class="flex flex-wrap gap-2">
											{#each parseJsonArray(book.tags) as tag}
												{@const parts = parseHierarchicalGenre(tag)}
												<div class="relative group">
														<div class="inline-flex items-center rounded-full border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-1 text-sm text-[var(--color-surface-text)] transition-colors duration-200 ease-out hover:border-[var(--color-surface-500)] hover:bg-[var(--color-surface-600)] {isHoveredPathInHierarchy(hoveredTagPath, tag) ? 'underline decoration-[var(--color-primary-300)] underline-offset-2' : ''}">
															{#each parts as part, i}
																<button
																	onclick={() => navigateWithFilter('tags', part.fullPath)}
																	class="transition-colors duration-200 ease-out hover:text-[var(--color-primary-300)] hover:underline focus-visible:outline-none focus-visible:underline {isHierarchyPartActive(hoveredTagPath, part.fullPath) ? 'text-[var(--color-primary-300)]' : 'text-[var(--color-primary-400)]'}"
																	onmouseenter={() => hoveredTagPath = part.fullPath}
																	onmouseleave={() => hoveredTagPath = null}
																>{part.text}</button>{#if i < parts.length - 1}<span class="{isHierarchyPartActive(hoveredTagPath, parts[i + 1].fullPath) ? 'text-[var(--color-primary-400)]' : 'text-[var(--color-surface-text-muted)]'}">.</span>{/if}
															{/each}
													</div>
											</div>
											{/each}
										</div>
									{/if}
								</dd>
							</div>
						</div>
					{/if}

					{#if editing || book.description}
						<div class="mt-6">
							{#if editing}
								<div class="flex items-start gap-2">
									<label class="w-24 flex-shrink-0 text-sm text-[var(--color-surface-text-muted)]" for="book-description">Description</label>
									<textarea id="book-description" bind:value={editForm.description} rows="4" class="min-w-0 flex-1 resize-y rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"></textarea>
								</div>
							{:else}
								<p class="text-sm text-[var(--color-surface-text)] whitespace-pre-wrap line-clamp-3">{book.description}</p>
							{/if}
						</div>
					{/if}
					{#if editing && saveError}
						<div class="mt-4 rounded-lg border border-red-500/50 bg-red-500/20 p-3 text-sm text-red-400">
							{saveError}
						</div>
					{/if}
				</div>
			</div>
		</section>

			<section
				class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5 sm:p-6 {navigatingBook ? 'pointer-events-none' : ''}"
				aria-busy={navigatingBook}
			>
					<div bind:this={bookTabContainer} class="relative flex justify-center gap-2 overflow-x-auto sm:gap-6" onscroll={updateBookTabIndicator}>
						<div class="book-tab-indicator" style={bookTabIndicatorStyle}></div>
						<button
							onclick={() => handleTabChange('similar')}
							data-tab-active={activeTab === 'similar'}
							class="flex flex-shrink-0 items-center gap-2 px-3 py-2 text-sm font-medium transition-all duration-200 ease-out sm:px-4 {activeTab === 'similar' ? 'text-[var(--color-primary-500)]' : 'text-[var(--color-surface-text-muted)] hover:-translate-y-px hover:text-[var(--color-surface-text)]'}"
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
							data-tab-active={activeTab === 'sessions'}
							class="flex-shrink-0 px-3 py-2 text-sm font-medium transition-all duration-200 ease-out sm:px-4 {activeTab === 'sessions' ? 'text-[var(--color-primary-500)]' : 'text-[var(--color-surface-text-muted)] hover:-translate-y-px hover:text-[var(--color-surface-text)]'}"
						>
							{isAudioItem ? 'Listening Sessions' : 'Reading Sessions'}
						</button>
						<button
							onclick={() => handleTabChange('files')}
							data-tab-active={activeTab === 'files'}
							class="flex-shrink-0 px-3 py-2 text-sm font-medium transition-all duration-200 ease-out sm:px-4 {activeTab === 'files' ? 'text-[var(--color-primary-500)]' : 'text-[var(--color-surface-text-muted)] hover:-translate-y-px hover:text-[var(--color-surface-text)]'}"
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
								<p class="text-[var(--color-surface-text-muted)]">{isAudioItem ? 'No listening sessions yet' : 'No reading sessions yet'}</p>
							</div>
						{:else}
							<div class="max-h-[min(32rem,55dvh)] space-y-3 overflow-y-auto overscroll-contain p-1 pr-2 [scrollbar-gutter:stable]">
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
												<span class="active-filter-chip rounded-full px-2.5 py-1 text-xs font-medium">
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
													class="accent-action rounded-lg px-3 py-1.5 text-sm font-medium transition-all duration-200 ease-out hover:-translate-y-0.5 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
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
																		class="accent-action rounded-lg px-3 py-2 text-sm font-medium transition-all duration-200 ease-out hover:-translate-y-0.5 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-overlay)]"
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
								<div class="grid grid-cols-2 gap-4 sm:grid-cols-[repeat(auto-fit,minmax(11rem,1fr))] sm:justify-items-center">
									{#each similarBooks as similar}
										<a href="/book/{similar.id}" class="group relative block w-full min-w-0 rounded-xl p-2 transition-all duration-200 ease-out hover:-translate-y-1 hover:bg-[var(--color-surface-overlay)] hover:shadow-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] sm:max-w-[12rem]">
											<BookCoverFrame
												src={similar.cover_path ? getCoverThumbUrl(similar.id, 'medium', similar.cover_updated_on) : null}
												alt={similar.title}
												format={similar.format}
												mode="contain"
												frameClass="aspect-[2/3] w-full mb-2"
												imageClass="transition-all duration-200 ease-out group-hover:scale-[1.02] group-hover:opacity-80"
												placeholderSize="md"
											/>
											{#if formatOnCover && similar.format}
												{@const formatColor = getFormatColor(similar.format)}
												<div
													class="absolute left-4 top-4 z-10 rounded border border-black/20 px-1.5 py-0.5 text-[10px] font-medium uppercase shadow-[0_1px_2px_rgba(0,0,0,0.35)]"
													style="background-color: {formatColor.bg}; color: {formatColor.text};"
												>
													{similar.format}
												</div>
											{/if}
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
				</section>
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
		confirmBeforeApply={true}
		hasUnsavedChanges={hasUnsavedMetadataChanges()}
		onClose={() => showMetadataLookup = false}
		onApplyCurrentEdits={applyMetadataEdits}
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
							{#if manualShelves.length === 0}
								<div class="py-6 text-center text-sm text-[var(--color-surface-text-muted)]">
									No manual shelves yet.
								</div>
							{:else}
								<div class="space-y-2">
									{#each manualShelves as shelf}
										<ShelfPickerRow
											{shelf}
											disabled={shelfActionInProgress}
											onSelect={(selectedShelf) => addBookToShelf(selectedShelf.id)}
										/>
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
							<button
								type="button"
								onclick={() => showCreateShelfModal = true}
								class="text-sm text-[var(--color-primary-400)] transition-colors duration-200 hover:text-[var(--color-primary-300)] focus-visible:outline-none focus-visible:underline"
							>
								New shelf
							</button>
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

<ShelfModal
	open={showCreateShelfModal}
	mode="create"
	initialMagic={false}
	allowMagicToggle={false}
	onClose={() => showCreateShelfModal = false}
	onSaved={handleShelfCreatedFromPicker}
/>

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
						format={book.format}
						mode="contain"
						frameClass="aspect-[2/3] w-full"
						imageClass="object-contain p-2"
					/>
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	.read-action-control {
		border: 1px solid color-mix(in srgb, var(--color-primary-500) 55%, transparent);
		background: color-mix(in srgb, var(--color-primary-500) 10%, var(--color-surface-overlay));
		color: var(--color-primary-400);
	}

	.read-action-control:hover {
		border-color: color-mix(in srgb, var(--color-primary-400) 70%, transparent);
		background: color-mix(in srgb, var(--color-primary-500) 16%, var(--color-surface-overlay));
	}

	.read-action-format-toggle {
		border-left: 1px solid color-mix(in srgb, var(--color-primary-500) 35%, transparent);
	}

	.book-tab-indicator {
		position: absolute;
		bottom: 0;
		left: 0;
		height: 2px;
		border-radius: 999px;
		background: var(--color-primary-500);
		pointer-events: none;
		transition: transform 160ms ease-out, width 160ms ease-out, opacity 120ms ease-out;
	}
</style>

<svelte:window onkeydown={handleCoverModalKeydown} />

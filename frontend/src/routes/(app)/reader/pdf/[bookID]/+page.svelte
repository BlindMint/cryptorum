<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { browser } from '$app/environment';
	import { defaultReaderSettings, readerSettings } from '$lib/stores/readerSettings';
	import type { PdfReaderSetting, PdfViewMode } from '$lib/stores/readerSettings';
	import { normalizeBookFormat } from '$lib/utils/book-formats';
	import { toggleReaderFullscreen } from '$lib/utils/fullscreen';
	import { isBottomSystemGestureStart } from '$lib/utils/system-gesture-guard';
	import EmbedPDFViewer from '$lib/components/EmbedPDFViewer.svelte';
	import ReaderProgressTrack from '$lib/components/ReaderProgressTrack.svelte';

	let book = $state<any>(null);
	let loading = $state(true);
	let error = $state('');
	let currentPage = $state(1);
	let numPages = $state(0);
	let savedProgress = $state<any>(null);
	let currentSessionId = $state<number | null>(null);
	let requestedFormat = $state('pdf');
	let embedPdfProgressReady = $state(false);
	let embedPdfInitialPage = $state(1);
	let embedPdfViewerReady = $state(false);
	let embedPdfScroll: any = null;
	let embedPdfRestoringInitialPage = false;
	let embedPdfRestoreTimers: ReturnType<typeof setTimeout>[] = [];
	let embedPdfSidebarOpen = $state(false);
	let pdfLoadRetryToken = $state(0);
	let pdfResumeToken = $state(0);
	let pdfLoadRetryAttempts = 0;
	let progressLoaded = false;
	let wasPageInterrupted = false;

	let settings = $state<PdfReaderSetting>({ ...defaultReaderSettings.pdf });
	let topBarHideTimeout: ReturnType<typeof setTimeout> | null = null;
	let topBarVisible = $state(false);
	let preserveChromeAfterSeekUntil = 0;
	let sessionEnded = false;
	let handlePageExit: (() => void) | null = null;
	let pdfReaderEl = $state<HTMLDivElement | null>(null);
	let isRestoringProgress = false;
	let progressSaveTimer: ReturnType<typeof setTimeout> | null = null;
	let lastSavedPage = 0;
	let closeTasksStarted = false;
	let readerClosing = false;
	let isDraggingProgress = $state(false);
	let pendingProgressPage = $state<number | null>(null);
	let progressPreviewPage = $state<number | null>(null);
	let pdfProgressBarEl = $state<HTMLElement | null>(null);
	let activeProgressPointerId: number | null = null;

	const progress = $derived(numPages > 0 ? (currentPage / numPages) * 100 : 0);
	const progressPreviewProgress = $derived(
		progressPreviewPage !== null && numPages > 0 ? (progressPreviewPage / numPages) * 100 : null
	);
	const progressPreviewLabel = $derived(
		progressPreviewPage !== null && numPages > 0
			? `Page ${progressPreviewPage} / ${numPages} • ${Math.round((progressPreviewPage / numPages) * 100)}%`
			: ''
	);
	const readerChromeReady = $derived(embedPdfViewerReady && !loading && !error);

	const topBarHideDelayMs = 2800;
	const scrollHideThresholdPx = 8;
	const seekScrollSuppressMs = 1400;
	const progressSaveDebounceMs = 750;
	const embedPdfDocumentId = 'cryptorum-pdf';

	const viewModeBgColors: Record<PdfViewMode, string> = {
		light: '#ffffff',
		dark: '#1a1a1a',
		trueDark: '#000000'
	};

	function clampPage(page: number, totalPages = numPages) {
		const normalized = Math.max(1, Math.floor(page || 1));
		return totalPages > 0 ? Math.min(normalized, totalPages) : normalized;
	}

	function getSavedProgressPage() {
		if (savedProgress?.page > 0) {
			return clampPage(savedProgress.page);
		}

		if (
			typeof savedProgress?.percent === 'number' &&
			savedProgress.percent > 0 &&
			numPages > 0
		) {
			return clampPage(Math.ceil((savedProgress.percent / 100) * numPages));
		}

		return 1;
	}

	function getSafeReturnPath(value: string | null) {
		if (!value) return null;
		if (!value.startsWith('/') || value.startsWith('//')) return null;
		if (value.startsWith('/login') || value.includes('/reader/')) return null;
		return value;
	}

	function getLaunchReturnPath() {
		const queryReturnTo = getSafeReturnPath($page.url.searchParams.get('returnTo'));
		if (queryReturnTo) return queryReturnTo;

		if (browser && document.referrer) {
			try {
				const referrer = new URL(document.referrer);
				if (referrer.origin === window.location.origin) {
					const referrerPath = getSafeReturnPath(`${referrer.pathname}${referrer.search}${referrer.hash}`);
					if (referrerPath) return referrerPath;
				}
			} catch {
				// Ignore invalid referrers and fall back to the book or library route.
			}
		}

		return null;
	}

	function getReaderReturnUrl() {
		const launchReturnPath = getLaunchReturnPath();
		if (launchReturnPath) return launchReturnPath;

		return book?.id ? `/book/${book.id}` : '/library';
	}

	function getPdfDisplayTitle() {
		const title = String(book?.title || book?.metadata?.title || '').trim();
		if (title) return title;

		const filePath = String(book?.path || book?.file_path || book?.filename || book?.file_name || '').trim();
		if (filePath) {
			const fileName = filePath.split(/[\\/]/).pop();
			if (fileName) return fileName.replace(/\.[^.]+$/, '');
		}

		return book ? 'PDF Document' : 'Loading...';
	}

	function getPdfSourceUrl() {
		if (!book?.id) return '';
		const params = new URLSearchParams();
		if (requestedFormat) {
			params.set('format', requestedFormat);
		}
		if (pdfLoadRetryToken > 0) {
			params.set('pdf_retry', String(pdfLoadRetryToken));
		}
		const query = params.toString();
		return `/api/books/${book.id}/file${query ? `?${query}` : ''}`;
	}

	function createFetchTimeout(timeoutMs?: number) {
		if (!timeoutMs || timeoutMs <= 0) return null;

		const controller = new AbortController();
		const timer = setTimeout(() => controller.abort(), timeoutMs);
		return {
			signal: controller.signal,
			cleanup: () => clearTimeout(timer)
		};
	}

	function isAbortError(error: unknown) {
		return error instanceof DOMException && error.name === 'AbortError';
	}

	function controlsNeedToStayVisible() {
		if (!readerChromeReady) return false;

		return (
			!settings.autoHideControls ||
			embedPdfSidebarOpen ||
			isDraggingProgress
		);
	}

	function clearTopBarHideTimer() {
		if (topBarHideTimeout) {
			clearTimeout(topBarHideTimeout);
			topBarHideTimeout = null;
		}
	}

	function clearProgressSaveTimer() {
		if (progressSaveTimer) {
			clearTimeout(progressSaveTimer);
			progressSaveTimer = null;
		}
	}

	function clearEmbedPdfRestoreTimers() {
		embedPdfRestoreTimers.forEach((timer) => clearTimeout(timer));
		embedPdfRestoreTimers = [];
	}

	function queueProgressSave() {
		if (!book || isRestoringProgress || embedPdfRestoringInitialPage || currentPage === lastSavedPage) return;
		clearProgressSaveTimer();
		progressSaveTimer = setTimeout(() => {
			progressSaveTimer = null;
			void saveProgress();
		}, progressSaveDebounceMs);
	}

	function hideTopBar() {
		if (!readerChromeReady) {
			topBarVisible = false;
			clearTopBarHideTimer();
			return;
		}
		if (controlsNeedToStayVisible()) return;
		topBarVisible = false;
	}

	function scheduleTopBarAutoHide() {
		clearTopBarHideTimer();
		if (!readerChromeReady) {
			topBarVisible = false;
			return;
		}
		if (controlsNeedToStayVisible()) return;

		topBarHideTimeout = setTimeout(() => {
			topBarHideTimeout = null;
			hideTopBar();
		}, topBarHideDelayMs);
	}

	function showTopBar(scheduleHide = true) {
		if (!readerChromeReady) {
			topBarVisible = false;
			clearTopBarHideTimer();
			return;
		}
		topBarVisible = true;
		if (scheduleHide) {
			scheduleTopBarAutoHide();
		} else {
			clearTopBarHideTimer();
		}
	}

	function toggleTopBarFromCenterTap() {
		if (
			!readerChromeReady ||
			!settings.autoHideControls ||
			embedPdfSidebarOpen ||
			isDraggingProgress
		) {
			return;
		}
		if (topBarVisible) {
			clearTopBarHideTimer();
			topBarVisible = false;
		} else {
			showTopBar(false);
		}
	}

	function resetTopBarBehavior() {
		if (!readerChromeReady) {
			topBarVisible = false;
			clearTopBarHideTimer();
			return;
		}

		if (!settings.autoHideControls || controlsNeedToStayVisible()) {
			showTopBar(false);
			return;
		}

		showTopBar(true);
	}

	function preserveChromeAfterSeek() {
		preserveChromeAfterSeekUntil = performance.now() + seekScrollSuppressMs;
		showTopBar(false);
	}

	function isTouchLikePointer(e?: PointerEvent | MouseEvent) {
		if (e && 'pointerType' in e && e.pointerType === 'touch') return true;
		return typeof window !== 'undefined' && window.matchMedia('(hover: none), (pointer: coarse)').matches;
	}

	function handleReaderPointerMove(e: MouseEvent) {
		if (!settings.autoHideControls || isTouchLikePointer(e)) return;
		if (e.clientY <= 72) {
			showTopBar(true);
		} else if (pdfReaderEl?.contains(e.target as Node)) {
			scheduleTopBarAutoHide();
		}
	}

	function handleEmbedPdfScrollActivity(delta: number, scrollTop: number) {
		if (!settings.autoHideControls || controlsNeedToStayVisible()) return;

		if (performance.now() < preserveChromeAfterSeekUntil) {
			showTopBar(false);
			return;
		}

		const touchLike = typeof window !== 'undefined' && window.matchMedia('(hover: none), (pointer: coarse)').matches;
		if (scrollTop <= 2 && !touchLike) {
			showTopBar(false);
			return;
		}

		if (delta > scrollHideThresholdPx) {
			hideTopBar();
		} else if (delta < -scrollHideThresholdPx && !touchLike) {
			showTopBar(false);
		}
	}

	function restoreEmbedPdfSavedPage() {
		const targetPage = getSavedProgressPage();
		if (!embedPdfScroll || targetPage <= 1) return;
		if (numPages > 0 && targetPage > numPages) return;

		clearEmbedPdfRestoreTimers();
		embedPdfRestoringInitialPage = true;
		currentPage = targetPage;
		embedPdfInitialPage = targetPage;

		const attemptRestore = () => {
			if (!embedPdfScroll || !embedPdfRestoringInitialPage) return;
			try {
				const scopedScroll = embedPdfScroll.forDocument?.(embedPdfDocumentId);
				scopedScroll?.scrollToPage?.({
					pageNumber: targetPage,
					behavior: 'instant',
					alignY: 0
				});
				if (scopedScroll?.getCurrentPage?.() === targetPage) {
					embedPdfRestoringInitialPage = false;
					clearEmbedPdfRestoreTimers();
				}
			} catch (e) {
				console.warn('Failed to restore saved PDF page:', e);
			}
		};

		attemptRestore();
		embedPdfRestoreTimers = [150, 400, 900, 1600, 2600].map((delay) =>
			setTimeout(attemptRestore, delay)
		);
	}

	function navigateToPage(pageNum: number, behavior: ScrollBehavior = 'smooth', save = true) {
		if (pageNum < 1 || (numPages > 0 && pageNum > numPages)) return;
		currentPage = clampPage(pageNum);

		clearEmbedPdfRestoreTimers();
		embedPdfRestoringInitialPage = false;
		embedPdfInitialPage = currentPage;
		try {
			embedPdfScroll?.forDocument?.(embedPdfDocumentId)?.scrollToPage?.({
				pageNumber: currentPage,
				behavior,
				alignY: 0
			});
		} catch (e) {
			console.warn('Failed to navigate EmbedPDF page:', e);
		}
		if (save) {
			void saveProgress();
		}
	}

	function jumpToPage(pageNum: number) {
		if (pageNum >= 1 && (numPages <= 0 || pageNum <= numPages)) {
			showTopBar(false);
			navigateToPage(pageNum, 'auto');
		}
	}

	function getProgressPageFromClientX(clientX: number) {
		if (!pdfProgressBarEl || numPages <= 0) return null;

		const rect = pdfProgressBarEl.getBoundingClientRect();
		if (rect.width <= 0) return null;

		const x = clientX - rect.left;
		const percentage = Math.max(0, Math.min(1, x / rect.width));
		return clampPage(Math.round(percentage * numPages), numPages);
	}

	function updateProgressDrag(clientX: number) {
		const newPage = getProgressPageFromClientX(clientX);
		if (newPage === null) return;

		pendingProgressPage = newPage;
		currentPage = newPage;
	}

	function updateProgressPreview(clientX: number) {
		const previewPage = getProgressPageFromClientX(clientX);
		if (previewPage === null) return;
		progressPreviewPage = previewPage;
	}

	function clearProgressPreview() {
		if (!isDraggingProgress) {
			progressPreviewPage = null;
		}
	}

	function handleProgressPointerDown(e: PointerEvent) {
		if (numPages <= 0) return;
		if (isBottomSystemGestureStart(e)) {
			return;
		}
		e.preventDefault();
		e.stopPropagation();
		showTopBar(false);
		isDraggingProgress = true;
		pendingProgressPage = null;
		activeProgressPointerId = e.pointerId;
		(e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
		updateProgressPreview(e.clientX);
		updateProgressDrag(e.clientX);
	}

	function handleProgressPointerEnter(e: PointerEvent) {
		if (e.pointerType === 'touch') return;
		updateProgressPreview(e.clientX);
	}

	function handleProgressPointerMove(e: PointerEvent) {
		if (e.pointerType !== 'touch') {
			updateProgressPreview(e.clientX);
		}
		if (!isDraggingProgress) return;
		if (activeProgressPointerId !== null && e.pointerId !== activeProgressPointerId) return;

		e.preventDefault();
		updateProgressDrag(e.clientX);
	}

	function finishProgressPointer(e?: PointerEvent) {
		if (!isDraggingProgress) return;
		const targetPage = pendingProgressPage;

		if (e && activeProgressPointerId !== null) {
			(e.currentTarget as HTMLElement).releasePointerCapture?.(activeProgressPointerId);
		}

		isDraggingProgress = false;
		activeProgressPointerId = null;
		pendingProgressPage = null;
		progressPreviewPage = null;

		if (targetPage !== null) {
			jumpToPage(targetPage);
		}
		preserveChromeAfterSeek();
	}

	function cancelProgressPointer(e?: PointerEvent) {
		if (e && activeProgressPointerId !== null) {
			(e.currentTarget as HTMLElement).releasePointerCapture?.(activeProgressPointerId);
		} else if (pdfProgressBarEl && activeProgressPointerId !== null) {
			try {
				pdfProgressBarEl.releasePointerCapture?.(activeProgressPointerId);
			} catch {
				// Android may already have released capture when a system gesture interrupts touch input.
			}
		}

		isDraggingProgress = false;
		activeProgressPointerId = null;
		pendingProgressPage = null;
		progressPreviewPage = null;
	}

	function handleProgressPointerUp(e: PointerEvent) {
		if (activeProgressPointerId !== null && e.pointerId !== activeProgressPointerId) return;
		e.preventDefault();
		e.stopPropagation();
		finishProgressPointer(e);
	}

	function handleProgressPointerCancel(e: PointerEvent) {
		if (activeProgressPointerId !== null && e.pointerId !== activeProgressPointerId) return;
		cancelProgressPointer(e);
	}

	function recoverInterruptedPdfGesture(remountViewer = false) {
		cancelProgressPointer();
		preserveChromeAfterSeekUntil = 0;

		if (!remountViewer || !book || !embedPdfProgressReady) return;

		clearEmbedPdfRestoreTimers();
		embedPdfScroll = null;
		embedPdfViewerReady = false;
		embedPdfInitialPage = currentPage;
		embedPdfRestoringInitialPage = currentPage > 1;
		pdfResumeToken += 1;
	}

	function handleReaderVisibilityChange() {
		if (document.visibilityState === 'hidden') {
			wasPageInterrupted = true;
			recoverInterruptedPdfGesture();
			return;
		}

		if (wasPageInterrupted) {
			wasPageInterrupted = false;
			recoverInterruptedPdfGesture(true);
		}
	}

	function handleReaderWindowBlur() {
		wasPageInterrupted = true;
		recoverInterruptedPdfGesture();
	}

	function handleReaderWindowFocus() {
		if (!wasPageInterrupted || document.visibilityState === 'hidden') return;
		wasPageInterrupted = false;
		recoverInterruptedPdfGesture(true);
	}

	function handleReaderPageShow(event: PageTransitionEvent) {
		if (!wasPageInterrupted && !event.persisted) return;
		wasPageInterrupted = false;
		recoverInterruptedPdfGesture(true);
	}

	function handleProgressBarKeydown(e: KeyboardEvent) {
		if (numPages <= 0) return;

		if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') {
			e.preventDefault();
			jumpToPage(Math.max(1, currentPage - 1));
			preserveChromeAfterSeek();
		} else if (e.key === 'ArrowRight' || e.key === 'ArrowUp') {
			e.preventDefault();
			jumpToPage(Math.min(numPages, currentPage + 1));
			preserveChromeAfterSeek();
		} else if (e.key === 'Home') {
			e.preventDefault();
			jumpToPage(1);
			preserveChromeAfterSeek();
		} else if (e.key === 'End') {
			e.preventDefault();
			jumpToPage(numPages);
			preserveChromeAfterSeek();
		}
	}

	function isTextEntryTarget(target: EventTarget | null) {
		if (!(target instanceof HTMLElement)) return false;
		return !!target.closest('input, textarea, select, [contenteditable="true"]');
	}

	function handleKeydown(e: KeyboardEvent) {
		if (isTextEntryTarget(e.target)) {
			return;
		}

		if (e.key === 'Tab' || e.key === 'Escape' || e.key.startsWith('Arrow') || e.key === ' ') {
			showTopBar(true);
		}

		if (e.key === 'Escape' && !embedPdfSidebarOpen) {
			void closeReader();
		}
	}

	function toggleFullscreen() {
		toggleReaderFullscreen(settings.useStandardFullscreen).catch(console.error);
	}

	function startCloseBackgroundTasks(defer = false) {
		if (closeTasksStarted) return;
		closeTasksStarted = true;
		clearProgressSaveTimer();

		const run = () => {
			void saveProgress(true);
			void endSession(true);
		};

		if (defer) {
			setTimeout(run, 0);
			return;
		}

		run();
	}

	function closeReader(e?: Event) {
		e?.preventDefault();
		const targetUrl = getReaderReturnUrl();
		readerClosing = true;
		startCloseBackgroundTasks(true);
		void goto(targetUrl, { replaceState: true });
	}

	async function startSession() {
		if (!book || !book.id) return;
		try {
			const res = await fetch(`/api/books/${book.id}/sessions`, {
				method: 'POST',
				credentials: 'same-origin',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ reader_type: 'pdf' })
			});
			if (res.ok) {
				const data = await res.json();
				currentSessionId = data.id;
			}
		} catch (e) {
			console.error('Failed to start session:', e);
		}
	}

	async function endSession(keepalive = false, timeoutMs = keepalive ? 3500 : 0) {
		if (sessionEnded || currentSessionId === null || !book || !book.id) return;
		sessionEnded = true;
		const timeout = createFetchTimeout(timeoutMs);
		try {
			await fetch(`/api/books/${book.id}/sessions/${currentSessionId}`, {
				method: 'PUT',
				credentials: 'same-origin',
				keepalive,
				signal: timeout?.signal
			});
		} catch (e) {
			if (isAbortError(e)) return;
			console.error('Failed to end session:', e);
		} finally {
			timeout?.cleanup();
		}
	}

	async function fetchProgress() {
		try {
			const res = await fetch(`/api/books/${book.id}/progress`, {
				credentials: 'same-origin'
			});
			if (res.ok) {
				savedProgress = await res.json();
				if (savedProgress?.page > 0) {
					lastSavedPage = savedProgress.page;
				}
				progressLoaded = true;
			}
		} catch (e) {
			console.error('Failed to fetch progress:', e);
		}
	}

	async function fetchPdfPageCount() {
		if (!book?.id) return;
		if (book.page_count > 0) {
			numPages = book.page_count;
			return;
		}

		try {
			const res = await fetch(
				`/api/books/${book.id}/pdf/pages${requestedFormat ? `?format=${encodeURIComponent(requestedFormat)}` : ''}`,
				{ credentials: 'same-origin' }
			);
			if (!res.ok) return;
			const data = await res.json();
			if (data.pages > 0) {
				numPages = data.pages;
				book = { ...book, page_count: data.pages };
			}
		} catch (e) {
			console.warn('Failed to fetch PDF page count:', e);
		}
	}

	async function saveProgress(keepalive = false, timeoutMs = keepalive ? 3500 : 0) {
		if (!book) return;
		if (!progressLoaded || !embedPdfProgressReady || embedPdfRestoringInitialPage) return;
		clearProgressSaveTimer();
		const percent = numPages > 0 ? (currentPage / numPages) * 100 : 0;
		const timeout = createFetchTimeout(timeoutMs);
		try {
			const res = await fetch(`/api/books/${book.id}/progress`, {
				method: 'PUT',
				credentials: 'same-origin',
				keepalive,
				signal: timeout?.signal,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					page: currentPage,
					percent: percent,
					status: percent >= 100 ? 'finished' : 'reading'
				})
			});
			if (res.ok) {
				lastSavedPage = currentPage;
			}
		} catch (e) {
			if (isAbortError(e)) return;
			console.error('Failed to save progress:', e);
		} finally {
			timeout?.cleanup();
		}
	}

	function handleEmbedPdfPageChange(pageNum: number, total?: number) {
		if (embedPdfRestoringInitialPage && total && total > 0) {
			embedPdfInitialPage = Math.min(embedPdfInitialPage, total);
		}

		if (embedPdfRestoringInitialPage && pageNum !== embedPdfInitialPage) {
			if (total && total > 0 && total !== numPages) {
				numPages = total;
				restoreEmbedPdfSavedPage();
			}
			return;
		}

		if (embedPdfRestoringInitialPage && pageNum === embedPdfInitialPage) {
			embedPdfRestoringInitialPage = false;
			clearEmbedPdfRestoreTimers();
		}

		if (pageNum !== currentPage) {
			currentPage = pageNum;
			if (total && total > 0) {
				numPages = total;
			}
			queueProgressSave();
		} else if (total && total > 0 && total !== numPages) {
			numPages = total;
		}
	}

	function handleEmbedPdfSidebarOpenChange(open: boolean) {
		embedPdfSidebarOpen = open;
		if (open) {
			showTopBar(false);
		} else {
			resetTopBarBehavior();
		}
	}

	function handleEmbedPdfReady(registry: any) {
		embedPdfScroll = registry.getPlugin?.('scroll')?.provides?.() ?? null;
		embedPdfViewerReady = true;
		pdfLoadRetryAttempts = 0;
		topBarVisible = true;
		restoreEmbedPdfSavedPage();
		resetTopBarBehavior();
	}

	function handleEmbedPdfError(message: string) {
		const normalized = String(message || '');
		const retryable =
			normalized.includes('FPDF_LoadMemDocument') ||
			normalized.toLowerCase().includes('failed to load document');

		if (retryable && pdfLoadRetryAttempts < 1) {
			pdfLoadRetryAttempts += 1;
			embedPdfViewerReady = false;
			error = '';
			pdfLoadRetryToken = Date.now();
			return;
		}

		error = normalized || 'Failed to load PDF';
	}

	onMount(() => {
		if (!browser) return;

		let mounted = true;
		const unsubscribeReaderSettings = readerSettings.subscribe((s) => {
			settings = { ...defaultReaderSettings.pdf, ...s.pdf };
			resetTopBarBehavior();
		});

		void (async () => {
			const bookId = $page.params.bookID;
			requestedFormat = normalizeBookFormat($page.url.searchParams.get('format')) || 'pdf';
			try {
				const res = await fetch(`/api/books/${bookId}`);
				if (res.ok) {
					book = await res.json();
					numPages = book.page_count || 0;
					await fetchProgress();
					if (!savedProgress?.page && savedProgress?.percent > 0 && numPages <= 0) {
						await fetchPdfPageCount();
					}
					currentPage = getSavedProgressPage();
					embedPdfInitialPage = currentPage;
					embedPdfRestoringInitialPage = embedPdfInitialPage > 1;
					embedPdfProgressReady = true;
					await startSession();
				} else {
					error = `Failed to load book details: ${res.status}`;
				}
			} catch (e) {
				console.error('Failed to load book:', e);
				error = 'Failed to load book';
			} finally {
				loading = false;
			}

			if (!mounted) return;

			handlePageExit = () => {
				wasPageInterrupted = true;
				recoverInterruptedPdfGesture();
				if (closeTasksStarted) return;
				void saveProgress(true);
				void endSession(true);
			};
			window.addEventListener('pagehide', handlePageExit);
			window.addEventListener('beforeunload', handlePageExit);
		})();

		window.addEventListener('keydown', handleKeydown);
		window.addEventListener('mousemove', handleReaderPointerMove);
		window.addEventListener('blur', handleReaderWindowBlur);
		window.addEventListener('focus', handleReaderWindowFocus);
		window.addEventListener('pageshow', handleReaderPageShow);
		document.addEventListener('visibilitychange', handleReaderVisibilityChange);

		return () => {
			mounted = false;
			unsubscribeReaderSettings();
			window.removeEventListener('keydown', handleKeydown);
			window.removeEventListener('mousemove', handleReaderPointerMove);
			window.removeEventListener('blur', handleReaderWindowBlur);
			window.removeEventListener('focus', handleReaderWindowFocus);
			window.removeEventListener('pageshow', handleReaderPageShow);
			document.removeEventListener('visibilitychange', handleReaderVisibilityChange);
			if (handlePageExit) {
				window.removeEventListener('pagehide', handlePageExit);
				window.removeEventListener('beforeunload', handlePageExit);
			}
		};
	});

	onDestroy(() => {
		readerClosing = true;
		clearTopBarHideTimer();
		clearProgressSaveTimer();
		clearEmbedPdfRestoreTimers();
		if (handlePageExit) {
			window.removeEventListener('pagehide', handlePageExit);
			window.removeEventListener('beforeunload', handlePageExit);
		}
		if (closeTasksStarted) return;
		void saveProgress(true);
		void endSession(true);
	});
</script>

<svelte:head>
	<title>{book?.title || 'Reading'} - Cryptorum</title>
</svelte:head>

<div
	bind:this={pdfReaderEl}
	class="pdf-reader"
	class:controls-hidden={!topBarVisible}
	class:reader-loading={!readerChromeReady}
	style="background-color: {viewModeBgColors[settings.viewMode]};"
	role="application"
>
	<div
		class="top-reveal-zone"
		role="presentation"
		aria-hidden="true"
		onpointerenter={() => showTopBar(true)}
	></div>

	<div class="embedpdf-wrapper">
			{#if readerChromeReady}
				<a
					href={getReaderReturnUrl()}
					onclick={closeReader}
					class="embedpdf-shell-control embedpdf-close-control nav-btn nav-close"
					title="Close (Esc)"
				>
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<line x1="18" y1="6" x2="6" y2="18"></line>
						<line x1="6" y1="6" x2="18" y2="18"></line>
					</svg>
				</a>
				<div class="embedpdf-shell-title" class:top-nav-hidden={!topBarVisible} title={getPdfDisplayTitle()}>
					<span>{getPdfDisplayTitle()}</span>
				</div>
				<button
					class="embedpdf-shell-control embedpdf-fullscreen-control nav-btn"
					onclick={toggleFullscreen}
					title="Fullscreen"
				>
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<polyline points="15 3 21 3 21 9"></polyline>
						<polyline points="9 21 3 21 3 15"></polyline>
						<line x1="21" y1="3" x2="14" y2="10"></line>
						<line x1="3" y1="21" x2="10" y2="14"></line>
					</svg>
				</button>
				<div class="embedpdf-top-progress-shell" class:top-nav-hidden={!topBarVisible}>
					<ReaderProgressTrack
						progress={progress}
						visible={topBarVisible}
						variant="top"
						ariaLabel="Reading progress"
						ariaValueMin={1}
						ariaValueMax={Math.max(numPages, currentPage)}
						ariaValueNow={currentPage}
					/>
				</div>
			{/if}
			{#if error}
				<div class="error-message">
					<p>{error}</p>
					<a href={getReaderReturnUrl()} class="btn">Return to Library</a>
				</div>
			{:else if book && embedPdfProgressReady}
				<div class="embedpdf-viewer-container">
					{#key `${book.id}:${requestedFormat}:${pdfLoadRetryToken}:${pdfResumeToken}`}
					<EmbedPDFViewer
						src={getPdfSourceUrl()}
						title={getPdfDisplayTitle()}
						initialPage={embedPdfInitialPage}
						toolbarVisible={readerChromeReady && topBarVisible}
						onScrollActivity={handleEmbedPdfScrollActivity}
						onPageChange={handleEmbedPdfPageChange}
						onReady={handleEmbedPdfReady}
						onSidebarOpenChange={handleEmbedPdfSidebarOpenChange}
						onDocumentCenterTap={toggleTopBarFromCenterTap}
						onError={handleEmbedPdfError}
						style="height: 100%; width: 100%;"
					/>
					{/key}
				</div>
			{/if}
			{#if !error && !embedPdfViewerReady}
				<div class="reader-loading-overlay" aria-live="polite">
					<div class="loading-spinner"></div>
					<p>{loading || !book ? 'Loading PDF...' : 'Preparing reader...'}</p>
				</div>
			{/if}
		</div>
		{#if readerChromeReady}
			<ReaderProgressTrack
				bind:element={pdfProgressBarEl}
				progress={progress}
				visible={topBarVisible}
				variant="bottom"
				metaAlign="center"
				interactive={true}
				showThumb={true}
				label={`Page ${currentPage} / ${Math.max(numPages, currentPage)} • ${Math.max(0, Math.min(100, Math.round(progress)))}%`}
				ariaLabel="Reading progress"
				ariaValueMin={1}
				ariaValueMax={Math.max(numPages, currentPage)}
				ariaValueNow={currentPage}
				previewLabel={progressPreviewLabel}
				previewProgress={progressPreviewProgress}
				onpointerdown={(e) => handleProgressPointerDown(e)}
				onpointerenter={(e) => handleProgressPointerEnter(e)}
				onpointerleave={() => clearProgressPreview()}
				onpointermove={(e) => handleProgressPointerMove(e)}
				onpointerup={(e) => handleProgressPointerUp(e)}
				onpointercancel={(e) => handleProgressPointerCancel(e)}
				onkeydown={(e) => handleProgressBarKeydown(e)}
			/>
		{/if}
</div>

<style>
	.pdf-reader {
		--reader-top-bar-height: 56px;
		position: fixed;
		inset: 0;
		z-index: 9999;
		display: block;
		font-family: system-ui, -apple-system, sans-serif;
		overflow: hidden;
	}

	.top-reveal-zone {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		height: 16px;
		z-index: 99;
	}

	.pdf-reader.reader-loading .top-reveal-zone {
		pointer-events: none;
	}

	.nav-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
		border: none;
		border-radius: 6px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		cursor: pointer;
		transition: background-color 0.15s;
	}

	.nav-btn:hover {
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
	}

	.nav-btn:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}

	.icon {
		width: 20px;
		height: 20px;
	}

	.reader-loading-overlay {
		position: absolute;
		inset: 0;
		z-index: 200;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 12px;
		text-align: center;
		color: var(--color-surface-text-muted, #94a3b8);
		background: var(--color-surface-base, #0f172a);
	}

	.reader-loading-overlay p {
		margin: 0;
		font-size: 14px;
	}

	.loading-spinner {
		width: 48px;
		height: 48px;
		border: 3px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-top-color: var(--color-primary-500, #22c55e);
		border-radius: 50%;
		animation: spin 1s linear infinite;
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
	}

	.error-message {
		text-align: center;
	}

	.error-message p {
		color: #ef4444;
		margin-bottom: 16px;
	}

	.btn {
		display: inline-block;
		padding: 8px 16px;
		background: var(--color-primary-500, #22c55e);
		color: white;
		border-radius: 6px;
		text-decoration: none;
	}

	@media (max-width: 768px) {
		.pdf-reader {
			--reader-top-bar-height: 72px;
		}

		.nav-btn {
			width: 40px;
			height: 44px;
		}

		.icon {
			width: 22px;
			height: 22px;
		}

		.loading-spinner {
			width: 40px;
			height: 40px;
		}
	}

	/* EmbedPDF minimal layout */
	.embedpdf-wrapper {
		position: absolute;
		inset: 0;
		background: var(--color-surface-base, #0f172a);
		z-index: 50;
		--embedpdf-shell-control-size: 42px;
		--embedpdf-shell-title-width: clamp(220px, 42vw, 560px);
		--embedpdf-shell-left-reserve: clamp(400px, 35vw, 420px);
		--embedpdf-shell-right-reserve: clamp(112px, 20vw, 240px);
	}

	:global([data-epdf-i="left-group"]) {
		margin-left: calc(56px + max(0px, env(safe-area-inset-left))) !important;
		margin-right: auto !important;
		flex-shrink: 0;
		transition: opacity 0.18s ease, transform 0.18s ease;
	}

	:global([data-epdf-i="center-group"]) {
		margin-left: 0 !important;
		transform: none !important;
		flex-shrink: 0;
		transition: opacity 0.18s ease, transform 0.18s ease;
	}

	:global([data-epdf-i="right-group"]) {
		position: relative;
		z-index: 25;
		margin-right: calc(56px + max(0px, env(safe-area-inset-right))) !important;
		transition: opacity 0.18s ease, transform 0.18s ease;
	}

	:global([data-epdf-i="comment-button"]),
	:global([data-epdf-cat~="panel-comment"]),
	:global([data-epdf-i="document-menu-button"]),
	:global([data-epdf-cat~="document-menu"]),
	:global(button[aria-label="Document Menu"]),
	:global([data-epdf-i="divider-1"]) {
		display: none !important;
		visibility: hidden !important;
		pointer-events: none !important;
	}

	:global(.pdf-reader.controls-hidden [data-epdf-i="left-group"]),
	:global(.pdf-reader.controls-hidden [data-epdf-i="center-group"]),
	:global(.pdf-reader.controls-hidden [data-epdf-i="right-group"]) {
		opacity: 0;
		pointer-events: none;
		transform: translateY(-10px);
	}

	:global([data-cryptorum-embedpdf-toolbar="true"]) {
		display: flex !important;
		align-items: center !important;
		justify-content: flex-start !important;
		gap: 4px !important;
		padding-left: 0 !important;
	}

	.embedpdf-shell-control {
		display: flex;
		align-items: center;
		justify-content: center;
		position: absolute;
		top: calc((var(--reader-top-bar-height) - var(--embedpdf-shell-control-size)) / 2);
		width: var(--embedpdf-shell-control-size);
		height: var(--embedpdf-shell-control-size);
		border: none;
		background: color-mix(in srgb, var(--color-surface-base, #0f172a) 92%, transparent);
		color: var(--color-surface-text, #e2e8f0);
		cursor: pointer;
		text-decoration: none;
		transition: opacity 0.18s ease, transform 0.18s ease, background-color 0.16s ease;
		z-index: 130;
	}

	.embedpdf-shell-control::after {
		display: none;
	}

	.embedpdf-close-control {
		left: calc(6px + max(0px, env(safe-area-inset-left)));
		border-radius: 10px;
	}

	.embedpdf-fullscreen-control {
		right: calc(6px + max(0px, env(safe-area-inset-right)));
		border-radius: 10px;
	}

	.embedpdf-shell-title {
		position: absolute;
		top: 0;
		left: calc(var(--embedpdf-shell-left-reserve) + max(0px, env(safe-area-inset-left)));
		right: calc(var(--embedpdf-shell-right-reserve) + max(0px, env(safe-area-inset-right)));
		height: var(--reader-top-bar-height);
		display: flex;
		align-items: center;
		justify-content: center;
		min-width: 0;
		padding: 0 14px;
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		background: var(--color-surface-base, #0f172a);
		color: var(--color-surface-text, #e2e8f0);
		font-size: 14px;
		font-weight: 600;
		line-height: 1.2;
		pointer-events: none;
		transition: opacity 0.18s ease, transform 0.18s ease;
		z-index: 110;
	}

	.embedpdf-shell-title span {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.embedpdf-top-progress-shell {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		height: var(--reader-top-bar-height);
		pointer-events: none;
		transition: opacity 0.18s ease, transform 0.18s ease;
		z-index: 120;
	}

	.embedpdf-viewer-container {
		position: absolute;
		inset: 0;
		overflow: hidden;
	}

	.embedpdf-shell-control:hover {
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
	}

	.pdf-reader.controls-hidden .embedpdf-shell-control {
		opacity: 0;
		pointer-events: none;
		transform: translateY(calc(-1 * var(--reader-top-bar-height)));
	}

	.pdf-reader.controls-hidden .embedpdf-shell-title,
	.pdf-reader.controls-hidden .embedpdf-top-progress-shell {
		opacity: 0;
		transform: translateY(-100%);
	}

	:global([data-overlay-id="page-controls"]),
	:global([data-overlay-id="page-controls"] *) {
		opacity: 0 !important;
		visibility: hidden !important;
		pointer-events: none !important;
		transform: translateY(8px) !important;
	}

	@media (min-width: 641px) and (max-width: 767px) {
		.embedpdf-wrapper {
			--embedpdf-shell-left-reserve: 272px;
			--embedpdf-shell-right-reserve: 84px;
		}
	}

	@media (max-width: 640px) {
		.embedpdf-wrapper {
			--embedpdf-shell-control-size: 44px;
			--embedpdf-shell-title-width: clamp(136px, 38vw, 260px);
			--embedpdf-shell-left-reserve: 224px;
			--embedpdf-shell-right-reserve: clamp(92px, 24vw, 180px);
		}

		.embedpdf-shell-title {
			padding: 0 10px;
			font-size: 13px;
		}
	}

	@media (max-width: 420px) {
		.embedpdf-wrapper {
			--embedpdf-shell-title-width: 108px;
			--embedpdf-shell-left-reserve: 204px;
			--embedpdf-shell-right-reserve: 84px;
		}

		.embedpdf-shell-title {
			padding: 0 8px;
		}
	}
</style>

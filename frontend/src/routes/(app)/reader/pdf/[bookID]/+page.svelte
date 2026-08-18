<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { ZoomMode } from '@embedpdf/plugin-zoom';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { browser } from '$app/environment';
	import { defaultReaderSettings, readerSettings } from '$lib/stores/readerSettings';
	import type { PdfReaderSetting, PdfViewMode } from '$lib/stores/readerSettings';
	import { normalizeBookFormat } from '$lib/utils/book-formats';
	import { getReaderDisplayTitle } from '$lib/utils/reader-title';
	import { toggleReaderFullscreen } from '$lib/utils/fullscreen';
	import { isBottomSystemGestureStart } from '$lib/utils/system-gesture-guard';
	import EmbedPDFViewer from '$lib/components/EmbedPDFViewer.svelte';
	import ReaderProgressTrack from '$lib/components/ReaderProgressTrack.svelte';
	import {
		ReadingProgressController,
		readingPositionAsLegacy,
		selectReaderFile,
		type ReaderFile
	} from '$lib/services/reading-progress';

	let book = $state<any>(null);
	let readerFiles = $state<any[]>([]);
	let loading = $state(true);
	let error = $state('');
	let currentPage = $state(1);
	let numPages = $state(0);
	let savedProgress = $state<any>(null);
	let requestedFormat = $state('pdf');
	let activeFile = $state<ReaderFile | null>(null);
	let progressController: ReadingProgressController | null = null;
	let embedPdfProgressReady = $state(false);
	let embedPdfInitialPage = $state(1);
	let embedPdfViewerReady = $state(false);
	let embedPdfScroll: any = null;
	let embedPdfUiScope = $state<any>(null);
	let embedPdfZoomScope = $state<any>(null);
	let embedPdfRotateScope = $state<any>(null);
	let embedPdfExportScope = $state<any>(null);
	let embedPdfPrintScope = $state<any>(null);
	let unsubscribeEmbedPdfZoomState: (() => void) | null = null;
	let embedPdfRestoringInitialPage = false;
	let embedPdfRestoreTimers: ReturnType<typeof setTimeout>[] = [];
	let embedPdfSidebarOpen = $state(false);
	let embedPdfThumbnailsOpen = $state(false);
	let embedPdfSearchOpen = $state(false);
	let rightSidebarOpen = $state(false);
	let pdfLoadRetryToken = $state(0);
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
	let currentZoomPercent = $state<number | null>(null);
	let currentZoomLevel = $state<number | string | null>(null);

	const progress = $derived(numPages > 1 ? ((currentPage - 1) / (numPages - 1)) * 100 : 0);
	const progressPreviewProgress = $derived(
		progressPreviewPage !== null && numPages > 1 ? ((progressPreviewPage - 1) / (numPages - 1)) * 100 : 0
	);
	const progressPreviewLabel = $derived(
		progressPreviewPage !== null && numPages > 0
			? `Page ${progressPreviewPage} / ${numPages} • ${Math.round(numPages > 1 ? ((progressPreviewPage - 1) / (numPages - 1)) * 100 : 0)}%`
			: ''
	);
	const readerChromeReady = $derived(embedPdfViewerReady && !loading && !error);
	const readerTitle = $derived(getReaderDisplayTitle(book, readerFiles, loading, requestedFormat));

	const topBarHideDelayMs = 2800;
	const scrollHideThresholdPx = 8;
	const seekScrollSuppressMs = 1400;
	const progressSaveDebounceMs = 750;
	const embedPdfDocumentId = 'cryptorum-pdf';
	const embedPdfRestoreDelays = [0, 120, 300, 650, 1200, 2200, 3600, 5600, 8200, 11500, 15500];

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
			// The renderer's document total is authoritative; metadata may be stale.
			return clampPage(savedProgress.page, 0);
		}

		if (
			typeof savedProgress?.percent === 'number' &&
			savedProgress.percent > 0 &&
			numPages > 0
		) {
			return clampPage(Math.round((savedProgress.percent / 100) * Math.max(0, numPages - 1)) + 1);
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
		return readerTitle;
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
		if (activeFile) {
			params.set('file_id', String(activeFile.id));
		}
		const query = params.toString();
		return `/api/books/${book.id}/file${query ? `?${query}` : ''}`;
	}

	function controlsNeedToStayVisible() {
		if (!readerChromeReady) return false;

		return (
			!settings.autoHideControls ||
			embedPdfSidebarOpen ||
			rightSidebarOpen ||
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
			} catch (e) {
				console.warn('Failed to restore saved PDF page:', e);
				return;
			}
		};

		embedPdfRestoreTimers = embedPdfRestoreDelays.map((delay) =>
			setTimeout(attemptRestore, delay)
		);
	}

	function navigateToPage(pageNum: number, behavior: ScrollBehavior = 'smooth', _save = true) {
		if (pageNum < 1 || (numPages > 0 && pageNum > numPages)) return;
		const targetPage = clampPage(pageNum);

		clearEmbedPdfRestoreTimers();
		embedPdfRestoringInitialPage = false;
		try {
			embedPdfScroll?.forDocument?.(embedPdfDocumentId)?.scrollToPage?.({
				pageNumber: targetPage,
				behavior,
				alignY: 0
			});
		} catch (e) {
			console.warn('Failed to navigate EmbedPDF page:', e);
		}
		// Progress is queued by handleEmbedPdfPageChange only after the viewport
		// reports the requested page. Never save this optimistic command target.
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
		return clampPage(Math.round(percentage * Math.max(0, numPages - 1)) + 1, numPages);
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

	function recoverInterruptedPdfGesture() {
		cancelProgressPointer();
		preserveChromeAfterSeekUntil = 0;
	}

	function handleReaderVisibilityChange() {
		if (document.visibilityState === 'hidden') {
			wasPageInterrupted = true;
			recoverInterruptedPdfGesture();
			void saveProgress(true);
			return;
		}

		if (wasPageInterrupted) {
			wasPageInterrupted = false;
			if (embedPdfRestoringInitialPage) {
				restoreEmbedPdfSavedPage();
			}
		}
	}

	function handleReaderWindowBlur() {
		wasPageInterrupted = true;
		recoverInterruptedPdfGesture();
	}

	function handleReaderWindowFocus() {
		if (!wasPageInterrupted || document.visibilityState === 'hidden') return;
		wasPageInterrupted = false;
		if (embedPdfRestoringInitialPage) {
			restoreEmbedPdfSavedPage();
		}
	}

	function handleReaderPageShow(event: PageTransitionEvent) {
		if (!wasPageInterrupted && !event.persisted) return;
		wasPageInterrupted = false;
		if (embedPdfRestoringInitialPage) {
			restoreEmbedPdfSavedPage();
		}
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

		if (e.key === 'Escape') {
			if (rightSidebarOpen) {
				rightSidebarOpen = false;
				resetTopBarBehavior();
			} else if (!embedPdfSidebarOpen) {
				void closeReader();
			}
		}
	}

	function toggleFullscreen() {
		toggleReaderFullscreen(settings.useStandardFullscreen).catch(console.error);
	}

	function closeEmbedPdfSidebars() {
		embedPdfUiScope?.closeSidebarSlot?.('left', 'main');
		embedPdfUiScope?.closeSidebarSlot?.('right', 'main');
	}

	function togglePdfThumbnails() {
		if (!embedPdfUiScope) return;
		rightSidebarOpen = false;
		showTopBar(false);
		embedPdfUiScope.toggleSidebar?.('left', 'main', 'sidebar-panel');
	}

	function togglePdfSearch() {
		if (!embedPdfUiScope) return;
		rightSidebarOpen = false;
		showTopBar(false);
		embedPdfUiScope.toggleSidebar?.('right', 'main', 'search-panel');
	}

	function toggleRightSidebar() {
		rightSidebarOpen = !rightSidebarOpen;
		if (rightSidebarOpen) {
			closeEmbedPdfSidebars();
			showTopBar(false);
		} else {
			resetTopBarBehavior();
		}
	}

	function updatePdfSetting<K extends keyof PdfReaderSetting>(key: K, value: PdfReaderSetting[K]) {
		settings = { ...settings, [key]: value };
		readerSettings.updatePdf({ [key]: value });
		resetTopBarBehavior();
	}

	function resetPdfSettings() {
		settings = { ...defaultReaderSettings.pdf };
		readerSettings.resetToDefaults('pdf');
		resetTopBarBehavior();
	}

	function zoomPdfOut() {
		embedPdfZoomScope?.zoomOut?.();
		showTopBar(false);
	}

	function zoomPdfIn() {
		embedPdfZoomScope?.zoomIn?.();
		showTopBar(false);
	}

	function fitPdfToWidth() {
		embedPdfZoomScope?.requestZoom?.(ZoomMode.FitWidth);
		showTopBar(false);
	}

	function rotatePdf() {
		embedPdfRotateScope?.rotateForward?.();
		showTopBar(false);
	}

	function downloadPdf() {
		embedPdfExportScope?.download?.();
		showTopBar(false);
	}

	function printPdf() {
		embedPdfPrintScope?.print?.();
		showTopBar(false);
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
		if (!book?.id || !activeFile) return;
		progressController?.destroy();
		progressController = new ReadingProgressController({
			bookId: book.id,
			file: activeFile,
			channel: 'standard',
			readerMode: 'pdf'
		});
		const position = await progressController.start();
		savedProgress = readingPositionAsLegacy(position, 'pdf');
		if (savedProgress?.page > 0) lastSavedPage = savedProgress.page;
		progressLoaded = true;
	}

	async function endSession(keepalive = false, _timeoutMs = 0) {
		if (sessionEnded || !progressController) return;
		sessionEnded = true;
		await progressController.end(keepalive);
	}

	async function fetchPdfPageCount() {
		if (!book?.id) return;
		if (book.page_count > 0) {
			numPages = book.page_count;
			return;
		}

		try {
			const res = await fetch(
				`/api/books/${book.id}/pdf/pages?format=${encodeURIComponent(requestedFormat)}${activeFile ? `&file_id=${activeFile.id}` : ''}`,
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

	async function saveProgress(_keepalive = false, _timeoutMs = 0) {
		if (!book || !progressController) return;
		if (!progressLoaded || !embedPdfProgressReady || embedPdfRestoringInitialPage) return;
		clearProgressSaveTimer();
		const percent = numPages > 1 ? ((currentPage - 1) / (numPages - 1)) * 100 : 0;
		await progressController.checkpoint({
			readerMode: 'pdf',
			percent,
			locator: { type: 'pdf_page', page: currentPage, total_pages: numPages },
			reachedEnd: numPages > 0 && currentPage === numPages
		});
		lastSavedPage = currentPage;
	}

	function handleEmbedPdfPageChange(pageNum: number, total?: number) {
		if (embedPdfRestoringInitialPage && total && total > 0) {
			embedPdfInitialPage = Math.min(embedPdfInitialPage, total);
		}

		if (embedPdfRestoringInitialPage && pageNum !== embedPdfInitialPage) {
			if (total && total > 0 && total !== numPages) {
				numPages = total;
			}
			return;
		}

		if (embedPdfRestoringInitialPage && pageNum === embedPdfInitialPage) {
			if (pageNum !== currentPage) {
				currentPage = pageNum;
			}
			if (total && total > 0 && total !== numPages) {
				numPages = total;
			}
			embedPdfViewerReady = true;
			topBarVisible = true;
			clearEmbedPdfRestoreTimers();
			embedPdfRestoringInitialPage = false;
			return;
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

	function handleEmbedPdfSidebarOpenChange(
		open: boolean,
		state?: { searchOpen?: boolean; thumbnailOpen?: boolean }
	) {
		embedPdfSidebarOpen = open;
		embedPdfSearchOpen = !!state?.searchOpen;
		embedPdfThumbnailsOpen = !!state?.thumbnailOpen;
		if (open) {
			showTopBar(false);
		} else {
			resetTopBarBehavior();
		}
	}

	function updateEmbedPdfZoomPercent() {
		const zoomState = embedPdfZoomScope?.getState?.();
		const zoomPercent = zoomState?.currentZoomLevel;
		const zoomLevel = zoomState?.zoomLevel;
		currentZoomPercent = typeof zoomPercent === 'number' ? Math.round(zoomPercent * 100) : null;
		currentZoomLevel = typeof zoomLevel === 'number' || typeof zoomLevel === 'string' ? zoomLevel : null;
	}

	function handleEmbedPdfReady(registry: any) {
		unsubscribeEmbedPdfZoomState?.();
		unsubscribeEmbedPdfZoomState = null;
		embedPdfScroll = registry.getPlugin?.('scroll')?.provides?.() ?? null;
		embedPdfUiScope = registry.getPlugin?.('ui')?.provides?.()?.forDocument?.(embedPdfDocumentId) ?? null;
		embedPdfZoomScope = registry.getPlugin?.('zoom')?.provides?.()?.forDocument?.(embedPdfDocumentId) ?? null;
		embedPdfRotateScope = registry.getPlugin?.('rotate')?.provides?.()?.forDocument?.(embedPdfDocumentId) ?? null;
		embedPdfExportScope = registry.getPlugin?.('export')?.provides?.()?.forDocument?.(embedPdfDocumentId) ?? null;
		embedPdfPrintScope = registry.getPlugin?.('print')?.provides?.()?.forDocument?.(embedPdfDocumentId) ?? null;
		updateEmbedPdfZoomPercent();
		unsubscribeEmbedPdfZoomState = embedPdfZoomScope?.onStateChange?.(() => {
			updateEmbedPdfZoomPercent();
		}) ?? null;
		embedPdfViewerReady = embedPdfInitialPage <= 1;
		pdfLoadRetryAttempts = 0;
		topBarVisible = true;
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
			unsubscribeEmbedPdfZoomState?.();
			unsubscribeEmbedPdfZoomState = null;
			embedPdfUiScope = null;
			embedPdfZoomScope = null;
			embedPdfRotateScope = null;
			embedPdfExportScope = null;
			embedPdfPrintScope = null;
			currentZoomPercent = null;
			currentZoomLevel = null;
			clearEmbedPdfRestoreTimers();
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
					const [bookRes, filesRes] = await Promise.all([
						fetch(`/api/books/${bookId}`),
						fetch(`/api/books/${bookId}/files`)
					]);
					if (bookRes.ok) {
						book = await bookRes.json();
					if (filesRes.ok) {
						readerFiles = await filesRes.json();
					}
					activeFile = selectReaderFile(readerFiles, requestedFormat, ['pdf']);
					numPages = book.page_count || 0;
					await startSession();
					if (
						numPages <= 0 &&
						(savedProgress?.page > 0 || savedProgress?.percent > 0)
					) {
						await fetchPdfPageCount();
					}
					currentPage = getSavedProgressPage();
					embedPdfInitialPage = currentPage;
					embedPdfRestoringInitialPage = embedPdfInitialPage > 1;
					embedPdfProgressReady = true;
					} else {
						error = `Failed to load book details: ${bookRes.status}`;
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
		unsubscribeEmbedPdfZoomState?.();
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
		progressController?.destroy();
	});
</script>

<svelte:head>
	<title>{readerTitle === 'Loading...' ? 'Reading' : readerTitle} - Cryptorum</title>
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

	{#if readerChromeReady}
		<header class="top-nav" class:top-nav-hidden={!topBarVisible}>
			<div class="nav-left">
				<a
					href={getReaderReturnUrl()}
					onclick={closeReader}
					class="nav-btn nav-close"
					title="Close (Esc)"
				>
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<line x1="18" y1="6" x2="6" y2="18"></line>
						<line x1="6" y1="6" x2="18" y2="18"></line>
					</svg>
				</a>

				<button
					onclick={togglePdfThumbnails}
					class="nav-btn"
					class:active={embedPdfThumbnailsOpen}
					disabled={!embedPdfUiScope}
					title="Pages"
				>
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<rect x="3" y="3" width="7" height="7"></rect>
						<rect x="14" y="3" width="7" height="7"></rect>
						<rect x="14" y="14" width="7" height="7"></rect>
						<rect x="3" y="14" width="7" height="7"></rect>
					</svg>
				</button>
			</div>

			<div class="nav-center">
				<span class="book-title">{getPdfDisplayTitle()}</span>
				<span class="chapter-title">Page {currentPage} / {Math.max(numPages, currentPage)}</span>
			</div>

			<div class="nav-right">
				<button
					onclick={togglePdfSearch}
					class="nav-btn mobile-secondary"
					class:active={embedPdfSearchOpen}
					disabled={!embedPdfUiScope}
					title="Search (Ctrl+F)"
				>
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<circle cx="11" cy="11" r="8"></circle>
						<line x1="21" y1="21" x2="16.65" y2="16.65"></line>
					</svg>
				</button>

				<div class="zoom-controls">
					<button onclick={zoomPdfOut} class="nav-btn mobile-secondary" disabled={!embedPdfZoomScope} title="Zoom out">
						<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<line x1="5" y1="12" x2="19" y2="12"></line>
						</svg>
					</button>

					<span class="zoom-label" aria-label="Zoom level">{currentZoomPercent === null ? 'Fit' : `${currentZoomPercent}%`}</span>

					<button onclick={zoomPdfIn} class="nav-btn mobile-secondary" disabled={!embedPdfZoomScope} title="Zoom in">
						<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<line x1="12" y1="5" x2="12" y2="19"></line>
							<line x1="5" y1="12" x2="19" y2="12"></line>
						</svg>
					</button>

					<button
						onclick={fitPdfToWidth}
						class="nav-btn"
						class:active={currentZoomLevel === ZoomMode.FitWidth}
						disabled={!embedPdfZoomScope}
						title="Fit to width"
						aria-label="Fit to width"
					>
						<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<path d="M6 3H4a1 1 0 0 0-1 1v16a1 1 0 0 0 1 1h2"></path>
							<path d="M18 3h2a1 1 0 0 1 1 1v16a1 1 0 0 1-1 1h-2"></path>
							<path d="M8 12h8"></path>
							<path d="m10 9-3 3 3 3"></path>
							<path d="m14 9 3 3-3 3"></path>
						</svg>
					</button>
				</div>

				<button onclick={rotatePdf} class="nav-btn mobile-secondary" disabled={!embedPdfRotateScope} title="Rotate">
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M21 12a9 9 0 1 1-3-6.7"></path>
						<polyline points="21 3 21 9 15 9"></polyline>
					</svg>
				</button>

				<button onclick={toggleRightSidebar} class="nav-btn" class:active={rightSidebarOpen} title="Settings">
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<circle cx="12" cy="12" r="3"></circle>
						<path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
					</svg>
				</button>

				<button onclick={toggleFullscreen} class="nav-btn mobile-secondary" title="Fullscreen">
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<polyline points="15 3 21 3 21 9"></polyline>
						<polyline points="9 21 3 21 3 15"></polyline>
						<line x1="21" y1="3" x2="14" y2="10"></line>
						<line x1="3" y1="21" x2="10" y2="14"></line>
					</svg>
				</button>
			</div>

			<ReaderProgressTrack
				progress={progress}
				visible={topBarVisible}
				variant="top"
				ariaLabel="Reading progress"
				ariaValueMin={1}
				ariaValueMax={Math.max(numPages, currentPage)}
				ariaValueNow={currentPage}
			/>
		</header>
	{/if}

	<div class="embedpdf-wrapper">
		{#if error}
			<div class="error-message">
				<p>{error}</p>
				<a href={getReaderReturnUrl()} class="btn">Return to Library</a>
			</div>
		{:else if book && embedPdfProgressReady}
			<div class="embedpdf-viewer-container">
				{#key `${book.id}:${requestedFormat}:${pdfLoadRetryToken}`}
				<EmbedPDFViewer
					src={getPdfSourceUrl()}
					title={getPdfDisplayTitle()}
					initialPage={embedPdfInitialPage}
					toolbarVisible={false}
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

		{#if readerChromeReady}
			<aside class="right-sidebar" class:open={rightSidebarOpen}>
				<div class="settings-tabs">
					<button class="settings-tab active" title="PDF Settings">PDF</button>
				</div>

				<div class="settings-content">
					<div class="settings-section">
						<label class="settings-label" for="pdf-view-mode">View mode</label>
						<select
							id="pdf-view-mode"
							value={settings.viewMode}
							onchange={(e) => updatePdfSetting('viewMode', e.currentTarget.value as PdfViewMode)}
							class="settings-select"
						>
							<option value="light">Light</option>
							<option value="dark">Dark</option>
							<option value="trueDark">True black</option>
						</select>
					</div>

					<div class="settings-section">
						<label class="toggle-option">
							<input
								type="checkbox"
								checked={settings.autoHideControls}
								onchange={(e) => updatePdfSetting('autoHideControls', e.currentTarget.checked)}
							/>
							<span>Auto-hide controls</span>
						</label>
					</div>

					<div class="settings-section">
						<label class="toggle-option">
							<input
								type="checkbox"
								checked={settings.useStandardFullscreen}
								onchange={(e) => updatePdfSetting('useStandardFullscreen', e.currentTarget.checked)}
							/>
							<span>Use browser fullscreen</span>
						</label>
					</div>

					<div class="settings-section">
						<div class="action-row">
							<button type="button" class="settings-action" onclick={downloadPdf} disabled={!embedPdfExportScope}>Download</button>
							<button type="button" class="settings-action" onclick={printPdf} disabled={!embedPdfPrintScope}>Print</button>
						</div>
					</div>

					<button type="button" class="reset-btn" onclick={resetPdfSettings}>Reset PDF settings</button>
				</div>
			</aside>
		{/if}
</div>

<style>
	.pdf-reader {
		--reader-top-bar-height: 64px;
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

	.top-nav {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		column-gap: 16px;
		height: var(--reader-top-bar-height);
		padding: 0 12px;
		background: var(--color-surface-base, #0f172a);
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		z-index: 150;
		transform: translateY(0);
		transition: transform 0.22s ease, opacity 0.22s ease;
		will-change: transform, opacity;
	}

	.top-nav-hidden {
		transform: translateY(-100%);
		opacity: 0;
		pointer-events: none;
	}

	.nav-left,
	.nav-center,
	.nav-right {
		display: flex;
		align-items: center;
		gap: 4px;
		min-width: 0;
	}

	.nav-center {
		justify-content: center;
		flex-direction: column;
		gap: 0;
		text-align: center;
	}

	.nav-right {
		justify-content: flex-end;
	}

	.zoom-controls {
		display: flex;
		align-items: center;
		gap: 0;
	}

	.nav-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 44px;
		height: 44px;
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

	.nav-btn.active {
		background: var(--color-primary-500, #22c55e);
		color: white;
	}

	.icon {
		width: 20px;
		height: 20px;
	}

	.book-title {
		max-width: min(48vw, 560px);
		overflow: hidden;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 14px;
		font-weight: 600;
		line-height: 1.2;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.chapter-title {
		max-width: min(44vw, 460px);
		overflow: hidden;
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 11px;
		line-height: 1.2;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.zoom-label {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 44px;
		height: 32px;
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 12px;
		font-weight: 600;
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

	.right-sidebar {
		position: absolute;
		right: 0;
		top: var(--reader-top-bar-height);
		bottom: 0;
		width: 360px;
		display: flex;
		flex-direction: column;
		background: var(--color-surface-base, #0f172a);
		border-left: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		transform: translateX(100%);
		transition: transform 0.25s ease-in-out;
		z-index: 120;
	}

	.right-sidebar.open {
		transform: translateX(0);
	}

	.settings-tabs {
		display: flex;
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
	}

	.settings-tab {
		flex: 1;
		padding: 12px 8px;
		border: none;
		background: transparent;
		color: var(--color-primary-500, #22c55e);
		font-size: 12px;
		font-weight: 600;
		box-shadow: inset 0 -2px 0 var(--color-primary-500, #22c55e);
	}

	.settings-content {
		flex: 1;
		overflow-y: auto;
		padding: 16px;
	}

	.settings-section {
		margin-bottom: 20px;
	}

	.settings-label {
		display: block;
		margin-bottom: 8px;
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 12px;
	}

	.settings-select {
		width: 100%;
		padding: 10px 12px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 6px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text, #e2e8f0);
		font-size: 13px;
	}

	.toggle-option {
		display: flex;
		align-items: center;
		gap: 10px;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 13px;
	}

	.toggle-option input[type="checkbox"] {
		width: 16px;
		height: 16px;
		accent-color: var(--color-primary-500, #22c55e);
	}

	.action-row {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 8px;
	}

	.settings-action,
	.reset-btn {
		width: 100%;
		padding: 10px 12px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 6px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text, #e2e8f0);
		font-size: 13px;
		cursor: pointer;
	}

	.settings-action:hover:not(:disabled),
	.reset-btn:hover {
		background: var(--color-primary-500, #22c55e);
		color: white;
	}

	.settings-action:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	@media (max-width: 768px) {
		.pdf-reader {
			--reader-top-bar-height: 72px;
		}

		.top-nav {
			column-gap: 8px;
			padding: 0 8px;
		}

		.nav-btn {
			width: 44px;
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

		.book-title {
			max-width: 36vw;
			font-size: 13px;
		}

		.chapter-title {
			max-width: 34vw;
		}
	}

	@media (max-width: 640px) {
		.mobile-secondary,
		.zoom-label {
			display: none;
		}

		.right-sidebar {
			left: 0;
			width: auto;
		}
	}

	/* EmbedPDF minimal layout */
	.embedpdf-wrapper {
		position: absolute;
		inset: 0;
		background: var(--color-surface-base, #0f172a);
		z-index: 50;
		--embedpdf-shell-control-size: 44px;
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

	.embedpdf-viewer-container {
		position: absolute;
		inset: 0;
		overflow: hidden;
	}

	:global([data-overlay-id="page-controls"]),
	:global([data-overlay-id="page-controls"] *) {
		opacity: 0 !important;
		visibility: hidden !important;
		pointer-events: none !important;
		transform: translateY(8px) !important;
	}

	@media (max-width: 640px) {
		.embedpdf-wrapper {
			--embedpdf-shell-control-size: 44px;
			--embedpdf-shell-title-width: clamp(136px, 38vw, 260px);
			--embedpdf-shell-left-reserve: 224px;
			--embedpdf-shell-right-reserve: clamp(92px, 24vw, 180px);
		}
	}
</style>

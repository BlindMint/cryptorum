<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { browser } from '$app/environment';
	import { readerSettings, cbxFitModes, cbxScrollModes, type CbxReaderSetting } from '$lib/stores/readerSettings';
	import ReaderProgressTrack from '$lib/components/ReaderProgressTrack.svelte';
	import { normalizeBookFormat } from '$lib/utils/book-formats';
	import { getReaderDisplayTitle } from '$lib/utils/reader-title';
	import { toggleReaderFullscreen } from '$lib/utils/fullscreen';
	import { isBottomSystemGestureStart } from '$lib/utils/system-gesture-guard';

	let book = $state<any>(null);
	let readerFiles = $state<any[]>([]);
	let loading = $state(true);
	let numPages = $state(0);
	let currentPage = $state(1);
	let currentSpreadPages = $state<[number, number] | null>(null);
	let savedProgress = $state<any>(null);
	let currentSessionId = $state<number | null>(null);

	let settings = $state<CbxReaderSetting>({
		pageSpread: 'off',
		pageLayout: 'single',
		fitMode: 'fit-width',
		scrollMode: 'paginated',
		backgroundColor: '#111111',
		readingDirection: 'ltr',
		stripMaxWidthPercent: 100,
		mangaMode: false,
		panelViewEnabled: false,
		spreadHandling: 'auto',
		pageTransitionSound: false,
		autoHideControls: true,
		useStandardFullscreen: false,
		vibrance: 100,
		saturation: 100
	});

	let leftSidebarOpen = $state(false);
	let rightSidebarOpen = $state(false);
	let activeSettingsTab = $state<'display' | 'comic' | 'advanced'>('display');
	let isDraggingProgress = $state(false);
	let pendingProgressPage = $state<number | null>(null);
	let progressPreviewPage = $state<number | null>(null);
	let progressBarEl = $state<HTMLElement | null>(null);
	let activeProgressPointerId: number | null = null;
	const progressPreviewProgress = $derived(
		progressPreviewPage !== null && numPages > 0 ? (progressPreviewPage / numPages) * 100 : null
	);
	const progressPreviewLabel = $derived(
		progressPreviewPage !== null && numPages > 0
			? `Page ${progressPreviewPage} / ${numPages} • ${Math.round((progressPreviewPage / numPages) * 100)}%`
			: ''
	);
	let lastWheelNavigationAt = 0;
	let topBarHideTimeout: ReturnType<typeof setTimeout> | null = null;
	let lastLongStripScrollTop = 0;
	let preserveChromeAfterSeekUntil = 0;
	let sessionEnded = false;
	let handlePageExit: (() => void) | null = null;
	let requestedFormat = $state('');
	let topBarVisible = $state(true);
	let stripScrollEl = $state<HTMLDivElement | null>(null);
	let stripProgressSaveTimeout: ReturnType<typeof setTimeout> | null = null;
	let stripRestoreScrollTimeout: ReturnType<typeof setTimeout> | null = null;
	let isRestoringCbxProgress = false;
	let closeTasksStarted = false;
	let readerPointerStart: { id: number; x: number; y: number } | null = null;
	let readerPointerMoved = false;
	let showCurrentSection = $state(true);
	let activeComicLoadToken = 0;
	let loadedComicKey = '';

	const TOP_BAR_HIDE_DELAY_MS = 2800;
	const TOP_BAR_SCROLL_DELTA = 12;
	const SEEK_SCROLL_SUPPRESS_MS = 1400;
	const TOP_BAR_REVEAL_EDGE_PX = 72;
	const STRIP_PADDING_PX = 16;

	const progress = $derived(numPages > 0 ? (currentPage / numPages) * 100 : 0);
	const readerTitle = $derived(getReaderDisplayTitle(book, readerFiles, loading, requestedFormat));

	async function loadComic(bookId: string, formatParam: string | null) {
		const nextFormat = normalizeBookFormat(formatParam);
		const nextKey = `${bookId}:${nextFormat}`;
		if (loadedComicKey === nextKey && book) return;

		const loadToken = ++activeComicLoadToken;
		if (book && currentSessionId !== null && !sessionEnded) {
			await endSession();
		}

		loadedComicKey = nextKey;
		loading = true;
		book = null;
		numPages = 0;
		currentPage = 1;
		currentSpreadPages = null;
		savedProgress = null;
		pendingProgressPage = null;
		leftSidebarOpen = false;
		rightSidebarOpen = false;
			currentSessionId = null;
			sessionEnded = false;
			closeTasksStarted = false;
			requestedFormat = nextFormat;
			readerFiles = [];

			try {
				const [res, filesRes] = await Promise.all([
					fetch(`/api/books/${bookId}`),
					fetch(`/api/books/${bookId}/files`)
				]);
				if (loadToken !== activeComicLoadToken) return;
				if (res.ok) {
					book = await res.json();
					if (filesRes.ok) {
						readerFiles = await filesRes.json();
					}
					await fetchProgress();
				if (loadToken !== activeComicLoadToken) return;
				await startSession();
				const pagesRes = await fetch(`/api/cbx/${bookId}/pages${requestedFormat ? `?format=${encodeURIComponent(requestedFormat)}` : ''}`);
				if (loadToken !== activeComicLoadToken) return;
				if (pagesRes.ok) {
					const data = await pagesRes.json();
					numPages = data.pages;
					if (savedProgress && savedProgress.page > 0) {
						currentPage = Math.max(1, Math.min(numPages, savedProgress.page));
					} else if (savedProgress?.percent > 0 && numPages > 0) {
						currentPage = Math.max(1, Math.min(numPages, Math.ceil((savedProgress.percent / 100) * numPages)));
					}
					updateSpreadPages();
					await tick();
					if (isStripMode()) {
						isRestoringCbxProgress = true;
						scrollToStripPage(currentPage, 'auto');
						setTimeout(() => {
							isRestoringCbxProgress = false;
						}, 250);
					}
				}
			}
		} catch (e) {
			console.error('Failed to load book:', e);
		} finally {
			if (loadToken === activeComicLoadToken) {
				loading = false;
			}
		}
	}

	onMount(() => {
		const handleReaderVisibilityChange = () => {
			if (document.visibilityState === 'hidden') {
				resetInterruptedGestureState();
				void saveProgress(true);
			}
		};
		const unsubscribeSettings = readerSettings.subscribe(s => {
			const previousScrollMode = settings.scrollMode;
			const previousStripWidth = settings.stripMaxWidthPercent;
			showCurrentSection = s.showCurrentSection ?? true;
			settings = { ...s.cbx };
			if (!settings.autoHideControls) {
				showTopBar();
			}
			if (previousScrollMode !== settings.scrollMode || previousStripWidth !== settings.stripMaxWidthPercent) {
				void tick().then(() => {
					if (isStripMode()) {
						scrollToStripPage(currentPage, 'auto');
					}
				});
			}
		});

		handlePageExit = () => {
			resetInterruptedGestureState();
			if (closeTasksStarted) return;
			void saveProgress(true);
			void readerSettings.flushPendingSave(true);
			void endSession(true);
		};
		window.addEventListener('pagehide', handlePageExit);
		window.addEventListener('beforeunload', handlePageExit);
		window.addEventListener('blur', resetInterruptedGestureState);
		document.addEventListener('visibilitychange', handleReaderVisibilityChange);

		resetTopBarBehavior();

		return () => {
			unsubscribeSettings();
			window.removeEventListener('blur', resetInterruptedGestureState);
			document.removeEventListener('visibilitychange', handleReaderVisibilityChange);
		};
	});

	$effect(() => {
		const routeBookId = $page.params.bookID;
		const routeFormat = $page.url.searchParams.get('format');
		if (!routeBookId) return;

		queueMicrotask(() => {
			void loadComic(routeBookId, routeFormat);
		});
	});

	async function fetchProgress() {
		try {
			const res = await fetch(`/api/books/${book.id}/progress`);
			if (res.ok) {
				savedProgress = await res.json();
			}
		} catch (e) {
			console.error('Failed to fetch progress:', e);
		}
	}

	async function saveProgress(keepalive = false) {
		if (!book) return;
		const percent = numPages > 0 ? (currentPage / numPages) * 100 : 0;
		try {
			await fetch(`/api/books/${book.id}/progress`, {
				method: 'PUT',
				keepalive,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					page: currentPage,
					percent: percent,
					status: percent >= 100 ? 'finished' : 'reading'
				})
			});
		} catch (e) {
			console.error('Failed to save progress:', e);
		}
	}

	function updateSpreadPages() {
		if (settings.pageSpread === 'off') {
			currentSpreadPages = null;
		} else if (settings.pageSpread === 'even') {
			const page = currentPage % 2 === 0 ? currentPage : currentPage + 1;
			currentSpreadPages = [page - 1, page];
		} else if (settings.pageSpread === 'odd') {
			const page = currentPage % 2 === 1 ? currentPage : currentPage + 1;
			currentSpreadPages = [page, page + 1];
		}
	}

	async function startSession() {
		if (!book || !book.id) return;
		try {
			const res = await fetch(`/api/books/${book.id}/sessions`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ reader_type: 'comic' })
			});
			if (res.ok) {
				const data = await res.json();
				currentSessionId = data.id;
			}
		} catch (e) {
			console.error('Failed to start session:', e);
		}
	}

	async function endSession(keepalive = false) {
		if (sessionEnded || currentSessionId === null || !book || !book.id) return;
		sessionEnded = true;
		try {
			await fetch(`/api/books/${book.id}/sessions/${currentSessionId}`, {
				method: 'PUT',
				keepalive
			});
		} catch (e) {
			console.error('Failed to end session:', e);
		}
	}

	function clearTopBarHideTimeout() {
		if (topBarHideTimeout) {
			clearTimeout(topBarHideTimeout);
			topBarHideTimeout = null;
		}
	}

	function controlsNeedToStayVisible() {
		return (
			loading ||
			leftSidebarOpen ||
			rightSidebarOpen ||
			isDraggingProgress ||
			!settings.autoHideControls
		);
	}

	function showTopBar(scheduleHide = false) {
		topBarVisible = true;
		clearTopBarHideTimeout();
		if (scheduleHide) {
			scheduleTopBarAutoHide();
		}
	}

	function hideTopBar() {
		if (!controlsNeedToStayVisible()) {
			topBarVisible = false;
		}
	}

	function toggleTopBarFromCenterTap() {
		if (controlsNeedToStayVisible()) return;
		if (topBarVisible) {
			hideTopBar();
		} else {
			showTopBar(settings.scrollMode !== 'long-strip' && settings.scrollMode !== 'infinite');
		}
	}

	function scheduleTopBarAutoHide() {
		clearTopBarHideTimeout();
		if (controlsNeedToStayVisible() || settings.scrollMode === 'long-strip' || settings.scrollMode === 'infinite') {
			return;
		}
		topBarHideTimeout = setTimeout(() => {
			if (!controlsNeedToStayVisible()) {
				topBarVisible = false;
			}
		}, TOP_BAR_HIDE_DELAY_MS);
	}

	function resetTopBarBehavior() {
		showTopBar();
		if (settings.scrollMode !== 'long-strip' && settings.scrollMode !== 'infinite') {
			scheduleTopBarAutoHide();
		}
	}

	function preserveChromeAfterSeek() {
		preserveChromeAfterSeekUntil = performance.now() + SEEK_SCROLL_SUPPRESS_MS;
		showTopBar(false);
	}

	function isReaderCenterTap(e: PointerEvent, surface: HTMLElement) {
		const rect = surface.getBoundingClientRect();
		if (rect.width <= 0 || rect.height <= 0) return false;

		const xRatio = (e.clientX - rect.left) / rect.width;
		const yRatio = (e.clientY - rect.top) / rect.height;

		return xRatio > 0.28 && xRatio < 0.72 && yRatio > 0.12 && yRatio < 0.88;
	}

	function isStripMode() {
		return settings.scrollMode === 'long-strip' || settings.scrollMode === 'infinite';
	}

	function updateCurrentPageFromStrip() {
		if (!stripScrollEl || !isStripMode() || !numPages || isRestoringCbxProgress) return;
		const pages = Array.from(stripScrollEl.querySelectorAll<HTMLElement>('.strip-page[data-page]'));
		if (pages.length === 0) return;

		const containerRect = stripScrollEl.getBoundingClientRect();
		const anchorY = containerRect.top + Math.min(160, stripScrollEl.clientHeight / 3);
		let activePage = currentPage;
		let activePageDistance = Number.POSITIVE_INFINITY;

		for (const pageEl of pages) {
			const pageNum = Number(pageEl.dataset.page);
			if (!Number.isFinite(pageNum)) continue;

			const rect = pageEl.getBoundingClientRect();
			if (rect.top <= anchorY && rect.bottom >= anchorY) {
				activePage = pageNum;
				activePageDistance = 0;
				break;
			}

			const distance = Math.min(Math.abs(rect.top - anchorY), Math.abs(rect.bottom - anchorY));
			if (distance < activePageDistance) {
				activePageDistance = distance;
				activePage = pageNum;
			}
		}

		if (activePage !== currentPage) {
			currentPage = activePage;
			updateSpreadPages();
			queueStripProgressSave();
		}
	}

	function scrollToStripPage(pageNum: number, behavior: ScrollBehavior = 'smooth') {
		if (!stripScrollEl) return;
		const pageEl = stripScrollEl.querySelector<HTMLElement>(`.strip-page[data-page="${pageNum}"]`);
		if (!pageEl) return;

		stripScrollEl.scrollTo({ top: Math.max(0, pageEl.offsetTop - STRIP_PADDING_PX), behavior });
		updateCurrentPageFromStrip();
	}

	function getStripPageLoading(pageNum: number): 'eager' | 'lazy' {
		return pageNum <= currentPage + 2 ? 'eager' : 'lazy';
	}

	function handleStripPageLoad(pageNum: number) {
		if (!isStripMode() || pageNum > currentPage) return;
		if (stripRestoreScrollTimeout) {
			clearTimeout(stripRestoreScrollTimeout);
		}
		stripRestoreScrollTimeout = setTimeout(() => {
			stripRestoreScrollTimeout = null;
			scrollToStripPage(currentPage, 'auto');
		}, 40);
	}

	function queueStripProgressSave() {
		if (stripProgressSaveTimeout) {
			clearTimeout(stripProgressSaveTimeout);
		}
		stripProgressSaveTimeout = setTimeout(() => {
			stripProgressSaveTimeout = null;
			void saveProgress();
		}, 650);
	}

	function handleReaderPointerDown(e: PointerEvent) {
		if (!e.isPrimary) return;
		readerPointerStart = { id: e.pointerId, x: e.clientX, y: e.clientY };
		readerPointerMoved = false;
	}

	function resetReaderPointerTracking() {
		readerPointerStart = null;
		readerPointerMoved = false;
	}

	function isTouchLikePointer(e?: PointerEvent) {
		if (e?.pointerType === 'touch') return true;
		return typeof window !== 'undefined' && window.matchMedia('(hover: none), (pointer: coarse)').matches;
	}

	function handleReaderPointerMove(e: PointerEvent) {
		if (readerPointerStart) {
			const dx = e.clientX - readerPointerStart.x;
			const dy = e.clientY - readerPointerStart.y;
			if (Math.hypot(dx, dy) > 10) {
				readerPointerMoved = true;
			}
		}

		if (!settings.autoHideControls || controlsNeedToStayVisible() || isTouchLikePointer(e)) return;
		if (e.clientY <= TOP_BAR_REVEAL_EDGE_PX) {
			showTopBar(settings.scrollMode !== 'long-strip' && settings.scrollMode !== 'infinite');
		}
	}

	function handleReaderPointerUp(e: PointerEvent) {
		const pointerStart = readerPointerStart;
		const pointerMoved = readerPointerMoved;
		resetReaderPointerTracking();

		if (controlsNeedToStayVisible()) return;
		const target = e.target as Element | null;
		if (target?.closest('input, textarea, select, [contenteditable="true"], .left-sidebar, .right-sidebar, .progress-bar, .reader-progress, .floating-nav')) {
			return;
		}
		if (pointerStart && pointerStart.id !== e.pointerId) return;
		if (pointerMoved) return;
		if (isReaderCenterTap(e, e.currentTarget as HTMLElement)) {
			toggleTopBarFromCenterTap();
		}
	}

	function handleReaderPointerCancel() {
		resetReaderPointerTracking();
	}

	onDestroy(() => {
		if (handlePageExit) {
			window.removeEventListener('pagehide', handlePageExit);
			window.removeEventListener('beforeunload', handlePageExit);
		}
		clearTopBarHideTimeout();
		if (stripProgressSaveTimeout) {
			clearTimeout(stripProgressSaveTimeout);
			stripProgressSaveTimeout = null;
		}
		if (stripRestoreScrollTimeout) {
			clearTimeout(stripRestoreScrollTimeout);
			stripRestoreScrollTimeout = null;
		}
		if (!closeTasksStarted) {
			void endSession(true);
		}
	});

	function updateSetting(key: string, value: any) {
		settings = { ...settings, [key]: value };
		readerSettings.updateCbx({ [key]: value });

		if (key === 'pageSpread') {
			updateSpreadPages();
		}
		if (key === 'scrollMode' || key === 'stripMaxWidthPercent') {
			void tick().then(() => {
				if (isStripMode()) {
					scrollToStripPage(currentPage, 'auto');
				}
			});
		}

		resetTopBarBehavior();
	}

	function prevPage() {
		resetTopBarBehavior();
		if (settings.mangaMode || settings.readingDirection === 'rtl') {
			if (currentPage < numPages) {
				currentPage++;
				updateSpreadPages();
			}
		} else {
			if (currentPage > 1) {
				currentPage--;
				updateSpreadPages();
			}
		}
		if (isStripMode()) {
			scrollToStripPage(currentPage);
		}
		saveProgress();
	}

	function nextPage() {
		resetTopBarBehavior();
		if (settings.mangaMode || settings.readingDirection === 'rtl') {
			if (currentPage > 1) {
				currentPage--;
				updateSpreadPages();
			}
		} else {
			if (currentPage < numPages) {
				currentPage++;
				updateSpreadPages();
			}
		}
		if (isStripMode()) {
			scrollToStripPage(currentPage);
		}
		saveProgress();
	}

	function goToPage(pageNum: number) {
		if (pageNum >= 1 && pageNum <= numPages) {
			resetTopBarBehavior();
			currentPage = pageNum;
			updateSpreadPages();
			if (isStripMode()) {
				scrollToStripPage(currentPage);
			}
			saveProgress();
		}
	}

	function toggleLeftSidebar() {
		if (rightSidebarOpen) {
			rightSidebarOpen = false;
		}
		leftSidebarOpen = !leftSidebarOpen;
		resetTopBarBehavior();
	}

	function toggleRightSidebar() {
		if (leftSidebarOpen) {
			leftSidebarOpen = false;
		}
		rightSidebarOpen = !rightSidebarOpen;
		resetTopBarBehavior();
	}

	function getProgressPageFromClientX(clientX: number) {
		if (!progressBarEl || numPages <= 0) return null;
		const rect = progressBarEl.getBoundingClientRect();
		if (rect.width <= 0) return null;
		const x = clientX - rect.left;
		const percentage = Math.max(0, Math.min(1, x / rect.width));
		const newPage = Math.max(1, Math.round(percentage * numPages));
		return newPage >= 1 && newPage <= numPages ? newPage : null;
	}

	function updateProgressDrag(clientX: number) {
		const newPage = getProgressPageFromClientX(clientX);
		if (newPage === null) return;
		pendingProgressPage = newPage;
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
			goToPage(targetPage);
		}
	}

	function cancelProgressPointer(e?: PointerEvent) {
		if (e && activeProgressPointerId !== null) {
			(e.currentTarget as HTMLElement).releasePointerCapture?.(activeProgressPointerId);
		} else if (progressBarEl && activeProgressPointerId !== null) {
			try {
				progressBarEl.releasePointerCapture?.(activeProgressPointerId);
			} catch {
				// The OS may have consumed the gesture and released capture already.
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
		preserveChromeAfterSeek();
	}

	function handleProgressPointerCancel(e: PointerEvent) {
		if (activeProgressPointerId !== null && e.pointerId !== activeProgressPointerId) return;
		cancelProgressPointer(e);
	}

	function resetInterruptedGestureState() {
		cancelProgressPointer();
		resetReaderPointerTracking();
		preserveChromeAfterSeekUntil = 0;
	}

	function handleProgressBarKeydown(e: KeyboardEvent) {
		if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
			e.preventDefault();
			prevPage();
			preserveChromeAfterSeek();
		} else if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
			e.preventDefault();
			nextPage();
			preserveChromeAfterSeek();
		}
	}

	function startCloseBackgroundTasks() {
		if (closeTasksStarted) return;
		closeTasksStarted = true;
		void saveProgress(true);
		void endSession(true);
	}

	function getSafeReturnPath(value: string | null) {
		if (!value) return null;
		if (!value.startsWith('/') || value.startsWith('//')) return null;
		if (value.startsWith('/login') || value.includes('/reader/')) return null;
		return value;
	}

	function getReaderReturnUrl() {
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
				// Ignore invalid referrers and fall back to the book route.
			}
		}

		return book ? `/book/${book.id}` : '/book';
	}

	function closeReader(e?: Event) {
		e?.preventDefault();
		const targetUrl = getReaderReturnUrl();
		startCloseBackgroundTasks();
		void goto(targetUrl, { replaceState: true });
	}

	function toggleFullscreen() {
		toggleReaderFullscreen(settings.useStandardFullscreen).catch(console.error);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (rightSidebarOpen) {
				rightSidebarOpen = false;
			} else if (leftSidebarOpen) {
				leftSidebarOpen = false;
			} else {
				void closeReader();
			}
		} else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
			e.preventDefault();
			resetTopBarBehavior();
			prevPage();
		} else if (e.key === 'ArrowRight' || e.key === 'ArrowDown' || e.key === ' ') {
			e.preventDefault();
			resetTopBarBehavior();
			nextPage();
		} else if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
			e.preventDefault();
			resetTopBarBehavior();
			toggleLeftSidebar();
		}
	}

	function shouldIgnoreWheelNavigation(target: EventTarget | null) {
		if (!(target instanceof Element)) return false;
		return !!target.closest('input, textarea, select, [contenteditable="true"], .left-sidebar, .right-sidebar');
	}

	function handleWheelNavigation(e: WheelEvent) {
		if (
			settings.scrollMode === 'long-strip' ||
			settings.scrollMode === 'infinite' ||
			shouldIgnoreWheelNavigation(e.target)
		) {
			return;
		}

		const dominantDelta = Math.abs(e.deltaY) >= Math.abs(e.deltaX) ? e.deltaY : e.deltaX;
		if (Math.abs(dominantDelta) < 12) return;

		const now = performance.now();
		if (now - lastWheelNavigationAt < 220) {
			e.preventDefault();
			return;
		}

		e.preventDefault();
		lastWheelNavigationAt = now;
		resetTopBarBehavior();

		if (dominantDelta > 0) {
			nextPage();
		} else {
			prevPage();
		}
	}

	function handleLongStripScroll(e: Event) {
		const container = e.currentTarget as HTMLElement | null;
		if (!container) return;
		const scrollTop = container.scrollTop;
		updateCurrentPageFromStrip();
		if (performance.now() < preserveChromeAfterSeekUntil) {
			showTopBar(false);
			lastLongStripScrollTop = scrollTop;
			return;
		}

		if (!settings.autoHideControls || controlsNeedToStayVisible()) {
			lastLongStripScrollTop = scrollTop;
			return;
		}

		const delta = scrollTop - lastLongStripScrollTop;
		const touchLike = typeof window !== 'undefined' && window.matchMedia('(hover: none), (pointer: coarse)').matches;
		if (scrollTop <= 2 && !touchLike) {
			showTopBar(false);
		} else if (delta > TOP_BAR_SCROLL_DELTA) {
			hideTopBar();
		} else if (delta < -TOP_BAR_SCROLL_DELTA && !touchLike) {
			showTopBar(false);
		}
		lastLongStripScrollTop = scrollTop;
	}

	function getPageUrl(pageNum: number): string {
		return `/api/cbx/${book.id}/page/${pageNum}${requestedFormat ? `?format=${encodeURIComponent(requestedFormat)}` : ''}`;
	}

	function getPageImageClass() {
		return `page-image fit-${settings.fitMode}`;
	}

	function resetToDefaults() {
		readerSettings.resetToDefaults('cbx');
	}

	$effect(() => {
		currentPage;
		updateSpreadPages();
	});

	onMount(() => {
		const handleResize = () => {
			if (isStripMode()) {
				scrollToStripPage(currentPage, 'auto');
			}
		};
		window.addEventListener('keydown', handleKeydown);
		window.addEventListener('wheel', handleWheelNavigation, { passive: false });
		window.addEventListener('resize', handleResize);
		return () => {
			window.removeEventListener('keydown', handleKeydown);
			window.removeEventListener('wheel', handleWheelNavigation);
			window.removeEventListener('resize', handleResize);
		};
	});
</script>

<svelte:head>
	<title>{readerTitle === 'Loading...' ? 'Reading' : readerTitle} - Cryptorum</title>
</svelte:head>

<div
	class="cbx-reader"
	style="background-color: {settings.backgroundColor};"
	role="presentation"
	onpointerdown={handleReaderPointerDown}
	onpointermove={handleReaderPointerMove}
	onpointerup={handleReaderPointerUp}
	onpointercancel={handleReaderPointerCancel}
>
	<!-- Top Navigation Bar -->
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
				onclick={toggleLeftSidebar}
				class="nav-btn"
				class:active={leftSidebarOpen}
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
				<span class="book-title">{readerTitle}</span>
			{#if showCurrentSection}
				<span class="chapter-title">Page {currentPage} / {Math.max(numPages, currentPage)}</span>
			{/if}
		</div>

		<div class="nav-right">
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
			ariaValueNow={numPages > 0 ? Math.round((currentPage / numPages) * 100) : 0}
		/>
	</header>

	<ReaderProgressTrack
		bind:element={progressBarEl}
		progress={progress}
		visible={topBarVisible}
		variant="bottom"
		metaAlign="center"
		interactive={true}
		showThumb={true}
		label={`Page ${currentPage} / ${numPages} • ${Math.max(0, Math.min(100, Math.round(progress)))}%`}
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
		onkeydown={handleProgressBarKeydown}
	/>

	<!-- Main Content Area -->
	<div class="main-content">
		<!-- Left Sidebar - Page Thumbnails -->
		<aside class="left-sidebar" class:open={leftSidebarOpen}>
			<div class="sidebar-header">
				<span>Pages</span>
			</div>
			<div class="sidebar-content">
				<div class="thumbnails-panel">
					{#each Array(numPages) as _, i}
						<button
							onclick={() => { goToPage(i + 1); leftSidebarOpen = false; }}
							class="thumbnail-item"
							class:active={currentPage === i + 1}
						>
							<img src={getPageUrl(i + 1)} alt="Page {i + 1}" class="thumbnail-image" loading="lazy" decoding="async" />
							<span class="thumbnail-number">{i + 1}</span>
						</button>
					{/each}
				</div>
			</div>
		</aside>

		<!-- CBX Container -->
		<div
			class="cbx-container"
			onclick={() => { if (leftSidebarOpen || rightSidebarOpen) { leftSidebarOpen = false; rightSidebarOpen = false; } }}
			role="button"
			tabindex="0"
			aria-label="Close sidebars"
			onkeydown={(e) => { if (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') { e.preventDefault(); leftSidebarOpen = false; rightSidebarOpen = false; } }}
		>
			{#if loading}
				<div class="loading-state" aria-live="polite">
					<div class="loading-spinner"></div>
					<p>Loading comic...</p>
				</div>
			{:else if numPages > 0}
				{#if settings.scrollMode === 'long-strip' || settings.scrollMode === 'infinite'}
					<div class="long-strip" bind:this={stripScrollEl} onscroll={handleLongStripScroll}>
						<div class="strip-content">
							{#each Array(numPages) as _, i (i)}
								{@const pageNum = i + 1}
								<img
									src={getPageUrl(pageNum)}
									alt="Page {pageNum}"
									class="strip-page"
									data-page={pageNum}
									loading={getStripPageLoading(pageNum)}
									decoding="async"
									onload={() => handleStripPageLoad(pageNum)}
									style="width: {settings.stripMaxWidthPercent}%; filter: saturate({settings.saturation}%) brightness({settings.vibrance / 100});"
								/>
							{/each}
						</div>
					</div>
				{:else}
					<div class="page-viewer">
						{#if currentSpreadPages && (settings.pageSpread === 'even' || settings.pageSpread === 'odd')}
							<img
								src={getPageUrl(currentSpreadPages[0])}
								alt="Page {currentSpreadPages[0]}"
								class={getPageImageClass()}
								style="filter: saturate({settings.saturation}%) brightness({settings.vibrance / 100});"
							/>
							{#if currentSpreadPages[1] <= numPages}
								<img
									src={getPageUrl(currentSpreadPages[1])}
									alt="Page {currentSpreadPages[1]}"
									class={getPageImageClass()}
									style="filter: saturate({settings.saturation}%) brightness({settings.vibrance / 100});"
								/>
							{/if}
						{:else}
							<img
								src={getPageUrl(currentPage)}
								alt="Page {currentPage}"
								class={getPageImageClass()}
								style="filter: saturate({settings.saturation}%) brightness({settings.vibrance / 100});"
							/>
						{/if}
					</div>

					<!-- Floating Nav Buttons -->
					<button
						class="floating-nav floating-prev"
						onclick={prevPage}
						aria-label="Previous page"
						disabled={settings.mangaMode || settings.readingDirection === 'rtl' ? currentPage >= numPages : currentPage <= 1}
					>
						<svg class="icon-lg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polyline points="15 18 9 12 15 6"></polyline>
						</svg>
					</button>
					<button
						class="floating-nav floating-next"
						onclick={nextPage}
						aria-label="Next page"
						disabled={settings.mangaMode || settings.readingDirection === 'rtl' ? currentPage <= 1 : currentPage >= numPages}
					>
						<svg class="icon-lg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polyline points="9 18 15 12 9 6"></polyline>
						</svg>
					</button>
				{/if}
			{:else}
				<div class="error-message">
					<p>No pages available</p>
					<a href={getReaderReturnUrl()} class="btn">Return to Library</a>
				</div>
			{/if}
		</div>

		<!-- Right Sidebar - Settings -->
		<aside class="right-sidebar" class:open={rightSidebarOpen}>
			<div class="settings-tabs">
				<button
					class="settings-tab"
					class:active={activeSettingsTab === 'display'}
					onclick={() => activeSettingsTab = 'display'}
				>
					Display
				</button>
				<button
					class="settings-tab"
					class:active={activeSettingsTab === 'comic'}
					onclick={() => activeSettingsTab = 'comic'}
				>
					Comic
				</button>
			</div>

			<div class="settings-content">
				{#if activeSettingsTab === 'display'}
					<div class="settings-section">
						<div class="settings-label">Fit Mode</div>
						<select
							value={settings.fitMode}
							onchange={(e) => updateSetting('fitMode', e.currentTarget.value)}
							class="settings-select"
						>
							{#each cbxFitModes as mode}
								<option value={mode.id}>{mode.name}</option>
							{/each}
						</select>
					</div>

					<div class="settings-section">
						<div class="settings-label">Scroll Mode</div>
						<select
							value={settings.scrollMode}
							onchange={(e) => updateSetting('scrollMode', e.currentTarget.value)}
							class="settings-select"
						>
							{#each cbxScrollModes as mode}
								<option value={mode.id}>{mode.name}</option>
							{/each}
						</select>
					</div>

					{#if settings.scrollMode === 'long-strip' || settings.scrollMode === 'infinite'}
						<div class="settings-section">
							<div class="settings-label">Strip Width: {settings.stripMaxWidthPercent}%</div>
							<input
								type="range"
								min="50"
								max="150"
								value={settings.stripMaxWidthPercent}
								oninput={(e) => updateSetting('stripMaxWidthPercent', parseInt(e.currentTarget.value))}
								class="range-input-full"
							/>
						</div>
					{/if}

					<div class="settings-section">
						<div class="settings-label">Background</div>
						<div class="color-picker">
							<input
								type="color"
								value={settings.backgroundColor}
								oninput={(e) => updateSetting('backgroundColor', e.currentTarget.value)}
								class="color-input"
							/>
							<span class="color-value">{settings.backgroundColor}</span>
						</div>
					</div>

					<div class="settings-section">
						<label class="toggle-option">
							<span>Use Standard Fullscreen</span>
							<input
								type="checkbox"
								checked={settings.useStandardFullscreen}
								onchange={(e) => updateSetting('useStandardFullscreen', e.currentTarget.checked)}
							/>
						</label>
					</div>

				{:else if activeSettingsTab === 'comic'}
					<div class="settings-section">
						<div class="settings-label">Reading Direction</div>
						<div class="button-group">
							<button
								onclick={() => updateSetting('readingDirection', 'ltr')}
								class="option-btn"
								class:active={settings.readingDirection === 'ltr'}
							>
								LTR
							</button>
							<button
								onclick={() => updateSetting('readingDirection', 'rtl')}
								class="option-btn"
								class:active={settings.readingDirection === 'rtl'}
							>
								RTL
							</button>
						</div>
					</div>

					<div class="settings-section">
						<div class="settings-label">Page Spread</div>
						<div class="button-group">
							<button
								onclick={() => updateSetting('pageSpread', 'off')}
								class="option-btn"
								class:active={settings.pageSpread === 'off'}
							>
								Off
							</button>
							<button
								onclick={() => updateSetting('pageSpread', 'even')}
								class="option-btn"
								class:active={settings.pageSpread === 'even'}
							>
								Even
							</button>
							<button
								onclick={() => updateSetting('pageSpread', 'odd')}
								class="option-btn"
								class:active={settings.pageSpread === 'odd'}
							>
								Odd
							</button>
						</div>
					</div>

					<div class="settings-section">
						<label class="toggle-option">
							<span>Manga Mode (RTL)</span>
							<input
								type="checkbox"
								checked={settings.mangaMode}
								onchange={(e) => updateSetting('mangaMode', e.currentTarget.checked)}
							/>
						</label>
					</div>

					<div class="settings-section">
						<div class="settings-label">Vibrance: {settings.vibrance}%</div>
						<input
							type="range"
							min="0"
							max="200"
							value={settings.vibrance}
							oninput={(e) => updateSetting('vibrance', parseInt(e.currentTarget.value))}
							class="range-input-full"
						/>
					</div>

					<div class="settings-section">
						<div class="settings-label">Saturation: {settings.saturation}%</div>
						<input
							type="range"
							min="0"
							max="200"
							value={settings.saturation}
							oninput={(e) => updateSetting('saturation', parseInt(e.currentTarget.value))}
							class="range-input-full"
						/>
					</div>
				{/if}
			</div>
		</aside>
	</div>
</div>

<style>
	.cbx-reader {
		--reader-top-bar-height: 64px;
		position: fixed;
		inset: 0;
		z-index: 9999;
		display: flex;
		flex-direction: column;
		font-family: system-ui, -apple-system, sans-serif;
		overflow: hidden;
		min-width: 0;
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
		flex-shrink: 0;
		z-index: 120;
		transform: translateY(0);
		transition: transform 0.22s ease, opacity 0.22s ease;
		will-change: transform, opacity;
	}

	.top-nav-hidden {
		transform: translateY(-100%);
		opacity: 0;
		pointer-events: none;
	}

	.nav-left, .nav-center, .nav-right {
		display: flex;
		align-items: center;
		gap: 4px;
	}

	.nav-left { min-width: 0; }
	.nav-center {
		flex-direction: column;
		justify-content: center;
		min-width: 0;
		text-align: center;
	}
	.nav-right {
		justify-content: flex-end;
		min-width: 0;
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

	.nav-btn:hover { background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85)); }
	.nav-btn:disabled { opacity: 0.3; cursor: not-allowed; }
	.nav-btn.active { background: var(--color-primary-500, #22c55e); color: white; }

	.book-title {
		color: var(--color-surface-text, #e2e8f0);
		font-size: 14px;
		font-weight: 500;
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.chapter-title {
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 11px;
		font-weight: 500;
	}

	.icon { width: 20px; height: 20px; }
	.icon-lg { width: 24px; height: 24px; }

	.main-content {
		flex: 1;
		display: flex;
		overflow: hidden;
		position: relative;
	}

	.left-sidebar {
		position: absolute;
		left: 0;
		top: var(--reader-top-bar-height);
		bottom: 0;
		width: 300px;
		display: flex;
		flex-direction: column;
		background: var(--color-surface-base, #0f172a);
		border-right: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		transform: translateX(-100%);
		transition: transform 0.25s ease-in-out;
		z-index: 50;
	}

	.left-sidebar.open { transform: translateX(0); }

	.sidebar-header {
		padding: 12px 16px;
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		font-size: 12px;
		font-weight: 600;
		color: var(--color-surface-text-muted, #94a3b8);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.sidebar-content { flex: 1; overflow-y: auto; }

	.thumbnails-panel {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 8px;
		padding: 8px;
	}

	.thumbnail-item {
		aspect-ratio: 3/4;
		position: relative;
		display: block;
		overflow: hidden;
		border: 2px solid transparent;
		border-radius: 4px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 12px;
		cursor: pointer;
		transition: all 0.15s;
	}

	.thumbnail-item:hover { border-color: var(--color-surface-border, rgba(55, 65, 81, 0.6)); }
	.thumbnail-item.active { border-color: var(--color-primary-500, #22c55e); }

	.thumbnail-image {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
	}

	.thumbnail-number {
		position: absolute;
		left: 6px;
		bottom: 6px;
		padding: 2px 6px;
		border-radius: 999px;
		background: rgba(15, 23, 42, 0.82);
		color: #f8fafc;
		font-size: 11px;
		font-weight: 600;
		line-height: 1.2;
	}

	.cbx-container {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		overflow: auto;
		position: relative;
		min-width: 0;
	}

	.page-viewer {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 16px;
		width: 100%;
		height: 100%;
		overflow: auto;
	}

	.page-image {
		object-fit: contain;
		flex: 0 0 auto;
	}

	.page-image.fit-fit-page,
	.page-image.fit-automatic {
		max-width: 100%;
		max-height: 100%;
		width: auto;
		height: auto;
	}

	.page-image.fit-fit-width {
		width: 100%;
		max-width: 100%;
		height: auto;
		max-height: none;
	}

	.page-image.fit-fit-height {
		height: 100%;
		max-height: 100%;
		width: auto;
		max-width: none;
	}

	.page-image.fit-actual-size {
		width: auto;
		height: auto;
		max-width: none;
		max-height: none;
	}

	.long-strip {
		width: 100%;
		height: 100%;
		overflow-y: auto;
		overflow-x: auto;
		padding: 16px 8px;
		box-sizing: border-box;
	}

	.strip-content {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		min-width: 100%;
		margin: 0 auto;
	}

	.strip-page {
		display: block;
		height: auto;
		max-width: none;
		object-fit: contain;
		flex: 0 0 auto;
		box-shadow: 0 1px 4px rgba(0, 0, 0, 0.18);
	}

	.floating-nav {
		position: absolute;
		top: 50%;
		transform: translateY(-50%);
		width: 48px;
		height: 80px;
		display: flex;
		align-items: center;
		justify-content: center;
		border: none;
		border-radius: 8px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text, #e2e8f0);
		cursor: pointer;
		transition: all 0.15s;
		z-index: 10;
	}

	.floating-nav:hover:not(:disabled) { background: var(--color-primary-500, #22c55e); }
	.floating-nav:disabled { opacity: 0.2; cursor: not-allowed; }

	.floating-prev { left: 16px; }
	.floating-next { right: 16px; }

	.loading-spinner {
		width: 48px;
		height: 48px;
		border: 3px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-top-color: var(--color-primary-500, #22c55e);
		border-radius: 50%;
		animation: spin 1s linear infinite;
	}

	.loading-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 12px;
		text-align: center;
		color: var(--color-surface-text-muted, #94a3b8);
	}

	.loading-state p {
		margin: 0;
		font-size: 14px;
	}

	@keyframes spin { to { transform: rotate(360deg); } }

	.error-message { text-align: center; }
	.error-message p { color: #ef4444; margin-bottom: 16px; }

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
		width: 400px;
		display: flex;
		flex-direction: column;
		background: var(--color-surface-base, #0f172a);
		border-left: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		transform: translateX(100%);
		transition: transform 0.25s ease-in-out;
		z-index: 50;
	}

	.right-sidebar.open { transform: translateX(0); }

	.settings-tabs {
		display: flex;
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
	}

	.settings-tab {
		position: relative;
		flex: 1;
		padding: 12px 8px;
		border: none;
		background: transparent;
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 12px;
		font-weight: 500;
		cursor: pointer;
		transition: color 0.15s ease-out;
	}

	.settings-tab::after {
		content: '';
		position: absolute;
		left: 12px;
		right: 12px;
		bottom: 0;
		height: 2px;
		border-radius: 999px;
		background: var(--color-primary-500, #22c55e);
		opacity: 0;
		transform: scaleX(0.35);
		transition: opacity 140ms ease-out, transform 160ms ease-out;
	}

	.settings-tab:hover { color: var(--color-surface-text, #e2e8f0); }
	.settings-tab.active {
		color: var(--color-primary-500, #22c55e);
	}

	.settings-tab.active::after {
		opacity: 1;
		transform: scaleX(1);
	}

	.settings-content {
		flex: 1;
		overflow-y: auto;
		padding: 16px;
	}

	.settings-section { margin-bottom: 20px; }

	.settings-label {
		display: block;
		font-size: 12px;
		color: var(--color-surface-text-muted, #94a3b8);
		margin-bottom: 8px;
	}

	.settings-select {
		width: 100%;
		padding: 10px 12px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 6px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text, #e2e8f0);
		font-size: 13px;
		cursor: pointer;
	}

	.settings-select:focus { outline: none; border-color: var(--color-primary-500, #22c55e); }

	.range-input-full {
		width: 100%;
		height: 4px;
		appearance: none;
		background: var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 2px;
		cursor: pointer;
	}

	.range-input-full::-webkit-slider-thumb {
		appearance: none;
		width: 16px;
		height: 16px;
		background: var(--color-primary-500, #22c55e);
		border-radius: 50%;
		cursor: pointer;
	}

	.color-picker {
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.color-input {
		width: 40px;
		height: 40px;
		border: none;
		border-radius: 6px;
		cursor: pointer;
	}

	.color-value {
		font-family: monospace;
		font-size: 13px;
		color: var(--color-surface-text, #e2e8f0);
	}

	.button-group { display: flex; gap: 4px; }

	.option-btn {
		flex: 1;
		padding: 8px 12px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 6px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 12px;
		cursor: pointer;
		transition: all 0.15s;
	}

	.option-btn:hover { border-color: var(--color-primary-500, #22c55e); }
	.option-btn.active {
		border-color: var(--color-primary-500, #22c55e);
		background: rgba(34, 197, 94, 0.1);
	}

	.toggle-option {
		display: flex;
		align-items: center;
		justify-content: space-between;
		font-size: 13px;
		color: var(--color-surface-text, #e2e8f0);
		cursor: pointer;
	}

	.toggle-option input[type="checkbox"] {
		width: 18px;
		height: 18px;
		accent-color: var(--color-primary-500, #22c55e);
	}

	@media (max-width: 768px) {
		.cbx-reader {
			--reader-top-bar-height: 72px;
		}

		.top-nav {
			padding: 0 10px;
			column-gap: 8px;
		}

		.nav-left,
		.nav-right {
			gap: 6px;
			min-width: 0;
		}

		.nav-center {
			display: none;
		}

		.nav-btn {
			width: 44px;
			height: 44px;
		}

		.mobile-secondary {
			display: none;
		}

		.icon,
		.icon-lg {
			width: 22px;
			height: 22px;
		}

		.left-sidebar,
		.right-sidebar {
			width: 100vw;
			max-width: 100vw;
		}

		.left-sidebar {
			border-right: none;
		}

		.right-sidebar {
			border-left: none;
		}

		.cbx-container {
			padding: 0 8px;
		}

		.page-viewer {
			flex-direction: column;
			gap: 12px;
			width: 100%;
		}

		.page-image.fit-fit-page,
		.page-image.fit-automatic {
			max-height: calc(100vh - 120px);
		}

		.page-image.fit-fit-width {
			width: 100%;
			max-width: 100%;
			max-height: none;
		}

		.page-image.fit-fit-height {
			height: calc(100vh - 120px);
			max-height: calc(100vh - 120px);
		}

		.long-strip {
			padding: 0 8px;
		}

		.floating-nav {
			width: 36px;
			height: 56px;
			border-radius: 999px;
		}

		.floating-prev {
			left: 8px;
		}

		.floating-next {
			right: 8px;
		}

		.loading-spinner {
			width: 40px;
			height: 40px;
		}
	}
</style>

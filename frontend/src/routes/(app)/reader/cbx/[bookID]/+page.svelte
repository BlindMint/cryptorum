<script lang="ts">
	import { requireReadableFile, fileResponseError } from '$lib/utils/file-availability';
	import { resolveComicSettings } from '$lib/utils/comic-settings';
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { browser } from '$app/environment';
	import { readerSettings, cbxFitModes, cbxScrollModes, type CbxReaderSetting } from '$lib/stores/readerSettings';
	import ChoiceChips from '$lib/components/ChoiceChips.svelte';
	import ReaderSizingControls from '$lib/components/ReaderSizingControls.svelte';
	import ReaderProgressTrack from '$lib/components/ReaderProgressTrack.svelte';
	import { normalizeBookFormat } from '$lib/utils/book-formats';
	import { getReaderDisplayTitle } from '$lib/utils/reader-title';
	import { toggleReaderFullscreen } from '$lib/utils/fullscreen';
	import { isBottomSystemGestureStart } from '$lib/utils/system-gesture-guard';
	import {
		ReadingProgressController,
		readingPositionAsLegacy,
		selectReaderFile,
		withReaderFile,
		type ReaderFile
	} from '$lib/services/reading-progress';

	const comicBookId = $derived($page.params.bookID ?? '');
	let readerError = $state('');
	let book = $state<any>(null);
	let readerFiles = $state<any[]>([]);
	let loading = $state(true);
	let numPages = $state(0);
	let currentPage = $state(1);
	let pageAspectRatios = $state<Record<number, number>>({});
	let currentSpreadPages = $state<[number, number] | null>(null);
	let savedProgress = $state<any>(null);

	let settings = $state<CbxReaderSetting>({
		pageSpread: 'off',
		pageLayout: 'single',
		openingFitMode: 'fit-page',
		fitMode: 'fit-page',
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

	let sessionFitMode = $state<CbxReaderSetting['fitMode']>('fit-page');
	let zoom = $state(1);
	let viewerEl = $state<HTMLDivElement>();
	const pointers = new Map<number, { x: number; y: number }>();
	let pinchDistance = 0;
	let gestureWasPinch = false;
	function changeZoom(value: number) {
		zoom = Math.max(0.25, Math.min(5, value));
	}
	function setFitMode(value: CbxReaderSetting['fitMode']) {
		const anchor = stripScrollEl?.querySelector<HTMLElement>(`.strip-page[data-page="${currentPage}"]`);
		const horizontal = settings.scrollMode === 'long-strip';
		const rtl = settings.readingDirection === 'rtl';
		const rect = anchor?.getBoundingClientRect();
		const viewport = stripScrollEl?.getBoundingClientRect();
		const fraction = rect && viewport ? (horizontal ? (rtl ? rect.right - viewport.right : viewport.left - rect.left) / Math.max(1, rect.width) : (viewport.top - rect.top) / Math.max(1, rect.height)) : 0;
		sessionFitMode = value;
		zoom = 1;
		if (anchor && stripScrollEl) {
			void tick().then(() => {
				if (!stripScrollEl) return;
				scrollToStripPage(currentPage, 'auto');
				if (horizontal) stripScrollEl.scrollLeft += fraction * anchor.offsetWidth * (rtl ? -1 : 1);
				else stripScrollEl.scrollTop += fraction * anchor.offsetHeight;
			});
		}
		readerSettings.updateComic(comicBookId, { fitMode: value, ...(value === 'fit-width' ? { stripMaxWidthPercent: 100 } : {}) });
	}
	let leftSidebarOpen = $state(false);
	let rightSidebarOpen = $state(false);
	let isDraggingProgress = $state(false);
	let pendingProgressPage = $state<number | null>(null);
	let progressPreviewPage = $state<number | null>(null);
	let progressBarEl = $state<HTMLElement | null>(null);
	let activeProgressPointerId: number | null = null;
	const progressPreviewProgress = $derived(
		progressPreviewPage !== null && numPages > 1 ? ((progressPreviewPage - 1) / (numPages - 1)) * 100 : 0
	);
	const progressPreviewLabel = $derived(
		progressPreviewPage !== null && numPages > 0
			? `Page ${progressPreviewPage} / ${numPages} • ${Math.round(numPages > 1 ? ((progressPreviewPage - 1) / (numPages - 1)) * 100 : 0)}%`
			: ''
	);
	let lastWheelNavigationAt = 0;
	let topBarHideTimeout: ReturnType<typeof setTimeout> | null = null;
	let lastLongStripScrollTop = 0;
	let preserveChromeAfterSeekUntil = 0;
	let sessionEnded = false;
	let handlePageExit: (() => void) | null = null;
	let requestedFormat = $state('');
	let activeFile = $state<ReaderFile | null>(null);
	let progressController: ReadingProgressController | null = null;
	let bottomBarHeight = $state(86);
	$effect(() => {
		const element = progressBarEl;
		if (!element) return;
		const observer = new ResizeObserver(() => bottomBarHeight = element.getBoundingClientRect().height);
		observer.observe(element);
		return () => observer.disconnect();
	});

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

	const hasComicOverride = $derived(!!$readerSettings.comicOverrides?.[comicBookId]);
	$effect(() => {
		const resolved = resolveComicSettings($readerSettings.cbx, $readerSettings.comicOverrides?.[comicBookId]);
		settings = resolved;
		sessionFitMode = resolved.fitMode;
	});

	const TOP_BAR_HIDE_DELAY_MS = 2800;
	const TOP_BAR_SCROLL_DELTA = 12;
	const SEEK_SCROLL_SUPPRESS_MS = 1400;
	const TOP_BAR_REVEAL_EDGE_PX = 72;
	const STRIP_PADDING_PX = 16;

	const progress = $derived(numPages > 1 ? ((currentPage - 1) / (numPages - 1)) * 100 : 0);
	const readerTitle = $derived(getReaderDisplayTitle(book, readerFiles, loading, requestedFormat));

	async function loadComic(bookId: string, formatParam: string | null) {
		const nextFormat = normalizeBookFormat(formatParam);
		const nextKey = `${bookId}:${nextFormat}`;
		if (loadedComicKey === nextKey && book) return;

		const loadToken = ++activeComicLoadToken;
		sessionFitMode = settings.fitMode;
		if (book && progressController && !sessionEnded) {
			await endSession();
		}
		progressController?.destroy();
		progressController = null;

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
			sessionEnded = false;
			closeTasksStarted = false;
			readerError = '';
			requestedFormat = nextFormat;
			readerFiles = [];
			pageAspectRatios = {};

			try {
				const [res, filesRes] = await Promise.all([
					fetch(`/api/books/${bookId}`),
					fetch(`/api/books/${bookId}/files`)
				]);
				if (loadToken !== activeComicLoadToken) return;
				if (!res.ok) throw new Error(await fileResponseError(res));
				if (res.ok) {
					book = await res.json();
					if (!filesRes.ok) throw new Error(await fileResponseError(filesRes));
					if (filesRes.ok) {
						readerFiles = await filesRes.json();
					}
					activeFile = selectReaderFile(readerFiles, requestedFormat, ['cbz', 'cbr', 'cb7', 'cbt']);
					requireReadableFile(activeFile);
				if (loadToken !== activeComicLoadToken) return;
				await startSession();
				const pagesRes = await fetch(withReaderFile(`/api/cbx/${bookId}/pages${requestedFormat ? `?format=${encodeURIComponent(requestedFormat)}` : ''}`, activeFile));
				if (loadToken !== activeComicLoadToken) return;
				if (!pagesRes.ok) throw new Error(await fileResponseError(pagesRes));
				if (pagesRes.ok) {
					const data = await pagesRes.json();
					numPages = data.pages;
					if (savedProgress && savedProgress.page > 0) {
						currentPage = Math.max(1, Math.min(numPages, savedProgress.page));
					} else if (savedProgress?.percent > 0 && numPages > 0) {
						currentPage = Math.max(1, Math.min(numPages, Math.round((savedProgress.percent / 100) * Math.max(0, numPages - 1)) + 1));
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
			readerError = e instanceof Error ? e.message : 'Failed to load book. Check your connection and try again.';
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
			settings = resolveComicSettings(s.cbx, s.comicOverrides?.[comicBookId]);
			sessionFitMode = settings.fitMode;
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
		const routeBookId = comicBookId;
		const routeFormat = $page.url.searchParams.get('format');
		if (!routeBookId) return;

		queueMicrotask(() => {
			void loadComic(routeBookId, routeFormat);
		});
	});

	async function saveProgress(_keepalive = false) {
		if (!book || !progressController || readerError || numPages <= 0) return;
		const percent = numPages > 1 ? ((currentPage - 1) / (numPages - 1)) * 100 : 0;
		await progressController.checkpoint({
			readerMode: 'comic',
			percent,
			locator: { type: 'comic_page', page: currentPage, total_pages: numPages },
			reachedEnd: numPages > 0 && currentPage === numPages
		});
	}

	function updateSpreadPages() {
		if (settings.pageSpread !== 'even' && settings.pageSpread !== 'odd') {
			currentSpreadPages = null;
		} else if (settings.pageSpread === 'even') {
			const page = currentPage % 2 === 0 ? currentPage : currentPage + 1;
			currentSpreadPages = [page - 1, page];
		} else if (settings.pageSpread === 'odd') {
			const page = currentPage % 2 === 1 ? currentPage : currentPage - 1;
			currentSpreadPages = [page, page + 1];
		}
	}

	async function startSession() {
		if (!book?.id || !activeFile) return;
		progressController = new ReadingProgressController({
			bookId: book.id,
			file: activeFile,
			channel: 'standard',
			readerMode: 'comic'
		});
		const position = await progressController.start();
		savedProgress = readingPositionAsLegacy(position, 'comic');
	}

	async function endSession(keepalive = false) {
		if (sessionEnded || !progressController) return;
		sessionEnded = true;
		await progressController.end(keepalive);
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
		const horizontal = settings.scrollMode === 'long-strip';
		const rtl = settings.readingDirection === 'rtl';
		const anchorPosition = horizontal
			? (rtl ? containerRect.right - Math.min(160, stripScrollEl.clientWidth / 3) : containerRect.left + Math.min(160, stripScrollEl.clientWidth / 3))
			: containerRect.top + Math.min(160, stripScrollEl.clientHeight / 3);
		let activePage = currentPage;
		let activePageDistance = Number.POSITIVE_INFINITY;

		for (const pageEl of pages) {
			const pageNum = Number(pageEl.dataset.page);
			if (!Number.isFinite(pageNum)) continue;

			const rect = pageEl.getBoundingClientRect();
			const start = horizontal ? rect.left : rect.top;
			const end = horizontal ? rect.right : rect.bottom;
			if (start <= anchorPosition && end >= anchorPosition) {
				activePage = pageNum;
				activePageDistance = 0;
				break;
			}

			const distance = Math.min(Math.abs(start - anchorPosition), Math.abs(end - anchorPosition));
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

		const rect = pageEl.getBoundingClientRect();
		const viewport = stripScrollEl.getBoundingClientRect();
		if (settings.scrollMode === 'long-strip') {
			const delta = settings.readingDirection === 'rtl' ? rect.right - viewport.right : rect.left - viewport.left;
			stripScrollEl.scrollTo({ left: stripScrollEl.scrollLeft + delta, behavior });
		} else {
			stripScrollEl.scrollTo({ top: Math.max(0, stripScrollEl.scrollTop + rect.top - viewport.top - STRIP_PADDING_PX), behavior });
		}
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
		if (!(e.target instanceof Element) || !e.target.closest('.page-viewer, .long-strip')) return;
		pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
		if (pointers.size > 1) {
			gestureWasPinch = true;
			const [a, b] = [...pointers.values()];
			pinchDistance = Math.hypot(a.x - b.x, a.y - b.y);
			return;
		}
		gestureWasPinch = false;
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
		const previous = pointers.get(e.pointerId);
		if (previous) pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
		if (!isStripMode() && pointers.size === 2) {
			const [a, b] = [...pointers.values()];
			const distance = Math.hypot(a.x - b.x, a.y - b.y);
			if (pinchDistance > 0) changeZoom(zoom * distance / pinchDistance);
			pinchDistance = distance;
			return;
		}
		if (!isStripMode() && previous && viewerEl) {
			viewerEl.scrollTop -= e.clientY - previous.y;
			if (zoom !== 1 || sessionFitMode === 'actual-size') viewerEl.scrollLeft -= e.clientX - previous.x;
		}
		if (!isStripMode() && zoom > 1 && previous && viewerEl?.parentElement) {
			viewerEl.parentElement.scrollLeft -= e.clientX - previous.x;
			viewerEl.parentElement.scrollTop -= e.clientY - previous.y;
		}
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
		pointers.delete(e.pointerId);
		resetReaderPointerTracking();
		if (gestureWasPinch) return;
		if (pointerStart && pointerMoved && !isStripMode() && zoom === 1) {
			const dx = e.clientX - pointerStart.x;
			const dy = e.clientY - pointerStart.y;
			if (Math.abs(dx) > 45 && Math.abs(dx) > Math.abs(dy) * 1.5) {
				if (dx < 0) nextPage(); else prevPage();
			}
			return;
		}
		if (controlsNeedToStayVisible()) return;
		const target = e.target as Element | null;
		if (target?.closest('input, textarea, select, [contenteditable="true"], .left-sidebar, .right-sidebar, .progress-bar, .reader-progress, .floating-nav')) {
			return;
		}
		if (!pointerStart || pointerStart.id !== e.pointerId) return;
		if (pointerMoved) return;
		if (isReaderCenterTap(e, e.currentTarget as HTMLElement)) {
			toggleTopBarFromCenterTap();
		}
	}

	function handleReaderPointerCancel() {
		pointers.clear();
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
		progressController?.destroy();
	});

	function updateSetting(key: string, value: any) {
		settings = { ...settings, [key]: value };
		readerSettings.updateComic(comicBookId, { [key]: value });

		if (key === 'pageSpread') {
			updateSpreadPages();
		}
		if (key === 'scrollMode' || key === 'stripMaxWidthPercent' || key === 'readingDirection') {
			void tick().then(() => {
				if (isStripMode()) {
					scrollToStripPage(currentPage, 'auto');
				}
			});
		}

		resetTopBarBehavior();
	}

	function prevPage() {
		zoom = 1;
		resetTopBarBehavior();
		if (settings.readingDirection === 'rtl') {
			if (currentPage < numPages) {
				currentPage = Math.min(numPages, !isStripMode() && currentSpreadPages ? currentSpreadPages[1] + 1 : currentPage + 1);
				updateSpreadPages();
			}
		} else {
			if (currentPage > 1) {
				currentPage = Math.max(1, !isStripMode() && currentSpreadPages ? currentSpreadPages[0] - 1 : currentPage - 1);
				updateSpreadPages();
			}
		}
		if (isStripMode()) {
			scrollToStripPage(currentPage);
		}
		saveProgress();
	}

	function nextPage() {
		zoom = 1;
		resetTopBarBehavior();
		if (settings.readingDirection === 'rtl') {
			if (currentPage > 1) {
				currentPage = Math.max(1, !isStripMode() && currentSpreadPages ? currentSpreadPages[0] - 1 : currentPage - 1);
				updateSpreadPages();
			}
		} else {
			if (currentPage < numPages) {
				currentPage = Math.min(numPages, !isStripMode() && currentSpreadPages ? currentSpreadPages[1] + 1 : currentPage + 1);
				updateSpreadPages();
			}
		}
		if (isStripMode()) {
			scrollToStripPage(currentPage);
		}
		saveProgress();
	}

	function goToPage(pageNum: number) {
		zoom = 1;
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
		const newPage = Math.max(1, Math.round(percentage * (numPages - 1)) + 1);
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
		pointers.clear();
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

	function handleCloseControl() {
		if (leftSidebarOpen || rightSidebarOpen) {
			leftSidebarOpen = false;
			rightSidebarOpen = false;
			resetTopBarBehavior();
			return;
		}
		closeReader();
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
		if (e.defaultPrevented || shouldIgnoreWheelNavigation(e.target)) return;
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
		if (shouldIgnoreWheelNavigation(e.target)) return;
		if (e.ctrlKey || e.metaKey) {
			if (!isStripMode()) {
				e.preventDefault();
				changeZoom(zoom * Math.exp(-e.deltaY * 0.005));
			}
			return;
		}
		if (settings.scrollMode === 'long-strip' && stripScrollEl && e.target instanceof Element && e.target.closest('.long-strip')) {
			if (Math.abs(e.deltaY) > Math.abs(e.deltaX)) {
				e.preventDefault();
				const delta = e.deltaY * (e.deltaMode === 1 ? 16 : e.deltaMode === 2 ? stripScrollEl.clientWidth : 1);
				stripScrollEl.scrollLeft += delta * (settings.readingDirection === 'rtl' ? -1 : 1);
			}
			return;
		}
		if (zoom !== 1 || sessionFitMode === 'fit-width' || sessionFitMode === 'actual-size' || sessionFitMode === 'fit-height') return;
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
		const scrollTop = settings.scrollMode === 'long-strip' ? Math.abs(container.scrollLeft) : container.scrollTop;
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
		return withReaderFile(`/api/cbx/${book.id}/page/${pageNum}${requestedFormat ? `?format=${encodeURIComponent(requestedFormat)}` : ''}`, activeFile);
	}

	function rememberPageAspect(pageNumber: number, event: Event) {
		const image = event.currentTarget;
		if (image instanceof HTMLImageElement && image.naturalHeight > 0) {
			pageAspectRatios[pageNumber] = image.naturalWidth / image.naturalHeight;
		}
	}

	function getPageImageClass() {
		return `page-image fit-${sessionFitMode}`;
	}

	function resetToDefaults() {
		sessionFitMode = 'fit-page';
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
	class="cbx-reader" style:--reader-bottom-bar-height={`${bottomBarHeight}px`}
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
			<button
				type="button"
				onclick={handleCloseControl}
				class="nav-btn nav-close"
				title={rightSidebarOpen ? 'Close Settings (Esc)' : leftSidebarOpen ? 'Close Pages (Esc)' : 'Close reader (Esc)'}
				aria-label={rightSidebarOpen ? 'Close Settings' : leftSidebarOpen ? 'Close Pages' : 'Close reader'}
			>
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<line x1="18" y1="6" x2="6" y2="18"></line>
					<line x1="6" y1="6" x2="18" y2="18"></line>
				</svg>
			</button>

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
				<ReaderSizingControls currentMode={sessionFitMode} onfit={(mode) => { setFitMode(mode); showTopBar(); }}
					showMenu={false} />
			<button onclick={toggleRightSidebar} class="nav-btn" class:active={rightSidebarOpen} title="Settings">
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="3"></circle>
					<path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
				</svg>
			</button>

			<button onclick={toggleFullscreen} class="nav-btn" title="Fullscreen">
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
			ariaValueNow={Math.round(progress)}
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
			{:else if numPages > 0 && !readerError}
				{#if settings.scrollMode === 'long-strip' || settings.scrollMode === 'infinite'}
					<div class="long-strip" class:horizontal-strip={settings.scrollMode === 'long-strip'} style:direction={settings.scrollMode === 'long-strip' && settings.readingDirection === 'rtl' ? 'rtl' : 'ltr'} bind:this={stripScrollEl} onscroll={handleLongStripScroll}>
						<div class="strip-content" class:strip-fit-page={sessionFitMode === 'fit-page' || sessionFitMode === 'automatic'} class:strip-fit-height={sessionFitMode === 'fit-height'}>
							{#each Array(numPages) as _, i (i)}
								{@const pageNum = i + 1}
								<img
									src={getPageUrl(pageNum)}
								onerror={() => { readerError = 'A comic page could not be loaded. The file may have become unavailable or the archive may be damaged. Try again after checking your library storage.'; }}
									alt="Page {pageNum}"
									class="strip-page"
									data-page={pageNum}
									loading={getStripPageLoading(pageNum)}
									decoding="async"
									onload={() => handleStripPageLoad(pageNum)}
									style="width: {sessionFitMode === 'fit-width' ? settings.stripMaxWidthPercent + '%' : 'auto'}; filter: saturate({settings.saturation}%) brightness({settings.vibrance / 100});"
								/>
							{/each}
						</div>
					</div>
				{:else}
					<div
						class="page-viewer" bind:this={viewerEl} style:zoom={zoom} style:direction={settings.readingDirection === 'rtl' ? 'rtl' : 'ltr'}
						style:grid-template-columns={currentSpreadPages && (sessionFitMode === 'fit-page' || sessionFitMode === 'automatic') && currentSpreadPages[1] <= numPages ? `${pageAspectRatios[currentSpreadPages[0]] ?? 1}fr ${pageAspectRatios[currentSpreadPages[1]] ?? 1}fr` : undefined}
						class:fit-page={sessionFitMode === 'fit-page' || sessionFitMode === 'automatic'}
						class:spread={!!currentSpreadPages && (settings.pageSpread === 'even' || settings.pageSpread === 'odd') && currentSpreadPages[1] <= numPages}
					>
						{#if currentSpreadPages && (settings.pageSpread === 'even' || settings.pageSpread === 'odd')}
							<img
								src={getPageUrl(currentSpreadPages[0])}
								alt="Page {currentSpreadPages[0]}"
								onload={(event) => currentSpreadPages && rememberPageAspect(currentSpreadPages[0], event)}
								class={getPageImageClass()}
								style="filter: saturate({settings.saturation}%) brightness({settings.vibrance / 100});"
							/>
							{#if currentSpreadPages[1] <= numPages}
								<img
									src={getPageUrl(currentSpreadPages[1])}
									alt="Page {currentSpreadPages[1]}"
									onload={(event) => currentSpreadPages && rememberPageAspect(currentSpreadPages[1], event)}
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
						disabled={settings.readingDirection === 'rtl' ? currentPage >= numPages : currentPage <= 1}
					>
						<svg class="icon-lg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polyline points="15 18 9 12 15 6"></polyline>
						</svg>
					</button>
					<button
						class="floating-nav floating-next"
						onclick={nextPage}
						aria-label="Next page"
						disabled={settings.readingDirection === 'rtl' ? currentPage <= 1 : currentPage >= numPages}
					>
						<svg class="icon-lg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polyline points="9 18 15 12 9 6"></polyline>
						</svg>
					</button>
				{/if}
			{:else}
				<div class="error-message">
					<p>{readerError || 'No readable pages were found. The archive may be damaged or unsupported.'}</p>
					<button type="button" class="btn" onclick={() => window.location.reload()}>Try again</button>
				<a href={getReaderReturnUrl()} class="btn">Back to book</a>
				</div>
			{/if}
		</div>

		<!-- Right Sidebar - Settings -->
		<aside class="right-sidebar" class:open={rightSidebarOpen} aria-label="Comic settings">
			<div class="settings-content">
				<div class="settings-section">
					<label class="toggle-option">
						<span>Remember reading settings for this comic</span>
						<input type="checkbox" checked={hasComicOverride} onchange={(e) => readerSettings.setComicOverride(comicBookId, e.currentTarget.checked)} />
					</label>
					<p class="settings-help">{hasComicOverride ? 'Reading choices apply to this comic. Appearance and behavior stay global.' : 'Changes update your global defaults for all comics.'}</p>
					{#if hasComicOverride}
						<button type="button" class="option-btn" onclick={() => readerSettings.setComicOverride(comicBookId, false)}>Use global defaults</button>
					{/if}
				</div>
				<div class="settings-section">
					<div class="settings-label">Page size</div>
					<ChoiceChips label="Page size" value={sessionFitMode} options={cbxFitModes.filter(mode => mode.id !== 'automatic')}
						onchange={(value) => setFitMode(value as CbxReaderSetting['fitMode'])} />
				</div>
				<div class="settings-section">
					<div class="settings-label">Scroll mode</div>
					<ChoiceChips label="Scroll mode" value={settings.scrollMode} options={cbxScrollModes}
						onchange={(value) => updateSetting('scrollMode', value)} />
				</div>
				{#if isStripMode()}
					<div class="settings-section">
						<label class="settings-label" for="comic-strip-width">Strip width: {settings.stripMaxWidthPercent}%</label>
						<input id="comic-strip-width" type="range" min="50" max="150" value={settings.stripMaxWidthPercent}
							oninput={(e) => { setFitMode('fit-width'); updateSetting('stripMaxWidthPercent', parseInt(e.currentTarget.value)); }}
							class="range-input-full" />
					</div>
				{/if}
				<div class="settings-section">
					<div class="settings-label">Reading direction</div>
					<ChoiceChips label="Reading direction" value={settings.readingDirection}
						options={[{ id: 'ltr', name: 'Left to right' }, { id: 'rtl', name: 'Right to left' }]}
						onchange={(value) => updateSetting('readingDirection', value)} />
				</div>
				{#if !isStripMode()}
					<div class="settings-section">
						<div class="settings-label">Page spread</div>
						<ChoiceChips label="Page spread" value={settings.pageSpread}
							options={[{ id: 'off', name: 'Off' }, { id: 'even', name: 'Even' }, { id: 'odd', name: 'Odd' }]}
							onchange={(value) => updateSetting('pageSpread', value)} />
					</div>
				{/if}
				<div class="settings-section">
					<label class="settings-label" for="comic-background">Background</label>
					<div class="color-picker">
						<input id="comic-background" type="color" value={settings.backgroundColor}
							oninput={(e) => updateSetting('backgroundColor', e.currentTarget.value)} class="color-input" />
						<span class="color-value">{settings.backgroundColor}</span>
					</div>
				</div>
				<div class="settings-section">
					<label class="settings-label" for="comic-brightness">Brightness: {settings.vibrance}%</label>
					<input id="comic-brightness" type="range" min="0" max="200" value={settings.vibrance}
						oninput={(e) => updateSetting('vibrance', parseInt(e.currentTarget.value))} class="range-input-full" />
				</div>
				<div class="settings-section">
					<label class="settings-label" for="comic-saturation">Saturation: {settings.saturation}%</label>
					<input id="comic-saturation" type="range" min="0" max="200" value={settings.saturation}
						oninput={(e) => updateSetting('saturation', parseInt(e.currentTarget.value))} class="range-input-full" />
				</div>
				<div class="settings-section">
					<label class="toggle-option"><span>Auto-hide controls</span>
						<input type="checkbox" checked={settings.autoHideControls} onchange={(e) => updateSetting('autoHideControls', e.currentTarget.checked)} />
					</label>
				</div>
				<div class="settings-section">
					<label class="toggle-option"><span>Use standard fullscreen</span>
						<input type="checkbox" checked={settings.useStandardFullscreen} onchange={(e) => updateSetting('useStandardFullscreen', e.currentTarget.checked)} />
					</label>
				</div>
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
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr);
		align-items: center;
		column-gap: 16px;
		height: var(--reader-top-bar-height);
		padding: 0 12px;
		background: var(--color-surface-base, #0f172a);
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		flex-shrink: 0;
		z-index: 120;
		isolation: isolate;
		transform: translateY(0);
		transition: opacity 0.15s ease;
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
		min-height: 0;
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
		justify-content: safe center;
		align-items: safe center;
		overflow: auto;
		position: relative;
		min-width: 0;
	}

	.cbx-container { box-sizing: border-box; min-height: 0; }

	.page-viewer {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0;
		width: 100%;
		height: 100%;
		overflow: auto;
		touch-action: none;
	}

	.page-viewer:not(.fit-page) { align-items: flex-start; justify-content: safe center; }
	.page-viewer.spread .page-image.fit-fit-width { width: 50%; }
	.page-viewer.fit-page { display: grid; grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(0, 1fr); }
	.page-viewer.fit-page.spread { grid-template-columns: repeat(2, minmax(0, 1fr)); }
	.page-viewer.fit-page .page-image { width: 100%; height: 100%; min-width: 0; min-height: 0; }

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
		padding: 16px 0;
		box-sizing: border-box;
	}

	.strip-content {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		width: 100%;
		min-width: 0;
		margin: 0 auto;
	}

	.horizontal-strip { padding: 0; }
	.horizontal-strip .strip-content {
		flex-direction: row;
		height: 100%;
		align-items: safe center;
	}

	.strip-fit-page .strip-page {
		max-width: 100%;
		max-height: calc(100dvh - 32px);
	}
	.strip-fit-height .strip-page {
		height: calc(100dvh - 32px);
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

	.settings-help { font-size: 12px; color: var(--color-surface-text-muted); margin: 8px 0; }

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
		.top-nav { grid-template-columns: auto minmax(0, 1fr); }

		.nav-btn {
			width: 44px;
			height: 44px;
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

		.page-viewer {
			flex-direction: row;
			gap: 0;
			width: 100%;
		}

		.page-image.fit-fit-page,
		.page-image.fit-automatic {
			max-height: 100%;
		}

		.page-image.fit-fit-width {
			width: 100%;
			max-width: 100%;
			max-height: none;
		}

		.page-image.fit-fit-height {
			height: calc(100vh - 120px);
			max-height: 100%;
		}

		.long-strip {
			padding: 0;
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

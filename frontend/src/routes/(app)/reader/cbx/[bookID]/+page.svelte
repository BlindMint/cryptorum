<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { readerSettings, cbxFitModes, cbxScrollModes, type CbxReaderSetting } from '$lib/stores/readerSettings';
	import { normalizeBookFormat } from '$lib/utils/book-formats';
	import { toggleReaderFullscreen } from '$lib/utils/fullscreen';

	let book = $state<any>(null);
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
	let progressBarEl = $state<HTMLElement | null>(null);
	let activeProgressPointerId: number | null = null;
	let lastWheelNavigationAt = 0;
	let topBarHideTimeout: ReturnType<typeof setTimeout> | null = null;
	let lastLongStripScrollTop = 0;
	let sessionEnded = false;
	let handlePageExit: (() => void) | null = null;
	let requestedFormat = $state('');
	let topBarVisible = $state(true);
	let stripScrollEl = $state<HTMLDivElement | null>(null);
	let stripContentWidth = $state(1);
	let stripTotalHeight = $state(0);
	let visibleStripPages = $state<number[]>([]);
	let stripPageLayouts = $state(new Map<number, { top: number; width: number; height: number }>());
	let stripProgressSaveTimeout: ReturnType<typeof setTimeout> | null = null;
	let closeTasksStarted = false;

	const TOP_BAR_HIDE_DELAY_MS = 2800;
	const TOP_BAR_SCROLL_DELTA = 12;
	const TOP_BAR_REVEAL_EDGE_PX = 72;
	const STRIP_PADDING_PX = 16;
	const STRIP_PAGE_GAP_PX = 8;
	const STRIP_OVERSCAN_PX = 2400;
	const STRIP_ESTIMATED_ASPECT_RATIO = 1.45;

	const progress = $derived(numPages > 0 ? (currentPage / numPages) * 100 : 0);

	onMount(async () => {
		const bookId = $page.params.bookID;
		try {
			const res = await fetch(`/api/books/${bookId}`);
			if (res.ok) {
				book = await res.json();
				requestedFormat = normalizeBookFormat($page.url.searchParams.get('format'));
				await fetchProgress();
				await startSession();
				const pagesRes = await fetch(`/api/cbx/${bookId}/pages${requestedFormat ? `?format=${encodeURIComponent(requestedFormat)}` : ''}`);
				if (pagesRes.ok) {
					const data = await pagesRes.json();
					numPages = data.pages;
					if (savedProgress && savedProgress.page > 0) {
						currentPage = savedProgress.page;
					}
					updateSpreadPages();
					await tick();
					rebuildStripLayout();
					if (isStripMode()) {
						scrollToStripPage(currentPage, 'auto');
					}
				}
			}
		} catch (e) {
			console.error('Failed to load book:', e);
		} finally {
			loading = false;
		}

		readerSettings.subscribe(s => {
			const previousScrollMode = settings.scrollMode;
			const previousStripWidth = settings.stripMaxWidthPercent;
			settings = { ...s.cbx };
			if (!settings.autoHideControls) {
				showTopBar();
			}
			if (previousScrollMode !== settings.scrollMode || previousStripWidth !== settings.stripMaxWidthPercent) {
				void tick().then(() => {
					rebuildStripLayout();
					if (isStripMode()) {
						scrollToStripPage(currentPage, 'auto');
					}
				});
			}
		});

		handlePageExit = () => {
			if (closeTasksStarted) return;
			void endSession(true);
		};
		window.addEventListener('pagehide', handlePageExit);
		window.addEventListener('beforeunload', handlePageExit);

		resetTopBarBehavior();
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

	function isStripMode() {
		return settings.scrollMode === 'long-strip' || settings.scrollMode === 'infinite';
	}

	function getStripPageWidth() {
		if (!stripScrollEl) return stripContentWidth;
		const horizontalPadding = window.matchMedia('(max-width: 640px)').matches ? 16 : 32;
		const availableWidth = Math.max(1, stripScrollEl.clientWidth - horizontalPadding);
		return Math.max(1, Math.floor(availableWidth * (settings.stripMaxWidthPercent / 100)));
	}

	function rebuildStripLayout() {
		if (!numPages) {
			stripPageLayouts = new Map();
			visibleStripPages = [];
			stripTotalHeight = 0;
			return;
		}

		const pageWidth = getStripPageWidth();
		const nextLayouts = new Map<number, { top: number; width: number; height: number }>();
		let top = STRIP_PADDING_PX;
		for (let pageNum = 1; pageNum <= numPages; pageNum++) {
			const previous = stripPageLayouts.get(pageNum);
			const height = previous?.height ?? Math.round(pageWidth * STRIP_ESTIMATED_ASPECT_RATIO);
			nextLayouts.set(pageNum, { top, width: pageWidth, height });
			top += height + STRIP_PAGE_GAP_PX;
		}

		stripContentWidth = pageWidth;
		stripPageLayouts = nextLayouts;
		stripTotalHeight = Math.max(0, top - STRIP_PAGE_GAP_PX + STRIP_PADDING_PX);
		updateVisibleStripPages();
	}

	function updateVisibleStripPages() {
		if (!stripScrollEl || !isStripMode() || !numPages) return;
		const viewportTop = Math.max(0, stripScrollEl.scrollTop - STRIP_OVERSCAN_PX);
		const viewportBottom = stripScrollEl.scrollTop + stripScrollEl.clientHeight + STRIP_OVERSCAN_PX;
		const nextVisible: number[] = [];
		let activePage = currentPage;
		let activePageDistance = Number.POSITIVE_INFINITY;
		const viewportAnchor = stripScrollEl.scrollTop + Math.min(160, stripScrollEl.clientHeight / 3);

		for (let pageNum = 1; pageNum <= numPages; pageNum++) {
			const layout = stripPageLayouts.get(pageNum);
			if (!layout) continue;
			const pageBottom = layout.top + layout.height;
			if (pageBottom >= viewportTop && layout.top <= viewportBottom) {
				nextVisible.push(pageNum);
			}
			const distance = Math.abs(layout.top - viewportAnchor);
			if (distance < activePageDistance) {
				activePageDistance = distance;
				activePage = pageNum;
			}
		}

		visibleStripPages = nextVisible;
		if (activePage !== currentPage) {
			currentPage = activePage;
			updateSpreadPages();
			queueStripProgressSave();
		}
	}

	function scrollToStripPage(pageNum: number, behavior: ScrollBehavior = 'smooth') {
		if (!stripScrollEl) return;
		if (stripPageLayouts.size === 0) {
			rebuildStripLayout();
		}
		const layout = stripPageLayouts.get(pageNum);
		if (!layout) return;
		stripScrollEl.scrollTo({ top: Math.max(0, layout.top - STRIP_PADDING_PX), behavior });
		updateVisibleStripPages();
	}

	function handleStripImageLoad(pageNum: number, e: Event) {
		const image = e.currentTarget as HTMLImageElement;
		const layout = stripPageLayouts.get(pageNum);
		if (!layout || !image.naturalWidth || !image.naturalHeight) return;

		const nextHeight = Math.round(layout.width * (image.naturalHeight / image.naturalWidth));
		if (Math.abs(nextHeight - layout.height) < 2) return;

		const currentTop = stripScrollEl?.scrollTop ?? 0;
		const shouldPreserveAnchor = currentTop > layout.top;
		const heightDelta = nextHeight - layout.height;
		layout.height = nextHeight;

		for (let nextPage = pageNum + 1; nextPage <= numPages; nextPage++) {
			const nextLayout = stripPageLayouts.get(nextPage);
			if (nextLayout) {
				nextLayout.top += heightDelta;
			}
		}

		stripTotalHeight += heightDelta;
		if (stripScrollEl && shouldPreserveAnchor) {
			stripScrollEl.scrollTop = currentTop + heightDelta;
		}
		updateVisibleStripPages();
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

	function handleReaderPointerMove(e: PointerEvent) {
		if (!settings.autoHideControls || controlsNeedToStayVisible()) return;
		if (e.clientY <= TOP_BAR_REVEAL_EDGE_PX) {
			showTopBar(settings.scrollMode !== 'long-strip' && settings.scrollMode !== 'infinite');
		}
	}

	function handleReaderPointerUp(e: PointerEvent) {
		if (controlsNeedToStayVisible()) return;
		const target = e.target as Element | null;
		if (target?.closest('input, textarea, select, [contenteditable="true"], .left-sidebar, .right-sidebar, .progress-bar')) {
			return;
		}
		showTopBar(settings.scrollMode !== 'long-strip' && settings.scrollMode !== 'infinite');
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
				rebuildStripLayout();
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
		const newPage = Math.round(percentage * numPages);
		return newPage >= 1 && newPage <= numPages ? newPage : null;
	}

	function updateProgressDrag(clientX: number) {
		const newPage = getProgressPageFromClientX(clientX);
		if (newPage === null) return;
		pendingProgressPage = newPage;
	}

	function handleProgressPointerDown(e: PointerEvent) {
		e.preventDefault();
		e.stopPropagation();
		resetTopBarBehavior();
		isDraggingProgress = true;
		pendingProgressPage = null;
		activeProgressPointerId = e.pointerId;
		(e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
		updateProgressDrag(e.clientX);
	}

	function handleProgressPointerMove(e: PointerEvent) {
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

		if (targetPage !== null) {
			goToPage(targetPage);
		}
	}

	function handleProgressPointerUp(e: PointerEvent) {
		if (activeProgressPointerId !== null && e.pointerId !== activeProgressPointerId) return;
		e.preventDefault();
		e.stopPropagation();
		resetTopBarBehavior();
		finishProgressPointer(e);
	}

	function handleProgressPointerCancel(e: PointerEvent) {
		if (activeProgressPointerId !== null && e.pointerId !== activeProgressPointerId) return;
		finishProgressPointer(e);
	}

	function handleProgressBarKeydown(e: KeyboardEvent) {
		if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
			e.preventDefault();
			resetTopBarBehavior();
			prevPage();
		} else if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
			e.preventDefault();
			resetTopBarBehavior();
			nextPage();
		}
	}

	function startCloseBackgroundTasks() {
		if (closeTasksStarted) return;
		closeTasksStarted = true;
		void saveProgress(true);
		void endSession(true);
	}

	function closeReader(e?: Event) {
		e?.preventDefault();
		const targetUrl = book ? `/book/${book.id}` : '/book';
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
		updateVisibleStripPages();
		if (!settings.autoHideControls || controlsNeedToStayVisible()) {
			lastLongStripScrollTop = scrollTop;
			return;
		}

		const delta = scrollTop - lastLongStripScrollTop;
		if (scrollTop <= 2) {
			showTopBar(false);
		} else if (delta > TOP_BAR_SCROLL_DELTA) {
			hideTopBar();
		} else if (delta < -TOP_BAR_SCROLL_DELTA) {
			showTopBar(false);
		}
		lastLongStripScrollTop = scrollTop;
	}

	function getPageUrl(pageNum: number): string {
		return `/api/cbx/${book.id}/page/${pageNum}${requestedFormat ? `?format=${encodeURIComponent(requestedFormat)}` : ''}`;
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
			rebuildStripLayout();
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
	<title>{book?.title || 'Reading'} - Cryptorum</title>
</svelte:head>

<div
	class="cbx-reader"
	style="background-color: {settings.backgroundColor};"
	role="presentation"
	onpointermove={handleReaderPointerMove}
	onpointerup={handleReaderPointerUp}
>
	<!-- Top Navigation Bar -->
	<header class="top-nav" class:top-nav-hidden={!topBarVisible}>
		<div class="nav-left">
			<a
				href={book ? `/book/${book.id}` : '/book'}
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
				title="Toggle Sidebar"
			>
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<line x1="3" y1="6" x2="21" y2="6"></line>
					<line x1="3" y1="12" x2="21" y2="12"></line>
					<line x1="3" y1="18" x2="21" y2="18"></line>
				</svg>
			</button>

			<div class="nav-divider"></div>

			<div class="page-controls">
				<button onclick={prevPage} class="nav-btn" disabled={settings.mangaMode || settings.readingDirection === 'rtl' ? currentPage >= numPages : currentPage <= 1} title="Previous Page">
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<polyline points="15 18 9 12 15 6"></polyline>
					</svg>
				</button>

				<span class="page-display">{currentPage} / {numPages}</span>

				<button onclick={nextPage} class="nav-btn" disabled={settings.mangaMode || settings.readingDirection === 'rtl' ? currentPage <= 1 : currentPage >= numPages} title="Next Page">
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<polyline points="9 18 15 12 9 6"></polyline>
					</svg>
				</button>
			</div>

			<div class="nav-divider"></div>

			<button onclick={toggleLeftSidebar} class="nav-btn mobile-secondary" title="Pages">
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<rect x="3" y="3" width="7" height="7"></rect>
					<rect x="14" y="3" width="7" height="7"></rect>
					<rect x="14" y="14" width="7" height="7"></rect>
					<rect x="3" y="14" width="7" height="7"></rect>
				</svg>
			</button>
		</div>

		<div class="nav-center">
			<span class="book-title">{book?.title || 'Loading...'}</span>
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

		<div
			bind:this={progressBarEl}
			class="progress-bar"
			onpointerdown={(e) => handleProgressPointerDown(e)}
			onpointermove={(e) => handleProgressPointerMove(e)}
			onpointerup={(e) => handleProgressPointerUp(e)}
			onpointercancel={(e) => handleProgressPointerCancel(e)}
			role="slider"
			aria-label="Reading progress"
			aria-valuemin="0"
			aria-valuemax="100"
			aria-valuenow={numPages > 0 ? Math.round((currentPage / numPages) * 100) : 0}
			tabindex="0"
			onkeydown={handleProgressBarKeydown}
		>
			<div class="progress-fill" style="--progress: {progress}%;"></div>
			<div
				class="progress-thumb"
				style="left: calc({progress}% - 6px);"
				role="presentation"
			></div>
		</div>
	</header>

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
						<div
							class="strip-content"
							style="width: {stripContentWidth}px; height: {stripTotalHeight}px;"
						>
							{#each visibleStripPages as pageNum (pageNum)}
								{@const layout = stripPageLayouts.get(pageNum)}
								{#if layout}
									<img
										src={getPageUrl(pageNum)}
										alt="Page {pageNum}"
										class="strip-image"
										style="top: {layout.top}px; width: {layout.width}px; filter: saturate({settings.saturation}%) brightness({settings.vibrance / 100});"
										onload={(e) => handleStripImageLoad(pageNum, e)}
									/>
								{/if}
							{/each}
						</div>
					</div>
				{:else}
					<div class="page-viewer">
						{#if currentSpreadPages && (settings.pageSpread === 'even' || settings.pageSpread === 'odd')}
							<img
								src={getPageUrl(currentSpreadPages[0])}
								alt="Page {currentSpreadPages[0]}"
								class="page-image"
								style="filter: saturate({settings.saturation}%) brightness({settings.vibrance / 100});"
							/>
							{#if currentSpreadPages[1] <= numPages}
								<img
									src={getPageUrl(currentSpreadPages[1])}
									alt="Page {currentSpreadPages[1]}"
									class="page-image"
									style="filter: saturate({settings.saturation}%) brightness({settings.vibrance / 100});"
								/>
							{/if}
						{:else}
							<img
								src={getPageUrl(currentPage)}
								alt="Page {currentPage}"
								class="page-image"
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
					<a href="/book/{book?.id}" class="btn">Return to Library</a>
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
								max="100"
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
		--reader-top-bar-height: 48px;
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
		display: flex;
		align-items: center;
		justify-content: space-between;
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

	.nav-left { flex: 1; }
	.nav-center { flex: 2; justify-content: center; }
	.nav-right { flex: 1; justify-content: flex-end; }

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

	.nav-btn:hover { background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85)); }
	.nav-btn:disabled { opacity: 0.3; cursor: not-allowed; }
	.nav-btn.active { background: var(--color-primary-500, #22c55e); color: white; }

	.nav-divider {
		width: 1px;
		height: 24px;
		background: var(--color-surface-border, rgba(55, 65, 81, 0.6));
		margin: 0 8px;
	}

	.page-controls { display: flex; align-items: center; gap: 4px; }

	.page-display {
		min-width: 80px;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 14px;
		text-align: center;
	}

	.book-title {
		color: var(--color-surface-text, #e2e8f0);
		font-size: 14px;
		font-weight: 500;
		max-width: 300px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.icon { width: 20px; height: 20px; }
	.icon-lg { width: 24px; height: 24px; }

	.progress-bar {
		position: absolute;
		bottom: 0;
		left: 0;
		right: 0;
		height: 12px;
		background: transparent;
		cursor: pointer;
		display: flex;
		align-items: flex-end;
		touch-action: none;
	}

	.progress-fill {
		position: absolute;
		left: 0;
		bottom: 0;
		height: 3px;
		background: var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 2px;
		width: 100%;
	}

	.progress-fill::after {
		content: '';
		position: absolute;
		left: 0;
		top: 0;
		height: 100%;
		width: var(--progress, 0%);
		background: var(--color-primary-500, #22c55e);
		border-radius: 2px;
	}

	.progress-thumb {
		position: absolute;
		bottom: -4px;
		width: 12px;
		height: 12px;
		background: var(--color-primary-500, #22c55e);
		border: 2px solid var(--color-surface-base, #0f172a);
		border-radius: 50%;
		cursor: grab;
		z-index: 10;
		transition: left 0.05s ease;
		touch-action: none;
	}

	.progress-thumb:hover { transform: scale(1.2); }

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
		display: flex;
		align-items: center;
		justify-content: center;
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
		height: 100%;
	}

	.page-image {
		max-width: 100%;
		max-height: 100%;
		object-fit: contain;
	}

	.long-strip {
		width: 100%;
		height: 100%;
		overflow-y: auto;
		overflow-x: auto;
	}

	.strip-content {
		position: relative;
		margin: 0 auto;
	}

	.strip-image {
		position: absolute;
		left: 0;
		display: block;
		height: auto;
		object-fit: contain;
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
		flex: 1;
		padding: 12px 8px;
		border: none;
		background: transparent;
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 12px;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s;
	}

	.settings-tab:hover { color: var(--color-surface-text, #e2e8f0); }
	.settings-tab.active {
		color: var(--color-primary-500, #22c55e);
		box-shadow: inset 0 -2px 0 var(--color-primary-500, #22c55e);
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
			--reader-top-bar-height: 64px;
		}

		.top-nav {
			padding: 0 10px;
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

		.page-controls {
			gap: 4px;
		}

		.progress-bar {
			height: 10px;
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

		.page-image {
			width: 100%;
			height: auto;
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

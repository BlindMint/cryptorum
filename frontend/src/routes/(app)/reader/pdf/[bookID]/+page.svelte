<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { browser } from '$app/environment';
	import { readerSettings, pdfZoomModes } from '$lib/stores/readerSettings';
	import type { PdfReaderSetting, PdfViewMode } from '$lib/stores/readerSettings';
	import { normalizeBookFormat } from '$lib/utils/book-formats';
	import { toggleReaderFullscreen } from '$lib/utils/fullscreen';
	import { isBottomSystemGestureStart } from '$lib/utils/system-gesture-guard';
	import EmbedPDFViewer from '$lib/components/EmbedPDFViewer.svelte';
	import ReaderProgressTrack from '$lib/components/ReaderProgressTrack.svelte';

	let book = $state<any>(null);
	let loading = $state(true);
	let error = $state('');
	let pdfDoc: any = null;
	let pdfLoadingTask: any = null;
	let currentPage = $state(1);
	let numPages = $state(0);
	let scale = $state(1);
	let pdfInstance: any = null;
	let PdfTextLayer: any = null;
	let textLayerSelectionListenersBound = false;
	let canvas: HTMLCanvasElement | undefined = undefined;
	let ctx: CanvasRenderingContext2D | null = null;
	let readerInitialized = false;
	let pdfReady = $state(false);
	let savedProgress = $state<any>(null);
	let pdfOutline = $state<any[]>([]);
	let expandedItems = $state<Set<string>>(new Set());
	let currentSessionId = $state<number | null>(null);
	let coverPreviewFailed = $state(false);
	let requestedFormat = $state('pdf');
	let embedPdfProgressReady = $state(false);
	let embedPdfInitialPage = $state(1);
	let embedPdfViewerReady = $state(false);
	let embedPdfScroll: any = null;
	let embedPdfRestoringInitialPage = false;
	let embedPdfSidebarOpen = $state(false);
	let pdfLoadRetryToken = $state(0);
	let pdfLoadRetryAttempts = 0;

	let settings = $state<PdfReaderSetting>({
		pageSpread: 'off',
		pageLayout: 'single',
		pageZoom: 'auto',
		zoomLevel: 100,
		renderQuality: 'high',
		autoHideControls: true,
		showSidebar: false,
		scrollDirection: 'vertical',
		scrollMode: 'continuous-vertical',
		pageRotation: 0,
		backgroundColor: '#111111',
		brightness: 100,
		contrast: 100,
		grayscale: 0,
		readingDirection: 'ltr',
		autoCropMargins: false,
		textLayerEnabled: true,
		annotationsEnabled: true,
		viewMode: 'dark',
		showChapterMarkers: false,
		showQuoteMarks: false,
		panMode: false,
		useStandardFullscreen: false,
		keepScreenOn: true
	});

	type SidebarTab = 'thumbnails' | 'bookmarks' | 'search';
	let leftSidebarOpen = $state(false);
	let rightSidebarOpen = $state(false);
	let activeSidebarTab = $state<SidebarTab>('thumbnails');
	let searchQuery = $state('');
	type SearchResult = {
		id: number;
		page: number;
		index: number;
		before: string;
		match: string;
		after: string;
	};
	let searchResults = $state<SearchResult[]>([]);
	let currentSearchResult = $state(0);
	let matchCase = $state(false);
	let isSearching = $state(false);
	let searchTimer: ReturnType<typeof setTimeout> | null = null;
	let searchRunId = 0;
	let searchFlashTimer: ReturnType<typeof setTimeout> | null = null;

	type ContinuousPageLayout = {
		top: number;
		width: number;
		height: number;
	};

	type ContinuousScrollSnapshot = {
		page: number;
		offsetRatio: number;
	};

	type PinchZoomAnchor = {
		surfaceXRatio: number;
		surfaceYRatio: number;
		viewportX: number;
		viewportY: number;
	};

	let pageCanvases: Map<number, HTMLCanvasElement> = new Map();
	let thumbnailCanvases: Map<number, HTMLCanvasElement> = new Map();
	let pageViewports: Map<number, any> = new Map();
	let continuousPageLayouts: Map<number, ContinuousPageLayout> = new Map();
	let continuousTotalHeight = $state(0);
	let renderedPages: Set<number> = new Set();
	let renderingPages: Set<number> = new Set();
	let renderedThumbnails: Set<number> = new Set();
	let renderTasks: Map<HTMLCanvasElement, any> = new Map();
	let continuousContainer: HTMLDivElement | undefined = undefined;
	let scrollbar: HTMLDivElement | undefined = undefined;
	let isDragging = $state(false);
	let dragStart = $state({ x: 0, y: 0 });
	let scrollStart = $state({ x: 0, y: 0 });
	let activePanScrollContainer: HTMLElement | null = null;
	let pageInputValue = $state('');
	let isEditingPage = $state(false);
	let lastWheelNavigationAt = 0;
	let touchPinchActive = false;
	let touchPinchStartDistance = 0;
	let touchPinchStartZoom = 0;
	let touchPinchLatestZoom: number | null = null;
	let touchPinchTargetEl: HTMLElement | null = null;
	let touchPinchAnchor: PinchZoomAnchor | null = null;
	let renderedZoomLevel = 100;
	let visualZoomLevel = 100;
	let visualZoomActive = false;
	let fitWidthSnapshot: { pageZoom: PdfReaderSetting['pageZoom']; zoomLevel: number } | null = null;
	let fitWidthActive = $state(false);
	let viewportResizeTimeout: ReturnType<typeof setTimeout> | null = null;
	let topBarHideTimeout: ReturnType<typeof setTimeout> | null = null;
	let topBarVisible = $state(false);
	let lastContinuousScrollTop = 0;
	let preserveChromeAfterSeekUntil = 0;
	let sessionEnded = false;
	let handlePageExit: (() => void) | null = null;
	let pdfReaderEl = $state<HTMLDivElement | null>(null);
	let pdfContainerEl = $state<HTMLDivElement | null>(null);
	let isRestoringProgress = false;
	let progressSaveTimer: ReturnType<typeof setTimeout> | null = null;
	let lastSavedPage = 0;
	let closeTasksStarted = false;
	let readerClosing = false;
	let readerPointerStart: { id: number; x: number; y: number } | null = null;
	let readerPointerMoved = false;
	let backgroundWarmupTimer: ReturnType<typeof setTimeout> | null = null;
	let backgroundWarmupRunId = 0;

	const progress = $derived(numPages > 0 ? (currentPage / numPages) * 100 : 0);
	const pageTotalLabel = $derived(numPages > 0 ? String(numPages) : '...');
	const pageNavigationReady = $derived(numPages > 0 && !readerClosing);
	const readerChromeReady = $derived(embedPdfViewerReady && !loading && !error);

	const renderQualityScale: Record<PdfReaderSetting['renderQuality'], number> = {
		standard: 1,
		high: 1.5,
		maximum: 2
	};
	const topBarHideDelayMs = 2800;
	const scrollHideThresholdPx = 8;
	const seekScrollSuppressMs = 1400;
	const progressSaveDebounceMs = 750;
	const continuousPageGapPx = 16;
	const continuousOverscanPx = 2200;
	const mobileContinuousOverscanPx = 900;
	const desktopPdfZoomHeadroomScale = 1.5;
	const mobilePdfZoomHeadroomScale = 1.2;
	const desktopMaxPdfCanvasPixels = 12_000_000;
	const mobileMaxPdfCanvasPixels = 4_000_000;
	const desktopMaxPdfCanvasDimension = 6144;
	const mobileMaxPdfCanvasDimension = 4096;
	const embedPdfDocumentId = 'cryptorum-pdf';

	const viewModeBgColors: Record<PdfViewMode, string> = {
		light: '#ffffff',
		dark: '#1a1a1a',
		trueDark: '#000000'
	};

	const viewModeTextColors: Record<PdfViewMode, string> = {
		light: '#333333',
		dark: '#e5e7eb',
		trueDark: '#ffffff'
	};

	function getPdfFileSignature() {
		return String(
			book?.file_hash ??
			book?.hash ??
			book?.last_modified ??
			book?.last_scanned ??
			book?.page_count ??
			'current'
		);
	}

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

	function clearBackgroundWarmup() {
		backgroundWarmupRunId++;
		if (backgroundWarmupTimer) {
			clearTimeout(backgroundWarmupTimer);
			backgroundWarmupTimer = null;
		}
	}

	function prewarmPdfPages(centerPage: number, radius = 1) {
		if (!pdfDoc || readerClosing) return;

		for (let offset = 1; offset <= radius; offset++) {
			[centerPage - offset, centerPage + offset].forEach((pageNum) => {
				if (pageNum >= 1 && pageNum <= numPages) {
					void pdfDoc.getPage(pageNum).catch(() => undefined);
				}
			});
		}
	}

	function scheduleContinuousBackgroundWarmup(centerPage: number) {
		if (settings.scrollMode !== 'continuous-vertical' || !pdfReady || readerClosing) return;

		clearBackgroundWarmup();
		const runId = backgroundWarmupRunId;
		backgroundWarmupTimer = setTimeout(() => {
			backgroundWarmupTimer = null;
			if (runId !== backgroundWarmupRunId || readerClosing || currentPage !== centerPage) return;

			void (async () => {
				for (const pageNum of [centerPage - 1, centerPage + 1]) {
					if (
						runId !== backgroundWarmupRunId ||
						readerClosing ||
						currentPage !== centerPage ||
						pageNum < 1 ||
						pageNum > numPages
					) {
						return;
					}
					await renderContinuousPage(pageNum);
				}
			})();
		}, 900);
	}

	function prewarmVisiblePdfWindow(centerPage: number) {
		prewarmPdfPages(centerPage, 2);
		if (settings.scrollMode === 'continuous-vertical') {
			scheduleContinuousBackgroundWarmup(centerPage);
		}
	}

	function destroyPdfLoading() {
		readerClosing = true;
		clearBackgroundWarmup();
		cleanupContinuousRendering();
		if (pdfLoadingTask?.destroy) {
			void pdfLoadingTask.destroy().catch(() => undefined);
		}
		if (pdfDoc?.destroy) {
			void pdfDoc.destroy().catch(() => undefined);
		}
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

	onMount(() => {
		if (!browser) return;

		let mounted = true;
		let unsubscribeReaderSettings: (() => void) | null = null;
		const globalMouseDownListener = (event: MouseEvent) => handleMouseDown(event);
		const globalClickListener = (event: MouseEvent) => {
			const target = event.target as HTMLElement | null;
			const container = document.getElementById('pdf-container');
			if (!target || !container) return;

			if (container.contains(target) && (leftSidebarOpen || rightSidebarOpen)) {
				leftSidebarOpen = false;
				rightSidebarOpen = false;
				resetTopBarBehavior();
			}
		};
		window.addEventListener('mousedown', globalMouseDownListener);
		window.addEventListener('click', globalClickListener);

		unsubscribeReaderSettings = readerSettings.subscribe(s => {
			settings = { ...s.pdf };
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
				if (closeTasksStarted) return;
				void saveProgress(true);
				void endSession(true);
			};
			window.addEventListener('pagehide', handlePageExit);
			window.addEventListener('beforeunload', handlePageExit);
		})();

		return () => {
			mounted = false;
			unsubscribeReaderSettings?.();
			window.removeEventListener('mousedown', globalMouseDownListener);
			window.removeEventListener('click', globalClickListener);
			if (handlePageExit) {
				window.removeEventListener('pagehide', handlePageExit);
				window.removeEventListener('beforeunload', handlePageExit);
			}
		};
	});

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

	onDestroy(() => {
		clearTopBarHideTimer();
		clearProgressSaveTimer();
		clearSearchTimer();
		clearSearchFlashTimer();
		destroyPdfLoading();
		if (handlePageExit) {
			window.removeEventListener('pagehide', handlePageExit);
			window.removeEventListener('beforeunload', handlePageExit);
		}
		if (closeTasksStarted) return;
		void saveProgress(true);
		void endSession(true);
	});

	async function updateZoomFromMode() {
		if (!pdfDoc) return;

		const container = document.getElementById('pdf-container');
		if (!container) return;

		const containerWidth = container.clientWidth - 32;
		const containerHeight = container.clientHeight - 32;
		const isMobileViewport = window.matchMedia('(max-width: 768px)').matches;

		const page = await pdfDoc.getPage(currentPage || 1);
		const baseViewport = page.getViewport({ scale: 1 });

		let newZoomLevel = settings.zoomLevel;

		switch (settings.pageZoom) {
			case 'page-fit':
				newZoomLevel = Math.min(
					(containerWidth / baseViewport.width) * 100,
					(containerHeight / baseViewport.height) * 100
				);
				break;
			case 'page-width':
				newZoomLevel = (containerWidth * 0.95 / baseViewport.width) * 100;
				break;
			case 'actual-size':
				newZoomLevel = 100;
				break;
			case 'auto':
			default:
				newZoomLevel = isMobileViewport
					? (containerWidth / baseViewport.width) * 100
					: Math.min(
							(containerWidth / baseViewport.width) * 100,
							(containerHeight / baseViewport.height) * 100,
							200
						);
				break;
		}

		if (Math.abs(settings.zoomLevel - newZoomLevel) > 1) {
			settings = { ...settings, zoomLevel: Math.round(newZoomLevel) };
			readerSettings.updatePdf({ zoomLevel: settings.zoomLevel });
		}
	}

	function getContinuousScrollSnapshot(): ContinuousScrollSnapshot | null {
		const scrollbarEl = scrollbar || document.getElementById('continuous-scrollbar') as HTMLDivElement | null;
		const layout = continuousPageLayouts.get(currentPage);
		if (!scrollbarEl || !layout) return null;

		return {
			page: currentPage,
			offsetRatio: Math.max(0, Math.min(1, (scrollbarEl.scrollTop - layout.top) / Math.max(1, layout.height)))
		};
	}

	function getPdfScrollElement() {
		if (settings.scrollMode === 'continuous-vertical') {
			return document.getElementById('continuous-scrollbar') as HTMLElement | null;
		}
		return pdfContainerEl;
	}

	function restorePinchScrollAnchor(anchor: PinchZoomAnchor | null) {
		if (!anchor) return;

		const surface = getPdfZoomSurface();
		const scroller = getPdfScrollElement();
		if (!surface || !scroller) return;

		scroller.scrollTo({
			left: surface.offsetLeft + surface.offsetWidth * anchor.surfaceXRatio - anchor.viewportX,
			top: surface.offsetTop + surface.offsetHeight * anchor.surfaceYRatio - anchor.viewportY,
			behavior: 'auto'
		});
		lastContinuousScrollTop = scroller.scrollTop;
	}

	async function settlePinchScrollAnchor(anchor: PinchZoomAnchor | null) {
		if (!anchor) return;

		resetPinchTransform();
		await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		restorePinchScrollAnchor(anchor);
	}

	async function updateSetting(key: string, value: any, pinchAnchor: PinchZoomAnchor | null = null) {
		const shouldPreserveContinuousScroll =
			settings.scrollMode === 'continuous-vertical' &&
			['pageZoom', 'zoomLevel', 'renderQuality', 'pageRotation'].includes(key);
		const continuousScrollSnapshot = shouldPreserveContinuousScroll
			? getContinuousScrollSnapshot()
			: null;

		settings = { ...settings, [key]: value };
		readerSettings.updatePdf({ [key]: value });
		showTopBar(key === 'scrollMode' || key === 'autoHideControls');

		if (key === 'scrollMode') {
			prepareForPdfRerender();
			const pageBeforeSwitch = currentPage;
			if (value === 'continuous-vertical') {
				await tick();
				renderAllPagesContinuous();
				await tick();
				scrollToPage(pageBeforeSwitch);
			} else {
				cleanupContinuousRendering();
				await tick();
				renderPage(pageBeforeSwitch);
			}
		} else if (key === 'pageZoom') {
			prepareForPdfRerender();
			await updateZoomFromMode();
			if (settings.scrollMode === 'continuous-vertical') {
				await tick();
				await renderAllPagesContinuous(undefined, continuousScrollSnapshot);
				await tick();
				await settlePinchScrollAnchor(pinchAnchor);
			} else {
				await tick();
				await renderPage(currentPage);
				await tick();
				await settlePinchScrollAnchor(pinchAnchor);
			}
		} else if (key === 'zoomLevel') {
			prepareForPdfRerender();
			if (settings.scrollMode === 'continuous-vertical') {
				await tick();
				await renderAllPagesContinuous(undefined, continuousScrollSnapshot);
				await tick();
				await settlePinchScrollAnchor(pinchAnchor);
			} else {
				await tick();
				await renderPage(currentPage);
				await tick();
				await settlePinchScrollAnchor(pinchAnchor);
			}
		} else if (key === 'renderQuality') {
			prepareForPdfRerender();
			if (settings.scrollMode === 'continuous-vertical') {
				await tick();
				await renderAllPagesContinuous(undefined, continuousScrollSnapshot);
			} else {
				await tick();
				renderPage(currentPage);
			}
		} else if (key === 'pageRotation') {
			prepareForPdfRerender();
			if (settings.scrollMode === 'continuous-vertical') {
				await tick();
				await renderAllPagesContinuous(undefined, continuousScrollSnapshot);
			} else {
				renderPage(currentPage);
			}
		} else if (['brightness', 'contrast', 'grayscale'].includes(key)) {
			applyVisualFilters();
		} else if (key === 'pageSpread' || key === 'pageLayout') {
			prepareForPdfRerender();
			if (settings.scrollMode === 'continuous-vertical') {
				renderAllPagesContinuous();
			} else {
				await tick();
				renderPage(currentPage);
			}
		} else if (key === 'viewMode') {
			applyViewMode();
		}
		resetTopBarBehavior();
	}

	function applyViewMode() {
		const container = document.getElementById('pdf-container');
		if (container) {
			container.style.backgroundColor = viewModeBgColors[settings.viewMode];
			if (settings.viewMode === 'trueDark') {
				container.style.filter = 'invert(1)';
			} else {
				container.style.filter = '';
			}
		}
	}

	function applyVisualFilters() {
		const container = document.getElementById('pdf-container');
		if (container) {
			const filter = `brightness(${settings.brightness}%) contrast(${settings.contrast}%) grayscale(${settings.grayscale}%)`;
			container.style.filter = settings.viewMode === 'trueDark' ? `invert(1) ${filter}` : filter;
		}
	}

	function getRenderScaleFactor() {
		const deviceScale = Math.max(window.devicePixelRatio || 1, 1);
		return deviceScale * renderQualityScale[settings.renderQuality];
	}

	function isMobileOrTouchViewport() {
		if (!browser) return false;
		return window.matchMedia('(max-width: 768px)').matches || navigator.maxTouchPoints > 0;
	}

	function getPdfPageOutputScale(width: number, height: number) {
		const baseScale = getRenderScaleFactor();
		const isMobileOrTouch = isMobileOrTouchViewport();
		const headroomScale = isMobileOrTouch ? mobilePdfZoomHeadroomScale : desktopPdfZoomHeadroomScale;
		const maxPixels = isMobileOrTouch ? mobileMaxPdfCanvasPixels : desktopMaxPdfCanvasPixels;
		const maxDimension = isMobileOrTouch ? mobileMaxPdfCanvasDimension : desktopMaxPdfCanvasDimension;
		const desiredScale = baseScale * headroomScale;
		const dimensionLimit = maxDimension / Math.max(width, height, 1);
		const pixelLimit = Math.sqrt(maxPixels / Math.max(width * height, 1));

		return Math.max(1, Math.min(desiredScale, dimensionLimit, pixelLimit));
	}

	function getContinuousOverscanPx() {
		return isMobileOrTouchViewport() ? mobileContinuousOverscanPx : continuousOverscanPx;
	}

	function controlsNeedToStayVisible() {
		if (!readerChromeReady) return false;

		return (
			!settings.autoHideControls ||
			leftSidebarOpen ||
			rightSidebarOpen ||
			embedPdfSidebarOpen ||
			isEditingPage ||
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

	function clearSearchTimer() {
		if (searchTimer) {
			clearTimeout(searchTimer);
			searchTimer = null;
		}
	}

	function clearSearchFlashTimer() {
		if (searchFlashTimer) {
			clearTimeout(searchFlashTimer);
			searchFlashTimer = null;
		}
	}

	function queueProgressSave() {
		if (!book || isRestoringProgress || currentPage === lastSavedPage) return;
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
		if (!readerChromeReady) return;
		if (controlsNeedToStayVisible()) return;
		if (topBarVisible) {
			hideTopBar();
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

	function isInteractiveReaderTarget(target: EventTarget | null) {
		if (!(target instanceof Element)) return false;
		return !!target.closest(
			'button, a, input, textarea, select, label, [contenteditable="true"], .top-nav, .left-sidebar, .right-sidebar, .floating-nav, .progress-bar, .reader-progress, .embedpdf-shell-control'
		);
	}

	function applyPanMode() {
		if (!pdfContainerEl) return;

		pdfContainerEl.style.cursor = settings.panMode ? 'grab' : '';
		pdfContainerEl.style.userSelect = settings.panMode ? 'none' : '';
		(pdfContainerEl.style as CSSStyleDeclaration & { webkitUserSelect?: string }).webkitUserSelect =
			settings.panMode ? 'none' : '';

		pdfContainerEl.querySelectorAll('.text-layer').forEach((layer) => {
			const textLayer = layer as HTMLDivElement;
			textLayer.style.pointerEvents = settings.panMode ? 'none' : 'auto';
			textLayer.style.userSelect = settings.panMode ? 'none' : 'text';
			(textLayer.style as CSSStyleDeclaration & { webkitUserSelect?: string }).webkitUserSelect =
				settings.panMode ? 'none' : 'text';
		});
	}

	function getPanScrollContainer(target: EventTarget | null) {
		if (!(target instanceof Element)) return document.getElementById('pdf-container');
		if (settings.scrollMode === 'continuous-vertical') {
			return target.closest('#continuous-scrollbar') as HTMLElement | null;
		}
		return document.getElementById('pdf-container');
	}

	function getTouchDistance(touches: TouchList) {
		if (touches.length < 2) return 0;
		const dx = touches[0].clientX - touches[1].clientX;
		const dy = touches[0].clientY - touches[1].clientY;
		return Math.hypot(dx, dy);
	}

	function getTouchCenter(touches: TouchList) {
		return {
			x: (touches[0].clientX + touches[1].clientX) / 2,
			y: (touches[0].clientY + touches[1].clientY) / 2
		};
	}

	function clampZoomLevel(zoom: number) {
		return Math.max(25, Math.min(400, Math.round(zoom)));
	}

	function setDocumentZoom(nextZoom: number) {
		void updateSetting('zoomLevel', clampZoomLevel(nextZoom));
	}

	function isPdfDocumentTarget(target: EventTarget | null) {
		if (!(target instanceof Element) || !pdfContainerEl) return false;
		return pdfContainerEl.contains(target) && !isInteractiveReaderTarget(target);
	}

	function isPdfDocumentKeyboardTarget(target: EventTarget | null) {
		if (!(target instanceof Element)) return target === document;
		if (isInteractiveReaderTarget(target)) return false;
		return target === document.body || isPdfDocumentTarget(target);
	}

	function getPdfZoomSurface() {
		if (settings.scrollMode === 'continuous-vertical') {
			return document.getElementById('continuous-container') as HTMLElement | null;
		}
		return document.getElementById('paged-viewer') as HTMLElement | null;
	}

	function resetPinchTransform() {
		if (!touchPinchTargetEl) return;
		touchPinchTargetEl.style.transform = '';
		touchPinchTargetEl.style.transformOrigin = '';
		touchPinchTargetEl.style.willChange = '';
		touchPinchTargetEl = null;
	}

	function clearVisualZoomStyles() {
		const surface = getPdfZoomSurface();
		if (surface) {
			surface.style.transform = '';
			surface.style.transformOrigin = '';
			surface.style.willChange = '';
		}

		pageCanvases.forEach((pageCanvas) => {
			const textLayer = pageCanvas.parentElement?.querySelector('.text-layer') as HTMLDivElement | null;
			if (textLayer) {
				textLayer.style.transform = '';
				textLayer.style.transformOrigin = '';
			}
		});
	}

	function prepareForPdfRerender() {
		visualZoomActive = false;
		visualZoomLevel = settings.zoomLevel;
		clearVisualZoomStyles();
	}

	function getPdfRenderZoomLevel() {
		return visualZoomActive ? renderedZoomLevel : settings.zoomLevel;
	}

	function markPdfRenderedAtCurrentZoom() {
		renderedZoomLevel = settings.zoomLevel;
		visualZoomLevel = settings.zoomLevel;
		visualZoomActive = false;
		clearVisualZoomStyles();
	}

	function getVisualZoomRatio() {
		return visualZoomActive ? visualZoomLevel / Math.max(1, renderedZoomLevel) : 1;
	}

	function getVisualPageSize(viewport: any) {
		const ratio = getVisualZoomRatio();
		return {
			width: Math.max(1, Math.floor(viewport.width * ratio)),
			height: Math.max(1, Math.floor(viewport.height * ratio))
		};
	}

	function setContinuousCanvasVisualSize(pageCanvas: HTMLCanvasElement, width: number, height: number) {
		pageCanvas.style.width = `${width}px`;
		pageCanvas.style.height = `${height}px`;

		const textLayer = pageCanvas.parentElement?.querySelector('.text-layer') as HTMLDivElement | null;
		if (textLayer) {
			const ratio = getVisualZoomRatio();
			textLayer.style.transformOrigin = '0 0';
			textLayer.style.transform = Math.abs(ratio - 1) < 0.001 ? '' : `scale(${ratio})`;
		}
	}

	function applyVisualZoomLayout(zoomLevel: number) {
		const previousZoomLevel = Math.max(1, visualZoomLevel);
		visualZoomLevel = zoomLevel;
		visualZoomActive = Math.abs(zoomLevel - renderedZoomLevel) >= 1;
		const ratio = zoomLevel / previousZoomLevel;

		if (settings.scrollMode === 'continuous-vertical') {
			continuousPageLayouts.forEach((layout, pageNum) => {
				continuousPageLayouts.set(pageNum, {
					top: layout.top * ratio,
					width: layout.width * ratio,
					height: layout.height * ratio
				});
			});
			recomputeContinuousOffsets(1);

			pageCanvases.forEach((pageCanvas, pageNum) => {
				const layout = continuousPageLayouts.get(pageNum);
				if (!layout) return;
				setContinuousCanvasVisualSize(
					pageCanvas,
					Math.floor(layout.width),
					Math.floor(layout.height)
				);
			});
			return;
		}

		const surface = touchPinchTargetEl || getPdfZoomSurface();
		if (!surface) return;
		const visualScale = zoomLevel / Math.max(1, renderedZoomLevel);
		surface.style.transform = Math.abs(visualScale - 1) < 0.001 ? '' : `scale(${visualScale})`;
		surface.style.willChange = 'transform';
	}

	function commitTouchZoomWithoutRender(zoomLevel: number) {
		settings = { ...settings, zoomLevel };
		readerSettings.updatePdf({ zoomLevel });
		applyVisualZoomLayout(zoomLevel);
		resetTopBarBehavior();
	}

	function getPinchZoomAnchor(midpoint: { x: number; y: number }): PinchZoomAnchor | null {
		if (!touchPinchTargetEl) return null;

		const scroller = getPdfScrollElement();
		if (!scroller) return null;

		const surfaceRect = touchPinchTargetEl.getBoundingClientRect();
		const scrollerRect = scroller.getBoundingClientRect();
		if (surfaceRect.width <= 0 || surfaceRect.height <= 0) return null;

		return {
			surfaceXRatio: Math.max(0, Math.min(1, (midpoint.x - surfaceRect.left) / surfaceRect.width)),
			surfaceYRatio: Math.max(0, Math.min(1, (midpoint.y - surfaceRect.top) / surfaceRect.height)),
			viewportX: midpoint.x - scrollerRect.left,
			viewportY: midpoint.y - scrollerRect.top
		};
	}

	function handleTouchStart(e: TouchEvent) {
		if (!pdfContainerEl || e.touches.length !== 2) return;
		if (!isPdfDocumentTarget(e.target)) return;
		e.preventDefault();

		showTopBar(true);
		touchPinchStartDistance = getTouchDistance(e.touches);
		touchPinchStartZoom = settings.zoomLevel;
		touchPinchLatestZoom = settings.zoomLevel;
		touchPinchTargetEl = getPdfZoomSurface();
		if (!touchPinchTargetEl) return;
		touchPinchActive = true;

		const midpoint = getTouchCenter(e.touches);
		touchPinchAnchor = getPinchZoomAnchor(midpoint);
		const rect = touchPinchTargetEl.getBoundingClientRect();
		touchPinchTargetEl.style.transformOrigin = `${midpoint.x - rect.left}px ${midpoint.y - rect.top}px`;
		touchPinchTargetEl.style.willChange = 'transform';
	}

	function handleTouchMove(e: TouchEvent) {
		if (!touchPinchActive || !touchPinchTargetEl || e.touches.length !== 2) return;
		e.preventDefault();

		const distance = getTouchDistance(e.touches);
		if (touchPinchStartDistance <= 0 || distance <= 0) return;

		const nextZoom = clampZoomLevel((touchPinchStartZoom * distance) / touchPinchStartDistance);
		touchPinchLatestZoom = nextZoom;
		applyVisualZoomLayout(nextZoom);
		restorePinchScrollAnchor(touchPinchAnchor);
	}

	function handleTouchEnd() {
		if (!touchPinchActive) return;
		touchPinchActive = false;
		const finalZoom = touchPinchLatestZoom === null ? settings.zoomLevel : Math.round(touchPinchLatestZoom);
		const pinchAnchor = touchPinchAnchor;
		touchPinchLatestZoom = null;
		touchPinchAnchor = null;

		commitTouchZoomWithoutRender(finalZoom);
		restorePinchScrollAnchor(pinchAnchor);
		if (touchPinchTargetEl) {
			touchPinchTargetEl.style.willChange = '';
		}
		touchPinchTargetEl = null;
	}

	function handleViewportResize() {
		if (viewportResizeTimeout) {
			clearTimeout(viewportResizeTimeout);
		}

		if (!pdfDoc) return;
		viewportResizeTimeout = setTimeout(() => {
			if (!pdfReady || !pdfDoc || settings.pageZoom === 'actual-size') return;

			void updateZoomFromMode().then(() => {
				prepareForPdfRerender();
				if (settings.scrollMode === 'continuous-vertical') {
					renderAllPagesContinuous();
				} else {
					renderPage(currentPage);
				}
			});
		}, 120);
	}

	function queuePageBeforePdfReady(pageNum: number) {
		currentPage = pageNum;
	}

	function navigateToPage(pageNum: number, behavior: ScrollBehavior = 'smooth', save = true) {
		if (pageNum < 1 || pageNum > numPages) return;
		currentPage = pageNum;

		embedPdfRestoringInitialPage = false;
		embedPdfInitialPage = pageNum;
		try {
			embedPdfScroll?.forDocument?.(embedPdfDocumentId)?.scrollToPage?.({
				pageNumber: pageNum,
				behavior,
				alignY: 0
			});
		} catch (e) {
			console.warn('Failed to navigate EmbedPDF page:', e);
		}
		if (save) {
			void saveProgress();
		}
		return;

		if (!pdfDoc || !pdfReady) {
			queuePageBeforePdfReady(pageNum);
			return;
		}

		if (settings.scrollMode === 'continuous-vertical') {
			void scrollToPage(pageNum, behavior);
		} else {
			void renderPage(pageNum);
		}
		if (save) {
			void saveProgress();
		}
	}

	function prevPage() {
		showTopBar(true);
		if (settings.readingDirection === 'rtl') {
			if (currentPage < numPages) {
				navigateToPage(currentPage + 1);
			}
		} else {
			if (currentPage > 1) {
				navigateToPage(currentPage - 1);
			}
		}
	}

	function nextPage() {
		showTopBar(true);
		if (settings.readingDirection === 'rtl') {
			if (currentPage > 1) {
				navigateToPage(currentPage - 1);
			}
		} else {
			if (currentPage < numPages) {
				navigateToPage(currentPage + 1);
			}
		}
	}

	function goToPage(pageNum: number) {
		if (pageNum >= 1 && pageNum <= numPages) {
			showTopBar(settings.scrollMode === 'paged');
			navigateToPage(pageNum);
		}
	}

	function goToOutlineItem(item: any) {
		if (item.dest) {
			const dest = typeof item.dest === 'string' ? item.dest : item.dest[0];
			if (typeof dest === 'object' && dest !== null) {
				pdfDoc.getPageIndex(dest).then((pageIndex: number) => {
					goToPage(pageIndex + 1);
				});
			} else if (typeof dest === 'string') {
				pdfDoc.getDestination(dest).then(async (foundDest: any) => {
					if (foundDest) {
						const pageIndex = await pdfDoc.getPageIndex(foundDest[0]);
						goToPage(pageIndex + 1);
					}
				});
			}
		} else if (item.url) {
			window.open(item.url, '_blank');
		}
	}

	function toggleOutlineItem(itemId: string) {
		if (expandedItems.has(itemId)) {
			expandedItems.delete(itemId);
		} else {
			expandedItems.add(itemId);
		}
		expandedItems = new Set(expandedItems);
	}

	function hasChildren(item: any): boolean {
		return item.items && item.items.length > 0;
	}

	function getItemId(item: any, index: number): string {
		return `outline-${index}-${item.title}`;
	}

	function startEditPage() {
		showTopBar(false);
		isEditingPage = true;
		pageInputValue = String(currentPage);
	}

	function finishEditPage() {
		isEditingPage = false;
		const pageNum = parseInt(pageInputValue);
		if (!isNaN(pageNum)) {
			goToPage(pageNum);
		}
	}

	function handlePageInputKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			finishEditPage();
		} else if (e.key === 'Escape') {
			isEditingPage = false;
		}
	}

	async function scrollToPage(pageNum: number, behavior: ScrollBehavior = 'smooth') {
		if (settings.scrollMode === 'continuous-vertical') {
			const layout = continuousPageLayouts.get(pageNum);
			const scrollbarEl = scrollbar || document.getElementById('continuous-scrollbar') as HTMLDivElement | null;
			if (layout && scrollbarEl) {
				scrollbarEl.scrollTo({ top: layout.top, behavior });
				await renderVisibleContinuousPages(scrollbarEl, true);
				await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
				lastContinuousScrollTop = scrollbarEl.scrollTop;
				return;
			}
		}

		const canvas = pageCanvases.get(pageNum);
		if (canvas) {
			canvas.scrollIntoView({ behavior, block: 'start' });
			await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
			lastContinuousScrollTop = scrollbar?.scrollTop ?? lastContinuousScrollTop;
		}
	}

	function handleContinuousScroll(e: Event) {
		if (settings.scrollMode !== 'continuous-vertical') return;

		const target = e.currentTarget as HTMLElement;
		const scrollTop = target.scrollTop;
		const delta = scrollTop - lastContinuousScrollTop;
		lastContinuousScrollTop = scrollTop;
		void renderVisibleContinuousPages(target);

		if (performance.now() < preserveChromeAfterSeekUntil) {
			showTopBar(false);
			return;
		}

		const touchLike = typeof window !== 'undefined' && window.matchMedia('(hover: none), (pointer: coarse)').matches;
		if (controlsNeedToStayVisible() || (scrollTop <= 0 && !touchLike)) {
			showTopBar(false);
			return;
		}

		if (delta > scrollHideThresholdPx) {
			hideTopBar();
		} else if (delta < -scrollHideThresholdPx && !touchLike) {
			showTopBar(false);
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

	function isReaderCenterTap(e: PointerEvent, surface: HTMLElement) {
		const rect = surface.getBoundingClientRect();
		if (rect.width <= 0 || rect.height <= 0) return false;

		const xRatio = (e.clientX - rect.left) / rect.width;
		const yRatio = (e.clientY - rect.top) / rect.height;

		return xRatio > 0.28 && xRatio < 0.72 && yRatio > 0.12 && yRatio < 0.88;
	}

	function isTouchLikePointer(e?: PointerEvent | MouseEvent) {
		if (e && 'pointerType' in e && e.pointerType === 'touch') return true;
		return typeof window !== 'undefined' && window.matchMedia('(hover: none), (pointer: coarse)').matches;
	}

	function handleReaderPointerDown(e: PointerEvent) {
		if (!e.isPrimary) return;
		readerPointerStart = { id: e.pointerId, x: e.clientX, y: e.clientY };
		readerPointerMoved = false;
	}

	function handlePdfContainerPointerUp(e: PointerEvent) {
		const interactionSurface = pdfReaderEl;
		if (!interactionSurface || !interactionSurface.contains(e.target as Node)) return;
		if (settings.panMode || isInteractiveReaderTarget(e.target)) return;
		if (readerPointerStart && readerPointerStart.id !== e.pointerId) return;
		if (readerPointerMoved) return;
		if (window.getSelection()?.toString()) return;

		if (isReaderCenterTap(e, interactionSurface)) {
			toggleTopBarFromCenterTap();
		}

		readerPointerStart = null;
	}

	function handleReaderPointerMove(e: MouseEvent) {
		if (readerPointerStart) {
			const dx = e.clientX - readerPointerStart.x;
			const dy = e.clientY - readerPointerStart.y;
			if (Math.hypot(dx, dy) > 10) {
				readerPointerMoved = true;
			}
		}

		if (!settings.autoHideControls || isTouchLikePointer(e)) return;
		if (e.clientY <= 72) {
			showTopBar(true);
		} else if (pdfReaderEl?.contains(e.target as Node)) {
			scheduleTopBarAutoHide();
		}
	}

	async function renderPage(pageNum: number, page?: any) {
		if (!pdfDoc) return;
		if (visualZoomActive) {
			prepareForPdfRerender();
		}

		const isRTL = settings.readingDirection === 'rtl';
		const isDouble = settings.pageLayout === 'double';

		if (isDouble) {
			const leftCanvas = document.getElementById('pdf-canvas-left') as HTMLCanvasElement;
			const rightCanvas = document.getElementById('pdf-canvas-right') as HTMLCanvasElement;
			if (!leftCanvas || !rightCanvas) return;

			let leftPage = pageNum;
			let rightPage = pageNum + 1;

			if (isRTL) {
				[leftPage, rightPage] = [rightPage, leftPage];
			}

			await Promise.all([
				renderSinglePage(leftCanvas, leftPage),
				rightPage <= numPages ? renderSinglePage(rightCanvas, rightPage) : Promise.resolve()
			]);
		} else {
			canvas = document.getElementById('pdf-canvas') as HTMLCanvasElement;
			if (!canvas) return;

			await renderSinglePage(canvas, pageNum, page);
		}
		markPdfRenderedAtCurrentZoom();
		prewarmPdfPages(pageNum, 1);
	}

	async function renderSinglePage(canvas: HTMLCanvasElement, pageNum: number, page?: any) {
		if (!pdfDoc || readerClosing || pageNum < 1 || pageNum > numPages) return;

		const existingTask = renderTasks.get(canvas);
		if (existingTask) {
			existingTask.cancel();
		}

		let task: any = null;
		try {
			const pageToRender = page ?? await pdfDoc.getPage(pageNum);
			if (!pageToRender || readerClosing) return;

			const displayScale = getPdfRenderZoomLevel() / 100;
			const cssViewport = pageToRender.getViewport({
				scale: displayScale,
				rotation: settings.pageRotation
			});
			const outputScale = getPdfPageOutputScale(cssViewport.width, cssViewport.height);
			const transform = outputScale !== 1 ? [outputScale, 0, 0, outputScale, 0, 0] : null;

			canvas.dataset.pageNumber = String(pageNum);
			canvas.style.width = `${Math.floor(cssViewport.width)}px`;
			canvas.style.height = `${Math.floor(cssViewport.height)}px`;
			canvas.width = Math.max(1, Math.floor(cssViewport.width * outputScale));
			canvas.height = Math.max(1, Math.floor(cssViewport.height * outputScale));
			if (canvas.parentElement) {
				canvas.parentElement.style.width = `${Math.floor(cssViewport.width)}px`;
				canvas.parentElement.style.height = `${Math.floor(cssViewport.height)}px`;
			}

			ctx = canvas.getContext('2d');
			if (!ctx) return;

			ctx.imageSmoothingEnabled = true;
			ctx.imageSmoothingQuality = 'high';

			task = pageToRender.render({
				canvasContext: ctx,
				viewport: cssViewport,
				transform
			});
			renderTasks.set(canvas, task);

			await task.promise;
			if (readerClosing) return;
			if (renderTasks.get(canvas) === task) {
				renderTasks.delete(canvas);
			}

			if (settings.textLayerEnabled) {
				void renderTextLayer(pageToRender, canvas, cssViewport, pageNum);
			}
		} catch (e: any) {
			if (task && renderTasks.get(canvas) === task) {
				renderTasks.delete(canvas);
			}
			if (e.name !== 'RenderingCancelledException') {
				error = `Failed to render: ${e.message}`;
			}
		}
	}

	async function renderTextLayer(page: any, canvas: HTMLCanvasElement, viewport: any, pageNum: number) {
		if (!PdfTextLayer) return;

		try {
			const textContent = await page.getTextContent();
			if (canvas.dataset.pageNumber !== String(pageNum)) return;
			if (textContent.items.length === 0) return;

			let textLayerDiv = canvas.parentElement?.querySelector('.text-layer') as HTMLDivElement;
			if (!textLayerDiv) {
				textLayerDiv = document.createElement('div');
				textLayerDiv.className = 'text-layer';
				textLayerDiv.style.position = 'absolute';
				textLayerDiv.style.left = '0';
				textLayerDiv.style.top = '0';
				canvas.parentElement?.appendChild(textLayerDiv);
			}

			textLayerDiv.style.pointerEvents = settings.panMode ? 'none' : 'auto';
			textLayerDiv.style.userSelect = settings.panMode ? 'none' : 'text';
			(textLayerDiv.style as CSSStyleDeclaration & { webkitUserSelect?: string }).webkitUserSelect =
				settings.panMode ? 'none' : 'text';
			textLayerDiv.innerHTML = '';

			const textLayer = new PdfTextLayer({
				textContentSource: textContent,
				container: textLayerDiv,
				viewport
			});
			await textLayer.render();
			if (settings.scrollMode === 'continuous-vertical') {
				const ratio = getVisualZoomRatio();
				textLayerDiv.style.transformOrigin = '0 0';
				textLayerDiv.style.transform = Math.abs(ratio - 1) < 0.001 ? '' : `scale(${ratio})`;
			}
			bindTextLayerSelection(textLayerDiv);
			applyPanMode();
			applySearchHighlightsToTextLayer(textLayerDiv, pageNum);
		} catch (e) {
			console.warn('Failed to render text layer:', e);
		}
	}

	function bindTextLayerSelection(textLayerDiv: HTMLDivElement) {
		if (!textLayerDiv.querySelector('.endOfContent')) {
			const endOfContent = document.createElement('div');
			endOfContent.className = 'endOfContent';
			textLayerDiv.appendChild(endOfContent);
		}

		if (!textLayerDiv.dataset.selectionBound) {
			textLayerDiv.dataset.selectionBound = 'true';
			textLayerDiv.addEventListener('mousedown', () => {
				if (!settings.panMode) {
					textLayerDiv.classList.add('selecting');
				}
			});
			textLayerDiv.addEventListener('copy', (event) => {
				const selection = document.getSelection()?.toString() || '';
				if (!selection) return;
				event.clipboardData?.setData('text/plain', selection.replace(/\u0000/g, ''));
				event.preventDefault();
			});
		}

		if (textLayerSelectionListenersBound) return;
		textLayerSelectionListenersBound = true;
		const resetTextLayerSelection = () => {
			document.querySelectorAll<HTMLDivElement>('.text-layer').forEach((layer) => {
				const hasSelection = document.getSelection()?.rangeCount
					? Array.from({ length: document.getSelection()!.rangeCount }).some((_, index) => {
							const range = document.getSelection()!.getRangeAt(index);
							return range.intersectsNode(layer);
						})
					: false;

				if (!hasSelection) {
					layer.classList.remove('selecting');
				}
			});
		};
		document.addEventListener('pointerup', resetTextLayerSelection);
		document.addEventListener('selectionchange', resetTextLayerSelection);
		window.addEventListener('blur', resetTextLayerSelection);
	}

	function textIncludesQuery(text: string, query: string) {
		if (!query) return false;
		return matchCase ? text.includes(query) : text.toLowerCase().includes(query.toLowerCase());
	}

	function applySearchHighlightsToTextLayer(textLayerDiv: HTMLDivElement, pageNum: number) {
		textLayerDiv.querySelectorAll('.pdf-search-text-match, .pdf-search-active-match').forEach((el) => {
			el.classList.remove('pdf-search-text-match', 'pdf-search-active-match');
		});

		const query = searchQuery.trim();
		if (!query || searchResults.length === 0) return;

		const resultPages = new Set(searchResults.map((result) => result.page));
		if (!resultPages.has(pageNum)) return;

		const activeResult = searchResults[currentSearchResult - 1];
		textLayerDiv.querySelectorAll('span').forEach((span) => {
			const text = span.textContent || '';
			if (!textIncludesQuery(text, query)) return;
			span.classList.add('pdf-search-text-match');
			if (activeResult?.page === pageNum && textIncludesQuery(text, activeResult.match)) {
				span.classList.add('pdf-search-active-match');
			}
		});
	}

	function refreshSearchHighlights() {
		document.querySelectorAll<HTMLDivElement>('.text-layer').forEach((layer) => {
			const pageNum = parseInt(layer.parentElement?.querySelector('canvas')?.dataset.pageNumber || '0');
			if (pageNum > 0) {
				applySearchHighlightsToTextLayer(layer, pageNum);
			}
		});
	}

	function flashActiveSearchMatch(attempt = 0) {
		clearSearchFlashTimer();
		document.querySelectorAll('.pdf-search-flash-match').forEach((el) => {
			el.classList.remove('pdf-search-flash-match');
		});

		const activeResult = searchResults[currentSearchResult - 1];
		if (!activeResult) return;

		refreshSearchHighlights();
		const activeLayer = Array.from(document.querySelectorAll<HTMLDivElement>('.text-layer')).find((layer) => {
			const pageNum = parseInt(layer.parentElement?.querySelector('canvas')?.dataset.pageNumber || '0');
			return pageNum === activeResult.page;
		});

		const targets = Array.from(
			activeLayer?.querySelectorAll<HTMLElement>('.pdf-search-active-match') || []
		);

		if (targets.length === 0 && attempt < 10) {
			searchFlashTimer = setTimeout(() => flashActiveSearchMatch(attempt + 1), 120);
			return;
		}

		targets.forEach((target) => {
			void target.offsetWidth;
			target.classList.add('pdf-search-flash-match');
		});
		searchFlashTimer = setTimeout(() => {
			targets.forEach((target) => target.classList.remove('pdf-search-flash-match'));
			searchFlashTimer = null;
		}, 1600);
	}

	async function renderThumbnail(pageNum: number) {
		if (renderedThumbnails.has(pageNum)) return;

		const canvas = document.getElementById(`thumbnail-${pageNum}`) as HTMLCanvasElement;
		if (!canvas || !pdfDoc) return;

		try {
			const page = await pdfDoc.getPage(pageNum);
			if (!page) return;

			const viewport = page.getViewport({ scale: 0.2 });
			const outputScale = getRenderScaleFactor();
			canvas.style.width = `${Math.floor(viewport.width)}px`;
			canvas.style.height = `${Math.floor(viewport.height)}px`;
			canvas.height = Math.max(1, Math.floor(viewport.height * outputScale));
			canvas.width = Math.max(1, Math.floor(viewport.width * outputScale));

			const ctx = canvas.getContext('2d');
			if (!ctx) return;

			await page.render({
				canvasContext: ctx,
				viewport: viewport,
				transform: outputScale !== 1 ? [outputScale, 0, 0, outputScale, 0, 0] : null
			}).promise;

			renderedThumbnails.add(pageNum);
		} catch (e) {
			console.warn(`Failed to render thumbnail for page ${pageNum}:`, e);
		}
	}

	function observeThumbnails() {
		const container = document.getElementById('thumbnails-container');
		if (!container) return;

		const observer = new IntersectionObserver(
			(entries) => {
				entries.forEach(entry => {
					if (entry.isIntersecting) {
						const pageNum = parseInt(entry.target.getAttribute('data-page') || '0');
						renderThumbnail(pageNum);
					}
				});
			},
			{ root: container, rootMargin: '50px' }
		);

		container.querySelectorAll('.thumbnail-item').forEach(el => {
			observer.observe(el);
		});
	}

	function updateContinuousContainerHeight(container?: HTMLElement | null) {
		const target = container || document.getElementById('continuous-container');
		if (target) {
			target.style.height = `${Math.max(1, Math.ceil(continuousTotalHeight))}px`;
			target.style.width = `${Math.max(1, getMaxContinuousPageWidth())}px`;
		}
	}

	function getMaxContinuousPageWidth() {
		let maxWidth = 0;
		continuousPageLayouts.forEach((layout) => {
			maxWidth = Math.max(maxWidth, layout.width);
		});
		return Math.ceil(maxWidth);
	}

	function recomputeContinuousOffsets(startPage = 1) {
		if (numPages <= 0) return;

		let top = 0;
		if (startPage > 1) {
			const previous = continuousPageLayouts.get(startPage - 1);
			if (previous) {
				top = previous.top + previous.height + continuousPageGapPx;
			}
		}

		for (let pageNum = Math.max(1, startPage); pageNum <= numPages; pageNum++) {
			const existing = continuousPageLayouts.get(pageNum);
			if (!existing) continue;
			continuousPageLayouts.set(pageNum, {
				...existing,
				top
			});
			top += existing.height + continuousPageGapPx;
		}

		const last = continuousPageLayouts.get(numPages);
		continuousTotalHeight = last ? last.top + last.height : 0;
		updateContinuousContainerHeight();

		pageCanvases.forEach((pageCanvas, pageNum) => {
			const wrapper = pageCanvas.parentElement as HTMLDivElement | null;
			const layout = continuousPageLayouts.get(pageNum);
			if (wrapper && layout) {
				wrapper.style.top = `${Math.floor(layout.top)}px`;
				wrapper.style.width = `${Math.floor(layout.width)}px`;
				wrapper.style.height = `${Math.floor(layout.height)}px`;
			}
		});
	}

	function buildEstimatedContinuousLayouts(placeholderViewport: any, container?: HTMLElement | null) {
		continuousPageLayouts.clear();
		const width = Math.max(1, Math.floor(placeholderViewport.width));
		const height = Math.max(1, Math.floor(placeholderViewport.height));

		let top = 0;
		for (let pageNum = 1; pageNum <= numPages; pageNum++) {
			continuousPageLayouts.set(pageNum, { top, width, height });
			top += height + continuousPageGapPx;
		}

		continuousTotalHeight = numPages > 0 ? top - continuousPageGapPx : 0;
		updateContinuousContainerHeight(container);
	}

	function updateContinuousLayoutFromViewport(pageNum: number, viewport: any) {
		const existing = continuousPageLayouts.get(pageNum);
		if (!existing) return;

		const { width, height } = getVisualPageSize(viewport);
		if (existing.width === width && existing.height === height) return;

		continuousPageLayouts.set(pageNum, {
			...existing,
			width,
			height
		});
		recomputeContinuousOffsets(pageNum);
	}

	function appendContinuousPageShell(pageNum: number, container: HTMLElement, viewport?: any) {
		const layout = continuousPageLayouts.get(pageNum);
		if (!layout || pageCanvases.has(pageNum)) return;

		const pageWrapper = document.createElement('div');
		pageWrapper.className = 'pdf-page-wrapper';
		pageWrapper.id = `pdf-page-${pageNum}`;
		pageWrapper.dataset.pageNumber = String(pageNum);
		pageWrapper.style.position = 'absolute';
		pageWrapper.style.left = '0';
		pageWrapper.style.top = `${Math.floor(layout.top)}px`;
		pageWrapper.style.width = `${Math.floor(layout.width)}px`;
		pageWrapper.style.height = `${Math.floor(layout.height)}px`;
		pageWrapper.style.transform = '';

		const pageCanvas = document.createElement('canvas');
		pageCanvas.className = 'pdf-page-canvas';
		pageCanvas.style.width = `${Math.floor(layout.width)}px`;
		pageCanvas.style.height = `${Math.floor(layout.height)}px`;
		pageCanvas.width = Math.max(1, Math.floor(pageCanvas.style.width ? parseFloat(pageCanvas.style.width) : layout.width));
		pageCanvas.height = Math.max(1, Math.floor(pageCanvas.style.height ? parseFloat(pageCanvas.style.height) : layout.height));
		pageWrapper.appendChild(pageCanvas);
		container.appendChild(pageWrapper);

		pageCanvases.set(pageNum, pageCanvas);
	}

	async function renderAllPagesContinuous(
		preloadedCurrentPage?: any,
		scrollSnapshot: ContinuousScrollSnapshot | null = null
	) {
		if (!pdfDoc || readerClosing || settings.scrollMode !== 'continuous-vertical') return;

		const container = document.getElementById('continuous-container');
		const scrollbarEl = document.getElementById('continuous-scrollbar');
		if (!container) {
			return;
		}

		cleanupContinuousRendering();
		renderedPages.clear();
		renderingPages.clear();
		pageCanvases.clear();
		pageViewports.clear();
		continuousPageLayouts.clear();
		continuousTotalHeight = 0;

		container.innerHTML = '';

		const targetPage = Math.min(Math.max(scrollSnapshot?.page ?? currentPage, 1), numPages || 1);
		currentPage = targetPage;

		const placeholderPage = preloadedCurrentPage ?? await pdfDoc.getPage(targetPage);
		const displayScale = getPdfRenderZoomLevel() / 100;
		const placeholderViewport = placeholderPage.getViewport({
			scale: displayScale,
			rotation: settings.pageRotation
		});

		buildEstimatedContinuousLayouts(placeholderViewport, container);

		await renderContinuousPage(targetPage, placeholderPage);
		await tick();
		isRestoringProgress = true;
		const layout = continuousPageLayouts.get(targetPage);
		if (layout && scrollbarEl) {
			const offsetRatio = scrollSnapshot?.offsetRatio ?? 0;
			scrollbarEl.scrollTo({
				top: layout.top + layout.height * offsetRatio,
				behavior: 'auto'
			});
			await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		}
		lastContinuousScrollTop = scrollbarEl?.scrollTop ?? 0;
		isRestoringProgress = false;
		markPdfRenderedAtCurrentZoom();
		prewarmVisiblePdfWindow(targetPage);
	}

	async function renderContinuousPage(pageNum: number, preloadedPage?: any) {
		if (renderedPages.has(pageNum) || renderingPages.has(pageNum)) return;

		let canvas = pageCanvases.get(pageNum);
		if (!pdfDoc || readerClosing) return;

		try {
			renderingPages.add(pageNum);
			const page = preloadedPage ?? await pdfDoc.getPage(pageNum);
			if (!page || readerClosing) return;

			let viewport = pageViewports.get(pageNum);
			if (!viewport) {
				viewport = page.getViewport({
					scale: getPdfRenderZoomLevel() / 100,
					rotation: settings.pageRotation
				});
				pageViewports.set(pageNum, viewport);
			}
			updateContinuousLayoutFromViewport(pageNum, viewport);

			const container = document.getElementById('continuous-container');
			if (!canvas && container) {
				appendContinuousPageShell(pageNum, container, viewport);
				canvas = pageCanvases.get(pageNum);
			}
			if (!canvas) return;

			const outputScale = getRenderScaleFactor();
			const transform = outputScale !== 1 ? [outputScale, 0, 0, outputScale, 0, 0] : null;

			const visualSize = getVisualPageSize(viewport);
			setContinuousCanvasVisualSize(canvas, visualSize.width, visualSize.height);
			canvas.height = Math.max(1, Math.floor(viewport.height * outputScale));
			canvas.width = Math.max(1, Math.floor(viewport.width * outputScale));
			if (canvas.parentElement) {
				canvas.parentElement.style.width = `${visualSize.width}px`;
				canvas.parentElement.style.height = `${visualSize.height}px`;
			}
			const ctx = canvas.getContext('2d');
			if (!ctx) return;

			const task = page.render({
				canvasContext: ctx,
				viewport,
				transform
			});
			renderTasks.set(canvas, task);

			await task.promise;
			if (readerClosing) return;
			if (renderTasks.get(canvas) === task) {
				renderTasks.delete(canvas);
			}

			if (!canvas.isConnected || pageCanvases.get(pageNum) !== canvas) {
				renderedPages.delete(pageNum);
				return;
			}

			renderedPages.add(pageNum);
			if (settings.textLayerEnabled) {
				void renderTextLayer(page, canvas, viewport, pageNum);
			}
		} catch (e: any) {
			if (canvas && renderTasks.has(canvas)) {
				renderTasks.delete(canvas);
			}
			renderedPages.delete(pageNum);
			if (e.name !== 'RenderingCancelledException') {
				console.error(`Failed to render page ${pageNum}:`, e);
			}
		} finally {
			renderingPages.delete(pageNum);
		}
	}

	function cleanupContinuousPage(pageNum: number) {
		const canvasForPage = pageCanvases.get(pageNum);
		if (!canvasForPage) return;

		const task = renderTasks.get(canvasForPage);
		if (task) {
			task.cancel();
			renderTasks.delete(canvasForPage);
		}
		canvasForPage.width = 0;
		canvasForPage.height = 0;
		canvasForPage.parentElement?.remove();
		pageCanvases.delete(pageNum);
		pageViewports.delete(pageNum);
		renderedPages.delete(pageNum);
		renderingPages.delete(pageNum);
	}

	async function renderVisibleContinuousPages(scrollbarEl?: HTMLElement | null, force = false) {
		if (!pdfDoc || settings.scrollMode !== 'continuous-vertical') return;

		const scroller = scrollbarEl || scrollbar || document.getElementById('continuous-scrollbar') as HTMLElement | null;
		if (!scroller || continuousPageLayouts.size === 0) return;

		const visibleTop = scroller.scrollTop;
		const visibleBottom = visibleTop + scroller.clientHeight;
		const overscanPx = getContinuousOverscanPx();
		const keepTop = Math.max(0, visibleTop - overscanPx);
		const keepBottom = visibleBottom + overscanPx;
		const renderTop = Math.max(0, visibleTop - overscanPx / 2);
		const renderBottom = visibleBottom + overscanPx / 2;
		const pagesToRender: number[] = [];
		let topmostVisiblePage = 0;
		let topmostVisibleTop = Number.POSITIVE_INFINITY;

		for (let pageNum = 1; pageNum <= numPages; pageNum++) {
			const layout = continuousPageLayouts.get(pageNum);
			if (!layout) continue;
			const pageBottom = layout.top + layout.height;
			const inKeepWindow = pageBottom >= keepTop && layout.top <= keepBottom;
			const inRenderWindow = pageBottom >= renderTop && layout.top <= renderBottom;
			const inViewport = pageBottom >= visibleTop && layout.top <= visibleBottom;

			if (inRenderWindow) {
				pagesToRender.push(pageNum);
			}
			if (inViewport && layout.top < topmostVisibleTop) {
				topmostVisiblePage = pageNum;
				topmostVisibleTop = layout.top;
			}
			if (!inKeepWindow && pageCanvases.has(pageNum)) {
				cleanupContinuousPage(pageNum);
			}
		}

		if (topmostVisiblePage > 0 && !isRestoringProgress && topmostVisiblePage !== currentPage) {
			currentPage = topmostVisiblePage;
			queueProgressSave();
		}

		if (force) {
			await renderContinuousWindow(currentPage, 2);
		}

		await Promise.all(pagesToRender.map((pageNum) => renderContinuousPage(pageNum)));
	}

	function renderContinuousWindow(centerPage: number, radius = 1) {
		const pages = [centerPage];

		for (let offset = 1; offset <= radius; offset++) {
			pages.push(centerPage + offset, centerPage - offset);
		}

		return Promise.all(
			pages
				.filter((pageNum) => pageNum >= 1 && pageNum <= numPages)
				.map((pageNum) => renderContinuousPage(pageNum))
			).then(() => undefined);
	}

	function cleanupContinuousRendering() {
		renderTasks.forEach((task) => task.cancel());
		renderTasks.clear();
		pageCanvases.forEach((pageCanvas) => {
			pageCanvas.width = 0;
			pageCanvas.height = 0;
		});
		renderedPages.clear();
		renderingPages.clear();
		pageCanvases.clear();
		pageViewports.clear();
	}

	function rotateLeft() {
		const newRotation = (settings.pageRotation - 90 + 360) % 360 as 0 | 90 | 180 | 270;
		updateSetting('pageRotation', newRotation);
	}

	function rotateRight() {
		const newRotation = (settings.pageRotation + 90) % 360 as 0 | 90 | 180 | 270;
		updateSetting('pageRotation', newRotation);
	}

	function toggleLeftSidebar() {
		showTopBar(false);
		if (rightSidebarOpen) {
			rightSidebarOpen = false;
		}
		leftSidebarOpen = !leftSidebarOpen;
		resetTopBarBehavior();
	}

	function toggleRightSidebar() {
		showTopBar(false);
		if (leftSidebarOpen) {
			leftSidebarOpen = false;
		}
		rightSidebarOpen = !rightSidebarOpen;
		resetTopBarBehavior();
	}

	function toggleSearchSidebar() {
		showTopBar(false);
		if (leftSidebarOpen && activeSidebarTab === 'search' && !rightSidebarOpen) {
			leftSidebarOpen = false;
			resetTopBarBehavior();
			return;
		}

		activeSidebarTab = 'search';
		leftSidebarOpen = true;
		rightSidebarOpen = false;
		resetTopBarBehavior();
	}

	function togglePanMode() {
		showTopBar(true);
		settings = { ...settings, panMode: !settings.panMode };
		readerSettings.updatePdf({ panMode: settings.panMode });
		applyPanMode();
	}

	async function toggleFitWidth() {
		showTopBar(true);
		if (fitWidthActive && fitWidthSnapshot) {
			const snapshot = fitWidthSnapshot;
			fitWidthSnapshot = null;
			fitWidthActive = false;
			settings = {
				...settings,
				pageZoom: snapshot.pageZoom,
				zoomLevel: snapshot.zoomLevel
			};
			readerSettings.updatePdf({
				pageZoom: snapshot.pageZoom,
				zoomLevel: snapshot.zoomLevel
			});
			await tick();
			if (settings.scrollMode === 'continuous-vertical') {
				await renderAllPagesContinuous();
			} else {
				await renderPage(currentPage);
			}
			return;
		}

		fitWidthSnapshot = {
			pageZoom: settings.pageZoom,
			zoomLevel: settings.zoomLevel
		};
		fitWidthActive = true;
		await updateSetting('pageZoom', 'page-width');
	}

	function handleMouseDown(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (settings.panMode && (settings.scrollMode === 'paged' || settings.scrollMode === 'continuous-vertical')) {
			if (
				target.closest('.floating-nav') ||
				target.closest('.top-nav') ||
				target.closest('.left-sidebar') ||
				target.closest('.right-sidebar') ||
				target.closest('button, a, input, textarea, select, label')
			) {
				return;
			}
			e.preventDefault();
			isDragging = true;
			dragStart = { x: e.clientX, y: e.clientY };
			activePanScrollContainer = getPanScrollContainer(e.target);
			if (activePanScrollContainer) {
				scrollStart = {
					x: activePanScrollContainer.scrollLeft,
					y: activePanScrollContainer.scrollTop
				};
			}
		}
	}

	function handleMouseMove(e: MouseEvent) {
		handleReaderPointerMove(e);

		if (isDragging && settings.panMode) {
			e.preventDefault();
			const dx = e.clientX - dragStart.x;
			const dy = e.clientY - dragStart.y;
			if (activePanScrollContainer) {
				activePanScrollContainer.scrollLeft = scrollStart.x - dx;
				activePanScrollContainer.scrollTop = scrollStart.y - dy;
			}
		}
	}

	function handleMouseUp() {
		isDragging = false;
		activePanScrollContainer = null;
	}

	let isDraggingProgress = $state(false);
	let pendingProgressPage = $state<number | null>(null);
	let progressPreviewPage = $state<number | null>(null);
	let pdfProgressBarEl = $state<HTMLElement | null>(null);
	let activeProgressPointerId: number | null = null;
	const progressPreviewProgress = $derived(
		progressPreviewPage !== null && numPages > 0 ? (progressPreviewPage / numPages) * 100 : null
	);
	const progressPreviewLabel = $derived(
		progressPreviewPage !== null && numPages > 0
			? `Page ${progressPreviewPage} / ${numPages} • ${Math.round((progressPreviewPage / numPages) * 100)}%`
			: ''
	);

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

	function handleProgressPointerUp(e: PointerEvent) {
		if (activeProgressPointerId !== null && e.pointerId !== activeProgressPointerId) return;
		e.preventDefault();
		e.stopPropagation();
		finishProgressPointer(e);
	}

	function handleProgressPointerCancel(e: PointerEvent) {
		if (activeProgressPointerId !== null && e.pointerId !== activeProgressPointerId) return;
		finishProgressPointer(e);
		progressPreviewPage = null;
	}

	function jumpToPage(pageNum: number) {
		if (pageNum >= 1 && pageNum <= numPages) {
			showTopBar(false);
			navigateToPage(pageNum, 'auto');
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

		if (isEditingPage) {
			if (e.key === 'Enter' || e.key === 'Escape') {
				finishEditPage();
			}
			return;
		}

		const isZoomShortcut = (e.ctrlKey || e.metaKey) && isPdfDocumentKeyboardTarget(e.target);
		if (isZoomShortcut && (e.key === '+' || e.key === '=')) {
			e.preventDefault();
			setDocumentZoom(settings.zoomLevel + 10);
			return;
		}
		if (isZoomShortcut && e.key === '-') {
			e.preventDefault();
			setDocumentZoom(settings.zoomLevel - 10);
			return;
		}
		if (isZoomShortcut && e.key === '0') {
			e.preventDefault();
			setDocumentZoom(100);
			return;
		}

		if (settings.scrollMode === 'continuous-vertical') {
			if (e.key === 'ArrowUp' || e.key === 'k') {
				e.preventDefault();
				if (currentPage > 1) {
					navigateToPage(currentPage - 1, 'auto');
				}
			} else if (e.key === 'ArrowDown' || e.key === 'j' || e.key === ' ') {
				e.preventDefault();
				if (currentPage < numPages) {
					navigateToPage(currentPage + 1, 'auto');
				}
			}
		} else {
			if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
				e.preventDefault();
				prevPage();
			} else if (e.key === 'ArrowRight' || e.key === 'ArrowDown' || e.key === ' ') {
				e.preventDefault();
				nextPage();
			}
		}

		if (e.key === 'Escape') {
			void closeReader();
		} else if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
			e.preventDefault();
			activeSidebarTab = 'search';
			leftSidebarOpen = true;
			rightSidebarOpen = false;
		} else if (e.key === 'h') {
			togglePanMode();
		} else if (e.key === '[') {
			rotateLeft();
		} else if (e.key === ']') {
			rotateRight();
		} else if (e.key === '+' || e.key === '=') {
			setDocumentZoom(settings.zoomLevel + 10);
		} else if (e.key === '-') {
			setDocumentZoom(settings.zoomLevel - 10);
		}
	}

	function shouldIgnoreWheelNavigation(target: EventTarget | null) {
		if (!(target instanceof Element)) return false;
		return !!target.closest('input, textarea, select, [contenteditable="true"], .left-sidebar, .right-sidebar');
	}

	function handleWheelNavigation(e: WheelEvent) {
		if ((e.ctrlKey || e.metaKey) && isPdfDocumentTarget(e.target)) {
			e.preventDefault();
			showTopBar(true);
			const direction = e.deltaY > 0 ? -1 : 1;
			setDocumentZoom(settings.zoomLevel + direction * 10);
			return;
		}

		if (settings.scrollMode !== 'paged' || settings.panMode || shouldIgnoreWheelNavigation(e.target)) {
			return;
		}
		showTopBar(true);

		const dominantDelta = Math.abs(e.deltaY) >= Math.abs(e.deltaX) ? e.deltaY : e.deltaX;
		if (Math.abs(dominantDelta) < 12) return;

		const now = performance.now();
		if (now - lastWheelNavigationAt < 220) {
			e.preventDefault();
			return;
		}

		e.preventDefault();
		lastWheelNavigationAt = now;

		if (dominantDelta > 0) {
			nextPage();
		} else {
			prevPage();
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

	function queueSearch() {
		clearSearchTimer();
		if (!searchQuery.trim()) {
			searchRunId++;
			searchResults = [];
			currentSearchResult = 0;
			isSearching = false;
			refreshSearchHighlights();
			return;
		}

		searchTimer = setTimeout(() => {
			searchTimer = null;
			void performSearch();
		}, 300);
	}

	function createSearchResult(id: number, page: number, index: number, text: string, queryLength: number): SearchResult {
		const contextLength = 48;
		const beforeStart = Math.max(0, index - contextLength);
		const afterEnd = Math.min(text.length, index + queryLength + contextLength);
		const beforePrefix = beforeStart > 0 ? '...' : '';
		const afterSuffix = afterEnd < text.length ? '...' : '';

		return {
			id,
			page,
			index,
			before: beforePrefix + text.slice(beforeStart, index),
			match: text.slice(index, index + queryLength),
			after: text.slice(index + queryLength, afterEnd) + afterSuffix
		};
	}

	async function performSearch() {
		const runId = ++searchRunId;
		clearSearchTimer();
		if (!pdfDoc || !searchQuery.trim()) {
			searchResults = [];
			currentSearchResult = 0;
			refreshSearchHighlights();
			return;
		}

		isSearching = true;
		searchResults = [];
		currentSearchResult = 0;
		refreshSearchHighlights();
		const nextResults: SearchResult[] = [];
		let resultId = 1;

		for (let i = 1; i <= numPages; i++) {
			if (runId !== searchRunId) return;
			try {
				const page = await pdfDoc.getPage(i);
				const textContent = await page.getTextContent();
				const text = textContent.items.map((item: any) => item.str).join(' ');

				const query = matchCase ? searchQuery : searchQuery.toLowerCase();
				const searchText = matchCase ? text : text.toLowerCase();

				let index = 0;
				while ((index = searchText.indexOf(query, index)) !== -1) {
					nextResults.push(createSearchResult(resultId, i, index, text, searchQuery.length));
					resultId++;
					index += Math.max(query.length, 1);
				}
			} catch (e) {
				console.warn(`Search failed on page ${i}:`, e);
			}
		}

		if (runId !== searchRunId) return;
		searchResults = nextResults;
		currentSearchResult = searchResults.length > 0 ? 1 : 0;
		isSearching = false;
		refreshSearchHighlights();
	}

	function selectSearchResult(index: number) {
		const result = searchResults[index];
		if (!result) return;
		currentSearchResult = index + 1;
		goToPage(result.page);
		flashActiveSearchMatch();
	}

	function prevSearchResult() {
		if (currentSearchResult > 1) {
			selectSearchResult(currentSearchResult - 2);
		}
	}

	function nextSearchResult() {
		if (currentSearchResult < searchResults.length) {
			selectSearchResult(currentSearchResult);
		}
	}

	async function loadPdfOutline() {
		if (!pdfDoc) return;

		try {
			const outline = await pdfDoc.getOutline();
			pdfOutline = outline || [];
		} catch (e) {
			console.warn('Failed to load PDF outline:', e);
			pdfOutline = [];
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

	$effect(() => {
		if (pdfReady) {
			applyPanMode();
		}
	});

	onMount(() => {
		window.addEventListener('keydown', handleKeydown);
		window.addEventListener('wheel', handleWheelNavigation, { passive: false });
		window.addEventListener('mousemove', handleMouseMove);
		window.addEventListener('mouseup', handleMouseUp);
		window.addEventListener('pointerdown', handleReaderPointerDown);
		window.addEventListener('pointermove', handleReaderPointerMove);
		window.addEventListener('pointerup', handlePdfContainerPointerUp);
		window.addEventListener('resize', handleViewportResize);
		pdfContainerEl?.addEventListener('touchstart', handleTouchStart, { passive: false });
		pdfContainerEl?.addEventListener('touchmove', handleTouchMove, { passive: false });
		pdfContainerEl?.addEventListener('touchend', handleTouchEnd);
		pdfContainerEl?.addEventListener('touchcancel', handleTouchEnd);
		return () => {
			window.removeEventListener('keydown', handleKeydown);
			window.removeEventListener('wheel', handleWheelNavigation);
			window.removeEventListener('mousemove', handleMouseMove);
			window.removeEventListener('mouseup', handleMouseUp);
			window.removeEventListener('pointerdown', handleReaderPointerDown);
			window.removeEventListener('pointermove', handleReaderPointerMove);
			window.removeEventListener('pointerup', handlePdfContainerPointerUp);
			window.removeEventListener('resize', handleViewportResize);
			pdfContainerEl?.removeEventListener('touchstart', handleTouchStart);
			pdfContainerEl?.removeEventListener('touchmove', handleTouchMove);
			pdfContainerEl?.removeEventListener('touchend', handleTouchEnd);
			pdfContainerEl?.removeEventListener('touchcancel', handleTouchEnd);
			if (viewportResizeTimeout) {
				clearTimeout(viewportResizeTimeout);
			}
			resetPinchTransform();
			clearTopBarHideTimer();
			cleanupContinuousRendering();
		};
	});

	function zoomIn() {
		updateSetting('zoomLevel', Math.min(settings.zoomLevel + 25, 400));
	}

	function zoomOut() {
		updateSetting('zoomLevel', Math.max(settings.zoomLevel - 25, 25));
	}

	function autoScale() {
		updateSetting('pageZoom', 'auto');
		updateZoomFromMode();
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
			embedPdfRestoringInitialPage = false;
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
		console.log('EmbedPDF ready, registry:', registry);
		embedPdfScroll = registry.getPlugin?.('scroll')?.provides?.() ?? null;
		embedPdfViewerReady = true;
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
			error = '';
			pdfLoadRetryToken = Date.now();
			return;
		}

		error = normalized || 'Failed to load PDF';
	}
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
					{#key `${book.id}:${requestedFormat}:${pdfLoadRetryToken}`}
					<EmbedPDFViewer
						src={getPdfSourceUrl()}
						title={getPdfDisplayTitle()}
						initialPage={embedPdfInitialPage}
						toolbarVisible={readerChromeReady && topBarVisible}
						onScrollActivity={handleEmbedPdfScrollActivity}
						onPageChange={handleEmbedPdfPageChange}
						onReady={handleEmbedPdfReady}
						onSidebarOpenChange={handleEmbedPdfSidebarOpenChange}
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

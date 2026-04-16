<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { browser } from '$app/environment';
	import { readerSettings, pdfZoomModes } from '$lib/stores/readerSettings';
	import type { PdfReaderSetting, PdfViewMode } from '$lib/stores/readerSettings';
	import { normalizeBookFormat } from '$lib/utils/book-formats';

	let book = $state<any>(null);
	let loading = $state(true);
	let error = $state('');
	let pdfDoc: any = null;
	let currentPage = $state(1);
	let numPages = $state(0);
	let scale = $state(1);
	let pdfInstance: any = null;
	let canvas: HTMLCanvasElement | undefined = undefined;
	let ctx: CanvasRenderingContext2D | null = null;
	let readerInitialized = false;
	let pdfReady = $state(false);
	let savedProgress = $state<any>(null);
	let pdfOutline = $state<any[]>([]);
	let expandedItems = $state<Set<string>>(new Set());
	let currentSessionId = $state<number | null>(null);

	let settings = $state<PdfReaderSetting>({
		pageSpread: 'off',
		pageLayout: 'single',
		pageZoom: 'auto',
		zoomLevel: 100,
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
		panMode: false
	});

	type SidebarTab = 'thumbnails' | 'bookmarks' | 'search';
	let leftSidebarOpen = $state(false);
	let rightSidebarOpen = $state(false);
	let activeSidebarTab = $state<SidebarTab>('thumbnails');
	let searchQuery = $state('');
	let searchResults = $state<any[]>([]);
	let currentSearchResult = $state(0);
	let matchCase = $state(false);
	let isSearching = $state(false);

	let pageCanvases: Map<number, HTMLCanvasElement> = new Map();
	let thumbnailCanvases: Map<number, HTMLCanvasElement> = new Map();
	let pageViewports: Map<number, any> = new Map();
	let renderedPages: Set<number> = new Set();
	let renderingPages: Set<number> = new Set();
	let renderedThumbnails: Set<number> = new Set();
	let renderTasks: Map<HTMLCanvasElement, any> = new Map();
	let continuousContainer: HTMLDivElement | undefined = undefined;
	let intersectionObserver: IntersectionObserver | null = null;
	let scrollbar: HTMLDivElement | undefined = undefined;
	let isDragging = $state(false);
	let dragStart = $state({ x: 0, y: 0 });
	let scrollStart = $state({ x: 0, y: 0 });
	let pageInputValue = $state('');
	let isEditingPage = $state(false);
	let lastWheelNavigationAt = 0;
	let touchPinchActive = false;
	let touchPinchStartDistance = 0;
	let touchPinchStartZoom = 0;
	let touchPinchPendingZoom: number | null = null;
	let touchPinchFrame: number | null = null;
	let fitWidthSnapshot: { pageZoom: PdfReaderSetting['pageZoom']; zoomLevel: number } | null = null;
	let fitWidthActive = $state(false);
	let viewportResizeTimeout: ReturnType<typeof setTimeout> | null = null;
	let sessionEnded = false;
	let handlePageExit: (() => void) | null = null;
	let pdfContainerEl: HTMLDivElement | null = null;

	const progress = $derived(numPages > 0 ? (currentPage / numPages) * 100 : 0);

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
			}
		};
		window.addEventListener('mousedown', globalMouseDownListener);
		window.addEventListener('click', globalClickListener);

		void (async () => {
			const bookId = $page.params.bookID;
			try {
				const res = await fetch(`/api/books/${bookId}`);
				if (res.ok) {
					book = await res.json();
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

			unsubscribeReaderSettings = readerSettings.subscribe(s => {
				if (pdfReady) {
					settings = { ...s.pdf };
				}
			});

			handlePageExit = () => {
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

	onDestroy(() => {
		if (handlePageExit) {
			window.removeEventListener('pagehide', handlePageExit);
			window.removeEventListener('beforeunload', handlePageExit);
		}
		void endSession(true);
	});

	async function updateZoomFromMode() {
		if (!pdfDoc) return;

		const container = document.getElementById('pdf-container');
		if (!container) return;

		const containerWidth = container.clientWidth - 32;
		const containerHeight = container.clientHeight - 32;
		const isMobileViewport = window.matchMedia('(max-width: 768px)').matches;

		const page = await pdfDoc.getPage(1);
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

	async function updateSetting(key: string, value: any) {
		settings = { ...settings, [key]: value };
		readerSettings.updatePdf({ [key]: value });

		if (key === 'scrollMode') {
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
			await updateZoomFromMode();
			if (settings.scrollMode === 'continuous-vertical') {
				await tick();
				renderAllPagesContinuous();
			} else {
				await tick();
				renderPage(currentPage);
			}
		} else if (key === 'zoomLevel') {
			if (settings.scrollMode === 'continuous-vertical') {
				await tick();
				renderAllPagesContinuous();
			} else {
				await tick();
				renderPage(currentPage);
			}
		} else if (key === 'pageRotation') {
			if (settings.scrollMode === 'continuous-vertical') {
				await tick();
				renderAllPagesContinuous();
			} else {
				renderPage(currentPage);
			}
		} else if (['brightness', 'contrast', 'grayscale'].includes(key)) {
			applyVisualFilters();
		} else if (key === 'pageSpread' || key === 'pageLayout') {
			if (settings.scrollMode === 'continuous-vertical') {
				renderAllPagesContinuous();
			} else {
				await tick();
				renderPage(currentPage);
			}
		} else if (key === 'viewMode') {
			applyViewMode();
		}
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

	function getTouchDistance(touches: TouchList) {
		if (touches.length < 2) return 0;
		const dx = touches[0].clientX - touches[1].clientX;
		const dy = touches[0].clientY - touches[1].clientY;
		return Math.hypot(dx, dy);
	}

	function queuePinchZoom(nextZoom: number) {
		touchPinchPendingZoom = nextZoom;
		if (touchPinchFrame !== null) return;

		touchPinchFrame = window.requestAnimationFrame(() => {
			touchPinchFrame = null;
			if (touchPinchPendingZoom === null) return;
			const zoom = Math.round(touchPinchPendingZoom);
			touchPinchPendingZoom = null;
			if (zoom !== settings.zoomLevel) {
				void updateSetting('zoomLevel', zoom);
			}
		});
	}

	function resetPinchTransform() {
		if (!pdfContainerEl) return;
		pdfContainerEl.style.transform = '';
		pdfContainerEl.style.transformOrigin = '';
		pdfContainerEl.style.willChange = '';
	}

	function handleTouchStart(e: TouchEvent) {
		if (!pdfContainerEl || e.touches.length !== 2) return;
		if (!pdfContainerEl.contains(e.target as Node)) return;

		touchPinchActive = true;
		touchPinchStartDistance = getTouchDistance(e.touches);
		touchPinchStartZoom = settings.zoomLevel;
		touchPinchPendingZoom = settings.zoomLevel;
		pdfContainerEl.style.transformOrigin = 'center center';
		pdfContainerEl.style.willChange = 'transform';
	}

	function handleTouchMove(e: TouchEvent) {
		if (!touchPinchActive || !pdfContainerEl || e.touches.length !== 2) return;
		e.preventDefault();

		const distance = getTouchDistance(e.touches);
		if (touchPinchStartDistance <= 0 || distance <= 0) return;

		const nextZoom = Math.max(
			25,
			Math.min(400, (touchPinchStartZoom * distance) / touchPinchStartDistance)
		);
		const visualScale = nextZoom / touchPinchStartZoom;
		pdfContainerEl.style.transform = `scale(${visualScale})`;
		queuePinchZoom(nextZoom);
	}

	function handleTouchEnd() {
		if (!touchPinchActive) return;
		touchPinchActive = false;
		resetPinchTransform();
		if (touchPinchPendingZoom !== null && Math.round(touchPinchPendingZoom) !== settings.zoomLevel) {
			void updateSetting('zoomLevel', Math.round(touchPinchPendingZoom));
		}
		touchPinchPendingZoom = null;
		if (touchPinchFrame !== null) {
			window.cancelAnimationFrame(touchPinchFrame);
			touchPinchFrame = null;
		}
	}

	function handleViewportResize() {
		if (viewportResizeTimeout) {
			clearTimeout(viewportResizeTimeout);
		}

		viewportResizeTimeout = setTimeout(() => {
			if (!pdfReady || !pdfDoc || settings.pageZoom === 'actual-size') return;

			void updateZoomFromMode().then(() => {
				if (settings.scrollMode === 'continuous-vertical') {
					renderAllPagesContinuous();
				} else {
					renderPage(currentPage);
				}
			});
		}, 120);
	}

	function prevPage() {
		if (settings.readingDirection === 'rtl') {
			if (currentPage < numPages) {
				currentPage++;
				if (settings.scrollMode === 'continuous-vertical') {
					scrollToPage(currentPage);
				} else {
					renderPage(currentPage);
				}
			}
		} else {
			if (currentPage > 1) {
				currentPage--;
				if (settings.scrollMode === 'continuous-vertical') {
					scrollToPage(currentPage);
				} else {
					renderPage(currentPage);
				}
			}
		}
		saveProgress();
	}

	function nextPage() {
		if (settings.readingDirection === 'rtl') {
			if (currentPage > 1) {
				currentPage--;
				if (settings.scrollMode === 'continuous-vertical') {
					scrollToPage(currentPage);
				} else {
					renderPage(currentPage);
				}
			}
		} else {
			if (currentPage < numPages) {
				currentPage++;
				if (settings.scrollMode === 'continuous-vertical') {
					scrollToPage(currentPage);
				} else {
					renderPage(currentPage);
				}
			}
		}
		saveProgress();
	}

	function goToPage(pageNum: number) {
		if (pageNum >= 1 && pageNum <= numPages) {
			currentPage = pageNum;
			if (settings.scrollMode === 'continuous-vertical') {
				scrollToPage(pageNum);
			} else {
				renderPage(pageNum);
			}
			saveProgress();
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

	async function scrollToPage(pageNum: number) {
		const canvas = pageCanvases.get(pageNum);
		if (canvas) {
			canvas.scrollIntoView({ behavior: 'smooth', block: 'start' });
			await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		}
	}

	async function renderPage(pageNum: number) {
		if (!pdfDoc) return;

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

			await renderSinglePage(canvas, pageNum);
		}
	}

	async function renderSinglePage(canvas: HTMLCanvasElement, pageNum: number) {
		if (!pdfDoc || pageNum < 1 || pageNum > numPages) return;

		const existingTask = renderTasks.get(canvas);
		if (existingTask) {
			existingTask.cancel();
		}

		let task: any = null;
		try {
			const page = await pdfDoc.getPage(pageNum);
			if (!page) return;

			const displayScale = settings.zoomLevel / 100;
			const cssViewport = page.getViewport({
				scale: displayScale,
				rotation: settings.pageRotation
			});
			const outputScale = Math.max(window.devicePixelRatio || 1, 1);
			const renderViewport = page.getViewport({
				scale: displayScale * outputScale,
				rotation: settings.pageRotation
			});

			canvas.dataset.pageNumber = String(pageNum);
			canvas.style.width = `${cssViewport.width}px`;
			canvas.style.height = `${cssViewport.height}px`;
			canvas.width = renderViewport.width;
			canvas.height = renderViewport.height;

			ctx = canvas.getContext('2d');
			if (!ctx) return;

			ctx.imageSmoothingEnabled = true;
			ctx.imageSmoothingQuality = 'high';

			task = page.render({
				canvasContext: ctx,
				viewport: renderViewport
			});
			renderTasks.set(canvas, task);

			await task.promise;
			if (renderTasks.get(canvas) === task) {
				renderTasks.delete(canvas);
			}

			if (settings.textLayerEnabled) {
				void renderTextLayer(page, canvas, cssViewport, pageNum);
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
				textLayerDiv.style.zIndex = '2';
				textLayerDiv.style.color = 'transparent';
				textLayerDiv.style.fontSize = '1px';
				canvas.parentElement?.appendChild(textLayerDiv);
			}

			textLayerDiv.style.width = viewport.width + 'px';
			textLayerDiv.style.height = viewport.height + 'px';
			textLayerDiv.style.pointerEvents = settings.panMode ? 'none' : 'auto';
			textLayerDiv.style.userSelect = settings.panMode ? 'none' : 'text';
			(textLayerDiv.style as CSSStyleDeclaration & { webkitUserSelect?: string }).webkitUserSelect =
				settings.panMode ? 'none' : 'text';
			textLayerDiv.innerHTML = '';

			textContent.items.forEach((item: any) => {
				const textDiv = document.createElement('div');
				textDiv.style.position = 'absolute';
				textDiv.style.left = item.transform[4] + 'px';
				textDiv.style.top = (viewport.height - item.transform[5]) + 'px';
				textDiv.style.fontSize = Math.abs(item.transform[0]) + 'px';
				textDiv.style.fontFamily = 'sans-serif';
				textDiv.style.color = 'transparent';
				textDiv.style.whiteSpace = 'pre';
				textDiv.textContent = item.str;
				textLayerDiv.appendChild(textDiv);
			});
		} catch (e) {
			console.warn('Failed to render text layer:', e);
		}
	}

	async function renderThumbnail(pageNum: number) {
		if (renderedThumbnails.has(pageNum)) return;

		const canvas = document.getElementById(`thumbnail-${pageNum}`) as HTMLCanvasElement;
		if (!canvas || !pdfDoc) return;

		try {
			const page = await pdfDoc.getPage(pageNum);
			if (!page) return;

			const viewport = page.getViewport({ scale: 0.2 });
			canvas.height = viewport.height;
			canvas.width = viewport.width;

			const ctx = canvas.getContext('2d');
			if (!ctx) return;

			await page.render({
				canvasContext: ctx,
				viewport: viewport
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

	function appendContinuousPageShell(
		pageNum: number,
		container: HTMLElement,
		placeholderViewport: any,
		placeholderRenderViewport: any
	) {
		const pageWrapper = document.createElement('div');
		pageWrapper.className = 'pdf-page-wrapper';
		pageWrapper.id = `pdf-page-${pageNum}`;
		pageWrapper.dataset.pageNumber = String(pageNum);

		const pageCanvas = document.createElement('canvas');
		pageCanvas.className = 'pdf-page-canvas';
		pageCanvas.style.width = `${placeholderViewport.width}px`;
		pageCanvas.style.height = `${placeholderViewport.height}px`;
		pageCanvas.width = placeholderRenderViewport.width;
		pageCanvas.height = placeholderRenderViewport.height;
		pageWrapper.appendChild(pageCanvas);
		container.appendChild(pageWrapper);

		pageCanvases.set(pageNum, pageCanvas);

		if (intersectionObserver) {
			intersectionObserver.observe(pageWrapper);
		}
	}

	async function appendRemainingContinuousPages(
		container: HTMLElement,
		placeholderViewport: any,
		placeholderRenderViewport: any,
		initialPages: Set<number>
	) {
		const batchSize = 20;
		let appended = 0;

		for (let i = 1; i <= numPages; i++) {
			if (initialPages.has(i)) continue;

			appendContinuousPageShell(i, container, placeholderViewport, placeholderRenderViewport);
			appended++;

			if (appended % batchSize === 0) {
				await tick();
				await new Promise<void>(resolve => requestAnimationFrame(() => resolve()));
			}
		}
	}

	async function renderAllPagesContinuous() {
		if (!pdfDoc || settings.scrollMode !== 'continuous-vertical') return;

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

		container.innerHTML = '';

		const placeholderPage = await pdfDoc.getPage(currentPage);
		const displayScale = settings.zoomLevel / 100;
		const outputScale = Math.max(window.devicePixelRatio || 1, 1);
		const placeholderViewport = placeholderPage.getViewport({
			scale: displayScale,
			rotation: settings.pageRotation
		});
		const placeholderRenderViewport = placeholderPage.getViewport({
			scale: displayScale * outputScale,
			rotation: settings.pageRotation
		});

		const initialPages = new Set<number>([currentPage]);
		for (let offset = 1; offset <= 2; offset++) {
			if (currentPage + offset <= numPages) initialPages.add(currentPage + offset);
			if (currentPage - offset >= 1) initialPages.add(currentPage - offset);
		}

		Array.from(initialPages)
			.sort((a, b) => a - b)
			.forEach((pageNum) => {
				appendContinuousPageShell(pageNum, container, placeholderViewport, placeholderRenderViewport);
			});

		await observePageVisibility(scrollbarEl);
		await renderContinuousPage(currentPage);
		void renderContinuousWindow(currentPage, 2);
		await tick();
		await scrollToPage(currentPage);
		void appendRemainingContinuousPages(container, placeholderViewport, placeholderRenderViewport, initialPages);
	}

	async function renderContinuousPage(pageNum: number) {
		if (renderedPages.has(pageNum) || renderingPages.has(pageNum)) return;

		const canvas = pageCanvases.get(pageNum);
		if (!canvas || !pdfDoc) return;

		try {
			renderingPages.add(pageNum);
			const page = await pdfDoc.getPage(pageNum);
			if (!page) return;

			let viewport = pageViewports.get(pageNum);
			if (!viewport) {
				viewport = page.getViewport({
					scale: settings.zoomLevel / 100,
					rotation: settings.pageRotation
				});
				pageViewports.set(pageNum, viewport);
			}

			const outputScale = Math.max(window.devicePixelRatio || 1, 1);
			const renderViewport = page.getViewport({
				scale: (settings.zoomLevel / 100) * outputScale,
				rotation: settings.pageRotation
			});

			canvas.style.width = `${viewport.width}px`;
			canvas.style.height = `${viewport.height}px`;
			canvas.height = renderViewport.height;
			canvas.width = renderViewport.width;
			const ctx = canvas.getContext('2d');
			if (!ctx) return;

			await page.render({
				canvasContext: ctx,
				viewport: renderViewport
			}).promise;

			renderedPages.add(pageNum);
		} catch (e: any) {
			if (e.name !== 'RenderingCancelledException') {
				console.error(`Failed to render page ${pageNum}:`, e);
			}
		} finally {
			renderingPages.delete(pageNum);
		}
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

	async function observePageVisibility(scrollbarEl: HTMLElement | null = null) {
		if (intersectionObserver) {
			intersectionObserver.disconnect();
		}

		intersectionObserver = new IntersectionObserver(
			(entries) => {
				let topmostVisiblePage = 0;
				let topmostVisibleTop = Number.POSITIVE_INFINITY;
				let topmostVisibleRatio = 0;

				entries.forEach(entry => {
					const pageNum = parseInt(entry.target.getAttribute('data-page-number') || '0');
					if (entry.intersectionRatio <= 0) return;

					void renderContinuousWindow(pageNum, 2);

					const top = entry.boundingClientRect.top;
					if (
						top < topmostVisibleTop - 1 ||
						(Math.abs(top - topmostVisibleTop) <= 1 && entry.intersectionRatio > topmostVisibleRatio)
					) {
						topmostVisiblePage = pageNum;
						topmostVisibleTop = top;
						topmostVisibleRatio = entry.intersectionRatio;
					}
				});

				if (topmostVisiblePage > 0) {
					currentPage = topmostVisiblePage;
				}
			},
			{
				root: scrollbarEl || scrollbar,
				rootMargin: '0px',
				threshold: [0, 0.25, 0.5, 0.75, 1]
			}
		);

		for (let i = 1; i <= numPages; i++) {
			const wrapper = document.getElementById(`pdf-page-${i}`);
			if (wrapper && intersectionObserver) {
				intersectionObserver.observe(wrapper);
			}
		}
	}

	function cleanupContinuousRendering() {
		if (intersectionObserver) {
			intersectionObserver.disconnect();
			intersectionObserver = null;
		}
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
		if (rightSidebarOpen) {
			rightSidebarOpen = false;
		}
		leftSidebarOpen = !leftSidebarOpen;
	}

	function toggleRightSidebar() {
		if (leftSidebarOpen) {
			leftSidebarOpen = false;
		}
		rightSidebarOpen = !rightSidebarOpen;
	}

	function toggleSearchSidebar() {
		if (leftSidebarOpen && activeSidebarTab === 'search' && !rightSidebarOpen) {
			leftSidebarOpen = false;
			return;
		}

		activeSidebarTab = 'search';
		leftSidebarOpen = true;
		rightSidebarOpen = false;
	}

	function togglePanMode() {
		settings = { ...settings, panMode: !settings.panMode };
		readerSettings.updatePdf({ panMode: settings.panMode });
		applyPanMode();
	}

	async function toggleFitWidth() {
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
			const container = document.getElementById('pdf-container');
			if (container) {
				scrollStart = { x: container.scrollLeft, y: container.scrollTop };
			}
		}
	}

	function handleMouseMove(e: MouseEvent) {
		if (isDragging && settings.panMode) {
			e.preventDefault();
			const dx = e.clientX - dragStart.x;
			const dy = e.clientY - dragStart.y;
			const container = document.getElementById('pdf-container');
			if (container) {
				container.scrollLeft = scrollStart.x - dx;
				container.scrollTop = scrollStart.y - dy;
			}
		}
	}

	function handleMouseUp() {
		isDragging = false;
		isDraggingProgress = false;
	}

	let isDraggingProgress = $state(false);
	let pendingProgressPage = $state<number | null>(null);

	function handleProgressThumbMouseDown(e: MouseEvent) {
		e.preventDefault();
		e.stopPropagation();
		isDraggingProgress = true;
		pendingProgressPage = null;
		window.addEventListener('mousemove', handleProgressMouseMove);
		window.addEventListener('mouseup', handleProgressMouseUp);
	}

	function handleProgressMouseMove(e: MouseEvent) {
		if (!isDraggingProgress) return;

		const progressBar = document.querySelector('.progress-bar') as HTMLElement;
		if (!progressBar) return;

		const rect = progressBar.getBoundingClientRect();
		const x = e.clientX - rect.left;
		const percentage = Math.max(0, Math.min(1, x / rect.width));
		const newPage = Math.round(percentage * numPages);
		if (newPage >= 1 && newPage <= numPages) {
			pendingProgressPage = newPage;
			currentPage = newPage;
		}
	}

	function handleProgressMouseUp() {
		isDraggingProgress = false;
		window.removeEventListener('mousemove', handleProgressMouseMove);
		window.removeEventListener('mouseup', handleProgressMouseUp);

		if (pendingProgressPage !== null) {
			jumpToPage(pendingProgressPage);
			pendingProgressPage = null;
		}
	}

	function jumpToPage(pageNum: number) {
		if (pageNum >= 1 && pageNum <= numPages) {
			currentPage = pageNum;
			if (settings.scrollMode === 'continuous-vertical') {
				const canvas = pageCanvases.get(pageNum);
				if (canvas) {
					canvas.scrollIntoView({ behavior: 'auto', block: 'start' });
				}
			} else {
				renderPage(pageNum);
			}
			saveProgress();
		}
	}

	function handleProgressBarClick(e: MouseEvent) {
		const progressBar = document.querySelector('.progress-bar') as HTMLElement;
		if (!progressBar) return;

		const rect = progressBar.getBoundingClientRect();
		const x = e.clientX - rect.left;
		const percentage = Math.max(0, Math.min(1, x / rect.width));
		const newPage = Math.round(percentage * numPages);
		if (newPage >= 1 && newPage <= numPages) {
			jumpToPage(newPage);
		}
	}

	function handleProgressBarKeydown(e: KeyboardEvent) {
		if (numPages <= 0) return;

		if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') {
			e.preventDefault();
			jumpToPage(Math.max(1, currentPage - 1));
		} else if (e.key === 'ArrowRight' || e.key === 'ArrowUp') {
			e.preventDefault();
			jumpToPage(Math.min(numPages, currentPage + 1));
		} else if (e.key === 'Home') {
			e.preventDefault();
			jumpToPage(1);
		} else if (e.key === 'End') {
			e.preventDefault();
			jumpToPage(numPages);
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (isEditingPage) {
			if (e.key === 'Enter' || e.key === 'Escape') {
				finishEditPage();
			}
			return;
		}

		if (settings.scrollMode === 'continuous-vertical') {
			if (e.key === 'ArrowUp' || e.key === 'k') {
				e.preventDefault();
				if (currentPage > 1) {
					currentPage--;
					scrollToPage(currentPage);
					saveProgress();
				}
			} else if (e.key === 'ArrowDown' || e.key === 'j' || e.key === ' ') {
				e.preventDefault();
				if (currentPage < numPages) {
					currentPage++;
					scrollToPage(currentPage);
					saveProgress();
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
			updateSetting('zoomLevel', Math.min(settings.zoomLevel + 10, 400));
		} else if (e.key === '-') {
			updateSetting('zoomLevel', Math.max(settings.zoomLevel - 10, 25));
		}
	}

	function shouldIgnoreWheelNavigation(target: EventTarget | null) {
		if (!(target instanceof Element)) return false;
		return !!target.closest('input, textarea, select, [contenteditable="true"], .left-sidebar, .right-sidebar');
	}

	function handleWheelNavigation(e: WheelEvent) {
		if (settings.scrollMode !== 'paged' || settings.panMode || shouldIgnoreWheelNavigation(e.target)) {
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

		if (dominantDelta > 0) {
			nextPage();
		} else {
			prevPage();
		}
	}

	function toggleFullscreen() {
		if (!document.fullscreenElement) {
			document.documentElement.requestFullscreen();
		} else {
			document.exitFullscreen();
		}
	}

	async function closeReader(e?: Event) {
		e?.preventDefault();
		await saveProgress();
		await endSession();
		window.location.href = book ? `/book/${book.id}` : '/book';
	}

	async function performSearch() {
		if (!pdfDoc || !searchQuery.trim()) {
			searchResults = [];
			return;
		}

		isSearching = true;
		searchResults = [];

		for (let i = 1; i <= numPages; i++) {
			try {
				const page = await pdfDoc.getPage(i);
				const textContent = await page.getTextContent();
				const text = textContent.items.map((item: any) => item.str).join(' ');

				const query = matchCase ? searchQuery : searchQuery.toLowerCase();
				const searchText = matchCase ? text : text.toLowerCase();

				let index = 0;
				while ((index = searchText.indexOf(query, index)) !== -1) {
					searchResults.push({ page: i, index, text: text.substring(Math.max(0, index - 30), index + query.length + 30) });
					index += query.length;
				}
			} catch (e) {
				console.warn(`Search failed on page ${i}:`, e);
			}
		}

		currentSearchResult = searchResults.length > 0 ? 1 : 0;
		isSearching = false;
	}

	function prevSearchResult() {
		if (currentSearchResult > 1) {
			currentSearchResult--;
			goToPage(searchResults[currentSearchResult - 1].page);
		}
	}

	function nextSearchResult() {
		if (currentSearchResult < searchResults.length) {
			currentSearchResult++;
			goToPage(searchResults[currentSearchResult - 1].page);
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

	async function initReader() {
		if (!browser || !book) return;

		try {
			await fetchProgress();

			const pdfjsLib = await import('pdfjs-dist');
			const { getDocument, GlobalWorkerOptions } = pdfjsLib;
			const requestedFormat = normalizeBookFormat($page.url.searchParams.get('format'));

			GlobalWorkerOptions.workerSrc = `/pdf.worker.min.mjs`;

			const loadingTask = getDocument({
				url: `/api/books/${book.id}/file${requestedFormat ? `?format=${encodeURIComponent(requestedFormat)}` : ''}`,
				withCredentials: true,
				disableAutoFetch: true,
				disableRange: false,
				disableStream: true,
				rangeChunkSize: 262144,
				verbosity: 0
			});

			pdfDoc = await loadingTask.promise;
			numPages = pdfDoc.numPages;
			pdfInstance = pdfDoc;

			await tick();
			await tick();

			canvas = document.getElementById('pdf-canvas') as HTMLCanvasElement;
			scrollbar = document.getElementById('continuous-scrollbar') as HTMLDivElement;
			continuousContainer = document.getElementById('continuous-container') as HTMLDivElement;

			if (canvas || continuousContainer) {
				const firstPage = await pdfDoc.getPage(1);
				const baseViewport = firstPage.getViewport({ scale: 1 });
				pdfReady = true;

				const container = document.getElementById('pdf-container');
				if (container) {
					const containerWidth = container.clientWidth - 32;
					const containerHeight = container.clientHeight - 32;
					const isMobileViewport = window.matchMedia('(max-width: 768px)').matches;
					const autoZoomLevel = isMobileViewport
						? (containerWidth / baseViewport.width) * 100
						: Math.min(
								(containerWidth / baseViewport.width) * 100,
								(containerHeight / baseViewport.height) * 100,
								200
							);
					settings = { ...settings, zoomLevel: Math.round(autoZoomLevel) };
				}

				if (savedProgress && savedProgress.page > 0) {
					currentPage = savedProgress.page;
				}

				applyViewMode();
				applyPanMode();

				if (settings.scrollMode === 'continuous-vertical') {
					await renderAllPagesContinuous();
				} else {
					await renderPage(currentPage);
				}

				tick().then(() => {
					observeThumbnails();
					void loadPdfOutline();
				});
			} else {
				error = 'Failed to initialize viewer';
			}

		} catch (e) {
			console.error('Failed to initialize PDF reader:', e);
			error = `Failed to load PDF: ${e instanceof Error ? e.message : String(e)}`;
		}
	}

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

	async function saveProgress() {
		if (!book) return;
		const percent = numPages > 0 ? (currentPage / numPages) * 100 : 0;
		try {
			await fetch(`/api/books/${book.id}/progress`, {
				method: 'PUT',
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

	$effect(() => {
		if (book && !loading && !readerInitialized) {
			readerInitialized = true;
			tick().then(() => {
				initReader();
			});
		}
	});

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
			window.removeEventListener('resize', handleViewportResize);
			pdfContainerEl?.removeEventListener('touchstart', handleTouchStart);
			pdfContainerEl?.removeEventListener('touchmove', handleTouchMove);
			pdfContainerEl?.removeEventListener('touchend', handleTouchEnd);
			pdfContainerEl?.removeEventListener('touchcancel', handleTouchEnd);
			if (viewportResizeTimeout) {
				clearTimeout(viewportResizeTimeout);
			}
			if (touchPinchFrame !== null) {
				window.cancelAnimationFrame(touchPinchFrame);
			}
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
</script>

<svelte:head>
	<title>{book?.title || 'Reading'} - Cryptorum</title>
</svelte:head>

<div
	class="pdf-reader"
	style="background-color: {viewModeBgColors[settings.viewMode]};"
	role="application"
>
	<!-- Top Navigation Bar -->
	<header class="top-nav">
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

			<button
				onclick={toggleSearchSidebar}
				class="nav-btn"
				title="Search (Ctrl+F)"
			>
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="11" cy="11" r="8"></circle>
					<line x1="21" y1="21" x2="16.65" y2="16.65"></line>
				</svg>
			</button>

			<button
				onclick={togglePanMode}
				class="nav-btn"
				class:active={settings.panMode}
				title="Hand Tool (H)"
			>
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M18 11V6a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v0"></path>
					<path d="M14 10V4a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v2"></path>
					<path d="M10 10.5V6a2 2 0 0 0-2-2v0a2 2 0 0 0-2 2v8"></path>
					<path d="M18 8a2 2 0 1 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"></path>
				</svg>
			</button>

			<div class="nav-divider"></div>

			<div class="page-controls">
				<button onclick={prevPage} class="nav-btn" disabled={currentPage <= 1} title="Previous Page">
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<polyline points="15 18 9 12 15 6"></polyline>
					</svg>
				</button>

				{#if isEditingPage}
					<input
						type="number"
						bind:value={pageInputValue}
						onblur={finishEditPage}
						onkeydown={handlePageInputKeydown}
						class="page-input"
						min="1"
						max={numPages}
					/>
				{:else}
					<button onclick={startEditPage} class="page-display" title="Click to edit">
						{currentPage}
					</button>
				{/if}
				<span class="page-separator">/</span>
				<span class="page-total">{numPages}</span>

				<button onclick={nextPage} class="nav-btn" disabled={currentPage >= numPages} title="Next Page">
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<polyline points="9 18 15 12 9 6"></polyline>
					</svg>
				</button>
			</div>

			<div class="nav-divider"></div>

			<button onclick={rotateLeft} class="nav-btn" title="Rotate Left ([)">
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M2.5 2v6h6M2.66 15.57a10 10 0 1 0 .57-8.38"></path>
				</svg>
			</button>

			<button onclick={rotateRight} class="nav-btn" title="Rotate Right (])">
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M21.5 2v6h-6M21.34 15.57a10 10 0 1 1-.57-8.38"></path>
				</svg>
			</button>
		</div>

		<div class="nav-center">
			<span class="book-title">{book?.title || 'Loading...'}</span>
		</div>

		<div class="nav-right">
			<button
				onclick={toggleFitWidth}
				class="nav-btn fit-width-btn"
				class:active={fitWidthActive}
				title={fitWidthActive ? 'Restore Zoom' : 'Fit to Width (95%)'}
				aria-pressed={fitWidthActive}
			>
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M4 12h16"></path>
					<polyline points="8 8 4 12 8 16"></polyline>
					<polyline points="16 8 20 12 16 16"></polyline>
				</svg>
			</button>

			<button onclick={toggleRightSidebar} class="nav-btn" class:active={rightSidebarOpen} title="Text Settings">
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="3"></circle>
					<path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
				</svg>
			</button>

			<button onclick={toggleFullscreen} class="nav-btn" title="Fullscreen (F11)">
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<polyline points="15 3 21 3 21 9"></polyline>
					<polyline points="9 21 3 21 3 15"></polyline>
					<line x1="21" y1="3" x2="14" y2="10"></line>
					<line x1="3" y1="21" x2="10" y2="14"></line>
				</svg>
			</button>
		</div>

		<div
			class="progress-bar"
			onclick={(e) => handleProgressBarClick(e)}
			onkeydown={(e) => handleProgressBarKeydown(e)}
			role="slider"
			aria-label="Reading progress"
			aria-valuemin="1"
			aria-valuemax={numPages}
			aria-valuenow={currentPage}
			tabindex="0"
		>
			<div class="progress-fill" style="--progress: {progress}%;"></div>
			<div
				class="progress-thumb"
				style="left: calc({progress}% - 6px);"
				onmousedown={(e) => handleProgressThumbMouseDown(e)}
				role="presentation"
			></div>
		</div>
	</header>

	<!-- Main Content Area -->
	<div class="main-content">
		<!-- Left Sidebar -->
		<aside class="left-sidebar" class:open={leftSidebarOpen}>
			<div class="sidebar-tabs">
				<button
					class="sidebar-tab"
					class:active={activeSidebarTab === 'thumbnails'}
					onclick={() => activeSidebarTab = 'thumbnails'}
					title="Thumbnails"
				>
					<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<rect x="3" y="3" width="7" height="7"></rect>
						<rect x="14" y="3" width="7" height="7"></rect>
						<rect x="14" y="14" width="7" height="7"></rect>
						<rect x="3" y="14" width="7" height="7"></rect>
					</svg>
				</button>
				<button
					class="sidebar-tab"
					class:active={activeSidebarTab === 'bookmarks'}
					onclick={() => activeSidebarTab = 'bookmarks'}
					title="Bookmarks"
				>
					<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"></path>
					</svg>
				</button>
				<button
					class="sidebar-tab"
					class:active={activeSidebarTab === 'search'}
					onclick={() => activeSidebarTab = 'search'}
					title="Search"
				>
					<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<circle cx="11" cy="11" r="8"></circle>
						<line x1="21" y1="21" x2="16.65" y2="16.65"></line>
					</svg>
				</button>
			</div>

			<div class="sidebar-content">
				{#if activeSidebarTab === 'thumbnails'}
					<div class="thumbnails-panel" id="thumbnails-container">
						{#each Array(numPages) as _, i}
							<button
								onclick={() => goToPage(i + 1)}
								class="thumbnail-item"
								class:active={currentPage === i + 1}
								data-page={i + 1}
							>
								<canvas
									class="thumbnail-canvas"
									id="thumbnail-{i + 1}"
								></canvas>
								<span class="thumbnail-number">{i + 1}</span>
							</button>
						{/each}
					</div>
				{:else if activeSidebarTab === 'bookmarks'}
					<div class="bookmarks-panel">
						{#if pdfOutline.length > 0}
							<ul class="outline-list">
								{#each pdfOutline as item, i}
									<li class="outline-item">
										<div class="outline-row">
										{#if hasChildren(item)}
											<button
												class="outline-expand"
												onclick={() => toggleOutlineItem(getItemId(item, i))}
												aria-label={`Toggle ${item.title}`}
												title={`Toggle ${item.title}`}
											>
													<svg class="icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="transform: rotate({expandedItems.has(getItemId(item, i)) ? 90 : 0}deg);">
														<polyline points="9 18 15 12 9 6"></polyline>
													</svg>
												</button>
											{:else}
												<span class="outline-spacer"></span>
											{/if}
											<button onclick={() => goToOutlineItem(item)} class="outline-btn">
												{item.title}
											</button>
										</div>
										{#if hasChildren(item) && expandedItems.has(getItemId(item, i))}
											<ul class="outline-sublist">
												{#each item.items as subItem, j}
													<li class="outline-item">
														<div class="outline-row">
																{#if hasChildren(subItem)}
																	<button
																		class="outline-expand"
																		onclick={() => toggleOutlineItem(getItemId(subItem, j) + '-sub')}
																		aria-label={`Toggle ${subItem.title}`}
																		title={`Toggle ${subItem.title}`}
																	>
																	<svg class="icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="transform: rotate({expandedItems.has(getItemId(subItem, j) + '-sub') ? 90 : 0}deg);">
																		<polyline points="9 18 15 12 9 6"></polyline>
																	</svg>
																</button>
															{:else}
																<span class="outline-spacer"></span>
															{/if}
															<button onclick={() => goToOutlineItem(subItem)} class="outline-btn">
																{subItem.title}
															</button>
														</div>
														{#if hasChildren(subItem) && expandedItems.has(getItemId(subItem, j) + '-sub')}
															<ul class="outline-sublist">
																{#each subItem.items as subSubItem, k}
																	<li class="outline-item">
																		<div class="outline-row">
																			<span class="outline-spacer"></span>
																			<button onclick={() => goToOutlineItem(subSubItem)} class="outline-btn">
																				{subSubItem.title}
																			</button>
																		</div>
																	</li>
																{/each}
															</ul>
														{/if}
													</li>
												{/each}
											</ul>
										{/if}
									</li>
								{/each}
							</ul>
						{:else}
							<p class="empty-message">No table of contents</p>
						{/if}
					</div>
				{:else if activeSidebarTab === 'search'}
					<div class="search-panel">
						<div class="search-input-wrap">
							<input
								type="text"
								bind:value={searchQuery}
								onkeydown={(e) => e.key === 'Enter' && performSearch()}
								placeholder="Search..."
								class="search-input"
							/>
							<div class="search-count">
								{#if searchResults.length > 0}
									<span>{currentSearchResult}</span>
									<span class="sep">/</span>
									<span>{searchResults.length}</span>
								{/if}
							</div>
						</div>
						<label class="search-option">
							<input type="checkbox" bind:checked={matchCase} />
							<span>Match case</span>
						</label>
						<div class="search-nav">
							<button onclick={prevSearchResult} class="nav-btn-sm" disabled={currentSearchResult <= 1} aria-label="Previous search result" title="Previous search result">
								<svg class="icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
									<polyline points="18 15 12 9 6 15"></polyline>
								</svg>
							</button>
							<button onclick={nextSearchResult} class="nav-btn-sm" disabled={currentSearchResult >= searchResults.length} aria-label="Next search result" title="Next search result">
								<svg class="icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
									<polyline points="6 9 12 15 18 9"></polyline>
								</svg>
							</button>
						</div>
						{#if isSearching}
							<p class="search-status">Searching...</p>
						{:else if searchResults.length === 0 && searchQuery}
							<p class="search-status">No results found</p>
						{/if}
					</div>
				{/if}
			</div>
		</aside>

		<!-- PDF Container -->
			<div
				id="pdf-container"
				bind:this={pdfContainerEl}
				class="pdf-container"
				class:pan-mode={settings.panMode}
			>
			{#if loading}
				<div class="loading-state" aria-live="polite">
					<div class="loading-spinner"></div>
					<p>Loading PDF...</p>
				</div>
			{:else if error}
				<div class="error-message">
					<p>{error}</p>
					<a href="/book/{book?.id}" class="btn">Return to Library</a>
				</div>
			{:else if settings.scrollMode === 'paged'}
				<div id="paged-viewer" class="paged-viewer {settings.pageLayout === 'double' ? 'double' : ''}">
					{#if settings.pageLayout === 'double'}
						<canvas id="pdf-canvas-left" class="pdf-canvas"></canvas>
						<canvas id="pdf-canvas-right" class="pdf-canvas"></canvas>
					{:else}
						<canvas id="pdf-canvas" class="pdf-canvas"></canvas>
					{/if}
				</div>

				{#if settings.pageLayout === 'single'}
						<button
							class="floating-nav floating-prev"
							onclick={() => settings.readingDirection === 'rtl' ? nextPage() : prevPage()}
							disabled={settings.readingDirection === 'ltr' ? currentPage <= 1 : currentPage >= numPages}
							aria-label={settings.readingDirection === 'rtl' ? 'Next page' : 'Previous page'}
							title={settings.readingDirection === 'rtl' ? 'Next page' : 'Previous page'}
						>
						<svg class="icon-lg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polyline points="15 18 9 12 15 6"></polyline>
						</svg>
					</button>
						<button
							class="floating-nav floating-next"
							onclick={() => settings.readingDirection === 'rtl' ? prevPage() : nextPage()}
							disabled={settings.readingDirection === 'ltr' ? currentPage >= numPages : currentPage <= 1}
							aria-label={settings.readingDirection === 'rtl' ? 'Previous page' : 'Next page'}
							title={settings.readingDirection === 'rtl' ? 'Previous page' : 'Next page'}
						>
						<svg class="icon-lg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polyline points="9 18 15 12 9 6"></polyline>
						</svg>
					</button>
				{/if}
			{:else}
				<div
					id="continuous-scrollbar"
					class="continuous-scrollbar"
				>
					<div id="continuous-container" class="continuous-container">
					</div>
				</div>
			{/if}
		</div>

		<!-- Right Sidebar - Text Settings -->
		<aside class="right-sidebar" class:open={rightSidebarOpen}>
			<div class="settings-section">
				<h3 class="section-title">Mode</h3>
				<div class="mode-buttons">
					<button
						class="mode-btn"
						class:active={settings.viewMode === 'light'}
						onclick={() => updateSetting('viewMode', 'light')}
					>
						<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<circle cx="12" cy="12" r="5"></circle>
							<line x1="12" y1="1" x2="12" y2="3"></line>
							<line x1="12" y1="21" x2="12" y2="23"></line>
							<line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line>
							<line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line>
							<line x1="1" y1="12" x2="3" y2="12"></line>
							<line x1="21" y1="12" x2="23" y2="12"></line>
							<line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line>
							<line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line>
						</svg>
						Light
					</button>
					<button
						class="mode-btn"
						class:active={settings.viewMode === 'dark'}
						onclick={() => updateSetting('viewMode', 'dark')}
					>
						<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path>
						</svg>
						Dark
					</button>
					<button
						class="mode-btn"
						class:active={settings.viewMode === 'trueDark'}
						onclick={() => updateSetting('viewMode', 'trueDark')}
					>
						<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<circle cx="12" cy="12" r="10"></circle>
							<line x1="4.93" y1="4.93" x2="19.07" y2="19.07"></line>
						</svg>
						True Dark
					</button>
				</div>
			</div>

			<div class="settings-section">
				<h3 class="section-title">View Mode</h3>
				<div class="mode-buttons">
					<button
						class="mode-btn"
						class:active={settings.scrollMode === 'continuous-vertical'}
						onclick={() => updateSetting('scrollMode', 'continuous-vertical')}
					>
						<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<line x1="12" y1="5" x2="12" y2="19"></line>
							<polyline points="19 12 12 19 5 12"></polyline>
						</svg>
						Scroll
					</button>
					<button
						class="mode-btn"
						class:active={settings.scrollMode === 'paged'}
						onclick={() => updateSetting('scrollMode', 'paged')}
					>
						<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<rect x="4" y="2" width="16" height="20" rx="2"></rect>
							<line x1="12" y1="6" x2="12" y2="18"></line>
						</svg>
						Paged
					</button>
				</div>
			</div>

			<div class="settings-section">
				<h3 class="section-title">Page Layout</h3>
				<div class="mode-buttons">
					<button
						class="mode-btn"
						class:active={settings.pageLayout === 'single'}
						onclick={() => updateSetting('pageLayout', 'single')}
					>
						<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<rect x="4" y="2" width="16" height="20" rx="2"></rect>
						</svg>
						Single
					</button>
					<button
						class="mode-btn"
						class:active={settings.pageLayout === 'double'}
						onclick={() => updateSetting('pageLayout', 'double')}
					>
						<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<rect x="2" y="2" width="9" height="20" rx="1"></rect>
							<rect x="13" y="2" width="9" height="20" rx="1"></rect>
						</svg>
						Double
					</button>
				</div>
			</div>

			<div class="settings-section">
				<h3 class="section-title">Progress Bar</h3>
				<div class="toggle-options">
					<label class="toggle-option">
						<span>Chapter Markers</span>
						<input
							type="checkbox"
							checked={settings.showChapterMarkers}
							onchange={(e) => updateSetting('showChapterMarkers', e.currentTarget.checked)}
						/>
					</label>
					<label class="toggle-option">
						<span>Quote Marks</span>
						<input
							type="checkbox"
							checked={settings.showQuoteMarks}
							onchange={(e) => updateSetting('showQuoteMarks', e.currentTarget.checked)}
						/>
					</label>
				</div>
			</div>

			<div class="settings-section">
				<h3 class="section-title">Scaling</h3>
				<div class="scaling-controls">
					<button onclick={zoomOut} class="scale-btn" title="Zoom Out (-)">
						<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<line x1="5" y1="12" x2="19" y2="12"></line>
						</svg>
					</button>
					<span class="zoom-level">{settings.zoomLevel}%</span>
					<button onclick={zoomIn} class="scale-btn" title="Zoom In (+)">
						<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<line x1="12" y1="5" x2="12" y2="19"></line>
							<line x1="5" y1="12" x2="19" y2="12"></line>
						</svg>
					</button>
				</div>
				<button onclick={autoScale} class="auto-scale-btn">
					Auto Scaling
				</button>
			</div>
		</aside>
	</div>
</div>

<style>
	.pdf-reader {
		position: fixed;
		inset: 0;
		z-index: 9999;
		display: flex;
		flex-direction: column;
		font-family: system-ui, -apple-system, sans-serif;
		overflow: hidden;
	}

	.top-nav {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: space-between;
		height: 48px;
		padding: 0 12px;
		background: var(--color-surface-base, #0f172a);
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		flex-shrink: 0;
		z-index: 100;
	}

	.nav-left,
	.nav-center,
	.nav-right {
		display: flex;
		align-items: center;
		gap: 4px;
	}

	.nav-left {
		flex: 1;
	}

	.nav-center {
		flex: 2;
		justify-content: center;
	}

	.nav-right {
		flex: 1;
		justify-content: flex-end;
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

	.nav-btn.active {
		background: var(--color-primary-500, #22c55e);
		color: white;
	}

	.fit-width-btn {
		margin-right: 4px;
	}

	.nav-divider {
		width: 1px;
		height: 24px;
		background: var(--color-surface-border, rgba(55, 65, 81, 0.6));
		margin: 0 8px;
	}

	.icon {
		width: 20px;
		height: 20px;
	}

	.icon-sm {
		width: 16px;
		height: 16px;
	}

	.icon-xs {
		width: 12px;
		height: 12px;
	}

	.icon-lg {
		width: 24px;
		height: 24px;
	}

	.page-controls {
		display: flex;
		align-items: center;
		gap: 4px;
	}

	.page-display {
		min-width: 40px;
		padding: 4px 8px;
		border: none;
		border-radius: 4px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 14px;
		font-weight: 500;
		text-align: center;
		cursor: pointer;
	}

	.page-display:hover {
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
	}

	.page-input {
		width: 50px;
		padding: 4px 8px;
		border: 1px solid var(--color-primary-500, #22c55e);
		border-radius: 4px;
		background: var(--color-surface-base, #0f172a);
		color: var(--color-surface-text, #e2e8f0);
		font-size: 14px;
		text-align: center;
		outline: none;
	}

	.page-separator {
		color: var(--color-surface-text-muted, #94a3b8);
		margin: 0 2px;
	}

	.page-total {
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 14px;
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
		padding-bottom: 0;
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
		transition: width 0.1s ease;
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
		transition: left 0.05s ease, transform 0.1s ease;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
	}

	.progress-thumb:hover {
		transform: scale(1.2);
	}

	.progress-thumb:active {
		cursor: grabbing;
		transform: scale(1.1);
	}

	.main-content {
		flex: 1;
		display: flex;
		overflow: hidden;
		position: relative;
	}

	.left-sidebar {
		position: absolute;
		left: 0;
		top: 0;
		bottom: 0;
		width: 300px;
		display: flex;
		flex-direction: column;
		background: var(--color-surface-base, #0f172a);
		border-right: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		flex-shrink: 0;
		transform: translateX(-100%);
		transition: transform 0.25s ease-in-out;
		z-index: 50;
	}

	.left-sidebar.open {
		transform: translateX(0);
	}

	.sidebar-tabs {
		display: flex;
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
	}

	.sidebar-tab {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 12px;
		border: none;
		background: transparent;
		color: var(--color-surface-text-muted, #94a3b8);
		cursor: pointer;
		transition: all 0.15s;
	}

	.sidebar-tab:hover {
		color: var(--color-surface-text, #e2e8f0);
	}

	.sidebar-tab.active {
		color: var(--color-primary-500, #22c55e);
		box-shadow: inset 0 -2px 0 var(--color-primary-500, #22c55e);
	}

	.sidebar-content {
		flex: 1;
		overflow-y: auto;
	}

	.thumbnails-panel {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 8px;
		padding: 8px;
	}

	.thumbnail-item {
		aspect-ratio: 3/4;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		border: 2px solid transparent;
		border-radius: 4px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 10px;
		cursor: pointer;
		transition: all 0.15s;
		overflow: hidden;
		position: relative;
	}

	.thumbnail-item:hover {
		border-color: var(--color-surface-border, rgba(55, 65, 81, 0.6));
	}

	.thumbnail-item.active {
		border-color: var(--color-primary-500, #22c55e);
	}

	.thumbnail-canvas {
		width: 100%;
		height: calc(100% - 16px);
		object-fit: contain;
	}

	.thumbnail-number {
		position: absolute;
		bottom: 2px;
		font-size: 10px;
	}

	.bookmarks-panel {
		padding: 12px;
	}

	.outline-list {
		list-style: none;
		padding: 0;
		margin: 0;
	}

	.outline-item {
		margin-bottom: 4px;
		list-style: none;
	}

	.outline-list {
		padding: 0;
		margin: 0;
	}

	.outline-sublist {
		padding-left: 16px;
		margin: 4px 0 0 0;
		list-style: none;
	}

	.outline-row {
		display: flex;
		align-items: center;
		gap: 4px;
	}

	.outline-expand {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 20px;
		height: 20px;
		padding: 0;
		border: none;
		border-radius: 4px;
		background: transparent;
		color: var(--color-surface-text-muted, #94a3b8);
		cursor: pointer;
		transition: all 0.15s;
		flex-shrink: 0;
	}

	.outline-expand:hover {
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text, #e2e8f0);
	}

	.outline-expand svg {
		transition: transform 0.15s ease;
	}

	.outline-spacer {
		width: 20px;
		flex-shrink: 0;
	}

	.outline-btn {
		flex: 1;
		text-align: left;
		padding: 6px 8px;
		border: none;
		border-radius: 4px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 13px;
		cursor: pointer;
		transition: background-color 0.15s;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.outline-btn:hover {
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
	}

	.outline-btn:active {
		background: var(--color-primary-500, #22c55e);
	}

	.empty-message {
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 13px;
		text-align: center;
		padding: 20px;
	}

	.search-input-wrap {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 8px;
	}

	.search-input {
		flex: 1;
		padding: 8px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 4px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text, #e2e8f0);
		font-size: 13px;
	}

	.search-input:focus {
		outline: none;
		border-color: var(--color-primary-500, #22c55e);
	}

	.search-count {
		font-size: 12px;
		color: var(--color-surface-text-muted, #94a3b8);
		white-space: nowrap;
	}

	.search-count .sep {
		margin: 0 2px;
	}

	.search-option {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		color: var(--color-surface-text, #e2e8f0);
		cursor: pointer;
		margin-bottom: 8px;
	}

	.search-nav {
		display: flex;
		gap: 4px;
	}

	.nav-btn-sm {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		border: none;
		border-radius: 4px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text, #e2e8f0);
		cursor: pointer;
	}

	.nav-btn-sm:hover:not(:disabled) {
		background: var(--color-primary-500, #22c55e);
	}

	.nav-btn-sm:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}

	.search-status {
		font-size: 12px;
		color: var(--color-surface-text-muted, #94a3b8);
		text-align: center;
		margin-top: 8px;
	}

	.pdf-container {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		overflow: auto;
		padding: 24px;
		position: relative;
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

	.pdf-container.pan-mode {
		cursor: grab;
	}

	.pdf-container {
		touch-action: pan-x pan-y;
	}

	.pdf-container.pan-mode:active {
		cursor: grabbing;
	}

	.paged-viewer {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 16px;
	}

	.paged-viewer.double {
		flex-direction: row;
		gap: 24px;
	}

	.pdf-canvas {
		box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4);
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

	.floating-nav:hover:not(:disabled) {
		background: var(--color-primary-500, #22c55e);
	}

	.floating-nav:disabled {
		opacity: 0.2;
		cursor: not-allowed;
	}

	.floating-prev {
		left: 16px;
	}

	.floating-next {
		right: 16px;
	}

	.continuous-scrollbar {
		width: 100%;
		height: 100%;
		overflow-y: auto;
		overflow-x: auto;
		touch-action: pan-x pan-y;
	}

	.continuous-container {
		margin: 0 auto;
		width: fit-content;
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
		top: 0;
		bottom: 0;
		width: 400px;
		background: var(--color-surface-base, #0f172a);
		border-left: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		overflow-y: auto;
		flex-shrink: 0;
		transform: translateX(100%);
		transition: transform 0.25s ease-in-out;
		z-index: 50;
	}

	.right-sidebar.open {
		transform: translateX(0);
	}

	.settings-section {
		padding: 16px;
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
	}

	.section-title {
		font-size: 11px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.5px;
		color: var(--color-surface-text-muted, #94a3b8);
		margin-bottom: 12px;
	}

	.mode-buttons {
		display: flex;
		gap: 8px;
	}

	.mode-btn {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 6px;
		padding: 12px 8px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 8px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 11px;
		cursor: pointer;
		transition: all 0.15s;
	}

	.mode-btn:hover {
		border-color: var(--color-primary-500, #22c55e);
	}

	.mode-btn.active {
		border-color: var(--color-primary-500, #22c55e);
		background: rgba(34, 197, 94, 0.1);
	}

	.toggle-options {
		display: flex;
		flex-direction: column;
		gap: 8px;
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

	.scaling-controls {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 16px;
		margin-bottom: 12px;
	}

	.scale-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 8px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		cursor: pointer;
		transition: all 0.15s;
	}

	.scale-btn:hover {
		border-color: var(--color-primary-500, #22c55e);
		background: rgba(34, 197, 94, 0.1);
	}

	.zoom-level {
		font-size: 16px;
		font-weight: 500;
		color: var(--color-surface-text, #e2e8f0);
		min-width: 60px;
		text-align: center;
	}

	.auto-scale-btn {
		width: 100%;
		padding: 10px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 6px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 13px;
		cursor: pointer;
		transition: all 0.15s;
	}

	.auto-scale-btn:hover {
		border-color: var(--color-primary-500, #22c55e);
		background: rgba(34, 197, 94, 0.1);
	}

	:global(.pdf-page-wrapper) {
		margin-bottom: 16px;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.18);
	}

	:global(.pdf-page-canvas) {
		display: block;
	}

	@media (max-width: 768px) {
		.top-nav {
			height: 44px;
			padding: 0 8px;
		}

		.nav-left,
		.nav-right {
			gap: 2px;
			min-width: 0;
		}

		.nav-center,
		.nav-divider {
			display: none;
		}

		.nav-btn {
			width: 32px;
			height: 32px;
		}

		.icon {
			width: 18px;
			height: 18px;
		}

		.icon-lg {
			width: 20px;
			height: 20px;
		}

		.page-controls {
			gap: 2px;
		}

		.progress-bar {
			height: 10px;
		}

		.main-content {
			min-width: 0;
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

		.pdf-container {
			padding: 12px 8px 20px;
			min-width: 0;
			overflow-x: hidden;
		}

		.paged-viewer {
			width: 100%;
			max-width: 100%;
			gap: 12px;
		}

		.paged-viewer.double {
			flex-direction: column;
		}

		.pdf-canvas {
			max-width: 100%;
			height: auto;
			box-shadow: none;
		}

		:global(.pdf-page-wrapper) {
			box-shadow: none;
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

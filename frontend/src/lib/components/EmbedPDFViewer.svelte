<script lang="ts">
	import { onDestroy } from 'svelte';
	import { PDFViewer } from '@embedpdf/svelte-pdf-viewer';
	import type { EmbedPdfContainer, PluginRegistry } from '@embedpdf/snippet';
	import type { ScrollCapability } from '@embedpdf/plugin-scroll/preact';
	import type { ZoomCapability, ZoomScope } from '@embedpdf/plugin-zoom/preact';
	import type { DocumentManagerCapability } from '@embedpdf/plugin-document-manager/preact';
	import type { IPlugin } from '@embedpdf/core';
	import { createEmbedPdfChromePatchController } from './embedpdf/chromePatch';
	import {
		EMBEDPDF_DOCUMENT_ID,
		embedPdfSelectors,
		embedPdfTheme,
		readingOnlyDisabledCategories
	} from './embedpdf/constants';
	import { eventPathIncludesSelector } from './embedpdf/dom';
	import { focusEmbedPdfSearchInput, getEmbedPdfSearchInput } from './embedpdf/search';
	import { createEmbedPdfScrollActivityController } from './embedpdf/scrollActivity';

	interface Props {
		src: string;
		title?: string;
		initialPage?: number;
		toolbarVisible?: boolean;
		onScrollActivity?: (delta: number, scrollTop: number) => void;
		onPageChange?: (page: number, totalPages: number) => void;
		onReady?: (registry: PluginRegistry) => void;
		onSidebarOpenChange?: (open: boolean, state?: EmbedPdfSidebarState) => void;
		onDocumentCenterTap?: () => void;
		onError?: (error: string) => void;
		style?: string;
	}

	interface EmbedPdfSidebarState {
		open: boolean;
		leftOpen: boolean;
		rightOpen: boolean;
		searchOpen: boolean;
		thumbnailOpen: boolean;
	}

	let {
		src,
		title = '',
		initialPage = 1,
		toolbarVisible = true,
		onScrollActivity,
		onPageChange,
		onReady,
		onSidebarOpenChange,
		onDocumentCenterTap,
		onError,
		style = 'height: 100%; width: 100%;'
	}: Props = $props();

	let initError = $state<string | null>(null);
	let restoredInitialPage = false;
	let layoutReadySeen = false;
	let restoreAttempts = 0;
	let unsubscribeScroll: (() => void) | null = null;
	let unsubscribeLayoutReady: (() => void) | null = null;
	let unsubscribeSidebarChange: (() => void) | null = null;
	let restoreTimers: ReturnType<typeof setTimeout>[] = [];
	let embedPdfContainerEl = $state<HTMLDivElement | null>(null);
	let searchFocusTimers: ReturnType<typeof setTimeout>[] = [];
	let searchFocusObserver: MutationObserver | null = null;
	let searchInputWasVisible = false;
	let embedPdfSidebarOpen = false;
	let lastSidebarState: EmbedPdfSidebarState | null = null;
	let embedPdfZoomScope: ZoomScope | null = null;
	let embedPdfUiScope: any = null;
	let embedPdfThumbnailScope: any = null;
	let embedPdfCurrentPage = $state(1);

	const documentId = EMBEDPDF_DOCUMENT_ID;
	const maxRestoreAttempts = 48;
	const MIN_PDF_ZOOM = 0.25;
	const MAX_PDF_ZOOM = 5;
	const scrollActivity = createEmbedPdfScrollActivityController((delta, scrollTop) => {
		onScrollActivity?.(delta, scrollTop);
	});
	const chromePatch = createEmbedPdfChromePatchController({
		getRoot: () => embedPdfContainerEl,
		getToolbarVisible: () => toolbarVisible,
		getSidebarOpen: () => embedPdfSidebarOpen,
		getScrollTargetCount: () => scrollActivity.targetCount,
		updateScrollListeners: () => scrollActivity.update(embedPdfContainerEl)
	});

	function clearRestoreTimers() {
		restoreTimers.forEach((timer) => clearTimeout(timer));
		restoreTimers = [];
	}

	function clearSearchFocusTimers() {
		searchFocusTimers.forEach((timer) => clearTimeout(timer));
		searchFocusTimers = [];
	}

	function scheduleSearchInputFocus(selectText = true) {
		if (typeof window === 'undefined') return;

		clearSearchFocusTimers();
		if (focusEmbedPdfSearchInput(embedPdfContainerEl, selectText)) return;

		const delays = [0, 40, 100, 200, 350];
		searchFocusTimers = delays.map((delay) =>
			setTimeout(() => {
				if (focusEmbedPdfSearchInput(embedPdfContainerEl, selectText)) {
					clearSearchFocusTimers();
				}
			}, delay)
		);
	}

	function isSearchSidebarOpen() {
		return !!embedPdfUiScope?.isSidebarOpen?.('right', 'main', 'search-panel');
	}

	function toggleEmbedPdfSearchSidebar() {
		if (!embedPdfUiScope) return false;

		if (isSearchSidebarOpen()) {
			clearSearchFocusTimers();
			embedPdfUiScope.closeSidebarSlot('right', 'main');
		} else {
			embedPdfUiScope.setActiveSidebar('right', 'main', 'search-panel');
			scheduleSearchInputFocus(true);
		}

		return true;
	}

	function handleSearchShortcutKeydown(event: KeyboardEvent) {
		if (
			(event.ctrlKey || event.metaKey) &&
			!event.altKey &&
			event.key.toLowerCase() === 'f'
		) {
			if (toggleEmbedPdfSearchSidebar()) {
				event.preventDefault();
				event.stopPropagation();
				event.stopImmediatePropagation();
			} else {
				scheduleSearchInputFocus(true);
			}
		}
	}

	function isSearchToggleEvent(event: Event) {
		return eventPathIncludesSelector(event, embedPdfSelectors.searchToggle);
	}

	function isEmbedPdfChromeOrSidebarEvent(event: Event) {
		return eventPathIncludesSelector(event, embedPdfSelectors.chromeOrSidebar);
	}

	function isEmbedPdfCenterTap(event: PointerEvent) {
		if (!embedPdfContainerEl) return false;

		const rect = embedPdfContainerEl.getBoundingClientRect();
		if (rect.width <= 0 || rect.height <= 0) return false;

		const xRatio = (event.clientX - rect.left) / rect.width;
		const yRatio = (event.clientY - rect.top) / rect.height;

		return xRatio > 0.28 && xRatio < 0.72 && yRatio > 0.12 && yRatio < 0.88;
	}

	function closeOpenEmbedPdfSidebars() {
		if (!embedPdfUiScope) return false;

		let closed = false;
		if (embedPdfUiScope.isSidebarOpen?.('left', 'main')) {
			embedPdfUiScope.closeSidebarSlot('left', 'main');
			closed = true;
		}
		if (embedPdfUiScope.isSidebarOpen?.('right', 'main')) {
			embedPdfUiScope.closeSidebarSlot('right', 'main');
			closed = true;
		}
		return closed;
	}

	function handleEmbedPdfClick(event: MouseEvent) {
		if (isSearchToggleEvent(event)) {
			scheduleSearchInputFocus(true);
		}
	}

	function handleEmbedPdfPointerUp(event: PointerEvent) {
		if (!event.isPrimary || isEmbedPdfChromeOrSidebarEvent(event)) return;

		if (closeOpenEmbedPdfSidebars()) return;

		if (isEmbedPdfCenterTap(event)) {
			onDocumentCenterTap?.();
		}
	}

	function updateSearchInputVisibility() {
		const isVisible = !!getEmbedPdfSearchInput(embedPdfContainerEl);
		if (isVisible && !searchInputWasVisible) {
			scheduleSearchInputFocus(true);
		}
		searchInputWasVisible = isVisible;
	}

	function startSearchFocusObserver() {
		if (
			typeof MutationObserver === 'undefined' ||
			!embedPdfContainerEl ||
			searchFocusObserver
		) {
			return;
		}

		searchInputWasVisible = !!getEmbedPdfSearchInput(embedPdfContainerEl);
		searchFocusObserver = new MutationObserver(updateSearchInputVisibility);
		searchFocusObserver.observe(embedPdfContainerEl, {
			childList: true,
			subtree: true,
			attributes: true,
			attributeFilter: ['class', 'style', 'hidden', 'aria-hidden']
		});
	}

	function scheduleThumbnailSidebarJump() {
		if (typeof window === 'undefined' || !embedPdfThumbnailScope) return;

		for (const delay of [0, 40, 120]) {
			setTimeout(() => {
				embedPdfThumbnailScope?.scrollToThumb?.(Math.max(0, embedPdfCurrentPage - 1));
			}, delay);
		}
	}

	function handleEmbedPdfWheelZoom(event: WheelEvent) {
		if ((!event.ctrlKey && !event.metaKey) || !embedPdfZoomScope) return;

		event.preventDefault();
		event.stopPropagation();
		event.stopImmediatePropagation();

		if (event.deltaY > 0) {
			embedPdfZoomScope.zoomOut();
		} else if (event.deltaY < 0) {
			embedPdfZoomScope.zoomIn();
		}
	}

	function getCapability<T>(registry: PluginRegistry, pluginId: string): T | null {
		const plugin = registry.getPlugin<IPlugin>(pluginId);
		return (plugin?.provides?.() as T | undefined) ?? null;
	}

	function getEmbedPdfSidebarState(uiScope: any): EmbedPdfSidebarState {
		const activeSidebars = uiScope?.getState?.()?.activeSidebars ?? {};
		const leftOpen = !!uiScope?.isSidebarOpen?.('left', 'main');
		const rightOpen = !!uiScope?.isSidebarOpen?.('right', 'main');
		const searchOpen = !!uiScope?.isSidebarOpen?.('right', 'main', 'search-panel');
		const thumbnailOpen = !!uiScope?.isSidebarOpen?.('left', 'main', 'sidebar-panel');

		return {
			open: leftOpen || rightOpen || Object.values(activeSidebars).some((sidebar: any) => !!sidebar?.isOpen),
			leftOpen,
			rightOpen,
			searchOpen,
			thumbnailOpen
		};
	}

	function setEmbedPdfSidebarOpen(state: EmbedPdfSidebarState) {
		if (
			lastSidebarState &&
			lastSidebarState.open === state.open &&
			lastSidebarState.leftOpen === state.leftOpen &&
			lastSidebarState.rightOpen === state.rightOpen &&
			lastSidebarState.searchOpen === state.searchOpen &&
			lastSidebarState.thumbnailOpen === state.thumbnailOpen
		) {
			return;
		}
		lastSidebarState = state;
		embedPdfSidebarOpen = state.open;
		onSidebarOpenChange?.(state.open, state);
		chromePatch.schedule();
	}

	function reportMeasuredPage(
		scopedScroll: ReturnType<ScrollCapability['forDocument']>,
		totalPages?: number
	) {
		const targetPage = Math.max(1, Math.floor(initialPage || 1));
		let measuredPage: number;

		try {
			// EmbedPDF updates getCurrentPage() before its deferred DOM scroll runs.
			// Viewport-derived metrics reflect the page that is actually on screen.
			measuredPage = scopedScroll.getMetrics().currentPage;
		} catch {
			return false;
		}

		if (!restoredInitialPage && targetPage > 1) {
			if (measuredPage !== Math.min(targetPage, totalPages || targetPage)) {
				return false;
			}

			restoredInitialPage = true;
			clearRestoreTimers();
		}

		embedPdfCurrentPage = measuredPage;
		onPageChange?.(measuredPage, totalPages || scopedScroll.getTotalPages());
		return true;
	}

	function restoreInitialPage(scroll: ScrollCapability, totalPages?: number, delay = 0) {
		const targetPage = Math.max(1, Math.floor(initialPage || 1));
		if (restoredInitialPage || targetPage <= 1) return;

		const clampedPage = totalPages && totalPages > 0
			? Math.min(targetPage, totalPages)
			: targetPage;

		const retryRestore = () => {
			if (restoreAttempts < maxRestoreAttempts) {
				const retryDelay = restoreAttempts < 10 ? 120 : restoreAttempts < 24 ? 250 : 500;
				restoreInitialPage(scroll, totalPages, retryDelay);
			}
		};

		const timer = setTimeout(() => {
			if (restoredInitialPage) return;

			restoreAttempts += 1;
			if (!layoutReadySeen) {
				retryRestore();
				return;
			}

			let scopedScroll: ReturnType<ScrollCapability['forDocument']>;
			try {
				scopedScroll = scroll.forDocument(documentId);
				scopedScroll.scrollToPage({
					pageNumber: clampedPage,
					behavior: 'auto',
					alignY: 0
				});
			} catch {
				retryRestore();
				return;
			}

			const verifyTimer = setTimeout(() => {
				if (reportMeasuredPage(scopedScroll, totalPages)) return;
				retryRestore();
			}, 100);
			restoreTimers.push(verifyTimer);
		}, delay);

		restoreTimers.push(timer);
	}

	function handleInit(_container: EmbedPdfContainer) {
		chromePatch.queueBurst();
	}

	function handleReady(registry: PluginRegistry) {
		chromePatch.startObserver();
		chromePatch.queueBurst();

		const scroll = getCapability<ScrollCapability>(registry, 'scroll');
		const zoom = getCapability<ZoomCapability>(registry, 'zoom');
		const thumbnail = getCapability<any>(registry, 'thumbnail');
		const documentManager = getCapability<DocumentManagerCapability>(registry, 'document-manager');
		const ui = getCapability<any>(registry, 'ui');
		embedPdfZoomScope = zoom?.forDocument?.(documentId) ?? null;
		embedPdfThumbnailScope = thumbnail?.forDocument?.(documentId) ?? null;

		if (scroll) {
			const scopedScroll = scroll.forDocument(documentId);
			unsubscribeScroll?.();
			unsubscribeScroll = scopedScroll.onScroll(() => {
				reportMeasuredPage(scopedScroll, scopedScroll.getTotalPages());
			});

			unsubscribeLayoutReady?.();
			unsubscribeLayoutReady = scroll.onLayoutReady((event) => {
				if (event.isInitial) {
					layoutReadySeen = true;
					restoreInitialPage(scroll, event.totalPages);
				}
			});

			registry.pluginsReady().then(() => {
				const activeDocument = documentManager?.getActiveDocument?.();
				const timer = setTimeout(() => {
					layoutReadySeen = true;
					restoreInitialPage(scroll, activeDocument?.pageCount);
				}, 1000);
				restoreTimers.push(timer);
			});
		}

		if (ui?.forDocument) {
			const uiScope = ui.forDocument(documentId);
			embedPdfUiScope = uiScope;
			unsubscribeSidebarChange?.();
			setEmbedPdfSidebarOpen(getEmbedPdfSidebarState(uiScope));
			unsubscribeSidebarChange = uiScope.onSidebarChanged?.((event: any) => {
				setEmbedPdfSidebarOpen(getEmbedPdfSidebarState(uiScope));
				if (event?.sidebarId === 'search-panel' && uiScope.isSidebarOpen?.('right', 'main', 'search-panel')) {
					scheduleSearchInputFocus(true);
				}
				if (event?.sidebarId === 'sidebar-panel' && uiScope.isSidebarOpen?.('left', 'main', 'sidebar-panel')) {
					scheduleThumbnailSidebarJump();
				}
			}) ?? null;
		}

		if (onReady) {
			onReady(registry);
		}
	}

	function handleViewerError(e: any) {
		console.error('EmbedPDF viewer error:', e);
		const msg = e?.message || String(e) || 'Unknown error';
		initError = msg;
		if (onError) {
			onError(msg);
		}
	}

	onDestroy(() => {
		unsubscribeScroll?.();
		unsubscribeLayoutReady?.();
		unsubscribeSidebarChange?.();
		embedPdfZoomScope = null;
		embedPdfUiScope = null;
		embedPdfThumbnailScope = null;
		if (embedPdfSidebarOpen) {
			onSidebarOpenChange?.(false);
		}
		clearRestoreTimers();
		clearSearchFocusTimers();
		scrollActivity.detach();
		chromePatch.disconnect();
		searchFocusObserver?.disconnect();
	});

	$effect(() => {
		if (embedPdfContainerEl) {
			chromePatch.startObserver();
			startSearchFocusObserver();
		}
	});

	$effect(() => {
		embedPdfCurrentPage = initialPage;
	});

	$effect(() => {
		if (typeof window === 'undefined') return;

		window.addEventListener('keydown', handleSearchShortcutKeydown, { capture: true });
		return () => {
			window.removeEventListener('keydown', handleSearchShortcutKeydown, { capture: true });
		};
	});

	$effect(() => {
		const element = embedPdfContainerEl;
		if (!element) return;

		element.addEventListener('wheel', handleEmbedPdfWheelZoom, {
			capture: true,
			passive: false
		});
		element.addEventListener('click', handleEmbedPdfClick);
		element.addEventListener('pointerup', handleEmbedPdfPointerUp, { capture: true });

		return () => {
			element.removeEventListener('wheel', handleEmbedPdfWheelZoom, { capture: true });
			element.removeEventListener('click', handleEmbedPdfClick);
			element.removeEventListener('pointerup', handleEmbedPdfPointerUp, { capture: true });
		};
	});

	$effect(() => {
		toolbarVisible;
		chromePatch.schedule();
	});
</script>

{#if initError}
	<div class="embedpdf-error" {style}>
		<p>Failed to initialize PDF viewer</p>
		<p class="error-detail">{initError}</p>
	</div>
{:else if src}
	<div class="embedpdf-container" bind:this={embedPdfContainerEl} onerror={handleViewerError}>
		{#if title}
			<div class="embedpdf-document-title" title={title}>
				<span class="embedpdf-document-title-text">{title}</span>
			</div>
		{/if}
		<PDFViewer
			config={{
				worker: true,
				theme: embedPdfTheme,
				documentManager: {
					initialDocuments: [
						{
							url: src,
							documentId,
							autoActivate: true,
							mode: 'range-request',
							requestOptions: {
								credentials: 'same-origin'
							}
						}
					],
					maxDocuments: 1
				},
				scroll: {
					defaultBufferSize: 10,
					defaultPageGap: 8
				},
				zoom: {
					minZoom: MIN_PDF_ZOOM,
					maxZoom: MAX_PDF_ZOOM
				},
				render: {
					withAnnotations: false,
					withForms: false,
					defaultImageType: 'image/png',
					defaultImageQuality: 1
				},
				tiling: {
					tileSize: 768,
					overlapPx: 3,
					extraRings: 2,
					defaultImageType: 'image/png'
				},
				thumbnails: {
					scrollBehavior: 'instant'
				},
				disabledCategories: readingOnlyDisabledCategories
			}}
			oninit={handleInit}
			onready={handleReady}
			{style}
		/>
	</div>
{:else}
	<div class="embedpdf-no-source" {style}>
		<p>No PDF source provided</p>
	</div>
{/if}

<style>
	.embedpdf-container {
		--embedpdf-left-clearance: 56px;
		--embedpdf-right-clearance: 56px;
		--embedpdf-shell-title-width: clamp(180px, 42vw, 520px);
		position: relative;
		width: 100%;
		height: 100%;
		overflow: hidden;
	}

	.embedpdf-container :global(img),
	.embedpdf-container :global(canvas) {
		image-rendering: crisp-edges;
		image-rendering: -webkit-optimize-contrast;
	}

	.embedpdf-container :global([data-epdf-i="left-group"]) {
		margin-left: calc(var(--embedpdf-left-clearance) + max(0px, env(safe-area-inset-left))) !important;
		margin-right: auto !important;
		flex-shrink: 0;
		transition: opacity 0.18s ease, transform 0.18s ease;
	}

	.embedpdf-container :global([data-epdf-i="center-group"]) {
		margin-left: 0 !important;
		transform: none !important;
		flex-shrink: 0;
		transition: opacity 0.18s ease, transform 0.18s ease;
	}

	.embedpdf-container :global([data-epdf-i="right-group"]) {
		position: relative;
		z-index: 25;
		margin-right: calc(var(--embedpdf-right-clearance) + max(0px, env(safe-area-inset-right)));
		transition: opacity 0.18s ease, transform 0.18s ease;
	}

	.embedpdf-container :global([data-epdf-i="comment-button"]) {
		display: none !important;
	}

	.embedpdf-container :global([data-epdf-i="document-menu-button"]),
	.embedpdf-container :global([data-epdf-cat~="document-menu"]),
	.embedpdf-container :global(button[aria-label="Document Menu"]),
	.embedpdf-container :global([data-epdf-i="divider-1"]) {
		display: none !important;
		visibility: hidden !important;
		pointer-events: none !important;
	}

	.embedpdf-container :global([data-overlay-id="page-controls"]) {
		transition: opacity 0.18s ease, transform 0.18s ease, visibility 0.18s ease;
		opacity: 0 !important;
		visibility: hidden !important;
		pointer-events: none !important;
		transform: translateY(8px) !important;
	}

	.embedpdf-document-title {
		display: none;
	}

	.embedpdf-error {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		height: 100%;
		color: var(--color-surface-text, #e2e8f0);
		padding: 20px;
		text-align: center;
	}

	.embedpdf-error p {
		margin: 0 0 8px;
	}

	.error-detail {
		font-size: 12px;
		color: var(--color-surface-text-muted, #94a3b8);
	}

	.embedpdf-no-source {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		color: var(--color-surface-text-muted, #94a3b8);
	}

	@media (max-width: 640px) {
		.embedpdf-container {
			--embedpdf-right-clearance: 56px;
			--embedpdf-shell-title-width: clamp(140px, 38vw, 260px);
		}

		.embedpdf-container :global([data-epdf-i="right-group"]) {
			margin-right: calc(var(--embedpdf-right-clearance) + max(0px, env(safe-area-inset-right)));
		}
	}

	@media (max-width: 420px) {
		.embedpdf-container {
			--embedpdf-shell-title-width: 108px;
		}
	}
</style>

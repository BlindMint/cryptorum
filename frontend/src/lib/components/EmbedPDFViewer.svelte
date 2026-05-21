<script lang="ts">
	import { onDestroy } from 'svelte';
	import { PDFViewer } from '@embedpdf/svelte-pdf-viewer';
	import type { EmbedPdfContainer, PluginRegistry } from '@embedpdf/snippet';
	import type {
		PageChangeEvent,
		ScrollCapability
	} from '@embedpdf/plugin-scroll/preact';
	import type { ZoomCapability, ZoomScope } from '@embedpdf/plugin-zoom/preact';
	import type { DocumentManagerCapability } from '@embedpdf/plugin-document-manager/preact';
	import type { IPlugin } from '@embedpdf/core';

	interface Props {
		src: string;
		title?: string;
		initialPage?: number;
		toolbarVisible?: boolean;
		onScrollActivity?: (delta: number, scrollTop: number) => void;
		onPageChange?: (page: number, totalPages: number) => void;
		onReady?: (registry: PluginRegistry) => void;
		onSidebarOpenChange?: (open: boolean) => void;
		onDocumentCenterTap?: () => void;
		onError?: (error: string) => void;
		style?: string;
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

	const readingOnlyDisabledCategories = [
		'mode',
		'annotation',
		'redaction',
		'insert',
		'form',
		'stamp',
		'stamps',
		'signature',
		'history'
	];

	let initError = $state<string | null>(null);
	let restoredInitialPage = false;
	let layoutReadySeen = false;
	let restoreAttempts = 0;
	let unsubscribePageChange: (() => void) | null = null;
	let unsubscribeLayoutReady: (() => void) | null = null;
	let unsubscribeSidebarChange: (() => void) | null = null;
	let restoreTimers: ReturnType<typeof setTimeout>[] = [];
	let embedPdfContainerEl = $state<HTMLDivElement | null>(null);
	let chromePatchObserver: MutationObserver | null = null;
	let chromePatchTimer: ReturnType<typeof setTimeout> | null = null;
	let chromePatchRetryTimers: ReturnType<typeof setTimeout>[] = [];
	let searchFocusTimers: ReturnType<typeof setTimeout>[] = [];
	let searchFocusObserver: MutationObserver | null = null;
	let searchInputWasVisible = false;
	let lastToolbarRoot: HTMLElement | null = null;
	let scrollListenerTargets = new Set<HTMLElement>();
	let scrollPositions = new WeakMap<HTMLElement, number>();
	let scrollActivityFrame: number | null = null;
	let pendingScrollActivity: { delta: number; scrollTop: number } | null = null;
	let stableChromePatchCount = 0;
	let embedPdfSidebarOpen = false;
	let embedPdfZoomScope: ZoomScope | null = null;
	let embedPdfUiScope: any = null;
	let embedPdfThumbnailScope: any = null;
	let embedPdfCurrentPage = $state(1);

	const documentId = 'cryptorum-pdf';
	const maxRestoreAttempts = 8;
	const stableChromePatchLimit = 3;
	const appTheme = {
		preference: 'dark' as const,
		light: {
			background: {
				app: 'var(--color-surface-base, #0f172a)',
				surface: 'var(--color-surface-base, #0f172a)',
				surfaceAlt: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
				elevated: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
				overlay: 'rgba(0, 0, 0, 0.6)',
				input: 'var(--color-surface-base, #0f172a)'
			},
			foreground: {
				primary: 'var(--color-surface-text, #e2e8f0)',
				secondary: 'var(--color-surface-text, #e2e8f0)',
				muted: 'var(--color-surface-text-muted, #94a3b8)',
				disabled: 'color-mix(in srgb, var(--color-surface-text-muted, #94a3b8) 55%, transparent)',
				onAccent: '#ffffff'
			},
			border: {
				default: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
				subtle: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
				strong: 'var(--color-primary-500, #f97316)'
			},
			accent: {
				primary: 'var(--color-primary-500, #f97316)',
				primaryHover: 'var(--color-primary-600, #ea580c)',
				primaryActive: 'var(--color-primary-600, #ea580c)',
				primaryLight: 'color-mix(in srgb, var(--color-primary-500, #f97316) 20%, transparent)',
				primaryForeground: '#ffffff'
			},
			interactive: {
				hover: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
				active: 'color-mix(in srgb, var(--color-primary-500, #f97316) 24%, transparent)',
				selected: 'color-mix(in srgb, var(--color-primary-500, #f97316) 18%, transparent)',
				focus: 'var(--color-primary-500, #f97316)',
				focusRing: 'color-mix(in srgb, var(--color-primary-500, #f97316) 35%, transparent)'
			},
			scrollbar: {
				track: 'transparent',
				thumb: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
				thumbHover: 'var(--color-primary-500, #f97316)'
			},
			tooltip: {
				background: 'var(--color-surface-base, #0f172a)',
				foreground: 'var(--color-surface-text, #e2e8f0)'
			}
		},
		dark: {
			background: {
				app: 'var(--color-surface-base, #0f172a)',
				surface: 'var(--color-surface-base, #0f172a)',
				surfaceAlt: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
				elevated: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
				overlay: 'rgba(0, 0, 0, 0.6)',
				input: 'var(--color-surface-base, #0f172a)'
			},
			foreground: {
				primary: 'var(--color-surface-text, #e2e8f0)',
				secondary: 'var(--color-surface-text, #e2e8f0)',
				muted: 'var(--color-surface-text-muted, #94a3b8)',
				disabled: 'color-mix(in srgb, var(--color-surface-text-muted, #94a3b8) 55%, transparent)',
				onAccent: '#ffffff'
			},
			border: {
				default: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
				subtle: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
				strong: 'var(--color-primary-500, #f97316)'
			},
			accent: {
				primary: 'var(--color-primary-500, #f97316)',
				primaryHover: 'var(--color-primary-600, #ea580c)',
				primaryActive: 'var(--color-primary-600, #ea580c)',
				primaryLight: 'color-mix(in srgb, var(--color-primary-500, #f97316) 20%, transparent)',
				primaryForeground: '#ffffff'
			},
			interactive: {
				hover: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
				active: 'color-mix(in srgb, var(--color-primary-500, #f97316) 24%, transparent)',
				selected: 'color-mix(in srgb, var(--color-primary-500, #f97316) 18%, transparent)',
				focus: 'var(--color-primary-500, #f97316)',
				focusRing: 'color-mix(in srgb, var(--color-primary-500, #f97316) 35%, transparent)'
			},
			scrollbar: {
				track: 'transparent',
				thumb: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
				thumbHover: 'var(--color-primary-500, #f97316)'
			},
			tooltip: {
				background: 'var(--color-surface-base, #0f172a)',
				foreground: 'var(--color-surface-text, #e2e8f0)'
			}
		}
	};

	function clearRestoreTimers() {
		restoreTimers.forEach((timer) => clearTimeout(timer));
		restoreTimers = [];
	}

	function clearChromePatchTimer() {
		if (chromePatchTimer) {
			clearTimeout(chromePatchTimer);
			chromePatchTimer = null;
		}
	}

	function clearChromePatchRetryTimers() {
		chromePatchRetryTimers.forEach((timer) => clearTimeout(timer));
		chromePatchRetryTimers = [];
	}

	function clearSearchFocusTimers() {
		searchFocusTimers.forEach((timer) => clearTimeout(timer));
		searchFocusTimers = [];
	}

	function detachEmbedPdfScrollListeners() {
		for (const target of scrollListenerTargets) {
			target.removeEventListener('scroll', handleEmbedPdfInternalScroll);
		}
		scrollListenerTargets.clear();
		scrollPositions = new WeakMap<HTMLElement, number>();
		if (scrollActivityFrame !== null) {
			cancelAnimationFrame(scrollActivityFrame);
			scrollActivityFrame = null;
		}
		pendingScrollActivity = null;
	}

	function queryAllDeep(selector: string, root: ParentNode): Element[] {
		const matches = Array.from(root.querySelectorAll(selector));
		const children = Array.from(root.querySelectorAll('*'));

		for (const child of children) {
			const shadowRoot = (child as HTMLElement).shadowRoot;
			if (shadowRoot) {
				matches.push(...queryAllDeep(selector, shadowRoot));
			}
		}

		return matches;
	}

	function queryFirstDeep(selector: string): HTMLElement | null {
		if (typeof document === 'undefined') return null;

		const root = embedPdfContainerEl ?? document;
		const element = queryAllDeep(selector, root)[0];
		return element instanceof HTMLElement ? element : null;
	}

	function isVisibleTextInput(element: Element): element is HTMLInputElement {
		if (!(element instanceof HTMLInputElement) || element.disabled || element.readOnly) {
			return false;
		}

		const rect = element.getBoundingClientRect();
		if (rect.width <= 0 || rect.height <= 0) return false;

		const style = getComputedStyle(element);
		return style.display !== 'none' && style.visibility !== 'hidden';
	}

	function getEmbedPdfSearchInput() {
		if (!embedPdfContainerEl) return null;

		const selectors = [
			'input[type="search"]',
			'input[placeholder="Search"]',
			'input[aria-label="Search"]',
			'input[aria-label*="search" i]',
			'[data-epdf-i="search-input"] input',
			'[data-epdf-cat~="panel-search"] input',
			'[data-epdf-i="search-panel"] input'
		].join(', ');

		return queryAllDeep(selectors, embedPdfContainerEl).find(isVisibleTextInput) ?? null;
	}

	function getDeepActiveElement(root: Document | ShadowRoot = document): Element | null {
		let activeElement = root.activeElement;
		while (activeElement instanceof HTMLElement && activeElement.shadowRoot?.activeElement) {
			activeElement = activeElement.shadowRoot.activeElement;
		}
		return activeElement;
	}

	function focusEmbedPdfSearchInput(selectText = true) {
		const input = getEmbedPdfSearchInput();
		if (!input) return false;

		input.focus({ preventScroll: true });
		if (selectText) {
			input.select();
		}

		return getDeepActiveElement() === input;
	}

	function scheduleSearchInputFocus(selectText = true) {
		if (typeof window === 'undefined') return;

		clearSearchFocusTimers();
		if (focusEmbedPdfSearchInput(selectText)) return;

		const delays = [0, 40, 100, 200, 350];
		searchFocusTimers = delays.map((delay) =>
			setTimeout(() => {
				if (focusEmbedPdfSearchInput(selectText)) {
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

	function eventPathIncludesSelector(event: Event, selector: string) {
		const path = event.composedPath?.() ?? [];
		return path.some((target) => {
			if (!(target instanceof Element)) return false;
			return target.matches(selector) || !!target.closest(selector);
		});
	}

	function isSearchToggleEvent(event: Event) {
		return eventPathIncludesSelector(
			event,
			[
				'[data-epdf-i="toggle-search"]',
				'button[data-epdf-cat~="panel-search"]',
				'[role="button"][data-epdf-cat~="panel-search"]',
				'button[aria-label*="search" i]',
				'button[title*="search" i]'
			].join(', ')
		);
	}

	function isEmbedPdfChromeOrSidebarEvent(event: Event) {
		return eventPathIncludesSelector(
			event,
			[
				'button',
				'a',
				'input',
				'textarea',
				'select',
				'label',
				'[contenteditable="true"]',
				'[role="button"]',
				'[data-epdf-i="left-group"]',
				'[data-epdf-i="center-group"]',
				'[data-epdf-i="right-group"]',
				'[data-epdf-i="sidebar-panel"]',
				'[data-epdf-i="search-panel"]',
				'[data-epdf-cat~="panel-sidebar"]',
				'[data-epdf-cat~="panel-search"]'
			].join(', ')
		);
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
		const isVisible = !!getEmbedPdfSearchInput();
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

		searchInputWasVisible = !!getEmbedPdfSearchInput();
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

	function setStyle(
		element: HTMLElement,
		property: string,
		value: string,
		priority: '' | 'important' = ''
	) {
		if (
			element.style.getPropertyValue(property) !== value ||
			element.style.getPropertyPriority(property) !== priority
		) {
			element.style.setProperty(property, value, priority);
		}
	}

	function removeStyle(element: HTMLElement, property: string) {
		if (element.style.getPropertyValue(property)) {
			element.style.removeProperty(property);
		}
	}

	function isScrollableElement(element: HTMLElement) {
		const style = getComputedStyle(element);
		const verticalOverflow = style.overflowY;
		const horizontalOverflow = style.overflowX;
		const canScrollVertically =
			element.scrollHeight > element.clientHeight + 8 &&
			['auto', 'scroll', 'overlay'].includes(verticalOverflow);
		const canScrollHorizontally =
			element.scrollWidth > element.clientWidth + 8 &&
			['auto', 'scroll', 'overlay'].includes(horizontalOverflow);

		return canScrollVertically || canScrollHorizontally;
	}

	function handleEmbedPdfInternalScroll(event: Event) {
		const target = event.currentTarget;
		if (!(target instanceof HTMLElement)) return;

		const scrollTop = target.scrollTop;
		const previousScrollTop = scrollPositions.get(target) ?? scrollTop;
		const delta = scrollTop - previousScrollTop;
		scrollPositions.set(target, scrollTop);

		if (Math.abs(delta) > 0.5) {
			pendingScrollActivity = { delta, scrollTop };
			if (scrollActivityFrame === null) {
				scrollActivityFrame = requestAnimationFrame(() => {
					scrollActivityFrame = null;
					if (!pendingScrollActivity) return;
					const activity = pendingScrollActivity;
					pendingScrollActivity = null;
					onScrollActivity?.(activity.delta, activity.scrollTop);
				});
			}
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

	function updateEmbedPdfScrollListeners() {
		if (!embedPdfContainerEl) return;

		const root = embedPdfContainerEl;
		const scrollableElements = queryAllDeep('*', root).filter((element): element is HTMLElement =>
			element instanceof HTMLElement && isScrollableElement(element)
		);

		for (const target of Array.from(scrollListenerTargets)) {
			if (!scrollableElements.includes(target)) {
				target.removeEventListener('scroll', handleEmbedPdfInternalScroll);
				scrollListenerTargets.delete(target);
			}
		}

		for (const target of scrollableElements) {
			if (scrollListenerTargets.has(target)) continue;
			scrollListenerTargets.add(target);
			scrollPositions.set(target, target.scrollTop);
			target.addEventListener('scroll', handleEmbedPdfInternalScroll, { passive: true });
		}
	}

	function findToolbarRoot(groups: HTMLElement[]) {
		const [firstGroup] = groups;
		if (!firstGroup) return null;

		let candidate = firstGroup.parentElement;
		let toolbarRoot: HTMLElement | null = null;

		while (candidate && candidate !== embedPdfContainerEl) {
			if (groups.every((group) => candidate?.contains(group))) {
				const rect = candidate.getBoundingClientRect();
				const containerRect = embedPdfContainerEl?.getBoundingClientRect();
				const isNearTop = !containerRect || rect.top <= containerRect.top + 96;

				if (isNearTop && rect.height > 0 && rect.height <= 96) {
					toolbarRoot = candidate;
				}
			}

			candidate = candidate.parentElement;
		}

		return toolbarRoot;
	}

	function applyEmbedPdfChromePatch() {
		if (typeof document === 'undefined') return;

		const root = embedPdfContainerEl ?? document;
		const leftGroup = queryFirstDeep('[data-epdf-i="left-group"]');
		const centerGroup = queryFirstDeep('[data-epdf-i="center-group"]');
		const rightGroup = queryFirstDeep('[data-epdf-i="right-group"]');
		const toolbarGroups = [leftGroup, centerGroup, rightGroup].filter(Boolean) as HTMLElement[];
		const toolbarRoot = findToolbarRoot(toolbarGroups);
		if (toolbarRoot) {
			lastToolbarRoot = toolbarRoot;
		}
		const activeToolbarRoot = toolbarRoot ?? lastToolbarRoot;
		const rightClearance =
			'calc(var(--embedpdf-right-clearance, 48px) + max(0px, env(safe-area-inset-right)))';
		const leftClearance =
			'calc(var(--embedpdf-left-clearance, 56px) + max(0px, env(safe-area-inset-left)))';

		for (const element of queryAllDeep(
			[
				'[data-epdf-i="comment-button"]',
				'[data-epdf-cat~="panel-comment"]',
				'[data-epdf-i="document-menu-button"]',
				'[data-epdf-cat~="document-menu"]',
				'button[aria-label="Document Menu"]',
				'[data-epdf-i="divider-1"]'
			].join(', '),
			root
		)) {
			if (element instanceof HTMLElement) {
				setStyle(element, 'display', 'none', 'important');
				setStyle(element, 'visibility', 'hidden', 'important');
				setStyle(element, 'pointer-events', 'none', 'important');
				element.setAttribute('aria-hidden', 'true');
			}
		}

		if (leftGroup) {
			setStyle(leftGroup, 'margin-left', leftClearance, 'important');
			setStyle(leftGroup, 'margin-right', 'auto', 'important');
			setStyle(leftGroup, 'flex-shrink', '0', 'important');
		}

		if (centerGroup) {
			setStyle(centerGroup, 'margin-left', '0', 'important');
			setStyle(centerGroup, 'margin-right', '0', 'important');
			setStyle(centerGroup, 'transform', 'none', 'important');
			setStyle(centerGroup, 'flex-shrink', '0', 'important');
		}

		if (rightGroup) {
			setStyle(rightGroup, 'position', 'relative');
			setStyle(rightGroup, 'z-index', '25');
			setStyle(rightGroup, 'margin-right', rightClearance, 'important');
		}

		for (const group of toolbarGroups) {
			setStyle(group, 'transition', 'opacity 0.18s ease, transform 0.18s ease');
			setStyle(group, 'opacity', toolbarVisible ? '1' : '0', 'important');
			setStyle(group, 'visibility', toolbarVisible ? 'visible' : 'hidden', 'important');
			setStyle(group, 'pointer-events', toolbarVisible ? 'auto' : 'none', 'important');
		}

		if (activeToolbarRoot) {
			setStyle(activeToolbarRoot, 'position', 'absolute', 'important');
			setStyle(activeToolbarRoot, 'top', '0', 'important');
			setStyle(activeToolbarRoot, 'left', '0', 'important');
			setStyle(activeToolbarRoot, 'right', '0', 'important');
			setStyle(activeToolbarRoot, 'display', 'flex', 'important');
			setStyle(activeToolbarRoot, 'align-items', 'center', 'important');
			setStyle(activeToolbarRoot, 'justify-content', 'flex-start', 'important');
			setStyle(activeToolbarRoot, 'gap', '4px', 'important');
			setStyle(activeToolbarRoot, 'padding-left', '0', 'important');
			setStyle(activeToolbarRoot, 'padding-right', '0', 'important');
			setStyle(activeToolbarRoot, 'height', 'var(--reader-top-bar-height, 56px)', 'important');
			setStyle(activeToolbarRoot, 'min-height', 'var(--reader-top-bar-height, 56px)', 'important');
			setStyle(activeToolbarRoot, 'z-index', '80', 'important');
			setStyle(activeToolbarRoot, 'transition', 'opacity 0.18s ease, transform 0.18s ease, visibility 0.18s ease');
			setStyle(activeToolbarRoot, 'opacity', toolbarVisible ? '1' : '0', 'important');
			setStyle(activeToolbarRoot, 'visibility', toolbarVisible ? 'visible' : 'hidden', 'important');
			setStyle(activeToolbarRoot, 'pointer-events', toolbarVisible ? 'auto' : 'none', 'important');
			setStyle(activeToolbarRoot, 'transform', toolbarVisible ? 'translateY(0)' : 'translateY(-100%)', 'important');
			activeToolbarRoot.dataset.cryptorumEmbedpdfToolbar = 'true';
		}

		const documentContent = queryFirstDeep('#document-content');
		if (documentContent) {
			if (embedPdfSidebarOpen) {
				setStyle(documentContent, 'margin-top', 'var(--reader-top-bar-height, 56px)', 'important');
				setStyle(documentContent, 'height', 'calc(100% - var(--reader-top-bar-height, 56px))', 'important');
				setStyle(documentContent, 'flex', '0 0 calc(100% - var(--reader-top-bar-height, 56px))', 'important');
			} else {
				removeStyle(documentContent, 'margin-top');
				removeStyle(documentContent, 'height');
				removeStyle(documentContent, 'flex');
			}
		}

		for (const element of queryAllDeep(
			[
				'[data-overlay-id="page-controls"]',
				'[data-epdf-i="page-controls"]',
				'[data-epdf-cat~="page-controls"]'
			].join(', '),
			root
		)) {
			if (element instanceof HTMLElement) {
				setStyle(element, 'transition', 'opacity 0.18s ease, transform 0.18s ease, visibility 0.18s ease');
				setStyle(element, 'opacity', '0', 'important');
				setStyle(element, 'visibility', 'hidden', 'important');
				setStyle(element, 'pointer-events', 'none', 'important');
				setStyle(element, 'transform', 'translateY(8px)', 'important');
			}
		}

		for (const element of queryAllDeep('[data-overlay-id="page-controls"] *', root)) {
			if (element instanceof HTMLElement) {
				setStyle(element, 'pointer-events', 'none', 'important');
			}
		}

		if (embedPdfContainerEl) {
			const containerRect = embedPdfContainerEl.getBoundingClientRect();
			const protectedLeftGroups = [leftGroup, centerGroup].filter(Boolean) as HTMLElement[];
			const protectedLeftEdge = Math.max(
				0,
				...protectedLeftGroups.map((group) => group.getBoundingClientRect().right - containerRect.left)
			);
			const protectedRightEdge = rightGroup
				? Math.max(0, containerRect.right - rightGroup.getBoundingClientRect().left)
				: 0;

			if (protectedLeftEdge > 0) {
				embedPdfContainerEl.style.setProperty(
					'--embedpdf-title-left-clearance',
					`${Math.ceil(protectedLeftEdge + 16)}px`
				);
			}

			if (protectedRightEdge > 0) {
				embedPdfContainerEl.style.setProperty(
					'--embedpdf-title-right-clearance',
					`${Math.ceil(protectedRightEdge + 16)}px`
				);
			}
		}

		updateEmbedPdfScrollListeners();
		if (activeToolbarRoot && scrollListenerTargets.size > 0) {
			stableChromePatchCount += 1;
			if (stableChromePatchCount >= stableChromePatchLimit) {
				chromePatchObserver?.disconnect();
				chromePatchObserver = null;
			}
		} else {
			stableChromePatchCount = 0;
		}
	}

	function scheduleEmbedPdfChromePatch(delay = 0) {
		if (typeof window === 'undefined') return;

		clearChromePatchTimer();
		chromePatchTimer = setTimeout(() => {
			chromePatchTimer = null;
			applyEmbedPdfChromePatch();
		}, delay);
	}

	function queueEmbedPdfChromePatchBurst() {
		if (typeof window === 'undefined') return;

		clearChromePatchRetryTimers();
		scheduleEmbedPdfChromePatch();
		chromePatchRetryTimers = [50, 150, 350].map((delay) =>
			setTimeout(applyEmbedPdfChromePatch, delay)
		);
	}

	function startEmbedPdfChromeObserver() {
		if (
			typeof MutationObserver === 'undefined' ||
			!embedPdfContainerEl ||
			chromePatchObserver
		) {
			return;
		}

		stableChromePatchCount = 0;
		chromePatchObserver = new MutationObserver(() => scheduleEmbedPdfChromePatch());
		chromePatchObserver.observe(embedPdfContainerEl, { childList: true, subtree: true });
		queueEmbedPdfChromePatchBurst();
	}

	function getCapability<T>(registry: PluginRegistry, pluginId: string): T | null {
		const plugin = registry.getPlugin<IPlugin>(pluginId);
		return (plugin?.provides?.() as T | undefined) ?? null;
	}

	function hasOpenEmbedPdfSidebar(uiScope: any) {
		const activeSidebars = uiScope?.getState?.()?.activeSidebars ?? {};
		return Object.values(activeSidebars).some((sidebar: any) => !!sidebar?.isOpen);
	}

	function setEmbedPdfSidebarOpen(open: boolean) {
		if (embedPdfSidebarOpen === open) return;
		embedPdfSidebarOpen = open;
		onSidebarOpenChange?.(open);
		scheduleEmbedPdfChromePatch();
	}

	function restoreInitialPage(scroll: ScrollCapability, totalPages?: number, delay = 0) {
		const targetPage = Math.max(1, Math.floor(initialPage || 1));
		if (restoredInitialPage || targetPage <= 1) return;

		const clampedPage = totalPages && totalPages > 0
			? Math.min(targetPage, totalPages)
			: targetPage;

		const timer = setTimeout(() => {
			if (restoredInitialPage) return;
			if (!layoutReadySeen) {
				restoreInitialPage(scroll, totalPages, 150);
				return;
			}

			restoreAttempts += 1;

			let scopedScroll: ReturnType<ScrollCapability['forDocument']>;
			try {
				scopedScroll = scroll.forDocument(documentId);
				scopedScroll.scrollToPage({
					pageNumber: clampedPage,
					behavior: 'instant',
					alignY: 0
				});
			} catch {
				if (restoreAttempts < maxRestoreAttempts) {
					restoreInitialPage(scroll, totalPages, 150);
				}
				return;
			}

			requestAnimationFrame(() => {
				const currentPage = scopedScroll.getCurrentPage();
				if (currentPage === clampedPage || restoreAttempts >= maxRestoreAttempts) {
					restoredInitialPage = true;
					clearRestoreTimers();
					if (onPageChange && totalPages && totalPages > 0) {
						onPageChange(clampedPage, totalPages);
					}
					return;
				}

				restoreInitialPage(scroll, totalPages, 120);
			});
		}, delay);

		restoreTimers.push(timer);
	}

	function handleInit(container: EmbedPdfContainer) {
		console.log('EmbedPDF container initialized:', container);
		queueEmbedPdfChromePatchBurst();
	}

	function handleReady(registry: PluginRegistry) {
		console.log('EmbedPDF registry ready:', registry);
		startEmbedPdfChromeObserver();
		queueEmbedPdfChromePatchBurst();

		const scroll = getCapability<ScrollCapability>(registry, 'scroll');
		const zoom = getCapability<ZoomCapability>(registry, 'zoom');
		const thumbnail = getCapability<any>(registry, 'thumbnail');
		const documentManager = getCapability<DocumentManagerCapability>(registry, 'document-manager');
		const ui = getCapability<any>(registry, 'ui');
		embedPdfZoomScope = zoom?.forDocument?.(documentId) ?? null;
		embedPdfThumbnailScope = thumbnail?.forDocument?.(documentId) ?? null;

		if (scroll) {
			unsubscribePageChange?.();
			unsubscribePageChange = scroll.onPageChange((event: PageChangeEvent) => {
				embedPdfCurrentPage = event.pageNumber;
				if (onPageChange) {
					onPageChange(event.pageNumber, event.totalPages);
				}
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
			setEmbedPdfSidebarOpen(hasOpenEmbedPdfSidebar(uiScope));
			unsubscribeSidebarChange = uiScope.onSidebarChanged?.((event: any) => {
				setEmbedPdfSidebarOpen(hasOpenEmbedPdfSidebar(uiScope));
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
		unsubscribePageChange?.();
		unsubscribeLayoutReady?.();
		unsubscribeSidebarChange?.();
		embedPdfZoomScope = null;
		embedPdfUiScope = null;
		embedPdfThumbnailScope = null;
		if (embedPdfSidebarOpen) {
			onSidebarOpenChange?.(false);
		}
		clearRestoreTimers();
		clearChromePatchTimer();
		clearChromePatchRetryTimers();
		clearSearchFocusTimers();
		detachEmbedPdfScrollListeners();
		chromePatchObserver?.disconnect();
		searchFocusObserver?.disconnect();
	});

	$effect(() => {
		if (embedPdfContainerEl) {
			startEmbedPdfChromeObserver();
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
		scheduleEmbedPdfChromePatch();
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
				theme: appTheme,
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
				render: {
					withAnnotations: false,
					withForms: false,
					defaultImageQuality: 0.96
				},
				tiling: {
					tileSize: 512,
					overlapPx: 2.5,
					extraRings: 1
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

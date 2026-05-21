import { embedPdfSelectors } from './constants';
import { queryAllDeep, queryFirstDeep, removeStyle, setStyle } from './dom';

interface ChromePatchOptions {
	getRoot: () => HTMLElement | null;
	getToolbarVisible: () => boolean;
	getSidebarOpen: () => boolean;
	getScrollTargetCount: () => number;
	updateScrollListeners: () => void;
}

export function createEmbedPdfChromePatchController(options: ChromePatchOptions) {
	let chromePatchObserver: MutationObserver | null = null;
	let chromePatchTimer: ReturnType<typeof setTimeout> | null = null;
	let chromePatchRetryTimers: ReturnType<typeof setTimeout>[] = [];
	let lastToolbarRoot: HTMLElement | null = null;
	let stableChromePatchCount = 0;
	const stableChromePatchLimit = 3;

	function clearTimer() {
		if (chromePatchTimer) {
			clearTimeout(chromePatchTimer);
			chromePatchTimer = null;
		}
	}

	function clearRetries() {
		chromePatchRetryTimers.forEach((timer) => clearTimeout(timer));
		chromePatchRetryTimers = [];
	}

	function findToolbarRoot(groups: HTMLElement[], container: HTMLElement) {
		const [firstGroup] = groups;
		if (!firstGroup) return null;

		let candidate = firstGroup.parentElement;
		let toolbarRoot: HTMLElement | null = null;

		while (candidate && candidate !== container) {
			if (groups.every((group) => candidate?.contains(group))) {
				const rect = candidate.getBoundingClientRect();
				const containerRect = container.getBoundingClientRect();
				const isNearTop = rect.top <= containerRect.top + 96;

				if (isNearTop && rect.height > 0 && rect.height <= 96) {
					toolbarRoot = candidate;
				}
			}

			candidate = candidate.parentElement;
		}

		return toolbarRoot;
	}

	function apply() {
		if (typeof document === 'undefined') return;

		const root = options.getRoot();
		if (!root) return;

		const toolbarVisible = options.getToolbarVisible();
		const sidebarOpen = options.getSidebarOpen();
		const leftGroup = queryFirstDeep(embedPdfSelectors.leftGroup, root);
		const centerGroup = queryFirstDeep(embedPdfSelectors.centerGroup, root);
		const rightGroup = queryFirstDeep(embedPdfSelectors.rightGroup, root);
		const toolbarGroups = [leftGroup, centerGroup, rightGroup].filter(Boolean) as HTMLElement[];
		const toolbarRoot = findToolbarRoot(toolbarGroups, root);
		if (toolbarRoot) {
			lastToolbarRoot = toolbarRoot;
		}
		const activeToolbarRoot = toolbarRoot ?? lastToolbarRoot;
		const rightClearance =
			'calc(var(--embedpdf-right-clearance, 48px) + max(0px, env(safe-area-inset-right)))';
		const leftClearance =
			'calc(var(--embedpdf-left-clearance, 56px) + max(0px, env(safe-area-inset-left)))';

		for (const element of queryAllDeep(embedPdfSelectors.hiddenChrome, root)) {
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

		const documentContent = queryFirstDeep('#document-content', root);
		if (documentContent) {
			if (sidebarOpen) {
				setStyle(documentContent, 'margin-top', 'var(--reader-top-bar-height, 56px)', 'important');
				setStyle(documentContent, 'height', 'calc(100% - var(--reader-top-bar-height, 56px))', 'important');
				setStyle(documentContent, 'flex', '0 0 calc(100% - var(--reader-top-bar-height, 56px))', 'important');
			} else {
				removeStyle(documentContent, 'margin-top');
				removeStyle(documentContent, 'height');
				removeStyle(documentContent, 'flex');
			}
		}

		for (const element of queryAllDeep(embedPdfSelectors.pageControls, root)) {
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

		const containerRect = root.getBoundingClientRect();
		const protectedLeftGroups = [leftGroup, centerGroup].filter(Boolean) as HTMLElement[];
		const protectedLeftEdge = Math.max(
			0,
			...protectedLeftGroups.map((group) => group.getBoundingClientRect().right - containerRect.left)
		);
		const protectedRightEdge = rightGroup
			? Math.max(0, containerRect.right - rightGroup.getBoundingClientRect().left)
			: 0;

		if (protectedLeftEdge > 0) {
			root.style.setProperty('--embedpdf-title-left-clearance', `${Math.ceil(protectedLeftEdge + 16)}px`);
		}

		if (protectedRightEdge > 0) {
			root.style.setProperty('--embedpdf-title-right-clearance', `${Math.ceil(protectedRightEdge + 16)}px`);
		}

		options.updateScrollListeners();
		if (activeToolbarRoot && options.getScrollTargetCount() > 0) {
			stableChromePatchCount += 1;
			if (stableChromePatchCount >= stableChromePatchLimit) {
				chromePatchObserver?.disconnect();
				chromePatchObserver = null;
			}
		} else {
			stableChromePatchCount = 0;
		}
	}

	function schedule(delay = 0) {
		if (typeof window === 'undefined') return;

		clearTimer();
		chromePatchTimer = setTimeout(() => {
			chromePatchTimer = null;
			apply();
		}, delay);
	}

	function queueBurst() {
		if (typeof window === 'undefined') return;

		clearRetries();
		schedule();
		chromePatchRetryTimers = [50, 150, 350].map((delay) =>
			setTimeout(apply, delay)
		);
	}

	function startObserver() {
		const root = options.getRoot();
		if (
			typeof MutationObserver === 'undefined' ||
			!root ||
			chromePatchObserver
		) {
			return;
		}

		stableChromePatchCount = 0;
		chromePatchObserver = new MutationObserver(() => schedule());
		chromePatchObserver.observe(root, { childList: true, subtree: true });
		queueBurst();
	}

	function disconnect() {
		clearTimer();
		clearRetries();
		chromePatchObserver?.disconnect();
		chromePatchObserver = null;
		lastToolbarRoot = null;
		stableChromePatchCount = 0;
	}

	return {
		apply,
		schedule,
		queueBurst,
		startObserver,
		disconnect
	};
}

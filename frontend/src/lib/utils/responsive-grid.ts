export const DEFAULT_GRID_SCALE = 100;
export const MIN_GRID_SCALE = 75;
export const MAX_GRID_SCALE = 150;
export const GRID_SCALE_STEP = 5;

const BASE_GRID_SLOT_WIDTH = 160;
const NARROW_CONTAINER_WIDTH = 520;

type ResponsiveGridOptions = {
	minColumns?: number;
	narrowMinColumns?: number;
	maxColumns?: number;
};

export function clampGridScale(value: number): number {
	if (!Number.isFinite(value)) return DEFAULT_GRID_SCALE;
	return Math.min(MAX_GRID_SCALE, Math.max(MIN_GRID_SCALE, Math.round(value)));
}

export function getResponsiveGridColumns(width: number, scale: number, options: ResponsiveGridOptions = {}): number {
	const containerWidth = Math.max(0, Math.round(width));
	const minColumns = containerWidth < NARROW_CONTAINER_WIDTH
		? options.narrowMinColumns ?? 2
		: options.minColumns ?? 3;
	const maxColumns = options.maxColumns ?? 20;

	if (containerWidth <= 0) return minColumns;

	const slotWidth = BASE_GRID_SLOT_WIDTH * (clampGridScale(scale) / 100);
	const columns = Math.round(containerWidth / slotWidth);
	return Math.min(maxColumns, Math.max(minColumns, columns));
}

export function observeElementWidth(node: HTMLElement, onWidthChange: (width: number) => void): () => void {
	const notify = () => onWidthChange(node.getBoundingClientRect().width);
	notify();

	if (typeof ResizeObserver === 'undefined') {
		window.addEventListener('resize', notify);
		return () => window.removeEventListener('resize', notify);
	}

	const observer = new ResizeObserver((entries) => {
		const width = entries[0]?.contentRect.width ?? node.getBoundingClientRect().width;
		onWidthChange(width);
	});
	observer.observe(node);

	return () => observer.disconnect();
}

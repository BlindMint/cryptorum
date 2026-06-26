export const DEFAULT_GRID_SCALE = 100;
export const MIN_GRID_SCALE = 75;
export const MAX_GRID_SCALE = 150;
export const GRID_SCALE_STEP = 5;

const BASE_GRID_SLOT_WIDTH = 160;
const BASE_GRID_COVER_WIDTH = 144;
const NARROW_CONTAINER_WIDTH = 520;
const ROW_GAP = 16;

type ResponsiveGridOptions = {
	minColumns?: number;
	narrowMinColumns?: number;
	maxColumns?: number;
};

export type ResponsiveGridLayout = {
	columns: number;
	coverWidth: number;
	rowGap: number;
};

export function clampGridScale(value: number): number {
	if (!Number.isFinite(value)) return DEFAULT_GRID_SCALE;
	return Math.min(MAX_GRID_SCALE, Math.max(MIN_GRID_SCALE, Math.round(value)));
}

export function getResponsiveGridColumns(width: number, scale: number, options: ResponsiveGridOptions = {}): number {
	return getResponsiveGridLayout(width, scale, options).columns;
}

export function getResponsiveGridLayout(width: number, scale: number, options: ResponsiveGridOptions = {}): ResponsiveGridLayout {
	const containerWidth = Math.max(0, Math.round(width));
	const minColumns = containerWidth < NARROW_CONTAINER_WIDTH
		? options.narrowMinColumns ?? 2
		: options.minColumns ?? 3;
	const maxColumns = options.maxColumns ?? 20;
	const scaleRatio = clampGridScale(scale) / 100;
	const slotWidth = BASE_GRID_SLOT_WIDTH * scaleRatio;
	const targetCoverWidth = Math.round(BASE_GRID_COVER_WIDTH * scaleRatio);

	if (containerWidth <= 0) {
		return {
			columns: minColumns,
			coverWidth: targetCoverWidth,
			rowGap: ROW_GAP
		};
	}

	const columns = Math.min(maxColumns, Math.max(minColumns, Math.round(containerWidth / slotWidth)));

	return {
		columns,
		coverWidth: targetCoverWidth,
		rowGap: ROW_GAP
	};
}

export function getResponsiveGridStyle(layout: ResponsiveGridLayout): string {
	return [
		`--grid-cover-width: ${layout.coverWidth}px`,
		`row-gap: ${layout.rowGap}px`,
		'grid-template-columns: repeat(auto-fill, var(--grid-cover-width))',
		'justify-content: space-between',
		'column-gap: 1rem'
	].join('; ');
}

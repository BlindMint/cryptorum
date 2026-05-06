export type CoverThumbSize = 'small' | 'medium' | 'large';

export function getLibraryCoverThumbSize(gridSize: number): CoverThumbSize {
	if (gridSize >= 7) return 'large';
	if (gridSize >= 5) return 'medium';
	return 'small';
}

export function getCoverThumbUrl(bookId: number | string, size: CoverThumbSize, updatedOn?: number): string {
	const params = new URLSearchParams({ size });
	if (updatedOn && updatedOn > 0) {
		params.set('v', String(updatedOn));
	}
	return `/api/covers/${bookId}/thumb?${params.toString()}`;
}

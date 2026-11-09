/**
 * Helpers for resuming a PDF at a saved page. EmbedPDF can report a tiny
 * totalPages value while a large document is still loading (especially over
 * range requests on slower devices). Clamping the saved page to that incomplete
 * total makes the reader treat page 1 as a successful restore and then save 0%.
 */

export function pageToRestore(targetPage: number, totalPages?: number): number {
	const target = Math.max(1, Math.floor(targetPage || 1));
	if (!totalPages || totalPages <= 0 || totalPages < target) return target;
	return Math.min(target, totalPages);
}

interface RestorePageVisibility {
	pageNumber: number;
	viewportY: number;
	scaled: { pageY: number; visibleHeight: number };
}

export function didRestoreLand(
	measuredPage: number,
	targetPage: number,
	visiblePages: readonly RestorePageVisibility[] = []
): boolean {
	const target = Math.max(1, Math.floor(targetPage || 1));
	if (target <= 1) return true;
	if (measuredPage === target) return true;
	// Fit-width zoom uses the widest page in the document. With mixed page
	// widths, a later page can be more visible than the page we aligned at the
	// top. Accept that alignment rather than waiting forever for currentPage.
	return visiblePages.some((page) =>
		page.pageNumber === target &&
		page.scaled.visibleHeight > 0 &&
		page.viewportY >= 0 && page.viewportY <= 16 &&
		page.scaled.pageY >= 0 && page.scaled.pageY <= 16
	);
}

export function shouldAdoptDocumentPageCount(reportedTotal: number | undefined, knownTotal: number): boolean {
	if (!reportedTotal || reportedTotal <= 0) return false;
	if (knownTotal <= 0) return true;
	return reportedTotal >= knownTotal;
}

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

export function didRestoreLand(measuredPage: number, targetPage: number): boolean {
	const target = Math.max(1, Math.floor(targetPage || 1));
	if (target <= 1) return true;
	return measuredPage === target;
}

export function shouldAdoptDocumentPageCount(reportedTotal: number | undefined, knownTotal: number): boolean {
	if (!reportedTotal || reportedTotal <= 0) return false;
	if (knownTotal <= 0) return true;
	return reportedTotal >= knownTotal;
}

// Require a loaded final image: lazy placeholders can temporarily make an
// unfinished strip look as though it has reached its scroll boundary.
export function isComicScrollEnd(position: number, extent: number, viewport: number, finalPageLoaded: boolean, allPagesLoaded: boolean): boolean {
	if (!finalPageLoaded || viewport <= 0) return false;
	const maximum = Math.max(0, extent - viewport);
	if (maximum <= 2) return allPagesLoaded;
	return Math.abs(position) >= maximum - 2;
}


export function comicProgressPageAtX(clientX: number, left: number, width: number, total: number, rtl: boolean): number | null {
	if (width <= 0 || total <= 0) return null;
	const fraction = Math.max(0, Math.min(1, (clientX - left) / width));
	return 1 + Math.round((rtl ? 1 - fraction : fraction) * (total - 1));
}

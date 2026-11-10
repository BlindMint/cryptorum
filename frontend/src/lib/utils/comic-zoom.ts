export function clampComicZoom(value: number): number {
	return Math.max(0.25, Math.min(5, value));
}

export function comicZoomScrollDelta(
	before: { left: number; top: number },
	after: { left: number; top: number },
	oldZoom: number,
	newZoom: number,
	anchor: { x: number; y: number }
): { left: number; top: number } {
	return {
		left: after.left + (anchor.x - before.left) * newZoom / oldZoom - anchor.x,
		top: after.top + (anchor.y - before.top) * newZoom / oldZoom - anchor.y
	};
}

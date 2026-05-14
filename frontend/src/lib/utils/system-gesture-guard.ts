import { browser } from '$app/environment';

const BOTTOM_EDGE_THRESHOLD_PX = 28;

export function isBottomSystemGestureStart(e: PointerEvent | TouchEvent | MouseEvent): boolean {
	if (!browser || !('innerHeight' in window)) return false;

	const pointerType = 'pointerType' in e ? e.pointerType : '';
	if (pointerType && pointerType !== 'touch') return false;

	const clientY = 'clientY' in e ? e.clientY : e.touches?.[0]?.clientY;
	if (typeof clientY !== 'number') return false;

	return window.innerHeight - clientY <= BOTTOM_EDGE_THRESHOLD_PX;
}

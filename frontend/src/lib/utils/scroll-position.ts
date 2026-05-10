const SCROLL_PREFIX = 'cryptorum:scroll:';

function routeKey(): string {
	if (typeof window === 'undefined') return '';
	return `${window.location.pathname}${window.location.search}`;
}

export function saveRouteScrollPosition() {
	if (typeof window === 'undefined') return;
	sessionStorage.setItem(`${SCROLL_PREFIX}${routeKey()}`, String(window.scrollY));
}

export function restoreRouteScrollPosition() {
	if (typeof window === 'undefined') return;
	const raw = sessionStorage.getItem(`${SCROLL_PREFIX}${routeKey()}`);
	if (!raw) return;
	const y = Number(raw);
	if (!Number.isFinite(y)) return;

	requestAnimationFrame(() => {
		requestAnimationFrame(() => {
			window.scrollTo(0, y);
		});
	});
}

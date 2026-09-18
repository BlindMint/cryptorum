import { writable } from 'svelte/store';

export const bulkActionBarHeight = writable(0);

const measuredBars = new Map<HTMLElement, number>();

function publishHeight() {
	bulkActionBarHeight.set(Math.max(0, ...measuredBars.values()));
}

export function trackBulkActionBar(node: HTMLElement) {
	const updateHeight = () => {
		measuredBars.set(node, Math.ceil(node.getBoundingClientRect().height));
		publishHeight();
	};
	const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(updateHeight);
	updateHeight();
	observer?.observe(node);

	return {
		destroy() {
			observer?.disconnect();
			measuredBars.delete(node);
			publishHeight();
		}
	};
}

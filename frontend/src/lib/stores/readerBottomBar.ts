import { writable } from 'svelte/store';

export const readerBottomBarHeight = writable(0);

const measuredBars = new Map<HTMLElement, number>();

function publishHeight() {
	readerBottomBarHeight.set(Math.max(0, ...measuredBars.values()));
}

export function trackReaderBottomBar(node: HTMLElement, visible = true) {
	let isVisible = visible;

	const updateHeight = () => {
		measuredBars.set(node, isVisible ? Math.ceil(node.getBoundingClientRect().height) : 0);
		publishHeight();
	};
	const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(updateHeight);
	updateHeight();
	observer?.observe(node);

	return {
		update(nextVisible: boolean) {
			isVisible = nextVisible;
			updateHeight();
		},
		destroy() {
			observer?.disconnect();
			measuredBars.delete(node);
			publishHeight();
		}
	};
}

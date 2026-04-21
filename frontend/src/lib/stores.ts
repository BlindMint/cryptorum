import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export const mobileMenuOpen = writable(false);

// Sidebar refresh store - increment to trigger refresh
export const sidebarRefresh = writable(0);

// Grid size for library and dashboard - default 4, will be overridden by screen size detection
function createGridSizeStore() {
	const defaultValue = 4;
	const { subscribe, set, update } = writable<number>(defaultValue);

	return {
		subscribe,
		set: (value: number) => {
			if (browser) {
				localStorage.setItem('gridSize', JSON.stringify(value));
			}
			set(value);
		},
		update: (fn: (value: number) => number) => {
			update(value => {
				const newValue = fn(value);
				if (browser) {
					localStorage.setItem('gridSize', JSON.stringify(newValue));
				}
				return newValue;
			});
		},
		init: () => {
			if (browser) {
				const stored = localStorage.getItem('gridSize');
				if (stored !== null) {
					set(JSON.parse(stored));
				}
			}
		}
	};
}

export const gridSize = createGridSizeStore();

// Show file format badge on book covers
function createShowFormatOnCoverStore() {
	const defaultValue = true;
	const { subscribe, set, update } = writable<boolean>(defaultValue);

	return {
		subscribe,
		set: (value: boolean) => {
			if (browser) {
				localStorage.setItem('showFormatOnCover', JSON.stringify(value));
			}
			set(value);
		},
		update: (fn: (value: boolean) => boolean) => {
			update(value => {
				const newValue = fn(value);
				if (browser) {
					localStorage.setItem('showFormatOnCover', JSON.stringify(newValue));
				}
				return newValue;
			});
		},
		init: () => {
			if (browser) {
				const stored = localStorage.getItem('showFormatOnCover');
				if (stored !== null) {
					set(JSON.parse(stored));
				}
			}
		}
	};
}

export const showFormatOnCover = createShowFormatOnCoverStore();

function createNotificationVisualIndicatorStore() {
	const defaultValue = true;
	const { subscribe, set, update } = writable<boolean>(defaultValue);

	return {
		subscribe,
		set: (value: boolean) => {
			if (browser) {
				localStorage.setItem('notificationVisualIndicator', JSON.stringify(value));
			}
			set(value);
		},
		update: (fn: (value: boolean) => boolean) => {
			update(value => {
				const newValue = fn(value);
				if (browser) {
					localStorage.setItem('notificationVisualIndicator', JSON.stringify(newValue));
				}
				return newValue;
			});
		},
		init: () => {
			if (browser) {
				const stored = localStorage.getItem('notificationVisualIndicator');
				if (stored !== null) {
					set(JSON.parse(stored));
				}
			}
		}
	};
}

export const notificationVisualIndicator = createNotificationVisualIndicatorStore();

// File format colors for badges
export const formatColors: Record<string, { bg: string; text: string }> = {
	epub: { bg: '#10b981', text: '#ffffff' },   // emerald-500
	pdf: { bg: '#ef4444', text: '#ffffff' },    // red-500
	cbz: { bg: '#8b5cf6', text: '#ffffff' },    // violet-500
	cbr: { bg: '#f59e0b', text: '#ffffff' },    // amber-500
	mobi: { bg: '#06b6d4', text: '#ffffff' },   // cyan-500
	azw: { bg: '#ec4899', text: '#ffffff' },    // pink-500
	azw3: { bg: '#ec4899', text: '#ffffff' },    // pink-500
	default: { bg: '#6b7280', text: '#ffffff' }  // gray-500
};

export function getFormatColor(format: string): { bg: string; text: string } {
	return formatColors[format.toLowerCase()] || formatColors.default;
}

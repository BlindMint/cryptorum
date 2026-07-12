import { writable } from 'svelte/store';
import { browser } from '$app/environment';
import { clampGridScale, DEFAULT_GRID_SCALE } from '$lib/utils/responsive-grid';

export const mobileMenuOpen = writable(false);
export const desktopSidebarCollapsed = writable(false);

// Sidebar refresh store - increment to trigger refresh
export const sidebarRefresh = writable(0);

// Legacy fixed grid column count. Kept so older localStorage state and any
// future compatibility reads do not break while responsive grid scale replaces it.
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

// Grid scale for responsive library/search grids. This replaces fixed column
// counts while keeping the old gridSize store available for compatibility.
function createGridScaleStore() {
	const { subscribe, set, update } = writable<number>(DEFAULT_GRID_SCALE);

	function persist(value: number) {
		const nextValue = clampGridScale(value);
		if (browser) {
			localStorage.setItem('gridScale', JSON.stringify(nextValue));
		}
		set(nextValue);
	}

	return {
		subscribe,
		set: persist,
		update: (fn: (value: number) => number) => {
			update(value => {
				const newValue = clampGridScale(fn(value));
				if (browser) {
					localStorage.setItem('gridScale', JSON.stringify(newValue));
				}
				return newValue;
			});
		},
		init: () => {
			if (browser) {
				const stored = localStorage.getItem('gridScale');
				if (stored !== null) {
					persist(JSON.parse(stored));
					return;
				}
			}
			set(DEFAULT_GRID_SCALE);
		}
	};
}

export const gridScale = createGridScaleStore();

export const FILTER_PANEL_MIN_WIDTH = 280;
export const FILTER_PANEL_MAX_WIDTH = 560;
export const DEFAULT_FILTER_PANEL_WIDTH = 320;

function clampFilterPanelWidth(value: number): number {
	return Math.min(FILTER_PANEL_MAX_WIDTH, Math.max(FILTER_PANEL_MIN_WIDTH, Math.round(value)));
}

function createFilterPanelWidthStore() {
	const { subscribe, set } = writable<number>(DEFAULT_FILTER_PANEL_WIDTH);

	function persist(value: number) {
		const nextValue = clampFilterPanelWidth(value);
		if (browser) {
			localStorage.setItem('filterPanelWidth', JSON.stringify(nextValue));
		}
		set(nextValue);
	}

	return {
		subscribe,
		set: persist,
		setTemporary: (value: number) => {
			set(clampFilterPanelWidth(value));
		},
		init: () => {
			if (browser) {
				const stored = localStorage.getItem('filterPanelWidth');
				if (stored !== null) {
					try {
						persist(JSON.parse(stored));
						return;
					} catch {
						// Ignore invalid localStorage state and restore the default width.
					}
				}
			}
			set(DEFAULT_FILTER_PANEL_WIDTH);
		}
	};
}

export const filterPanelWidth = createFilterPanelWidthStore();

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

// Show tappable reading-progress chip on book covers
function createShowProgressChipOnCoverStore() {
	const defaultValue = true;
	const { subscribe, set, update } = writable<boolean>(defaultValue);

	return {
		subscribe,
		set: (value: boolean) => {
			if (browser) {
				localStorage.setItem('showProgressChipOnCover', JSON.stringify(value));
			}
			set(value);
		},
		update: (fn: (value: boolean) => boolean) => {
			update((value) => {
				const newValue = fn(value);
				if (browser) {
					localStorage.setItem('showProgressChipOnCover', JSON.stringify(newValue));
				}
				return newValue;
			});
		},
		init: () => {
			if (browser) {
				const stored = localStorage.getItem('showProgressChipOnCover');
				if (stored !== null) {
					try {
						set(JSON.parse(stored));
					} catch {
						// Ignore invalid localStorage state and keep the default.
					}
				}
			}
		}
	};
}

export const showProgressChipOnCover = createShowProgressChipOnCoverStore();

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

export type NotificationEventPreferences = {
	appNotifications: boolean;
	completedJobs: boolean;
	authEvents: boolean;
	appEvents: boolean;
};

const defaultNotificationEventPreferences: NotificationEventPreferences = {
	appNotifications: true,
	completedJobs: true,
	authEvents: false,
	appEvents: false
};

function createNotificationEventPreferencesStore() {
	const { subscribe, set, update } = writable<NotificationEventPreferences>(defaultNotificationEventPreferences);

	function persist(value: NotificationEventPreferences) {
		if (browser) {
			localStorage.setItem('notificationEventPreferences', JSON.stringify(value));
		}
	}

	return {
		subscribe,
		set: (value: NotificationEventPreferences) => {
			persist(value);
			set(value);
		},
		setKey: (key: keyof NotificationEventPreferences, value: boolean) => {
			update((current) => {
				const next = { ...current, [key]: value };
				persist(next);
				return next;
			});
		},
		init: () => {
			if (!browser) return;
			const stored = localStorage.getItem('notificationEventPreferences');
			if (!stored) return;
			try {
				set({ ...defaultNotificationEventPreferences, ...JSON.parse(stored) });
			} catch {
				set(defaultNotificationEventPreferences);
			}
		}
	};
}

export const notificationEventPreferences = createNotificationEventPreferencesStore();

let latestNotificationEventPreferences = defaultNotificationEventPreferences;
notificationEventPreferences.subscribe((value) => {
	latestNotificationEventPreferences = value;
});

function createReviewedMetadataLookupJobsStore() {
	const { subscribe, set, update } = writable<Set<number>>(new Set());

	return {
		subscribe,
		init: () => {
			if (!browser) return;
			try {
				const stored = JSON.parse(sessionStorage.getItem('reviewedMetadataLookupJobs') || '[]');
				if (Array.isArray(stored)) {
					set(new Set(stored.filter((id) => typeof id === 'number')));
				}
			} catch {
				set(new Set());
			}
		},
		mark: (jobId: number) => {
			if (!jobId || jobId < 0) return;
			update((current) => {
				const next = new Set(current);
				next.add(jobId);
				if (browser) {
					sessionStorage.setItem('reviewedMetadataLookupJobs', JSON.stringify(Array.from(next)));
				}
				return next;
			});
		}
	};
}

export const reviewedMetadataLookupJobs = createReviewedMetadataLookupJobsStore();

export type ActivityJob = {
	id: number;
	job_type: string;
	title: string;
	status: string;
	total_items: number;
	completed_items: number;
	failed_items: number;
	payload?: Record<string, any>;
	result?: Record<string, any>;
	error?: string;
	created_at: number;
};

export type ActivityNotification = {
	id: number;
	source?: 'notification' | 'job' | 'log';
	kind: string;
	title: string;
	message?: string;
	url?: string;
	read_at?: number;
	created_at: number;
	job?: ActivityJob;
};

type ActivityState = {
	notifications: ActivityNotification[];
	activeJobs: ActivityJob[];
	loading: boolean;
	error: string | null;
	lastUpdated: number;
};

export type PendingActivityJobInput = {
	job_type: string;
	title: string;
	total_items?: number;
	payload?: Record<string, any>;
};

type PendingActivityJob = {
	token: string;
	job: ActivityJob;
	expectedJobIds: number[];
	expiresAt: number;
};

function createAppActivityStore() {
	const initialState: ActivityState = {
		notifications: [],
		activeJobs: [],
		loading: false,
		error: null,
		lastUpdated: 0
	};
	const { subscribe, set, update } = writable<ActivityState>(initialState);
	let timer: ReturnType<typeof setTimeout> | null = null;
	let initialized = false;
	let inFlight: Promise<void> | null = null;
	let latestState = initialState;
	let pendingCounter = 0;
	const pendingJobs = new Map<string, PendingActivityJob>();

	subscribe((value) => {
		latestState = value;
	});

	function nextInterval(): number {
		if (!browser || document.visibilityState !== 'visible') return 30000;
		return latestState.activeJobs.length > 0 ? 5000 : 15000;
	}

	function mergePendingJobs(activeJobs: ActivityJob[]): ActivityJob[] {
		const now = Date.now();
		for (const [token, pending] of pendingJobs) {
			const matchedExpectedJob = pending.expectedJobIds.length > 0 &&
				pending.expectedJobIds.some((id) => activeJobs.some((job) => job.id === id));
			if (matchedExpectedJob || pending.expiresAt <= now) {
				pendingJobs.delete(token);
			}
		}
		const pending = Array.from(pendingJobs.values())
			.sort((a, b) => b.job.created_at - a.job.created_at)
			.map((entry) => entry.job);
		return [...pending, ...activeJobs];
	}

	function refreshActiveJobsFromPending() {
		update((state) => ({
			...state,
			activeJobs: mergePendingJobs(state.activeJobs.filter((job) => job.id > 0)),
			lastUpdated: Date.now()
		}));
	}

	function shouldShowNotification(item: ActivityNotification): boolean {
		const prefs = latestNotificationEventPreferences;
		if (item.source === 'job') {
			return !!prefs.completedJobs &&
				item.job?.status === 'completed' &&
				(item.job.job_type === 'metadata_lookup' || item.job.job_type === 'bulk_metadata_update');
		}
		if (item.source === 'log') {
			const kind = String(item.kind || '').toLowerCase();
			if (kind.includes('auth')) return !!prefs.authEvents;
			return !!prefs.appEvents;
		}
		return !!prefs.appNotifications;
	}

	function pendingExpiry(status: string) {
		switch (status) {
			case 'failed':
				return Date.now() + 8000;
			case 'queued':
			case 'running':
				return Date.now() + 15000;
			default:
				return Date.now() + 30000;
		}
	}

	function schedule(delay = nextInterval()) {
		if (!browser) return;
		if (timer) window.clearTimeout(timer);
		timer = window.setTimeout(async () => {
			if (document.visibilityState === 'visible') {
				await refresh();
			} else {
				schedule();
			}
		}, delay);
	}

	async function refresh(): Promise<void> {
		if (!browser) return;
		if (inFlight) return inFlight;

		inFlight = (async () => {
			update((state) => ({ ...state, loading: true, error: null }));
			try {
				const [notificationsRes, jobsRes] = await Promise.all([
					fetch('/api/notifications?limit=20', { cache: 'no-store' }),
					fetch('/api/jobs?status=queued,running,cancelling&limit=100', { cache: 'no-store' })
				]);

				let notifications = latestState.notifications;
				let activeJobs = latestState.activeJobs;
				if (notificationsRes.ok) {
					const data = await notificationsRes.json();
					const items: ActivityNotification[] = data.items ?? [];
					notifications = items.filter(shouldShowNotification);
				}
				if (jobsRes.ok) {
					activeJobs = mergePendingJobs(await jobsRes.json());
				}

				set({
					notifications,
					activeJobs,
					loading: false,
					error: null,
					lastUpdated: Date.now()
				});
			} catch (error) {
				console.error('Failed to load app activity:', error);
				update((state) => ({
					...state,
					loading: false,
					error: 'Unable to load app activity.'
				}));
			} finally {
				inFlight = null;
				schedule();
			}
		})();

		return inFlight;
	}

	function handleVisibilityChange() {
		if (document.visibilityState === 'visible') {
			schedule(250);
		}
	}

	return {
		subscribe,
		init: () => {
			if (!browser || initialized) return;
			initialized = true;
			document.addEventListener('visibilitychange', handleVisibilityChange);
			void refresh();
		},
		refresh,
		startPendingJob: (input: PendingActivityJobInput) => {
			const token = `pending-${Date.now()}-${++pendingCounter}`;
			pendingJobs.set(token, {
				token,
				expectedJobIds: [],
				expiresAt: pendingExpiry('starting'),
				job: {
					id: -pendingCounter,
					job_type: input.job_type,
					title: input.title,
					status: 'starting',
					total_items: input.total_items ?? 0,
					completed_items: 0,
					failed_items: 0,
					payload: { ...(input.payload ?? {}), optimistic: true },
					created_at: Math.floor(Date.now() / 1000)
				}
			});
			refreshActiveJobsFromPending();
			schedule(750);
			return token;
		},
		confirmPendingJob: (token: string, jobOrIds?: ActivityJob | number | number[]) => {
			const pending = pendingJobs.get(token);
			if (!pending) return;
			if (typeof jobOrIds === 'number') {
				pending.expectedJobIds = [jobOrIds];
				pending.job = { ...pending.job, id: -Math.abs(pending.job.id), status: 'queued' };
			} else if (Array.isArray(jobOrIds)) {
				pending.expectedJobIds = jobOrIds.filter((id) => id > 0);
				pending.job = { ...pending.job, status: 'queued' };
			} else if (jobOrIds) {
				pending.expectedJobIds = [jobOrIds.id].filter((id) => id > 0);
				pending.job = { ...pending.job, ...jobOrIds };
			} else {
				pending.job = { ...pending.job, status: 'queued' };
			}
			pending.expiresAt = pendingExpiry(pending.job.status);
			pendingJobs.set(token, pending);
			refreshActiveJobsFromPending();
			schedule(250);
		},
		failPendingJob: (token: string, error = 'Unable to start job.') => {
			const pending = pendingJobs.get(token);
			if (!pending) return;
			pending.expectedJobIds = [];
			pending.expiresAt = pendingExpiry('failed');
			pending.job = {
				...pending.job,
				status: 'failed',
				error
			};
			pendingJobs.set(token, pending);
			refreshActiveJobsFromPending();
			schedule(5000);
		},
		removePendingJob: (token: string) => {
			if (!pendingJobs.delete(token)) return;
			refreshActiveJobsFromPending();
		}
	};
}

export const appActivity = createAppActivityStore();

// File format colors for badges
export const formatColors: Record<string, { bg: string; text: string }> = {
	epub: { bg: '#10b981', text: '#ffffff' },   // emerald-500
	pdf: { bg: '#ef4444', text: '#ffffff' },    // red-500
	cbz: { bg: '#8b5cf6', text: '#ffffff' },    // violet-500
	cbr: { bg: '#f59e0b', text: '#ffffff' },    // amber-500
	mobi: { bg: '#06b6d4', text: '#ffffff' },   // cyan-500
	azw: { bg: '#ec4899', text: '#ffffff' },    // pink-500
	azw3: { bg: '#ec4899', text: '#ffffff' },    // pink-500
	mp3: { bg: '#0ea5e9', text: '#ffffff' },    // sky-500
	m4a: { bg: '#0ea5e9', text: '#ffffff' },    // sky-500
	m4b: { bg: '#0ea5e9', text: '#ffffff' },    // sky-500
	flac: { bg: '#0ea5e9', text: '#ffffff' },   // sky-500
	ogg: { bg: '#0ea5e9', text: '#ffffff' },    // sky-500
	wav: { bg: '#0ea5e9', text: '#ffffff' },    // sky-500
	default: { bg: '#6b7280', text: '#ffffff' }  // gray-500
};

export function getFormatColor(format: string): { bg: string; text: string } {
	return formatColors[format.toLowerCase()] || formatColors.default;
}

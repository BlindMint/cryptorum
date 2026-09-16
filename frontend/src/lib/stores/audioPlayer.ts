import { browser } from '$app/environment';
import { get, writable } from 'svelte/store';
import { readerSettings } from '$lib/stores/readerSettings';
import { ReadingProgressController, readingPositionAsLegacy } from '$lib/services/reading-progress';

export interface AudioQueueItem {
	id: number;
	book_id: number;
	file_id: number;
	title: string;
	authors: string[];
	filename: string;
	format: string;
}

interface QueueResponse {
	items: AudioQueueItem[];
	current_item_id: number | null;
}

export interface AudioPlayerState {
	items: AudioQueueItem[];
	currentItemId: number | null;
	isPlaying: boolean;
	isLoading: boolean;
	currentTime: number;
	duration: number;
	playbackSpeed: number;
	expanded: boolean;
	queueOpen: boolean;
	initialized: boolean;
	error: string;
}

const initialState: AudioPlayerState = {
	items: [],
	currentItemId: null,
	isPlaying: false,
	isLoading: false,
	currentTime: 0,
	duration: 0,
	playbackSpeed: 1,
	expanded: true,
	queueOpen: false,
	initialized: false,
	error: ''
};

function createAudioPlayerStore() {
	const { subscribe, update, set } = writable<AudioPlayerState>(initialState);
	let audio: HTMLAudioElement | null = null;
	let progress: ReadingProgressController | null = null;
	let unsubscribeSettings: (() => void) | null = null;
	let pendingSeek: number | null = null;
	let initializedPromise: Promise<void> | null = null;

	function currentItem(state = get({ subscribe })): AudioQueueItem | null {
		return state.items.find((item) => item.id === state.currentItemId) ?? null;
	}

	function applyQueue(response: QueueResponse) {
		update((state) => ({
			...state,
			items: response.items ?? [],
			currentItemId: response.current_item_id ?? response.items?.[0]?.id ?? null,
			initialized: true
		}));
	}

	async function requestQueue(path = '', options?: RequestInit): Promise<QueueResponse> {
		const response = await fetch(`/api/audio/queue${path}`, {
			credentials: 'same-origin',
			...options,
			headers: options?.body ? { 'Content-Type': 'application/json', ...options.headers } : options?.headers
		});
		if (!response.ok) {
			let message = `Audio queue request failed (${response.status})`;
			try {
				const body = await response.json();
				message = body.error || body.message || message;
			} catch {
				// Keep the status-based message.
			}
			throw new Error(message);
		}
		return response.json();
	}

	async function initialize() {
		if (!browser) return;
		if (initializedPromise) return initializedPromise;
		initializedPromise = (async () => {
			try {
				applyQueue(await requestQueue());
				const state = get({ subscribe });
				if (state.currentItemId) await prepareCurrent(false);
			} catch (error) {
				update((state) => ({ ...state, initialized: true, error: error instanceof Error ? error.message : 'Unable to load audio queue' }));
			}
		})();
		return initializedPromise;
	}

	async function endProgress(reachedEnd = false) {
		const controller = progress;
		progress = null;
		if (!controller) return;
		if (audio && Number.isFinite(audio.duration) && audio.duration > 0) {
			await controller.checkpoint({
				readerMode: 'audio',
				percent: reachedEnd ? 100 : (audio.currentTime / audio.duration) * 100,
				locator: { type: 'audio_time', seconds: audio.currentTime, duration: audio.duration },
				reachedEnd
			});
		}
		await controller.end();
		controller.destroy();
	}

	async function prepareCurrent(autoplay: boolean) {
		if (!audio) return;
		const item = currentItem();
		const source = item ? `/api/books/${item.book_id}/file?file_id=${item.file_id}` : '';
		const keepPrimedPlayback = !!item && autoplay && audio.getAttribute('src') === source && !audio.paused;
		await endProgress();
		if (!keepPrimedPlayback) audio.pause();
		pendingSeek = null;
		update((state) => ({ ...state, isPlaying: false, isLoading: !!item, currentTime: 0, duration: 0, error: '' }));
		if (!item) {
			audio.removeAttribute('src');
			audio.load();
			return;
		}
		progress = new ReadingProgressController({
			bookId: item.book_id,
			file: { id: item.file_id, format: item.format },
			channel: 'standard',
			readerMode: 'audio',
			isActivityActive: () => !!audio && !audio.paused && !audio.ended,
			idleTimeoutMs: null
		});
		const position = await progress.start();
		const legacy = readingPositionAsLegacy(position, 'audio');
		pendingSeek = Number(legacy?.seconds) || 0;
		progress.startPeriodicCheckpoint(() => {
			if (!audio || !Number.isFinite(audio.duration) || audio.duration <= 0) return null;
			return {
				readerMode: 'audio',
				percent: (audio.currentTime / audio.duration) * 100,
				locator: { type: 'audio_time', seconds: audio.currentTime, duration: audio.duration }
			};
		});
		if (keepPrimedPlayback) {
			if (pendingSeek !== null && pendingSeek > 0 && Number.isFinite(audio.duration) && pendingSeek < audio.duration) audio.currentTime = pendingSeek;
			pendingSeek = null;
			audio.playbackRate = get({ subscribe }).playbackSpeed;
			update((state) => ({ ...state, isPlaying: true, isLoading: audio?.readyState === 0, currentTime: audio?.currentTime ?? 0, duration: Number.isFinite(audio?.duration) ? audio!.duration : state.duration }));
			return;
		}
		audio.src = source;
		audio.playbackRate = get({ subscribe }).playbackSpeed;
		audio.load();
		if (autoplay) {
			try {
				await audio.play();
			} catch (error) {
				update((state) => ({ ...state, isLoading: false, error: error instanceof Error ? error.message : 'Playback could not start' }));
			}
		}
	}

	function attach(element: HTMLAudioElement) {
		audio = element;
		unsubscribeSettings?.();
		unsubscribeSettings = readerSettings.subscribe((settings) => {
			if (audio) audio.playbackRate = settings.audio.playbackSpeed;
			update((state) => ({ ...state, playbackSpeed: settings.audio.playbackSpeed }));
		});
		return () => {
			unsubscribeSettings?.();
			unsubscribeSettings = null;
			if (audio === element) audio = null;
		};
	}

	function handleLoadedMetadata() {
		if (!audio) return;
		if (pendingSeek !== null && pendingSeek > 0 && pendingSeek < audio.duration) audio.currentTime = pendingSeek;
		pendingSeek = null;
		update((state) => ({ ...state, duration: Number.isFinite(audio?.duration) ? audio!.duration : 0, currentTime: audio?.currentTime ?? 0, isLoading: false }));
	}

	function handleTimeUpdate() {
		if (!audio) return;
		update((state) => ({ ...state, currentTime: audio?.currentTime ?? 0, duration: Number.isFinite(audio?.duration) ? audio!.duration : state.duration }));
	}

	function handlePlaying() {
		update((state) => ({ ...state, isPlaying: true, isLoading: false, error: '' }));
	}

	function handlePause() {
		update((state) => ({ ...state, isPlaying: false }));
		if (progress && audio && Number.isFinite(audio.duration) && audio.duration > 0) {
			void progress.checkpoint({
				readerMode: 'audio',
				percent: (audio.currentTime / audio.duration) * 100,
				locator: { type: 'audio_time', seconds: audio.currentTime, duration: audio.duration }
			});
		}
	}

	function handleWaiting() {
		update((state) => ({ ...state, isLoading: true }));
	}

	function handleError() {
		update((state) => ({ ...state, isLoading: false, isPlaying: false, error: 'This audio file could not be played.' }));
	}

	async function setCurrent(itemID: number, autoplay = true) {
		try {
			const response = await requestQueue('/current', { method: 'PUT', body: JSON.stringify({ item_id: itemID }) });
			applyQueue(response);
			await prepareCurrent(autoplay);
		} catch (error) {
			update((state) => ({ ...state, error: error instanceof Error ? error.message : 'Unable to select audio' }));
		}
	}

	async function playBook(bookID: number, fileID?: number) {
		if (audio && fileID) {
			const source = `/api/books/${bookID}/file?file_id=${fileID}`;
			if (audio.getAttribute('src') !== source) {
				audio.src = source;
				audio.load();
			}
			update((state) => ({ ...state, isLoading: true, error: '' }));
			void audio.play().catch(() => undefined);
		}
		await initialize();
		try {
			const response = await requestQueue('/items', {
				method: 'POST',
				body: JSON.stringify({ book_id: bookID, file_id: fileID, placement: get({ subscribe }).items.length ? 'next' : 'append', make_current: true })
			});
			applyQueue(response);
			update((state) => ({ ...state, expanded: true }));
			await prepareCurrent(true);
		} catch (error) {
			update((state) => ({ ...state, error: error instanceof Error ? error.message : 'Unable to play audio', expanded: true }));
		}
	}

	async function addToQueue(bookID: number, fileID?: number, placement: 'append' | 'next' = 'append') {
		await initialize();
		const hadCurrent = get({ subscribe }).currentItemId !== null;
		try {
			applyQueue(await requestQueue('/items', {
				method: 'POST',
				body: JSON.stringify({ book_id: bookID, file_id: fileID, placement, make_current: false })
			}));
			if (!hadCurrent) await prepareCurrent(false);
		} catch (error) {
			update((state) => ({ ...state, error: error instanceof Error ? error.message : 'Unable to add audio to the queue' }));
		}
	}

	async function togglePlay() {
		if (!audio) return;
		if (audio.paused) {
			try {
				await audio.play();
			} catch (error) {
				update((state) => ({ ...state, error: error instanceof Error ? error.message : 'Playback could not start' }));
			}
		} else {
			audio.pause();
		}
	}

	function seek(seconds: number) {
		if (!audio || !Number.isFinite(audio.duration)) return;
		audio.currentTime = Math.max(0, Math.min(audio.duration, seconds));
		handleTimeUpdate();
	}

	function skip(delta: number) {
		if (audio) seek(audio.currentTime + delta);
	}

	async function next() {
		const state = get({ subscribe });
		const index = state.items.findIndex((item) => item.id === state.currentItemId);
		if (index >= 0 && index < state.items.length - 1) await setCurrent(state.items[index + 1].id);
		else {
			audio?.pause();
			seek(0);
		}
	}

	async function handleEnded() {
		await endProgress(true);
		const state = get({ subscribe });
		const index = state.items.findIndex((item) => item.id === state.currentItemId);
		if (index >= 0 && index < state.items.length - 1) {
			await setCurrent(state.items[index + 1].id);
		} else {
			update((value) => ({ ...value, isPlaying: false, currentTime: value.duration }));
		}
	}

	async function previous() {
		if (audio && audio.currentTime > 5) {
			seek(0);
			return;
		}
		const state = get({ subscribe });
		const index = state.items.findIndex((item) => item.id === state.currentItemId);
		if (index > 0) await setCurrent(state.items[index - 1].id);
		else seek(0);
	}

	async function remove(itemID: number) {
		const wasCurrent = get({ subscribe }).currentItemId === itemID;
		try {
			applyQueue(await requestQueue(`/items/${itemID}`, { method: 'DELETE' }));
			if (wasCurrent) await prepareCurrent(get({ subscribe }).currentItemId !== null);
		} catch (error) {
			update((state) => ({ ...state, error: error instanceof Error ? error.message : 'Unable to remove queue item' }));
		}
	}

	async function clear() {
		try {
			applyQueue(await requestQueue('', { method: 'DELETE' }));
			await prepareCurrent(false);
		} catch (error) {
			update((state) => ({ ...state, error: error instanceof Error ? error.message : 'Unable to clear audio queue' }));
		}
	}

	async function move(itemID: number, direction: -1 | 1) {
		const state = get({ subscribe });
		const from = state.items.findIndex((item) => item.id === itemID);
		const to = from + direction;
		if (from < 0 || to < 0 || to >= state.items.length) return;
		const reordered = [...state.items];
		[reordered[from], reordered[to]] = [reordered[to], reordered[from]];
		update((value) => ({ ...value, items: reordered }));
		try {
			applyQueue(await requestQueue('/reorder', { method: 'PUT', body: JSON.stringify({ item_ids: reordered.map((item) => item.id) }) }));
		} catch (error) {
			update((value) => ({ ...value, items: state.items, error: error instanceof Error ? error.message : 'Unable to reorder queue' }));
		}
	}

	function setPlaybackSpeed(speed: number) {
		const normalized = Math.max(0.5, Math.min(3, speed));
		if (audio) audio.playbackRate = normalized;
		readerSettings.updateAudio({ playbackSpeed: normalized });
		update((state) => ({ ...state, playbackSpeed: normalized }));
	}

	function expand(queueOpen = false) {
		update((state) => ({ ...state, expanded: true, queueOpen: queueOpen || state.queueOpen }));
	}

	function minimize() {
		update((state) => ({ ...state, expanded: false, queueOpen: false }));
	}

	function toggleQueue() {
		update((state) => ({ ...state, expanded: true, queueOpen: !state.queueOpen }));
	}

	return {
		subscribe,
		initialize,
		attach,
		handleLoadedMetadata,
		handleTimeUpdate,
		handlePlaying,
		handlePause,
		handleWaiting,
		handleError,
		handleEnded: () => void handleEnded(),
		playBook,
		addToQueue,
		setCurrent,
		togglePlay,
		seek,
		skip,
		next,
		previous,
		remove,
		clear,
		move,
		setPlaybackSpeed,
		expand,
		minimize,
		toggleQueue,
		reset: () => set(initialState)
	};
}

export const audioPlayer = createAudioPlayerStore();

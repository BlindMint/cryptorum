import { browser } from '$app/environment';

export type ReadingChannel = 'standard' | 'speed';
export type ReaderMode = 'epub_paginated' | 'continuous_text' | 'pdf' | 'comic' | 'audio' | 'speed';
export type ProgressSyncState = 'restoring' | 'saving' | 'synced' | 'offline' | 'conflict' | 'error';

export interface ReaderFile {
	id: number;
	format: string;
	hash?: string;
	path?: string;
}

export interface StoredReadingLocator {
	revision: number;
	locator: Record<string, unknown>;
}

export interface ReadingPosition {
	id?: number;
	book_id: number;
	file_id: number;
	channel: ReadingChannel;
	percent: number;
	active_reader_mode: ReaderMode;
	locators: Record<string, StoredReadingLocator>;
	source_hash: string;
	revision: number;
	updated_at_ms: number;
}

export interface ReadingCheckpoint {
	readerMode: ReaderMode;
	percent: number;
	locator?: Record<string, unknown>;
	reachedEnd?: boolean;
}

interface ReadingProgressOptions {
	bookId: number;
	file: ReaderFile;
	channel: ReadingChannel;
	readerMode: ReaderMode;
	onStateChange?: (state: ProgressSyncState) => void;
}

interface OutboxRecord {
	key: string;
	checkpoint: ReadingCheckpoint;
	updatedAt: number;
}

interface CheckpointAttempt {
	checkpoint: ReadingCheckpoint;
	clientSequence: number;
	baseRevision: number;
}

interface ConflictDetail {
	message: string;
	reloadRemote: () => void;
	takeOver: () => Promise<void>;
}

const DB_NAME = 'cryptorum-reading-progress';
const STORE_NAME = 'checkpoints';
const DB_VERSION = 1;
let outboxDatabasePromise: Promise<IDBDatabase | null> | null = null;

function openOutbox(): Promise<IDBDatabase | null> {
	if (typeof indexedDB === 'undefined') return Promise.resolve(null);
	if (outboxDatabasePromise) return outboxDatabasePromise;
	outboxDatabasePromise = new Promise((resolve) => {
		const request = indexedDB.open(DB_NAME, DB_VERSION);
		request.onupgradeneeded = () => {
			if (!request.result.objectStoreNames.contains(STORE_NAME)) {
				request.result.createObjectStore(STORE_NAME, { keyPath: 'key' });
			}
		};
		request.onsuccess = () => resolve(request.result);
		request.onerror = () => {
			outboxDatabasePromise = null;
			resolve(null);
		};
	});
	return outboxDatabasePromise;
}

async function readOutbox(key: string): Promise<OutboxRecord | null> {
	const db = await openOutbox();
	if (!db) return null;
	return new Promise((resolve) => {
		const request = db.transaction(STORE_NAME, 'readonly').objectStore(STORE_NAME).get(key);
		request.onsuccess = () => resolve((request.result as OutboxRecord | undefined) ?? null);
		request.onerror = () => resolve(null);
	});
}

async function writeOutbox(record: OutboxRecord): Promise<void> {
	const db = await openOutbox();
	if (!db) return;
	await new Promise<void>((resolve) => {
		const request = db.transaction(STORE_NAME, 'readwrite').objectStore(STORE_NAME).put(record);
		request.onsuccess = () => resolve();
		request.onerror = () => resolve();
	});
}

async function deleteOutbox(key: string): Promise<void> {
	const db = await openOutbox();
	if (!db) return;
	await new Promise<void>((resolve) => {
		const request = db.transaction(STORE_NAME, 'readwrite').objectStore(STORE_NAME).delete(key);
		request.onsuccess = () => resolve();
		request.onerror = () => resolve();
	});
}

function clampPercent(percent: number): number {
	if (!Number.isFinite(percent)) return 0;
	return Number(Math.max(0, Math.min(100, percent)).toFixed(4));
}

function dispatchConflict(detail: ConflictDetail) {
	if (!browser) return;
	window.dispatchEvent(new CustomEvent<ConflictDetail>('cryptorum-progress-conflict', { detail }));
}

export class ReadingProgressController {
	readonly bookId: number;
	readonly file: ReaderFile;
	readonly channel: ReadingChannel;
	readerMode: ReaderMode;
	position: ReadingPosition;
	state: ProgressSyncState = 'restoring';

	private sessionId: number | null = null;
	private clientSequence = 0;
	private pending: ReadingCheckpoint | null = null;
	private activeAttempt: CheckpointAttempt | null = null;
	private inFlight: Promise<void> | null = null;
	private paused = false;
	private ended = false;
	private checkpointTimer: ReturnType<typeof setInterval> | null = null;
	private retryTimer: ReturnType<typeof setTimeout> | null = null;
	private retryAttempt = 0;
	private onStateChange?: (state: ProgressSyncState) => void;
	private onlineHandler = () => {
		if (this.paused) return;
		if (this.sessionId === null) {
			void this.start();
		} else {
			void this.flush();
		}
	};
	private focusHandler = () => {
		void this.verifyAuthority();
	};

	constructor(options: ReadingProgressOptions) {
		this.bookId = options.bookId;
		this.file = options.file;
		this.channel = options.channel;
		this.readerMode = options.readerMode;
		this.onStateChange = options.onStateChange;
		this.position = {
			book_id: options.bookId,
			file_id: options.file.id,
			channel: options.channel,
			percent: 0,
			active_reader_mode: options.readerMode,
			locators: {},
			source_hash: options.file.hash ?? '',
			revision: 0,
			updated_at_ms: 0
		};
		if (browser) {
			window.addEventListener('online', this.onlineHandler);
			window.addEventListener('focus', this.focusHandler);
			window.addEventListener('pageshow', this.focusHandler);
		}
	}

	private get outboxKey() {
		return `${this.bookId}:${this.file.id}:${this.channel}`;
	}

	private get remoteReloadKey() {
		return `cryptorum-progress-remote:${this.outboxKey}`;
	}

	private setState(state: ProgressSyncState) {
		this.state = state;
		this.onStateChange?.(state);
	}

	async start(): Promise<ReadingPosition> {
		this.setState('restoring');
		if (browser) {
			const rawRemote = sessionStorage.getItem(this.remoteReloadKey);
			if (rawRemote) {
				sessionStorage.removeItem(this.remoteReloadKey);
				try {
					this.position = JSON.parse(rawRemote) as ReadingPosition;
					this.position.locators ||= {};
					this.paused = true;
					this.setState('conflict');
					setTimeout(() => this.pauseForConflict('remote_loaded', this.position), 0);
					return this.position;
				} catch {
					// Ignore malformed transient state and start normally.
				}
			}
		}
		const local = await readOutbox(this.outboxKey);
		try {
			const response = await fetch(`/api/books/${this.bookId}/reading-sessions`, {
				method: 'POST',
				credentials: 'same-origin',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					file_id: this.file.id,
					channel: this.channel,
					reader_mode: this.readerMode
				})
			});
			if (!response.ok) throw new Error(`Session start failed (${response.status})`);
			const data = await response.json();
			this.sessionId = data.session.id;
			this.clientSequence = 0;
			this.ended = false;
			this.paused = false;
			this.position = data.position as ReadingPosition;
			this.position.locators ||= {};
			if (local?.checkpoint) {
				this.pending = local.checkpoint;
				await this.flush();
			}
			this.setState(this.pending ? 'saving' : 'synced');
			return this.position;
		} catch (error) {
			if (local?.checkpoint) {
				this.pending = local.checkpoint;
				this.position = positionFromCheckpoint(this.position, local.checkpoint);
			}
			this.setState(typeof navigator !== 'undefined' && !navigator.onLine ? 'offline' : 'error');
			console.error('Failed to start reading progress session:', error);
			return this.position;
		}
	}

	async checkpoint(checkpoint: ReadingCheckpoint): Promise<void> {
		const normalized: ReadingCheckpoint = {
			...checkpoint,
			percent: clampPercent(checkpoint.percent)
		};
		this.readerMode = normalized.readerMode;
		this.pending = normalized;
		this.position = positionFromCheckpoint(this.position, normalized);
		await writeOutbox({ key: this.outboxKey, checkpoint: normalized, updatedAt: Date.now() });
		if (!this.paused) await this.flush();
	}

	async flush(): Promise<void> {
		if (this.inFlight) return this.inFlight;
		if ((!this.pending && !this.activeAttempt) || this.paused || this.sessionId === null) return;
		this.inFlight = this.flushLoop().finally(() => {
			this.inFlight = null;
		});
		return this.inFlight;
	}

	private async flushLoop(): Promise<void> {
		while ((this.pending || this.activeAttempt) && !this.paused && this.sessionId !== null) {
			if (!this.activeAttempt) {
				const checkpoint = this.pending;
				if (!checkpoint) return;
				this.pending = null;
				this.activeAttempt = {
					checkpoint,
					clientSequence: ++this.clientSequence,
					baseRevision: this.position.revision
				};
			}
			const attempt = this.activeAttempt;
			this.setState('saving');
			try {
				const response = await fetch(
					`/api/books/${this.bookId}/reading-sessions/${this.sessionId}/position`,
					{
						method: 'PUT',
						credentials: 'same-origin',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify({
							client_sequence: attempt.clientSequence,
							base_revision: attempt.baseRevision,
							reader_mode: attempt.checkpoint.readerMode,
							percent: attempt.checkpoint.percent,
							locator: attempt.checkpoint.locator,
							source_hash: this.file.hash ?? '',
							reached_end: attempt.checkpoint.reachedEnd === true
						})
					}
				);
				if (response.status === 409) {
					const data = await response.json();
					this.pending ??= attempt.checkpoint;
					this.activeAttempt = null;
					this.pauseForConflict(data.reason ?? 'session_superseded', data.position);
					return;
				}
				if (!response.ok) throw new Error(`Checkpoint failed (${response.status})`);
				const data = await response.json();
				this.position = data.position as ReadingPosition;
				this.activeAttempt = null;
				this.retryAttempt = 0;
				if (this.retryTimer) {
					clearTimeout(this.retryTimer);
					this.retryTimer = null;
				}
				if (!this.pending) {
					await deleteOutbox(this.outboxKey);
					this.setState('synced');
				}
			} catch (error) {
				this.setState(typeof navigator !== 'undefined' && !navigator.onLine ? 'offline' : 'error');
				console.error('Failed to save reading progress:', error);
				const retryDelay = Math.min(30_000, 1000 * 2 ** Math.min(this.retryAttempt, 5));
				this.retryAttempt += 1;
				if (this.retryTimer) clearTimeout(this.retryTimer);
				this.retryTimer = setTimeout(() => {
					this.retryTimer = null;
					void this.flush();
				}, retryDelay);
				return;
			}
		}
	}

	private pauseForConflict(reason: string, remotePosition?: ReadingPosition) {
		this.paused = true;
		this.setState('conflict');
		dispatchConflict({
			message: reason === 'source_changed'
				? 'This book file changed while the reader was open. Progress saving is paused.'
				: reason === 'remote_loaded'
					? 'The remote position is loaded. Progress saving remains paused until you take over.'
					: 'This book was opened in a newer reader session. Progress saving is paused.',
			reloadRemote: () => {
				this.pending = null;
				this.activeAttempt = null;
				if (remotePosition) {
					this.position = reason === 'source_changed'
						? { ...remotePosition, locators: {}, source_hash: this.file.hash ?? '' }
						: remotePosition;
					sessionStorage.setItem(this.remoteReloadKey, JSON.stringify(this.position));
				}
				void deleteOutbox(this.outboxKey).finally(() => window.location.reload());
			},
			takeOver: async () => {
				if (!this.pending) {
					const stored = this.position.locators?.[this.readerMode];
					this.pending = {
						readerMode: this.readerMode,
						percent: this.position.percent,
						locator: stored?.locator
					};
					await writeOutbox({ key: this.outboxKey, checkpoint: this.pending, updatedAt: Date.now() });
				}
				this.paused = false;
				await this.start();
			}
		});
	}

	async verifyAuthority(): Promise<void> {
		if (this.sessionId === null || this.paused || this.ended) return;
		try {
			const response = await fetch(`/api/books/${this.bookId}/reading-sessions/${this.sessionId}/position`, {
				credentials: 'same-origin',
				cache: 'no-store'
			});
			if (response.status === 409) {
				const data = await response.json();
				this.pauseForConflict(data.reason ?? 'session_superseded', data.position);
				return;
			}
			if (!response.ok) return;
			const data = await response.json();
			const remote = data.position as ReadingPosition;
			if (remote.revision > this.position.revision && !this.pending) {
				this.pauseForConflict('remote_updated', remote);
			}
		} catch {
			// Focus verification is advisory; the durable save path handles retries.
		}
	}

	startPeriodicCheckpoint(capture: () => ReadingCheckpoint | null, intervalMs = 10_000) {
		this.stopPeriodicCheckpoint();
		this.checkpointTimer = setInterval(() => {
			const checkpoint = capture();
			if (checkpoint) void this.checkpoint(checkpoint);
		}, intervalMs);
	}

	stopPeriodicCheckpoint() {
		if (this.checkpointTimer) {
			clearInterval(this.checkpointTimer);
			this.checkpointTimer = null;
		}
	}

	async end(keepalive = false): Promise<void> {
		if (this.ended) return;
		this.ended = true;
		this.stopPeriodicCheckpoint();
		await this.flush();
		if (this.sessionId === null) return;
		try {
			await fetch(`/api/books/${this.bookId}/reading-sessions/${this.sessionId}`, {
				method: 'PUT',
				credentials: 'same-origin',
				keepalive
			});
		} catch {
			// The durable outbox already contains any unacknowledged checkpoint.
		}
	}

	destroy() {
		this.stopPeriodicCheckpoint();
		if (this.retryTimer) {
			clearTimeout(this.retryTimer);
			this.retryTimer = null;
		}
		if (browser) {
			window.removeEventListener('online', this.onlineHandler);
			window.removeEventListener('focus', this.focusHandler);
			window.removeEventListener('pageshow', this.focusHandler);
		}
	}
}

function positionFromCheckpoint(position: ReadingPosition, checkpoint: ReadingCheckpoint): ReadingPosition {
	const locators = { ...(position.locators ?? {}) };
	if (checkpoint.locator) {
		locators[checkpoint.readerMode] = {
			revision: position.revision,
			locator: checkpoint.locator
		};
	}
	return {
		...position,
		percent: clampPercent(checkpoint.percent),
		active_reader_mode: checkpoint.readerMode,
		locators
	};
}

export function readingPositionAsLegacy(position: ReadingPosition | null | undefined, mode: ReaderMode) {
	if (!position) return null;
	const stored = position.locators?.[mode];
	const locator = stored?.revision === position.revision ? stored.locator : undefined;
	return {
		percent: position.percent ?? 0,
		cfi: locator?.type === 'epub_cfi' ? locator.cfi : undefined,
		page: locator?.type === 'pdf_page' || locator?.type === 'comic_page' ? locator.page : undefined,
		seconds: locator?.type === 'audio_time' ? locator.seconds : undefined,
		duration: locator?.type === 'audio_time' ? locator.duration : undefined,
		text_anchor: locator?.type === 'text_quote' ? locator : undefined,
		speed_reader_word_index: locator?.type === 'word_index' ? locator.word_index : undefined,
		speed_reader_word_count: locator?.type === 'word_index' ? locator.word_count : undefined,
		speed_reader_text_fingerprint: locator?.type === 'word_index' ? locator.text_fingerprint : undefined,
		speed_reader_anchor_words: locator?.type === 'word_index' ? locator.anchor_words : undefined,
		speed_reader_percent: position.channel === 'speed' ? position.percent : 0
	};
}

export function selectReaderFile(files: ReaderFile[], requestedFormat: string, fallbackFormats: string[] = []): ReaderFile | null {
	if (browser) {
		const requestedFileId = Number(new URLSearchParams(window.location.search).get('file_id'));
		if (requestedFileId > 0) {
			const exactFile = files.find((file) => file.id === requestedFileId);
			if (exactFile) return exactFile;
		}
	}
	const normalized = requestedFormat.trim().toLowerCase();
	if (normalized) {
		const exact = files.find((file) => file.format.toLowerCase() === normalized);
		if (exact) return exact;
	}
	for (const format of fallbackFormats) {
		const match = files.find((file) => file.format.toLowerCase() === format.toLowerCase());
		if (match) return match;
	}
	return files[0] ?? null;
}

export function withReaderFile(url: string, file: ReaderFile | null): string {
	if (!file) return url;
	const separator = url.includes('?') ? '&' : '?';
	return `${url}${separator}file_id=${encodeURIComponent(file.id)}`;
}

import { browser } from '$app/environment';

interface CachedBook {
	id: number;
	content: string;
	processedAt: number;
}

interface CachedProcessedEpub {
	id: number;
	buffer: ArrayBuffer;
	locations?: string;
	processedAt: number;
}

const MAX_CACHE_SIZE = 3;
const bookCache = new Map<number, CachedBook>();
const processedEpubCache = new Map<number, CachedProcessedEpub>();

export function getCachedBook(bookId: number): string | null {
	if (!browser) return null;
	const cached = bookCache.get(bookId);
	if (cached) {
		touchCache(bookId);
		return cached.content;
	}
	return null;
}

export function cacheBook(bookId: number, content: string): void {
	if (!browser) return;

	if (bookCache.has(bookId)) {
		bookCache.get(bookId)!.content = content;
		bookCache.get(bookId)!.processedAt = Date.now();
		touchCache(bookId);
		return;
	}

	if (bookCache.size >= MAX_CACHE_SIZE) {
		const lruKey = findLRUKey();
		if (lruKey !== null) {
			bookCache.delete(lruKey);
		}
	}

	bookCache.set(bookId, {
		id: bookId,
		content,
		processedAt: Date.now()
	});
}

export function getCachedProcessedEpub(bookId: number): ArrayBuffer | null {
	if (!browser) return null;
	const cached = processedEpubCache.get(bookId);
	if (!cached) return null;
	touchProcessedEpubCache(bookId);
	return cached.buffer.slice(0);
}

export function cacheProcessedEpub(bookId: number, buffer: ArrayBuffer): void {
	if (!browser) return;

	if (processedEpubCache.has(bookId)) {
		const cached = processedEpubCache.get(bookId)!;
		cached.buffer = buffer.slice(0);
		cached.processedAt = Date.now();
		touchProcessedEpubCache(bookId);
		return;
	}

	if (processedEpubCache.size >= MAX_CACHE_SIZE) {
		const lruKey = findProcessedEpubLRUKey();
		if (lruKey !== null) {
			processedEpubCache.delete(lruKey);
		}
	}

	processedEpubCache.set(bookId, {
		id: bookId,
		buffer: buffer.slice(0),
		processedAt: Date.now()
	});
}

export function getCachedEpubLocations(bookId: number): string | null {
	if (!browser) return null;
	const cached = processedEpubCache.get(bookId);
	if (!cached?.locations) return null;
	touchProcessedEpubCache(bookId);
	return cached.locations;
}

export function cacheEpubLocations(bookId: number, locations: string): void {
	if (!browser) return;
	const cached = processedEpubCache.get(bookId);
	if (!cached) return;
	cached.locations = locations;
	cached.processedAt = Date.now();
}

function touchCache(bookId: number): void {
	const cached = bookCache.get(bookId);
	if (cached) {
		cached.processedAt = Date.now();
	}
}

function touchProcessedEpubCache(bookId: number): void {
	const cached = processedEpubCache.get(bookId);
	if (cached) {
		cached.processedAt = Date.now();
	}
}

function findLRUKey(): number | null {
	let oldestTime = Infinity;
	let oldestKey: number | null = null;

	for (const [key, value] of bookCache.entries()) {
		if (value.processedAt < oldestTime) {
			oldestTime = value.processedAt;
			oldestKey = key;
		}
	}

	return oldestKey;
}

function findProcessedEpubLRUKey(): number | null {
	let oldestTime = Infinity;
	let oldestKey: number | null = null;

	for (const [key, value] of processedEpubCache.entries()) {
		if (value.processedAt < oldestTime) {
			oldestTime = value.processedAt;
			oldestKey = key;
		}
	}

	return oldestKey;
}

export function isBookCached(bookId: number): boolean {
	return bookCache.has(bookId);
}

export function clearBookCache(): void {
	bookCache.clear();
	processedEpubCache.clear();
}

export function getCacheSize(): number {
	return bookCache.size + processedEpubCache.size;
}

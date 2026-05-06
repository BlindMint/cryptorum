import { browser } from '$app/environment';

interface CachedBook {
	id: string;
	content: string;
	processedAt: number;
}

interface CachedProcessedEpub {
	id: string;
	buffer: ArrayBuffer;
	locations?: string;
	processedAt: number;
}

const MAX_CACHE_SIZE = 3;
const bookCache = new Map<string, CachedBook>();
const processedEpubCache = new Map<string, CachedProcessedEpub>();

function makeCacheKey(bookId: number, variant = 'epub'): string {
	return `${bookId}:${variant}`;
}

export function getCachedBook(bookId: number, variant = 'epub'): string | null {
	if (!browser) return null;
	const key = makeCacheKey(bookId, variant);
	const cached = bookCache.get(key);
	if (cached) {
		touchCache(key);
		return cached.content;
	}
	return null;
}

export function cacheBook(bookId: number, content: string, variant = 'epub'): void {
	if (!browser) return;
	const key = makeCacheKey(bookId, variant);

	if (bookCache.has(key)) {
		bookCache.get(key)!.content = content;
		bookCache.get(key)!.processedAt = Date.now();
		touchCache(key);
		return;
	}

	if (bookCache.size >= MAX_CACHE_SIZE) {
		const lruKey = findLRUKey();
		if (lruKey !== null) {
			bookCache.delete(lruKey);
		}
	}

	bookCache.set(key, {
		id: key,
		content,
		processedAt: Date.now()
	});
}

export function getCachedProcessedEpub(bookId: number, variant = 'epub'): ArrayBuffer | null {
	if (!browser) return null;
	const key = makeCacheKey(bookId, variant);
	const cached = processedEpubCache.get(key);
	if (!cached) return null;
	touchProcessedEpubCache(key);
	return cached.buffer.slice(0);
}

export function cacheProcessedEpub(bookId: number, buffer: ArrayBuffer, variant = 'epub'): void {
	if (!browser) return;
	const key = makeCacheKey(bookId, variant);

	if (processedEpubCache.has(key)) {
		const cached = processedEpubCache.get(key)!;
		cached.buffer = buffer.slice(0);
		cached.processedAt = Date.now();
		touchProcessedEpubCache(key);
		return;
	}

	if (processedEpubCache.size >= MAX_CACHE_SIZE) {
		const lruKey = findProcessedEpubLRUKey();
		if (lruKey !== null) {
			processedEpubCache.delete(lruKey);
		}
	}

	processedEpubCache.set(key, {
		id: key,
		buffer: buffer.slice(0),
		processedAt: Date.now()
	});
}

export function getCachedEpubLocations(bookId: number, variant = 'epub'): string | null {
	if (!browser) return null;
	const key = makeCacheKey(bookId, variant);
	const cached = processedEpubCache.get(key);
	if (!cached?.locations) return null;
	touchProcessedEpubCache(key);
	return cached.locations;
}

export function cacheEpubLocations(bookId: number, locations: string, variant = 'epub'): void {
	if (!browser) return;
	const key = makeCacheKey(bookId, variant);
	const cached = processedEpubCache.get(key);
	if (!cached) return;
	cached.locations = locations;
	cached.processedAt = Date.now();
}

function touchCache(cacheKey: string): void {
	const cached = bookCache.get(cacheKey);
	if (cached) {
		cached.processedAt = Date.now();
	}
}

function touchProcessedEpubCache(cacheKey: string): void {
	const cached = processedEpubCache.get(cacheKey);
	if (cached) {
		cached.processedAt = Date.now();
	}
}

function findLRUKey(): string | null {
	let oldestTime = Infinity;
	let oldestKey: string | null = null;

	for (const [key, value] of bookCache.entries()) {
		if (value.processedAt < oldestTime) {
			oldestTime = value.processedAt;
			oldestKey = key;
		}
	}

	return oldestKey;
}

function findProcessedEpubLRUKey(): string | null {
	let oldestTime = Infinity;
	let oldestKey: string | null = null;

	for (const [key, value] of processedEpubCache.entries()) {
		if (value.processedAt < oldestTime) {
			oldestTime = value.processedAt;
			oldestKey = key;
		}
	}

	return oldestKey;
}

export function isBookCached(bookId: number, variant = 'epub'): boolean {
	return bookCache.has(makeCacheKey(bookId, variant));
}

export function clearBookCache(): void {
	bookCache.clear();
	processedEpubCache.clear();
}

export function getCacheSize(): number {
	return bookCache.size + processedEpubCache.size;
}

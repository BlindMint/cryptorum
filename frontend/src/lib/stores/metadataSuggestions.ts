import { writable } from 'svelte/store';

export type MetadataSuggestionField = 'authors' | 'series' | 'publishers' | 'languages' | 'genres' | 'tags';

type MetadataSuggestionState = Record<MetadataSuggestionField, string[]>;

const fields: MetadataSuggestionField[] = ['authors', 'series', 'publishers', 'languages', 'genres', 'tags'];

const emptySuggestions: MetadataSuggestionState = {
	authors: [],
	series: [],
	publishers: [],
	languages: [],
	genres: [],
	tags: []
};

export const metadataSuggestions = writable<MetadataSuggestionState>(emptySuggestions);

let loaded = false;
let loadingPromise: Promise<void> | null = null;

function extractSuggestionNames(items: unknown): string[] {
	if (!Array.isArray(items)) return [];
	return items
		.map((item) => {
			if (typeof item === 'string') return item;
			const name = (item as { name?: unknown })?.name;
			return typeof name === 'string' ? name : '';
		})
		.map((item) => item.trim())
		.filter(Boolean);
}

function mergeSuggestionValues(...lists: string[][]): string[] {
	const seen = new Set<string>();
	const values: string[] = [];

	for (const list of lists) {
		for (const value of list) {
			const trimmed = value.trim();
			if (!trimmed) continue;
			const key = trimmed.toLocaleLowerCase();
			if (seen.has(key)) continue;
			seen.add(key);
			values.push(trimmed);
		}
	}

	return values.sort((a, b) => a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' }));
}

function normalizeSuggestions(values: Partial<Record<MetadataSuggestionField, string[]>>): MetadataSuggestionState {
	const normalized = { ...emptySuggestions };
	for (const field of fields) {
		normalized[field] = mergeSuggestionValues(values[field] ?? []);
	}
	return normalized;
}

export function addMetadataSuggestions(values: Partial<Record<MetadataSuggestionField, string[]>>) {
	const normalized = normalizeSuggestions(values);
	metadataSuggestions.update((current) => {
		const next = { ...current };
		for (const field of fields) {
			next[field] = mergeSuggestionValues(current[field], normalized[field] ?? []);
		}
		return next;
	});
}

function replaceMetadataSuggestions(values: Partial<Record<MetadataSuggestionField, string[]>>) {
	metadataSuggestions.set(normalizeSuggestions(values));
}

export function addMetadataSuggestionsFromPayload(payload: {
	authors?: string[];
	series?: string;
	publisher?: string;
	language?: string;
	genres?: string[];
	tags?: string[];
}) {
	addMetadataSuggestions({
		authors: payload.authors ?? [],
		series: payload.series ? [payload.series] : [],
		publishers: payload.publisher ? [payload.publisher] : [],
		languages: payload.language ? [payload.language] : [],
		genres: payload.genres ?? [],
		tags: payload.tags ?? []
	});
}

export async function loadMetadataSuggestions(force = false): Promise<void> {
	if (loaded && !force) return;
	if (loadingPromise && !force) return loadingPromise;

	loadingPromise = fetch('/api/filter-options', { credentials: 'same-origin' })
		.then(async (res) => {
			if (!res.ok) return;
			const data = await res.json();
			replaceMetadataSuggestions({
				authors: extractSuggestionNames(data.authors),
				series: extractSuggestionNames(data.series),
				publishers: extractSuggestionNames(data.publishers),
				languages: extractSuggestionNames(data.languages),
				genres: extractSuggestionNames(data.genres),
				tags: extractSuggestionNames(data.tags)
			});
			loaded = true;
		})
		.catch(() => {
			// Keep any locally injected suggestions available if a refresh fails.
		})
		.finally(() => {
			loadingPromise = null;
		});

	return loadingPromise;
}

export function refreshMetadataSuggestions(): Promise<void> {
	return loadMetadataSuggestions(true);
}

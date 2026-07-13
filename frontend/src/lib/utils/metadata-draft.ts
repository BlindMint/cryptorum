import type { MetadataEditForm } from './metadata-edit';

const STORAGE_KEY = 'cryptorumMetadataDrafts';
const STORAGE_VERSION = 1;
const MAX_DRAFTS = 40;
const DRAFT_TTL_MS = 30 * 24 * 60 * 60 * 1000;

export interface MetadataDraft {
	id: string;
	scope: string;
	bookId: number;
	form: MetadataEditForm;
	authors: string[];
	baselinePayload: Record<string, unknown>;
	createdAt: number;
	updatedAt: number;
}

export interface PreparedMetadataDraft {
	form: MetadataEditForm;
	baselinePayload: Record<string, unknown>;
	serverChanged: boolean;
}

interface MetadataDraftStore {
	version: number;
	drafts: Record<string, MetadataDraft>;
}

interface PersistMetadataDraftInput {
	id: string;
	bookId: number;
	form: MetadataEditForm;
	authors: string[];
	baselinePayload: Record<string, unknown>;
}

function storageScope(): string {
	if (typeof sessionStorage === 'undefined') return 'default';
	try {
		return sessionStorage.getItem('cryptorumUserId') || 'default';
	} catch {
		return 'default';
	}
}

function emptyStore(): MetadataDraftStore {
	return { version: STORAGE_VERSION, drafts: {} };
}

function readStore(): MetadataDraftStore {
	if (typeof localStorage === 'undefined') return emptyStore();
	try {
		const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
		if (parsed?.version !== STORAGE_VERSION || !parsed?.drafts || typeof parsed.drafts !== 'object') {
			return emptyStore();
		}
		return pruneStore(parsed as MetadataDraftStore);
	} catch {
		return emptyStore();
	}
}

function pruneStore(store: MetadataDraftStore): MetadataDraftStore {
	const cutoff = Date.now() - DRAFT_TTL_MS;
	const drafts = Object.values(store.drafts)
		.filter((draft) =>
			draft &&
			typeof draft.id === 'string' &&
			Number.isFinite(draft.bookId) &&
			draft.updatedAt >= cutoff
		)
		.sort((a, b) => b.updatedAt - a.updatedAt)
		.slice(0, MAX_DRAFTS);
	return {
		version: STORAGE_VERSION,
		drafts: Object.fromEntries(drafts.map((draft) => [draft.id, draft]))
	};
}

function writeStore(store: MetadataDraftStore): boolean {
	if (typeof localStorage === 'undefined') return false;
	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(pruneStore(store)));
		return true;
	} catch {
		return false;
	}
}

function copyDraft(draft: MetadataDraft): MetadataDraft {
	return JSON.parse(JSON.stringify(draft)) as MetadataDraft;
}

export function createMetadataDraftId(bookId: number): string {
	return `${bookId}-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
}

export function prepareMetadataDraftRestore(
	draft: MetadataDraft,
	savedPayload: Record<string, unknown>
): PreparedMetadataDraft {
	const baselinePayload = { ...draft.baselinePayload };
	const form = { ...draft.form };
	if (
		typeof draft.baselinePayload.status === 'string' &&
		form.status === draft.baselinePayload.status &&
		typeof savedPayload.status === 'string' &&
		savedPayload.status !== draft.baselinePayload.status
	) {
		// Reading may move an unread book to reading while the form is parked.
		// Keep that newer status unless the draft explicitly changed it.
		form.status = savedPayload.status;
		baselinePayload.status = savedPayload.status;
	}
	return {
		form,
		baselinePayload,
		serverChanged: JSON.stringify(baselinePayload) !== JSON.stringify(savedPayload)
	};
}

export function persistMetadataDraft(input: PersistMetadataDraftInput): MetadataDraft | null {
	const store = readStore();
	const existing = store.drafts[input.id];
	const now = Date.now();
	const draft: MetadataDraft = {
		id: input.id,
		scope: storageScope(),
		bookId: input.bookId,
		form: JSON.parse(JSON.stringify(input.form)) as MetadataEditForm,
		authors: [...input.authors],
		baselinePayload: JSON.parse(JSON.stringify(input.baselinePayload)),
		createdAt: existing?.createdAt || now,
		updatedAt: now
	};
	store.drafts[draft.id] = draft;
	return writeStore(store) ? copyDraft(draft) : null;
}

export function getMetadataDraft(id: string | null | undefined, bookId?: number): MetadataDraft | null {
	if (!id) return null;
	const draft = readStore().drafts[id];
	if (!draft || draft.scope !== storageScope()) return null;
	if (bookId !== undefined && draft.bookId !== bookId) return null;
	return copyDraft(draft);
}

export function getLatestMetadataDraft(bookId: number): MetadataDraft | null {
	const scope = storageScope();
	const draft = Object.values(readStore().drafts)
		.filter((candidate) => candidate.scope === scope && candidate.bookId === bookId)
		.sort((a, b) => b.updatedAt - a.updatedAt)[0];
	return draft ? copyDraft(draft) : null;
}

export function removeMetadataDraft(id: string | null | undefined): void {
	if (!id) return;
	const store = readStore();
	delete store.drafts[id];
	writeStore(store);
}

export function removeMetadataDraftsForBook(bookId: number): void {
	const scope = storageScope();
	const store = readStore();
	for (const [id, draft] of Object.entries(store.drafts)) {
		if (draft.scope === scope && draft.bookId === bookId) {
			delete store.drafts[id];
		}
	}
	writeStore(store);
}

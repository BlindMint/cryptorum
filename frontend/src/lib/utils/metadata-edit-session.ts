const METADATA_EDIT_SELECTION_STORAGE_KEY = 'cryptorumMetadataEditSelection';

export interface MetadataEditSelectionSession {
	id: string;
	bookIds: number[];
	sourcePath: string;
	createdAt: number;
}

export function startMetadataEditSession(bookIds: number[], sourcePath: string): MetadataEditSelectionSession | null {
	if (typeof window === 'undefined') return null;
	const cleanedIds = Array.from(new Set(bookIds.filter((id) => Number.isFinite(id))));
	if (cleanedIds.length === 0) return null;

	const session: MetadataEditSelectionSession = {
		id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
		bookIds: cleanedIds,
		sourcePath,
		createdAt: Date.now()
	};

	sessionStorage.setItem(METADATA_EDIT_SELECTION_STORAGE_KEY, JSON.stringify(session));
	return session;
}

export function getMetadataEditSession(sessionId?: string | null): MetadataEditSelectionSession | null {
	if (typeof window === 'undefined') return null;
	try {
		const raw = sessionStorage.getItem(METADATA_EDIT_SELECTION_STORAGE_KEY);
		if (!raw) return null;
		const session = JSON.parse(raw) as MetadataEditSelectionSession;
		if (sessionId && session.id !== sessionId) return null;
		if (!Array.isArray(session.bookIds) || session.bookIds.length === 0) return null;
		return session;
	} catch {
		return null;
	}
}

export function getMetadataEditUrl(bookId: number, session?: MetadataEditSelectionSession | null, index?: number): string {
	const params = new URLSearchParams();
	if (session) {
		params.set('selection', session.id);
		params.set('index', String(index ?? Math.max(0, session.bookIds.indexOf(bookId))));
	}
	const query = params.toString();
	return `/book/${bookId}/metadata${query ? `?${query}` : ''}`;
}

export function getInlineMetadataEditUrl(bookId: number, session?: MetadataEditSelectionSession | null, index?: number): string {
	const params = new URLSearchParams();
	params.set('edit', 'metadata');
	if (session) {
		params.set('selection', session.id);
		params.set('index', String(index ?? Math.max(0, session.bookIds.indexOf(bookId))));
	}
	return `/book/${bookId}?${params.toString()}`;
}

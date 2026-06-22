const METADATA_EDIT_SELECTION_STORAGE_KEY = 'cryptorumMetadataEditSelection';
const MAX_STORED_METADATA_EDIT_SESSIONS = 12;

export interface MetadataEditSelectionSession {
	id: string;
	bookIds: number[];
	sourcePath: string;
	createdAt: number;
	kind?: 'selection' | 'context';
	navigationUrl?: string;
	totalCount?: number;
}

interface StoredMetadataEditSessions {
	latestId?: string;
	sessions: Record<string, MetadataEditSelectionSession>;
}

function readStoredSessions(): StoredMetadataEditSessions {
	if (typeof window === 'undefined') return { sessions: {} };
	try {
		const raw = sessionStorage.getItem(METADATA_EDIT_SELECTION_STORAGE_KEY);
		if (!raw) return { sessions: {} };
		const parsed = JSON.parse(raw);
		if (parsed?.sessions && typeof parsed.sessions === 'object') {
			return {
				latestId: typeof parsed.latestId === 'string' ? parsed.latestId : undefined,
				sessions: parsed.sessions
			};
		}
		if (parsed?.id && Array.isArray(parsed.bookIds)) {
			return {
				latestId: parsed.id,
				sessions: { [parsed.id]: parsed as MetadataEditSelectionSession }
			};
		}
	} catch {
		// Ignore malformed session state and replace it on the next write.
	}
	return { sessions: {} };
}

function writeStoredSessions(store: StoredMetadataEditSessions) {
	if (typeof window === 'undefined') return;
	const sessions = Object.values(store.sessions)
		.filter((session) => Array.isArray(session.bookIds) && session.bookIds.length > 0)
		.sort((a, b) => b.createdAt - a.createdAt)
		.slice(0, MAX_STORED_METADATA_EDIT_SESSIONS);
	const nextStore: StoredMetadataEditSessions = {
		latestId: store.latestId,
		sessions: Object.fromEntries(sessions.map((session) => [session.id, session]))
	};
	if (nextStore.latestId && !nextStore.sessions[nextStore.latestId]) {
		nextStore.latestId = sessions[0]?.id;
	}
	sessionStorage.setItem(METADATA_EDIT_SELECTION_STORAGE_KEY, JSON.stringify(nextStore));
}

export function startMetadataEditSession(
	bookIds: number[],
	sourcePath: string,
	kind: 'selection' | 'context' = 'selection',
	options: { navigationUrl?: string; totalCount?: number } = {}
): MetadataEditSelectionSession | null {
	if (typeof window === 'undefined') return null;
	const cleanedIds = Array.from(new Set(bookIds.filter((id) => Number.isFinite(id))));
	if (cleanedIds.length === 0) return null;

	const session: MetadataEditSelectionSession = {
		id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
		bookIds: cleanedIds,
		sourcePath,
		createdAt: Date.now(),
		kind,
		navigationUrl: options.navigationUrl,
		totalCount: Number.isFinite(options.totalCount) ? options.totalCount : cleanedIds.length
	};

	const store = readStoredSessions();
	store.sessions[session.id] = session;
	store.latestId = session.id;
	writeStoredSessions(store);
	return session;
}

export function startMetadataEditContextSession(
	bookIds: number[],
	sourcePath: string,
	options: { navigationUrl?: string; totalCount?: number } = {}
): MetadataEditSelectionSession | null {
	return startMetadataEditSession(bookIds, sourcePath, 'context', options);
}

export function getMetadataEditSession(sessionId?: string | null): MetadataEditSelectionSession | null {
	if (typeof window === 'undefined') return null;
	try {
		const store = readStoredSessions();
		const id = sessionId || store.latestId;
		if (!id) return null;
		const session = store.sessions[id];
		if (!session) return null;
		if (!Array.isArray(session.bookIds) || session.bookIds.length === 0) return null;
		return session;
	} catch {
		return null;
	}
}

function setSessionParams(params: URLSearchParams, bookId: number, session: MetadataEditSelectionSession, index?: number) {
	const sessionKey = session.kind === 'context' ? 'context' : 'selection';
	params.set(sessionKey, session.id);
	params.set('index', String(index ?? Math.max(0, session.bookIds.indexOf(bookId))));
}

export function getMetadataEditUrl(bookId: number, session?: MetadataEditSelectionSession | null, index?: number): string {
	const params = new URLSearchParams();
	if (session) {
		setSessionParams(params, bookId, session, index);
	}
	const query = params.toString();
	return `/book/${bookId}/metadata${query ? `?${query}` : ''}`;
}

export function getBookDetailContextUrl(bookId: number, session?: MetadataEditSelectionSession | null, index?: number): string {
	const params = new URLSearchParams();
	if (session) {
		setSessionParams(params, bookId, session, index);
	}
	const query = params.toString();
	return `/book/${bookId}${query ? `?${query}` : ''}`;
}

export function getInlineMetadataEditUrl(bookId: number, session?: MetadataEditSelectionSession | null, index?: number): string {
	const params = new URLSearchParams();
	params.set('edit', 'metadata');
	if (session) {
		setSessionParams(params, bookId, session, index);
	}
	return `/book/${bookId}?${params.toString()}`;
}

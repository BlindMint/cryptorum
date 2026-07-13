import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createMetadataEditForm } from './metadata-edit';
import {
	getLatestMetadataDraft,
	getMetadataDraft,
	persistMetadataDraft,
	prepareMetadataDraftRestore,
	removeMetadataDraft,
	removeMetadataDraftsForBook
} from './metadata-draft';

class MemoryStorage implements Storage {
	private values = new Map<string, string>();

	get length() {
		return this.values.size;
	}

	clear() {
		this.values.clear();
	}

	getItem(key: string) {
		return this.values.get(key) ?? null;
	}

	key(index: number) {
		return [...this.values.keys()][index] ?? null;
	}

	removeItem(key: string) {
		this.values.delete(key);
	}

	setItem(key: string, value: string) {
		this.values.set(key, value);
	}
}

function draftInput(id: string, bookId: number, title: string) {
	const form = createMetadataEditForm({ title, authors: '["Author"]' });
	return {
		id,
		bookId,
		form,
		authors: ['Author', ''],
		baselinePayload: { title: 'Original title' }
	};
}

beforeEach(() => {
	vi.useFakeTimers();
	vi.setSystemTime(new Date('2026-07-13T12:00:00Z'));
	vi.stubGlobal('localStorage', new MemoryStorage());
	vi.stubGlobal('sessionStorage', new MemoryStorage());
	sessionStorage.setItem('cryptorumUserId', '7');
});

afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe('metadata drafts', () => {
	it('persists a draft for the active user and returns an isolated copy', () => {
		const saved = persistMetadataDraft(draftInput('draft-1', 10, 'Edited title'));
		expect(saved?.form.title).toBe('Edited title');

		const loaded = getMetadataDraft('draft-1', 10);
		expect(loaded?.authors).toEqual(['Author', '']);
		if (loaded) loaded.form.title = 'Changed after loading';
		expect(getMetadataDraft('draft-1', 10)?.form.title).toBe('Edited title');

		sessionStorage.setItem('cryptorumUserId', '8');
		expect(getMetadataDraft('draft-1', 10)).toBeNull();
		expect(getLatestMetadataDraft(10)).toBeNull();
	});

	it('finds the latest book draft and supports targeted cleanup', () => {
		persistMetadataDraft(draftInput('older', 10, 'Older edit'));
		vi.setSystemTime(Date.now() + 1000);
		persistMetadataDraft(draftInput('newer', 10, 'Newer edit'));
		persistMetadataDraft(draftInput('other-book', 11, 'Other book'));

		expect(getLatestMetadataDraft(10)?.id).toBe('newer');
		removeMetadataDraft('newer');
		expect(getLatestMetadataDraft(10)?.id).toBe('older');

		removeMetadataDraftsForBook(10);
		expect(getLatestMetadataDraft(10)).toBeNull();
		expect(getLatestMetadataDraft(11)?.id).toBe('other-book');
	});

	it('keeps a reader-driven status change unless the draft edited status explicitly', () => {
		const unchangedStatusDraft = persistMetadataDraft({
			...draftInput('status-unchanged', 10, 'Edited title'),
			baselinePayload: { title: 'Original title', status: 'unread' }
		});
		expect(unchangedStatusDraft).not.toBeNull();
		const readerUpdated = prepareMetadataDraftRestore(unchangedStatusDraft!, {
			title: 'Original title',
			status: 'reading'
		});
		expect(readerUpdated.form.status).toBe('reading');
		expect(readerUpdated.serverChanged).toBe(false);

		const explicitStatusDraft = {
			...unchangedStatusDraft!,
			form: { ...unchangedStatusDraft!.form, status: 'finished' }
		};
		const explicitStatus = prepareMetadataDraftRestore(explicitStatusDraft, {
			title: 'Original title',
			status: 'reading'
		});
		expect(explicitStatus.form.status).toBe('finished');
		expect(explicitStatus.serverChanged).toBe(true);
	});
});

import { describe, expect, it } from 'vitest';
import {
	MAX_COMBINE_BOOKS,
	combineSelectionError,
	suggestedCombinePrimaryId,
	uniqueLibraryIds
} from './combine-books';

describe('combineSelectionError', () => {
	it('rejects filtered-mode selections', () => {
		expect(combineSelectionError(8, true)).toMatch(/specific books/);
	});

	it('requires at least two books', () => {
		expect(combineSelectionError(1)).toMatch(/at least two/);
	});

	it('caps the selection size', () => {
		expect(combineSelectionError(MAX_COMBINE_BOOKS + 1)).toMatch(/at most/);
	});

	it('allows a small explicit selection', () => {
		expect(combineSelectionError(2)).toBeNull();
	});
});

describe('suggestedCombinePrimaryId', () => {
	it('prefers the book with the most recent reading activity', () => {
		expect(suggestedCombinePrimaryId([
			{ id: 1, title: 'A', last_read_at: 10, percent: 90 },
			{ id: 2, title: 'B', last_read_at: 50, percent: 10 }
		])).toBe(2);
	});

	it('falls back to progress, then cover', () => {
		expect(suggestedCombinePrimaryId([
			{ id: 3, title: 'No Cover' },
			{ id: 4, title: 'Has Cover', cover_path: '/covers/4.jpg' }
		])).toBe(4);
		expect(suggestedCombinePrimaryId([
			{ id: 5, title: 'Unread' },
			{ id: 6, title: 'Reading', status: 'reading', percent: 12 }
		])).toBe(6);
	});
});

describe('uniqueLibraryIds', () => {
	it('collects distinct library ids', () => {
		expect(uniqueLibraryIds([
			{ id: 1, library_id: 1 },
			{ id: 2, library_id: 1 },
			{ id: 3, library_id: 2 }
		])).toEqual([1, 2]);
	});
});

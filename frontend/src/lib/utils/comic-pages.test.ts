import { describe, expect, it } from 'vitest';
import { comicPageGroups, comicGroupIndex, comicSnapIndex } from './comic-pages';

describe('comic snap groups', () => {
	it('keeps every page in one stable frame when spreads are off', () => {
		expect(comicPageGroups(3, 'off')).toEqual([[1], [2], [3]]);
		expect(comicPageGroups(0, 'off')).toEqual([]);
	});
	it('starts even spreads after the cover and odd spreads on the first page', () => {
		expect(comicPageGroups(5, 'even')).toEqual([[1], [2, 3], [4, 5]]);
		expect(comicPageGroups(5, 'odd')).toEqual([[1, 2], [3, 4], [5]]);
		expect(comicPageGroups(4, 'even')).toEqual([[1], [2, 3], [4]]);
	});
	it('locates either page of a spread for seeking and navigation', () => {
		const groups = comicPageGroups(5, 'even');
		expect(comicGroupIndex(groups, 2)).toBe(1);
		expect(comicGroupIndex(groups, 3)).toBe(1);
		expect(comicGroupIndex(groups, 5)).toBe(2);
	});
	it('finds the same settled frame in LTR and RTL and bounds overscroll', () => {
		expect(comicSnapIndex(901, 900, 5)).toBe(1);
		expect(comicSnapIndex(-901, 900, 5)).toBe(1);
		expect(comicSnapIndex(10000, 900, 5)).toBe(4);
		expect(comicSnapIndex(200, 0, 5)).toBe(0);
	});
});

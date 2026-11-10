import { describe, expect, it } from 'vitest';
import { isComicScrollEnd, comicProgressPageAtX } from './comic-progress';

describe('comic scroll completion', () => {
	it.each([
		['vertical end', 900, 1000, 100, true, true, true],
		['fractional end', 898.5, 1000, 100, true, true, true],
		['RTL horizontal end', -900, 1000, 100, true, true, true],
		['before end', 890, 1000, 100, true, true, false],
		['unloaded final page', 900, 1000, 100, false, false, false],
		['collapsed lazy strip', 0, 100, 100, true, false, false],
		['fully visible loaded comic', 0, 90, 100, true, true, true]
	])('%s', (_name, position, extent, viewport, loaded, allLoaded, expected) => {
		expect(isComicScrollEnd(position as number, extent as number, viewport as number, loaded as boolean, allLoaded as boolean)).toBe(expected);
	});
});


describe('comic progress direction', () => {
	it('maps the beginning and end to opposite physical edges in RTL', () => {
		expect(comicProgressPageAtX(20, 20, 200, 6, false)).toBe(1);
		expect(comicProgressPageAtX(220, 20, 200, 6, false)).toBe(6);
		expect(comicProgressPageAtX(220, 20, 200, 6, true)).toBe(1);
		expect(comicProgressPageAtX(20, 20, 200, 6, true)).toBe(6);
	});
	it('mirrors preview and drag coordinates while clamping outside the track', () => {
		expect(comicProgressPageAtX(70, 20, 200, 5, false)).toBe(2);
		expect(comicProgressPageAtX(70, 20, 200, 5, true)).toBe(4);
		expect(comicProgressPageAtX(-100, 20, 200, 5, true)).toBe(5);
		expect(comicProgressPageAtX(500, 20, 200, 5, true)).toBe(1);
		expect(comicProgressPageAtX(20, 20, 200, 1, true)).toBe(1);
		expect(comicProgressPageAtX(20, 20, 0, 6, true)).toBeNull();
	});
});

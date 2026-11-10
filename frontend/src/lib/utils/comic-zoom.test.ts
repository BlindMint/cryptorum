import { describe, expect, it } from 'vitest';
import { clampComicZoom, comicZoomScrollDelta } from './comic-zoom';

describe('comic-only zoom', () => {
	it('bounds image zoom without changing the viewport', () => {
		expect(clampComicZoom(0.01)).toBe(0.25);
		expect(clampComicZoom(20)).toBe(5);
		expect(clampComicZoom(1.5)).toBe(1.5);
	});
	it('keeps the image point beneath the pinch midpoint', () => {
		expect(comicZoomScrollDelta({ left: 0, top: -200 }, { left: 0, top: -200 }, 1, 2, { x: 100, y: 300 })).toEqual({ left: 100, top: 500 });
	});
	it('accounts for centering or RTL layout shifts while zooming', () => {
		expect(comicZoomScrollDelta({ left: -400, top: 0 }, { left: -1000, top: 0 }, 1, 2, { x: 100, y: 200 })).toEqual({ left: -100, top: 200 });
	});
});

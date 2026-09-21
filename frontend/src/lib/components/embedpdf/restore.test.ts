import { describe, expect, it } from 'vitest';
import { didRestoreLand, pageToRestore, shouldAdoptDocumentPageCount } from './restore';

describe('pageToRestore', () => {
	it('keeps the saved page when the document total is still unknown', () => {
		expect(pageToRestore(309)).toBe(309);
		expect(pageToRestore(309, 0)).toBe(309);
	});

	it('does not clamp a mid-book resume to an incomplete first-page total', () => {
		expect(pageToRestore(309, 1)).toBe(309);
		expect(pageToRestore(327, 2)).toBe(327);
	});

	it('keeps a valid in-document page and does not clamp down to a smaller total', () => {
		expect(pageToRestore(309, 736)).toBe(309);
		expect(pageToRestore(900, 736)).toBe(900);
	});
});

describe('didRestoreLand', () => {
	it('does not treat page 1 as restored when the saved page is later in the book', () => {
		expect(didRestoreLand(1, 309)).toBe(false);
		expect(didRestoreLand(1, 327)).toBe(false);
	});

	it('lands only when the measured page is the saved page', () => {
		expect(didRestoreLand(309, 309)).toBe(true);
		expect(didRestoreLand(308, 309)).toBe(false);
	});

	it('has nothing to restore when the saved page is the start of the book', () => {
		expect(didRestoreLand(1, 1)).toBe(true);
	});
});

describe('shouldAdoptDocumentPageCount', () => {
	it('ignores a 1-page total while metadata already knows the full document', () => {
		expect(shouldAdoptDocumentPageCount(1, 736)).toBe(false);
		expect(shouldAdoptDocumentPageCount(2, 736)).toBe(false);
	});

	it('adopts a total once it matches or exceeds the known page count', () => {
		expect(shouldAdoptDocumentPageCount(736, 736)).toBe(true);
		expect(shouldAdoptDocumentPageCount(736, 0)).toBe(true);
		expect(shouldAdoptDocumentPageCount(800, 736)).toBe(true);
	});
});

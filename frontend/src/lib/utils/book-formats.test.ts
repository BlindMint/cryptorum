import { describe, expect, it } from 'vitest';
import {
	bookFileName,
	formatHasDuplicateFiles,
	getPrimaryReadableFile,
	getReadableBookFiles
} from './book-formats';

const files = [
	{ id: 1, format: 'pdf', path: '/books/Nmap Official.pdf', size: 50 },
	{ id: 2, format: 'pdf', path: '/books/Nmap Scan.pdf', size: 120 },
	{ id: 3, format: 'epub', path: '/books/Nmap.epub', size: 8 },
	{ id: 4, format: 'cb7', path: '/books/skip.cb7', size: 3 }
];

describe('getReadableBookFiles', () => {
	it('omits unsupported comic archives', () => {
		expect(getReadableBookFiles(files).map((file) => file.id)).toEqual([1, 2, 3]);
	});
});

describe('formatHasDuplicateFiles', () => {
	it('detects two PDFs of the same book', () => {
		expect(formatHasDuplicateFiles(files, 'pdf')).toBe(true);
		expect(formatHasDuplicateFiles(files, 'epub')).toBe(false);
	});
});

describe('getPrimaryReadableFile', () => {
	it('uses the resume file when it is still present', () => {
		expect(getPrimaryReadableFile(files, 2)?.id).toBe(2);
	});

	it('falls back to the preferred format', () => {
		expect(getPrimaryReadableFile(files)?.id).toBe(3);
	});
});

describe('bookFileName', () => {
	it('returns the basename', () => {
		expect(bookFileName('/books/Nmap Official.pdf')).toBe('Nmap Official.pdf');
	});
});

import { describe, expect, it } from 'vitest';
import {
	createPendingCover,
	imageFileFromClipboard,
	imageFileFromDataTransfer,
	imageFileFromList,
	isImageFile,
	pendingCoverDirty,
	pendingCoverDisplaySrc,
	pendingCoverIsCustom,
	stagePendingCoverReset
} from './cover-upload';

function file(name: string, type: string): File {
	return new File(['cover'], name, { type });
}

describe('isImageFile', () => {
	it('accepts image MIME types and common extensions', () => {
		expect(isImageFile(file('shot.png', 'image/png'))).toBe(true);
		expect(isImageFile(file('shot.jpg', ''))).toBe(true);
		expect(isImageFile(file('notes.txt', 'text/plain'))).toBe(false);
	});
});

describe('imageFileFromList', () => {
	it('returns the first image and ignores other files', () => {
		expect(imageFileFromList([file('notes.txt', 'text/plain'), file('cover.webp', 'image/webp')])?.name).toBe('cover.webp');
		expect(imageFileFromList([file('notes.txt', 'text/plain')])).toBeNull();
	});
});

describe('imageFileFromClipboard', () => {
	it('reads an image from clipboard files', () => {
		const clipboardData = {
			files: [file('paste.png', 'image/png')],
			items: []
		} as unknown as DataTransfer;
		const event = { clipboardData } as ClipboardEvent;
		expect(imageFileFromClipboard(event)?.name).toBe('paste.png');
	});
});

describe('imageFileFromDataTransfer', () => {
	it('reads the first dropped image', () => {
		const data = { files: [file('drop.jpg', 'image/jpeg')] } as unknown as DataTransfer;
		expect(imageFileFromDataTransfer(data)?.name).toBe('drop.jpg');
	});
});

describe('pending cover staging', () => {
	it('treats a staged file as a custom cover until reset', () => {
		const staged = { file: file('cover.png', 'image/png'), previewUrl: 'blob:cover', reset: false };
		expect(pendingCoverDirty(staged)).toBe(true);
		expect(pendingCoverIsCustom(staged, '')).toBe(true);
		expect(pendingCoverDisplaySrc(staged, '/original.jpg')).toBe('blob:cover');
	});

	it('discards a staged file without resetting the original cover', () => {
		const staged = { file: file('cover.png', 'image/png'), previewUrl: null, reset: false };
		const next = stagePendingCoverReset(staged, true);
		expect(next).toEqual(createPendingCover());
		expect(pendingCoverDisplaySrc(next, '/original.jpg')).toBe('/original.jpg');
	});

	it('stages a reset of an existing custom cover', () => {
		const next = stagePendingCoverReset(createPendingCover(), true, 'blob:source');
		expect(next.reset).toBe(true);
		expect(pendingCoverIsCustom(next, 'custom')).toBe(false);
		expect(pendingCoverDisplaySrc(next, '/original.jpg')).toBe('blob:source');
	});
});

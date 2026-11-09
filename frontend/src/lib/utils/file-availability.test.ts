import { describe, expect, it } from 'vitest';
import { fileResponseError, requireReadableFile } from './file-availability';
import { getPrimaryReadableFile } from './book-formats';

describe('file availability', () => {
	it('chooses an accessible alternative when the resume file is missing', () => {
		const files = [
			{ id: 1, format: 'pdf', availability: { code: 'file_missing' } },
			{ id: 2, format: 'epub', availability: { code: 'available' } }
		];
		expect(getPrimaryReadableFile(files, 1, 'pdf')?.id).toBe(2);
	});

	it('stops unavailable and absent files before opening a reader session', () => {
		expect(() => requireReadableFile(null)).toThrow('saved reading progress is preserved');
		expect(() => requireReadableFile({ availability: { code: 'file_unreadable', message: 'Permission required' } })).toThrow('Permission required');
		expect(() => requireReadableFile({ availability: { code: 'available' } })).not.toThrow();
	});

	it('preserves structured errors and handles expired sessions and proxy failures', async () => {
		expect(await fileResponseError(new Response(JSON.stringify({ code: 'file_missing', error: 'Folder disconnected' }), { status: 404 }))).toBe('Folder disconnected');
		expect(await fileResponseError(new Response('', { status: 401 }))).toContain('Sign in again');
		expect(await fileResponseError(new Response('<html>Bad gateway</html>', { status: 502 }))).toContain('library storage');
	});
});

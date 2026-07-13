import 'fake-indexeddb/auto';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
	ReadingProgressController,
	readingPositionAsLegacy,
	type ReadingPosition
} from './reading-progress';

function position(overrides: Partial<ReadingPosition> = {}): ReadingPosition {
	return {
		book_id: 1,
		file_id: 10,
		channel: 'standard',
		percent: 0,
		active_reader_mode: 'pdf',
		locators: {},
		source_hash: 'hash-one',
		revision: 0,
		updated_at_ms: 0,
		...overrides
	};
}

function jsonResponse(body: unknown, status = 200) {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe('ReadingProgressController', () => {
	it('persists a failed checkpoint and sends it after the reader is reopened', async () => {
		vi.spyOn(console, 'error').mockImplementation(() => undefined);
		let sessionStarts = 0;
		let checkpointAttempts = 0;
		const sentBodies: any[] = [];
		vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = String(input);
			if (url.endsWith('/reading-sessions')) {
				sessionStarts += 1;
				return jsonResponse({
					session: { id: sessionStarts },
					position: position()
				}, 201);
			}
			if (url.endsWith('/position')) {
				checkpointAttempts += 1;
				const body = JSON.parse(String(init?.body));
				sentBodies.push(body);
				if (checkpointAttempts === 1) throw new Error('offline');
				return jsonResponse({
					status: 'ok',
					position: position({
						percent: body.percent,
						revision: 1,
						locators: {
							pdf: { revision: 1, locator: body.locator }
						}
					})
				});
			}
			return jsonResponse({ status: 'ok' });
		}));

		const first = new ReadingProgressController({
			bookId: 1001,
			file: { id: 10, format: 'pdf', hash: 'hash-one' },
			channel: 'standard',
			readerMode: 'pdf'
		});
		await first.start();
		await first.checkpoint({
			readerMode: 'pdf',
			percent: 31,
			locator: { type: 'pdf_page', page: 228, total_pages: 736 }
		});
		expect(['offline', 'error']).toContain(first.state);

		const reopened = new ReadingProgressController({
			bookId: 1001,
			file: { id: 10, format: 'pdf', hash: 'hash-one' },
			channel: 'standard',
			readerMode: 'pdf'
		});
		const restored = await reopened.start();
		expect(restored.percent).toBe(31);
		expect(sentBodies.at(-1)).toMatchObject({
			base_revision: 0,
			percent: 31,
			locator: { type: 'pdf_page', page: 228 }
		});
		expect(reopened.state).toBe('synced');
		first.destroy();
		reopened.destroy();
	});

	it('retries an unacknowledged checkpoint with the same ordering values', async () => {
		vi.spyOn(console, 'error').mockImplementation(() => undefined);
		const bodies: any[] = [];
		vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = String(input);
			if (url.endsWith('/reading-sessions')) {
				return jsonResponse({ session: { id: 12 }, position: position() }, 201);
			}
			if (url.endsWith('/position')) {
				const body = JSON.parse(String(init?.body));
				bodies.push(body);
				if (bodies.length === 1) throw new Error('response lost');
				return jsonResponse({
					status: 'ok',
					position: position({
						percent: body.percent,
						revision: body.client_sequence,
						locators: {
							pdf: { revision: body.client_sequence, locator: body.locator }
						}
					})
				});
			}
			return jsonResponse({ status: 'ok' });
		}));

		const controller = new ReadingProgressController({
			bookId: 1003,
			file: { id: 10, format: 'pdf', hash: 'hash-one' },
			channel: 'standard',
			readerMode: 'pdf'
		});
		await controller.start();
		await controller.checkpoint({
			readerMode: 'pdf',
			percent: 10,
			locator: { type: 'pdf_page', page: 2, total_pages: 10 }
		});
		await controller.checkpoint({
			readerMode: 'pdf',
			percent: 20,
			locator: { type: 'pdf_page', page: 3, total_pages: 10 }
		});

		expect(bodies.map((body) => [body.client_sequence, body.base_revision, body.percent])).toEqual([
			[1, 0, 10],
			[1, 0, 10],
			[2, 1, 20]
		]);
		expect(controller.state).toBe('synced');
		controller.destroy();
	});

	it('ignores a mode locator captured at an older shared revision', () => {
		const legacy = readingPositionAsLegacy(position({
			percent: 42,
			revision: 5,
			locators: {
				epub_paginated: {
					revision: 4,
					locator: { type: 'epub_cfi', cfi: 'epubcfi(/6/2)' }
				}
			}
		}), 'epub_paginated');

		expect(legacy?.percent).toBe(42);
		expect(legacy?.cfi).toBeUndefined();
	});

	it('normalizes the first and last speed-reader words to 0 and 100 percent', async () => {
		const bodies: any[] = [];
		vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = String(input);
			if (url.endsWith('/reading-sessions')) {
				return jsonResponse({ session: { id: 9 }, position: position({ channel: 'speed', active_reader_mode: 'speed' }) }, 201);
			}
			if (url.endsWith('/position')) {
				const body = JSON.parse(String(init?.body));
				bodies.push(body);
				return jsonResponse({ status: 'ok', position: position({ channel: 'speed', active_reader_mode: 'speed', percent: body.percent, revision: bodies.length }) });
			}
			return jsonResponse({ status: 'ok' });
		}));
		const controller = new ReadingProgressController({
			bookId: 1002,
			file: { id: 10, format: 'epub', hash: 'hash-one' },
			channel: 'speed',
			readerMode: 'speed'
		});
		await controller.start();
		await controller.checkpoint({ readerMode: 'speed', percent: 0, locator: { type: 'word_index', word_index: 0, word_count: 11 } });
		await controller.checkpoint({ readerMode: 'speed', percent: 100, locator: { type: 'word_index', word_index: 10, word_count: 11 }, reachedEnd: true });
		expect(bodies.map((body) => body.percent)).toEqual([0, 100]);
		controller.destroy();
	});
});

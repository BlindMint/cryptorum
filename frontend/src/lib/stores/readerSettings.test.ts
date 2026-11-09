import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { resolveComicSettings } from '$lib/utils/comic-settings';

vi.mock('$app/environment', () => ({ browser: true }));

beforeEach(() => {
	vi.resetModules();
	vi.useFakeTimers();
	const data = new Map<string, string>();
	vi.stubGlobal('localStorage', {
		getItem: (key: string) => data.get(key) ?? null,
		setItem: (key: string, value: string) => data.set(key, value)
	});
	vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true }));
});
afterEach(() => { vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals(); });

describe('comic reading preferences', () => {
	it('updates global defaults until an override is enabled, then isolates reading choices', async () => {
		const { readerSettings } = await import('./readerSettings');
		readerSettings.updateComic('12', { fitMode: 'fit-width', scrollMode: 'infinite' });
		expect(get(readerSettings).cbx.fitMode).toBe('fit-width');
		readerSettings.setComicOverride('12', true);
		readerSettings.updateComic('12', { fitMode: 'actual-size', readingDirection: 'rtl', backgroundColor: '#ffffff', autoHideControls: false });
		let value = get(readerSettings);
		expect(value.cbx.fitMode).toBe('fit-width');
		expect(value.cbx.readingDirection).toBe('ltr');
		expect(value.cbx.backgroundColor).toBe('#ffffff');
		expect(value.cbx.autoHideControls).toBe(false);
		expect(resolveComicSettings(value.cbx, value.comicOverrides?.['12']).fitMode).toBe('actual-size');
		expect(resolveComicSettings(value.cbx, value.comicOverrides?.['13']).fitMode).toBe('fit-width');
		// Global changes leave the saved reading choices intact.
		readerSettings.updateCbx({ scrollMode: 'paginated' });
		value = get(readerSettings);
		expect(resolveComicSettings(value.cbx, value.comicOverrides?.['12']).scrollMode).toBe('infinite');
		readerSettings.setComicOverride('12', false);
		value = get(readerSettings);
		expect(value.comicOverrides?.['12']).toBeUndefined();
		expect(resolveComicSettings(value.cbx).scrollMode).toBe('paginated');
	});

	it('persists overrides locally and through the existing backend settings endpoint', async () => {
		const { readerSettings } = await import('./readerSettings');
		readerSettings.setComicOverride('12', true);
		readerSettings.updateComic('12', { fitMode: 'fit-height', pageSpread: 'odd' });
		await readerSettings.flushPendingSave();
		expect(JSON.parse(localStorage.getItem('readerSettings')!).comicOverrides['12'].fitMode).toBe('fit-height');
		expect(JSON.parse(vi.mocked(fetch).mock.calls.at(-1)![1]!.body as string).reader.comicOverrides['12'].pageSpread).toBe('odd');
		vi.resetModules();
		const reloaded = await import('./readerSettings');
		expect(get(reloaded.readerSettings).comicOverrides?.['12'].fitMode).toBe('fit-height');
	});

	it('migrates manga mode and opening size once without undoing later selections', async () => {
		localStorage.setItem('readerSettings', JSON.stringify({ cbx: { mangaMode: true, openingFitMode: 'fit-width', fitMode: 'fit-page' } }));
		const { readerSettings } = await import('./readerSettings');
		expect(get(readerSettings).cbx.readingDirection).toBe('rtl');
		expect(get(readerSettings).cbx.fitMode).toBe('fit-width');
		readerSettings.updateCbx({ readingDirection: 'ltr', fitMode: 'fit-height' });
		vi.resetModules();
		const reloaded = await import('./readerSettings');
		expect(get(reloaded.readerSettings).cbx.readingDirection).toBe('ltr');
		expect(get(reloaded.readerSettings).cbx.fitMode).toBe('fit-height');
	});
});

import type { CbxReaderSetting } from '$lib/stores/readerSettings';

export const comicReadingKeys = ['fitMode', 'scrollMode', 'readingDirection', 'pageSpread', 'stripMaxWidthPercent'] as const;
export type ComicReadingSettings = Pick<CbxReaderSetting, typeof comicReadingKeys[number]>;

export function pickComicReading(settings: CbxReaderSetting): ComicReadingSettings {
	return Object.fromEntries(comicReadingKeys.map(key => [key, settings[key]])) as ComicReadingSettings;
}

export function resolveComicSettings(defaults: CbxReaderSetting, override?: Partial<ComicReadingSettings>): CbxReaderSetting {
	return { ...defaults, ...Object.fromEntries(comicReadingKeys.filter(key => override?.[key] !== undefined).map(key => [key, override![key]])), mangaMode: false };
}

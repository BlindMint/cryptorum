import { writable, derived } from 'svelte/store';
import { browser } from '$app/environment';

const READER_SETTINGS_VERSION = 5;
const READER_SETTINGS_VERSION_KEY = 'readerSettingsVersion';
const DEFAULT_READER_THEME = 'catppuccin';

export interface EpubReaderSetting {
	fontFamily: string;
	fontSize: number;
	fontWeight: number;
	fontStyle: 'normal' | 'italic';
	lineHeight: number;
	letterSpacing: number;
	paragraphSpacing: number;
	paragraphIndent: number;
	justify: boolean;
	hyphenate: boolean;
	hyphenationLanguage: string;
	maxColumnCount: number;
	gap: number;
	theme: string;
	isDark: boolean;
	flow: 'paginated' | 'scrolled';
	maxInlineSize: number;
	maxBlockSize: number;
	margin: number;
	continuousMaxWidth: number;
	brightness: number;
	contrast: number;
	pageAnimation: 'slide' | 'fade' | 'none';
	autoAdvance: boolean;
	autoAdvanceTimer: number;
	fullscreenLock: boolean;
	useStandardFullscreen: boolean;
	autoHideControls: boolean;
	customCss: string;
	showTextLayer: boolean;
	originalLayout: boolean;
	continuousMode: boolean;
	showImages: boolean;
	imageSize: 'fit-width' | 'fit-page' | 'actual-size';
	imageGrayscale: boolean;
}

export type PdfViewMode = 'light' | 'dark' | 'trueDark';
export type PdfPageLayout = 'single' | 'double';

export interface PdfReaderSetting {
	pageSpread: 'off' | 'even' | 'odd';
	pageLayout: PdfPageLayout;
	pageZoom: string;
	zoomLevel: number;
	renderQuality: 'standard' | 'high' | 'maximum';
	autoHideControls: boolean;
	showSidebar: boolean;
	scrollDirection: 'vertical' | 'horizontal';
	scrollMode: 'paged' | 'continuous-vertical';
	pageRotation: 0 | 90 | 180 | 270;
	backgroundColor: string;
	brightness: number;
	contrast: number;
	grayscale: number;
	readingDirection: 'ltr' | 'rtl';
	autoCropMargins: boolean;
	textLayerEnabled: boolean;
	annotationsEnabled: boolean;
	viewMode: PdfViewMode;
	showChapterMarkers: boolean;
	showQuoteMarks: boolean;
	panMode: boolean;
	useStandardFullscreen: boolean;
	keepScreenOn: boolean;
}

export interface CbxReaderSetting {
	pageSpread: 'off' | 'even' | 'odd' | 'auto';
	pageLayout: 'single' | 'double';
	fitMode: 'fit-page' | 'fit-width' | 'fit-height' | 'actual-size' | 'automatic';
	scrollMode: 'paginated' | 'infinite' | 'long-strip';
	backgroundColor: string;
	readingDirection: 'ltr' | 'rtl' | 'webtoon';
	stripMaxWidthPercent: number;
	mangaMode: boolean;
	panelViewEnabled: boolean;
	spreadHandling: 'auto' | 'force-single' | 'force-double' | 'never-split';
	pageTransitionSound: boolean;
	autoHideControls: boolean;
	useStandardFullscreen: boolean;
	vibrance: number;
	saturation: number;
}

export interface AudioReaderSetting {
	playbackSpeed: number;
	skipForward: number;
	skipBackward: number;
	autoAdvance: boolean;
	autoHideControls: boolean;
	gaplessPlayback: boolean;
	sleepTimer: 'off' | '15min' | '30min' | '60min' | 'end-of-chapter' | 'custom';
	sleepTimerCustom: number;
	theme: 'cover-focused' | 'minimal';
	waveformStyle: 'line' | 'bars' | 'filled';
	backgroundStyle: 'cover-blur' | 'solid' | 'none';
	voiceBoost: boolean;
	equalizerLow: number;
	equalizerMid: number;
	equalizerHigh: number;
}

export interface SpeedReaderSetting {
	wpm: number;
	wordSize: number;
	fontFamily: string;
	fontWeight: number;
	focalPoint: number;
	centerWord: boolean;
	accentEnabled: boolean;
	accentColor: string;
	accentOpacity: number;
	focusIndicator: 'off' | 'lines' | 'arrows';
	focusIndicatorDistance: number;
	horizontalBars: boolean;
	horizontalBarsColor: string;
	horizontalBarsOpacity: number;
	verticalIndicator: 'off' | 'line';
	sentencePause: number;
	autoSentencePause: boolean;
	keepScreenOn: boolean;
	theme: string;
	letterSpacing: number;
	focusIndicatorLength: number;
	showWordCount: boolean;
}

export interface ReaderSettings {
	keepScreenOnWhileReading: boolean;
	keepScreenOnWhileAppOpen: boolean;
	readerTheme: string;
	showCurrentSection: boolean;
	settingsUpdatedAt: number;
	epub: EpubReaderSetting;
	pdf: PdfReaderSetting;
	cbx: CbxReaderSetting;
	audio: AudioReaderSetting;
	speedReader: SpeedReaderSetting;
}

export const defaultReaderSettings: ReaderSettings = {
	keepScreenOnWhileReading: true,
	keepScreenOnWhileAppOpen: false,
	readerTheme: DEFAULT_READER_THEME,
	showCurrentSection: true,
	settingsUpdatedAt: 0,
	epub: {
		fontFamily: 'serif',
		fontSize: 18,
		fontWeight: 400,
		fontStyle: 'normal',
		lineHeight: 1.6,
		letterSpacing: 0,
		paragraphSpacing: 0,
		paragraphIndent: 0,
		justify: true,
		hyphenate: false,
		hyphenationLanguage: 'en',
		maxColumnCount: 1,
		gap: 5,
		theme: DEFAULT_READER_THEME,
		isDark: true,
		flow: 'paginated',
		maxInlineSize: 680,
		maxBlockSize: 1440,
		margin: 20,
		continuousMaxWidth: 720,
		brightness: 100,
		contrast: 100,
		pageAnimation: 'slide',
		autoAdvance: false,
		autoAdvanceTimer: 0,
		fullscreenLock: false,
		useStandardFullscreen: false,
		autoHideControls: true,
		customCss: '',
		showTextLayer: true,
		originalLayout: false,
		continuousMode: true,
		showImages: true,
		imageSize: 'fit-width',
		imageGrayscale: false
	},
	pdf: {
		pageSpread: 'off',
		pageLayout: 'single',
		pageZoom: 'auto',
		zoomLevel: 100,
		renderQuality: 'high',
		autoHideControls: true,
		showSidebar: false,
		scrollDirection: 'vertical',
		scrollMode: 'continuous-vertical',
		pageRotation: 0,
		backgroundColor: '#111111',
		brightness: 100,
		contrast: 100,
		grayscale: 0,
		readingDirection: 'ltr',
		autoCropMargins: false,
		textLayerEnabled: true,
		annotationsEnabled: true,
		viewMode: 'dark',
		showChapterMarkers: false,
		showQuoteMarks: false,
		panMode: false,
		useStandardFullscreen: false,
		keepScreenOn: true
	},
	cbx: {
		pageSpread: 'auto',
		pageLayout: 'single',
		fitMode: 'fit-width',
		scrollMode: 'paginated',
		backgroundColor: '#111111',
		readingDirection: 'ltr',
		stripMaxWidthPercent: 100,
		mangaMode: false,
		panelViewEnabled: false,
		spreadHandling: 'auto',
		pageTransitionSound: false,
		autoHideControls: true,
		useStandardFullscreen: false,
		vibrance: 100,
		saturation: 100
	},
	audio: {
		playbackSpeed: 1.0,
		skipForward: 15,
		skipBackward: 15,
		autoAdvance: false,
		autoHideControls: true,
		gaplessPlayback: true,
		sleepTimer: 'off',
		sleepTimerCustom: 30,
		theme: 'cover-focused',
		waveformStyle: 'line',
		backgroundStyle: 'cover-blur',
		voiceBoost: false,
		equalizerLow: 50,
		equalizerMid: 50,
		equalizerHigh: 50
	},
	speedReader: {
		wpm: 300,
		wordSize: 48,
		fontFamily: 'serif',
		fontWeight: 400,
		focalPoint: 0.38,
		centerWord: false,
		accentEnabled: true,
		accentColor: '#ef4444',
		accentOpacity: 1.0,
		focusIndicator: 'lines',
		focusIndicatorDistance: 20,
		horizontalBars: true,
		horizontalBarsColor: '#666666',
		horizontalBarsOpacity: 1.0,
		verticalIndicator: 'off',
		sentencePause: 350,
		autoSentencePause: true,
		keepScreenOn: true,
		theme: DEFAULT_READER_THEME,
		letterSpacing: 0,
		focusIndicatorLength: 20,
		showWordCount: false
	}
};

function loadSettings(): ReaderSettings {
	if (!browser) return defaultReaderSettings;
	
	const stored = localStorage.getItem('readerSettings');
	if (stored) {
		try {
			const settings = migrateSettings(deepMerge(defaultReaderSettings, JSON.parse(stored)));
			localStorage.setItem('readerSettings', JSON.stringify(settings));
			localStorage.setItem(READER_SETTINGS_VERSION_KEY, String(READER_SETTINGS_VERSION));
			return settings;
		} catch {
			return defaultReaderSettings;
		}
	}
	localStorage.setItem(READER_SETTINGS_VERSION_KEY, String(READER_SETTINGS_VERSION));
	return defaultReaderSettings;
}

function migrateSettings(settings: ReaderSettings): ReaderSettings {
	const version = Number(localStorage.getItem(READER_SETTINGS_VERSION_KEY) || '0');
	if (version >= READER_SETTINGS_VERSION) {
		const readerTheme = settings.readerTheme || settings.epub?.theme || settings.speedReader?.theme || DEFAULT_READER_THEME;
		return {
			...settings,
			readerTheme,
			showCurrentSection: settings.showCurrentSection ?? true,
			settingsUpdatedAt: settings.settingsUpdatedAt ?? 0,
			epub: { ...settings.epub, theme: readerTheme },
			speedReader: { ...settings.speedReader, theme: readerTheme }
		};
	}

	const readerTheme = settings.readerTheme || settings.epub?.theme || settings.speedReader?.theme || DEFAULT_READER_THEME;
	return {
		...settings,
		keepScreenOnWhileReading: settings.keepScreenOnWhileReading ?? true,
		keepScreenOnWhileAppOpen: settings.keepScreenOnWhileAppOpen ?? false,
		readerTheme,
		showCurrentSection: settings.showCurrentSection ?? true,
		settingsUpdatedAt: settings.settingsUpdatedAt ?? 0,
		epub: {
			...settings.epub,
			theme: readerTheme,
			flow: 'scrolled',
			continuousMode: true
		},
		pdf: {
			...settings.pdf,
			scrollMode: settings.pdf.scrollMode === 'paged' ? 'continuous-vertical' : settings.pdf.scrollMode
		},
		speedReader: {
			...settings.speedReader,
			theme: readerTheme,
			fontWeight: settings.speedReader.fontWeight ?? 400
		}
	};
}

function deepMerge<T extends Record<string, any>>(target: T, source: Partial<T>): T {
	const result = { ...target };
	for (const key in source) {
		if (source[key] && typeof source[key] === 'object' && !Array.isArray(source[key])) {
			result[key] = { ...target[key], ...source[key] };
		} else if (source[key] !== undefined) {
			result[key] = source[key] as T[Extract<keyof T, string>];
		}
	}
	return result;
}

function createReaderSettingsStore() {
	const { subscribe, set, update } = writable<ReaderSettings>(loadSettings());
	let saveTimer: ReturnType<typeof setTimeout> | null = null;
	let pendingBackendSave: ReaderSettings | null = null;

	function normalizeSettings(value: ReaderSettings): ReaderSettings {
		const readerTheme = value.readerTheme || value.epub?.theme || value.speedReader?.theme || DEFAULT_READER_THEME;
		return {
			...value,
			readerTheme,
			showCurrentSection: value.showCurrentSection ?? true,
			settingsUpdatedAt: value.settingsUpdatedAt ?? 0,
			epub: { ...value.epub, theme: readerTheme },
			speedReader: { ...value.speedReader, theme: readerTheme }
		};
	}

	function touchSettings(value: ReaderSettings): ReaderSettings {
		return normalizeSettings({
			...value,
			settingsUpdatedAt: Date.now()
		});
	}

	function persistLocal(value: ReaderSettings) {
		if (browser) {
			localStorage.setItem('readerSettings', JSON.stringify(value));
			localStorage.setItem(READER_SETTINGS_VERSION_KEY, String(READER_SETTINGS_VERSION));
		}
	}

	async function saveSettingsToBackend(value: ReaderSettings, keepalive = false) {
		const res = await fetch('/api/settings/reader', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ reader: value }),
			keepalive
		});
		if (!res.ok) {
			throw new Error(`Failed to save reader settings: ${res.status}`);
		}
	}

	function scheduleSaveToBackend(value: ReaderSettings) {
		if (!browser) return;
		pendingBackendSave = value;
		if (saveTimer) clearTimeout(saveTimer);
		saveTimer = setTimeout(async () => {
			saveTimer = null;
			const valueToSave = pendingBackendSave;
			pendingBackendSave = null;
			if (!valueToSave) return;
			try {
				await saveSettingsToBackend(valueToSave);
			} catch (e) {
				console.error('Failed to save reader settings:', e);
			}
		}, 700);
	}

	return {
		subscribe,
		set: (value: ReaderSettings) => {
			const newValue = touchSettings(value);
			persistLocal(newValue);
			set(newValue);
			scheduleSaveToBackend(newValue);
		},
		update: (fn: (value: ReaderSettings) => ReaderSettings) => {
			update(value => {
				const newValue = touchSettings(fn(value));
				persistLocal(newValue);
				scheduleSaveToBackend(newValue);
				return newValue;
			});
		},
		updateEpub: (epubSettings: Partial<EpubReaderSetting>) => {
			update(s => {
				const nextEpub = { ...s.epub, ...epubSettings };
				const newSettings = touchSettings({
					...s, 
					epub: nextEpub,
					readerTheme: epubSettings.theme ?? s.readerTheme
				});
				persistLocal(newSettings);
				scheduleSaveToBackend(newSettings);
				return newSettings;
			});
		},
		updatePdf: (pdfSettings: Partial<PdfReaderSetting>) => {
			update(s => {
				const newSettings = touchSettings({ 
					...s, 
					pdf: { ...s.pdf, ...pdfSettings } 
				});
				persistLocal(newSettings);
				scheduleSaveToBackend(newSettings);
				return newSettings;
			});
		},
		updateCbx: (cbxSettings: Partial<CbxReaderSetting>) => {
			update(s => {
				const newSettings = touchSettings({ 
					...s, 
					cbx: { ...s.cbx, ...cbxSettings } 
				});
				persistLocal(newSettings);
				scheduleSaveToBackend(newSettings);
				return newSettings;
			});
		},
		updateAudio: (audioSettings: Partial<AudioReaderSetting>) => {
			update(s => {
				const newSettings = touchSettings({ 
					...s, 
					audio: { ...s.audio, ...audioSettings } 
				});
				persistLocal(newSettings);
				scheduleSaveToBackend(newSettings);
				return newSettings;
			});
		},
		updateReaderTheme: (theme: string) => {
			update(s => {
				const newSettings = touchSettings({
					...s,
					readerTheme: theme,
					epub: { ...s.epub, theme },
					speedReader: { ...s.speedReader, theme }
				});
				persistLocal(newSettings);
				scheduleSaveToBackend(newSettings);
				return newSettings;
			});
		},
		syncWithBackend: async () => {
			if (!browser) return;
			try {
				const res = await fetch('/api/settings', { cache: 'no-store' });
				if (res.ok) {
					const data = await res.json();
					if (data.reader) {
						const merged = normalizeSettings(migrateSettings(deepMerge(defaultReaderSettings, data.reader)));
						const local = normalizeSettings(loadSettings());
						if ((merged.settingsUpdatedAt || 0) >= (local.settingsUpdatedAt || 0)) {
							persistLocal(merged);
							set(merged);
						} else {
							persistLocal(local);
							set(local);
							scheduleSaveToBackend(local);
						}
					}
				}
			} catch (e) {
				console.error('Failed to sync reader settings with backend:', e);
			}
		},
		saveToBackend: async () => {
			try {
				const current = normalizeSettings(loadSettings());
				await saveSettingsToBackend(current);
				pendingBackendSave = null;
				return true;
			} catch (e) {
				console.error('Failed to save reader settings:', e);
				return false;
			}
		},
		flushPendingSave: async (keepalive = false) => {
			if (!browser) return false;
			if (saveTimer) {
				clearTimeout(saveTimer);
				saveTimer = null;
			}
			const current = normalizeSettings(pendingBackendSave ?? loadSettings());
			pendingBackendSave = null;
			try {
				await saveSettingsToBackend(current, keepalive);
				return true;
			} catch (e) {
				console.error('Failed to flush reader settings:', e);
				return false;
			}
		},
		resetToDefaults: (readerType?: 'epub' | 'pdf' | 'cbx' | 'audio' | 'speedReader') => {
			update(s => {
				let next: ReaderSettings;
				if (readerType) {
					next = {
						...s,
						[readerType]: { ...defaultReaderSettings[readerType] }
					};
				} else {
					next = {
						keepScreenOnWhileReading: defaultReaderSettings.keepScreenOnWhileReading,
						keepScreenOnWhileAppOpen: defaultReaderSettings.keepScreenOnWhileAppOpen,
						readerTheme: defaultReaderSettings.readerTheme,
						showCurrentSection: defaultReaderSettings.showCurrentSection,
						settingsUpdatedAt: defaultReaderSettings.settingsUpdatedAt,
						epub: { ...defaultReaderSettings.epub },
						pdf: { ...defaultReaderSettings.pdf },
						cbx: { ...defaultReaderSettings.cbx },
						audio: { ...defaultReaderSettings.audio },
						speedReader: { ...defaultReaderSettings.speedReader }
					};
				}
				const newSettings = touchSettings(next);
				persistLocal(newSettings);
				scheduleSaveToBackend(newSettings);
				return newSettings;
			});
		},
		updateSpeedReader: (speedReaderSettings: Partial<SpeedReaderSetting>) => {
			update(s => {
				const nextSpeedReader = { ...s.speedReader, ...speedReaderSettings };
				const newSettings = touchSettings({
					...s,
					speedReader: nextSpeedReader,
					readerTheme: speedReaderSettings.theme ?? s.readerTheme
				});
				persistLocal(newSettings);
				scheduleSaveToBackend(newSettings);
				return newSettings;
			});
		}
	};
}

export const readerSettings = createReaderSettingsStore();

export const epubThemes = [
	{ id: 'catppuccin', name: 'Catppuccin', bg: '#1e1e2e', text: '#cdd6f4' },
	{ id: 'light', name: 'Light', bg: '#ffffff', text: '#333333' },
	{ id: 'sepia', name: 'Sepia', bg: '#f4ecd8', text: '#5b4636' },
	{ id: 'dark', name: 'Dark', bg: '#111111', text: '#e5e7eb' },
	{ id: 'amoled', name: 'AMOLED', bg: '#000000', text: '#ffffff' }
];

export const speedReaderThemes = [
	{ id: 'catppuccin', name: 'Catppuccin', bg: '#1e1e2e', text: '#cdd6f4' },
	{ id: 'light', name: 'Light', bg: '#ffffff', text: '#333333' },
	{ id: 'sepia', name: 'Sepia', bg: '#f4ecd8', text: '#5b4636' },
	{ id: 'dark', name: 'Dark', bg: '#111111', text: '#e5e7eb' },
	{ id: 'black', name: 'Black', bg: '#000000', text: '#ffffff' }
];

export const fontFamilies = [
	{ id: 'serif', name: 'Serif', family: 'Georgia, "Times New Roman", serif' },
	{ id: 'spectral', name: 'Spectral', family: '"Spectral", Georgia, serif', weights: [200, 300, 400, 500, 600, 700, 800] },
	{ id: 'sans-serif', name: 'Sans Serif', family: 'system-ui, -apple-system, sans-serif' },
	{ id: 'roboto', name: 'Roboto', family: '"Roboto", sans-serif' },
	{ id: 'open-dyslexic', name: 'Open Dyslexic', family: '"OpenDyslexic", sans-serif' },
	{ id: 'literata', name: 'Literata', family: 'Literata, serif' },
	{ id: 'atkinson', name: 'Atkinson Hyperlegible', family: '"Atkinson Hyperlegible", sans-serif' },
	{ id: 'cursive', name: 'Cursive', family: 'cursive' },
	{ id: 'monospace', name: 'Monospace', family: '"Courier New", Courier, monospace' }
];

export function resolveFontFamily(fontId: string): string {
	const font = fontFamilies.find(f => f.id === fontId);
	return font ? font.family : (fontId || 'Georgia, "Times New Roman", serif');
}

export const fontWeightOptions = [
	{ value: 200, label: 'ExtraLight' },
	{ value: 300, label: 'Light' },
	{ value: 400, label: 'Regular' },
	{ value: 500, label: 'Medium' },
	{ value: 600, label: 'SemiBold' },
	{ value: 700, label: 'Bold' },
	{ value: 800, label: 'ExtraBold' }
];

export const cbxFitModes = [
	{ id: 'fit-page', name: 'Fit Page' },
	{ id: 'fit-width', name: 'Fit Width' },
	{ id: 'fit-height', name: 'Fit Height' },
	{ id: 'actual-size', name: 'Actual Size' },
	{ id: 'automatic', name: 'Automatic' }
];

export const cbxScrollModes = [
	{ id: 'paginated', name: 'Paginated' },
	{ id: 'infinite', name: 'Infinite Scroll' },
	{ id: 'long-strip', name: 'Long Strip' }
];

export const pdfZoomModes = [
	{ id: 'auto', name: 'Auto Zoom' },
	{ id: 'page-fit', name: 'Page Fit' },
	{ id: 'page-width', name: 'Page Width' },
	{ id: 'actual-size', name: 'Actual Size' }
];

export const pdfBackgroundColors = [
	{ id: 'black', name: 'Black', color: '#000000' },
	{ id: 'dark-gray', name: 'Dark Gray', color: '#1a1a1a' },
	{ id: 'white', name: 'White', color: '#ffffff' }
];

export const skipIntervalOptions = [
	{ value: 10, label: '10s' },
	{ value: 15, label: '15s' },
	{ value: 30, label: '30s' },
	{ value: 60, label: '60s' }
];

export const sleepTimerOptions = [
	{ value: 'off', label: 'Off' },
	{ value: '15min', label: '15 min' },
	{ value: '30min', label: '30 min' },
	{ value: '60min', label: '60 min' },
	{ value: 'end-of-chapter', label: 'End of Chapter' },
	{ value: 'custom', label: 'Custom' }
];

export const waveformStyles = [
	{ id: 'line', name: 'Line' },
	{ id: 'bars', name: 'Bars' },
	{ id: 'filled', name: 'Filled' }
];

export const supportedReaderFormats = {
	epub: ['epub', 'mobi', 'azw', 'azw3', 'cbz', 'cbr', 'cbt'],
	pdf: ['pdf'],
	cbx: ['cbz', 'cbr', 'cb7', 'cbt'],
	audio: ['mp3', 'm4b', 'm4a', 'opus', 'ogg', 'aac']
};

export function getReaderTypeForFormat(format: string): 'epub' | 'pdf' | 'cbx' | 'audio' | 'epub' {
	const f = format.toLowerCase();
	if (['mp3', 'm4b', 'm4a', 'opus', 'ogg', 'aac'].includes(f)) return 'audio';
	if (['pdf'].includes(f)) return 'pdf';
	if (['cbz', 'cbr', 'cb7', 'cbt'].includes(f)) return 'cbx';
	return 'epub';
}

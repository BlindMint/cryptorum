<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { readerSettings, epubThemes, fontFamilies, pdfZoomModes, cbxFitModes, cbxScrollModes, skipIntervalOptions, sleepTimerOptions, waveformStyles, type ReaderSettings } from '$lib/stores/readerSettings';
	import { currentTheme, primaryColors, surfaceColors, addCustomTheme, updateCustomTheme, removeCustomTheme, resetPrimaryToDefault, resetSurfaceToDefault, generateId, updateGlowEnabled, updateGlowAutoMode, updateGlowColor, updateGlowIntensity, updateBgImageEnabled, updateBgImageTransparency, updateBgImageDisplay, updateSelectedBgImage, addBackgroundImage, removeBackgroundImage, DEFAULT_THEME_PRIMARY, DEFAULT_THEME_SURFACE } from '$lib/stores/theme';
	import type { BackgroundImageDisplay } from '$lib/stores/theme';
import ThemePreviewSwatch from '$lib/components/ThemePreviewSwatch.svelte';
import AdminPanel from './AdminPanel.svelte';
import MetadataManagerContent from '$lib/components/MetadataManagerContent.svelte';
import UsersPanel from './UsersPanel.svelte';
import LibraryIconPicker from '$lib/components/LibraryIconPicker.svelte';
import { parseLibraryIcon } from '$lib/utils/library-icons';
	
	let settings = $state<any>({
		libraries: [],
		bookdrop: null,
		metadata: {
			providers: [],
			auto_fetch_on_import: false
		}
	});
	let localReaderSettings = $state<ReaderSettings>({
		epub: { fontFamily: 'serif', fontSize: 18, fontWeight: 400, fontStyle: 'normal' as const, lineHeight: 1.6, letterSpacing: 0, paragraphSpacing: 0, paragraphIndent: 0, justify: true, hyphenate: false, hyphenationLanguage: 'en', maxColumnCount: 1, gap: 5, theme: 'dark', isDark: true, flow: 'paginated' as const, maxInlineSize: 680, maxBlockSize: 1440, margin: 5, continuousMaxWidth: 720, brightness: 100, contrast: 100, pageAnimation: 'slide' as const, autoAdvance: false, autoAdvanceTimer: 0, fullscreenLock: false, useStandardFullscreen: false, autoHideControls: true, customCss: '', showTextLayer: true, originalLayout: false, continuousMode: true, showImages: true, imageSize: 'fit-width' as const, imageGrayscale: false },
		pdf: { pageSpread: 'off' as const, pageLayout: 'single' as const, pageZoom: 'auto', zoomLevel: 100, renderQuality: 'high' as const, autoHideControls: true, showSidebar: false, scrollDirection: 'vertical' as const, scrollMode: 'paged' as const, pageRotation: 0 as const, backgroundColor: '#111111', brightness: 100, contrast: 100, grayscale: 0, readingDirection: 'ltr' as const, autoCropMargins: false, textLayerEnabled: true, annotationsEnabled: true, viewMode: 'dark' as const, showChapterMarkers: false, showQuoteMarks: false, panMode: false, useStandardFullscreen: false },
		cbx: { pageSpread: 'auto' as const, pageLayout: 'single' as const, fitMode: 'fit-width' as const, scrollMode: 'paginated' as const, backgroundColor: '#111111', readingDirection: 'ltr' as const, stripMaxWidthPercent: 100, mangaMode: false, panelViewEnabled: false, spreadHandling: 'auto' as const, pageTransitionSound: false, autoHideControls: true, useStandardFullscreen: false, vibrance: 100, saturation: 100 },
		audio: { playbackSpeed: 1.0, skipForward: 15, skipBackward: 15, autoAdvance: false, autoHideControls: true, gaplessPlayback: true, sleepTimer: 'off' as const, sleepTimerCustom: 30, theme: 'cover-focused' as const, waveformStyle: 'line' as const, backgroundStyle: 'cover-blur' as const, voiceBoost: false, equalizerLow: 50, equalizerMid: 50, equalizerHigh: 50 },
		speedReader: { wpm: 300, wordSize: 48, fontFamily: 'serif', focalPoint: 0.38, centerWord: false, accentEnabled: true, accentColor: '#ef4444', accentOpacity: 1.0, focusIndicator: 'lines' as const, focusIndicatorDistance: 20, horizontalBars: true, horizontalBarsColor: '#666666', horizontalBarsOpacity: 1.0, verticalIndicator: 'off' as const, sentencePause: 350, autoSentencePause: true, keepScreenOn: true, theme: 'dark', letterSpacing: 0, focusIndicatorLength: 20 }
	});
	let loading = $state(true);
	let scanning = $state(false);
	let deletingLibraryId = $state<number | null>(null);
	let scanPollTimer: number | null = null;
	let activeLibraryScanJobs = $state<any[]>([]);
	let activeTab = $state<'general' | 'metadata' | 'reader' | 'appearance' | 'users' | 'admin'>('general');
	let settingsSaved = $state(false);
	let bookCoverSettings = $state({
		preserve_full_cover: true,
		vertical_cropping: true,
		horizontal_cropping: true,
		aspect_ratio_threshold: 2.5,
		smart_cropping: true
	});
	let coverSettingsSaved = $state(false);
	let coverSettingsSaving = $state(false);
	let coverRegenerating = $state(false);
	let coverActionMessage = $state('');

	let showLibraryModal = $state(false);
	let showLibraryIconPicker = $state(false);
	let editingLibrary = $state<any>(null);
	let showDirectoryModal = $state(false);
	let currentDirectory = $state('/');
	let directoryContents = $state<any[]>([]);
	let directoryTarget = $state<'library' | 'bookdrop'>('library');
	let libraryForm = $state({
		name: '',
		icon: '',
		paths: ['']
	});
	let currentLibraryIcon = $derived(parseLibraryIcon(libraryForm.icon));
	let anyLibraryScanning = $derived(
		!!settings.libraries?.some((library: any) => library.is_importing)
	);
	let scanActive = $derived(scanning || anyLibraryScanning || activeLibraryScanJobs.length > 0);

	let themeState = $state<{ primary: string; surface: string; appearance: { glowEnabled: boolean; glowAutoMode: boolean; glowColor: string; glowIntensity: number; bgImageEnabled: boolean; bgImageTransparency: number; bgImageDisplay: BackgroundImageDisplay; backgroundImages: string[]; selectedBgImageIndex: number; customThemes: any[]; selectedCustomThemeId: string | null } }>({ primary: DEFAULT_THEME_PRIMARY, surface: DEFAULT_THEME_SURFACE, appearance: { glowEnabled: true, glowAutoMode: true, glowColor: '#f97316', glowIntensity: 10, bgImageEnabled: false, bgImageTransparency: 50, bgImageDisplay: 'fill', backgroundImages: [], selectedBgImageIndex: 0, customThemes: [], selectedCustomThemeId: null } });
	const bgImageDisplayOptions: Array<{ id: BackgroundImageDisplay; label: string; description: string }> = [
		{ id: 'fill', label: 'Fill', description: 'Cover the whole screen' },
		{ id: 'fit', label: 'Fit', description: 'Show the whole image' },
		{ id: 'center', label: 'Center', description: 'Natural size, centered' },
		{ id: 'stretch', label: 'Stretch', description: 'Stretch to viewport' },
		{ id: 'tile', label: 'Tile', description: 'Repeat the image' }
	];
	let showCustomThemeEditor = $state(false);
	let editingCustomTheme = $state<any>(null);
	let customThemeName = $state('');
	let customThemeFg = $state('#ffffff');
	let customThemeFgHexInput = $state('#ffffff');
	let customThemeFgRgb = $state('rgb(255, 255, 255)');
	let customThemeBg = $state('#111111');
	let customThemeBgHexInput = $state('#111111');
	let customThemeBgRgb = $state('rgb(17, 17, 17)');

	function normalizeHexColor(value: string): string | null {
		const trimmed = value.trim();
		const match = /^#?([a-f\d]{3}|[a-f\d]{6})$/i.exec(trimmed);
		if (!match) return null;
		let hex = match[1];
		if (hex.length === 3) {
			hex = hex.split('').map(char => char + char).join('');
		}
		return `#${hex.toLowerCase()}`;
	}

	function clampColorChannel(value: number): number {
		return Math.max(0, Math.min(255, Math.round(value)));
	}

	function hexToRgbComponents(hex: string): { r: number; g: number; b: number } | null {
		const normalized = normalizeHexColor(hex);
		if (!normalized) return null;
		const parts = normalized.slice(1).match(/.{2}/g);
		if (!parts || parts.length !== 3) return null;
		return {
			r: parseInt(parts[0], 16),
			g: parseInt(parts[1], 16),
			b: parseInt(parts[2], 16)
		};
	}

	function rgbComponentsToHex(r: number, g: number, b: number): string {
		return `#${[r, g, b].map(value => clampColorChannel(value).toString(16).padStart(2, '0')).join('')}`;
	}

	function rgbStringToHex(value: string): string | null {
		const trimmed = value.trim();
		const match = trimmed.match(/^rgb\s*\(\s*(\d{1,3})\s*[, ]\s*(\d{1,3})\s*[, ]\s*(\d{1,3})\s*\)$/i)
			?? trimmed.match(/^(\d{1,3})\s*[, ]\s*(\d{1,3})\s*[, ]\s*(\d{1,3})$/);
		if (!match) return null;
		const r = Number(match[1]);
		const g = Number(match[2]);
		const b = Number(match[3]);
		if ([r, g, b].some(value => Number.isNaN(value) || value < 0 || value > 255)) return null;
		return rgbComponentsToHex(r, g, b);
	}

	function rgbComponentsToString(hex: string): string {
		const rgb = hexToRgbComponents(hex);
		if (!rgb) return 'rgb(0, 0, 0)';
		return `rgb(${rgb.r}, ${rgb.g}, ${rgb.b})`;
	}

	function parseColorInput(value: string): string | null {
		return normalizeHexColor(value) ?? rgbStringToHex(value);
	}

	function resolvePreviewColor(value: string, fallback: string): string {
		return parseColorInput(value) ?? fallback;
	}

	function isValidThemeColor(value: string): boolean {
		return parseColorInput(value) !== null;
	}

	function setForegroundColorFromHex(value: string) {
		customThemeFgHexInput = value;
		const normalized = normalizeHexColor(value);
		if (normalized) {
			customThemeFg = normalized;
			customThemeFgHexInput = normalized;
			customThemeFgRgb = rgbComponentsToString(normalized);
		}
	}

	function setForegroundColorFromRgb(value: string) {
		customThemeFgRgb = value;
		const normalized = rgbStringToHex(value);
		if (normalized) {
			customThemeFg = normalized;
			customThemeFgHexInput = normalized;
			customThemeFgRgb = rgbComponentsToString(normalized);
		}
	}

	function setBackgroundColorFromHex(value: string) {
		customThemeBgHexInput = value;
		const normalized = normalizeHexColor(value);
		if (normalized) {
			customThemeBg = normalized;
			customThemeBgHexInput = normalized;
			customThemeBgRgb = rgbComponentsToString(normalized);
		}
	}

	function setBackgroundColorFromRgb(value: string) {
		customThemeBgRgb = value;
		const normalized = rgbStringToHex(value);
		if (normalized) {
			customThemeBg = normalized;
			customThemeBgHexInput = normalized;
			customThemeBgRgb = rgbComponentsToString(normalized);
		}
	}

	let showRemoveBgModal = $state(false);
	let bgToRemove = $state<number | null>(null);

	let showBookdropModal = $state(false);
	let bookdropPath = $state('');

	$effect(() => {
		readerSettings.subscribe(s => {
			localReaderSettings = JSON.parse(JSON.stringify(s));
		});
	});

	$effect(() => {
		currentTheme.subscribe(t => {
			themeState = { ...t };
		});
	});

	function handleFileUpload(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files && input.files[0]) {
			const file = input.files[0];
			const reader = new FileReader();
			reader.onload = (e) => {
				if (e.target?.result) {
					addBackgroundImage(e.target.result as string);
				}
			};
			reader.readAsDataURL(file);
		}
	}

	function confirmRemoveBg(index: number) {
		bgToRemove = index;
		showRemoveBgModal = true;
	}

	function doRemoveBg() {
		if (bgToRemove !== null) {
			removeBackgroundImage(bgToRemove);
			bgToRemove = null;
		}
		showRemoveBgModal = false;
	}

	function openAddCustomTheme() {
		editingCustomTheme = null;
		customThemeName = '';
		customThemeFg = '#ffffff';
		customThemeFgHexInput = '#ffffff';
		customThemeFgRgb = 'rgb(255, 255, 255)';
		customThemeBg = '#111111';
		customThemeBgHexInput = '#111111';
		customThemeBgRgb = 'rgb(17, 17, 17)';
		showCustomThemeEditor = true;
	}

	function openEditCustomTheme(theme: any) {
		editingCustomTheme = theme;
		customThemeName = theme.name;
		customThemeFg = theme.foreground;
		customThemeFgHexInput = theme.foreground;
		customThemeFgRgb = rgbComponentsToString(theme.foreground);
		customThemeBg = theme.background;
		customThemeBgHexInput = theme.background;
		customThemeBgRgb = rgbComponentsToString(theme.background);
		showCustomThemeEditor = true;
	}

	function saveCustomTheme() {
		if (!customThemeName.trim()) return;
		const foreground = parseColorInput(customThemeFg);
		const background = parseColorInput(customThemeBg);
		if (!foreground || !background) return;
		if (editingCustomTheme) {
			updateCustomTheme(editingCustomTheme.id, {
				name: customThemeName.trim(),
				foreground,
				background
			});
		} else {
			addCustomTheme({
				id: generateId(),
				name: customThemeName.trim(),
				foreground,
				background
			});
		}
		showCustomThemeEditor = false;
	}

	function deleteCustomTheme(themeId: string) {
		removeCustomTheme(themeId);
	}

	let canSaveCustomTheme = $derived(
		customThemeName.trim().length > 0 &&
		isValidThemeColor(customThemeFgHexInput) &&
		isValidThemeColor(customThemeFgRgb) &&
		isValidThemeColor(customThemeBgHexInput) &&
		isValidThemeColor(customThemeBgRgb)
	);

	async function saveBookdropLocation() {
		const path = bookdropPath.trim();
		if (!path) return;
		try {
			const response = await fetch('/api/bookdrop', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ path })
			});
			if (response.ok) {
				const data = await response.json();
				settings = { ...settings, bookdrop: data.bookdrop };
				showBookdropModal = false;
			}
		} catch (e) {
			console.error('Failed to save bookdrop path:', e);
		}
	}

  	function openLibraryModal(library: any = null) {
  		editingLibrary = library;
  		if (library) {
  			libraryForm = {
  				name: library.name,
  				icon: library.icon || '',
  				paths: library.paths.filter((p: string) => p.trim())
  			};
  		} else {
  			libraryForm = {
  				name: '',
  				icon: '',
  				paths: ['']
  			};
  		}
  		showLibraryModal = true;
  	}

	function closeLibraryModal() {
		showLibraryModal = false;
		showLibraryIconPicker = false;
		editingLibrary = null;
	}

	function openLibraryIconPicker() {
		showLibraryIconPicker = true;
	}

	function closeLibraryIconPicker() {
		showLibraryIconPicker = false;
	}

	function selectLibraryIcon(iconValue: string) {
		libraryForm.icon = iconValue;
	}

	function clearLibraryIcon() {
		libraryForm.icon = '';
	}

	$effect(() => {
		currentLibraryIcon = parseLibraryIcon(libraryForm.icon);
	});

	async function saveReaderSettings() {
		readerSettings.set(localReaderSettings);
		const success = await readerSettings.saveToBackend();
		if (success) {
			settingsSaved = true;
			setTimeout(() => settingsSaved = false, 2000);
		}
	}

	function updateEpubSetting(key: string, value: any) {
		localReaderSettings.epub = { ...localReaderSettings.epub, [key]: value };
	}

	function updatePdfSetting(key: string, value: any) {
		localReaderSettings.pdf = { ...localReaderSettings.pdf, [key]: value };
	}

	function updateCbxSetting(key: string, value: any) {
		localReaderSettings.cbx = { ...localReaderSettings.cbx, [key]: value };
	}

	function updateAudioSetting(key: string, value: any) {
		localReaderSettings.audio = { ...localReaderSettings.audio, [key]: value };
	}

	function updateSpeedReaderSetting(key: string, value: any) {
		localReaderSettings.speedReader = { ...localReaderSettings.speedReader, [key]: value };
	}

	function updateBookCoverSetting(key: string, value: any) {
		bookCoverSettings = { ...bookCoverSettings, [key]: value };
	}

	async function saveBookCoverSettings() {
		coverSettingsSaving = true;
		coverActionMessage = '';
		try {
			const response = await fetch('/api/settings/book-covers', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(bookCoverSettings)
			});
			if (response.ok) {
				const data = await response.json();
				bookCoverSettings = data;
				settings = { ...settings, book_covers: data };
				coverSettingsSaved = true;
				setTimeout(() => coverSettingsSaved = false, 2000);
			}
		} catch (e) {
			console.error('Failed to save book cover settings:', e);
		} finally {
			coverSettingsSaving = false;
		}
	}

	async function regenerateBookCovers(mode: 'all' | 'missing') {
		coverRegenerating = true;
		coverActionMessage = '';
		try {
			const response = await fetch('/api/settings/book-covers/regenerate', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ mode, settings: bookCoverSettings })
			});
			if (response.ok) {
				coverActionMessage = mode === 'all'
					? 'Cover regeneration job queued for all books.'
					: 'Cover regeneration job queued for books missing covers.';
			}
		} catch (e) {
			console.error('Failed to start cover regeneration:', e);
		} finally {
			coverRegenerating = false;
			setTimeout(() => coverActionMessage = '', 4000);
		}
	}

	function removePath(index: number) {
		libraryForm.paths = libraryForm.paths.filter((_, i) => i !== index);
	}

	async function openDirectoryModal(target: 'library' | 'bookdrop' = 'library') {
		directoryTarget = target;
		showDirectoryModal = true;
		try {
			const response = await fetch('/api/directories?path=/books');
			if (response.ok) {
				loadDirectoryContents('/books');
			} else {
				loadDirectoryContents('/');
			}
		} catch (e) {
			loadDirectoryContents('/');
		}
	}

	function closeDirectoryModal() {
		showDirectoryModal = false;
	}

	async function loadDirectoryContents(path: string) {
		currentDirectory = path;
		try {
			const response = await fetch(`/api/directories?path=${encodeURIComponent(path)}`);
			if (response.ok) {
				directoryContents = await response.json();
			} else {
				directoryContents = [];
			}
		} catch (e) {
			directoryContents = [];
		}
	}

	function selectDirectory(item: any) {
		if (item.type === 'directory') {
			loadDirectoryContents(item.path);
		}
	}

	function addSelectedDirectory() {
		const newPath = currentDirectory;
		if (directoryTarget === 'bookdrop') {
			bookdropPath = newPath;
		} else if (!libraryForm.paths.includes(newPath)) {
			libraryForm.paths = [...libraryForm.paths, newPath];
		}
		closeDirectoryModal();
	}

	async function saveLibrary() {
		if (!libraryForm.name.trim()) return;

		const filteredPaths = libraryForm.paths.filter(p => p.trim());
		if (filteredPaths.length === 0) return;

		const data = {
			name: libraryForm.name.trim(),
			icon: libraryForm.icon.trim(),
			paths: filteredPaths
		};

		try {
			let response;
			if (editingLibrary) {
				response = await fetch(`/api/libraries/${editingLibrary.id}`, {
					method: 'PUT',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(data)
				});
			} else {
				response = await fetch('/api/libraries', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(data)
				});
			}

			if (response.ok) {
				await loadSettings();
				if ((window as any).refreshSidebar) {
					(window as any).refreshSidebar();
				}
				closeLibraryModal();
			}
		} catch (e) {
			console.error('Failed to save library:', e);
		}
	}

	async function deleteLibrary(library: any) {
		if (!confirm(`Delete library "${library.name}" and all its books? This cannot be undone.`)) return;

		deletingLibraryId = library.id;
		try {
			const response = await fetch(`/api/libraries/${library.id}`, { method: 'DELETE' });
			if (response.ok) {
				await loadSettings();
				if ((window as any).refreshSidebar) {
					(window as any).refreshSidebar();
				}
			}
		} catch (e) {
			console.error('Failed to delete library:', e);
		} finally {
			deletingLibraryId = null;
		}
	}

	async function loadSettings() {
		try {
			const res = await fetch('/api/settings', { cache: 'no-store' });
			if (!res.ok) {
				throw new Error(`Unable to load settings: ${res.status}`);
			}
			const data = await res.json();
			settings = data;
			bookdropPath = settings.bookdrop?.path || '';
			bookCoverSettings = {
				preserve_full_cover: data.book_covers?.preserve_full_cover ?? true,
				vertical_cropping: data.book_covers?.vertical_cropping ?? true,
				horizontal_cropping: data.book_covers?.horizontal_cropping ?? true,
				aspect_ratio_threshold: data.book_covers?.aspect_ratio_threshold ?? 2.5,
				smart_cropping: data.book_covers?.smart_cropping ?? true
			};
			return data;
		} catch (e) {
			console.error('Failed to fetch settings:', e);
			return null;
		}
	}

	function getJobLibraryId(job: any): number | null {
		const payload = job?.payload;
		if (!payload) return null;
		if (typeof payload.library_id === 'number') return payload.library_id;
		if (typeof payload.library_id === 'string') {
			const parsed = Number.parseInt(payload.library_id, 10);
			return Number.isNaN(parsed) ? null : parsed;
		}
		return null;
	}

	function getLibraryScanJob(libraryId: number) {
		return activeLibraryScanJobs.find((job: any) => getJobLibraryId(job) === libraryId);
	}

	function isLibraryScanQueued(library: any): boolean {
		return getLibraryScanJob(library.id)?.status === 'queued';
	}

	function isLibraryScanRunning(library: any): boolean {
		const job = getLibraryScanJob(library.id);
		return library.is_importing || job?.status === 'running';
	}

	function isLibraryScanActive(library: any): boolean {
		return isLibraryScanQueued(library) || isLibraryScanRunning(library);
	}

	async function refreshScanState() {
		const [data, activeJobs] = await Promise.all([loadSettings(), loadActiveLibraryScanJobs()]);
		const libraryScanning = !!data?.libraries?.some((library: any) => library.is_importing);
		scanning = libraryScanning || activeJobs.length > 0;
		if (scanning) {
			startScanPolling();
		} else {
			stopScanPolling();
		}
	}

	async function loadActiveLibraryScanJobs() {
		try {
			const res = await fetch('/api/notifications?unread=true&limit=100', { cache: 'no-store' });
			if (!res.ok) {
				activeLibraryScanJobs = [];
				return [];
			}
			const data = await res.json();
			const jobs = (data.items ?? [])
				.filter((item: any) =>
					item.source === 'job' &&
					item.job?.job_type === 'library_scan' &&
					['queued', 'running'].includes(item.job.status)
				)
				.map((item: any) => item.job);
			activeLibraryScanJobs = jobs;
			return jobs;
		} catch (e) {
			console.error('Failed to load active scan jobs:', e);
			activeLibraryScanJobs = [];
			return [];
		}
	}

	async function scanLibrary(library: any) {
		try {
			const response = await fetch(`/api/libraries/${library.id}/scan`, { method: 'POST' });
			if (response.ok) {
				if ((window as any).refreshSidebar) {
					(window as any).refreshSidebar();
				}
				if ((window as any).refreshScanStatus) {
					(window as any).refreshScanStatus();
				}
				await Promise.all([loadSettings(), loadActiveLibraryScanJobs()]);
				scanning = true;
				startScanPolling();
			}
		} catch (e) {
			console.error('Failed to scan library:', e);
		}
	}

	function startScanPolling() {
		if (scanPollTimer !== null) return;
		scanPollTimer = window.setInterval(async () => {
			if ((window as any).refreshSidebar) {
				(window as any).refreshSidebar();
			}
			if ((window as any).refreshScanStatus) {
				(window as any).refreshScanStatus();
			}
			const [data, activeJobs] = await Promise.all([loadSettings(), loadActiveLibraryScanJobs()]);
			const libraryScanning = !!data?.libraries?.some((library: any) => library.is_importing);
			const stillActive = libraryScanning || activeJobs.length > 0;
			scanning = stillActive;
			if (!stillActive) {
				stopScanPolling();
				await loadSettings();
				if ((window as any).refreshSidebar) {
					(window as any).refreshSidebar();
				}
			}
		}, 3000);
	}

	function stopScanPolling() {
		if (scanPollTimer !== null) {
			window.clearInterval(scanPollTimer);
			scanPollTimer = null;
		}
	}

	async function triggerScan() {
		scanning = true;
		try {
			const response = await fetch('/api/scan', { method: 'POST' });
			const result = response.ok ? await response.json().catch(() => null) : null;
			if ((window as any).refreshSidebar) {
				(window as any).refreshSidebar();
			}
			const [data, activeJobs] = await Promise.all([loadSettings(), loadActiveLibraryScanJobs()]);
			const queued = result?.queued_count ?? 0;
			const libraryScanning = !!data?.libraries?.some((library: any) => library.is_importing);
			scanning = queued > 0 || libraryScanning || activeJobs.length > 0;
			if (scanning) {
				startScanPolling();
			} else {
				stopScanPolling();
			}
		} catch (e) {
			console.error('Scan failed:', e);
			scanning = false;
		}
	}

	function setActiveTab(tab: 'general' | 'metadata' | 'reader' | 'appearance' | 'users' | 'admin') {
		activeTab = tab;
		if (typeof window === 'undefined') return;
		const url = new URL(window.location.href);
		if (tab === 'general') {
			url.searchParams.delete('tab');
		} else {
			url.searchParams.set('tab', tab);
		}
		window.history.replaceState({}, '', `${url.pathname}${url.search}${url.hash}`);
	}
	
	onMount(async () => {
		if (typeof window !== 'undefined') {
			const tab = new URLSearchParams(window.location.search).get('tab');
			if (tab === 'general' || tab === 'metadata' || tab === 'reader' || tab === 'appearance' || tab === 'users' || tab === 'admin') {
				activeTab = tab;
			}
		}
		await Promise.all([loadSettings(), loadActiveLibraryScanJobs()]);
		if (scanActive) {
			startScanPolling();
		}
		(window as any).refreshSettingsScans = refreshScanState;
		void readerSettings.syncWithBackend();
		loading = false;
	});

	onDestroy(() => {
		stopScanPolling();
		if ((window as any).refreshSettingsScans === refreshScanState) {
			delete (window as any).refreshSettingsScans;
		}
	});
</script>

<div class="space-y-6">
 	<div class="flex items-center justify-between">
 		<div>
 			<h1 class="text-2xl font-bold text-[var(--color-surface-text)]">Settings</h1>
 			<p class="text-[var(--color-surface-text-muted)] mt-1">Manage your library and application settings</p>
 		</div>
 	</div>

  	<!-- Tabs -->
  	<div class="flex space-x-1 border-b border-[var(--color-surface-border)]">
  		<button
  			onclick={() => setActiveTab('general')}
  			class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {activeTab === 'general' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
  		>
  			General
  		</button>
  		<button
  			onclick={() => setActiveTab('appearance')}
  			class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {activeTab === 'appearance' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
  		>
  			Appearance
  		</button>
  		<button
  			onclick={() => setActiveTab('reader')}
  			class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {activeTab === 'reader' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
  		>
  			Reader
  		</button>
  		<button
  			onclick={() => setActiveTab('metadata')}
  			class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {activeTab === 'metadata' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
  		>
  			Metadata
  		</button>
  		<button
  			onclick={() => setActiveTab('users')}
  			class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {activeTab === 'users' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
  		>
  			Users
  		</button>
  		<button
  			onclick={() => setActiveTab('admin')}
  			class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {activeTab === 'admin' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-500)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
  		>
  			Admin
  		</button>
  	</div>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if activeTab === 'appearance'}
		<div class="space-y-6">
			<!-- Colors Section -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] p-6">
				<h3 class="text-lg font-semibold text-[var(--color-surface-text)] mb-4">Colors</h3>
				
				<!-- Primary Color -->
				<div class="mb-6">
					<div class="mb-3 flex items-center justify-between">
						<div class="text-sm font-medium text-[var(--color-surface-text-muted)]">Primary Color</div>
						<button
							type="button"
							onclick={resetPrimaryToDefault}
							class="inline-flex h-5 w-5 items-center justify-center rounded text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
							title="Reset to default colors"
							aria-label="Reset primary color to default"
						>
								<svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" aria-hidden="true">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12a8 8 0 1 1-2.343-5.657"></path>
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 4v4h-4"></path>
								</svg>
						</button>
					</div>
					<div class="flex flex-wrap gap-2">
						{#each primaryColors as color}
							<button
								onclick={() => currentTheme.set({ ...themeState, primary: color })}
								class="w-8 h-8 rounded-full border-2 transition-all hover:scale-110 {themeState.primary === color ? 'border-white ring-2 ring-[var(--color-primary-500)]' : 'border-[var(--color-surface-border)]'}"
								style="background-color: {(() => {
									const colorMap: Record<string, string> = {
										'red': '#ef4444', 'orange': '#f97316', 'yellow': '#eab308', 'green': '#22c55e',
										'teal': '#14b8a6', 'blue': '#3b82f6', 'indigo': '#6366f1', 'purple': '#a855f7', 'pink': '#ec4899', 'rose': '#f43f5e',
										'red-400': '#f87171', 'orange-400': '#fb923c', 'yellow-400': '#facc15', 'green-400': '#4ade80', 'teal-400': '#2dd4bf', 'blue-400': '#60a5fa', 'indigo-400': '#818cf8', 'purple-400': '#c084fc', 'pink-400': '#f9a8d4', 'rose-400': '#fb7185',
										'red-600': '#dc2626', 'orange-600': '#ea580c', 'yellow-600': '#ca8a04', 'green-600': '#16a34a', 'teal-600': '#0f766e', 'blue-600': '#2563eb', 'indigo-600': '#4f46e5', 'purple-600': '#9333ea', 'pink-600': '#db2777', 'rose-600': '#e11d48',
										'red-800': '#991b1b', 'orange-800': '#9a3412', 'yellow-800': '#a16207', 'green-800': '#166534', 'teal-800': '#134e4a', 'blue-800': '#1e40af', 'indigo-800': '#3730a3', 'purple-800': '#6b21a8', 'pink-800': '#9d174d', 'rose-800': '#9f1239'
									};
									return colorMap[color] || '#888';
								})()}"
								title={color}
							></button>
						{/each}
					</div>
				</div>

				<!-- Surface Color -->
				<div>
					<div class="mb-3 flex items-center justify-between">
						<div class="text-sm font-medium text-[var(--color-surface-text-muted)]">Surface Color</div>
						<button
							type="button"
							onclick={resetSurfaceToDefault}
							class="inline-flex h-5 w-5 items-center justify-center rounded text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
							title="Reset to default colors"
							aria-label="Reset surface color to default"
						>
								<svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" aria-hidden="true">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12a8 8 0 1 1-2.343-5.657"></path>
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 4v4h-4"></path>
								</svg>
						</button>
					</div>
					<div class="flex flex-wrap gap-2">
						{#each surfaceColors as color}
							<button
								onclick={() => currentTheme.set({ ...themeState, surface: color })}
								class="w-10 h-10 rounded-lg border-2 transition-all hover:scale-105 {themeState.surface === color ? 'border-white ring-2 ring-[var(--color-primary-500)]' : 'border-[var(--color-surface-border)]'}"
								style="background-color: {(() => {
									const colorMap: Record<string, string> = {
										'dark': '#0f172a', 'darker': '#020617', 'light': '#f8fafc', 'lighter': '#ffffff',
										'slate': '#0f172a', 'gray': '#111827', 'zinc': '#09090b', 'neutral': '#171717', 'stone': '#1c1917',
										'red-surface': '#0f0a0a', 'orange-surface': '#0f0d07', 'yellow-surface': '#0f0f06', 'green-surface': '#0a0f0a',
										'teal-surface': '#0a0f0f', 'blue-surface': '#0a0d0f', 'indigo-surface': '#0d0a0f', 'purple-surface': '#0f0a0f', 'pink-surface': '#0f0a0d'
									};
									return colorMap[color] || '#888';
								})()}"
								title={color}
							></button>
						{/each}
					</div>
				</div>
			</div>

			<!-- Glow Section -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] p-6">
				<div class="flex items-center justify-between mb-4">
					<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Top Glow</h3>
					<div class="flex items-center space-x-3">
						<span class="text-sm text-[var(--color-surface-text-muted)]">{themeState.appearance.glowEnabled ? 'On' : 'Off'}</span>
						<button
							type="button"
							onclick={() => updateGlowEnabled(!themeState.appearance.glowEnabled)}
							class="relative w-12 h-6 rounded-full transition-colors {themeState.appearance.glowEnabled ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-surface-border)]'}"
							aria-label={themeState.appearance.glowEnabled ? 'Disable top glow' : 'Enable top glow'}
							title={themeState.appearance.glowEnabled ? 'Disable top glow' : 'Enable top glow'}
						>
							<span
								class="absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform {themeState.appearance.glowEnabled ? 'left-7' : 'left-1'}"
							></span>
						</button>
					</div>
				</div>
				
				<!-- Auto Mode Toggle -->
				<div class="flex items-center space-x-3 mb-4 {themeState.appearance.glowEnabled ? '' : 'opacity-50 pointer-events-none'}">
					<input
						type="checkbox"
						id="glowAutoMode"
						checked={themeState.appearance.glowAutoMode}
						onchange={(e) => updateGlowAutoMode(e.currentTarget.checked)}
						class="rounded bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
					>
					<label for="glowAutoMode" class="text-sm font-medium text-[var(--color-surface-text)]">Automatic</label>
					<span class="text-xs text-[var(--color-surface-text-muted)]">(Match primary color)</span>
				</div>

				<!-- Glow Color (disabled when auto) -->
				<div class="mb-4">
					<div class="text-sm font-medium text-[var(--color-surface-text-muted)] mb-2">Custom Glow Color</div>
					<div class="flex items-center space-x-3">
						<input
							type="color"
							value={themeState.appearance.glowColor}
							oninput={(e) => updateGlowColor(e.currentTarget.value)}
							disabled={themeState.appearance.glowAutoMode}
							class="w-10 h-10 rounded-lg cursor-pointer border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] disabled:opacity-50 disabled:cursor-not-allowed"
						>
						<input
							type="text"
							value={themeState.appearance.glowColor}
							oninput={(e) => updateGlowColor(e.currentTarget.value)}
							disabled={themeState.appearance.glowAutoMode}
							class="flex-1 px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] disabled:opacity-50 disabled:cursor-not-allowed"
							placeholder="#22c55e"
						>
					</div>
				</div>

				<!-- Glow Intensity -->
				<div class="{themeState.appearance.glowEnabled ? '' : 'opacity-50 pointer-events-none'}">
					<div class="flex items-center justify-between mb-2">
						<div class="text-sm font-medium text-[var(--color-surface-text-muted)]">Intensity</div>
						<div class="text-sm text-[var(--color-surface-text)]">{themeState.appearance.glowIntensity}%</div>
					</div>
					<input
						type="range"
						min="0"
						max="100"
						value={themeState.appearance.glowIntensity}
						oninput={(e) => updateGlowIntensity(parseInt(e.currentTarget.value))}
						class="w-full h-2 rounded-lg appearance-none cursor-pointer"
					>
				</div>
			</div>

			<!-- Background Images Section -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] p-6">
				<div class="flex items-center justify-between mb-4">
					<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Background Image</h3>
					<div class="flex items-center space-x-3">
						<span class="text-sm text-[var(--color-surface-text-muted)]">{themeState.appearance.bgImageEnabled ? 'On' : 'Off'}</span>
						<button
							type="button"
							onclick={() => updateBgImageEnabled(!themeState.appearance.bgImageEnabled)}
							class="relative w-12 h-6 rounded-full transition-colors {themeState.appearance.bgImageEnabled ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-surface-border)]'}"
							aria-label={themeState.appearance.bgImageEnabled ? 'Disable background image' : 'Enable background image'}
							title={themeState.appearance.bgImageEnabled ? 'Disable background image' : 'Enable background image'}
						>
							<span
								class="absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform {themeState.appearance.bgImageEnabled ? 'left-7' : 'left-1'}"
							></span>
						</button>
					</div>
				</div>

				<div class="mb-4 {themeState.appearance.bgImageEnabled ? '' : 'opacity-50 pointer-events-none'}">
					<div class="text-sm font-medium text-[var(--color-surface-text-muted)] mb-2">Display</div>
					<div class="grid grid-cols-2 gap-2 sm:grid-cols-5">
						{#each bgImageDisplayOptions as option}
							<button
								type="button"
								onclick={() => updateBgImageDisplay(option.id)}
								class="rounded-lg border px-3 py-2 text-left transition-colors {themeState.appearance.bgImageDisplay === option.id ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10 text-[var(--color-surface-text)]' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-surface-text)] hover:border-[var(--color-surface-500)] hover:bg-[var(--color-surface-overlay)]'}"
								title={option.description}
							>
								<span class="block text-sm font-medium">{option.label}</span>
								<span class="block text-xs text-[var(--color-surface-text-muted)]">{option.description}</span>
							</button>
						{/each}
					</div>
				</div>

				<!-- Transparency Slider -->
				<div class="mb-4 {themeState.appearance.bgImageEnabled ? '' : 'opacity-50 pointer-events-none'}">
					<div class="flex items-center justify-between mb-2">
						<div class="text-sm font-medium text-[var(--color-surface-text-muted)]">Transparency</div>
						<div class="text-sm text-[var(--color-surface-text)]">{themeState.appearance.bgImageTransparency}%</div>
					</div>
					<input
						type="range"
						min="0"
						max="100"
						value={themeState.appearance.bgImageTransparency}
						oninput={(e) => updateBgImageTransparency(parseInt(e.currentTarget.value))}
						class="w-full h-2 rounded-lg appearance-none cursor-pointer"
					>
					<div class="flex justify-between text-xs text-[var(--color-surface-text-muted)] mt-1">
						<span>0% (Hidden)</span>
						<span>100% (Fully visible)</span>
					</div>
				</div>
				
				<div class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 gap-3 {themeState.appearance.bgImageEnabled ? '' : 'opacity-50 pointer-events-none'}">
					<!-- Placeholder images (user will add actual images) -->
					<div class="aspect-video bg-[var(--color-surface-base)] rounded-lg border-2 border-dashed border-[var(--color-surface-border)] flex items-center justify-center text-[var(--color-surface-text-muted)]">
						<span class="text-xs">Soon</span>
					</div>
					<div class="aspect-video bg-[var(--color-surface-base)] rounded-lg border-2 border-dashed border-[var(--color-surface-border)] flex items-center justify-center text-[var(--color-surface-text-muted)]">
						<span class="text-xs">Soon</span>
					</div>
					<div class="aspect-video bg-[var(--color-surface-base)] rounded-lg border-2 border-dashed border-[var(--color-surface-border)] flex items-center justify-center text-[var(--color-surface-text-muted)]">
						<span class="text-xs">Soon</span>
					</div>

					<!-- Custom background images from user -->
					{#each themeState.appearance.backgroundImages as img, index}
						{@const bgIndex = index}
						{@const isSelected = themeState.appearance.selectedBgImageIndex === bgIndex}
						<div class="relative aspect-video bg-[var(--color-surface-base)] rounded-lg overflow-hidden group cursor-pointer transition-all duration-200 {isSelected ? 'ring-2 ring-[var(--color-primary-500)]' : 'hover:ring-2 hover:ring-[var(--color-surface-500)] hover:scale-105'}">
							<img src={img} alt="Background {index + 1}" class="w-full h-full object-cover pointer-events-none">
							<!-- Selected indicator -->
							{#if isSelected}
								<div class="absolute bottom-1 left-1 right-1 bg-[var(--color-primary-500)] text-white text-xs py-0.5 px-2 rounded text-center">
									Selected
								</div>
							{/if}
							<!-- Remove button (visible on hover) -->
							<div
								role="button"
								tabindex="0"
								onclick={(e) => { e.stopPropagation(); bgToRemove = bgIndex; showRemoveBgModal = true; }}
								onkeydown={(e) => { if (e.key === 'Enter') { e.stopPropagation(); bgToRemove = bgIndex; showRemoveBgModal = true; } }}
								class="absolute top-1 right-1 z-10 w-6 h-6 bg-red-500 hover:bg-red-600 rounded-full flex items-center justify-center cursor-pointer opacity-0 group-hover:opacity-100 transition-opacity"
								title="Remove background"
							>
								<svg class="w-4 h-4 text-white pointer-events-none" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
								</svg>
							</div>
							<!-- Click to select (overlay for easier clicking) -->
							<div
								role="button"
								tabindex="0"
								onclick={() => updateSelectedBgImage(bgIndex)}
								onkeydown={(e) => { if (e.key === 'Enter') updateSelectedBgImage(bgIndex); }}
								class="absolute inset-0 z-[5] cursor-pointer"
								title="Select background"
							></div>
						</div>
					{/each}

					<!-- Add new background button -->
					<label class="aspect-video bg-[var(--color-surface-base)] rounded-lg border-2 border-dashed border-[var(--color-surface-border)] flex items-center justify-center cursor-pointer hover:border-[var(--color-primary-500)] transition-colors">
						<svg class="w-8 h-8 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"></path>
						</svg>
						<input type="file" accept="image/*" onchange={handleFileUpload} class="hidden">
					</label>
				</div>
			</div>
		</div>
	{:else if activeTab === 'reader'}
		<div class="space-y-6">
			<!-- EPUB Reader Settings -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] overflow-hidden">
				<div class="px-6 py-4 border-b border-[var(--color-surface-border)] flex items-center space-x-3">
					<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center">
						<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
						</svg>
					</div>
					<div>
						<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">eBook Reader</h2>
						<p class="text-sm text-[var(--color-surface-text-muted)]">Settings for EPUB, FB2, MOBI, AZW3 formats</p>
					</div>
				</div>
				
				<div class="p-6 space-y-6">
					<!-- Appearance -->
					<div>
						<h3 class="text-sm font-semibold text-[var(--color-surface-text)] mb-4 flex items-center">
							<svg class="w-4 h-4 mr-2 text-[var(--color-surface-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01"></path>
							</svg>
							Appearance
						</h3>
						<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
							<!-- Theme -->
							<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Theme</div>
								<div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
									{#each epubThemes as theme}
										<button
											onclick={() => updateEpubSetting('theme', theme.id)}
											class="flex flex-col items-center p-2 rounded-lg border-2 transition-all {localReaderSettings.epub.theme === theme.id ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] hover:border-[var(--color-surface-500)]'}"
										>
											<ThemePreviewSwatch background={theme.bg} foreground={theme.text} sizeClass="h-8 w-8 mb-1" />
											<span class="text-xs text-[var(--color-surface-text)]">{theme.name}</span>
											<span class="text-[10px] text-[var(--color-surface-text-muted)] font-mono">{theme.bg} / {theme.text}</span>
										</button>
									{/each}
									{#if themeState.appearance.customThemes?.length}
										{#each themeState.appearance.customThemes as theme}
											<div class="relative group">
												<button
													onclick={() => updateEpubSetting('theme', theme.id)}
													class="w-full flex flex-col items-center p-2 rounded-lg border-2 transition-all {localReaderSettings.epub.theme === theme.id ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] hover:border-[var(--color-surface-500)]'}"
												>
													<ThemePreviewSwatch background={theme.background} foreground={theme.foreground} sizeClass="h-8 w-8 mb-1" />
													<span class="text-xs text-[var(--color-surface-text)] truncate w-full">{theme.name}</span>
													<span class="text-[10px] text-[var(--color-surface-text-muted)] font-mono truncate w-full">{theme.background} / {theme.foreground}</span>
												</button>
												<div class="absolute top-1 right-1 flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
													<button
														onclick={(e) => { e.stopPropagation(); openEditCustomTheme(theme); }}
														class="p-1 rounded bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text-muted)]"
														title="Edit custom theme"
													>
														<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
															<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z"></path>
														</svg>
													</button>
													<button
														onclick={(e) => { e.stopPropagation(); deleteCustomTheme(theme.id); }}
														class="p-1 rounded bg-red-500/20 hover:bg-red-500/40 text-red-400"
														title="Delete custom theme"
													>
														<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
															<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
														</svg>
													</button>
												</div>
											</div>
										{/each}
									{/if}
									<button
										onclick={openAddCustomTheme}
										class="flex flex-col items-center justify-center p-2 rounded-lg border-2 border-dashed border-[var(--color-surface-border)] hover:border-[var(--color-primary-500)] transition-all"
									>
										<div class="w-8 h-8 rounded-full mb-1 border border-[var(--color-surface-border)] flex items-center justify-center">
											<svg class="w-4 h-4 text-[var(--color-surface-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
											</svg>
										</div>
										<span class="text-xs text-[var(--color-surface-text)]">Custom</span>
									</button>
								</div>
							</div>
						</div>
					</div>

					<!-- Typography -->
					<div>
						<h3 class="text-sm font-semibold text-[var(--color-surface-text)] mb-4 flex items-center">
							<svg class="w-4 h-4 mr-2 text-[var(--color-surface-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
							</svg>
							Typography
						</h3>
						<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
							<!-- Font Family -->
							<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Font Family</div>
								<div class="flex flex-wrap gap-2">
									{#each fontFamilies as font}
										<button
											onclick={() => updateEpubSetting('fontFamily', font.id)}
											class="px-3 py-2 rounded-lg border-2 transition-all text-sm text-[var(--color-surface-text)] {localReaderSettings.epub.fontFamily === font.id ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] hover:border-[var(--color-surface-500)]'}"
											style="font-family: {font.family}"
										>
											{font.name}
										</button>
									{/each}
									<button
										onclick={() => {}}
										class="px-3 py-2 rounded-lg border-2 border-dashed border-[var(--color-surface-border)] hover:border-[var(--color-primary-500)] transition-all text-sm flex items-center gap-1 text-[var(--color-surface-text)]"
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
										</svg>
										Custom
									</button>
								</div>
							</div>

							<!-- Font Size -->
							<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Font Size</div>
								<div class="flex items-center space-x-2">
									<button
										onclick={() => updateEpubSetting('fontSize', Math.max(10, localReaderSettings.epub.fontSize - 1))}
										class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
									>−</button>
									<span class="text-[var(--color-surface-text)] w-12 text-center">{localReaderSettings.epub.fontSize}px</span>
									<button
										onclick={() => updateEpubSetting('fontSize', Math.min(32, localReaderSettings.epub.fontSize + 1))}
										class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
									>+</button>
								</div>
							</div>

							<!-- Line Height -->
							<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Line Height</div>
								<div class="flex items-center space-x-2">
									<button
										onclick={() => updateEpubSetting('lineHeight', Math.max(1.0, localReaderSettings.epub.lineHeight - 0.1))}
										class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
									>−</button>
									<span class="text-[var(--color-surface-text)] w-12 text-center">{localReaderSettings.epub.lineHeight.toFixed(1)}</span>
									<button
										onclick={() => updateEpubSetting('lineHeight', Math.min(3.0, localReaderSettings.epub.lineHeight + 0.1))}
										class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
									>+</button>
								</div>
							</div>

							<!-- Text Justification -->
							<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Text Alignment</div>
								<div class="flex space-x-1">
									<button
										onclick={() => updateEpubSetting('justify', false)}
										class="flex-1 px-3 py-2 rounded-lg border transition-all {localReaderSettings.epub.justify === false ? 'bg-[var(--color-primary-500)] border-[var(--color-primary-500)] text-white' : 'bg-[var(--color-surface-base)] border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]'}"
									>
										Left
									</button>
									<button
										onclick={() => updateEpubSetting('justify', true)}
										class="flex-1 px-3 py-2 rounded-lg border transition-all {localReaderSettings.epub.justify === true ? 'bg-[var(--color-primary-500)] border-[var(--color-primary-500)] text-white' : 'bg-[var(--color-surface-base)] border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]'}"
									>
										Justified
									</button>
								</div>
							</div>

							<!-- Hyphenation -->
							<div class="flex items-center space-x-3">
								<input
									type="checkbox"
									id="hyphenate"
									checked={localReaderSettings.epub.hyphenate}
									onchange={(e) => updateEpubSetting('hyphenate', e.currentTarget.checked)}
									class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
								>
								<label for="hyphenate" class="text-sm font-medium text-[var(--color-surface-text)]">Enable Hyphenation</label>
							</div>
						</div>
					</div>

					<!-- Layout -->
					<div>
						<h3 class="text-sm font-semibold text-[var(--color-surface-text)] mb-4 flex items-center">
							<svg class="w-4 h-4 mr-2 text-[var(--color-surface-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z"></path>
							</svg>
							Layout
						</h3>
						<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
							<!-- Flow -->
							<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Flow</div>
								<div class="flex space-x-1">
									<button
										onclick={() => updateEpubSetting('flow', 'paginated')}
										class="flex-1 px-3 py-2 rounded-lg border transition-all {localReaderSettings.epub.flow === 'paginated' ? 'bg-[var(--color-primary-500)] border-[var(--color-primary-500)] text-white' : 'bg-[var(--color-surface-base)] border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]'}"
									>
										Paginated
									</button>
									<button
										onclick={() => updateEpubSetting('flow', 'scrolled')}
										class="flex-1 px-3 py-2 rounded-lg border transition-all {localReaderSettings.epub.flow === 'scrolled' ? 'bg-[var(--color-primary-500)] border-[var(--color-primary-500)] text-white' : 'bg-[var(--color-surface-base)] border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]'}"
									>
										Scrolled
									</button>
								</div>
							</div>

							<!-- Column Gap -->
							<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Column Gap: {localReaderSettings.epub.gap}%</div>
								<input
									type="range"
									min="0"
									max="20"
									value={localReaderSettings.epub.gap}
									oninput={(e) => updateEpubSetting('gap', parseInt(e.currentTarget.value))}
									class="w-full h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer"
								>
							</div>

							<!-- Max Width -->
							<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Max Width: {localReaderSettings.epub.maxInlineSize}px</div>
								<input
									type="range"
									min="400"
									max="1200"
									step="20"
									value={localReaderSettings.epub.maxInlineSize}
									oninput={(e) => updateEpubSetting('maxInlineSize', parseInt(e.currentTarget.value))}
									class="w-full h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer"
								>
							</div>

							<!-- Max Height -->
							<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Max Height: {localReaderSettings.epub.maxBlockSize}px</div>
								<input
									type="range"
									min="400"
									max="2000"
									step="40"
									value={localReaderSettings.epub.maxBlockSize}
									oninput={(e) => updateEpubSetting('maxBlockSize', parseInt(e.currentTarget.value))}
									class="w-full h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer"
								>
							</div>
						</div>
					</div>
				</div>
			</div>

			<div class="flex items-center space-x-3">
				<input
					type="checkbox"
					id="epub-auto-hide-controls"
					checked={localReaderSettings.epub.autoHideControls}
					onchange={(e) => updateEpubSetting('autoHideControls', e.currentTarget.checked)}
					class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
				>
				<label for="epub-auto-hide-controls" class="text-sm font-medium text-[var(--color-surface-text)]">Auto-hide Controls</label>
			</div>

			<!-- PDF Reader Settings -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] overflow-hidden">
				<div class="px-6 py-4 border-b border-[var(--color-surface-border)] flex items-center space-x-3">
					<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center">
						<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
						</svg>
					</div>
					<div>
						<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">PDF Reader</h2>
						<p class="text-sm text-[var(--color-surface-text-muted)]">Settings for PDF documents</p>
					</div>
				</div>
				
				<div class="p-6 space-y-6">
					<!-- Display Settings -->
					<div>
						<h3 class="text-sm font-semibold text-[var(--color-surface-text)] mb-4">Display</h3>
						<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
							<!-- Page Spread -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Page Spread</div>
								<select 
									value={localReaderSettings.pdf.pageSpread} 
									onchange={(e) => updatePdfSetting('pageSpread', e.currentTarget.value)}
									class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								>
									<option value="off">None</option>
									<option value="even">Even</option>
									<option value="odd">Odd</option>
								</select>
							</div>

							<!-- Default Zoom -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Default Zoom</div>
								<select 
									value={localReaderSettings.pdf.pageZoom} 
									onchange={(e) => updatePdfSetting('pageZoom', e.currentTarget.value)}
									class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								>
									{#each pdfZoomModes as mode}
										<option value={mode.id}>{mode.name}</option>
									{/each}
								</select>
							</div>

							<!-- Scroll Mode -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Scroll Mode</div>
								<select 
									value={localReaderSettings.pdf.scrollMode} 
									onchange={(e) => updatePdfSetting('scrollMode', e.currentTarget.value)}
									class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								>
									<option value="paged">Paged</option>
									<option value="continuous-vertical">Continuous Vertical</option>
									<option value="continuous-horizontal">Continuous Horizontal</option>
								</select>
							</div>

							<!-- Page Rotation -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Page Rotation</div>
								<select 
									value={localReaderSettings.pdf.pageRotation} 
									onchange={(e) => updatePdfSetting('pageRotation', parseInt(e.currentTarget.value))}
									class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								>
									<option value={0}>0°</option>
									<option value={90}>90°</option>
									<option value={180}>180°</option>
									<option value={270}>270°</option>
								</select>
							</div>

							<!-- Background Color -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Background</div>
								<div class="flex items-center space-x-2">
									<input
										type="color"
										value={localReaderSettings.pdf.backgroundColor}
										oninput={(e) => updatePdfSetting('backgroundColor', e.currentTarget.value)}
										class="w-10 h-10 rounded cursor-pointer"
									>
									<span class="text-sm text-[var(--color-surface-text)] font-mono">{localReaderSettings.pdf.backgroundColor}</span>
								</div>
							</div>
						</div>
					</div>

					<!-- Reading Experience -->
					<div>
						<h3 class="text-sm font-semibold text-[var(--color-surface-text)] mb-4">Reading Experience</h3>
						<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
							<!-- Reading Direction -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Reading Direction</div>
								<select 
									value={localReaderSettings.pdf.readingDirection} 
									onchange={(e) => updatePdfSetting('readingDirection', e.currentTarget.value)}
									class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								>
									<option value="ltr">Left to Right</option>
									<option value="rtl">Right to Left</option>
								</select>
							</div>

							<!-- Auto-crop Margins -->
							<div class="flex items-center space-x-3">
								<input
									type="checkbox"
									id="autoCrop"
									checked={localReaderSettings.pdf.autoCropMargins}
									onchange={(e) => updatePdfSetting('autoCropMargins', e.currentTarget.checked)}
									class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
								>
								<label for="autoCrop" class="text-sm font-medium text-[var(--color-surface-text)]">Auto-crop Margins</label>
							</div>

							<!-- Grayscale -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Grayscale: {localReaderSettings.pdf.grayscale}%</div>
								<input
									type="range"
									min="0"
									max="100"
									value={localReaderSettings.pdf.grayscale}
									oninput={(e) => updatePdfSetting('grayscale', parseInt(e.currentTarget.value))}
									class="w-full h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer"
								>
							</div>
						</div>
					</div>

					<!-- Advanced -->
					<div>
						<h3 class="text-sm font-semibold text-[var(--color-surface-text)] mb-4">Advanced</h3>
						<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
							<!-- Text Layer -->
							<div class="flex items-center space-x-3">
								<input
									type="checkbox"
									id="textLayer"
									checked={localReaderSettings.pdf.textLayerEnabled}
									onchange={(e) => updatePdfSetting('textLayerEnabled', e.currentTarget.checked)}
									class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
								>
								<label for="textLayer" class="text-sm font-medium text-[var(--color-surface-text)]">Enable Text Layer</label>
							</div>

							<!-- Annotations -->
							<div class="flex items-center space-x-3">
								<input
									type="checkbox"
									id="annotations"
									checked={localReaderSettings.pdf.annotationsEnabled}
									onchange={(e) => updatePdfSetting('annotationsEnabled', e.currentTarget.checked)}
									class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
								>
								<label for="annotations" class="text-sm font-medium text-[var(--color-surface-text)]">Enable Annotations</label>
							</div>

							<!-- Auto-hide Controls -->
							<div class="flex items-center space-x-3">
								<input
									type="checkbox"
									id="pdfAutoHideControls"
									checked={localReaderSettings.pdf.autoHideControls}
									onchange={(e) => updatePdfSetting('autoHideControls', e.currentTarget.checked)}
									class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
								>
								<label for="pdfAutoHideControls" class="text-sm font-medium text-[var(--color-surface-text)]">Auto-hide Controls</label>
							</div>
						</div>
					</div>
				</div>
			</div>

			<!-- Comic Reader Settings -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] overflow-hidden">
				<div class="px-6 py-4 border-b border-[var(--color-surface-border)] flex items-center space-x-3">
					<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center">
						<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"></path>
						</svg>
					</div>
					<div>
						<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Comic Book Reader</h2>
						<p class="text-sm text-[var(--color-surface-text-muted)]">Settings for CBZ, CBR, CB7, CBT archives</p>
					</div>
				</div>
				
				<div class="p-6 space-y-6">
					<!-- Display Settings -->
					<div>
						<h3 class="text-sm font-semibold text-[var(--color-surface-text)] mb-4">Display</h3>
						<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
							<!-- Page Spread -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Page Spread</div>
								<select 
									value={localReaderSettings.cbx.pageSpread} 
									onchange={(e) => updateCbxSetting('pageSpread', e.currentTarget.value)}
									class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								>
									<option value="auto">Auto</option>
									<option value="off">None</option>
									<option value="even">Even</option>
									<option value="odd">Odd</option>
								</select>
							</div>

							<!-- Fit Mode -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Fit Mode</div>
								<select 
									value={localReaderSettings.cbx.fitMode} 
									onchange={(e) => updateCbxSetting('fitMode', e.currentTarget.value)}
									class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								>
									{#each cbxFitModes as mode}
										<option value={mode.id}>{mode.name}</option>
									{/each}
								</select>
							</div>

							<!-- Scroll Mode -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Scroll Mode</div>
								<select 
									value={localReaderSettings.cbx.scrollMode} 
									onchange={(e) => updateCbxSetting('scrollMode', e.currentTarget.value)}
									class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								>
									{#each cbxScrollModes as mode}
										<option value={mode.id}>{mode.name}</option>
									{/each}
								</select>
							</div>

							<!-- Reading Direction -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Reading Direction</div>
								<select 
									value={localReaderSettings.cbx.readingDirection} 
									onchange={(e) => updateCbxSetting('readingDirection', e.currentTarget.value)}
									class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								>
									<option value="ltr">Left to Right</option>
									<option value="rtl">Right to Left (Manga)</option>
									<option value="webtoon">Webtoon (Vertical)</option>
								</select>
							</div>

							<!-- Background Color -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Background Color</div>
								<div class="flex items-center space-x-2">
									<input
										type="color"
										value={localReaderSettings.cbx.backgroundColor}
										oninput={(e) => updateCbxSetting('backgroundColor', e.currentTarget.value)}
										class="w-10 h-10 rounded cursor-pointer"
									>
									<span class="text-sm text-[var(--color-surface-text)] font-mono">{localReaderSettings.cbx.backgroundColor}</span>
								</div>
							</div>
						</div>
					</div>

					<!-- Comic-Specific -->
					<div>
						<h3 class="text-sm font-semibold text-[var(--color-surface-text)] mb-4">Comic-Specific</h3>
						<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
							<!-- Manga Mode -->
							<div class="flex items-center space-x-3">
								<input
									type="checkbox"
									id="mangaMode"
									checked={localReaderSettings.cbx.mangaMode}
									onchange={(e) => updateCbxSetting('mangaMode', e.currentTarget.checked)}
									class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
								>
								<label for="mangaMode" class="text-sm font-medium text-[var(--color-surface-text)]">Manga Mode (RTL)</label>
							</div>

							<!-- Panel View -->
							<div class="flex items-center space-x-3">
								<input
									type="checkbox"
									id="panelView"
									checked={localReaderSettings.cbx.panelViewEnabled}
									onchange={(e) => updateCbxSetting('panelViewEnabled', e.currentTarget.checked)}
									class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
								>
								<label for="panelView" class="text-sm font-medium text-[var(--color-surface-text)]">Guided Panel Zoom</label>
							</div>

							<!-- Spread Handling -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Spread Handling</div>
								<select 
									value={localReaderSettings.cbx.spreadHandling} 
									onchange={(e) => updateCbxSetting('spreadHandling', e.currentTarget.value)}
									class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								>
									<option value="auto">Auto</option>
									<option value="force-single">Force Single</option>
									<option value="force-double">Force Double</option>
									<option value="never-split">Never Split</option>
								</select>
							</div>

							<!-- Vibrance -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Vibrance: {localReaderSettings.cbx.vibrance}%</div>
								<input
									type="range"
									min="0"
									max="200"
									value={localReaderSettings.cbx.vibrance}
									oninput={(e) => updateCbxSetting('vibrance', parseInt(e.currentTarget.value))}
									class="w-full h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer"
								>
							</div>

							<!-- Saturation -->
							<div>
								<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Saturation: {localReaderSettings.cbx.saturation}%</div>
								<input
									type="range"
									min="0"
									max="200"
									value={localReaderSettings.cbx.saturation}
									oninput={(e) => updateCbxSetting('saturation', parseInt(e.currentTarget.value))}
									class="w-full h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer"
								>
							</div>
						</div>
					</div>

					{#if localReaderSettings.cbx.scrollMode === 'long-strip' || localReaderSettings.cbx.scrollMode === 'infinite'}
						<!-- Strip Max Width -->
						<div class="max-w-xs">
							<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">
								Strip Max Width: {localReaderSettings.cbx.stripMaxWidthPercent}%
							</div>
							<input
								type="range"
								min="50"
								max="100"
								value={localReaderSettings.cbx.stripMaxWidthPercent}
								oninput={(e) => updateCbxSetting('stripMaxWidthPercent', parseInt(e.currentTarget.value))}
								class="w-full h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer"
							>
							<div class="flex justify-between text-xs text-[var(--color-surface-text-muted)] mt-1">
								<span>50%</span>
								<span>100%</span>
							</div>
						</div>
					{/if}

					<div class="flex items-center space-x-3">
						<input
							type="checkbox"
							id="cbx-auto-hide-controls"
							checked={localReaderSettings.cbx.autoHideControls}
							onchange={(e) => updateCbxSetting('autoHideControls', e.currentTarget.checked)}
							class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
						>
						<label for="cbx-auto-hide-controls" class="text-sm font-medium text-[var(--color-surface-text)]">Auto-hide Controls</label>
					</div>
				</div>
			</div>

			<!-- Speed Reader Settings -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] overflow-hidden">
				<div class="px-6 py-4 border-b border-[var(--color-surface-border)] flex items-center space-x-3">
					<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center">
						<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
						</svg>
					</div>
					<div>
						<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Speed Reader</h2>
						<p class="text-sm text-[var(--color-surface-text-muted)]">RSVP speed reading settings</p>
					</div>
				</div>
				
				<div class="p-6 space-y-6">
					<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
						<!-- WPM -->
						<div>
							<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Words Per Minute</div>
							<div class="flex items-center space-x-2">
								<button
									onclick={() => updateSpeedReaderSetting('wpm', Math.max(100, localReaderSettings.speedReader.wpm - 25))}
									class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
								>−</button>
								<span class="text-[var(--color-surface-text)] w-16 text-center font-mono">{localReaderSettings.speedReader.wpm}</span>
								<button
									onclick={() => updateSpeedReaderSetting('wpm', Math.min(1200, localReaderSettings.speedReader.wpm + 25))}
									class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
								>+</button>
							</div>
						</div>

						<!-- Word Size -->
						<div>
							<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Word Size</div>
							<div class="flex items-center space-x-2">
								<button
									onclick={() => updateSpeedReaderSetting('wordSize', Math.max(24, localReaderSettings.speedReader.wordSize - 4))}
									class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
								>−</button>
								<span class="text-[var(--color-surface-text)] w-12 text-center">{localReaderSettings.speedReader.wordSize}px</span>
								<button
									onclick={() => updateSpeedReaderSetting('wordSize', Math.min(72, localReaderSettings.speedReader.wordSize + 4))}
									class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
								>+</button>
							</div>
						</div>

						<!-- Focal Point -->
						<div>
							<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Focal Point</div>
							<div class="flex items-center space-x-2">
								<button
									onclick={() => updateSpeedReaderSetting('focalPoint', Math.max(0.2, localReaderSettings.speedReader.focalPoint - 0.02))}
									class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
								>−</button>
								<span class="text-[var(--color-surface-text)] w-12 text-center">{(localReaderSettings.speedReader.focalPoint * 100).toFixed(0)}%</span>
								<button
									onclick={() => updateSpeedReaderSetting('focalPoint', Math.min(0.6, localReaderSettings.speedReader.focalPoint + 0.02))}
									class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
								>+</button>
							</div>
						</div>

						<!-- Focus Indicator -->
						<div>
							<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Focus Indicator</div>
							<select 
								value={localReaderSettings.speedReader.focusIndicator} 
								onchange={(e) => updateSpeedReaderSetting('focusIndicator', e.currentTarget.value)}
								class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
							>
								<option value="off">Off</option>
								<option value="lines">Lines</option>
								<option value="arrows">Arrows</option>
							</select>
						</div>

						<!-- Sentence Pause -->
						<div>
							<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Sentence Pause</div>
							<div class="flex items-center space-x-2">
								<button
									onclick={() => updateSpeedReaderSetting('sentencePause', Math.max(100, localReaderSettings.speedReader.sentencePause - 50))}
									class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
								>−</button>
								<span class="text-[var(--color-surface-text)] w-16 text-center">{localReaderSettings.speedReader.sentencePause}ms</span>
								<button
									onclick={() => updateSpeedReaderSetting('sentencePause', Math.min(500, localReaderSettings.speedReader.sentencePause + 50))}
									class="w-8 h-8 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] flex items-center justify-center"
								>+</button>
							</div>
						</div>
					</div>

					<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
						<!-- Accent Color -->
						<div>
							<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Accent Color</div>
							<div class="flex items-center space-x-3">
								<input
									type="color"
									value={localReaderSettings.speedReader.accentColor}
									onchange={(e) => updateSpeedReaderSetting('accentColor', e.currentTarget.value)}
									class="w-10 h-10 rounded cursor-pointer"
								>
								<span class="text-sm text-[var(--color-surface-text)] font-mono">{localReaderSettings.speedReader.accentColor}</span>
							</div>
						</div>

						<!-- Accent Enabled -->
						<div class="flex items-center space-x-3">
							<input
								type="checkbox"
								id="accent-enabled"
								checked={localReaderSettings.speedReader.accentEnabled}
								onchange={(e) => updateSpeedReaderSetting('accentEnabled', e.currentTarget.checked)}
								class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
							>
							<label for="accent-enabled" class="text-sm font-medium text-[var(--color-surface-text)]">Enable accent character</label>
						</div>

						<!-- Keep Screen On -->
						<div class="flex items-center space-x-3">
							<input
								type="checkbox"
								id="keep-screen-on"
								checked={localReaderSettings.speedReader.keepScreenOn}
								onchange={(e) => updateSpeedReaderSetting('keepScreenOn', e.currentTarget.checked)}
								class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
							>
							<label for="keep-screen-on" class="text-sm font-medium text-[var(--color-surface-text)]">Keep screen on</label>
						</div>

						<!-- Horizontal Bars -->
						<div class="flex items-center space-x-3">
							<input
								type="checkbox"
								id="horizontal-bars"
								checked={localReaderSettings.speedReader.horizontalBars}
								onchange={(e) => updateSpeedReaderSetting('horizontalBars', e.currentTarget.checked)}
								class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
							>
							<label for="horizontal-bars" class="text-sm font-medium text-[var(--color-surface-text)]">Show horizontal focus lines</label>
						</div>
					</div>
				</div>
			</div>

			<!-- Audio Reader Settings -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] overflow-hidden">
				<div class="px-6 py-4 border-b border-[var(--color-surface-border)] flex items-center space-x-3">
					<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center">
						<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3"></path>
						</svg>
					</div>
					<div>
						<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Audio Book Reader</h2>
						<p class="text-sm text-[var(--color-surface-text-muted)]">Settings for audio books</p>
					</div>
				</div>
				
				<div class="p-6">
					<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
						<!-- Playback Speed -->
						<div>
							<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Default Playback Speed</div>
							<select 
								value={localReaderSettings.audio.playbackSpeed} 
								onchange={(e) => updateAudioSetting('playbackSpeed', parseFloat(e.currentTarget.value))}
								class="w-full px-3 py-2 bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
							>
								<option value="0.5">0.5x</option>
								<option value="0.75">0.75x</option>
								<option value="1.0">1.0x (Normal)</option>
								<option value="1.25">1.25x</option>
								<option value="1.5">1.5x</option>
								<option value="1.75">1.75x</option>
								<option value="2.0">2.0x</option>
							</select>
						</div>

						<!-- Auto-advance -->
						<div class="flex items-center space-x-3">
							<input
								type="checkbox"
								id="auto-advance"
								checked={localReaderSettings.audio.autoAdvance}
								onchange={(e) => updateAudioSetting('autoAdvance', e.currentTarget.checked)}
								class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
							>
							<label for="auto-advance" class="text-sm font-medium text-[var(--color-surface-text)]">Auto-advance to next chapter</label>
						</div>

						<div class="flex items-center space-x-3">
							<input
								type="checkbox"
								id="audio-auto-hide-controls"
								checked={localReaderSettings.audio.autoHideControls}
								onchange={(e) => updateAudioSetting('autoHideControls', e.currentTarget.checked)}
								class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
							>
							<label for="audio-auto-hide-controls" class="text-sm font-medium text-[var(--color-surface-text)]">Auto-hide Controls</label>
						</div>
					</div>
				</div>
			</div>

			<!-- Save Button -->
			<div class="flex justify-end pt-4">
				<button
					onclick={saveReaderSettings}
					class="px-6 py-2.5 bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white rounded-lg transition-colors flex items-center space-x-2"
				>
					{#if settingsSaved}
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
						</svg>
						<span>Saved</span>
					{:else}
						<span>Save Reader Settings</span>
					{/if}
				</button>
			</div>
		</div>

	{:else if activeTab === 'general'}
  		<div class="space-y-6">
  			<!-- Libraries -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] overflow-hidden">
				<div class="px-6 py-4 border-b border-[var(--color-surface-border)] flex items-center justify-between">
					<div class="flex items-center space-x-3">
						<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center">
							<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 14v3m4-3v3m4-3v3M3 21h18M3 10h18M3 7l9-4 9 4M4 10h16v11H4V10z"></path>
							</svg>
						</div>
						<div>
							<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Libraries</h2>
							<p class="text-sm text-[var(--color-surface-text-muted)]">Configure your book libraries</p>
						</div>
					</div>
					<div class="flex items-center gap-2">
						<button
							onclick={triggerScan}
							disabled={scanActive}
							class="inline-flex items-center gap-2 rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text-muted)] transition-colors hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)] disabled:cursor-not-allowed disabled:opacity-70"
							title="Scan all libraries"
						>
							<svg class="h-4 w-4 {scanActive ? 'animate-scan-spin text-[var(--color-primary-400)]' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
							</svg>
							<span>{scanActive ? 'Scanning...' : 'Scan Libraries'}</span>
						</button>
						<button
							onclick={() => openLibraryModal()}
							class="p-2 rounded-lg border border-[var(--color-surface-border)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors"
							title="Add Library"
						>
							<svg class="w-4 h-4 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
							</svg>
						</button>
					</div>
				</div>
				<div class="p-6">
					{#if settings.libraries && settings.libraries.length > 0}
						<div class="space-y-4">
							{#each settings.libraries as lib}
								<div class="bg-[var(--color-surface-overlay)] rounded-lg p-4 border transition-colors {isLibraryScanActive(lib) ? 'border-[var(--color-primary-500)]/70' : 'border-[var(--color-surface-border)]'}">
									<div class="flex items-center justify-between mb-3">
										<div class="flex items-center space-x-3">
											{#if isLibraryScanActive(lib)}
												<svg class="w-5 h-5 animate-scan-spin text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
													<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
												</svg>
											{:else}
												<svg class="w-5 h-5 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
													<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
												</svg>
											{/if}
											<h3 class="font-medium text-[var(--color-surface-text)]">{lib.name}</h3>
											{#if isLibraryScanRunning(lib)}
												<span class="rounded-full bg-[var(--color-primary-500)]/15 px-2 py-0.5 text-xs font-semibold text-[var(--color-primary-300)]">Scanning</span>
											{:else if isLibraryScanQueued(lib)}
												<span class="rounded-full bg-[var(--color-surface-base)] px-2 py-0.5 text-xs font-semibold text-[var(--color-surface-text-muted)]">Queued</span>
											{/if}
										</div>
										<div class="flex items-center space-x-2">
											<button
												onclick={() => scanLibrary(lib)}
												disabled={isLibraryScanActive(lib)}
												class="p-1.5 rounded text-[var(--color-surface-text-muted)] hover:text-[var(--color-primary-500)] hover:bg-[var(--color-surface-overlay)] transition-colors disabled:cursor-not-allowed disabled:opacity-70"
												title="Scan Library"
											>
												<svg class="w-4 h-4 {isLibraryScanActive(lib) ? 'animate-scan-spin text-[var(--color-primary-400)]' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
													<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
												</svg>
											</button>
											<button
												onclick={() => openLibraryModal(lib)}
												class="p-1.5 rounded text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] transition-colors"
												title="Edit Library"
											>
												<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
													<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path>
												</svg>
											</button>
											<button
												onclick={() => deleteLibrary(lib)}
												disabled={deletingLibraryId === lib.id}
												class="p-1.5 rounded text-red-400 hover:text-red-300 hover:bg-red-500/10 transition-colors disabled:opacity-50"
												title="Delete Library"
											>
												{#if deletingLibraryId === lib.id}
													<svg class="w-4 h-4 animate-scan-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
														<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
													</svg>
												{:else}
													<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
														<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
													</svg>
												{/if}
											</button>
										</div>
									</div>
									<div class="ml-8 space-y-2">
										{#each lib.paths.filter((p: string) => p.trim()) as path}
											<div class="flex items-center space-x-2 text-sm text-[var(--color-surface-text-muted)]">
												<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
													<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
												</svg>
												<span class="font-mono text-[var(--color-surface-text)]">{path}</span>
											</div>
										{/each}
									</div>
								</div>
							{/each}
						</div>
					{:else}
						<div class="text-center py-8">
							<svg class="w-12 h-12 text-[var(--color-primary-400)] mx-auto mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
							</svg>
							<p class="text-[var(--color-surface-text-muted)]">No libraries configured</p>
						</div>
					{/if}
				</div>
			</div>

			<!-- BookDrop -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] overflow-hidden">
				<div class="px-6 py-4 border-b border-[var(--color-surface-border)] flex items-center space-x-3">
					<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center">
						<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"></path>
						</svg>
					</div>
					<div class="flex-1">
						<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">BookDrop</h2>
						<p class="text-sm text-[var(--color-surface-text-muted)]">Auto-import folder for dropped books</p>
					</div>
					<button
						onclick={() => { bookdropPath = settings.bookdrop?.path || ''; showBookdropModal = true; }}
						class="p-2 rounded-lg border border-[var(--color-surface-border)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors"
						title="Add Bookdrop Location"
					>
						<svg class="w-4 h-4 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
						</svg>
					</button>
				</div>
				<div class="p-6">
					{#if settings.bookdrop}
						<div class="flex items-center justify-between">
							<div class="flex items-center space-x-3 text-[var(--color-surface-text)]">
								<svg class="w-5 h-5 text-[var(--color-surface-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
								</svg>
								<span class="font-mono text-[var(--color-surface-text)]">{settings.bookdrop.path}</span>
							</div>
							<span class="text-xs px-2 py-1 rounded bg-[var(--color-surface-700)] text-[var(--color-surface-text-muted)]">Active</span>
						</div>
						<p class="text-sm text-[var(--color-surface-text-muted)] mt-3">Drop files here to automatically import them into your library.</p>
					{:else}
						<div class="flex items-center justify-between">
							<p class="text-[var(--color-surface-text-muted)]">BookDrop is not configured</p>
							<span class="text-xs px-2 py-1 rounded bg-red-500/20 text-red-400">Inactive</span>
						</div>
					{/if}
				</div>
			</div>

			<!-- About -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] overflow-hidden">
				<div class="px-6 py-4 border-b border-[var(--color-surface-border)] flex items-center space-x-3">
					<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center">
						<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
						</svg>
					</div>
					<div>
						<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">About Cryptorum</h2>
						<p class="text-sm text-[var(--color-surface-text-muted)]">Version and system information</p>
					</div>
				</div>
				<div class="p-6">
					<p class="text-[var(--color-surface-text)] mb-4">
						Cryptorum is a personal digital library application designed for single-user self-hosting.
					</p>
					<div class="flex items-center space-x-4 text-sm text-[var(--color-surface-500)]">
						<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)] border border-[var(--color-surface-border)]">
							Go Backend
						</span>
						<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)] border border-[var(--color-surface-border)]">
							SvelteKit Frontend
						</span>
						<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)] border border-[var(--color-surface-border)]">
							SQLite Database
						</span>
					</div>
				</div>
			</div>

			<!-- Book Covers -->
			<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] overflow-hidden">
				<div class="px-6 py-4 border-b border-[var(--color-surface-border)] flex items-center justify-between">
					<div class="flex items-center space-x-3">
						<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center">
							<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 20l9-5-9-5-9 5 9 5zm0-10l9-5-9-5-9 5 9 5z"></path>
							</svg>
						</div>
						<div>
							<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Book Covers</h2>
							<p class="text-sm text-[var(--color-surface-text-muted)]">Regenerate and fit stored book covers</p>
						</div>
					</div>
					<div class="flex items-center gap-2">
						<button
							onclick={() => regenerateBookCovers('missing')}
							disabled={coverRegenerating}
							class="px-3 py-2 rounded-lg border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors disabled:opacity-50"
						>
							Regenerate Missing
						</button>
						<button
							onclick={() => regenerateBookCovers('all')}
							disabled={coverRegenerating}
							class="px-3 py-2 rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white transition-colors disabled:opacity-50"
						>
							{coverRegenerating ? 'Starting...' : 'Regenerate All'}
						</button>
					</div>
				</div>
				<div class="p-6 space-y-6">
					<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-4 py-3">
						<div class="flex items-center justify-between gap-4">
							<div>
								<div class="text-sm font-medium text-[var(--color-surface-text)]">Preserve Full Cover Art</div>
								<div class="text-xs text-[var(--color-surface-text-muted)]">Fit the entire cover into a filled frame without trimming.</div>
							</div>
							<input
								type="checkbox"
								checked={bookCoverSettings.preserve_full_cover}
								onchange={(e) => updateBookCoverSetting('preserve_full_cover', e.currentTarget.checked)}
								class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
							>
						</div>
						{#if !bookCoverSettings.preserve_full_cover}
							<div class="mt-4 grid grid-cols-1 md:grid-cols-2 gap-4">
								<div class="flex items-center justify-between rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-4 py-3">
									<div>
										<div class="text-sm font-medium text-[var(--color-surface-text)]">Vertical Cover Cropping</div>
										<div class="text-xs text-[var(--color-surface-text-muted)]">Crop very tall covers from the top</div>
									</div>
									<input
										type="checkbox"
										checked={bookCoverSettings.vertical_cropping}
										onchange={(e) => updateBookCoverSetting('vertical_cropping', e.currentTarget.checked)}
										class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
									>
								</div>
								<div class="flex items-center justify-between rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-4 py-3">
									<div>
										<div class="text-sm font-medium text-[var(--color-surface-text)]">Horizontal Cover Cropping</div>
										<div class="text-xs text-[var(--color-surface-text-muted)]">Crop very wide covers from the left</div>
									</div>
									<input
										type="checkbox"
										checked={bookCoverSettings.horizontal_cropping}
										onchange={(e) => updateBookCoverSetting('horizontal_cropping', e.currentTarget.checked)}
										class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
									>
								</div>
							</div>
							<div class="mt-4 grid grid-cols-1 md:grid-cols-2 gap-4">
								<div>
									<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Aspect Ratio Threshold</div>
									<div class="flex items-center gap-3">
										<input
											type="range"
											min="1.25"
											max="5"
											step="0.05"
											value={bookCoverSettings.aspect_ratio_threshold}
											oninput={(e) => updateBookCoverSetting('aspect_ratio_threshold', parseFloat(e.currentTarget.value))}
											class="flex-1 h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer"
										>
										<input
											type="number"
											min="1.25"
											max="5"
											step="0.05"
											value={bookCoverSettings.aspect_ratio_threshold}
											oninput={(e) => updateBookCoverSetting('aspect_ratio_threshold', parseFloat(e.currentTarget.value))}
											class="w-24 px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] font-mono"
										>
									</div>
								</div>
								<div class="flex items-center justify-between rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-4 py-3">
									<div>
										<div class="text-sm font-medium text-[var(--color-surface-text)]">Smart Cropping</div>
										<div class="text-xs text-[var(--color-surface-text-muted)]">Skip uniform margins when cropping</div>
									</div>
									<input
										type="checkbox"
										checked={bookCoverSettings.smart_cropping}
										onchange={(e) => updateBookCoverSetting('smart_cropping', e.currentTarget.checked)}
										class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
									>
								</div>
							</div>
						{/if}
					</div>
					<div class="flex items-center justify-between gap-3">
						<p class="text-sm text-[var(--color-surface-text-muted)]">
							{#if coverActionMessage}
								{coverActionMessage}
							{:else}
								Stored covers are regenerated from embedded image data where available.
							{/if}
						</p>
						<button
							onclick={saveBookCoverSettings}
							disabled={coverSettingsSaving}
							class="px-4 py-2 rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white transition-colors disabled:opacity-50"
						>
							{coverSettingsSaved ? 'Saved' : coverSettingsSaving ? 'Saving...' : 'Save Cover Settings'}
						</button>
					</div>
				</div>
			</div>
		</div>
	{:else if activeTab === 'metadata'}
		<MetadataManagerContent showHeader={false} />
	{:else if activeTab === 'users'}
		<UsersPanel />
	{:else if activeTab === 'admin'}
		<AdminPanel />
	{/if}
</div>

<!-- Remove Background Confirmation Modal -->
{#if showRemoveBgModal}
		<div class="fixed inset-0 bg-black/50 flex items-center justify-center z-[100] p-4" role="dialog" aria-modal="true" tabindex="-1">
		<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] w-full max-w-sm shadow-2xl">
			<div class="p-6">
				<h3 class="text-lg font-semibold text-[var(--color-surface-text)] mb-2">Remove Background</h3>
				<p class="text-[var(--color-surface-text-muted)] mb-6">Are you sure you want to remove this background image?</p>
				<div class="flex justify-end space-x-3">
					<button
						onclick={() => showRemoveBgModal = false}
						class="px-4 py-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] transition-colors"
					>
						Cancel
					</button>
					<button
						onclick={doRemoveBg}
						class="px-4 py-2 rounded-lg bg-red-500 hover:bg-red-600 text-white font-medium transition-colors"
					>
						Remove
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

<!-- Bookdrop Modal -->
{#if showBookdropModal}
		<div class="fixed inset-0 z-[110] flex items-center justify-center p-4">
		<button
			type="button"
			class="absolute inset-0 bg-black/60"
			aria-label="Close bookdrop modal"
			onclick={() => showBookdropModal = false}
		></button>
		<div class="relative z-10 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] w-full max-w-2xl overflow-hidden shadow-2xl">
			<div class="px-6 py-4 border-b border-[var(--color-surface-border)]">
				<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Add Bookdrop Location</h3>
				<p class="text-sm text-[var(--color-surface-text-muted)] mt-1">Choose the folder where dropped books should be watched for import.</p>
			</div>
			<div class="p-6 space-y-4">
				<div>
					<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Path</div>
					<div class="flex items-center gap-2">
						<input
							type="text"
							bind:value={bookdropPath}
							placeholder="/path/to/bookdrop"
							class="flex-1 px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
						>
						<button
							onclick={() => openDirectoryModal('bookdrop')}
							class="px-3 py-2 rounded-lg border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors"
						>
							Folder
						</button>
					</div>
				</div>
				<div class="flex items-center justify-between text-sm text-[var(--color-surface-text-muted)]">
					<span>Paths are persisted into `config.yaml`.</span>
					<span class="font-mono">{bookdropPath || 'No path selected'}</span>
				</div>
			</div>
			<div class="px-6 py-4 border-t border-[var(--color-surface-border)] flex justify-end space-x-3">
				<button
					onclick={() => showBookdropModal = false}
					class="px-4 py-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] transition-colors"
				>
					Cancel
				</button>
				<button
					onclick={saveBookdropLocation}
					disabled={!bookdropPath.trim()}
					class="px-4 py-2 rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
				>
					Save Location
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Custom Theme Modal -->
{#if showCustomThemeEditor}
		<div class="fixed inset-0 z-[120] flex items-center justify-center p-4">
		<button
			type="button"
			class="absolute inset-0 bg-black/60"
			aria-label="Close custom theme modal"
			onclick={() => showCustomThemeEditor = false}
		></button>
		<div class="relative z-10 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] w-full max-w-xl overflow-hidden shadow-2xl">
			<div class="px-6 py-4 border-b border-[var(--color-surface-border)]">
				<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">
					{editingCustomTheme ? 'Edit Custom Theme' : 'Add Custom Theme'}
				</h3>
			</div>
			<div class="p-6 space-y-4">
				<div>
					<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Theme Name</div>
					<input
						type="text"
						bind:value={customThemeName}
						placeholder="My Theme"
						class="w-full px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
					>
				</div>
				<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
					<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Text Color</div>
						<div class="flex items-start gap-3">
							<label class="relative flex h-12 w-12 flex-shrink-0 items-center justify-center overflow-hidden rounded-full border border-[var(--color-surface-border)] shadow-inner" style="background-color: {resolvePreviewColor(customThemeFg, '#ffffff')};">
								<span class="absolute inset-0 ring-1 ring-inset ring-black/10"></span>
								<input
									type="color"
									value={resolvePreviewColor(customThemeFg, '#ffffff')}
									oninput={(e) => setForegroundColorFromHex(e.currentTarget.value)}
									class="absolute inset-0 h-full w-full cursor-pointer opacity-0"
									aria-label="Pick text color"
								>
							</label>
							<div class="flex-1 space-y-2">
								<input
									type="text"
									value={customThemeFgHexInput}
									oninput={(e) => setForegroundColorFromHex(e.currentTarget.value)}
									placeholder="#ffffff"
									class="w-full px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] font-mono"
								>
								<input
									type="text"
									value={customThemeFgRgb}
									oninput={(e) => setForegroundColorFromRgb(e.currentTarget.value)}
									placeholder="rgb(255, 255, 255)"
									class="w-full px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] font-mono"
								>
							</div>
						</div>
					</div>
					<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">Background Color</div>
						<div class="flex items-start gap-3">
							<label class="relative flex h-12 w-12 flex-shrink-0 items-center justify-center overflow-hidden rounded-full border border-[var(--color-surface-border)] shadow-inner" style="background-color: {resolvePreviewColor(customThemeBg, '#111111')};">
								<span class="absolute inset-0 ring-1 ring-inset ring-black/10"></span>
								<input
									type="color"
									value={resolvePreviewColor(customThemeBg, '#111111')}
									oninput={(e) => setBackgroundColorFromHex(e.currentTarget.value)}
									class="absolute inset-0 h-full w-full cursor-pointer opacity-0"
									aria-label="Pick background color"
								>
							</label>
							<div class="flex-1 space-y-2">
								<input
									type="text"
									value={customThemeBgHexInput}
									oninput={(e) => setBackgroundColorFromHex(e.currentTarget.value)}
									placeholder="#111111"
									class="w-full px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] font-mono"
								>
								<input
									type="text"
									value={customThemeBgRgb}
									oninput={(e) => setBackgroundColorFromRgb(e.currentTarget.value)}
									placeholder="rgb(17, 17, 17)"
									class="w-full px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] font-mono"
								>
							</div>
						</div>
					</div>
				</div>
				<p class="text-xs text-[var(--color-surface-text-muted)]">
					Use HEX like <span class="font-mono">#ffffff</span> or RGB like <span class="font-mono">rgb(255, 255, 255)</span>.
				</p>
				<div class="rounded-lg border border-[var(--color-surface-border)] p-4 flex items-center gap-3" style="background-color: {resolvePreviewColor(customThemeBg, '#111111')}; color: {resolvePreviewColor(customThemeFg, '#ffffff')};">
					<ThemePreviewSwatch background={resolvePreviewColor(customThemeBg, '#111111')} foreground={resolvePreviewColor(customThemeFg, '#ffffff')} />
					<div>
						<div class="font-medium">{customThemeName || 'Theme Preview'}</div>
						<div class="text-xs opacity-80">{resolvePreviewColor(customThemeBg, '#111111')} / {resolvePreviewColor(customThemeFg, '#ffffff')}</div>
					</div>
				</div>
			</div>
			<div class="px-6 py-4 border-t border-[var(--color-surface-border)] flex justify-end space-x-3">
				<button
					onclick={() => showCustomThemeEditor = false}
					class="px-4 py-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] transition-colors"
				>
					Cancel
				</button>
				<button
					onclick={saveCustomTheme}
					disabled={!canSaveCustomTheme}
					class="px-4 py-2 rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
				>
					{editingCustomTheme ? 'Update Theme' : 'Add Theme'}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Library Modal -->
{#if showLibraryModal}
		<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
		<button
			type="button"
			class="absolute inset-0 bg-black/80"
			aria-label="Close library modal"
			onclick={closeLibraryModal}
		></button>
		<div class="relative z-10 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] w-full max-w-2xl max-h-[90vh] overflow-hidden shadow-2xl">
			<div class="px-6 py-4 border-b border-[var(--color-surface-border)]">
				<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">
					{editingLibrary ? 'Edit Library' : 'Add Library'}
				</h3>
			</div>
			<div class="p-6 space-y-6 overflow-y-auto max-h-[calc(90vh-120px)]">
				<div class="grid grid-cols-2 gap-4">
					<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">
							Library Name
						</div>
						<input
							type="text"
							bind:value={libraryForm.name}
							class="w-full px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)] focus:border-transparent"
							placeholder="Enter library name"
						/>
					</div>
					<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">
							Icon
						</div>
						{#if currentLibraryIcon}
							<div class="flex items-center gap-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2">
								<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-[var(--color-surface-overlay)] text-[var(--color-primary-400)]">
									{#if currentLibraryIcon.svg}
										<div class="h-5 w-5 overflow-hidden">{@html currentLibraryIcon.svg}</div>
									{:else}
										<span class="text-xs font-semibold uppercase">{currentLibraryIcon.name.slice(0, 1) || '?'}</span>
									{/if}
								</div>
								<div class="min-w-0 flex-1">
									<div class="truncate text-sm font-medium text-[var(--color-surface-text)]">{currentLibraryIcon.name}</div>
									<div class="text-xs text-[var(--color-surface-text-muted)]">
										{currentLibraryIcon.source === 'custom' ? 'Custom SVG' : currentLibraryIcon.source === 'svg' ? 'SVG Library' : 'Prime Icons'}
									</div>
								</div>
								<button
									type="button"
									onclick={clearLibraryIcon}
									class="inline-flex h-7 w-7 items-center justify-center rounded-md text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
									title="Remove icon"
									aria-label="Remove icon"
								>
									<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
										<path d="M18 6 6 18"></path>
										<path d="m6 6 12 12"></path>
									</svg>
								</button>
							</div>
						{:else}
							<button
								type="button"
								onclick={openLibraryIconPicker}
								class="inline-flex w-full items-center justify-center gap-2 rounded-lg border border-dashed border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-3 text-sm font-medium text-[var(--color-primary-400)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-overlay)]"
							>
								<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
									<path d="M12 5v14"></path>
									<path d="M5 12h14"></path>
								</svg>
								+ Select icon
							</button>
						{/if}
					</div>
				</div>

				<div>
					<div class="flex items-center justify-between mb-2">
						<div class="block text-sm font-medium text-[var(--color-surface-text)]">
							Book Folders
						</div>
						<button
							onclick={() => openDirectoryModal('bookdrop')}
							class="px-3 py-1.5 text-sm bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white rounded-lg transition-colors"
						>
							Add Folder
						</button>
					</div>
					<div class="space-y-2 min-h-[60px] border-2 border-dashed border-[var(--color-surface-border)] rounded-lg p-4">
						{#if libraryForm.paths.filter(p => p.trim()).length === 0}
							<div class="text-center py-4 text-[var(--color-surface-text-muted)]">
								<svg class="w-8 h-8 mx-auto mb-2 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
								</svg>
								<p class="text-sm">No folders selected</p>
								<p class="text-xs mt-1">Click "Add Folder" to select directories</p>
							</div>
						{:else}
							{#each libraryForm.paths.filter(p => p.trim()) as path, i}
								<div class="flex items-center justify-between bg-[var(--color-surface-base)] rounded-lg p-3 border border-[var(--color-surface-border)]">
									<div class="flex items-center space-x-3">
										<svg class="w-5 h-5 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
										</svg>
										<span class="font-mono text-sm text-[var(--color-surface-text)]">{path}</span>
									</div>
									<button
										onclick={() => removePath(libraryForm.paths.indexOf(path))}
										class="p-1 rounded text-red-400 hover:text-red-300 hover:bg-red-500/10 transition-colors"
										title="Remove folder"
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
										</svg>
									</button>
								</div>
							{/each}
						{/if}
					</div>
				</div>
			</div>
			<div class="px-6 py-4 border-t border-[var(--color-surface-border)] flex justify-end space-x-3">
				<button
					onclick={closeLibraryModal}
					class="px-4 py-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] transition-colors"
				>
					Cancel
				</button>
				<button
					onclick={saveLibrary}
					disabled={!libraryForm.name.trim() || libraryForm.paths.filter(p => p.trim()).length === 0}
					class="px-4 py-2 rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
				>
					{editingLibrary ? 'Update' : 'Create'} Library
				</button>
			</div>
		</div>
	</div>
{/if}

<LibraryIconPicker
	open={showLibraryIconPicker}
	selectedIcon={libraryForm.icon}
	onSelect={selectLibraryIcon}
	onClose={closeLibraryIconPicker}
/>

<!-- Directory Selection Modal -->
{#if showDirectoryModal}
	<div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
		<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] w-full max-w-lg max-h-[80vh] flex flex-col overflow-hidden">
			<div class="px-6 py-4 border-b border-[var(--color-surface-border)] flex-shrink-0">
				<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">
					Select Directory
				</h3>
				<p class="text-sm text-[var(--color-surface-text-muted)] mt-1">
					Current: <span class="font-mono">{currentDirectory}</span>
				</p>
			</div>
			<div class="p-6 overflow-y-auto custom-scrollbar flex-1 min-h-0">
				{#if directoryContents.length === 0}
					<div class="text-center py-8">
						<svg class="w-8 h-8 text-[var(--color-surface-text-muted)] mx-auto mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
						</svg>
						<p class="text-[var(--color-surface-text-muted)]">No contents available</p>
					</div>
				{:else}
					<div class="space-y-1">
						{#each directoryContents as item}
							<button
								onclick={() => selectDirectory(item)}
								class="w-full flex items-center space-x-3 px-3 py-2 rounded-lg text-left hover:bg-[var(--color-surface-base)] transition-colors {item.type === 'directory' ? 'cursor-pointer' : 'cursor-default'}"
								disabled={item.type !== 'directory'}
							>
								{#if item.type === 'directory'}
									<svg class="w-5 h-5 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
									</svg>
								{:else}
									<svg class="w-5 h-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
									</svg>
								{/if}
								<span class="text-[var(--color-surface-text)]">{item.name}</span>
							</button>
						{/each}
					</div>
				{/if}
			</div>
			<div class="px-6 py-4 border-t border-[var(--color-surface-border)] flex justify-end space-x-3 flex-shrink-0">
				<button
					onclick={closeDirectoryModal}
					class="px-4 py-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] transition-colors"
				>
					Cancel
				</button>
				<button
					onclick={addSelectedDirectory}
					disabled={!currentDirectory || currentDirectory === '/'}
					class="px-4 py-2 rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
				>
					Select Directory
				</button>
			</div>
		</div>
	</div>
{/if}

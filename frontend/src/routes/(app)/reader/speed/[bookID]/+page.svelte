<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { browser } from '$app/environment';
	import { readerSettings, speedReaderThemes, fontFamilies, fontWeightOptions, resolveFontFamily, type SpeedReaderSetting } from '$lib/stores/readerSettings';
	import { currentTheme as appThemeStore, resolveThemeColors, addCustomTheme, removeCustomTheme, generateId, type FullTheme } from '$lib/stores/theme';
	import ThemePreviewSwatch from '$lib/components/ThemePreviewSwatch.svelte';
	import { normalizeBookFormat } from '$lib/utils/book-formats';
	import { getReaderDisplayTitle } from '$lib/utils/reader-title';

	interface ProcessedWord {
		text: string;
	}

	let book = $state<any>(null);
	let readerFiles = $state<any[]>([]);
	let loading = $state(true);
	let loadError = $state('');
	let words = $state<ProcessedWord[]>([]);
	let currentIndex = $state(0);
	let isPlaying = $state(false);
	let intervalId: number | null = null;
	let savedProgress = $state<any>(null);
	let showWordPicker = $state(false);
	let showControls = $state(true);
	let showSettings = $state(false);
	let showWpmMenu = $state(false);
	let activeSettingsTab = $state<'reading' | 'typography' | 'focus'>('reading');
	let controlsTimeout: ReturnType<typeof setTimeout> | null = null;
	let containerEl: HTMLDivElement;
	let settingsPanelRef: HTMLDivElement | null = $state(null);
	let wpmMenuRef: HTMLDivElement | null = $state(null);
	let wordContainerEl = $state<HTMLDivElement | null>(null);
	let accentCharEl = $state<HTMLSpanElement | null>(null);
	let accentWidth = $state(0);
	let currentSessionId = $state<number | null>(null);
	let lastWheelNavigationAt = 0;
	let sessionEnded = false;
	let handlePageExit: (() => void) | null = null;
	let closeTasksStarted = false;
	let requestedFormat = $state('');
	const readerTitle = $derived(getReaderDisplayTitle(book, readerFiles, loading, requestedFormat));

	// Word picker state
	let wordPickerPending = $state(0);   // word the user is about to jump to
	let wordPickerOrigin = $state(0);    // word the user was at when panel opened
	const WORD_PICKER_WINDOW_RADIUS = 450;

	function preprocessText(text: string): string {
		const result: string[] = [];
		let inWhitespace = true;

		for (const char of text) {
			if (char === '\n' || char === '\r' || char === '\t' || /\s/.test(char)) {
				if (!inWhitespace) {
					result.push(' ');
					inWhitespace = true;
				}
			} else {
				result.push(char);
				inWhitespace = false;
			}
		}

		let processed = result.join('').trim();
		processed = processed.replace(/\s+([—–])\s+/g, ' $1 ');
		return processed;
	}

	function splitIntoWords(text: string): string[] {
		return text.split(/\s+/).filter(w => w.length > 0);
	}

	function cleanWordForSpeedReader(word: string): string {
		const punctuation = /[\p{P}\p{S}]/u;
		let result = '';
		for (const char of word) {
			if (/[\p{L}\p{N}\p{M}]/u.test(char)) {
				result += char;
			} else if (punctuation.test(char)) {
				result += char;
			}
		}
		return result;
	}

	function isSentenceEnding(word: string): boolean {
		const trimmed = word.trim();
		return trimmed.endsWith('.') || trimmed.endsWith('!') || trimmed.endsWith('?') ||
		       trimmed.endsWith(';') || trimmed.endsWith(':');
	}

	function buildParagraphs(ws: ProcessedWord[]): { start: number; end: number }[] {
		// Group words into rough paragraphs: new paragraph after a sentence-ending word
		// once the current paragraph has reached MIN_WORDS, or unconditionally at MAX_WORDS.
		const MIN_WORDS = 45;
		const MAX_WORDS = 100;
		const result: { start: number; end: number }[] = [];
		let paraStart = 0;

		for (let i = 0; i < ws.length; i++) {
			const count = i - paraStart + 1;
			if ((isSentenceEnding(ws[i].text) && count >= MIN_WORDS) || count >= MAX_WORDS) {
				result.push({ start: paraStart, end: i + 1 });
				paraStart = i + 1;
			}
		}
		if (paraStart < ws.length) {
			result.push({ start: paraStart, end: ws.length });
		}
		return result;
	}

	let wordPickerParagraphs = $derived(buildParagraphs(words));
	let wordPickerVisibleStart = $derived(Math.max(0, wordPickerPending - WORD_PICKER_WINDOW_RADIUS));
	let wordPickerVisibleEnd = $derived(Math.min(words.length, wordPickerPending + WORD_PICKER_WINDOW_RADIUS + 1));
	let wordPickerVisibleParagraphs = $derived(
		wordPickerParagraphs
			.filter((para) => para.end > wordPickerVisibleStart && para.start < wordPickerVisibleEnd)
			.map((para) => ({
				start: Math.max(para.start, wordPickerVisibleStart),
				end: Math.min(para.end, wordPickerVisibleEnd)
			}))
	);

	function processText(text: string): ProcessedWord[] {
		const processed: ProcessedWord[] = [];
		const cleaned = preprocessText(text);
		const rawWords = splitIntoWords(cleaned);

		for (const rawWord of rawWords) {
			const cleanWord = cleanWordForSpeedReader(rawWord);
			if (cleanWord.length > 0) {
				processed.push({ text: cleanWord });
			}
		}

		return processed;
	}

	async function responseErrorMessage(response: Response, fallback: string): Promise<string> {
		const text = (await response.text().catch(() => '')).trim();
		if (!text) return fallback;
		try {
			const parsed = JSON.parse(text);
			if (typeof parsed?.error === 'string' && parsed.error.trim()) {
				return parsed.error.trim();
			}
		} catch {
			// Fall through to the raw response body.
		}
		return text;
	}

	let settings = $state<SpeedReaderSetting>({
		wpm: 300,
		wordSize: 64,
		fontFamily: 'serif',
		fontWeight: 400,
		focalPoint: 0.50,
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
		theme: 'catppuccin',
		letterSpacing: 0,
		focusIndicatorLength: 20,
		showWordCount: false
	});

	let readerTheme = $state(speedReaderThemes[0]);
	let appTheme = $state<FullTheme | null>(null);
	let currentFontFamily = $derived(resolveFontFamily(settings.fontFamily));

	function updateReaderTheme() {
		const theme = resolveThemeColors(
			settings.theme,
			speedReaderThemes,
			appTheme?.appearance.customThemes ?? [],
			{ foreground: '#e5e7eb', background: '#111111' }
		);
		readerTheme = {
			id: settings.theme,
			name: 'Selected',
			bg: theme.background,
			text: theme.foreground
		};
	}

	const unsubTheme = appThemeStore.subscribe(theme => {
		appTheme = theme;
		updateReaderTheme();
	});

	const unsubSettings = readerSettings.subscribe(s => {
		settings = { ...s.speedReader, theme: s.readerTheme || s.speedReader.theme };
		updateReaderTheme();
	});

	onMount(() => {
		const globalTapListener = (event: MouseEvent) => {
			const target = event.target as Node | null;
			if (!target || !containerEl || !containerEl.contains(target)) return;
			handleTap(event);
		};

		void (async () => {
				await document.fonts?.ready;
				const bookId = $page.params.bookID;
				requestedFormat = normalizeBookFormat($page.url.searchParams.get('format'));
				try {
					const [res, filesRes] = await Promise.all([
						fetch(`/api/books/${bookId}`),
						fetch(`/api/books/${bookId}/files`)
					]);
					if (res.ok) {
						book = await res.json();
						if (filesRes.ok) {
							readerFiles = await filesRes.json();
						}
						await fetchProgress();
						await loadText();
					await startSession();
				}
			} catch (e) {
				console.error('Failed to load book:', e);
			} finally {
				loading = false;
			}

			handlePageExit = () => {
				if (closeTasksStarted) return;
				stop();
				void endSession(true);
			};

			window.addEventListener('pagehide', handlePageExit);
			window.addEventListener('beforeunload', handlePageExit);
			window.addEventListener('keydown', handleKeyDown);
			window.addEventListener('wheel', handleWheelNavigation, { passive: false });
			window.addEventListener('click', globalTapListener);
		})();

		return () => {
			window.removeEventListener('click', globalTapListener);
		};
	});

	function handleKeyDown(e: KeyboardEvent) {
		if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
		if (showWordPicker || showSettings || showWpmMenu) return;

		if (e.code === 'Space') {
			e.preventDefault();
			if (isPlaying) stop(); else play();
		} else if (e.code === 'ArrowRight') {
			e.preventDefault();
			stepWord(1);
		} else if (e.code === 'ArrowLeft') {
			e.preventDefault();
			stepWord(-1);
		} else if (e.key === 'Escape') {
			e.preventDefault();
			void closeReader();
		}
	}

	function shouldIgnoreWheelNavigation(target: EventTarget | null) {
		if (!(target instanceof Element)) return false;
		return !!target.closest('input, textarea, select, [contenteditable="true"]') ||
			showWordPicker ||
			showSettings ||
			showWpmMenu;
	}

	function handleWheelNavigation(e: WheelEvent) {
		if (shouldIgnoreWheelNavigation(e.target)) return;

		const dominantDelta = Math.abs(e.deltaY) >= Math.abs(e.deltaX) ? e.deltaY : e.deltaX;
		if (Math.abs(dominantDelta) < 12) return;

		const now = performance.now();
		if (now - lastWheelNavigationAt < 120) {
			e.preventDefault();
			return;
		}

		e.preventDefault();
		lastWheelNavigationAt = now;
		stepWord(dominantDelta > 0 ? 1 : -1);
	}

	onDestroy(() => {
		if (handlePageExit) {
			window.removeEventListener('pagehide', handlePageExit);
			window.removeEventListener('beforeunload', handlePageExit);
		}
		window.removeEventListener('keydown', handleKeyDown);
		window.removeEventListener('wheel', handleWheelNavigation);
		unsubSettings();
		unsubTheme();
		stop();
		if (!closeTasksStarted) {
			void endSession(true);
		}
	});

	async function fetchProgress() {
		try {
			const res = await fetch(`/api/books/${book.id}/progress`);
			if (res.ok) {
				savedProgress = await res.json();
			}
		} catch (e) {
			console.error('Failed to fetch progress:', e);
		}
	}

	async function startSession() {
		try {
			const res = await fetch(`/api/books/${book.id}/sessions`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ reader_type: 'speed' })
			});
			if (res.ok) {
				const data = await res.json();
				currentSessionId = data.id;
			}
		} catch (e) {
			console.error('Failed to start session:', e);
		}
	}

	async function endSession(keepalive = false) {
		if (sessionEnded || currentSessionId === null) return;
		sessionEnded = true;
		try {
			await fetch(`/api/books/${book.id}/sessions/${currentSessionId}`, {
				method: 'PUT',
				keepalive
			});
		} catch (e) {
			console.error('Failed to end session:', e);
		}
	}

	async function loadText() {
		try {
			const res = await fetch(`/api/books/${book.id}/text${requestedFormat ? `?format=${encodeURIComponent(requestedFormat)}` : ''}`);
			if (res.ok) {
				const text = await res.text();
				words = processText(text);
				if (words.length === 0) {
					loadError = 'No text was extracted for speed reading.';
					currentIndex = 0;
				} else if (savedProgress?.speed_reader_word_index > 0) {
					loadError = '';
					currentIndex = Math.max(0, Math.min(words.length - 1, savedProgress.speed_reader_word_index));
				} else if (savedProgress?.speed_reader_percent > 0) {
					loadError = '';
					currentIndex = Math.max(
						0,
						Math.min(words.length - 1, Math.floor((savedProgress.speed_reader_percent / 100) * words.length))
					);
				} else {
					loadError = '';
				}
			} else {
				loadError = await responseErrorMessage(res, 'Failed to load text for speed reading.');
			}
		} catch (e) {
			console.error('Failed to load text:', e);
			loadError = 'Failed to load text for speed reading.';
		}
	}

	async function saveProgress(keepalive = false) {
		if (!book || words.length === 0) return;
		const percent = (currentIndex / words.length) * 100;
		try {
			await fetch(`/api/books/${book.id}/progress`, {
				method: 'PUT',
				keepalive,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					percent: percent,
					status: percent >= 100 ? 'finished' : 'reading'
				})
			});
			await fetch(`/api/books/${book.id}/speed-reader`, {
				method: 'PUT',
				keepalive,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					word_index: currentIndex,
					percent: percent
				})
			});
		} catch (e) {
			console.error('Failed to save progress:', e);
		}
	}

	function calculateAutoSentencePause(wpm: number): number {
		const baseWpm = 300;
		const basePause = 350;
		const minPause = 50;
		return Math.max(minPause, Math.min(1000, Math.round(basePause * (baseWpm / wpm))));
	}

	function getWordDelay(word: ProcessedWord): number {
		const wpm = settings.wpm;
		const baseDelay = 60000 / wpm;

		if (settings.autoSentencePause && isSentenceEnding(word.text)) {
			return baseDelay + calculateAutoSentencePause(wpm);
		}

		if (!settings.autoSentencePause && isSentenceEnding(word.text)) {
			return baseDelay + settings.sentencePause;
		}

		if (/[,;]$/.test(word.text)) {
			return baseDelay * 1.5;
		}

		return baseDelay;
	}

	function play() {
		if (words.length === 0) return;
		isPlaying = true;
		hideControls();

		const advanceWord = () => {
			if (currentIndex < words.length - 1) {
				currentIndex++;
				const delay = getWordDelay(words[currentIndex]);
				intervalId = window.setTimeout(advanceWord, delay);
			} else {
				stop();
			}
		};

		const delay = getWordDelay(words[currentIndex]);
		intervalId = window.setTimeout(advanceWord, delay);
	}

	function stop() {
		isPlaying = false;
		if (intervalId) {
			clearTimeout(intervalId);
			intervalId = null;
		}
		saveProgress();
		showControlsTemporarily();
	}

	function togglePlay(e: MouseEvent) {
		e.stopPropagation();
		if (isPlaying) {
			stop();
		} else {
			play();
		}
	}

	function reset() {
		stop();
		currentIndex = 0;
		saveProgress();
	}

	function stepWord(direction: 1 | -1) {
		if (isPlaying) stop();
		if (direction > 0 && currentIndex < words.length - 1) {
			currentIndex++;
		} else if (direction < 0 && currentIndex > 0) {
			currentIndex--;
		}
	}

	function prevWord(e: MouseEvent) {
		e.stopPropagation();
		stepWord(-1);
	}

	function nextWord(e: MouseEvent) {
		e.stopPropagation();
		stepWord(1);
	}

	function openWordPicker(e: MouseEvent | { stopPropagation: () => void }) {
		e.stopPropagation();
		showWpmMenu = false;
		wordPickerPending = currentIndex;
		wordPickerOrigin = currentIndex;
		showWordPicker = true;
	}

	function confirmWordPicker(e?: Event) {
		e?.stopPropagation();
		currentIndex = wordPickerPending;
		showWordPicker = false;
	}

	function cancelWordPicker(e?: Event) {
		e?.stopPropagation();
		showWordPicker = false;
	}

	function showControlsTemporarily() {
		showControls = true;
		if (controlsTimeout) clearTimeout(controlsTimeout);
		controlsTimeout = setTimeout(() => {
			if (isPlaying) showControls = false;
		}, 3000);
	}

	function hideControls() {
		showControls = false;
	}

	function handleTap(event: MouseEvent | TouchEvent) {
		const target = event.target as HTMLElement;

		if (target.closest('.top-nav') || target.closest('.speed-footer')) {
			return;
		}

		if (showSettings) {
			if (settingsPanelRef && settingsPanelRef.contains(target)) {
				return;
			}
			showSettings = false;
			return;
		}

		if (showWordPicker) {
			// Word picker covers the full screen and handles its own taps
			return;
		}

		if (showWpmMenu) {
			if (wpmMenuRef && wpmMenuRef.contains(target)) {
				return;
			}
			showWpmMenu = false;
			return;
		}

		const rect = containerEl.getBoundingClientRect();
		let x: number;

		if ('touches' in event) {
			x = event.touches[0].clientX - rect.left;
		} else {
			x = event.clientX - rect.left;
		}

		const width = rect.width;
		const zone = x / width;

		if (zone < 0.2) {
			if (isPlaying) stop();
			else prevWord(event as MouseEvent);
		} else if (zone > 0.8) {
			if (isPlaying) stop();
			else nextWord(event as MouseEvent);
		} else {
			togglePlay(event as MouseEvent);
		}
	}

	function splitGraphemes(text: string): string[] {
		if (typeof Intl !== 'undefined' && 'Segmenter' in Intl) {
			const segmenter = new Intl.Segmenter(undefined, { granularity: 'grapheme' });
			return Array.from(segmenter.segment(text), segment => segment.segment);
		}
		return Array.from(text);
	}

	function findAccentCharIndex(graphemes: string[]): number {
		const readableIndices = graphemes
			.map((grapheme, index) => /[\p{L}\p{N}]/u.test(grapheme) ? index : -1)
			.filter(index => index >= 0);

		const len = readableIndices.length;
		if (len === 0) return graphemes.length > 0 ? 0 : -1;

		let readableAccentIndex = 0;
		if (len <= 1) readableAccentIndex = 0;
		else if (len <= 5) readableAccentIndex = 1;
		else if (len <= 9) readableAccentIndex = 2;
		else if (len <= 13) readableAccentIndex = 3;
		else readableAccentIndex = 4;

		return readableIndices[Math.min(readableAccentIndex, len - 1)];
	}

	function getWordParts(word: ProcessedWord): { before: string; accent: string; after: string } {
		const graphemes = splitGraphemes(word.text);
		const accentIndex = findAccentCharIndex(graphemes);
		if (accentIndex === -1) {
			return { before: word.text, accent: '', after: '' };
		}

		return {
			before: graphemes.slice(0, accentIndex).join(''),
			accent: graphemes[accentIndex] || '',
			after: graphemes.slice(accentIndex + 1).join('')
		};
	}

	function updateWpm(newWpm: number) {
		settings = { ...settings, wpm: newWpm };
		readerSettings.updateSpeedReader({ wpm: newWpm });
	}

	function updateSetting(key: string, value: any) {
		settings = { ...settings, [key]: value };
		if (key === 'theme') {
			readerSettings.updateReaderTheme(value);
		} else {
			readerSettings.updateSpeedReader({ [key]: value });
		}
	}

	function addReaderTheme() {
		const name = window.prompt('Theme name');
		if (!name?.trim()) return;
		const foreground = window.prompt('Text color', readerTheme.text);
		if (!foreground?.trim()) return;
		const background = window.prompt('Background color', readerTheme.bg);
		if (!background?.trim()) return;

		const id = generateId();
		addCustomTheme({
			id,
			name: name.trim(),
			foreground: foreground.trim(),
			background: background.trim()
		});
		updateSetting('theme', id);
	}

	function deleteReaderTheme(themeId: string, themeName: string) {
		if (!window.confirm(`Remove theme "${themeName}"?`)) return;
		removeCustomTheme(themeId);
		if (settings.theme === themeId) {
			updateSetting('theme', 'catppuccin');
		}
	}

	function formatProgress(): string {
		const percent = words.length > 0 ? Math.round((currentIndex / words.length) * 100) : 0;
		return `${percent}%`;
	}

	function formatWordProgress(): string {
		if (words.length <= 0) return '0 / 0 • 0%';
		const currentWord = Math.min(currentIndex + 1, words.length);
		return `${currentWord.toLocaleString()} / ${words.length.toLocaleString()}`;
	}

	function openWpmMenu(e: MouseEvent) {
		e.stopPropagation();
		showWpmMenu = true;
	}

	function closeWpmMenu(e?: Event) {
		e?.stopPropagation();
		showWpmMenu = false;
	}

	function toggleFullscreen() {
		if (!document.fullscreenElement) {
			document.documentElement.requestFullscreen().catch(console.error);
		} else {
			document.exitFullscreen().catch(console.error);
		}
	}

	function startCloseBackgroundTasks() {
		if (closeTasksStarted) return;
		closeTasksStarted = true;
		stop();
		void saveProgress(true);
		void endSession(true);
	}

	function getSafeReturnPath(value: string | null) {
		if (!value) return null;
		if (!value.startsWith('/') || value.startsWith('//')) return null;
		if (value.startsWith('/login') || value.includes('/reader/')) return null;
		return value;
	}

	function getReaderReturnUrl() {
		const queryReturnTo = getSafeReturnPath($page.url.searchParams.get('returnTo'));
		if (queryReturnTo) return queryReturnTo;

		if (browser && document.referrer) {
			try {
				const referrer = new URL(document.referrer);
				if (referrer.origin === window.location.origin) {
					const referrerPath = getSafeReturnPath(`${referrer.pathname}${referrer.search}${referrer.hash}`);
					if (referrerPath) return referrerPath;
				}
			} catch {
				// Ignore invalid referrers and fall back to the book route.
			}
		}

		return book ? `/book/${book.id}` : '/book';
	}

	function closeReader(e?: Event) {
		e?.preventDefault();
		const targetUrl = getReaderReturnUrl();
		startCloseBackgroundTasks();
		void goto(targetUrl, { replaceState: true });
	}

	// Scroll to the pending word in the word picker, debounced so slider drags don't thrash
	$effect(() => {
		if (!showWordPicker) return;
		const idx = wordPickerPending;
		const timer = setTimeout(() => {
			tick().then(() => {
				const el = document.getElementById(`wk-${idx}`);
				if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
			});
		}, 120);
		return () => clearTimeout(timer);
	});

	$effect(() => {
		currentIndex;
		settings.wordSize;
		settings.fontFamily;
		settings.fontWeight;
		settings.letterSpacing;
		settings.accentEnabled;

		tick().then(() => {
			accentWidth = accentCharEl?.getBoundingClientRect().width ?? 0;
		});
	});
</script>

<svelte:head>
	<title>{readerTitle === 'Loading...' ? 'Speed Reader' : readerTitle} - Cryptorum</title>
	<link rel="stylesheet" href="/fonts/spectral.css" />
</svelte:head>

	<div
		bind:this={containerEl}
		class="speed-reader-root fixed inset-0 z-50 flex flex-col select-none"
			style="--speed-reader-bg: {readerTheme.bg}; --speed-reader-chrome: {readerTheme.text}; background-color: {readerTheme.bg}; color: {readerTheme.text};"
		role="application"
		aria-label="Speed Reader"
	>
	<!-- Top Bar -->
	<header
		class="top-nav transition-opacity duration-200 {showControls ? 'opacity-100' : 'opacity-0 pointer-events-none'}"
	>
		<div class="nav-left">
			<a href={getReaderReturnUrl()} onclick={closeReader} class="nav-btn nav-close" title="Close">
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<line x1="18" y1="6" x2="6" y2="18"></line>
					<line x1="6" y1="6" x2="18" y2="18"></line>
				</svg>
			</a>
		</div>

			<div class="nav-center">
				<span class="book-title">{readerTitle}</span>
			</div>

		<div class="nav-right">
				<button
					type="button"
					onclick={(e) => { e.stopPropagation(); showSettings = !showSettings; }}
					data-settings-button
					class="nav-btn"
				class:active={showSettings}
				title="Settings"
			>
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="3"></circle>
					<path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
				</svg>
			</button>
				<button
					type="button"
					onclick={(e) => { e.stopPropagation(); toggleFullscreen(); }}
					class="nav-btn"
					title="Toggle fullscreen"
			>
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<polyline points="15 3 21 3 21 9"></polyline>
					<polyline points="9 21 3 21 3 15"></polyline>
					<line x1="21" y1="3" x2="14" y2="10"></line>
					<line x1="3" y1="21" x2="10" y2="14"></line>
				</svg>
			</button>
		</div>
	</header>

	<!-- Word Display Area -->
	<div class="flex-1 flex items-center justify-center relative">
		{#if loading}
			<div class="animate-spin rounded-full h-12 w-12 border-b-2" style="border-color: var(--color-primary-500);"></div>
		{:else if words.length > 0}
			<!-- Focus Indicators -->
			{#if settings.focusIndicator !== 'off'}
				{#if settings.focusIndicator === 'lines'}
					{@const barGap = settings.wordSize / 2 + settings.focusIndicatorDistance}
					{@const focalPct = settings.focalPoint * 100}
					{@const barStyle = `background: ${settings.horizontalBarsColor}; opacity: ${settings.horizontalBarsOpacity};`}

					<!-- Top horizontal bar — full width, above word (toggled by horizontalBars) -->
					{#if settings.horizontalBars}
						<div
							class="absolute left-0 right-0 pointer-events-none"
							style="top: calc(50% - {barGap}px); height: 2px; {barStyle}"
						></div>
					{/if}

					<!-- Top vertical stub — drops from top bar toward word (T shape, always shown) -->
					<div
						class="absolute pointer-events-none"
						style="left: calc({focalPct}% - 1px); top: calc(50% - {barGap}px); width: 2px; height: {settings.focusIndicatorLength}px; {barStyle}"
					></div>

					<!-- Bottom horizontal bar — full width, below word (toggled by horizontalBars) -->
					{#if settings.horizontalBars}
						<div
							class="absolute left-0 right-0 pointer-events-none"
							style="top: calc(50% + {barGap}px); height: 2px; {barStyle}"
						></div>
					{/if}

					<!-- Bottom vertical stub — rises from bottom bar toward word (inverted T, always shown) -->
					<div
						class="absolute pointer-events-none"
						style="left: calc({focalPct}% - 1px); top: calc(50% + {barGap - settings.focusIndicatorLength}px); width: 2px; height: {settings.focusIndicatorLength}px; {barStyle}"
					></div>
				{:else if settings.focusIndicator === 'arrows'}
					{@const focalPct = settings.focalPoint * 100}
					{@const arrowGap = settings.wordSize / 2 + settings.focusIndicatorDistance}

					<!-- Up arrow pointing down toward word -->
					<div
						class="absolute pointer-events-none"
						style="left: calc({focalPct}% - 10px); top: calc(50% - {arrowGap + 16}px);"
					>
						<svg width="20" height="16" viewBox="0 0 20 16" fill="{settings.horizontalBarsColor}" opacity="{settings.horizontalBarsOpacity}">
							<path d="M10 16 L0 0 L20 0 Z"/>
						</svg>
					</div>

					<!-- Down arrow pointing up toward word -->
					<div
						class="absolute pointer-events-none"
						style="left: calc({focalPct}% - 10px); top: calc(50% + {arrowGap}px);"
					>
						<svg width="20" height="16" viewBox="0 0 20 16" fill="{settings.horizontalBarsColor}" opacity="{settings.horizontalBarsOpacity}">
							<path d="M10 0 L0 16 L20 16 Z"/>
						</svg>
					</div>
				{/if}
			{/if}

			<!-- Word -->
			{@const wordParts = getWordParts(words[currentIndex])}
			<div
				bind:this={wordContainerEl}
				class="absolute speed-word-stage"
				style="--focal-point: {settings.focalPoint * 100}%; --accent-half: {accentWidth / 2}px; top: 50%;"
			>
				{#if settings.centerWord}
					<p
						class="speed-word speed-word-centered"
						style="font-family: {currentFontFamily}; font-size: {settings.wordSize}px; font-weight: {settings.fontWeight}; letter-spacing: {settings.letterSpacing}px;"
					>
						<span>{wordParts.before}</span><span
							bind:this={accentCharEl}
							class="accent-char"
							style={settings.accentEnabled ? `color: ${settings.accentColor}; opacity: ${settings.accentOpacity};` : ''}
						>{wordParts.accent}</span><span>{wordParts.after}</span>
					</p>
				{:else}
					<p
						class="speed-word speed-word-orp"
						style="font-family: {currentFontFamily}; font-size: {settings.wordSize}px; font-weight: {settings.fontWeight}; letter-spacing: {settings.letterSpacing}px;"
					>
						<span class="speed-word-before">{wordParts.before}</span>
						<span
							bind:this={accentCharEl}
							class="accent-char speed-word-accent"
							style={settings.accentEnabled ? `color: ${settings.accentColor}; opacity: ${settings.accentOpacity};` : ''}
						>{wordParts.accent}</span>
						<span class="speed-word-after">{wordParts.after}</span>
					</p>
				{/if}
			</div>
		{:else}
			<p class="max-w-xl px-6 text-center text-white/60">{loadError || 'No text available for speed reading'}</p>
		{/if}
	</div>

	<!-- Bottom Bar -->
	<footer
		class="speed-footer absolute bottom-0 left-0 right-0 transition-opacity duration-200 {showControls ? 'opacity-100' : 'opacity-0 pointer-events-none'}"
	>
		<div class="speed-footer-inner">
			<div class="speed-playback-controls">
				<button
					type="button"
					onclick={prevWord}
					class="speed-transport-button"
					aria-label="Previous word"
					title="Previous word"
				>
					<svg class="speed-transport-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4">
						<polyline points="15 18 9 12 15 6"></polyline>
					</svg>
				</button>

				<button
					type="button"
					onclick={togglePlay}
					class="speed-play-button"
					aria-label={isPlaying ? 'Pause' : 'Play'}
					title={isPlaying ? 'Pause' : 'Play'}
				>
					{#if isPlaying}
						<svg class="speed-play-icon" fill="currentColor" viewBox="0 0 24 24">
							<path d="M6 4h4v16H6V4zm8 0h4v16h-4V4z"></path>
						</svg>
					{:else}
						<svg class="speed-play-icon speed-play-icon-play" fill="currentColor" viewBox="0 0 24 24">
							<path d="M8 5v14l11-7z"></path>
						</svg>
					{/if}
				</button>

				<button
					type="button"
					onclick={nextWord}
					class="speed-transport-button"
					aria-label="Next word"
					title="Next word"
				>
					<svg class="speed-transport-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4">
						<polyline points="9 18 15 12 9 6"></polyline>
					</svg>
				</button>
			</div>

			<div class="speed-status-strip">
				<div class="speed-status-row">
					<div class="speed-progress-line" aria-label="Speed reader progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={words.length > 0 ? Math.round((currentIndex / words.length) * 100) : 0}>
						<div class="speed-progress-fill" style="width: {words.length > 0 ? (currentIndex / words.length) * 100 : 0}%;"></div>
					</div>
					{#if settings.showWordCount}
						<span class="speed-word-count">{formatWordProgress()}</span>
						<span class="speed-progress-separator">•</span>
					{/if}
					<span class="speed-progress-label">{formatProgress()}</span>
					<button
						type="button"
						onclick={openWpmMenu}
						class="speed-status-button"
						title="Playback speed"
					>
						{settings.wpm} wpm
					</button>
					<button
						type="button"
						onclick={(e) => { e.stopPropagation(); openWordPicker(e); }}
						class="speed-status-icon"
						title="Word Picker"
						aria-label="Word Picker"
					>
						<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h7"></path>
						</svg>
					</button>
				</div>
			</div>
		</div>
	</footer>

	<!-- WPM Menu Popup -->
		{#if showWpmMenu}
			<div
				class="speed-sheet-backdrop"
				role="presentation"
				onclick={closeWpmMenu}
			>
				<div
					bind:this={wpmMenuRef}
					class="speed-wpm-sheet"
					role="dialog"
					aria-modal="true"
					aria-label="Playback speed"
					tabindex="-1"
					onclick={(e) => e.stopPropagation()}
					onkeydown={(e) => { if (e.key === 'Escape') closeWpmMenu(e); }}
					style="background-color: {readerTheme.bg}; border-color: {readerTheme.text}22;"
				>
					<div class="speed-wpm-value-row">
					<button
						type="button"
						onclick={() => updateWpm(Math.max(50, settings.wpm - 100))}
							class="speed-wpm-step-button speed-wpm-step-edge"
							style="background-color: {readerTheme.text}18; color: {readerTheme.text};"
					>
						-100
					</button>
					<button
						type="button"
						onclick={() => updateWpm(Math.max(50, settings.wpm - 50))}
							class="speed-wpm-step-button speed-wpm-step-inner speed-wpm-step-minus"
						style="background-color: {readerTheme.text}18; color: {readerTheme.text};"
					>
						-50
					</button>
					<span class="speed-wpm-value" style="color: {readerTheme.text};">{settings.wpm}</span>
					<button
						type="button"
						onclick={() => updateWpm(Math.min(1200, settings.wpm + 50))}
							class="speed-wpm-step-button speed-wpm-step-inner speed-wpm-step-plus"
						style="background-color: {readerTheme.text}18; color: {readerTheme.text};"
					>
						+50
					</button>
					<button
						type="button"
						onclick={() => updateWpm(Math.min(1200, settings.wpm + 100))}
							class="speed-wpm-step-button speed-wpm-step-edge"
						style="background-color: {readerTheme.text}18; color: {readerTheme.text};"
					>
						+100
					</button>
				</div>

					<div class="speed-wpm-slider-row">
					<button
						type="button"
						onclick={() => updateWpm(Math.max(50, settings.wpm - 10))}
							class="speed-wpm-nudge-button"
						style="background-color: {readerTheme.text}14; color: {readerTheme.text};"
						aria-label="Decrease speed"
					>
						-
					</button>
					<input
						type="range"
						min="50"
						max="1200"
						step="10"
						value={settings.wpm}
						oninput={(e) => updateWpm(parseInt(e.currentTarget.value))}
						class="speed-wpm-slider"
						style="background-color: {readerTheme.text}20;"
					/>
					<button
						type="button"
						onclick={() => updateWpm(Math.min(1200, settings.wpm + 10))}
							class="speed-wpm-nudge-button"
						style="background-color: {readerTheme.text}14; color: {readerTheme.text};"
						aria-label="Increase speed"
					>
						+
					</button>
				</div>
			</div>
		</div>
	{/if}

	<!-- Word Picker Panel -->
		{#if showWordPicker}
			<div
				class="speed-word-picker-modal"
				style="background-color: {readerTheme.bg};"
				role="dialog"
				aria-modal="true"
				aria-label="Word picker"
				tabindex="0"
				onkeydown={(e) => { if (e.key === 'Escape') cancelWordPicker(e); }}
				onclick={(e) => { if (e.target === e.currentTarget) cancelWordPicker(e); }}
			>
			<!-- Header -->
			<div class="flex-shrink-0 flex items-center justify-between px-5 py-4 border-b" style="border-color: {readerTheme.text}20;">
				<div>
					<h3 class="font-semibold text-base" style="color: {readerTheme.text};">Jump to Position</h3>
					<p class="text-xs mt-0.5" style="color: {readerTheme.text}60;">
						Tap a word to select it, then confirm
					</p>
				</div>
					<button type="button" onclick={(e) => cancelWordPicker(e)} class="speed-word-picker-close" style="color: {readerTheme.text}80;" aria-label="Close word picker" title="Close word picker">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
					</svg>
				</button>
			</div>

			<!-- Legend -->
			<div class="flex-shrink-0 flex items-center gap-4 px-5 py-2 text-xs border-b" style="border-color: {readerTheme.text}10; color: {readerTheme.text}60;">
				<span class="flex items-center gap-1.5">
					<span class="inline-block w-3 h-3 rounded-sm" style="background-color: var(--color-primary-500);"></span>
					Current position
				</span>
				<span class="flex items-center gap-1.5">
					<span class="inline-block w-3 h-3 rounded-sm border-2" style="border-color: var(--color-primary-400);"></span>
					Selected destination
				</span>
			</div>

			<!-- Text content -->
			<div class="flex-1 overflow-y-auto px-6 py-5">
				{#if words.length === 0}
					<div class="speed-word-picker-loading" style="color: {readerTheme.text}80;">
						<div class="speed-word-picker-spinner" style="border-color: {readerTheme.text}24; border-bottom-color: {readerTheme.text};"></div>
						<span>Preparing word list...</span>
					</div>
				{:else}
					<div class="speed-word-picker-window-note" style="color: {readerTheme.text}60;">
						Showing words {(wordPickerVisibleStart + 1).toLocaleString()}-{wordPickerVisibleEnd.toLocaleString()} of {words.length.toLocaleString()}
					</div>
					{#each wordPickerVisibleParagraphs as para}
						<p class="mb-5 leading-loose text-base select-none" style="color: {readerTheme.text}; font-family: Georgia, serif;">
							{#each words.slice(para.start, para.end) as word, j}
								{@const idx = para.start + j}
								{@const isOrigin = idx === wordPickerOrigin}
								{@const isPending = idx === wordPickerPending}
									<span
										id="wk-{idx}"
										onclick={() => { wordPickerPending = idx; }}
										role="button"
										tabindex="0"
										onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); wordPickerPending = idx; } }}
										class="cursor-pointer rounded px-0.5 py-px transition-colors {
										isOrigin && isPending
											? 'text-white'
											: isOrigin
												? 'text-white'
												: isPending
													? 'outline outline-2'
													: 'hover:bg-white/10'
									}"
									style="{
										isOrigin && isPending
											? 'background-color: var(--color-primary-500);'
											: isOrigin
												? 'background-color: var(--color-primary-500);'
												: isPending
													? 'outline-color: var(--color-primary-400);'
													: ''
									}"
								>{word.text}</span>&#8203;{' '}
							{/each}
						</p>
						{/each}
				{/if}
			</div>

			<!-- Seek bar -->
			<div class="flex-shrink-0 px-5 py-3 border-t" style="border-color: {readerTheme.text}20;">
				<input
					type="range"
					min="0"
					max={words.length - 1}
					value={wordPickerPending}
					oninput={(e) => { wordPickerPending = parseInt(e.currentTarget.value); }}
					class="w-full h-2 rounded-lg appearance-none cursor-pointer"
					style="background-color: {readerTheme.text}20;"
				/>
				<div class="flex justify-between text-xs mt-1.5" style="color: {readerTheme.text}50;">
					<span>Start</span>
					<span class="font-mono" style="color: {readerTheme.text}80;">
						Word {wordPickerPending + 1} / {words.length}
						({Math.round((wordPickerPending / Math.max(1, words.length - 1)) * 100)}%)
					</span>
					<span>End</span>
				</div>
			</div>

			<!-- Actions -->
			<div class="flex-shrink-0 flex gap-3 px-5 py-4 border-t" style="border-color: {readerTheme.text}20;">
					<button
						type="button"
						onclick={(e) => cancelWordPicker(e)}
						class="speed-word-picker-action speed-word-picker-action-secondary"
					style="background-color: {readerTheme.text}15; color: {readerTheme.text};"
				>
					Cancel
				</button>
					<button
						type="button"
						onclick={(e) => confirmWordPicker(e)}
						class="speed-word-picker-action speed-word-picker-action-primary"
					style="background-color: var(--color-primary-500); color: white;"
				>
					Start Here
				</button>
			</div>
		</div>
	{/if}

	<!-- Settings Panel -->
	{#if showSettings}
		<div
			bind:this={settingsPanelRef}
			class="fixed right-0 w-[480px] shadow-xl z-[60] flex flex-col transform transition-transform duration-300"
			style="
				top: var(--speed-reader-top-bar-height);
				height: calc(100vh - var(--speed-reader-top-bar-height));
				background-color: {readerTheme.bg}f2;
				border-left: 1px solid {readerTheme.text}26;
				color: {readerTheme.text};
				backdrop-filter: blur(18px);
				--color-surface-text: {readerTheme.text};
				--color-surface-text-muted: {readerTheme.text}99;
				--color-surface-border: {readerTheme.text}26;
				--color-surface-base: {readerTheme.text}0f;
				--color-surface-overlay: {readerTheme.bg}f2;
				--color-surface-700: {readerTheme.text}18;
				--color-surface-600: {readerTheme.text}24;
			"
		>
			<div class="flex-1 overflow-y-auto p-4 space-y-6 custom-scrollbar">
				<div class="grid grid-cols-3 gap-2 rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-1">
					<button
						type="button"
						onclick={() => activeSettingsTab = 'reading'}
						class="rounded-lg px-3 py-2 text-sm font-medium transition-colors {activeSettingsTab === 'reading' ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
					>
						Reading
					</button>
					<button
						type="button"
						onclick={() => activeSettingsTab = 'typography'}
						class="rounded-lg px-3 py-2 text-sm font-medium transition-colors {activeSettingsTab === 'typography' ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
					>
						Typography
					</button>
					<button
						type="button"
						onclick={() => activeSettingsTab = 'focus'}
						class="rounded-lg px-3 py-2 text-sm font-medium transition-colors {activeSettingsTab === 'focus' ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
					>
						Focus
					</button>
				</div>

				{#if activeSettingsTab === 'typography'}
					<!-- Theme -->
					<div>
						<div class="text-sm font-medium block mb-2" style="color: var(--color-surface-text);">Theme</div>
					<div class="grid grid-cols-2 gap-2">
						{#each speedReaderThemes as theme}
							<button
								onclick={() => updateSetting('theme', theme.id)}
								class="flex flex-col items-center p-2 rounded-lg border-2 transition-all text-sm {settings.theme === theme.id ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] hover:border-[var(--color-surface-500)]'}"
							>
								<ThemePreviewSwatch background={theme.bg} foreground={theme.text} sizeClass="h-8 w-8 mb-1" />
								<span class="text-xs text-[var(--color-surface-text)]">{theme.name}</span>
							</button>
						{/each}
						{#if appTheme?.appearance.customThemes?.length}
							{#each appTheme.appearance.customThemes as customTheme}
								<div class="relative">
									<button
										onclick={() => updateSetting('theme', customTheme.id)}
										class="flex w-full flex-col items-center p-2 rounded-lg border-2 transition-all text-sm {settings.theme === customTheme.id ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] hover:border-[var(--color-surface-500)]'}"
									>
										<ThemePreviewSwatch background={customTheme.background} foreground={customTheme.foreground} sizeClass="h-8 w-8 mb-1" />
										<span class="text-xs text-[var(--color-surface-text)]">{customTheme.name}</span>
									</button>
									<button
										type="button"
										onclick={() => deleteReaderTheme(customTheme.id, customTheme.name)}
										class="absolute right-1 top-1 flex h-6 w-6 items-center justify-center rounded-full bg-[var(--color-surface-base)] text-sm text-[var(--color-surface-text-muted)] shadow hover:text-[var(--color-surface-text)]"
										aria-label="Remove {customTheme.name}"
									>
										×
									</button>
								</div>
							{/each}
						{/if}
						<button
							type="button"
							onclick={addReaderTheme}
							class="flex min-h-[72px] flex-col items-center justify-center rounded-lg border-2 border-dashed border-[var(--color-surface-border)] p-2 text-sm text-[var(--color-surface-text-muted)] transition-colors hover:border-[var(--color-primary-500)] hover:text-[var(--color-surface-text)]"
						>
							<span class="text-xl leading-none">+</span>
							<span class="text-xs">Add Theme</span>
						</button>
					</div>
					<p class="text-xs mt-1" style="color: var(--color-surface-text-muted);">Applies to the reading background and foreground.</p>
				</div>

					<!-- Font Family -->
					<div>
						<div class="text-sm font-medium block mb-2" style="color: var(--color-surface-text);">Font Family</div>
					<div class="grid grid-cols-2 gap-2">
						{#each fontFamilies as font}
							<button
								onclick={() => updateSetting('fontFamily', font.id)}
								class="px-3 py-2 rounded-lg border transition-all text-sm truncate {settings.fontFamily === font.id ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-base)]'}"
								style="color: var(--color-surface-text); font-family: {font.family};"
							>
								{font.name}
							</button>
						{/each}
					</div>
				</div>

					<!-- Font Thickness -->
					<div>
						<div class="text-sm font-medium block mb-2" style="color: var(--color-surface-text);">Font Thickness</div>
					<div class="grid grid-cols-2 gap-2">
						{#each fontWeightOptions as option}
							<button
								onclick={() => updateSetting('fontWeight', option.value)}
								class="px-3 py-2 rounded-lg border transition-all text-sm {settings.fontWeight === option.value ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-base)]'}"
								style="color: var(--color-surface-text); font-family: {currentFontFamily}; font-weight: {option.value};"
							>
								{option.label}
							</button>
						{/each}
					</div>
				</div>

					<!-- Letter Spacing -->
					<div>
						<div class="flex justify-between mb-2">
							<div class="text-sm font-medium" style="color: var(--color-surface-text);">Letter Spacing</div>
						<span class="text-sm" style="color: var(--color-surface-text-muted);">{settings.letterSpacing}px</span>
					</div>
					<input
						type="range"
						min="-2"
						max="10"
						step="0.5"
						value={settings.letterSpacing}
						oninput={(e) => updateSetting('letterSpacing', parseFloat(e.currentTarget.value))}
						class="w-full h-2 rounded-lg appearance-none cursor-pointer"
						style="background: var(--color-surface-700);"
					/>
				</div>

				{:else if activeSettingsTab === 'focus'}
					<!-- Focal Point -->
					<div>
						<div class="flex justify-between mb-2">
							<div class="text-sm font-medium" style="color: var(--color-surface-text);">Focal Point</div>
						<span class="text-sm" style="color: var(--color-surface-text-muted);">{(settings.focalPoint * 100).toFixed(0)}%</span>
					</div>
					<input
						type="range"
						min="20"
						max="80"
						value={settings.focalPoint * 100}
						oninput={(e) => updateSetting('focalPoint', parseInt(e.currentTarget.value) / 100)}
						class="w-full h-2 rounded-lg appearance-none cursor-pointer"
						style="background: var(--color-surface-700);"
					/>
					<p class="text-xs mt-1" style="color: var(--color-surface-text-muted);">Position of accent character on screen</p>
				</div>

					<!-- Center Word Toggle -->
						<div class="flex items-center justify-between">
						<div>
							<div class="text-sm font-medium block" style="color: var(--color-surface-text);">Center Word</div>
							<p class="text-xs" style="color: var(--color-surface-text-muted);">Center entire word instead of focal point</p>
						</div>
						<button
							type="button"
							onclick={() => updateSetting('centerWord', !settings.centerWord)}
							class="relative w-12 h-6 rounded-full transition-colors {settings.centerWord ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-surface-700)]'}"
							aria-label={settings.centerWord ? 'Disable center word' : 'Enable center word'}
							title={settings.centerWord ? 'Disable center word' : 'Enable center word'}
						>
						<span
							class="absolute top-1 w-4 h-4 bg-white rounded-full transition-transform {settings.centerWord ? 'left-7' : 'left-1'}"
						></span>
					</button>
				</div>

					<!-- Accent Color -->
					<div>
						<div class="text-sm font-medium block mb-2" style="color: var(--color-surface-text);">Accent Color</div>
					<div class="flex items-center space-x-3">
						<input
							type="color"
							value={settings.accentColor}
							oninput={(e) => updateSetting('accentColor', e.currentTarget.value)}
							class="w-12 h-12 rounded-lg cursor-pointer border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]"
						/>
						<input
							type="text"
							value={settings.accentColor}
							oninput={(e) => updateSetting('accentColor', e.currentTarget.value)}
							class="flex-1 px-3 py-2 rounded-lg font-mono text-sm"
							style="background-color: var(--color-surface-700); border: 1px solid var(--color-surface-border); color: var(--color-surface-text);"
						/>
					</div>
				</div>

					<!-- Accent Toggle -->
						<div class="flex items-center justify-between">
						<div>
							<div class="text-sm font-medium block" style="color: var(--color-surface-text);">Accent Character</div>
							<p class="text-xs" style="color: var(--color-surface-text-muted);">Highlight the focal character</p>
						</div>
						<button
							type="button"
							onclick={() => updateSetting('accentEnabled', !settings.accentEnabled)}
							class="relative w-12 h-6 rounded-full transition-colors {settings.accentEnabled ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-surface-700)]'}"
							aria-label={settings.accentEnabled ? 'Disable accent character' : 'Enable accent character'}
							title={settings.accentEnabled ? 'Disable accent character' : 'Enable accent character'}
						>
						<span
							class="absolute top-1 w-4 h-4 bg-white rounded-full transition-transform {settings.accentEnabled ? 'left-7' : 'left-1'}"
						></span>
					</button>
				</div>

					<!-- Focus Indicator -->
					<div>
						<div class="text-sm font-medium block mb-2" style="color: var(--color-surface-text);">Focus Indicator</div>
					<div class="flex space-x-2">
						{#each [['off', 'Off'], ['lines', 'Lines'], ['arrows', 'Arrows']] as [value, label]}
							<button
								onclick={() => updateSetting('focusIndicator', value)}
								class="flex-1 px-3 py-2 rounded-lg border transition-all text-sm {settings.focusIndicator === value ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/20' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-base)]'}"
								style="color: var(--color-surface-text);"
							>
								{label}
							</button>
						{/each}
					</div>
				</div>

					<!-- Focus Indicator Distance -->
					<div>
						<div class="flex justify-between mb-2">
							<div class="text-sm font-medium" style="color: var(--color-surface-text);">Focus Distance</div>
						<span class="text-sm" style="color: var(--color-surface-text-muted);">{settings.focusIndicatorDistance}px</span>
					</div>
					<input
						type="range"
						min="5"
						max="200"
						step="5"
						value={settings.focusIndicatorDistance}
						oninput={(e) => updateSetting('focusIndicatorDistance', parseInt(e.currentTarget.value))}
						class="w-full h-2 rounded-lg appearance-none cursor-pointer"
						style="background-color: var(--color-surface-700);"
					/>
				</div>

					<!-- Focus Indicator Length -->
					<div>
						<div class="flex justify-between mb-2">
							<div class="text-sm font-medium" style="color: var(--color-surface-text);">Indicator Length</div>
						<span class="text-sm" style="color: var(--color-surface-text-muted);">{settings.focusIndicatorLength}px</span>
					</div>
					<input
						type="range"
						min="2"
						max="80"
						step="2"
						value={settings.focusIndicatorLength}
						oninput={(e) => updateSetting('focusIndicatorLength', parseInt(e.currentTarget.value))}
						class="w-full h-2 rounded-lg appearance-none cursor-pointer"
						style="background-color: var(--color-surface-700);"
					/>
					<p class="text-xs mt-1" style="color: var(--color-surface-text-muted);">Length of the vertical T-bar stubs</p>
				</div>

					<!-- Horizontal Bars Toggle -->
						<div class="flex items-center justify-between">
						<div>
							<div class="text-sm font-medium block" style="color: var(--color-surface-text);">Horizontal Bars</div>
							<p class="text-xs" style="color: var(--color-surface-text-muted);">Show focus guide lines</p>
						</div>
						<button
							type="button"
							onclick={() => updateSetting('horizontalBars', !settings.horizontalBars)}
							class="relative w-12 h-6 rounded-full transition-colors {settings.horizontalBars ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-surface-700)]'}"
							aria-label={settings.horizontalBars ? 'Hide horizontal bars' : 'Show horizontal bars'}
							title={settings.horizontalBars ? 'Hide horizontal bars' : 'Show horizontal bars'}
						>
						<span
							class="absolute top-1 w-4 h-4 bg-white rounded-full transition-transform {settings.horizontalBars ? 'left-7' : 'left-1'}"
						></span>
					</button>
				</div>

				{:else}
					<!-- Reading Speed -->
					<div>
						<div class="flex justify-between mb-2">
							<div class="text-sm font-medium" style="color: var(--color-surface-text);">Words Per Minute</div>
							<span class="text-sm" style="color: var(--color-surface-text-muted);">{settings.wpm}</span>
						</div>
						<input
							type="range"
							min="100"
							max="1000"
							step="10"
							value={settings.wpm}
							oninput={(e) => updateWpm(parseInt(e.currentTarget.value))}
							class="w-full h-2 rounded-lg appearance-none cursor-pointer"
							style="background-color: var(--color-surface-700);"
						/>
					</div>

					<!-- Word Count Toggle -->
					<div class="flex items-center justify-between">
						<div>
							<div class="text-sm font-medium block" style="color: var(--color-surface-text);">Show Word Count</div>
							<p class="text-xs" style="color: var(--color-surface-text-muted);">Show current word and total words beside progress</p>
						</div>
						<button
							type="button"
							onclick={() => updateSetting('showWordCount', !settings.showWordCount)}
							class="relative w-12 h-6 rounded-full transition-colors {settings.showWordCount ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-surface-700)]'}"
							aria-label={settings.showWordCount ? 'Hide word count' : 'Show word count'}
							title={settings.showWordCount ? 'Hide word count' : 'Show word count'}
						>
							<span
								class="absolute top-1 w-4 h-4 bg-white rounded-full transition-transform {settings.showWordCount ? 'left-7' : 'left-1'}"
							></span>
						</button>
					</div>

					<!-- Automatic Sentence Pause -->
						<div class="flex items-center justify-between">
						<div>
							<div class="text-sm font-medium block" style="color: var(--color-surface-text);">Auto Sentence Pause</div>
							<p class="text-xs" style="color: var(--color-surface-text-muted);">Calculate pause based on WPM</p>
						</div>
						<button
							type="button"
							onclick={() => updateSetting('autoSentencePause', !settings.autoSentencePause)}
							class="relative w-12 h-6 rounded-full transition-colors {settings.autoSentencePause ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-surface-700)]'}"
							aria-label={settings.autoSentencePause ? 'Disable automatic sentence pause' : 'Enable automatic sentence pause'}
							title={settings.autoSentencePause ? 'Disable automatic sentence pause' : 'Enable automatic sentence pause'}
						>
						<span
							class="absolute top-1 w-4 h-4 bg-white rounded-full transition-transform {settings.autoSentencePause ? 'left-7' : 'left-1'}"
						></span>
					</button>
				</div>

					<!-- Manual Sentence Pause (disabled when auto is on) -->
					<div class:opacity-50={settings.autoSentencePause}>
						<div class="flex justify-between mb-2">
							<div class="text-sm font-medium" style="color: var(--color-surface-text);">Sentence Pause</div>
						<span class="text-sm" style="color: var(--color-surface-text-muted);">{settings.sentencePause}ms</span>
					</div>
					<input
						type="range"
						min="50"
						max="1000"
						step="50"
						value={settings.sentencePause}
						oninput={(e) => updateSetting('sentencePause', parseInt(e.currentTarget.value))}
						disabled={settings.autoSentencePause}
						class="w-full h-2 rounded-lg appearance-none cursor-pointer disabled:cursor-not-allowed"
						style="background-color: var(--color-surface-700);"
					/>
				</div>

					<!-- Word Size -->
					<div>
						<div class="flex justify-between mb-2">
							<div class="text-sm font-medium" style="color: var(--color-surface-text);">Word Size</div>
						<span class="text-sm" style="color: var(--color-surface-text-muted);">{settings.wordSize}px</span>
					</div>
					<input
						type="range"
						min="24"
						max="144"
						step="4"
						value={settings.wordSize}
						oninput={(e) => updateSetting('wordSize', parseInt(e.currentTarget.value))}
						class="w-full h-2 rounded-lg appearance-none cursor-pointer"
						style="background-color: var(--color-surface-700);"
					/>
				</div>

				{/if}

				<!-- Reset -->
				<div class="pt-4 border-t" style="border-color: var(--color-surface-border);">
					<button
						onclick={() => readerSettings.resetToDefaults('speedReader')}
						class="w-full px-4 py-2 rounded-lg transition-colors"
						style="background-color: var(--color-surface-700); color: var(--color-surface-text);"
					>
						Reset to Defaults
					</button>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.speed-reader-root {
		--speed-reader-top-bar-height: 56px;
	}

	.top-nav {
		position: relative;
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		column-gap: 16px;
		height: var(--speed-reader-top-bar-height);
		padding: 0 clamp(12px, 3vw, 32px);
		color: var(--speed-reader-chrome, currentColor);
		flex-shrink: 0;
		z-index: 100;
	}

	.speed-footer {
		z-index: 50;
		padding: 0 clamp(12px, 4vw, 48px) calc(14px + env(safe-area-inset-bottom));
		color: var(--speed-reader-chrome, currentColor);
	}

	.speed-footer-inner {
		width: min(100%, 920px);
		margin: 0 auto;
	}

	.speed-playback-controls {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: clamp(4px, 1.2vw, 10px);
		padding: 0 0 24px;
		transform: translateY(-12px);
	}

	.speed-transport-button,
	.speed-play-button {
		display: flex;
		align-items: center;
		justify-content: center;
		border: 0;
		border-radius: 999px;
		background: transparent;
		color: currentColor;
		cursor: pointer;
		transition: background-color 0.15s ease, opacity 0.15s ease, transform 0.15s ease;
	}

	.speed-transport-button {
		width: 44px;
		height: 44px;
		opacity: 0.74;
	}

	.speed-play-button {
		width: 156px;
		height: 156px;
		opacity: 0.9;
	}

	.speed-transport-button:hover,
	.speed-play-button:hover,
	.speed-transport-button:focus-visible,
	.speed-play-button:focus-visible {
		background: color-mix(in srgb, currentColor 12%, transparent);
		opacity: 1;
	}

	.speed-play-button:hover,
	.speed-play-button:focus-visible {
		transform: scale(1.03);
	}

	.speed-transport-button:focus-visible,
	.speed-play-button:focus-visible {
		outline: 2px solid currentColor;
		outline-offset: 3px;
	}

	.speed-transport-icon {
		width: 26px;
		height: 26px;
	}

	.speed-play-icon {
		width: 84px;
		height: 84px;
	}

	.speed-play-icon-play {
		margin-left: 8px;
	}

	.speed-status-strip {
		width: 100%;
		padding: 0;
	}

	.speed-progress-line {
		flex: 1 1 auto;
		position: relative;
		height: 4px;
		overflow: hidden;
		border-radius: 999px;
		background: color-mix(in srgb, currentColor 18%, transparent);
	}

	.speed-progress-fill {
		height: 100%;
		border-radius: inherit;
		background: currentColor;
		opacity: 0.72;
		transition: width 0.1s ease;
	}

	.speed-status-row {
		display: flex;
		align-items: center;
		gap: 10px;
		color: currentColor;
		font-size: 13px;
		font-weight: 500;
		font-variant-numeric: tabular-nums;
		opacity: 0.82;
	}

	.speed-word-count {
		flex: 0 1 auto;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.speed-progress-separator {
		margin: 0 4px;
		opacity: 0.65;
	}

	.speed-progress-label {
		flex: 0 0 auto;
		min-width: 0;
		overflow: hidden;
		text-align: right;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.speed-status-button,
	.speed-status-icon {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 32px;
		border: 0;
		border-radius: 6px;
		background: transparent;
		color: currentColor;
		cursor: pointer;
		transition: background-color 0.15s;
	}

	.speed-status-button {
		flex: 0 0 auto;
		padding: 0 10px;
		font: inherit;
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
	}

	.speed-status-icon {
		flex: 0 0 auto;
		width: 32px;
	}

	.speed-status-button:hover,
	.speed-status-icon:hover {
		background: color-mix(in srgb, currentColor 20%, transparent);
	}

	.speed-sheet-backdrop {
		position: fixed;
		inset: 0;
		z-index: 130;
		display: flex;
		align-items: flex-end;
		justify-content: center;
		padding: 0 clamp(12px, 4vw, 48px) calc(12px + env(safe-area-inset-bottom));
		background: transparent;
	}

	.speed-wpm-sheet {
		width: min(100%, 560px);
		padding: 18px;
		border: 1px solid;
		border-radius: 18px 18px 0 0;
		box-shadow: 0 -18px 48px rgba(0, 0, 0, 0.28);
		animation: speed-sheet-in 0.16s ease-out;
	}

	.speed-wpm-value-row {
		display: grid;
		grid-template-columns: auto auto minmax(92px, 1fr) auto auto;
		align-items: center;
		column-gap: 12px;
		margin-bottom: 18px;
	}

	.speed-wpm-step-button,
	.speed-wpm-nudge-button {
		border: 0;
		border-radius: 8px;
		cursor: pointer;
		font-weight: 700;
		transition: background-color 0.15s ease, transform 0.15s ease;
	}

	.speed-wpm-step-button {
		min-width: 54px;
		min-height: 38px;
		padding: 0 10px;
		font-size: 13px;
	}

	.speed-wpm-step-minus {
		justify-self: end;
		margin-right: clamp(4px, 1.6vw, 14px);
	}

	.speed-wpm-step-plus {
		justify-self: start;
		margin-left: clamp(4px, 1.6vw, 14px);
	}

	.speed-wpm-value {
		display: block;
		min-width: 0;
		text-align: center;
		font-size: 28px;
		font-weight: 800;
		line-height: 1;
		font-variant-numeric: tabular-nums;
	}

	.speed-wpm-slider-row {
		display: grid;
		grid-template-columns: 48px minmax(0, 1fr) 48px;
		align-items: center;
		gap: 14px;
	}

	.speed-wpm-nudge-button {
		width: 48px;
		height: 48px;
		font-size: 28px;
		line-height: 1;
	}

	.speed-wpm-step-button:hover,
	.speed-wpm-nudge-button:hover {
		transform: translateY(-1px);
	}

	.speed-wpm-slider {
		width: 100%;
		height: 8px;
		border-radius: 999px;
		appearance: none;
		cursor: pointer;
	}

	.speed-word-picker-modal {
		position: fixed;
		inset: 0;
		z-index: 150;
		display: flex;
		flex-direction: column;
		padding-top: env(safe-area-inset-top);
	}

	.speed-word-picker-window-note {
		margin-bottom: 14px;
		font-size: 12px;
		text-align: center;
	}

	.speed-word-picker-loading {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 12px;
		min-height: 220px;
		font-size: 13px;
	}

	.speed-word-picker-spinner {
		width: 34px;
		height: 34px;
		border: 3px solid;
		border-radius: 50%;
		animation: speed-spin 0.8s linear infinite;
	}

	.speed-word-picker-close {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
		border: 0;
		border-radius: 10px;
		background: transparent;
		cursor: pointer;
		transition: background-color 0.16s ease, color 0.16s ease, transform 0.16s ease;
	}

	.speed-word-picker-close:hover,
	.speed-word-picker-close:focus-visible {
		background: color-mix(in srgb, currentColor 12%, transparent);
		color: currentColor !important;
		transform: translateY(-1px);
	}

	.speed-word-picker-close:active,
	.speed-word-picker-action:active {
		transform: translateY(0) scale(0.985);
	}

	.speed-word-picker-action {
		flex: 1;
		padding: 10px 16px;
		border: 0;
		border-radius: 10px;
		font-size: 14px;
		font-weight: 650;
		cursor: pointer;
		transition: background-color 0.16s ease, box-shadow 0.16s ease, filter 0.16s ease, transform 0.16s ease;
	}

	.speed-word-picker-action:hover,
	.speed-word-picker-action:focus-visible {
		transform: translateY(-1px);
	}

	.speed-word-picker-action-secondary:hover,
	.speed-word-picker-action-secondary:focus-visible {
		background-color: color-mix(in srgb, currentColor 22%, transparent) !important;
		box-shadow: 0 8px 20px color-mix(in srgb, currentColor 10%, transparent);
	}

	.speed-word-picker-action-primary:hover,
	.speed-word-picker-action-primary:focus-visible {
		filter: brightness(1.08);
		box-shadow: 0 10px 24px rgba(34, 197, 94, 0.22);
	}

	@keyframes speed-sheet-in {
		from {
			opacity: 0;
			transform: translateY(18px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	@keyframes speed-spin {
		to {
			transform: rotate(360deg);
		}
	}

	.nav-left,
	.nav-center,
	.nav-right {
		display: flex;
		align-items: center;
		gap: 4px;
	}

	.nav-left { min-width: 0; }
	.nav-center {
		justify-content: center;
		min-width: 0;
		text-align: center;
	}
	.nav-right {
		justify-content: flex-end;
		min-width: 0;
	}

	.nav-btn,
	.nav-close {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
		border: none;
		border-radius: 6px;
		background: transparent;
		color: currentColor;
		cursor: pointer;
		transition: background-color 0.15s, color 0.15s;
	}

	.nav-btn:hover,
	.nav-close:hover { background: color-mix(in srgb, currentColor 12%, transparent); }
	.nav-btn:disabled { opacity: 0.3; cursor: not-allowed; }
	.nav-btn.active { background: color-mix(in srgb, currentColor 16%, transparent); color: currentColor; }

	.nav-close { text-decoration: none; }

	@media (max-width: 768px) {
		.top-nav {
			padding: 0 10px;
			column-gap: 8px;
		}

		.speed-status-row {
			gap: 8px;
			font-size: 12px;
		}

		.speed-playback-controls {
			gap: 4px;
			padding-bottom: 20px;
			transform: translateY(-8px);
		}

		.speed-transport-button {
			width: 42px;
			height: 42px;
		}

		.speed-play-button {
			width: 144px;
			height: 144px;
		}

		.speed-play-icon {
			width: 78px;
			height: 78px;
		}

		.speed-wpm-sheet {
			width: min(100%, 520px);
			padding: 16px;
		}

		.speed-wpm-value-row {
			grid-template-columns: repeat(2, minmax(48px, auto)) minmax(64px, 1fr) repeat(2, minmax(48px, auto));
			gap: 8px;
		}

		.speed-wpm-step-minus {
			margin-right: 4px;
		}

		.speed-wpm-step-plus {
			margin-left: 4px;
		}

		.speed-wpm-step-button {
			min-width: 48px;
			padding: 0 8px;
		}

		.nav-left,
		.nav-right {
			gap: 6px;
			min-width: 0;
		}

		.nav-btn,
		.nav-close {
			width: 44px;
			height: 44px;
		}

		.icon {
			width: 22px;
			height: 22px;
		}
	}

	@media (max-width: 520px) {
		.speed-footer {
			padding-inline: 12px;
		}

		.speed-word-count,
		.speed-progress-separator {
			display: none;
		}

		.speed-status-button {
			padding: 0 8px;
		}

		.speed-wpm-value-row {
			grid-template-columns: repeat(2, 1fr);
		}

		.speed-wpm-step-minus,
		.speed-wpm-step-plus {
			justify-self: stretch;
			margin: 0;
		}

		.speed-wpm-value {
			grid-column: 1 / -1;
			grid-row: 1;
			margin-bottom: 4px;
		}

		.speed-wpm-slider-row {
			grid-template-columns: 44px minmax(0, 1fr) 44px;
			gap: 10px;
		}

		.speed-wpm-nudge-button {
			width: 44px;
			height: 44px;
		}
	}

	.book-title {
		color: currentColor;
		font-size: 14px;
		font-weight: 500;
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.icon { width: 20px; height: 20px; }

	.speed-word-stage {
		left: 0;
		right: 0;
		transform: translateY(-50%);
	}

	.speed-word {
		margin: 0;
		white-space: nowrap;
		line-height: 1;
	}

	.speed-word-centered {
		display: flex;
		justify-content: center;
	}

	.speed-word-orp {
		display: grid;
		grid-template-columns: minmax(0, calc(var(--focal-point) - var(--accent-half))) max-content minmax(0, 1fr);
		align-items: baseline;
		width: 100%;
	}

	.speed-word-before {
		justify-self: end;
		min-width: 0;
		overflow: visible;
		text-align: right;
	}

	.speed-word-accent {
		justify-self: center;
	}

	.speed-word-after {
		justify-self: start;
		min-width: 0;
		overflow: visible;
	}

	@media (max-width: 768px) {
		.speed-reader-root {
			--speed-reader-top-bar-height: 72px;
		}
	}
</style>

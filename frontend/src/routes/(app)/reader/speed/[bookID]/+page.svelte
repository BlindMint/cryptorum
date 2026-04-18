<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { browser } from '$app/environment';
	import { readerSettings, speedReaderThemes, fontFamilies, type SpeedReaderSetting } from '$lib/stores/readerSettings';
	import { currentTheme as appThemeStore, resolveThemeColors, type FullTheme } from '$lib/stores/theme';
	import ThemePreviewSwatch from '$lib/components/ThemePreviewSwatch.svelte';
	import { normalizeBookFormat } from '$lib/utils/book-formats';

	interface ProcessedWord {
		text: string;
	}

	let book = $state<any>(null);
	let loading = $state(true);
	let words = $state<ProcessedWord[]>([]);
	let currentIndex = $state(0);
	let isPlaying = $state(false);
	let intervalId: number | null = null;
	let savedProgress = $state<any>(null);
	let showWordPicker = $state(false);
	let showControls = $state(true);
	let showSettings = $state(false);
	let showWpmMenu = $state(false);
	let controlsTimeout: ReturnType<typeof setTimeout> | null = null;
	let containerEl: HTMLDivElement;
	let settingsPanelRef: HTMLDivElement | null = $state(null);
	let wpmMenuRef: HTMLDivElement | null = $state(null);
	let wordContainerEl = $state<HTMLDivElement | null>(null);
	let currentSessionId = $state<number | null>(null);
	let lastWheelNavigationAt = 0;
	let sessionEnded = false;
	let handlePageExit: (() => void) | null = null;

	// Word picker state
	let wordPickerPending = $state(0);   // word the user is about to jump to
	let wordPickerOrigin = $state(0);    // word the user was at when panel opened

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

	let settings = $state<SpeedReaderSetting>({
		wpm: 300,
		wordSize: 64,
		fontFamily: 'serif',
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
		theme: 'dark',
		letterSpacing: 0,
		focusIndicatorLength: 20
	});

	let readerTheme = $state(speedReaderThemes[0]);
	let appTheme = $state<FullTheme | null>(null);
	let wakeLock: WakeLockSentinel | null = null;

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
		settings = { ...s.speedReader };
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
			try {
				const res = await fetch(`/api/books/${bookId}`);
				if (res.ok) {
					book = await res.json();
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
		void endSession(true);
		if (wakeLock) {
			wakeLock.release();
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
			const requestedFormat = normalizeBookFormat($page.url.searchParams.get('format'));
			const res = await fetch(`/api/books/${book.id}/text${requestedFormat ? `?format=${encodeURIComponent(requestedFormat)}` : ''}`);
			if (res.ok) {
				const text = await res.text();
				words = processText(text);
				if (savedProgress && savedProgress.speed_reader_percent > 0) {
					currentIndex = Math.floor((savedProgress.speed_reader_percent / 100) * words.length);
				}
			}
		} catch (e) {
			console.error('Failed to load text:', e);
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

		if (settings.keepScreenOn && browser && 'wakeLock' in navigator) {
			(async () => {
				try {
					wakeLock = await navigator.wakeLock.request('screen');
				} catch (e) {
					console.warn('Wake lock failed:', e);
				}
			})();
		}

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
		if (wakeLock) {
			wakeLock.release();
			wakeLock = null;
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
		wordPickerPending = currentIndex;
		wordPickerOrigin = currentIndex;
		showWordPicker = true;
	}

	function confirmWordPicker() {
		currentIndex = wordPickerPending;
		showWordPicker = false;
	}

	function cancelWordPicker() {
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

	function findAccentCharIndex(word: string): number {
		const len = word.length;
		if (len <= 1) return 0;
		if (len <= 3) return 1;
		if (len <= 5) return 1;
		return Math.floor(len * settings.focalPoint);
	}

	function getWordParts(word: ProcessedWord): { before: string; accent: string; after: string } {
		const text = word.text;
		const accentIndex = findAccentCharIndex(text);
		if (accentIndex === -1) {
			return { before: text, accent: '', after: '' };
		}

		return {
			before: text.substring(0, accentIndex),
			accent: text[accentIndex] || '',
			after: text.substring(accentIndex + 1)
		};
	}

	function updateWpm(newWpm: number) {
		settings = { ...settings, wpm: newWpm };
		readerSettings.updateSpeedReader({ wpm: newWpm });
	}

	function updateSetting(key: string, value: any) {
		settings = { ...settings, [key]: value };
		readerSettings.updateSpeedReader({ [key]: value });
	}

	function formatProgress(): string {
		const percent = words.length > 0 ? Math.round((currentIndex / words.length) * 100) : 0;
		return `${percent}%`;
	}

	function toggleFullscreen() {
		if (!document.fullscreenElement) {
			document.documentElement.requestFullscreen().catch(console.error);
		} else {
			document.exitFullscreen().catch(console.error);
		}
	}

	async function closeReader(e?: Event) {
		e?.preventDefault();
		const targetUrl = book ? `/book/${book.id}` : '/book';
		stop();
		void saveProgress(true);
		void endSession(true);
		window.location.href = targetUrl;
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
</script>

<svelte:head>
	<title>{book?.title || 'Speed Reader'} - Cryptorum</title>
</svelte:head>

	<div
		bind:this={containerEl}
		class="fixed inset-0 z-50 flex flex-col select-none"
			style="background-color: {readerTheme.bg}; color: {readerTheme.text}; font-family: {settings.fontFamily};"
		role="application"
		aria-label="Speed Reader"
	>
	<!-- Top Bar -->
	<header
		class="top-nav transition-opacity duration-200 {showControls ? 'opacity-100' : 'opacity-0 pointer-events-none'}"
	>
		<div class="nav-left">
			<a href={book ? `/book/${book.id}` : '/book'} onclick={closeReader} class="nav-btn nav-close" title="Close">
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<line x1="18" y1="6" x2="6" y2="18"></line>
					<line x1="6" y1="6" x2="18" y2="18"></line>
				</svg>
			</a>
		</div>

		<div class="nav-center">
			<span class="book-title">{book?.title || 'Loading...'}</span>
			<span class="chapter-title">Speed Reader</span>
		</div>

		<div class="nav-right">
			<span class="nav-stat hidden sm:inline">{(currentIndex + 1).toLocaleString()} / {words.length.toLocaleString()}</span>
			<span class="nav-stat hidden sm:inline">{formatProgress()}</span>
			<div class="nav-divider hidden sm:block"></div>
				<button
					type="button"
					onclick={(e) => { e.stopPropagation(); showWpmMenu = !showWpmMenu; }}
					class="nav-btn nav-pill hidden sm:inline-flex"
					title="Playback speed"
			>
				{settings.wpm} wpm
			</button>
				<button
					type="button"
					onclick={(e) => { e.stopPropagation(); openWordPicker(e); }}
					class="nav-btn"
					title="Word Picker"
			>
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h7"></path>
				</svg>
			</button>
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
				style="--focal-point: {settings.focalPoint * 100}%; top: 50%;"
			>
				{#if settings.centerWord}
					<p
						class="speed-word speed-word-centered font-bold text-white"
						style="font-size: {settings.wordSize}px; letter-spacing: {settings.letterSpacing}px;"
					>
						<span>{wordParts.before}</span><span
							class="accent-char"
							style={settings.accentEnabled ? `color: ${settings.accentColor}; opacity: ${settings.accentOpacity};` : ''}
						>{wordParts.accent}</span><span>{wordParts.after}</span>
					</p>
				{:else}
					<p
						class="speed-word speed-word-orp font-bold text-white"
						style="font-size: {settings.wordSize}px; letter-spacing: {settings.letterSpacing}px;"
					>
						<span class="speed-word-before">{wordParts.before}</span>
						<span
							class="accent-char speed-word-accent"
							style={settings.accentEnabled ? `color: ${settings.accentColor}; opacity: ${settings.accentOpacity};` : ''}
						>{wordParts.accent}</span>
						<span class="speed-word-after">{wordParts.after}</span>
					</p>
				{/if}
			</div>
		{:else}
			<p class="text-white/60">No text available for speed reading</p>
		{/if}
	</div>

	<!-- Bottom Bar -->
	<footer
		class="speed-footer absolute bottom-0 left-0 right-0 p-4 transition-opacity duration-200 {showControls ? 'opacity-100' : 'opacity-0 pointer-events-none'}"
	>
		<div class="flex items-center justify-center gap-6">
			<button
				type="button"
				onclick={prevWord}
				class="w-12 h-12 rounded-full bg-white/10 hover:bg-white/20 text-white flex items-center justify-center transition-colors text-2xl font-light"
				aria-label="Previous word"
				title="Previous word"
			>
				&lt;
			</button>

			<button
				type="button"
				onclick={togglePlay}
				class="w-16 h-16 rounded-full text-white flex items-center justify-center transition-colors shadow-lg"
				style="background-color: var(--color-primary-500);"
				aria-label={isPlaying ? 'Pause' : 'Play'}
				title={isPlaying ? 'Pause' : 'Play'}
			>
				{#if isPlaying}
					<svg class="w-8 h-8" fill="currentColor" viewBox="0 0 24 24">
						<path d="M6 4h4v16H6V4zm8 0h4v16h-4V4z"></path>
					</svg>
				{:else}
					<svg class="w-8 h-8 ml-1" fill="currentColor" viewBox="0 0 24 24">
						<path d="M8 5v14l11-7z"></path>
					</svg>
				{/if}
			</button>

			<button
				type="button"
				onclick={nextWord}
				class="w-12 h-12 rounded-full bg-white/10 hover:bg-white/20 text-white flex items-center justify-center transition-colors text-2xl font-light"
				aria-label="Next word"
				title="Next word"
			>
				&gt;
			</button>
		</div>
	</footer>

	<!-- WPM Menu Popup -->
		{#if showWpmMenu}
			<div
				bind:this={wpmMenuRef}
				class="absolute bottom-24 right-4 w-64 rounded-xl shadow-xl z-[70] p-4"
				style="background-color: {readerTheme.bg}; border: 1px solid {readerTheme.text}20;"
			>
			<!-- WPM Value Display -->
			<div class="flex items-center justify-between mb-4">
					<button
						type="button"
						onclick={() => updateWpm(Math.max(50, settings.wpm - 100))}
						class="px-2 py-1 rounded text-sm transition-colors"
						style="background-color: {readerTheme.text}20; color: {readerTheme.text};"
				>
					-100
				</button>
					<button
						type="button"
						onclick={() => updateWpm(Math.max(50, settings.wpm - 50))}
						class="px-2 py-1 rounded text-sm transition-colors"
					style="background-color: {readerTheme.text}20; color: {readerTheme.text};"
				>
					-50
				</button>
				<span class="text-2xl font-bold text-center flex-1" style="color: {readerTheme.text};">{settings.wpm}</span>
					<button
						type="button"
						onclick={() => updateWpm(Math.min(1200, settings.wpm + 50))}
						class="px-2 py-1 rounded text-sm transition-colors"
					style="background-color: {readerTheme.text}20; color: {readerTheme.text};"
				>
					+50
				</button>
					<button
						type="button"
						onclick={() => updateWpm(Math.min(1200, settings.wpm + 100))}
						class="px-2 py-1 rounded text-sm transition-colors"
					style="background-color: {readerTheme.text}20; color: {readerTheme.text};"
				>
					+100
				</button>
			</div>

			<!-- WPM Slider Row -->
			<div class="flex items-center gap-2">
					<button
						type="button"
						onclick={() => updateWpm(Math.max(50, settings.wpm - 10))}
						class="text-xl transition-colors"
					style="color: {readerTheme.text};"
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
					class="flex-1 h-2 rounded-lg appearance-none cursor-pointer"
					style="background-color: {readerTheme.text}20;"
				/>
					<button
						type="button"
						onclick={() => updateWpm(Math.min(1200, settings.wpm + 10))}
						class="text-xl transition-colors"
					style="color: {readerTheme.text};"
				>
					+
				</button>
			</div>
		</div>
	{/if}

	<!-- Word Picker Panel -->
		{#if showWordPicker}
			<div
				class="fixed inset-0 z-[60] flex flex-col"
				style="background-color: {readerTheme.bg};"
				role="dialog"
				aria-modal="true"
				aria-label="Word picker"
				tabindex="0"
				onkeydown={(e) => { if (e.key === 'Escape') cancelWordPicker(); }}
				onclick={(e) => { if (e.target === e.currentTarget) cancelWordPicker(); }}
			>
			<!-- Header -->
			<div class="flex-shrink-0 flex items-center justify-between px-5 py-4 border-b" style="border-color: {readerTheme.text}20;">
				<div>
					<h3 class="font-semibold text-base" style="color: {readerTheme.text};">Jump to Position</h3>
					<p class="text-xs mt-0.5" style="color: {readerTheme.text}60;">
						Tap a word to select it, then confirm
					</p>
				</div>
					<button type="button" onclick={cancelWordPicker} class="p-1.5 transition-colors" style="color: {readerTheme.text}80;" aria-label="Close word picker" title="Close word picker">
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
				{#each wordPickerParagraphs as para}
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
						onclick={cancelWordPicker}
						class="flex-1 px-4 py-2.5 rounded-lg text-sm font-medium transition-colors"
					style="background-color: {readerTheme.text}15; color: {readerTheme.text};"
				>
					Cancel
				</button>
					<button
						type="button"
						onclick={confirmWordPicker}
						class="flex-1 px-4 py-2.5 rounded-lg text-sm font-medium transition-colors"
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
			class="fixed top-12 right-0 h-[calc(100vh-3rem)] w-[480px] shadow-xl z-[60] flex flex-col transform transition-transform duration-300"
			style="background-color: var(--color-surface-overlay); border-left: 1px solid var(--color-surface-border);"
		>
			<div class="flex-1 overflow-y-auto p-4 space-y-6 custom-scrollbar">
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
								<button
									onclick={() => updateSetting('theme', customTheme.id)}
									class="flex flex-col items-center p-2 rounded-lg border-2 transition-all text-sm {settings.theme === customTheme.id ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] hover:border-[var(--color-surface-500)]'}"
								>
									<ThemePreviewSwatch background={customTheme.background} foreground={customTheme.foreground} sizeClass="h-8 w-8 mb-1" />
									<span class="text-xs text-[var(--color-surface-text)]">{customTheme.name}</span>
								</button>
							{/each}
						{/if}
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

					<!-- Keep Screen On -->
						<div class="flex items-center justify-between">
						<div>
							<div class="text-sm font-medium block" style="color: var(--color-surface-text);">Keep Screen On</div>
							<p class="text-xs" style="color: var(--color-surface-text-muted);">Prevent screen from turning off</p>
						</div>
						<button
							type="button"
							onclick={() => updateSetting('keepScreenOn', !settings.keepScreenOn)}
							class="relative w-12 h-6 rounded-full transition-colors {settings.keepScreenOn ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-surface-700)]'}"
							aria-label={settings.keepScreenOn ? 'Disable keep screen on' : 'Enable keep screen on'}
							title={settings.keepScreenOn ? 'Disable keep screen on' : 'Enable keep screen on'}
						>
						<span
							class="absolute top-1 w-4 h-4 bg-white rounded-full transition-transform {settings.keepScreenOn ? 'left-7' : 'left-1'}"
						></span>
					</button>
				</div>

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
	.top-nav {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: space-between;
		height: 48px;
		padding: 0 12px;
		background: var(--color-surface-base, #0f172a);
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		flex-shrink: 0;
		z-index: 100;
	}

	.nav-left,
	.nav-center,
	.nav-right {
		display: flex;
		align-items: center;
		gap: 4px;
	}

	.nav-left { flex: 1; }
	.nav-center {
		flex: 2;
		justify-content: center;
		flex-direction: column;
		gap: 0;
		min-width: 0;
	}
	.nav-right { flex: 1; justify-content: flex-end; }

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
		color: var(--color-surface-text, #e2e8f0);
		cursor: pointer;
		transition: background-color 0.15s, color 0.15s;
	}

	.nav-btn:hover,
	.nav-close:hover { background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85)); }
	.nav-btn:disabled { opacity: 0.3; cursor: not-allowed; }
	.nav-btn.active { background: var(--color-primary-500, #22c55e); color: white; }

	.nav-close { text-decoration: none; }

	.nav-pill {
		width: auto;
		padding: 0 12px;
		font-size: 12px;
		font-weight: 500;
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
		white-space: nowrap;
	}

	.nav-divider {
		width: 1px;
		height: 24px;
		background: var(--color-surface-border, rgba(55, 65, 81, 0.6));
		margin: 0 8px;
	}

	.book-title {
		color: var(--color-surface-text, #e2e8f0);
		font-size: 14px;
		font-weight: 500;
		max-width: 300px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.chapter-title {
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 11px;
		max-width: 300px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.nav-stat {
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 12px;
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
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
		grid-template-columns: minmax(0, var(--focal-point)) auto 1fr;
		align-items: baseline;
		width: 100%;
	}

	.speed-word-before {
		justify-self: end;
		text-align: right;
	}

	.speed-word-accent {
		justify-self: center;
	}

	.speed-word-after {
		justify-self: start;
	}
</style>

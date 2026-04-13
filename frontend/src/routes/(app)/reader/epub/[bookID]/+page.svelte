<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { browser } from '$app/environment';
	import ePub from 'epubjs';
	import { readerSettings, epubThemes, fontFamilies, fontWeightOptions, type EpubReaderSetting } from '$lib/stores/readerSettings';
	import { currentTheme, resolveThemeColors, type FullTheme } from '$lib/stores/theme';
	import ThemePreviewSwatch from '$lib/components/ThemePreviewSwatch.svelte';
	import {
		getCachedBook,
		cacheBook,
		isBookCached,
		getCachedProcessedEpub,
		cacheProcessedEpub,
		getCachedEpubLocations,
		cacheEpubLocations
	} from '$lib/stores/bookCache';

	let book = $state<any>(null);
	let bookFormat = $state('epub');
	let bookLoaded = $state(false);
	let loading = $state(true);
	let error = $state('');
	let epubInstance: any = null;
	let rendition: any = null;
	let paginatedPreloadPromise: Promise<void> | null = null;
	let currentLocation = $state('');
	let canPrev = $state(false);
	let canNext = $state(false);
	let isBookmarked = $state(false);
	let currentChapter = $state('');
	let savedProgress = $state<any>(null);
	let epubToc = $state<any[]>([]);
	let currentSessionId = $state<number | null>(null);
	let appTheme = $state<FullTheme | null>(null);

	let settings = $state<EpubReaderSetting>({
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
		theme: 'dark',
		isDark: true,
		flow: 'scrolled',
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
		customCss: '',
		showTextLayer: true,
		originalLayout: false,
		continuousMode: true,
		showImages: true,
		imageSize: 'fit-width',
		imageGrayscale: false
	});

	let continuousContent = $state('');
	let continuousToc = $state<any[]>([]);
	let continuousLoading = $state(false);
	let continuousScrollEl: HTMLElement | null = null;
	let isRestoringContinuousProgress = $state(false);
	let initialProcessing = $state(false);
	let processingMessage = $state('Preparing book...');

	type SidebarTab = 'toc' | 'bookmarks' | 'search';
	let leftSidebarOpen = $state(false);
	let rightSidebarOpen = $state(false);
	let activeSidebarTab = $state<SidebarTab>('toc');
	let activeSettingsTab = $state<'typography' | 'display'>('typography');
	let searchQuery = $state('');
	let searchResults = $state<any[]>([]);
	let currentSearchResult = $state(0);
	let matchCase = $state(false);
	let isSearching = $state(false);
	let isDraggingProgress = $state(false);
	let pendingProgressPage = $state<number | null>(null);
	let currentProgress = $state(0);
	let lastSavedProgress: number | null = null;
	let progressSaveTimer: ReturnType<typeof setTimeout> | null = null;
	let pendingProgressCfi: string | undefined;
	let lastWheelNavigationAt = 0;
	let wheelNavigationTarget: Document | null = null;
	let sessionEnded = false;

	const progress = $derived(currentProgress);
	const PROGRESS_SAVE_DEBOUNCE_MS = 750;
	const PROGRESS_SAVE_MIN_DELTA = 0.05;

	const unsubTheme = currentTheme.subscribe(theme => {
		appTheme = theme;
	});

	onMount(async () => {
		const bookId = $page.params.bookID;
		try {
			const [bookRes, filesRes] = await Promise.all([
				fetch(`/api/books/${bookId}`),
				fetch(`/api/books/${bookId}/files`)
			]);
			if (bookRes.ok) {
				book = await bookRes.json();
			} else {
				error = 'Failed to load book details';
			}
			if (filesRes.ok) {
				const files = await filesRes.json();
				if (files.length > 0) {
					bookFormat = files[0].format?.toLowerCase() ?? 'epub';
				}
			}
		} catch (e) {
			console.error('Failed to load book:', e);
			error = 'Failed to load book';
		} finally {
			loading = false;
			bookLoaded = true;
		}

		if (book && book.id) {
			await startSession();
		}

		readerSettings.subscribe(s => {
			settings = { ...s.epub };
			applyTheme(settings.theme);
			if (rendition) {
				updateTypographyOverrides();
			}
		});
	});

	async function preloadBookContent() {
		if (!book || !book.id) return false;
		
		if (isBookCached(book.id)) {
			const cached = getCachedBook(book.id);
			if (cached) {
				continuousContent = cached;
				await tick();
				applyContinuousContentStyles();
				await loadContinuousToc();
				await restoreContinuousProgress();
				preloadPaginatedReader();
				return true;
			}
		}
		
		initialProcessing = true;
		processingMessage = 'Extracting text content...';
		
		try {
			const res = await fetch(`/api/books/${book.id}/continuous`);
			if (res.ok) {
				const content = await res.text();
				continuousContent = content;
				cacheBook(book.id, content);
				processingMessage = 'Applying styles...';
				await tick();
				applyContinuousContentStyles();
				await loadContinuousToc();
				await restoreContinuousProgress();
				preloadPaginatedReader();
				return true;
			}
		} catch (e) {
			console.error('Failed to preload content:', e);
		} finally {
			initialProcessing = false;
		}
		return false;
	}

	async function startSession() {
		if (!book || !book.id) return;
		try {
			const res = await fetch(`/api/books/${book.id}/sessions`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ reader_type: 'epub' })
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
		if (sessionEnded || currentSessionId === null || !book || !book.id) return;
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

	onDestroy(() => {
		flushProgressSave();
		void endSession(true);
		unsubTheme();
	});

	async function closeReader(e?: Event) {
		e?.preventDefault();
		if (progressSaveTimer) {
			clearTimeout(progressSaveTimer);
			progressSaveTimer = null;
		}
		const cfi = pendingProgressCfi;
		pendingProgressCfi = undefined;
		await saveProgressNow(cfi);
		await endSession();
		window.location.href = book ? `/book/${book.id}` : '/book';
	}

	function getReaderTheme(themeId: string | null) {
		return resolveThemeColors(
			themeId,
			epubThemes,
			appTheme?.appearance.customThemes ?? [],
			epubThemes[0]
				? { foreground: epubThemes[0].text, background: epubThemes[0].bg }
				: { foreground: '#e5e7eb', background: '#111111' }
		);
	}

	function applyTheme(themeId: string) {
		const theme = getReaderTheme(themeId);
		const readerEl = document.getElementById('epub-container');
		if (readerEl) {
			readerEl.style.backgroundColor = theme.background;
		}
		if (settings.continuousMode && continuousContent) {
			applyContinuousContentStyles();
		}
	}

	function getFontFamily(fontId: string): string {
		const font = fontFamilies.find(f => f.id === fontId);
		return font ? font.family : 'Georgia, serif';
	}

	function updateThemeOverrides() {
		if (!rendition) return;
		const theme = getReaderTheme(settings.theme);
		rendition.themes.override("background-color", theme.background);
		rendition.themes.override("color", theme.foreground);
	}

	function injectCssOverride() {
		if (!rendition) return;
		setTimeout(() => {
			const iframe = document.querySelector('#epub-container iframe') as HTMLIFrameElement;
			if (!iframe) return;
			const doc = iframe.contentDocument || iframe.contentWindow?.document;
			if (!doc) return;

			let styleEl = doc.getElementById('custom-css-override');
			if (!styleEl) {
				styleEl = doc.createElement('style');
				styleEl.id = 'custom-css-override';
				doc.head.appendChild(styleEl);
			}

			if (settings.originalLayout) {
				// Load preserved original CSS from ebook-converter
				loadOriginalStyles();
				styleEl.textContent = '';
				return;
			}

			const theme = getReaderTheme(settings.theme);
			const fontFamily = getFontFamily(settings.fontFamily);
			const margin = settings.margin;
			const colCount = settings.maxColumnCount;
			const containerWidth = 680;
			const columnWidth = colCount > 1 ? Math.floor((containerWidth - margin * 2) / colCount) : containerWidth;
			const gutter = colCount > 1 ? margin * 2 : 0;

			styleEl.textContent = `
			/* CSS Reset - Override epub CSS */
			html, body, body * {
				column-count: none !important;
				column-width: auto !important;
				columns: none !important;
				-webkit-column-count: none !important;
				-webkit-column-width: auto !important;
				-webkit-columns: none !important;
				column-fill: auto !important;
			}
			/* Apply our typography and theme */
			body, body * {
				font-family: ${fontFamily} !important;
				font-size: ${settings.fontSize}px !important;
				line-height: ${settings.lineHeight} !important;
				font-weight: ${settings.fontWeight} !important;
				font-style: ${settings.fontStyle} !important;
				text-align: ${settings.justify ? 'justify' : 'left'} !important;
				-webkit-hyphens: ${settings.hyphenate ? 'auto' : 'none'} !important;
				hyphens: ${settings.hyphenate ? 'auto' : 'none'} !important;
				letter-spacing: ${settings.letterSpacing}em !important;
				max-width: none !important;
				background-color: ${theme.background} !important;
				color: ${theme.foreground} !important;
			}
			/* Paragraph spacing */
			p, div, section, article {
				margin-bottom: ${settings.paragraphSpacing}px !important;
				text-indent: ${settings.paragraphIndent}px !important;
			}
			/* Column layout for multi-column */
			body {
				column-count: ${colCount} !important;
				column-width: ${colCount > 1 ? columnWidth + 'px' : 'auto'} !important;
				column-gap: ${gutter}px !important;
				-webkit-column-count: ${colCount} !important;
				-webkit-column-width: ${colCount > 1 ? columnWidth + 'px' : 'auto'} !important;
				-webkit-column-gap: ${gutter}px !important;
				column-fill: auto !important;
			}
			/* Text container margins */
			section, div, article, p, h1, h2, h3, h4, h5, h6 {
				margin-left: ${margin}px !important;
				margin-right: ${margin}px !important;
				padding-left: ${margin}px !important;
				padding-right: ${margin}px !important;
			}
			/* Images */
			img {
				${settings.showImages === false ? 'display: none !important;' : 'max-width: 100% !important; height: auto !important; display: block !important; margin: 0 auto !important;'}
				${settings.showImages === false ? '' : ''}
			}
			`;
		}, 50);
	}

	function applyContinuousContentStyles() {
		const container = document.getElementById('continuous-container');
		const content = document.querySelector('.continuous-content') as HTMLElement;
		if (!content) return;

		const theme = getReaderTheme(settings.theme);
		const fontFamily = getFontFamily(settings.fontFamily);
		const maxWidth = settings.continuousMaxWidth;
		const brightness = settings.brightness;
		const contrast = settings.contrast;

		if (container) {
			container.style.backgroundColor = theme.bg;
			container.style.filter = `brightness(${brightness}%) contrast(${contrast}%)`;
		}

		content.style.fontFamily = fontFamily;
		content.style.fontSize = settings.fontSize + 'px';
		content.style.lineHeight = settings.lineHeight.toString();
		content.style.letterSpacing = settings.letterSpacing + 'em';
		content.style.fontWeight = settings.fontWeight.toString();
		content.style.fontStyle = settings.fontStyle;
		content.style.textAlign = settings.justify ? 'justify' : 'left';
		content.style.color = theme.text;
		content.style.backgroundColor = theme.bg;
		content.style.maxWidth = maxWidth + 'px';
		content.style.margin = '0 auto';
		content.style.padding = `0 ${settings.margin}px`;

		// Apply image styles via injected CSS
		applyImageStyles();
	}

	function applyImageStyles() {
		let styleEl = document.getElementById('pandoc-image-styles') as HTMLStyleElement | null;
		if (!styleEl) {
			styleEl = document.createElement('style');
			styleEl.id = 'pandoc-image-styles';
			document.head.appendChild(styleEl);
		}

		if (!settings.showImages) {
			styleEl.textContent = `.continuous-content img { display: none !important; }`;
			return;
		}

		const grayscale = settings.imageGrayscale ? 'grayscale(100%)' : 'none';
		let sizeCss = '';
		if (settings.imageSize === 'fit-width') {
			sizeCss = `max-width: 100% !important; width: auto !important; height: auto !important;`;
		} else if (settings.imageSize === 'fit-page') {
			sizeCss = `max-width: 100% !important; max-height: 80vh !important; width: auto !important; height: auto !important; object-fit: contain !important;`;
		} else {
			sizeCss = `max-width: none !important; width: auto !important; height: auto !important;`;
		}

		styleEl.textContent = `
			.continuous-content img {
				display: block !important;
				margin: 1em auto !important;
				filter: ${grayscale} !important;
				${sizeCss}
			}
		`;
	}

	async function loadOriginalStyles() {
		try {
			const response = await fetch(`/api/books/${book.id}/continuous/styles`);
			if (response.ok) {
				const css = await response.text();
				const originalStyleEl = document.getElementById('original-styles');
				if (!originalStyleEl) {
					const newStyleEl = document.createElement('style');
					newStyleEl.id = 'original-styles';
					newStyleEl.textContent = css;
					document.head.appendChild(newStyleEl);
				} else {
					originalStyleEl.textContent = css;
				}
			}
		} catch (error) {
			console.warn('Failed to load original styles:', error);
		}
	}

	function updateTypographyOverrides() {
		if (!rendition) return;
		if (!settings.originalLayout) {
			injectCssOverride();
			return;
		}
		if (settings.continuousMode && continuousContent) {
			applyContinuousContentStyles();
			return;
		}
		rendition.themes.fontSize(settings.fontSize + 'px');
		const fontFamily = getFontFamily(settings.fontFamily);
		rendition.themes.font(fontFamily);
		rendition.themes.override("font-family", fontFamily);
		rendition.themes.override("line-height", settings.lineHeight.toString());
		rendition.themes.override("font-weight", settings.fontWeight.toString());
		rendition.themes.override("font-style", settings.fontStyle);
		rendition.themes.override("text-align", settings.justify ? 'justify' : 'left');
		rendition.themes.override("-webkit-hyphens", settings.hyphenate ? 'auto' : 'none');
		rendition.themes.override("hyphens", settings.hyphenate ? 'auto' : 'none');
		rendition.themes.override("letter-spacing", settings.letterSpacing + 'em');
		rendition.themes.override("margin-bottom", settings.paragraphSpacing + 'px');
		rendition.themes.override("text-indent", settings.paragraphIndent + 'px');

		const margin = settings.margin;
		rendition.themes.override("margin", `0 ${margin}px`);
		rendition.themes.override("padding", `0 ${margin}px`);

		if (settings.maxColumnCount > 1) {
			const containerWidth = 680;
			const totalGutter = margin * 2;
			const columnWidth = Math.floor((containerWidth - totalGutter) / settings.maxColumnCount);
			rendition.themes.override("max-width", containerWidth + 'px');
			rendition.themes.override("column-width", columnWidth + 'px');
			rendition.themes.override("column-gap", totalGutter + 'px');
			rendition.themes.override("webkitColumnWidth", columnWidth + 'px');
			rendition.themes.override("webkitColumnGap", totalGutter + 'px');
			rendition.themes.override("column-count", settings.maxColumnCount);
			rendition.themes.override("columns", settings.maxColumnCount);
		} else {
			rendition.themes.override("max-width", settings.maxInlineSize + 'px');
			rendition.themes.override("column-width", 'auto');
			rendition.themes.override("column-gap", '0px');
			rendition.themes.override("column-count", 'none');
			rendition.themes.override("columns", 'none');
			rendition.themes.override("webkitColumnWidth", 'auto');
			rendition.themes.override("webkitColumnGap", '0px');
		}

		if (settings.showImages === false) {
			rendition.themes.override("img", {
				"display": "none"
			});
		} else {
			rendition.themes.override("img", {
				"max-width": "100%",
				"height": "auto",
				"display": "block",
				"margin": "0 auto"
			});
		}

		rendition.themes.override("body", {
			"column-width": settings.maxColumnCount > 1 ? (680 / settings.maxColumnCount) + 'px' : 'auto',
			"column-count": settings.maxColumnCount > 1 ? settings.maxColumnCount : 'none',
			"column-gap": margin * 2 + 'px',
			"column-fill": 'auto'
		});
	}

	async function updateSetting(key: string, value: any) {
		settings = { ...settings, [key]: value };
		readerSettings.updateEpub({ [key]: value });

		if (key === 'theme') {
			applyTheme(value);
			updateThemeOverrides();
		} else if (['fontFamily', 'fontSize', 'lineHeight', 'justify', 'hyphenate', 'maxInlineSize', 'maxBlockSize', 'margin', 'letterSpacing', 'fontWeight', 'fontStyle', 'paragraphSpacing', 'paragraphIndent'].includes(key)) {
			updateTypographyOverrides();
			if (settings.continuousMode) {
				applyContinuousContentStyles();
			}
		} else if (key === 'brightness' || key === 'contrast') {
			applyVisualFilters();
			if (settings.continuousMode) {
				applyContinuousContentStyles();
			}
		} else if (key === 'maxColumnCount' || key === 'margin') {
			updateTypographyOverrides();
			injectCssOverride();
			if (settings.continuousMode) {
				applyContinuousContentStyles();
			}
		} else if (key === 'originalLayout') {
			injectCssOverride();
		} else if (key === 'continuousMode') {
			if (value === true) {
				cleanupRenditionOnly();
				if (continuousContent) {
					await tick();
					applyContinuousContentStyles();
					await restoreContinuousProgress();
				} else {
					loadContinuousContent();
				}
			} else {
				initReader();
			}
		} else if (key === 'showImages' || key === 'imageSize' || key === 'imageGrayscale') {
			applyImageStyles();
			if (!settings.continuousMode && rendition) {
				updateTypographyOverrides();
			}
		} else if (key === 'flow' && rendition) {
			await reinitializeRendition();
		} else if (settings.continuousMode && continuousContent) {
			applyContinuousContentStyles();
		}
	}

	function applyVisualFilters() {
		const readerEl = document.getElementById('epub-container');
		if (readerEl) {
			readerEl.style.filter = `brightness(${settings.brightness}%) contrast(${settings.contrast}%)`;
		}
	}

	function toggleLeftSidebar() {
		if (rightSidebarOpen) {
			rightSidebarOpen = false;
		}
		leftSidebarOpen = !leftSidebarOpen;
	}

	function toggleRightSidebar() {
		if (leftSidebarOpen) {
			leftSidebarOpen = false;
			rightSidebarOpen = true;
		} else {
			rightSidebarOpen = !rightSidebarOpen;
		}
	}

	function toggleBookmark() {
		isBookmarked = !isBookmarked;
	}

	function cleanup() {
		detachWheelNavigationTarget();
		if (rendition) {
			rendition.destroy();
			rendition = null;
		}
		if (epubInstance) {
			epubInstance.destroy();
			epubInstance = null;
		}
	}

	function cleanupRenditionOnly() {
		detachWheelNavigationTarget();
		if (rendition) {
			rendition.destroy();
			rendition = null;
		}
	}

	function updateNavState() {
		if (rendition) {
			canPrev = rendition.location && rendition.location.start ? true : false;
			const spine = epubInstance?.spine;
			const location = rendition.location;
			if (spine && location && location.start && location.end) {
				canNext = location.start.index < spine.length - 1;
			}
		}
	}

	async function goNext() {
		if (rendition) {
			await rendition.next();
			updateNavState();
		}
	}

	async function goPrev() {
		if (rendition) {
			await rendition.prev();
			updateNavState();
		}
	}

	function jumpToPage(pageNum: number) {
		if (!rendition || !epubInstance) return;
		const total = epubInstance.locations.total;
		if (total > 0) {
			const percentage = (pageNum / numChapters()) * 100;
			const cfi = epubInstance.locations.cfiFromPercentage(percentage / 100);
			rendition.display(cfi);
		}
	}

	function numChapters(): number {
		return epubToc?.length || 1;
	}

	function goToTocItem(item: any) {
		if (settings.continuousMode) {
			// Navigate by scrolling to the heading element in continuous content
			if (item.id && continuousScrollEl) {
				const el = continuousScrollEl.querySelector('#' + CSS.escape(item.id)) as HTMLElement;
				if (el) {
					el.scrollIntoView({ behavior: 'smooth', block: 'start' });
				}
			}
			leftSidebarOpen = false;
			return;
		}
		if (rendition && item.href) {
			rendition.display(item.href);
			leftSidebarOpen = false;
		}
	}

	function handleProgressThumbMouseDown(e: MouseEvent) {
		e.preventDefault();
		e.stopPropagation();
		isDraggingProgress = true;
		pendingProgressPage = null;
		window.addEventListener('mousemove', handleProgressMouseMove);
		window.addEventListener('mouseup', handleProgressMouseUp);
	}

	function handleProgressMouseMove(e: MouseEvent) {
		if (!isDraggingProgress) return;
		const progressBar = document.querySelector('.progress-bar') as HTMLElement;
		if (!progressBar) return;
		const rect = progressBar.getBoundingClientRect();
		const x = e.clientX - rect.left;
		const percentage = Math.max(0, Math.min(1, x / rect.width));

		if (settings.continuousMode && continuousScrollEl) {
			const scrollHeight = continuousScrollEl.scrollHeight - continuousScrollEl.clientHeight;
			if (scrollHeight > 0) {
				continuousScrollEl.scrollTop = percentage * scrollHeight;
			}
		} else {
			const newPage = Math.round(percentage * numChapters());
			if (newPage >= 1 && newPage <= numChapters()) {
				pendingProgressPage = newPage;
			}
		}
	}

	function handleProgressMouseUp() {
		isDraggingProgress = false;
		window.removeEventListener('mousemove', handleProgressMouseMove);
		window.removeEventListener('mouseup', handleProgressMouseUp);
		if (pendingProgressPage !== null) {
			jumpToPage(pendingProgressPage);
			pendingProgressPage = null;
		}
	}

	function handleProgressBarClick(e: MouseEvent) {
		if (isDraggingProgress) return;
		const progressBar = document.querySelector('.progress-bar') as HTMLElement;
		if (!progressBar) return;
		const rect = progressBar.getBoundingClientRect();
		const x = e.clientX - rect.left;
		const percentage = Math.max(0, Math.min(1, x / rect.width));

		if (settings.continuousMode && continuousScrollEl) {
			const scrollHeight = continuousScrollEl.scrollHeight - continuousScrollEl.clientHeight;
			if (scrollHeight > 0) {
				continuousScrollEl.scrollTop = percentage * scrollHeight;
			}
		} else {
			const newPage = Math.round(percentage * numChapters());
			if (newPage >= 1 && newPage <= numChapters()) {
				jumpToPage(newPage);
			}
		}
	}

	function handleProgressBarKeydown(e: KeyboardEvent) {
		if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') {
			e.preventDefault();
			goPrev();
		} else if (e.key === 'ArrowRight' || e.key === 'ArrowUp') {
			e.preventDefault();
			goNext();
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (rightSidebarOpen) {
				rightSidebarOpen = false;
			} else if (leftSidebarOpen) {
				leftSidebarOpen = false;
			} else {
				void closeReader();
			}
		} else if (e.key === 'ArrowRight' || e.key === ' ') {
			e.preventDefault();
			goNext();
		} else if (e.key === 'ArrowLeft') {
			e.preventDefault();
			goPrev();
		} else if (e.key === 'b' || e.key === 'B') {
			toggleBookmark();
		} else if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
			e.preventDefault();
			activeSidebarTab = 'search';
			leftSidebarOpen = true;
			rightSidebarOpen = false;
		}
	}

	function shouldIgnoreWheelNavigation(target: EventTarget | null) {
		if (!(target instanceof Element)) return false;
		return !!target.closest('input, textarea, select, [contenteditable="true"], .left-sidebar, .right-sidebar');
	}

	function handleWheelNavigation(e: WheelEvent) {
		if (settings.continuousMode || shouldIgnoreWheelNavigation(e.target)) return;

		const dominantDelta = Math.abs(e.deltaY) >= Math.abs(e.deltaX) ? e.deltaY : e.deltaX;
		if (Math.abs(dominantDelta) < 12) return;

		const now = performance.now();
		if (now - lastWheelNavigationAt < 220) {
			e.preventDefault();
			return;
		}

		e.preventDefault();
		lastWheelNavigationAt = now;

		if (dominantDelta > 0) {
			goNext();
		} else {
			goPrev();
		}
	}

	function detachWheelNavigationTarget() {
		if (!wheelNavigationTarget) return;
		wheelNavigationTarget.removeEventListener('wheel', handleWheelNavigation);
		wheelNavigationTarget = null;
	}

	function attachWheelNavigationTarget(view?: any) {
		if (!rendition || settings.continuousMode) return;

		const doc =
			view?.document ||
			view?.contents?.document ||
			view?.contents?.contentDocument ||
			document.querySelector('#epub-container iframe')?.contentDocument ||
			null;

		if (!doc || doc === wheelNavigationTarget) return;

		detachWheelNavigationTarget();
		doc.addEventListener('wheel', handleWheelNavigation, { passive: false });
		wheelNavigationTarget = doc;
	}

	function toggleFullscreen() {
		if (!document.fullscreenElement) {
			document.documentElement.requestFullscreen();
		} else {
			document.exitFullscreen();
		}
	}

	async function performSearch() {
		if (!epubInstance || !searchQuery.trim()) {
			searchResults = [];
			return;
		}
		isSearching = true;
		searchResults = [];

		try {
			const results = await epubInstance.search(searchQuery, { case: matchCase });
			searchResults = results.slice(0, 50);
			currentSearchResult = searchResults.length > 0 ? 1 : 0;
		} catch (e) {
			console.warn('Search failed:', e);
		}
		isSearching = false;
	}

	function prevSearchResult() {
		if (currentSearchResult > 1) {
			currentSearchResult--;
			goToSearchResult(currentSearchResult - 1);
		}
	}

	function nextSearchResult() {
		if (currentSearchResult < searchResults.length) {
			currentSearchResult++;
			goToSearchResult(currentSearchResult - 1);
		}
	}

	function goToSearchResult(index: number) {
		if (searchResults[index] && rendition) {
			rendition.display(searchResults[index].cfi);
		}
	}

	function setupScrollPreload() {
		const container = document.getElementById('epub-container');
		if (!container) return;
		let preloadTimeout: ReturnType<typeof setTimeout> | null = null;
		container.addEventListener('scroll', () => {
			if (preloadTimeout) clearTimeout(preloadTimeout);
			preloadTimeout = setTimeout(() => {
				const threshold = 500;
				const scrollTop = container.scrollTop;
				const scrollHeight = container.scrollHeight;
				const clientHeight = container.clientHeight;
				if (scrollHeight - scrollTop - clientHeight < threshold) {
					rendition?.next();
				}
			}, 100);
		});
	}

	async function reinitializeRendition() {
		if (!epubInstance) return;
		const savedCfi = getResumeCfi();
		if (rendition) {
			rendition.destroy();
			rendition = null;
		}
		const isScrolled = settings.flow === 'scrolled';
		rendition = epubInstance.renderTo('epub-container', {
			width: '100%',
			height: '100%',
			manager: isScrolled ? 'continuous' : 'default',
			flow: isScrolled ? 'scrolled' : settings.flow,
			spread: settings.maxColumnCount > 1 ? 'auto' : 'none',
			minSpreadWidth: isScrolled ? 0 : 800,
			gap: isScrolled ? '20px' : '0'
		});
		rendition.on('relocated', (location: any) => {
			currentLocation = location.start.href;
			const cfi = location.start.cfi;
			if (epubInstance && epubInstance.locations) {
				const percentage = epubInstance.locations.percentageFromCfi(cfi);
				if (percentage !== null && !isNaN(percentage)) {
					setCurrentProgress(percentage * 100);
				}
			}
			if (location.start.href) {
				const toc = epubInstance.toc;
				const chapter = toc?.find((item: any) => item.href === location.start.href);
				currentChapter = chapter?.label || '';
			}
			updateNavState();
			queueProgressSave(cfi);
		});
		rendition.on('rendered', () => {
			if (!settings.originalLayout) {
				injectCssOverride();
			}
		});
		if (savedCfi) {
			await rendition.display(savedCfi);
		} else {
			await rendition.display();
		}
		injectCssOverride();
		if (isScrolled) {
			setupScrollPreload();
		}
	}

	function preloadPaginatedReader() {
		if (!browser || !book || bookFormat !== 'epub' || epubInstance || paginatedPreloadPromise) {
			return;
		}

		paginatedPreloadPromise = ensurePaginatedBookReady()
			.catch((e) => {
				console.warn('Failed to preload paginated reader:', e);
			})
			.finally(() => {
				paginatedPreloadPromise = null;
			});
	}

	async function ensurePaginatedBookReady() {
		if (!browser || !book || epubInstance) return;

		let bookData = getCachedProcessedEpub(book.id);
		if (!bookData) {
			const response = await fetch(`/api/books/${book.id}/processed-file`);
			if (!response.ok) {
				throw new Error(`Failed to fetch processed EPUB file: ${response.status}`);
			}
			bookData = await response.arrayBuffer();
			cacheProcessedEpub(book.id, bookData);
		}

		epubInstance = ePub(bookData);
		await epubInstance.ready;

		const cachedLocations = getCachedEpubLocations(book.id);
		if (cachedLocations) {
			epubInstance.locations.load(cachedLocations);
		} else {
			await epubInstance.locations.generate(1200);
			cacheEpubLocations(book.id, epubInstance.locations.save());
		}

		epubToc = epubInstance.toc || [];
	}

	async function initReader() {
		if (!browser || !book) return;

		try {
			await fetchProgress();
			if (paginatedPreloadPromise) {
				await paginatedPreloadPromise;
			}
			await ensurePaginatedBookReady();

			const isScrolled = settings.flow === 'scrolled';
			cleanupRenditionOnly();
			rendition = epubInstance.renderTo('epub-container', {
				width: '100%',
				height: '100%',
				manager: isScrolled ? 'continuous' : 'default',
				flow: isScrolled ? 'scrolled' : settings.flow,
				spread: settings.maxColumnCount > 1 ? 'auto' : 'none',
				minSpreadWidth: isScrolled ? 0 : 800,
				gap: isScrolled ? '20px' : '0'
			});

			updateTypographyOverrides();
			updateThemeOverrides();
			applyVisualFilters();

			if (savedProgress && savedProgress.cfi) {
				setCurrentProgress(savedProgress.percent ?? 0);
				await rendition.display(savedProgress.cfi);
			} else if (savedProgress && typeof savedProgress.percent === 'number' && epubInstance.locations) {
				const cfi = epubInstance.locations.cfiFromPercentage(
					Math.max(0, Math.min(1, savedProgress.percent / 100))
				);
				if (cfi) {
					setCurrentProgress(savedProgress.percent);
					await rendition.display(cfi);
				} else {
					setCurrentProgress(savedProgress.percent);
					await rendition.display();
				}
			} else {
				await rendition.display();
			}
			injectCssOverride();
			updateNavState();

			if (isScrolled) {
				setupScrollPreload();
			}

			rendition.on('relocated', (location: any) => {
				currentLocation = location.start.href;
				const cfi = location.start.cfi;

				if (epubInstance && epubInstance.locations) {
					const percentage = epubInstance.locations.percentageFromCfi(cfi);
					if (percentage !== null && !isNaN(percentage)) {
						setCurrentProgress(percentage * 100);
					}
				}

				if (location.start.href) {
					const toc = epubInstance.toc;
					const chapter = toc?.find((item: any) => item.href === location.start.href);
					currentChapter = chapter?.label || '';
				}

				updateNavState();
				queueProgressSave(cfi);
			});

			rendition.on('rendered', (_section: any, view: any) => {
				attachWheelNavigationTarget(view);
				if (!settings.originalLayout) {
					injectCssOverride();
				}
			});

			window.addEventListener('keydown', handleKeydown);

		} catch (e) {
			console.error('Failed to initialize reader:', e);
			error = `Failed to initialize reader: ${e instanceof Error ? e.message : String(e)}`;
			cleanup();
		}
	}

	async function fetchProgress() {
		try {
			const res = await fetch(`/api/books/${book.id}/progress`);
			if (res.ok) {
				savedProgress = await res.json();
				if (typeof savedProgress?.percent === 'number') {
					setCurrentProgress(savedProgress.percent);
					lastSavedProgress = currentProgress;
				}
			}
		} catch (e) {
			console.error('Failed to fetch progress:', e);
		}
	}

	function setCurrentProgress(percent: number) {
		if (!Number.isFinite(percent)) return;
		currentProgress = Number(Math.max(0, Math.min(100, percent)).toFixed(4));
	}

	function queueProgressSave(cfi?: string, immediate = false) {
		if (!book || isRestoringContinuousProgress) return;
		pendingProgressCfi = cfi || pendingProgressCfi;

		if (!immediate && lastSavedProgress !== null) {
			const delta = Math.abs(currentProgress - lastSavedProgress);
			if (delta < PROGRESS_SAVE_MIN_DELTA) {
				return;
			}
		}

		if (progressSaveTimer) {
			clearTimeout(progressSaveTimer);
			progressSaveTimer = null;
		}

		if (immediate) {
			void saveProgressNow(pendingProgressCfi);
			pendingProgressCfi = undefined;
			return;
		}

		progressSaveTimer = setTimeout(() => {
			progressSaveTimer = null;
			void saveProgressNow(pendingProgressCfi);
			pendingProgressCfi = undefined;
		}, PROGRESS_SAVE_DEBOUNCE_MS);
	}

	function flushProgressSave() {
		if (progressSaveTimer) {
			clearTimeout(progressSaveTimer);
			progressSaveTimer = null;
		}

		if (!book || isRestoringContinuousProgress) return;
		void saveProgressNow(pendingProgressCfi);
		pendingProgressCfi = undefined;
	}

	async function saveProgressNow(cfi?: string) {
		if (!book) return;
		try {
			const percent = currentProgress;
			await fetch(`/api/books/${book.id}/progress`, {
				method: 'PUT',
				keepalive: true,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					cfi: cfi || savedProgress?.cfi,
					percent,
					status: percent >= 100 ? 'finished' : 'reading'
				})
			});
			lastSavedProgress = percent;
			savedProgress = {
				...(savedProgress || {}),
				cfi: cfi || savedProgress?.cfi,
				percent,
				status: percent >= 100 ? 'finished' : 'reading'
			};
		} catch (e) {
			console.error('Failed to save progress:', e);
		}
	}

	async function loadContinuousContent() {
		if (!book) return;
		continuousLoading = true;
		try {
			const res = await fetch(`/api/books/${book.id}/continuous`);
			if (res.ok) {
				continuousContent = await res.text();
				cacheBook(book.id, continuousContent);
				loadContinuousToc();
			} else {
				console.error('Failed to load continuous content');
			}
			await tick();
			applyContinuousContentStyles();
			await restoreContinuousProgress();
		} catch (e) {
			console.error('Failed to load continuous content:', e);
		} finally {
			continuousLoading = false;
		}
	}

	async function restoreContinuousProgress() {
		if (!savedProgress?.percent) {
			setCurrentProgress(0);
			return;
		}

		setCurrentProgress(savedProgress.percent);
		if (!continuousScrollEl) return;

		isRestoringContinuousProgress = true;

		try {
			await tick();

			for (let attempt = 0; attempt < 8; attempt++) {
				if (!continuousScrollEl) {
					return;
				}

				await new Promise((resolve) => requestAnimationFrame(() => resolve(undefined)));

				const maxScroll = continuousScrollEl.scrollHeight - continuousScrollEl.clientHeight;
				if (maxScroll <= 0) {
					continue;
				}

				const targetScrollTop = (savedProgress.percent / 100) * maxScroll;
				continuousScrollEl.scrollTop = targetScrollTop;
				await new Promise((resolve) => requestAnimationFrame(() => resolve(undefined)));

				if (Math.abs(continuousScrollEl.scrollTop - targetScrollTop) <= 2) {
					return;
				}
			}
		} finally {
			setTimeout(() => {
				isRestoringContinuousProgress = false;
			}, 150);
		}
	}

	function getResumeCfi(): string | null {
		if (savedProgress?.cfi) {
			return savedProgress.cfi;
		}

		if (
			epubInstance?.locations &&
			typeof savedProgress?.percent === 'number' &&
			savedProgress.percent > 0
		) {
			return epubInstance.locations.cfiFromPercentage(
				Math.max(0, Math.min(1, savedProgress.percent / 100))
			);
		}

		return null;
	}

	async function loadContinuousToc() {
		if (!book) return;
		try {
			const res = await fetch(`/api/books/${book.id}/continuous/toc`);
			if (res.ok) {
				continuousToc = await res.json();
			}
		} catch (e) {
			console.error('Failed to load TOC:', e);
		}
	}

	function handleContinuousScroll() {
		if (!continuousScrollEl || isRestoringContinuousProgress) return;
		const { scrollTop, scrollHeight, clientHeight } = continuousScrollEl;
		const maxScroll = scrollHeight - clientHeight;
		if (maxScroll > 0) {
			setCurrentProgress((scrollTop / maxScroll) * 100);
			queueProgressSave();
		}
	}

	function resetToDefaults() {
		readerSettings.resetToDefaults('epub');
	}

	onMount(() => {
		const handlePageExit = () => {
			flushProgressSave();
			void endSession(true);
		};

		window.addEventListener('pagehide', handlePageExit);
		window.addEventListener('beforeunload', handlePageExit);
		window.addEventListener('wheel', handleWheelNavigation, { passive: false });

		return () => {
			window.removeEventListener('pagehide', handlePageExit);
			window.removeEventListener('beforeunload', handlePageExit);
			window.removeEventListener('wheel', handleWheelNavigation);
			window.removeEventListener('keydown', handleKeydown);
			cleanup();
		};
	});

		$effect(() => {
		if (bookLoaded && book && !loading) {
			tick().then(async () => {
				initialProcessing = true;
				processingMessage = 'Loading book content...';
				await fetchProgress();

				// Continuous mode uses the converted HTML reader and restores by percent.
				// Paginated EPUB mode uses epub.js and restores by CFI.
				if (bookFormat !== 'epub' || settings.continuousMode) {
					settings = { ...settings, continuousMode: true };
					await preloadBookContent();
				} else {
					await initReader();
				}

				initialProcessing = false;
				if (settings.continuousMode && continuousContent) {
					await tick();
					applyContinuousContentStyles();
					await restoreContinuousProgress();
				}
			});
		}
	});

	$effect(() => {
		const cm = settings.continuousMode;
		const b = book;
		const bl = bookLoaded;
		const loading2 = loading;
		const cc = continuousContent;
		const ip = initialProcessing;
		// Only use cache when not initial processing
		if (cm && b && bl && !loading2 && !cc && !ip) {
			const cached = getCachedBook(b.id);
			if (cached) {
				continuousContent = cached;
				tick().then(async () => {
					applyContinuousContentStyles();
					await restoreContinuousProgress();
				});
			} else {
				loadContinuousContent();
			}
		}
	});
</script>

<svelte:head>
	<title>{book?.title || 'Reading'} - Cryptorum</title>
	<link rel="stylesheet" href="/fonts/spectral.css" />
</svelte:head>

<div
	class="epub-reader"
	style="background-color: {epubThemes.find(t => t.id === settings.theme)?.bg || '#111111'};"
>
	{#if initialProcessing}
		<div class="initial-loading-overlay">
			<div class="initial-loading-content">
				<div class="initial-loading-logo">
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
						<path d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
					</svg>
				</div>
				<p class="initial-loading-title">{book?.title || 'Loading...'}</p>
				<div class="initial-loading-spinner"></div>
				<p class="initial-loading-message">{processingMessage}</p>
			</div>
		</div>
	{/if}

	<!-- Top Navigation Bar -->
	<header class="top-nav">
		<div class="nav-left">
			<a
				href={book ? `/book/${book.id}` : '/book'}
				class="nav-btn nav-close"
				title="Close (Esc)"
				onclick={closeReader}
			>
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<line x1="18" y1="6" x2="6" y2="18"></line>
					<line x1="6" y1="6" x2="18" y2="18"></line>
				</svg>
			</a>

			<button
				onclick={toggleLeftSidebar}
				class="nav-btn"
				class:active={leftSidebarOpen}
				title="Toggle Sidebar"
			>
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<line x1="3" y1="6" x2="21" y2="6"></line>
					<line x1="3" y1="12" x2="21" y2="12"></line>
					<line x1="3" y1="18" x2="21" y2="18"></line>
				</svg>
			</button>

			<button
				onclick={() => { activeSidebarTab = 'search'; leftSidebarOpen = true; rightSidebarOpen = false; }}
				class="nav-btn"
				title="Search (Ctrl+F)"
			>
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="11" cy="11" r="8"></circle>
					<line x1="21" y1="21" x2="16.65" y2="16.65"></line>
				</svg>
			</button>

			<div class="nav-divider"></div>

			<div class="page-controls">
				<button onclick={goPrev} class="nav-btn" disabled={!canPrev} title="Previous Page">
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<polyline points="15 18 9 12 15 6"></polyline>
					</svg>
				</button>

				<button onclick={goNext} class="nav-btn" disabled={!canNext} title="Next Page">
					<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<polyline points="9 18 15 12 9 6"></polyline>
					</svg>
				</button>
			</div>

			<div class="nav-divider"></div>

			<button
				onclick={toggleBookmark}
				class="nav-btn"
				class:active={isBookmarked}
				title="Toggle Bookmark (B)"
			>
				<svg class="icon" viewBox="0 0 24 24" fill={isBookmarked ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2">
					<path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"></path>
				</svg>
			</button>
		</div>

		<div class="nav-center">
			<span class="book-title">{book?.title || 'Loading...'}</span>
			{#if currentChapter}
				<span class="chapter-title">{currentChapter}</span>
			{/if}
		</div>

		<div class="nav-right">
			<button onclick={toggleRightSidebar} class="nav-btn" class:active={rightSidebarOpen} title="Settings">
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="3"></circle>
					<path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
				</svg>
			</button>

			<button onclick={toggleFullscreen} class="nav-btn" title="Fullscreen">
				<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<polyline points="15 3 21 3 21 9"></polyline>
					<polyline points="9 21 3 21 3 15"></polyline>
					<line x1="21" y1="3" x2="14" y2="10"></line>
					<line x1="3" y1="21" x2="10" y2="14"></line>
				</svg>
			</button>
		</div>

		<div
			class="progress-bar"
			onclick={(e) => handleProgressBarClick(e)}
			role="slider"
			aria-label="Reading progress"
			aria-valuemin="0"
			aria-valuemax="100"
			aria-valuenow={Math.max(0, Math.min(100, Math.round(progress)))}
			tabindex="0"
			onkeydown={handleProgressBarKeydown}
		>
			<div class="progress-fill" style="--progress: {progress}%;"></div>
			<div
				class="progress-thumb"
				style="left: calc({progress}% - 6px);"
				onmousedown={(e) => handleProgressThumbMouseDown(e)}
				role="presentation"
			></div>
		</div>
	</header>

	<!-- Main Content Area -->
	<div class="main-content">
		<!-- Left Sidebar -->
		<aside class="left-sidebar" class:open={leftSidebarOpen}>
			<div class="sidebar-tabs">
				<button
					class="sidebar-tab"
					class:active={activeSidebarTab === 'toc'}
					onclick={() => activeSidebarTab = 'toc'}
					title="Table of Contents"
				>
					<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<line x1="8" y1="6" x2="21" y2="6"></line>
						<line x1="8" y1="12" x2="21" y2="12"></line>
						<line x1="8" y1="18" x2="21" y2="18"></line>
						<line x1="3" y1="6" x2="3.01" y2="6"></line>
						<line x1="3" y1="12" x2="3.01" y2="12"></line>
						<line x1="3" y1="18" x2="3.01" y2="18"></line>
					</svg>
				</button>
				<button
					class="sidebar-tab"
					class:active={activeSidebarTab === 'bookmarks'}
					onclick={() => activeSidebarTab = 'bookmarks'}
					title="Bookmarks"
				>
					<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"></path>
					</svg>
				</button>
				<button
					class="sidebar-tab"
					class:active={activeSidebarTab === 'search'}
					onclick={() => activeSidebarTab = 'search'}
					title="Search"
				>
					<svg class="icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<circle cx="11" cy="11" r="8"></circle>
						<line x1="21" y1="21" x2="16.65" y2="16.65"></line>
					</svg>
				</button>
			</div>

			<div class="sidebar-content">
				{#if activeSidebarTab === 'toc'}
					<div class="toc-panel">
						{#snippet tocItems(items: any[], depth: number)}
							{#each items as item}
								<li class="outline-item" style="padding-left: {depth * 12}px">
									<button onclick={() => goToTocItem(item)} class="outline-btn" title={item.label || item.href}>
										{item.label || item.href || 'Untitled'}
									</button>
									{#if item.children && item.children.length > 0}
										<ul class="outline-list">
											{@render tocItems(item.children, depth + 1)}
										</ul>
									{/if}
									{#if item.subitems && item.subitems.length > 0}
										<ul class="outline-list">
											{@render tocItems(item.subitems, depth + 1)}
										</ul>
									{/if}
								</li>
							{/each}
						{/snippet}

						{#if settings.continuousMode}
							{#if continuousToc.length > 0}
								<ul class="outline-list">
									{@render tocItems(continuousToc, 0)}
								</ul>
							{:else}
								<p class="empty-message">No table of contents</p>
							{/if}
						{:else}
							{#if epubToc.length > 0}
								<ul class="outline-list">
									{@render tocItems(epubToc, 0)}
								</ul>
							{:else}
								<p class="empty-message">No table of contents</p>
							{/if}
						{/if}
					</div>
				{:else if activeSidebarTab === 'bookmarks'}
					<div class="bookmarks-panel">
						<p class="empty-message">No bookmarks yet</p>
					</div>
				{:else if activeSidebarTab === 'search'}
					<div class="search-panel">
						<div class="search-input-wrap">
							<input
								type="text"
								bind:value={searchQuery}
								onkeydown={(e) => e.key === 'Enter' && performSearch()}
								placeholder="Search..."
								class="search-input"
							/>
							<div class="search-count">
								{#if searchResults.length > 0}
									<span>{currentSearchResult}</span>
									<span class="sep">/</span>
									<span>{searchResults.length}</span>
								{/if}
							</div>
						</div>
						<div class="search-nav">
							<button onclick={prevSearchResult} class="nav-btn-sm" disabled={currentSearchResult <= 1} aria-label="Previous search result">
								<svg class="icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
									<polyline points="18 15 12 9 6 15"></polyline>
								</svg>
							</button>
							<button onclick={nextSearchResult} class="nav-btn-sm" disabled={currentSearchResult >= searchResults.length} aria-label="Next search result">
								<svg class="icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
									<polyline points="6 9 12 15 18 9"></polyline>
								</svg>
							</button>
						</div>
						{#if isSearching}
							<p class="search-status">Searching...</p>
						{:else if searchResults.length === 0 && searchQuery}
							<p class="search-status">No results found</p>
						{/if}
					</div>
				{/if}
			</div>
		</aside>

		<!-- EPUB Container - Both modes rendered, show/hide based on mode -->
		<div
			id="continuous-container"
			class="continuous-container"
			class:hidden-mode={!settings.continuousMode}
			onclick={() => { if (leftSidebarOpen || rightSidebarOpen) { leftSidebarOpen = false; rightSidebarOpen = false; } }}
			role="button"
			tabindex="0"
			aria-label="Close sidebars"
			onkeydown={(e) => { if (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') { e.preventDefault(); leftSidebarOpen = false; rightSidebarOpen = false; } }}
		>
			<div class="continuous-content" bind:this={continuousScrollEl} onscroll={handleContinuousScroll}>
				{#if continuousContent}
					{@html continuousContent}
				{:else if continuousLoading || initialProcessing}
					<div class="loading-spinner"></div>
				{:else}
					<p class="error-text">No content loaded. Please try refreshing.</p>
				{/if}
			</div>
		</div>

		<div id="epub-container" class="epub-container" class:hidden-mode={settings.continuousMode}>
			{#if loading}
				<div class="loading-spinner"></div>
			{:else if error}
				<div class="error-message">
					<p>{error}</p>
					<a href="/book/{book?.id}" class="btn">Return to Library</a>
				</div>
			{:else}
				<!-- Tap zones for navigation in paged mode -->
				{#if settings.flow === 'paginated'}
					<button
						class="tap-zone tap-prev"
						onclick={goPrev}
						disabled={!canPrev}
						aria-label="Previous page"
					></button>
					<button
						class="tap-zone tap-next"
						onclick={goNext}
						disabled={!canNext}
						aria-label="Next page"
					></button>
					<button
						class="floating-nav floating-prev"
						onclick={goPrev}
						aria-label="Previous page"
						disabled={!canPrev}
					>
						<svg class="icon-lg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polyline points="15 18 9 12 15 6"></polyline>
						</svg>
					</button>
					<button
						class="floating-nav floating-next"
						onclick={goNext}
						aria-label="Next page"
						disabled={!canNext}
					>
						<svg class="icon-lg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polyline points="9 18 15 12 9 6"></polyline>
						</svg>
					</button>
				{/if}
				<!-- Click to close sidebar overlay -->
				{#if leftSidebarOpen || rightSidebarOpen}
					<button
						class="sidebar-close-overlay"
						onclick={() => { leftSidebarOpen = false; rightSidebarOpen = false; }}
						aria-label="Close sidebar"
					></button>
				{/if}
			{/if}
		</div>

		<!-- Right Sidebar - Settings -->
		<aside class="right-sidebar" class:open={rightSidebarOpen}>
			<div class="settings-tabs">
				<button
					class="settings-tab"
					class:active={activeSettingsTab === 'typography'}
					onclick={() => activeSettingsTab = 'typography'}
				>
					Typography
				</button>
				<button
					class="settings-tab"
					class:active={activeSettingsTab === 'display'}
					onclick={() => activeSettingsTab = 'display'}
				>
					Display
				</button>
			</div>

			<div class="settings-content">
				{#if activeSettingsTab === 'typography'}
					<div class="settings-section">
						<div class="settings-label">Font Family</div>
						<div class="grid grid-cols-2 gap-2">
							{#each fontFamilies as font}
								<button
									onclick={() => updateSetting('fontFamily', font.id)}
									class="font-btn"
									class:active={settings.fontFamily === font.id}
									style="font-family: {font.family};"
								>
									{font.name}
								</button>
							{/each}
						</div>
					</div>

					<div class="settings-section">
						<div class="settings-label">Font Size: {settings.fontSize}px</div>
						<div class="range-row">
							<button onclick={() => updateSetting('fontSize', Math.max(10, settings.fontSize - 1))} class="range-btn">−</button>
							<input
								type="range"
								min="10"
								max="32"
								value={settings.fontSize}
								oninput={(e) => updateSetting('fontSize', parseInt(e.currentTarget.value))}
								class="range-input"
							/>
							<button onclick={() => updateSetting('fontSize', Math.min(32, settings.fontSize + 1))} class="range-btn">+</button>
						</div>
					</div>

					<div class="settings-section">
						<div class="settings-label">Line Height: {settings.lineHeight.toFixed(1)}</div>
						<div class="range-row">
							<button onclick={() => updateSetting('lineHeight', Math.max(1.0, settings.lineHeight - 0.1))} class="range-btn">−</button>
							<input
								type="range"
								min="1.0"
								max="2.2"
								step="0.1"
								value={settings.lineHeight}
								oninput={(e) => updateSetting('lineHeight', parseFloat(e.currentTarget.value))}
								class="range-input"
							/>
							<button onclick={() => updateSetting('lineHeight', Math.min(2.2, settings.lineHeight + 0.1))} class="range-btn">+</button>
						</div>
					</div>

					<div class="settings-section">
						<div class="settings-label">Text Alignment</div>
						<div class="button-group">
							<button
								onclick={() => updateSetting('justify', false)}
								class="option-btn"
								class:active={settings.justify === false}
							>
								Left
							</button>
							<button
								onclick={() => updateSetting('justify', true)}
								class="option-btn"
								class:active={settings.justify === true}
							>
								Justified
							</button>
						</div>
					</div>

				{:else if activeSettingsTab === 'display'}
					<div class="settings-section">
						<div class="settings-label">Theme</div>
						<div class="theme-grid">
							{#each epubThemes as theme}
								<button
									onclick={() => updateSetting('theme', theme.id)}
									class="theme-btn"
									class:active={settings.theme === theme.id}
								>
									<ThemePreviewSwatch background={theme.bg} foreground={theme.text} sizeClass="h-8 w-8" />
									<span>{theme.name}</span>
								</button>
							{/each}
							{#if appTheme?.appearance.customThemes?.length}
								{#each appTheme.appearance.customThemes as customTheme}
									<button
										onclick={() => updateSetting('theme', customTheme.id)}
										class="theme-btn"
										class:active={settings.theme === customTheme.id}
									>
										<ThemePreviewSwatch background={customTheme.background} foreground={customTheme.foreground} sizeClass="h-8 w-8" />
										<span>{customTheme.name}</span>
									</button>
								{/each}
							{/if}
						</div>
					</div>

					<div class="settings-section">
						<div class="settings-label">Max Width</div>
						<div class="max-width-control">
							<button
								onclick={() => updateSetting('continuousMaxWidth', Math.max(200, settings.continuousMaxWidth - 40))}
								class="max-width-btn"
							>
								−
							</button>
							<input
								type="text"
								value={settings.continuousMaxWidth}
								onchange={(e) => {
									const val = parseInt(e.currentTarget.value);
									if (!isNaN(val) && val >= 200 && val <= 1600) {
										updateSetting('continuousMaxWidth', val);
									} else {
										e.currentTarget.value = settings.continuousMaxWidth.toString();
									}
								}}
								class="max-width-input"
							/>
							<button
								onclick={() => updateSetting('continuousMaxWidth', Math.min(1600, settings.continuousMaxWidth + 40))}
								class="max-width-btn"
							>
								+
							</button>
						</div>
					</div>

					<div class="settings-section">
						<div class="settings-label">Brightness: {settings.brightness}%</div>
						<input
							type="range"
							min="50"
							max="150"
							value={settings.brightness}
							oninput={(e) => updateSetting('brightness', parseInt(e.currentTarget.value))}
							class="range-input-full"
						/>
					</div>

					<div class="settings-section">
						<div class="settings-label">Columns</div>
						<div class="button-group">
							{#each [1, 2] as cols}
								<button
									onclick={() => updateSetting('maxColumnCount', cols)}
									class="option-btn"
									class:active={settings.maxColumnCount === cols}
								>
									{cols}
								</button>
							{/each}
						</div>
					</div>

					{#if bookFormat === 'epub'}
					<div class="settings-section">
						<div class="settings-label">Reading Mode</div>
						<div class="button-group">
							<button
								onclick={() => { updateSetting('continuousMode', false); updateSetting('flow', 'paginated'); }}
								class="option-btn"
								class:active={!settings.continuousMode}
							>
								Paged
							</button>
							<button
								onclick={() => { updateSetting('continuousMode', true); updateSetting('flow', 'scrolled'); }}
								class="option-btn"
								class:active={settings.continuousMode}
							>
								Scrolled
							</button>
						</div>
					</div>
					{/if}

					<div class="settings-section">
						<div class="settings-label">Images</div>
						<div class="button-group">
							<button
								onclick={() => updateSetting('showImages', true)}
								class="option-btn"
								class:active={settings.showImages !== false}
							>
								Show
							</button>
							<button
								onclick={() => updateSetting('showImages', false)}
								class="option-btn"
								class:active={settings.showImages === false}
							>
								Hide
							</button>
						</div>
					</div>

					{#if settings.showImages !== false}
						<div class="settings-section">
							<div class="settings-label">Image Size</div>
							<div class="button-group">
								<button
									onclick={() => updateSetting('imageSize', 'fit-width')}
									class="option-btn"
									class:active={settings.imageSize === 'fit-width' || !settings.imageSize}
								>
									Width
								</button>
								<button
									onclick={() => updateSetting('imageSize', 'fit-page')}
									class="option-btn"
									class:active={settings.imageSize === 'fit-page'}
								>
									Page
								</button>
								<button
									onclick={() => updateSetting('imageSize', 'actual-size')}
									class="option-btn"
									class:active={settings.imageSize === 'actual-size'}
								>
									Actual
								</button>
							</div>
						</div>

						<div class="settings-section">
							<div class="settings-label">Image Filter</div>
							<div class="button-group">
								<button
									onclick={() => updateSetting('imageGrayscale', false)}
									class="option-btn"
									class:active={!settings.imageGrayscale}
								>
									Color
								</button>
								<button
									onclick={() => updateSetting('imageGrayscale', true)}
									class="option-btn"
									class:active={settings.imageGrayscale}
								>
									Grayscale
								</button>
							</div>
						</div>
					{/if}

					<div class="settings-section">
						<div class="settings-label">Layout</div>
						<div class="button-group">
							<button
								onclick={() => updateSetting('originalLayout', false)}
								class="option-btn"
								class:active={!settings.originalLayout}
							>
								Custom
							</button>
							<button
								onclick={() => updateSetting('originalLayout', true)}
								class="option-btn"
								class:active={settings.originalLayout}
							>
								Original
							</button>
						</div>
					</div>
				{/if}
			</div>
		</aside>
	</div>
</div>

<style>
	.epub-reader {
		position: fixed;
		inset: 0;
		z-index: 9999;
		display: flex;
		flex-direction: column;
		font-family: system-ui, -apple-system, sans-serif;
		overflow: hidden;
	}

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
	}
	.nav-right { flex: 1; justify-content: flex-end; }

	.nav-btn {
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
		transition: background-color 0.15s;
	}

	.nav-btn:hover { background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85)); }
	.nav-btn:disabled { opacity: 0.3; cursor: not-allowed; }
	.nav-btn.active { background: var(--color-primary-500, #22c55e); color: white; }

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
		transition: background-color 0.15s;
		text-decoration: none;
	}

	.nav-close:hover { background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85)); }

	.nav-divider {
		width: 1px;
		height: 24px;
		background: var(--color-surface-border, rgba(55, 65, 81, 0.6));
		margin: 0 8px;
	}

	.page-controls { display: flex; align-items: center; gap: 4px; }

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

	.icon { width: 20px; height: 20px; }
	.icon-sm { width: 16px; height: 16px; }
	.icon-xs { width: 12px; height: 12px; }

	.progress-bar {
		position: absolute;
		bottom: 0;
		left: 0;
		right: 0;
		height: 12px;
		background: transparent;
		cursor: pointer;
		display: flex;
		align-items: flex-end;
	}

	.progress-fill {
		position: absolute;
		left: 0;
		bottom: 0;
		height: 3px;
		background: var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 2px;
		width: 100%;
	}

	.progress-fill::after {
		content: '';
		position: absolute;
		left: 0;
		top: 0;
		height: 100%;
		width: var(--progress, 0%);
		background: var(--color-primary-500, #22c55e);
		border-radius: 2px;
		transition: width 0.1s ease;
	}

	.progress-thumb {
		position: absolute;
		bottom: -4px;
		width: 12px;
		height: 12px;
		background: var(--color-primary-500, #22c55e);
		border: 2px solid var(--color-surface-base, #0f172a);
		border-radius: 50%;
		cursor: grab;
		z-index: 10;
		transition: left 0.05s ease, transform 0.1s ease;
	}

	.progress-thumb:hover { transform: scale(1.2); }

	.main-content {
		flex: 1;
		display: flex;
		overflow: hidden;
		position: relative;
	}

	.left-sidebar {
		position: absolute;
		left: 0;
		top: 0;
		bottom: 0;
		width: 300px;
		display: flex;
		flex-direction: column;
		background: var(--color-surface-base, #0f172a);
		border-right: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		transform: translateX(-100%);
		transition: transform 0.25s ease-in-out;
		z-index: 50;
	}

	.left-sidebar.open { transform: translateX(0); }

	.sidebar-tabs {
		display: flex;
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
	}

	.sidebar-tab {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 12px;
		border: none;
		background: transparent;
		color: var(--color-surface-text-muted, #94a3b8);
		cursor: pointer;
		transition: all 0.15s;
	}

	.sidebar-tab:hover { color: var(--color-surface-text, #e2e8f0); }
	.sidebar-tab.active {
		color: var(--color-primary-500, #22c55e);
		box-shadow: inset 0 -2px 0 var(--color-primary-500, #22c55e);
	}

	.sidebar-content { flex: 1; overflow-y: auto; }

	.toc-panel,
	.bookmarks-panel,
	.search-panel { padding: 12px; }

	.outline-list { list-style: none; padding: 0; margin: 0; }
	.outline-item { margin-bottom: 4px; }

	.outline-btn {
		width: 100%;
		text-align: left;
		padding: 8px 12px;
		border: none;
		border-radius: 4px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 13px;
		cursor: pointer;
		transition: background-color 0.15s;
	}

	.outline-btn:hover { background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85)); }

	.empty-message {
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 13px;
		text-align: center;
		padding: 20px;
	}

	.search-input-wrap {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 8px;
	}

	.search-input {
		flex: 1;
		padding: 8px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 4px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text, #e2e8f0);
		font-size: 13px;
	}

	.search-input:focus { outline: none; border-color: var(--color-primary-500, #22c55e); }

	.search-count { font-size: 12px; color: var(--color-surface-text-muted, #94a3b8); }
	.search-count .sep { margin: 0 2px; }

	.search-nav { display: flex; gap: 4px; }

	.nav-btn-sm {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		border: none;
		border-radius: 4px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text, #e2e8f0);
		cursor: pointer;
	}

	.nav-btn-sm:hover:not(:disabled) { background: var(--color-primary-500, #22c55e); }
	.nav-btn-sm:disabled { opacity: 0.3; cursor: not-allowed; }

	.search-status { font-size: 12px; color: var(--color-surface-text-muted, #94a3b8); text-align: center; margin-top: 8px; }

	.continuous-container {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
		position: relative;
		min-height: 0;
		height: 100%;
	}

	.continuous-content {
		flex: 1;
		overflow-y: scroll;
		scrollbar-width: none;
		-ms-overflow-style: none;
		width: 100%;
		height: 100%;
		box-sizing: border-box;
	}

	.continuous-content::-webkit-scrollbar {
		display: none;
	}

	.continuous-content :global(body) {
		margin: 0;
		padding: 0;
		color: inherit;
	}

	.continuous-content :global(p) {
		margin-bottom: 1em;
		color: inherit;
	}

	.continuous-content :global(br) {
		display: block;
		content: "";
		margin: 0.5em 0;
	}

	.continuous-content :global(img) {
		max-width: 100%;
		height: auto;
	}

	.error-text {
		color: #ef4444;
		text-align: center;
		padding: 20px;
	}

	.epub-container {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		overflow: auto;
		position: relative;
	}

	.hidden-mode {
		display: none !important;
	}

	.loading-spinner {
		width: 48px;
		height: 48px;
		border: 3px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-top-color: var(--color-primary-500, #22c55e);
		border-radius: 50%;
		animation: spin 1s linear infinite;
	}

	.floating-nav {
		position: absolute;
		top: 50%;
		transform: translateY(-50%);
		width: 48px;
		height: 80px;
		display: flex;
		align-items: center;
		justify-content: center;
		border: none;
		border-radius: 8px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text, #e2e8f0);
		cursor: pointer;
		transition: all 0.15s;
		z-index: 10;
	}

	.floating-nav:hover:not(:disabled) { background: var(--color-primary-500, #22c55e); }
	.floating-nav:disabled { opacity: 0.2; cursor: not-allowed; }

	.floating-prev { left: 16px; }
	.floating-next { right: 16px; }

	.tap-zone {
		position: absolute;
		top: 0;
		bottom: 0;
		width: 25%;
		z-index: 5;
		background: transparent;
		border: none;
		cursor: pointer;
	}

	.tap-prev { left: 0; cursor: w-resize; }
	.tap-next { right: 0; cursor: e-resize; }

	.sidebar-close-overlay {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		z-index: 40;
		background: transparent;
		border: none;
		cursor: pointer;
	}

	@keyframes spin { to { transform: rotate(360deg); } }

	.error-message { text-align: center; }
	.error-message p { color: #ef4444; margin-bottom: 16px; }

	.btn {
		display: inline-block;
		padding: 8px 16px;
		background: var(--color-primary-500, #22c55e);
		color: white;
		border-radius: 6px;
		text-decoration: none;
	}

	.right-sidebar {
		position: absolute;
		right: 0;
		top: 0;
		bottom: 0;
		width: 400px;
		display: flex;
		flex-direction: column;
		background: var(--color-surface-base, #0f172a);
		border-left: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		transform: translateX(100%);
		transition: transform 0.25s ease-in-out;
		z-index: 50;
	}

	.right-sidebar.open { transform: translateX(0); }

	.settings-tabs {
		display: flex;
		border-bottom: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
	}

	.settings-tab {
		flex: 1;
		padding: 12px 8px;
		border: none;
		background: transparent;
		color: var(--color-surface-text-muted, #94a3b8);
		font-size: 12px;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s;
	}

	.settings-tab:hover { color: var(--color-surface-text, #e2e8f0); }
	.settings-tab.active {
		color: var(--color-primary-500, #22c55e);
		box-shadow: inset 0 -2px 0 var(--color-primary-500, #22c55e);
	}

	.settings-content {
		flex: 1;
		overflow-y: auto;
		padding: 16px;
	}

	.settings-section {
		margin-bottom: 20px;
	}

	.settings-label {
		display: block;
		font-size: 12px;
		color: var(--color-surface-text-muted, #94a3b8);
		margin-bottom: 8px;
	}

	.grid { display: grid; gap: 8px; }
	.grid-cols-2 { grid-template-columns: repeat(2, 1fr); }

	.font-btn {
		padding: 10px 8px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 6px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 12px;
		cursor: pointer;
		transition: all 0.15s;
	}

	.font-btn:hover { border-color: var(--color-primary-500, #22c55e); }
	.font-btn.active {
		border-color: var(--color-primary-500, #22c55e);
		background: rgba(34, 197, 94, 0.1);
	}

	.range-row {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.range-btn {
		width: 32px;
		height: 32px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 6px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 16px;
		cursor: pointer;
		transition: all 0.15s;
	}

	.range-btn:hover { border-color: var(--color-primary-500, #22c55e); }

	.range-input {
		flex: 1;
		height: 4px;
		appearance: none;
		background: var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 2px;
		cursor: pointer;
	}

	.range-input::-webkit-slider-thumb {
		appearance: none;
		width: 16px;
		height: 16px;
		background: var(--color-primary-500, #22c55e);
		border-radius: 50%;
		cursor: pointer;
	}

	.range-input-full {
		width: 100%;
		height: 4px;
		appearance: none;
		background: var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 2px;
		cursor: pointer;
	}

	.range-input-full::-webkit-slider-thumb {
		appearance: none;
		width: 16px;
		height: 16px;
		background: var(--color-primary-500, #22c55e);
		border-radius: 50%;
		cursor: pointer;
	}

	.button-group {
		display: flex;
		gap: 4px;
	}

	.option-btn {
		flex: 1;
		padding: 8px 12px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 6px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 12px;
		cursor: pointer;
		transition: all 0.15s;
	}

	.option-btn:hover { border-color: var(--color-primary-500, #22c55e); }
	.option-btn.active {
		border-color: var(--color-primary-500, #22c55e);
		background: rgba(34, 197, 94, 0.1);
	}

	.theme-grid {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 8px;
	}

	.theme-btn {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 4px;
		padding: 8px;
		border: 2px solid transparent;
		border-radius: 6px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 10px;
		cursor: pointer;
		transition: all 0.15s;
	}

	.theme-btn:hover { border-color: var(--color-surface-border, rgba(55, 65, 81, 0.6)); }
	.theme-btn.active { border-color: var(--color-primary-500, #22c55e); }

	.max-width-control {
		display: flex;
		align-items: center;
		gap: 4px;
	}

	.max-width-btn {
		width: 32px;
		height: 32px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 6px;
		background: transparent;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 16px;
		cursor: pointer;
		transition: all 0.15s;
	}

	.max-width-btn:hover { border-color: var(--color-primary-500, #22c55e); }

	.max-width-input {
		width: 60px;
		height: 32px;
		border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-radius: 6px;
		background: var(--color-surface-overlay, rgba(15, 23, 42, 0.85));
		color: var(--color-surface-text, #e2e8f0);
		font-size: 14px;
		text-align: center;
		padding: 0 4px;
	}

	.max-width-input:focus {
		outline: none;
		border-color: var(--color-primary-500, #22c55e);
	}

	.initial-loading-overlay {
		position: fixed;
		inset: 0;
		z-index: 9999;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--color-surface-base, #0f172a);
	}

	.initial-loading-content {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 16px;
		text-align: center;
		max-width: 300px;
	}

	.initial-loading-logo {
		width: 64px;
		height: 64px;
		color: var(--color-primary-500, #22c55e);
		opacity: 0.8;
	}

	.initial-loading-logo svg {
		width: 100%;
		height: 100%;
	}

	.initial-loading-title {
		font-size: 16px;
		font-weight: 500;
		color: var(--color-surface-text, #e2e8f0);
		margin: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 100%;
	}

	.initial-loading-spinner {
		width: 32px;
		height: 32px;
		border: 3px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		border-top-color: var(--color-primary-500, #22c55e);
		border-radius: 50%;
		animation: spin 1s linear infinite;
	}

	.initial-loading-message {
		font-size: 14px;
		color: var(--color-surface-text-muted, #94a3b8);
		margin: 0;
	}
</style>

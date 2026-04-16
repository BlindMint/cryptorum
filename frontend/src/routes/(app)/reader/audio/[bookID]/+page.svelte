<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { browser } from '$app/environment';
	import { readerSettings, waveformStyles, skipIntervalOptions, sleepTimerOptions, type AudioReaderSetting } from '$lib/stores/readerSettings';
	import { normalizeBookFormat } from '$lib/utils/book-formats';

	let book = $state<any>(null);
	let loading = $state(true);
	let audioElement = $state<HTMLAudioElement | null>(null);
	let isPlaying = $state(false);
	let currentTime = $state(0);
	let duration = $state(0);
	let playbackRate = $state(1);
	let showControls = $state(true);
	let showSettings = $state(false);
	let controlsTimeout: ReturnType<typeof setTimeout> | null = null;
	let chapters: any[] = [];
	let currentChapter = $state(0);
	let sleepTimerRemaining = $state<number | null>(null);
	let sleepTimerInterval: ReturnType<typeof setInterval> | null = null;
	let waveformCanvas = $state<HTMLCanvasElement | null>(null);
	let waveformCtx: CanvasRenderingContext2D | null = null;
	let audioContext: AudioContext | null = null;
	let analyser: AnalyserNode | null = null;
	let audioData = $state<Uint8Array<ArrayBuffer> | null>(null);
	let savedProgress = $state<any>(null);
	let progressSaveTimeout: ReturnType<typeof setTimeout> | null = null;
	let currentSessionId = $state<number | null>(null);
	let sessionEnded = false;
	let handlePageExit: (() => void) | null = null;
	let requestedFormat = $state('');

	let settings = $state<AudioReaderSetting>({
		playbackSpeed: 1.0,
		skipForward: 15,
		skipBackward: 15,
		autoAdvance: false,
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
	});

	let activeSettingsTab = $state<'playback' | 'display' | 'chapters' | 'accessibility'>('playback');

	onMount(async () => {
		const bookId = $page.params.bookID;
		try {
			const res = await fetch(`/api/books/${bookId}`);
			if (res.ok) {
				book = await res.json();
				requestedFormat = normalizeBookFormat($page.url.searchParams.get('format'));
				await fetchProgress();
				await startSession();
			}
		} catch (e) {
			console.error('Failed to load book:', e);
		} finally {
			loading = false;
		}

		readerSettings.subscribe(s => {
			settings = { ...s.audio };
		});

		if (browser) {
			showControls = false;
		}

		handlePageExit = () => {
			void endSession(true);
		};
		window.addEventListener('pagehide', handlePageExit);
		window.addEventListener('beforeunload', handlePageExit);
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

	async function saveProgress() {
		if (!book || !duration) return;
		const percent = (currentTime / duration) * 100;
		try {
			await fetch(`/api/books/${book.id}/progress`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					percent: percent,
					status: percent >= 100 ? 'finished' : 'reading'
				})
			});
		} catch (e) {
			console.error('Failed to save progress:', e);
		}
	}

	function debouncedSaveProgress() {
		if (progressSaveTimeout) {
			clearTimeout(progressSaveTimeout);
		}
		progressSaveTimeout = setTimeout(() => {
			saveProgress();
		}, 2000);
	}

	async function startSession() {
		if (!book || !book.id) return;
		try {
			const res = await fetch(`/api/books/${book.id}/sessions`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ reader_type: 'audio' })
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
		if (handlePageExit) {
			window.removeEventListener('pagehide', handlePageExit);
			window.removeEventListener('beforeunload', handlePageExit);
		}
		void endSession(true);
	});

	function resetControlsTimer() {
		showControls = true;
		if (controlsTimeout) clearTimeout(controlsTimeout);
		controlsTimeout = setTimeout(() => {
			if (!showSettings) showControls = false;
		}, 3000);
	}

	function toggleControls() {
		showControls = !showControls;
		if (showControls) resetControlsTimer();
	}

	function updateSetting(key: string, value: any) {
		settings = { ...settings, [key]: value };
		readerSettings.updateAudio({ [key]: value });

		if (key === 'playbackSpeed' && audioElement) {
			audioElement.playbackRate = value;
		}
	}

	function togglePlay() {
		if (!audioElement) return;
		
		if (isPlaying) {
			audioElement.pause();
		} else {
			audioElement.play();
		}
		isPlaying = !isPlaying;
	}

	function handleTimeUpdate() {
		if (audioElement) {
			currentTime = audioElement.currentTime;
			updateWaveform();
			debouncedSaveProgress();
		}
	}

	function handleLoadedMetadata() {
		if (audioElement) {
			duration = audioElement.duration;
			setupAudioAnalysis();
			if (savedProgress && savedProgress.percent > 0 && savedProgress.percent < 100) {
				const resumeTime = (savedProgress.percent / 100) * duration;
				audioElement.currentTime = resumeTime;
			}
		}
	}

	function handleEnded() {
		isPlaying = false;
		saveProgress();
		if (settings.autoAdvance) {
		}
	}

	function seek(e: Event) {
		const target = e.target as HTMLInputElement;
		if (audioElement) {
			audioElement.currentTime = Number(target.value);
		}
	}

	function setPlaybackRate(rate: number) {
		playbackRate = rate;
		updateSetting('playbackSpeed', rate);
		if (audioElement) {
			audioElement.playbackRate = rate;
		}
	}

	function skipForward() {
		if (audioElement) {
			audioElement.currentTime = Math.min(audioElement.currentTime + settings.skipForward, duration);
		}
	}

	function skipBackward() {
		if (audioElement) {
			audioElement.currentTime = Math.max(audioElement.currentTime - settings.skipBackward, 0);
		}
	}

	function startSleepTimer() {
		if (sleepTimerInterval) {
			clearInterval(sleepTimerInterval);
			sleepTimerInterval = null;
		}

		let timerSeconds = 0;
		switch (settings.sleepTimer) {
			case '15min': timerSeconds = 15 * 60; break;
			case '30min': timerSeconds = 30 * 60; break;
			case '60min': timerSeconds = 60 * 60; break;
			case 'end-of-chapter': timerSeconds = duration - currentTime; break;
			case 'custom': timerSeconds = settings.sleepTimerCustom * 60; break;
			default: return;
		}

		sleepTimerRemaining = timerSeconds;
		sleepTimerInterval = setInterval(() => {
			if (sleepTimerRemaining !== null) {
					sleepTimerRemaining--;
					if (sleepTimerRemaining <= 0) {
						audioElement?.pause();
						isPlaying = false;
						clearInterval(sleepTimerInterval!);
					sleepTimerInterval = null;
					sleepTimerRemaining = null;
				}
			}
		}, 1000);
	}

	function cancelSleepTimer() {
		if (sleepTimerInterval) {
			clearInterval(sleepTimerInterval);
			sleepTimerInterval = null;
		}
		sleepTimerRemaining = null;
	}

	function setupAudioAnalysis() {
		if (!audioElement || !browser) return;
		
		try {
			audioContext = new AudioContext();
			analyser = audioContext.createAnalyser();
			analyser.fftSize = 256;
			
			const source = audioContext.createMediaElementSource(audioElement);
			source.connect(analyser);
			analyser.connect(audioContext.destination);
			
			audioData = new Uint8Array(analyser.frequencyBinCount);
		} catch (e) {
			console.error('Failed to setup audio analysis:', e);
		}
	}

	function updateWaveform() {
		if (!waveformCanvas || !analyser || !audioData) return;
		
		analyser.getByteFrequencyData(audioData);
		waveformCtx = waveformCanvas.getContext('2d');
		if (!waveformCtx) return;

		const width = waveformCanvas.width;
		const height = waveformCanvas.height;
		
		waveformCtx.clearRect(0, 0, width, height);
		waveformCtx.fillStyle = 'rgba(255, 255, 255, 0.1)';
		
		const barCount = settings.waveformStyle === 'bars' ? 64 : width;
		const barWidth = width / barCount;
		
		for (let i = 0; i < barCount; i++) {
			const dataIndex = Math.floor(i * audioData.length / barCount);
			const value = audioData[dataIndex] / 255;
			const barHeight = value * height * 0.8;
			
			if (settings.waveformStyle === 'bars') {
				waveformCtx.fillRect(i * barWidth, height - barHeight, barWidth - 1, barHeight);
			} else if (settings.waveformStyle === 'filled') {
				waveformCtx.fillRect(i * barWidth, height - barHeight, barWidth, barHeight);
			} else {
				waveformCtx.fillRect(i * barWidth, (height - barHeight) / 2, barWidth, barHeight);
			}
		}
	}

	function formatTime(seconds: number): string {
		if (isNaN(seconds)) return '0:00';
		const hrs = Math.floor(seconds / 3600);
		const mins = Math.floor((seconds % 3600) / 60);
		const secs = Math.floor(seconds % 60);
		if (hrs > 0) {
			return `${hrs}:${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
		}
		return `${mins}:${secs.toString().padStart(2, '0')}`;
	}

	function formatSleepTime(seconds: number): string {
		const mins = Math.floor(seconds / 60);
		const secs = seconds % 60;
		return `${mins}:${secs.toString().padStart(2, '0')}`;
	}

	function handleKeydown(e: KeyboardEvent) {
		if (showSettings && e.key === 'Escape') {
			showSettings = false;
			return;
		}

		if (e.key === ' ' || e.key === 'k') {
			e.preventDefault();
			togglePlay();
		} else if (e.key === 'ArrowLeft') {
			e.preventDefault();
			skipBackward();
		} else if (e.key === 'ArrowRight') {
			e.preventDefault();
			skipForward();
		} else if (e.key === 'Escape' && showSettings) {
			showSettings = false;
		} else if (e.key === 'Escape') {
			window.location.href = `/book/${book?.id}`;
		} else if ((e.ctrlKey || e.metaKey) && e.key === 's') {
			e.preventDefault();
			showSettings = !showSettings;
		}
	}

	function handleMouseMove(e: MouseEvent) {
		if (e.movementY !== 0) {
			resetControlsTimer();
		}
	}

	function resetToDefaults() {
		readerSettings.resetToDefaults('audio');
	}

	$effect(() => {
		if (audioElement && settings.playbackSpeed) {
			audioElement.playbackRate = settings.playbackSpeed;
		}
	});
</script>

<svelte:head>
	<title>{book?.title || 'Reading'} - Cryptorum</title>
</svelte:head>

<div 
	class="fixed inset-0 z-50 flex flex-col bg-slate-900"
	role="presentation"
	onmousemove={handleMouseMove}
>
	<!-- Top Bar -->
	<header 
		class="absolute top-0 left-0 right-0 h-14 flex items-center justify-between px-4 transition-opacity duration-200 z-20 {showControls ? 'opacity-100' : 'opacity-0 pointer-events-none'}"
		style="background: linear-gradient(to bottom, rgba(0,0,0,0.7), transparent);"
	>
		<a href="/book/{book?.id}" class="text-white/80 hover:text-white transition-colors" aria-label="Back to book details">
			<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
			</svg>
		</a>
		<div class="text-center max-w-md mx-auto">
			<h1 class="text-white font-medium truncate">{book?.title || 'Loading...'}</h1>
			{#if currentChapter > 0 && chapters.length > 0}
				<p class="text-white/60 text-sm truncate">{chapters[currentChapter]?.title || ''}</p>
			{/if}
		</div>
		<div class="flex items-center space-x-3">
			<button
				onclick={() => { showSettings = !showSettings; showControls = true; resetControlsTimer(); }}
				aria-label="Open audio settings"
				class="p-2 rounded-lg text-white/80 hover:text-white transition-colors"
				title="Settings (Ctrl+S)"
			>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
				</svg>
			</button>
		</div>
	</header>

	<!-- Main Content -->
	<div class="flex-1 flex items-center justify-center p-8">
		{#if loading}
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-amber-500"></div>
		{:else}
			<div class="w-full max-w-lg">
				<div class="bg-slate-800 rounded-2xl p-8 border border-slate-700">
					<!-- Cover / Waveform Area -->
					<div class="w-48 h-48 mx-auto mb-6 rounded-xl bg-slate-700 flex items-center justify-center overflow-hidden">
						{#if book?.cover_path}
							<img src="/api/covers/{book.id}" alt={book.title} class="w-full h-full object-cover" />
						{:else}
							<svg class="w-16 h-16 text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3"></path>
							</svg>
						{/if}
					</div>

					<audio
						bind:this={audioElement}
						ontimeupdate={handleTimeUpdate}
						onloadedmetadata={handleLoadedMetadata}
						onended={handleEnded}
						src={book?.id ? `/api/books/${book.id}/file${requestedFormat ? `?format=${encodeURIComponent(requestedFormat)}` : ''}` : ''}
					></audio>

					<!-- Waveform -->
					<canvas 
						bind:this={waveformCanvas}
						width="400" 
						height="60" 
						class="w-full h-15 mb-4 mx-auto"
					></canvas>

					<!-- Progress Bar -->
					<div class="mb-6">
						<input
							type="range"
							min="0"
							max={duration || 100}
							value={currentTime}
							oninput={seek}
							class="w-full h-2 bg-slate-700 rounded-lg appearance-none cursor-pointer"
						/>
						<div class="flex justify-between text-sm text-slate-400 mt-2">
							<span>{formatTime(currentTime)}</span>
							<span>{formatTime(duration)}</span>
						</div>
					</div>

					<!-- Playback Controls -->
					<div class="flex items-center justify-center space-x-6 mb-6">
						<button 
							onclick={skipBackward}
							class="text-white hover:text-amber-400 transition-colors"
							title="Skip Back ({settings.skipBackward}s)"
						>
							<svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12.066 11.2a1 1 0 000 1.6l5.334 4A1 1 0 0019 16V8a1 1 0 00-1.6-.8l-5.333 4zM4.066 11.2a1 1 0 000 1.6l5.334 4A1 1 0 0011 16V8a1 1 0 00-1.6-.8l-5.333 4z"></path>
							</svg>
						</button>

						<button
							onclick={togglePlay}
							class="w-16 h-16 rounded-full bg-amber-500 hover:bg-amber-600 text-white flex items-center justify-center transition-colors"
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
							onclick={skipForward}
							class="text-white hover:text-amber-400 transition-colors"
							title="Skip Forward ({settings.skipForward}s)"
						>
							<svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11.933 12.8a1 1 0 000-1.6L6.6 7.2A1 1 0 005 8v8a1 1 0 001.6.8l5.333-4zM19.933 12.8a1 1 0 000-1.6l-5.333-4A1 1 0 0013 8v8a1 1 0 001.6.8l5.333-4z"></path>
							</svg>
						</button>
					</div>

					<!-- Playback Speed -->
					<div class="flex items-center justify-center space-x-2 mb-4">
						{#each [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3] as rate}
							<button
								onclick={() => setPlaybackRate(rate)}
								class="px-2 py-1 rounded-lg text-sm transition-colors {playbackRate === rate ? 'bg-amber-500 text-white' : 'bg-slate-700 text-slate-300 hover:bg-slate-600'}"
							>
								{rate}x
							</button>
						{/each}
					</div>

					<!-- Sleep Timer Indicator -->
					{#if sleepTimerRemaining !== null}
						<div class="text-center mb-4">
							<span class="text-amber-500 text-sm">
								Sleep timer: {formatSleepTime(sleepTimerRemaining)}
							</span>
							<button 
								onclick={cancelSleepTimer}
								class="ml-2 text-slate-400 hover:text-white text-sm"
							>
								Cancel
							</button>
						</div>
					{/if}
				</div>
			</div>
		{/if}

		<!-- Tap Zone -->
		<button 
			class="absolute inset-0 z-10" 
			onclick={toggleControls}
			aria-label="Toggle controls"
		></button>

		<!-- Settings Panel -->
		{#if showSettings}
			<div class="absolute top-0 right-0 h-full w-[480px] bg-slate-800/95 backdrop-blur border-l border-slate-700 shadow-xl z-30 flex flex-col">
				<div class="p-4 border-b border-slate-700 flex items-center justify-between flex-shrink-0">
					<h2 class="text-white font-semibold">Audio Settings</h2>
					<button onclick={() => showSettings = false} aria-label="Close audio settings" class="text-slate-400 hover:text-white">
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
						</svg>
					</button>
				</div>

				<div class="flex border-b border-slate-700 flex-shrink-0">
					<button
						onclick={() => activeSettingsTab = 'playback'}
						class="flex-1 px-4 py-3 text-sm font-medium transition-colors {activeSettingsTab === 'playback' ? 'text-amber-500 border-b-2 border-amber-500' : 'text-slate-400 hover:text-white'}"
					>
						Playback
					</button>
					<button
						onclick={() => activeSettingsTab = 'display'}
						class="flex-1 px-4 py-3 text-sm font-medium transition-colors {activeSettingsTab === 'display' ? 'text-amber-500 border-b-2 border-amber-500' : 'text-slate-400 hover:text-white'}"
					>
						Display
					</button>
					<button
						onclick={() => activeSettingsTab = 'accessibility'}
						class="flex-1 px-4 py-3 text-sm font-medium transition-colors {activeSettingsTab === 'accessibility' ? 'text-amber-500 border-b-2 border-amber-500' : 'text-slate-400 hover:text-white'}"
					>
						Accessibility
					</button>
				</div>

				<div class="flex-1 overflow-y-auto p-4 space-y-6">
					{#if activeSettingsTab === 'playback'}
						<!-- Skip Intervals -->
						<div>
							<div class="block text-sm font-medium text-slate-300 mb-2">Skip Interval</div>
							<div class="grid grid-cols-4 gap-2">
								{#each skipIntervalOptions as opt}
									<button
										onclick={() => { updateSetting('skipForward', opt.value); updateSetting('skipBackward', opt.value); }}
										class="px-3 py-2 rounded-lg border transition-all {(settings.skipForward === opt.value && settings.skipBackward === opt.value) ? 'bg-amber-500 border-amber-500 text-white' : 'bg-slate-700 border-slate-600 text-slate-300 hover:bg-slate-600'}"
									>
										{opt.label}
									</button>
								{/each}
							</div>
						</div>

						<!-- Sleep Timer -->
						<div>
							<div class="block text-sm font-medium text-slate-300 mb-2">Sleep Timer</div>
							<div class="grid grid-cols-3 gap-2">
								{#each sleepTimerOptions as opt}
									<button
										onclick={() => { updateSetting('sleepTimer', opt.value); if (opt.value !== 'off') startSleepTimer(); else cancelSleepTimer(); }}
										class="px-3 py-2 rounded-lg border transition-all {settings.sleepTimer === opt.value ? 'bg-amber-500 border-amber-500 text-white' : 'bg-slate-700 border-slate-600 text-slate-300 hover:bg-slate-600'}"
									>
										{opt.label}
									</button>
								{/each}
							</div>
						</div>

						<!-- Auto-advance -->
						<div class="flex items-center space-x-3">
							<input
								type="checkbox"
								id="autoAdvance"
								checked={settings.autoAdvance}
								onchange={(e) => updateSetting('autoAdvance', e.currentTarget.checked)}
								class="rounded bg-slate-700 text-amber-500 focus:ring-amber-500"
							>
							<label for="autoAdvance" class="text-sm font-medium text-slate-300">Auto-advance to next chapter</label>
						</div>

						<!-- Gapless Playback -->
						<div class="flex items-center space-x-3">
							<input
								type="checkbox"
								id="gapless"
								checked={settings.gaplessPlayback}
								onchange={(e) => updateSetting('gaplessPlayback', e.currentTarget.checked)}
								class="rounded bg-slate-700 text-amber-500 focus:ring-amber-500"
							>
							<label for="gapless" class="text-sm font-medium text-slate-300">Gapless Playback</label>
						</div>

					{:else if activeSettingsTab === 'display'}
						<!-- Waveform Style -->
						<div>
							<div class="block text-sm font-medium text-slate-300 mb-2">Waveform Style</div>
							<div class="grid grid-cols-3 gap-2">
								{#each waveformStyles as style}
									<button
										onclick={() => updateSetting('waveformStyle', style.id)}
										class="px-3 py-2 rounded-lg border transition-all {settings.waveformStyle === style.id ? 'bg-amber-500 border-amber-500 text-white' : 'bg-slate-700 border-slate-600 text-slate-300 hover:bg-slate-600'}"
									>
										{style.name}
									</button>
								{/each}
							</div>
						</div>

						<!-- Background Style -->
						<div>
							<label class="block text-sm font-medium text-slate-300 mb-2" for="audio-background-style">Background Style</label>
							<select
								id="audio-background-style"
								value={settings.backgroundStyle}
								onchange={(e) => updateSetting('backgroundStyle', e.currentTarget.value)}
								class="w-full px-3 py-2 bg-slate-700 border border-slate-600 rounded-lg text-white"
							>
								<option value="cover-blur">Cover Blur</option>
								<option value="solid">Solid Color</option>
								<option value="none">None</option>
							</select>
						</div>

					{:else if activeSettingsTab === 'accessibility'}
						<!-- Voice Boost -->
						<div class="flex items-center space-x-3">
							<input
								type="checkbox"
								id="voiceBoost"
								checked={settings.voiceBoost}
								onchange={(e) => updateSetting('voiceBoost', e.currentTarget.checked)}
								class="rounded bg-slate-700 text-amber-500 focus:ring-amber-500"
							>
							<label for="voiceBoost" class="text-sm font-medium text-slate-300">Voice Boost</label>
						</div>

						<!-- Equalizer -->
						<div>
							<div class="block text-sm font-medium text-slate-300 mb-2">Low EQ: {settings.equalizerLow}%</div>
							<input
								type="range"
								min="0"
								max="100"
								value={settings.equalizerLow}
								oninput={(e) => updateSetting('equalizerLow', parseInt(e.currentTarget.value))}
								class="w-full h-2 bg-slate-700 rounded-lg appearance-none cursor-pointer"
							>
						</div>

						<div>
							<div class="block text-sm font-medium text-slate-300 mb-2">Mid EQ: {settings.equalizerMid}%</div>
							<input
								type="range"
								min="0"
								max="100"
								value={settings.equalizerMid}
								oninput={(e) => updateSetting('equalizerMid', parseInt(e.currentTarget.value))}
								class="w-full h-2 bg-slate-700 rounded-lg appearance-none cursor-pointer"
							>
						</div>

						<div>
							<div class="block text-sm font-medium text-slate-300 mb-2">High EQ: {settings.equalizerHigh}%</div>
							<input
								type="range"
								min="0"
								max="100"
								value={settings.equalizerHigh}
								oninput={(e) => updateSetting('equalizerHigh', parseInt(e.currentTarget.value))}
								class="w-full h-2 bg-slate-700 rounded-lg appearance-none cursor-pointer"
							>
						</div>

						<!-- Reset to Defaults -->
						<div class="pt-4 border-t border-slate-700">
							<button
								onclick={resetToDefaults}
								class="w-full px-4 py-2 bg-slate-700 text-slate-300 rounded-lg hover:bg-slate-600 transition-colors"
							>
								Reset to Defaults
							</button>
						</div>
					{/if}
				</div>
			</div>
		{/if}
	</div>
</div>

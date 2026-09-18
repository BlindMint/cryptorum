<script lang="ts">
	import { onMount } from 'svelte';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import { audioPlayer } from '$lib/stores/audioPlayer';
	import { readerSettings } from '$lib/stores/readerSettings';
	import { bulkActionBarHeight } from '$lib/stores/bulkActionBar';
	import { readerBottomBarHeight } from '$lib/stores/readerBottomBar';
	import { getCoverThumbUrl } from '$lib/utils/covers';

	let { readerMode = false } = $props<{ readerMode?: boolean }>();
	let speedButtonElement = $state<HTMLButtonElement>();
	let speedMenuElement = $state<HTMLDivElement>();
	let sleepButtonElement = $state<HTMLButtonElement>();
	let sleepMenuElement = $state<HTMLDivElement>();
	let confirmClear = $state(false);
	let queueMessage = $state('');
	let speedMenuOpen = $state(false);
	let speedMenuLeft = $state(0);
	let speedMenuBottom = $state(0);
	let sleepMenuOpen = $state(false);
	let sleepMenuLeft = $state(0);
	let sleepMenuBottom = $state(0);
	const current = $derived($audioPlayer.items.find((item) => item.id === $audioPlayer.currentItemId));
	const currentIndex = $derived($audioPlayer.items.findIndex((item) => item.id === $audioPlayer.currentItemId));
	const hasPrevious = $derived(currentIndex > 0 && $audioPlayer.items.slice(0, currentIndex).some((item) => !item.unavailable));
	const hasNext = $derived(currentIndex >= 0 && $audioPlayer.items.slice(currentIndex + 1).some((item) => !item.unavailable));
	const currentBookTrackCount = $derived(current ? $audioPlayer.items.filter((item) => item.book_id === current.book_id).length : 0);
	const progressPercent = $derived(
		$audioPlayer.duration > 0
			? Math.max(0, Math.min(100, ($audioPlayer.currentTime / $audioPlayer.duration) * 100))
			: 0
	);
	const bottomBarHeight = $derived(Math.max($bulkActionBarHeight, $readerBottomBarHeight));
	const speedOptions = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3];

	onMount(() => {
		const handleOutsidePointer = (event: PointerEvent) => {
			const target = event.target as Node;
			if (speedMenuOpen && !speedButtonElement?.contains(target) && !speedMenuElement?.contains(target)) speedMenuOpen = false;
			if (sleepMenuOpen && !sleepButtonElement?.contains(target) && !sleepMenuElement?.contains(target)) sleepMenuOpen = false;
		};
		const handleEscape = (event: KeyboardEvent) => {
			if (event.key !== 'Escape' || (!speedMenuOpen && !sleepMenuOpen)) return;
			event.preventDefault();
			if (speedMenuOpen) { speedMenuOpen = false; speedButtonElement?.focus(); }
			if (sleepMenuOpen) { sleepMenuOpen = false; sleepButtonElement?.focus(); }
		};
		const closeMenus = () => { speedMenuOpen = false; sleepMenuOpen = false; };
		document.addEventListener('pointerdown', handleOutsidePointer);
		document.addEventListener('keydown', handleEscape);
		window.addEventListener('resize', closeMenus);
		void audioPlayer.initialize();
		return () => {
			document.removeEventListener('pointerdown', handleOutsidePointer);
			document.removeEventListener('keydown', handleEscape);
			window.removeEventListener('resize', closeMenus);
		};
	});

	$effect(() => {
		if (!$audioPlayer.expanded || $audioPlayer.dismissed) { speedMenuOpen = false; sleepMenuOpen = false; }
	});

	function formatTime(value: number) {
		if (!Number.isFinite(value) || value < 0) return '0:00';
		const hours = Math.floor(value / 3600);
		const minutes = Math.floor((value % 3600) / 60);
		const seconds = Math.floor(value % 60).toString().padStart(2, '0');
		return hours > 0 ? `${hours}:${minutes.toString().padStart(2, '0')}:${seconds}` : `${minutes}:${seconds}`;
	}

	function handleSeek(event: Event) {
		audioPlayer.seek(Number((event.currentTarget as HTMLInputElement).value));
	}

	function handleVolume(event: Event) {
		audioPlayer.setVolume(Number((event.currentTarget as HTMLInputElement).value));
	}

	function formatSleepTimer(value: number | null) {
		if (!value) return 'Sleep';
		const minutes = Math.ceil(value / 60);
		return `${minutes}m`;
	}

	function toggleSleepMenu() {
		if (sleepMenuOpen) { sleepMenuOpen = false; return; }
		if (!sleepButtonElement) return;
		const rect = sleepButtonElement.getBoundingClientRect();
		const menuWidth = 176;
		sleepMenuLeft = Math.min(Math.max(8, rect.left + (rect.width - menuWidth) / 2), Math.max(8, window.innerWidth - menuWidth - 8));
		sleepMenuBottom = Math.max(8, window.innerHeight - rect.top + 8);
		sleepMenuOpen = true;
	}

	function chooseSleep(mode: 'off' | 'timer' | 'track' | 'chapter', minutes?: number) { audioPlayer.setSleepMode(mode, minutes); sleepMenuOpen = false; sleepButtonElement?.focus(); }

	function trackSubtitle(item: typeof current) {
		if (!item) return '';
		const author = item.authors.join(', ');
		if (item.category === 'music') return [author, item.album].filter(Boolean).join(' · ') || item.filename;
		if (item.category === 'podcast') return item.show_title || author || item.filename;
		return currentBookTrackCount > 1 ? [item.filename, author].filter(Boolean).join(' · ') : author || item.filename;
	}

	function toggleSpeedMenu() {
		if (speedMenuOpen) {
			speedMenuOpen = false;
			return;
		}
		if (!speedButtonElement) return;
		const rect = speedButtonElement.getBoundingClientRect();
		const menuWidth = 96;
		const viewportGutter = 8;
		const centeredLeft = rect.left + (rect.width - menuWidth) / 2;
		speedMenuLeft = Math.min(
			Math.max(viewportGutter, centeredLeft),
			Math.max(viewportGutter, window.innerWidth - menuWidth - viewportGutter)
		);
		speedMenuBottom = Math.max(8, window.innerHeight - rect.top + 8);
		speedMenuOpen = true;
	}

	function selectSpeed(speed: number) {
		audioPlayer.setPlaybackSpeed(speed);
		speedMenuOpen = false;
		speedButtonElement?.focus();
	}

	async function clearQueue() {
		if (!confirmClear) {
			confirmClear = true;
			return;
		}
		confirmClear = false;
		await audioPlayer.clear();
	}

	async function saveQueueAsPlaylist() {
		const audioIDs = $audioPlayer.items.flatMap((item) => item.audio_id ? [item.audio_id] : []);
		if (!audioIDs.length) { queueMessage = 'Scan these files before saving this queue.'; return; }
		const name = prompt('Playlist name')?.trim();
		if (!name) return;
		const response = await fetch('/api/audio/playlists', {
			method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name, audio_ids: audioIDs })
		});
		queueMessage = response.ok ? `Saved as “${name}”.` : 'Unable to save this queue.';
	}
</script>

{#if $audioPlayer.initialized && $audioPlayer.expanded && !current && !$audioPlayer.dismissed}
	<section class="audio-player-bottom fixed left-1/2 z-[10000] w-[min(94vw,36rem)] -translate-x-1/2 rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5 shadow-2xl backdrop-blur" style:bottom={`calc(0.75rem + ${bottomBarHeight}px)`} aria-label="Audio player">
		<div class="flex items-start justify-between gap-4">
			<div class="flex min-w-0 items-start gap-3">
				<div class="flex h-11 w-11 flex-none items-center justify-center rounded-xl bg-[var(--color-primary-500)]/15 text-[var(--color-primary-400)]">
					<svg class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M9 18V5l11-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="17" cy="16" r="3"/></svg>
				</div>
				<div class="min-w-0">
					<h2 class="font-semibold text-[var(--color-surface-text)]">{$audioPlayer.error ? 'Unable to open audio queue' : 'Audio queue is empty'}</h2>
					<p class="mt-1 text-sm leading-5 {$audioPlayer.error ? 'text-red-400' : 'text-[var(--color-surface-text-muted)]'}">{$audioPlayer.error || 'Open an audio book and choose Play Audio or Add to queue.'}</p>
				</div>
			</div>
			<button type="button" class="flex-none rounded-md p-2 text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" aria-label="Close audio player" onclick={() => audioPlayer.dismiss()}>
				<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg>
			</button>
		</div>
	</section>
{:else if current && !$audioPlayer.dismissed}
	{#if !$audioPlayer.expanded}
		{#if readerMode}
			<button
				type="button"
				class="audio-reader-tab fixed right-0 top-1/2 z-[10000] flex -translate-y-1/2 flex-col items-center gap-1.5 rounded-l-xl border border-r-0 border-[var(--color-primary-500)]/60 bg-[var(--color-surface-overlay)] px-2.5 py-3 text-[var(--color-surface-text)] shadow-2xl ring-1 ring-black/20 backdrop-blur"
				class:is-playing={$audioPlayer.isPlaying}
				title="Open audio player"
				aria-label="Open audio player for {current.title}"
				onclick={() => audioPlayer.expand()}
			>
				<svg class="music-note h-5 w-5 text-[var(--color-primary-400)]" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 3v10.55A4 4 0 1 0 14 17V7h5V3h-7Z"/></svg>
				<span class="text-[9px] font-bold uppercase tracking-wide text-[var(--color-surface-text-muted)] [writing-mode:vertical-rl]">Audio</span>
				<span class="text-[10px] font-semibold">{Math.round(progressPercent)}%</span>
			</button>
		{:else}
			<div class="audio-player-bottom fixed left-1/2 z-[10000] flex w-[min(94vw,42rem)] -translate-x-1/2 items-center gap-2 rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-2 shadow-2xl backdrop-blur sm:gap-3" style:bottom={`calc(0.75rem + ${bottomBarHeight}px)`}>
				<button type="button" class="accent-action flex-none rounded-full p-2.5 shadow-lg shadow-black/20 focus-visible:outline-2" aria-label={$audioPlayer.isPlaying ? 'Pause' : 'Play'} onclick={() => void audioPlayer.togglePlay()}>
					{#if $audioPlayer.isPlaying}
						<svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor"><path d="M6 4h4v16H6zm8 0h4v16h-4z"/></svg>
					{:else}
						<svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor"><path d="m8 5 11 7-11 7V5z"/></svg>
					{/if}
				</button>
				<div class="min-w-0 flex-1">
					<button type="button" class="block w-full truncate text-left text-sm font-semibold text-[var(--color-surface-text)]" onclick={() => audioPlayer.expand()}>{current.title}</button>
					<div class="mt-1 flex min-w-0 items-center gap-2">
						<span class="flex-none whitespace-nowrap text-[10px] tabular-nums text-[var(--color-surface-text-muted)] sm:text-xs">{formatTime($audioPlayer.currentTime)} / {formatTime($audioPlayer.duration)}</span>
						<div
							class="minimized-audio-progress min-w-12 flex-1 sm:min-w-28"
							role="progressbar"
							aria-label="Audio progress"
							aria-valuemin="0"
							aria-valuemax="100"
							aria-valuenow={Math.round(progressPercent)}
						>
							<div class="minimized-audio-progress-fill" style:width={`${progressPercent}%`}></div>
						</div>
						<span class="w-8 flex-none text-right text-[10px] font-semibold tabular-nums text-[var(--color-primary-300)] sm:text-xs">{Math.round(progressPercent)}%</span>
					</div>
				</div>
				<button type="button" class="rounded-md p-2 text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]" aria-label="Expand player" onclick={() => audioPlayer.expand()}>
					<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m7 14 5-5 5 5"/></svg>
				</button>
				<button type="button" class="rounded-md p-2 text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]" aria-label="Close audio player" onclick={() => audioPlayer.dismiss()}>
					<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 6l12 12M18 6 6 18"/></svg>
				</button>
			</div>
		{/if}
	{:else}
		<section class="audio-player-bottom fixed left-1/2 z-[10000] w-[min(94vw,64rem)] -translate-x-1/2 overflow-hidden rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl backdrop-blur" style:bottom={`calc(0.75rem + ${bottomBarHeight}px)`} aria-label="Audio player">
			<div class="flex items-start gap-3 p-3 sm:items-center sm:gap-4 sm:p-4">
				<BookCoverFrame
					src={getCoverThumbUrl(current.book_id, 'small')}
					alt={`${current.title} cover`}
					format={current.format}
					placeholderKind="audio"
					placeholderSize="xs"
					loading="eager"
					frameClass="h-14 w-11 flex-none rounded-md sm:h-20 sm:w-14"
				/>
				<div class="min-w-0 flex-1">
					<div class="flex items-start justify-between gap-2">
						<div class="min-w-0">
							<a href={current.category === 'audiobook' || !current.category ? `/book/${current.book_id}` : `/audio?tab=${current.category}`} class="block truncate text-sm font-semibold text-[var(--color-surface-text)] hover:text-[var(--color-primary-400)] sm:text-base">{current.title}</a>
							<p class="truncate text-xs text-[var(--color-surface-text-muted)] sm:text-sm">{trackSubtitle(current)}</p>
						</div>
						<div class="flex flex-none">
							<button type="button" class="rounded-md p-2 text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" class:panel-active={$audioPlayer.activePanel === 'queue' && $audioPlayer.queueOpen} title="Queue" aria-label="Toggle queue" aria-expanded={$audioPlayer.queueOpen} onclick={() => audioPlayer.toggleQueue()}>
								<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 6h12M3 12h9M3 18h6"/><path d="m16 15 5 3-5 3v-6Z"/></svg>
							</button>
							{#if current.audio_id && (current.chapter_count || $audioPlayer.chapters.length)}<button type="button" class="rounded-md p-2 text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" class:panel-active={$audioPlayer.activePanel === 'chapters'} title="Chapters" aria-label="Open chapters" onclick={() => audioPlayer.openPanel('chapters')}><svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 5h3v3H4zM10 6h10M4 11h3v3H4zM10 12h10M4 17h3v3H4zM10 18h10"/></svg></button>{/if}
							{#if current.audio_id}<button type="button" class="rounded-md p-2 text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" class:panel-active={$audioPlayer.activePanel === 'bookmarks'} title="Bookmarks" aria-label="Open audio bookmarks" onclick={() => audioPlayer.openPanel('bookmarks')}><svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 4h12v17l-6-4-6 4V4z"/></svg></button>{/if}
							<button type="button" class="rounded-md p-2 text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" title="Minimize" aria-label="Minimize audio player" onclick={() => audioPlayer.minimize()}>
								<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14"/></svg>
							</button>
							<button type="button" class="rounded-md p-2 text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" title="Close" aria-label="Close audio player" onclick={() => audioPlayer.dismiss()}>
								<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 6l12 12M18 6 6 18"/></svg>
							</button>
						</div>
					</div>

					<div class="mt-2 flex items-center gap-2">
						<span class="w-10 text-right text-[10px] tabular-nums text-[var(--color-surface-text-muted)] sm:w-12 sm:text-xs">{formatTime($audioPlayer.currentTime)}</span>
						<input class="audio-seek min-w-0 flex-1" type="range" min="0" max={$audioPlayer.duration || 0} step="0.1" value={$audioPlayer.currentTime} aria-label="Audio position" oninput={handleSeek} />
						<span class="w-10 text-[10px] tabular-nums text-[var(--color-surface-text-muted)] sm:w-12 sm:text-xs">{formatTime($audioPlayer.duration)}</span>
					</div>

					<div class="mt-2 flex flex-wrap items-center justify-center gap-1 sm:gap-2">
						<button type="button" class="player-control" disabled={!hasPrevious} aria-label="Previous queue item" onclick={() => void audioPlayer.previous()}><svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor"><path d="M6 5h2v14H6zm3 7 10-7v14L9 12z"/></svg></button>
						<button type="button" class="player-control text-xs font-semibold" aria-label="Skip backward" onclick={() => audioPlayer.skip(-$readerSettings.audio.skipBackward)}>−{$readerSettings.audio.skipBackward}</button>
						<button type="button" class="accent-action rounded-full p-3 focus-visible:outline-2" disabled={$audioPlayer.isLoading} aria-label={$audioPlayer.isPlaying ? 'Pause' : 'Play'} onclick={() => void audioPlayer.togglePlay()}>
							{#if $audioPlayer.isLoading}
								<svg class="h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none"><circle class="opacity-25" cx="12" cy="12" r="9" stroke="currentColor" stroke-width="3"/><path class="opacity-80" fill="currentColor" d="M12 3a9 9 0 0 1 9 9h-3a6 6 0 0 0-6-6V3Z"/></svg>
							{:else if $audioPlayer.isPlaying}
								<svg class="h-5 w-5" viewBox="0 0 24 24" fill="currentColor"><path d="M6 4h4v16H6zm8 0h4v16h-4z"/></svg>
							{:else}
								<svg class="h-5 w-5" viewBox="0 0 24 24" fill="currentColor"><path d="m8 5 11 7-11 7V5z"/></svg>
							{/if}
						</button>
						<button type="button" class="player-control text-xs font-semibold" aria-label="Skip forward" onclick={() => audioPlayer.skip($readerSettings.audio.skipForward)}>+{$readerSettings.audio.skipForward}</button>
						<button type="button" class="player-control" disabled={!hasNext} aria-label="Next queue item" onclick={() => void audioPlayer.next()}><svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor"><path d="M16 5h2v14h-2zM5 5l10 7-10 7V5z"/></svg></button>
						<div class="ml-1">
							<button
								bind:this={speedButtonElement}
								type="button"
								class="audio-speed-button flex items-center gap-1 rounded-md border px-2 py-1.5 text-xs font-semibold"
								class:open={speedMenuOpen}
								aria-label={`Playback speed: ${$audioPlayer.playbackSpeed}×`}
								aria-haspopup="menu"
								aria-expanded={speedMenuOpen}
								onclick={toggleSpeedMenu}
							>
								<span>{$audioPlayer.playbackSpeed}×</span>
								<svg class="h-3 w-3" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m3 4.5 3 3 3-3"/></svg>
							</button>
						</div>
						<div class="ml-1 flex items-center gap-1.5">
							<button type="button" class="player-control" aria-label={$audioPlayer.muted || $audioPlayer.volume === 0 ? 'Unmute' : 'Mute'} title={$audioPlayer.muted || $audioPlayer.volume === 0 ? 'Unmute' : 'Mute'} onclick={() => audioPlayer.toggleMute()}>
								{#if $audioPlayer.muted || $audioPlayer.volume === 0}
									<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M11 5 6 9H2v6h4l5 4V5Z"/><path d="m22 9-6 6m0-6 6 6"/></svg>
								{:else}
									<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M11 5 6 9H2v6h4l5 4V5Z"/><path d="M15.5 8.5a5 5 0 0 1 0 7M18 6a8 8 0 0 1 0 12"/></svg>
								{/if}
							</button>
							<input class="audio-volume w-16 sm:w-20" type="range" min="0" max="1" step="0.05" value={$audioPlayer.muted ? 0 : $audioPlayer.volume} aria-label="Volume" oninput={handleVolume} />
						</div>
						<button bind:this={sleepButtonElement} type="button" class="audio-sleep-button rounded-md px-2 py-1.5 text-xs font-semibold" class:active={$audioPlayer.sleepMode !== 'off'} aria-label={`Sleep setting: ${$audioPlayer.sleepMode === 'timer' ? formatSleepTimer($audioPlayer.sleepTimerRemaining) : $audioPlayer.sleepMode}. Activate to change.`} aria-haspopup="menu" aria-expanded={sleepMenuOpen} title="Sleep settings" onclick={toggleSleepMenu}>
							<svg class="mr-1 inline h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>{$audioPlayer.sleepMode === 'timer' ? formatSleepTimer($audioPlayer.sleepTimerRemaining) : $audioPlayer.sleepMode === 'track' ? 'Track' : $audioPlayer.sleepMode === 'chapter' ? 'Chapter' : 'Sleep'}
						</button>
					</div>
					{#if $audioPlayer.error}
						<div class="mt-2 flex items-center justify-center gap-2 text-xs" role="alert">
							<span class="text-red-400">{$audioPlayer.error}</span>
							<button type="button" class="rounded-md border border-[var(--color-surface-border)] px-2 py-1 text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)]" onclick={() => void audioPlayer.retryCurrent()}>Retry</button>
							{#if hasNext}<button type="button" class="rounded-md border border-[var(--color-surface-border)] px-2 py-1 text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)]" onclick={() => void audioPlayer.next()}>Skip</button>{/if}
						</div>
					{/if}
				</div>
			</div>

			{#if $audioPlayer.queueOpen && $audioPlayer.activePanel === 'queue'}
				<div class="max-h-[42vh] overflow-y-auto border-t border-[var(--color-surface-border)] bg-[var(--color-surface-base)]/75 p-3">
					<div class="mb-2 flex items-center justify-between gap-2">
						<h2 class="text-sm font-semibold text-[var(--color-surface-text)]">Queue <span class="text-[var(--color-surface-text-muted)]">({$audioPlayer.items.length})</span></h2>
						<div class="flex items-center gap-1"><button type="button" class="rounded-md px-2 py-1 text-xs text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" onclick={() => void saveQueueAsPlaylist()}>Save playlist</button><button type="button" class="rounded-md px-2 py-1 text-xs text-[var(--color-surface-text-muted)] hover:bg-red-500/10 hover:text-red-400" onclick={() => void clearQueue()}>{confirmClear ? 'Confirm clear' : 'Clear queue'}</button></div>
					</div>
					{#if queueMessage}<p class="mb-2 text-xs text-[var(--color-surface-text-muted)]" aria-live="polite">{queueMessage}</p>{/if}
					<ol class="space-y-1">
						{#each $audioPlayer.items as item, index (item.id)}
							<li class="flex items-center gap-2 rounded-lg px-2 py-2 {item.id === $audioPlayer.currentItemId ? 'bg-[var(--color-primary-500)]/12' : 'hover:bg-[var(--color-surface-700)]/70'}">
								<button type="button" class="min-w-0 flex-1 text-left disabled:opacity-55" disabled={item.unavailable} aria-label={`Play ${item.title}`} onclick={() => void audioPlayer.setCurrent(item.id)}>
									<span class="block truncate text-sm font-medium {item.id === $audioPlayer.currentItemId ? 'text-[var(--color-primary-400)]' : 'text-[var(--color-surface-text)]'}">{index + 1}. {item.title}</span>
									<span class="block truncate text-xs text-[var(--color-surface-text-muted)]">{item.unavailable ? 'File unavailable' : $audioPlayer.items.filter((queued) => queued.book_id === item.book_id).length > 1 ? `${item.filename}${item.authors.length ? ` · ${item.authors.join(', ')}` : ''}` : item.authors.join(', ') || item.filename}</span>
								</button>
								<button type="button" class="queue-control" disabled={index === 0} aria-label="Move up" onclick={() => void audioPlayer.move(item.id, -1)}>↑</button>
								<button type="button" class="queue-control" disabled={index === $audioPlayer.items.length - 1} aria-label="Move down" onclick={() => void audioPlayer.move(item.id, 1)}>↓</button>
								<button type="button" class="queue-control hover:!text-red-400" aria-label={`Remove ${item.title}`} onclick={() => void audioPlayer.remove(item.id)}>×</button>
							</li>
						{/each}
					</ol>
				</div>
			{:else if $audioPlayer.activePanel === 'chapters'}
				<div class="max-h-[42vh] overflow-y-auto border-t border-[var(--color-surface-border)] bg-[var(--color-surface-base)]/75 p-3">
					<h2 class="mb-2 text-sm font-semibold text-[var(--color-surface-text)]">Chapters <span class="text-[var(--color-surface-text-muted)]">({$audioPlayer.chapters.length})</span></h2>
					{#if $audioPlayer.contextLoading}<p class="py-4 text-center text-xs text-[var(--color-surface-text-muted)]">Loading chapters…</p>
					{:else if !$audioPlayer.chapters.length}<p class="py-4 text-center text-xs text-[var(--color-surface-text-muted)]">This file has no embedded chapters.</p>
					{:else}<ol class="space-y-1">{#each $audioPlayer.chapters as chapter, index}<li><button type="button" class="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left hover:bg-[var(--color-surface-700)]" onclick={() => audioPlayer.seek(chapter.start_seconds)}><span class="w-6 text-right text-xs text-[var(--color-surface-text-muted)]">{index + 1}</span><span class="min-w-0 flex-1 truncate text-sm text-[var(--color-surface-text)]">{chapter.title}</span><span class="text-xs tabular-nums text-[var(--color-surface-text-muted)]">{formatTime(chapter.start_seconds)}</span></button></li>{/each}</ol>{/if}
				</div>
			{:else if $audioPlayer.activePanel === 'bookmarks'}
				<div class="max-h-[42vh] overflow-y-auto border-t border-[var(--color-surface-border)] bg-[var(--color-surface-base)]/75 p-3">
					<div class="mb-2 flex items-center justify-between gap-2"><h2 class="text-sm font-semibold text-[var(--color-surface-text)]">Bookmarks <span class="text-[var(--color-surface-text-muted)]">({$audioPlayer.bookmarks.length})</span></h2><button type="button" class="accent-action rounded-lg px-2.5 py-1.5 text-xs" onclick={() => { const label = prompt('Bookmark label (optional)') ?? ''; void audioPlayer.addBookmark(label); }}>Bookmark {formatTime($audioPlayer.currentTime)}</button></div>
					{#if $audioPlayer.contextLoading}<p class="py-4 text-center text-xs text-[var(--color-surface-text-muted)]">Loading bookmarks…</p>
					{:else if !$audioPlayer.bookmarks.length}<p class="py-4 text-center text-xs text-[var(--color-surface-text-muted)]">No bookmarks for this audio yet.</p>
					{:else}<ol class="space-y-1">{#each $audioPlayer.bookmarks as bookmark}<li class="flex items-center gap-2 rounded-lg px-2 py-2 hover:bg-[var(--color-surface-700)]"><button type="button" class="min-w-0 flex-1 text-left" onclick={() => audioPlayer.seek(bookmark.seconds)}><span class="block truncate text-sm text-[var(--color-surface-text)]">{bookmark.label || `Bookmark at ${formatTime(bookmark.seconds)}`}</span><span class="text-xs tabular-nums text-[var(--color-surface-text-muted)]">{formatTime(bookmark.seconds)}</span></button><button type="button" class="queue-control" aria-label="Rename bookmark" onclick={() => { const label = prompt('Bookmark label', bookmark.label) ?? bookmark.label; void audioPlayer.renameBookmark(bookmark.id, label); }}>✎</button><button type="button" class="queue-control hover:!text-red-400" aria-label="Remove bookmark" onclick={() => void audioPlayer.deleteBookmark(bookmark.id)}>×</button></li>{/each}</ol>{/if}
				</div>
			{/if}
		</section>
		{#if speedMenuOpen}
			<div
				bind:this={speedMenuElement}
				class="audio-speed-menu fixed z-[10001] grid w-24 gap-1 rounded-xl border p-1.5"
				style:left={`${speedMenuLeft}px`}
				style:bottom={`${speedMenuBottom}px`}
				role="menu"
				aria-label="Playback speed"
			>
				{#each speedOptions as speed}
					<button
						type="button"
						class="audio-speed-option flex items-center justify-between rounded-lg px-2.5 py-1.5 text-left text-xs font-semibold"
						class:selected={$audioPlayer.playbackSpeed === speed}
						role="menuitemradio"
						aria-checked={$audioPlayer.playbackSpeed === speed}
						onclick={() => selectSpeed(speed)}
					>
						<span>{speed}×</span>
						{#if $audioPlayer.playbackSpeed === speed}<span aria-hidden="true">✓</span>{/if}
					</button>
				{/each}
			</div>
		{/if}
		{#if sleepMenuOpen}
			<div bind:this={sleepMenuElement} class="audio-speed-menu fixed z-[10001] grid w-44 gap-1 rounded-xl border p-1.5" style:left={`${sleepMenuLeft}px`} style:bottom={`${sleepMenuBottom}px`} role="menu" aria-label="Sleep settings">
				<button type="button" class="audio-speed-option rounded-lg px-2.5 py-1.5 text-left text-xs font-semibold" class:selected={$audioPlayer.sleepMode === 'off'} onclick={() => chooseSleep('off')}>Off</button>
				{#each [15, 30, 60] as minutes}<button type="button" class="audio-speed-option rounded-lg px-2.5 py-1.5 text-left text-xs font-semibold" class:selected={$audioPlayer.sleepMode === 'timer' && $audioPlayer.sleepTimerMinutes === minutes} onclick={() => chooseSleep('timer', minutes)}>{minutes} minutes</button>{/each}
				<button type="button" class="audio-speed-option rounded-lg px-2.5 py-1.5 text-left text-xs font-semibold" class:selected={$audioPlayer.sleepMode === 'track'} onclick={() => chooseSleep('track')}>End of track</button>
				{#if $audioPlayer.chapters.length}<button type="button" class="audio-speed-option rounded-lg px-2.5 py-1.5 text-left text-xs font-semibold" class:selected={$audioPlayer.sleepMode === 'chapter'} onclick={() => chooseSleep('chapter')}>End of chapter</button>{/if}
			</div>
		{/if}
	{/if}
{/if}

<style>
	.player-control {
		display: inline-flex;
		height: 2.25rem;
		min-width: 2.25rem;
		align-items: center;
		justify-content: center;
		border-radius: 9999px;
		color: var(--color-surface-text-muted);
	}
	.player-control:hover:not(:disabled), .queue-control:hover:not(:disabled) { background: var(--color-surface-700); color: var(--color-surface-text); }
	.player-control:disabled, .queue-control:disabled { opacity: 0.35; }
	.queue-control { border-radius: 0.375rem; padding: 0.25rem 0.45rem; color: var(--color-surface-text-muted); }
	.audio-seek, .audio-volume { accent-color: var(--color-primary-500); }
	.audio-sleep-button { color: var(--color-surface-text-muted); }
	.audio-sleep-button:hover { background: var(--color-surface-700); color: var(--color-surface-text); }
	.audio-sleep-button.active { background: color-mix(in srgb, var(--color-primary-500) 16%, transparent); color: var(--color-primary-300); }
	.panel-active { background: color-mix(in srgb, var(--color-primary-500) 14%, transparent); color: var(--color-primary-300); }
	.minimized-audio-progress {
		height: 0.5rem;
		overflow: hidden;
		border: 1px solid color-mix(in srgb, var(--color-surface-border) 85%, var(--color-surface-text-muted));
		border-radius: 9999px;
		background: color-mix(in srgb, var(--color-surface-base) 80%, transparent);
		box-shadow: inset 0 1px 2px rgba(0, 0, 0, 0.28);
	}
	.minimized-audio-progress-fill {
		height: 100%;
		border-radius: 0 9999px 9999px 0;
		background: var(--color-primary-400);
		box-shadow: 1px 0 4px color-mix(in srgb, var(--color-primary-500) 45%, transparent);
		transition: width 120ms linear;
	}
	.audio-speed-button {
		border-color: transparent;
		background: transparent;
		color: var(--color-surface-text);
		cursor: pointer;
	}
	.audio-speed-button:hover,
	.audio-speed-button.open {
		border-color: color-mix(in srgb, var(--color-primary-500) 35%, transparent);
		background: color-mix(in srgb, var(--color-primary-500) 10%, transparent);
	}
	.audio-speed-button:focus-visible {
		border-color: var(--color-primary-500);
		outline: 2px solid color-mix(in srgb, var(--color-primary-500) 55%, transparent);
		outline-offset: 2px;
	}
	.audio-speed-menu {
		border-color: var(--color-surface-border);
		background: var(--color-surface-overlay);
		color: var(--color-surface-text);
		backdrop-filter: blur(18px) saturate(135%);
		box-shadow: 0 16px 40px rgba(0, 0, 0, 0.38), inset 0 1px 0 color-mix(in srgb, var(--color-surface-text) 10%, transparent);
	}
	.audio-speed-option { color: var(--color-surface-text-muted); }
	.audio-speed-option:hover,
	.audio-speed-option:focus-visible {
		background: color-mix(in srgb, var(--color-primary-500) 12%, transparent);
		color: var(--color-surface-text);
		outline: none;
	}
	.audio-speed-option.selected { background: color-mix(in srgb, var(--color-primary-500) 18%, transparent); color: var(--color-primary-300); }
	.audio-player-bottom { transition: bottom 180ms ease; }
	.audio-reader-tab.is-playing .music-note { animation: audio-pulse 1.2s ease-in-out infinite; }
	@keyframes audio-pulse { 50% { transform: translateY(-2px) rotate(6deg); } }
	@media (prefers-reduced-motion: reduce) { .audio-reader-tab.is-playing .music-note { animation: none; } }
</style>

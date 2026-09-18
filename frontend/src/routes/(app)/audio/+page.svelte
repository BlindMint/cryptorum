<script lang="ts">
	import { onMount } from 'svelte';
	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/stores';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import CombineBooksModal from '$lib/components/CombineBooksModal.svelte';
	import { audioPlayer } from '$lib/stores/audioPlayer';
	import { bulkActionBarHeight, trackBulkActionBar } from '$lib/stores/bulkActionBar';

	type Category = 'audiobook' | 'music' | 'podcast';
	type Tab = Category | 'playlists';
	type MusicView = 'albums' | 'artists' | 'tracks';

	interface AudioItem {
		id: number; book_id: number; file_id: number; library_id: number; category: Category; title: string;
		artists: string[]; album_artist: string; album: string; track_number: number | null; disc_number: number | null;
		release_date: string; genre: string; duration_seconds: number; show_title: string; episode_number: number | null;
		published_at: string; filename: string; format: string; status: 'unplayed' | 'in_progress' | 'played';
		position_seconds: number; playback_speed: number | null; unavailable: boolean; chapter_count: number; bookmark_count: number;
	}

	interface Playlist { id: number; name: string; description: string; item_count: number; duration_seconds: number; created_at: number; updated_at: number; }
	interface AudioLibrary { id: number; name: string; media_scope: 'mixed' | 'books' | 'audio'; }

	let activeTab = $state<Tab>('audiobook');
	let musicView = $state<MusicView>('albums');
	let items = $state<AudioItem[]>([]);
	let playlists = $state<Playlist[]>([]);
	let playlistItems = $state<Record<number, AudioItem[]>>({});
	let expandedPlaylistID = $state<number | null>(null);
	let query = $state('');
	let statusFilter = $state('');
	let sort = $state('title');
	let selectedIDs = $state<number[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let editing = $state<AudioItem | null>(null);
	let playlistName = $state('');
	let showPlaylistForm = $state(false);
	let showPlaylistPicker = $state(false);
	let showTypeMenu = $state(false);
	let typeMenuContainer = $state<HTMLDivElement | null>(null);
	let typeMenuButton = $state<HTMLButtonElement | null>(null);
	let showGroupAudiobookModal = $state(false);
	let groupAudiobookBookIDs = $state<number[]>([]);
	let bulkSelectionAnchorID = $state<number | null>(null);
	let longPressTimer: number | null = null;
	let suppressNextClickID: number | null = null;
	let longPressTouchStart: { x: number; y: number } | null = null;
	let activeLibraryID = $state<number | null>(null);
	let activeLibrary = $state<AudioLibrary | null>(null);
	let loadedRouteKey = '';
	const LONG_PRESS_THRESHOLD = 500;
	const LONG_PRESS_MOVE_TOLERANCE = 10;

	const selectedSet = $derived(new Set(selectedIDs));
	const selectedItems = $derived(items.filter((item) => selectedSet.has(item.id)));
	const allResultsSelected = $derived(items.length > 0 && items.every((item) => selectedSet.has(item.id)));
	const canGroupAudiobook = $derived(new Set(selectedItems.map((item) => item.book_id)).size >= 2);
	const tabOptions = $derived(activeLibraryID === null
		? [['audiobook', 'Audiobooks'], ['music', 'Music'], ['podcast', 'Podcasts'], ['playlists', 'Playlists']]
		: [['audiobook', 'Audiobooks'], ['music', 'Music'], ['podcast', 'Podcasts']]);
	const groupedItems = $derived.by(() => {
		const groups = new Map<string, AudioItem[]>();
		for (const item of items) {
			const key = activeTab === 'music' && musicView === 'artists'
				? item.album_artist || item.artists.join(', ') || 'Unknown Artist'
				: activeTab === 'podcast'
					? item.show_title || 'Ungrouped Episodes'
					: item.album || 'Ungrouped Tracks';
			if (!groups.has(key)) groups.set(key, []);
			groups.get(key)!.push(item);
		}
		return [...groups.entries()];
	});

	function formatDuration(seconds: number) {
		if (!seconds) return 'Unknown length';
		const hours = Math.floor(seconds / 3600);
		const minutes = Math.floor((seconds % 3600) / 60);
		return hours ? `${hours}h ${minutes}m` : `${minutes}m`;
	}

	function categoryLabel(category: Category) { return category === 'audiobook' ? 'Audiobooks' : category === 'music' ? 'Music' : 'Podcasts'; }

	async function setTab(tab: Tab) {
		activeTab = tab;
		deselectAll();
		if (tab === 'podcast' && sort === 'title') sort = 'published';
		else if (tab !== 'podcast' && sort === 'published') sort = 'title';
		const url = new URL($page.url);
		url.searchParams.set('tab', tab);
		await goto(`${url.pathname}${url.search}`, { replaceState: true, noScroll: true, keepFocus: true });
	}

	async function load() {
		loading = true; error = '';
		try {
			if (activeTab === 'playlists') {
				const response = await fetch('/api/audio/playlists', { credentials: 'same-origin' });
				if (!response.ok) throw new Error('Unable to load playlists');
				playlists = await response.json();
			} else {
				const params = new URLSearchParams({ category: activeTab, sort });
				if (activeLibraryID !== null) params.set('library_id', String(activeLibraryID));
				if (query.trim()) params.set('q', query.trim());
				if (statusFilter) params.set('status', statusFilter);
				const response = await fetch(`/api/audio/items?${params}`, { credentials: 'same-origin' });
				if (!response.ok) throw new Error('Unable to load the audio library');
				const data = await response.json();
				items = data.items ?? [];
				const resultIDs = new Set(items.map((item) => item.id));
				selectedIDs = selectedIDs.filter((id) => resultIDs.has(id));
				if (bulkSelectionAnchorID !== null && !resultIDs.has(bulkSelectionAnchorID)) bulkSelectionAnchorID = null;
				if (!selectedIDs.length) showTypeMenu = false;
			}
		} catch (reason) { error = reason instanceof Error ? reason.message : 'Unable to load audio'; }
		finally { loading = false; }
	}

	async function loadActiveLibrary() {
		activeLibrary = null;
		if (activeLibraryID === null) return;
		try {
			const response = await fetch(`/api/libraries/${activeLibraryID}`, { credentials: 'same-origin', cache: 'no-store' });
			if (!response.ok) throw new Error('Unable to load the audio library');
			const library = await response.json();
			activeLibrary = { id: library.id, name: library.name, media_scope: library.media_scope || 'mixed' };
		} catch (reason) {
			error = reason instanceof Error ? reason.message : 'Unable to load the audio library';
		}
	}

	function visibleAudioIDs(): number[] {
		if (activeTab === 'music' && musicView !== 'tracks' || activeTab === 'podcast') {
			return groupedItems.flatMap(([, group]) => group.map((item) => item.id));
		}
		return items.map((item) => item.id);
	}

	function toggleSelected(id: number, event?: MouseEvent) {
		if (event) {
			event.preventDefault();
			event.stopPropagation();
		}
		const visibleIDs = visibleAudioIDs();
		if (event?.shiftKey && bulkSelectionAnchorID !== null) {
			const anchorIndex = visibleIDs.indexOf(bulkSelectionAnchorID);
			const targetIndex = visibleIDs.indexOf(id);
			if (anchorIndex !== -1 && targetIndex !== -1) {
				const next = new Set(selectedIDs);
				const shouldSelect = !selectedSet.has(id);
				const [start, end] = anchorIndex < targetIndex ? [anchorIndex, targetIndex] : [targetIndex, anchorIndex];
				for (const rangeID of visibleIDs.slice(start, end + 1)) {
					if (shouldSelect) next.add(rangeID);
					else next.delete(rangeID);
				}
				selectedIDs = [...next];
				return;
			}
		}
		selectedIDs = selectedSet.has(id) ? selectedIDs.filter((value) => value !== id) : [...selectedIDs, id];
		bulkSelectionAnchorID = id;
	}

	function selectAllResults() { selectedIDs = items.map((item) => item.id); bulkSelectionAnchorID = null; }
	function deselectAll() { selectedIDs = []; bulkSelectionAnchorID = null; showTypeMenu = false; }

	function handleWindowClick(event: MouseEvent) {
		if (!showTypeMenu || !typeMenuContainer) return;
		if (event.target instanceof Node && !typeMenuContainer.contains(event.target)) showTypeMenu = false;
	}

	function handleWindowKeydown(event: KeyboardEvent) {
		if (event.key !== 'Escape' || !showTypeMenu) return;
		showTypeMenu = false;
		typeMenuButton?.focus();
	}

	function handleAudioClick(event: MouseEvent) {
		const id = Number((event.currentTarget as HTMLElement).dataset.audioId);
		if (suppressNextClickID === id) {
			event.preventDefault();
			event.stopPropagation();
			suppressNextClickID = null;
			return;
		}
		if (!selectedIDs.length) return;
		const target = event.target as HTMLElement;
		if (target.closest('[data-audio-action]')) return;
		toggleSelected(id, event);
	}

	function handleAudioKeydown(event: KeyboardEvent) {
		if (event.key !== 'Enter' && event.key !== ' ') return;
		if (!selectedIDs.length) return;
		event.preventDefault();
		handleAudioClick(event as unknown as MouseEvent);
	}

	function clearLongPressTimer() {
		if (longPressTimer !== null) {
			clearTimeout(longPressTimer);
			longPressTimer = null;
		}
	}

	function handleAudioMouseDown(event: MouseEvent) {
		if ('ontouchstart' in window || (event.target as HTMLElement).closest('[data-audio-action]')) return;
		const id = Number((event.currentTarget as HTMLElement).dataset.audioId);
		longPressTimer = window.setTimeout(() => {
			suppressNextClickID = id;
			toggleSelected(id);
			longPressTimer = null;
		}, LONG_PRESS_THRESHOLD);
	}

	function handleAudioTouchStart(event: TouchEvent) {
		if ((event.target as HTMLElement).closest('[data-audio-action]')) return;
		const id = Number((event.currentTarget as HTMLElement).dataset.audioId);
		const touch = event.touches[0];
		longPressTouchStart = touch ? { x: touch.clientX, y: touch.clientY } : null;
		longPressTimer = window.setTimeout(() => {
			suppressNextClickID = id;
			toggleSelected(id);
			longPressTimer = null;
		}, LONG_PRESS_THRESHOLD);
	}

	function handleAudioTouchMove(event: TouchEvent) {
		if (longPressTimer === null || !longPressTouchStart) return;
		const touch = event.touches[0];
		if (!touch) return;
		if (Math.abs(touch.clientX - longPressTouchStart.x) > LONG_PRESS_MOVE_TOLERANCE || Math.abs(touch.clientY - longPressTouchStart.y) > LONG_PRESS_MOVE_TOLERANCE) {
			clearLongPressTimer();
			longPressTouchStart = null;
		}
	}

	function finishAudioPress() {
		clearLongPressTimer();
		longPressTouchStart = null;
	}
	function resultTypeLabel() {
		if (activeTab === 'music') return items.length === 1 ? 'Track' : 'Tracks';
		if (activeTab === 'podcast') return items.length === 1 ? 'Episode' : 'Episodes';
		return items.length === 1 ? 'Audiobook' : 'Audiobooks';
	}

	async function classify(category: Category) {
		if (!selectedIDs.length) return;
		showTypeMenu = false;
		saving = true;
		const response = await fetch('/api/audio/items/classify', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ids: selectedIDs, category }) });
		saving = false;
		if (!response.ok) { error = 'Unable to update audio categories'; return; }
		deselectAll(); await load();
	}

	function openGroupAudiobookModal() {
		const selected = items.filter((item) => selectedSet.has(item.id));
		const bookIDs = [...new Set(selected.map((item) => item.book_id))];
		if (bookIDs.length < 2) { error = 'Select tracks from at least two separate book records to group them.'; return; }
		groupAudiobookBookIDs = bookIDs;
		showGroupAudiobookModal = true;
	}

	function handleAudiobookGrouped() {
		showGroupAudiobookModal = false;
		groupAudiobookBookIDs = [];
		deselectAll();
		void load();
	}

	function play(item: AudioItem) { void audioPlayer.playBook(item.book_id, item.file_id); }
	function queue(item: AudioItem, placement: 'append' | 'next' = 'append') { void audioPlayer.addToQueue(item.book_id, item.file_id, placement); }

	async function setPodcastPlayed(item: AudioItem, played: boolean) {
		const response = await fetch(`/api/audio/items/${item.id}/listening`, {
			method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ position_seconds: played ? item.duration_seconds : 0, duration_seconds: item.duration_seconds, status: played ? 'played' : 'unplayed' })
		});
		if (!response.ok) { error = 'Unable to update episode status'; return; }
		items = items.map((entry) => entry.id === item.id ? { ...entry, status: played ? 'played' : 'unplayed', position_seconds: played ? entry.duration_seconds : 0 } : entry);
	}

	async function playItems(nextItems: AudioItem[]) {
		if (!nextItems.length) return;
		if ($audioPlayer.items.length && !confirm('Replace the current audio queue?')) return;
		await audioPlayer.replaceQueue(nextItems.filter((item) => !item.unavailable).map((item) => item.id));
	}

	async function queueItems(nextItems: AudioItem[]) {
		for (const item of nextItems) { if (!item.unavailable) await audioPlayer.addToQueue(item.book_id, item.file_id); }
		audioPlayer.openPanel('queue');
	}

	async function queueSelectedItems() {
		if (!selectedItems.length || saving) return;
		showTypeMenu = false;
		saving = true;
		try { await queueItems(selectedItems); }
		finally { saving = false; }
	}

	async function saveMetadata() {
		if (!editing) return;
		saving = true; error = '';
		const response = await fetch(`/api/audio/items/${editing.id}`, { method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(editing) });
		saving = false;
		if (!response.ok) { error = 'Unable to save audio metadata'; return; }
		editing = null; await load();
	}

	function openAudioEditor(item: AudioItem, event?: MouseEvent) {
		event?.stopPropagation();
		editing = { ...item, artists: [...item.artists] };
	}

	async function createPlaylist(audioIDs = selectedIDs) {
		if (!playlistName.trim()) return;
		saving = true;
		const response = await fetch('/api/audio/playlists', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: playlistName.trim(), audio_ids: audioIDs }) });
		saving = false;
		if (!response.ok) { error = 'Unable to create playlist'; return; }
		playlistName = ''; showPlaylistForm = false; deselectAll();
		if (activeTab === 'playlists') await load();
	}

	async function openPlaylistPicker() {
		showTypeMenu = false;
		const response = await fetch('/api/audio/playlists', { credentials: 'same-origin' });
		if (response.ok) playlists = await response.json();
		showPlaylistPicker = true;
	}

	async function addSelectedToPlaylist(playlist: Playlist) {
		let existing = playlistItems[playlist.id];
		if (!existing) { const response = await fetch(`/api/audio/playlists/${playlist.id}/items`, { credentials: 'same-origin' }); if (!response.ok) return; existing = await response.json(); }
		const selected = items.filter((item) => selectedSet.has(item.id));
		const next = [...existing, ...selected.filter((item) => !existing.some((entry) => entry.id === item.id))];
		await savePlaylistOrder(playlist.id, next); deselectAll(); showPlaylistPicker = false;
	}

	async function togglePlaylist(playlist: Playlist) {
		if (expandedPlaylistID === playlist.id) { expandedPlaylistID = null; return; }
		expandedPlaylistID = playlist.id;
		if (playlistItems[playlist.id]) return;
		const response = await fetch(`/api/audio/playlists/${playlist.id}/items`, { credentials: 'same-origin' });
		if (response.ok) playlistItems = { ...playlistItems, [playlist.id]: await response.json() };
	}

	async function deletePlaylist(playlist: Playlist) {
		if (!confirm(`Delete playlist “${playlist.name}”?`)) return;
		const response = await fetch(`/api/audio/playlists/${playlist.id}`, { method: 'DELETE', credentials: 'same-origin' });
		if (response.ok) await load();
	}

	async function renamePlaylist(playlist: Playlist) {
		const name = prompt('Playlist name', playlist.name)?.trim();
		if (!name || name === playlist.name) return;
		const response = await fetch(`/api/audio/playlists/${playlist.id}`, { method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name, description: playlist.description }) });
		if (response.ok) await load(); else error = 'Unable to rename playlist';
	}

	async function savePlaylistOrder(playlistID: number, nextItems: AudioItem[]) {
		const response = await fetch(`/api/audio/playlists/${playlistID}/items`, { method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ audio_ids: nextItems.map((item) => item.id) }) });
		if (!response.ok) { error = 'Unable to update playlist'; return; }
		playlistItems = { ...playlistItems, [playlistID]: nextItems };
		playlists = playlists.map((playlist) => playlist.id === playlistID ? { ...playlist, item_count: nextItems.length } : playlist);
	}

	function movePlaylistItem(playlistID: number, index: number, direction: -1 | 1) {
		const current = playlistItems[playlistID] ?? []; const target = index + direction;
		if (target < 0 || target >= current.length) return;
		const next = [...current]; [next[index], next[target]] = [next[target], next[index]]; void savePlaylistOrder(playlistID, next);
	}

	function loadRoute() {
		const routeKey = `${$page.url.pathname}${$page.url.search}`;
		if (routeKey === loadedRouteKey) return;
		loadedRouteKey = routeKey;
		const libraryID = Number.parseInt($page.url.searchParams.get('library') || '', 10);
		activeLibraryID = Number.isNaN(libraryID) || libraryID <= 0 ? null : libraryID;
		const tab = $page.url.searchParams.get('tab');
		activeTab = tab === 'music' || tab === 'podcast' || (tab === 'playlists' && activeLibraryID === null) ? tab : 'audiobook';
		if (activeTab === 'podcast' && sort === 'title') sort = 'published';
		else if (activeTab !== 'podcast' && sort === 'published') sort = 'title';
		deselectAll();
		void loadActiveLibrary();
		void load();
	}

	onMount(() => {
		loadRoute();
	});

	afterNavigate(() => {
		loadRoute();
	});
</script>

<svelte:window onclick={handleWindowClick} onkeydown={handleWindowKeydown} />

<svelte:head><title>{activeLibrary?.name || 'All Audio'} · Cryptorum</title></svelte:head>

<div class="min-h-full bg-transparent px-3 py-4 sm:px-5 lg:px-7" style:padding-bottom={selectedIDs.length ? `calc(1rem + ${$bulkActionBarHeight}px)` : undefined}>
	<header class="mx-auto mb-5 flex max-w-7xl flex-wrap items-end justify-between gap-4">
		<div>
			<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--color-primary-400)]">{activeLibraryID === null ? 'Local audio' : 'Audio library'}</p>
			<h1 class="mt-1 text-2xl font-semibold text-[var(--color-surface-text)] sm:text-3xl">{activeLibrary?.name || (activeLibraryID === null ? 'All Audio' : 'Audio Library')}</h1>
			<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">{activeLibraryID === null ? 'Audiobooks, music, and podcast files across all of your libraries.' : 'Audiobooks, music, and podcast files from this folder-backed library.'}</p>
			{#if activeLibraryID !== null}<a href="/audio" class="mt-2 inline-flex text-xs font-medium text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">View all audio</a>{/if}
		</div>
		{#if activeTab !== 'playlists'}
			<form class="flex w-full gap-2 sm:w-auto" onsubmit={(event) => { event.preventDefault(); void load(); }}>
				<input bind:value={query} class="min-w-0 flex-1 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-sm text-[var(--color-surface-text)] outline-none focus:ring-2 focus:ring-[var(--color-primary-500)] sm:w-64" placeholder="Search audio" aria-label="Search audio" />
				<button type="submit" class="accent-action rounded-lg px-4 py-2 text-sm font-semibold">Search</button>
			</form>
		{/if}
	</header>

	<div class="mx-auto max-w-7xl">
		<nav class="mb-4 flex gap-1 overflow-x-auto rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-1" aria-label="Audio sections">
			{#each tabOptions as option}
				<button type="button" class="whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition {activeTab === option[0] ? 'bg-[var(--color-primary-500)]/18 text-[var(--color-primary-300)]' : 'text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)]'}" onclick={() => void setTab(option[0] as Tab)}>{option[1]}</button>
			{/each}
		</nav>

		{#if activeTab !== 'playlists'}
			<div class="mb-4 flex flex-wrap items-center gap-2">
				<select bind:value={statusFilter} onchange={() => void load()} class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-sm text-[var(--color-surface-text)]"><option value="">All progress</option><option value="unplayed">Unplayed</option><option value="in_progress">In progress</option><option value="played">Played</option></select>
				<select bind:value={sort} onchange={() => void load()} class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-sm text-[var(--color-surface-text)]"><option value="title">Title</option>{#if activeTab === 'podcast'}<option value="published">Newest episodes</option>{/if}<option value="recent">Recently played</option><option value="album">Collection order</option><option value="added">Recently added</option></select>
				{#if activeTab === 'music'}
					<div class="flex rounded-lg border border-[var(--color-surface-border)] p-0.5">{#each ['albums', 'artists', 'tracks'] as view}<button type="button" class="rounded-md px-2.5 py-1.5 text-xs capitalize {musicView === view ? 'bg-[var(--color-surface-600)] text-[var(--color-surface-text)]' : 'text-[var(--color-surface-text-muted)]'}" onclick={() => musicView = view as MusicView}>{view}</button>{/each}</div>
				{/if}
				<span class="ml-auto text-xs text-[var(--color-surface-text-muted)]">{items.length} {items.length === 1 ? 'item' : 'items'}</span>
			</div>
		{/if}

		{#if error}<div class="mb-4 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-300" role="alert">{error}</div>{/if}

		{#if loading}
			<div class="flex min-h-56 items-center justify-center"><div class="h-9 w-9 animate-spin rounded-full border-2 border-[var(--color-surface-border)] border-t-[var(--color-primary-400)]"></div></div>
		{:else if activeTab === 'playlists'}
			<div class="mb-4 flex justify-end"><button type="button" class="accent-action rounded-lg px-3 py-2 text-sm font-semibold" onclick={() => showPlaylistForm = true}>New playlist</button></div>
			{#if playlists.length === 0}<div class="rounded-2xl border border-dashed border-[var(--color-surface-border)] p-10 text-center text-[var(--color-surface-text-muted)]">No playlists yet.</div>{/if}
			<div class="space-y-3">
				{#each playlists as playlist}
					<section class="overflow-hidden rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)]">
						<div class="flex flex-wrap items-center gap-2 p-3 sm:gap-3">
							<button type="button" class="min-w-44 flex-1 text-left" onclick={() => void togglePlaylist(playlist)}><span class="block truncate font-semibold text-[var(--color-surface-text)]">{playlist.name}</span><span class="text-xs text-[var(--color-surface-text-muted)]">{playlist.item_count} tracks · {formatDuration(playlist.duration_seconds)}</span></button>
							<button type="button" class="accent-action rounded-lg px-3 py-2 text-sm" disabled={!playlistItems[playlist.id]?.length && expandedPlaylistID !== playlist.id} onclick={async () => { if (!playlistItems[playlist.id]) await togglePlaylist(playlist); await playItems(playlistItems[playlist.id] ?? []); }}>Play</button>
							<button type="button" class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)]" onclick={async () => { if (!playlistItems[playlist.id]) await togglePlaylist(playlist); await queueItems(playlistItems[playlist.id] ?? []); }}>Queue</button>
							<button type="button" class="rounded-lg px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)]" onclick={() => void renamePlaylist(playlist)}>Rename</button>
							<button type="button" class="rounded-lg px-3 py-2 text-sm text-red-300 hover:bg-red-500/10" onclick={() => void deletePlaylist(playlist)}>Delete</button>
						</div>
						{#if expandedPlaylistID === playlist.id}<div class="border-t border-[var(--color-surface-border)] p-2">{#each playlistItems[playlist.id] ?? [] as item, index}<div class="flex items-center gap-2 rounded-lg px-2 py-2 hover:bg-[var(--color-surface-base)]"><button type="button" class="flex h-8 w-8 items-center justify-center rounded-full text-[var(--color-primary-300)]" onclick={() => play(item)}>▶</button><div class="min-w-0 flex-1"><div class="truncate text-sm font-medium text-[var(--color-surface-text)]">{item.title}</div><div class="truncate text-xs text-[var(--color-surface-text-muted)]">{item.artists.join(', ') || item.filename}</div></div><button type="button" class="rounded p-1.5 text-[var(--color-surface-text-muted)]" disabled={index === 0} onclick={() => movePlaylistItem(playlist.id, index, -1)}>↑</button><button type="button" class="rounded p-1.5 text-[var(--color-surface-text-muted)]" disabled={index === (playlistItems[playlist.id]?.length ?? 0) - 1} onclick={() => movePlaylistItem(playlist.id, index, 1)}>↓</button><button type="button" class="rounded p-1.5 text-red-300" onclick={() => void savePlaylistOrder(playlist.id, (playlistItems[playlist.id] ?? []).filter((entry) => entry.id !== item.id))}>×</button></div>{/each}</div>{/if}
					</section>
				{/each}
			</div>
		{:else if items.length === 0}
			<div class="rounded-2xl border border-dashed border-[var(--color-surface-border)] p-10 text-center"><div class="text-4xl">♪</div><h2 class="mt-3 font-semibold text-[var(--color-surface-text)]">No {categoryLabel(activeTab)} found</h2><p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Scan a library containing supported audio files, or reclassify existing audio.</p></div>
		{:else if activeTab === 'music' && musicView !== 'tracks' || activeTab === 'podcast'}
			<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
				{#each groupedItems as [name, group]}
					<section class="overflow-hidden rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-sm">
						<div class="flex items-center gap-3 border-b border-[var(--color-surface-border)] p-3"><BookCoverFrame src={`/api/covers/${group[0].book_id}/thumb?size=small`} alt={`${name} cover`} format={group[0].format} placeholderKind="audio" placeholderSize="xs" frameClass="h-14 w-14 flex-none rounded-lg" /><div class="min-w-0 flex-1"><h2 class="truncate font-semibold text-[var(--color-surface-text)]">{name}</h2><p class="text-xs text-[var(--color-surface-text-muted)]">{group.length} {activeTab === 'podcast' ? (group.length === 1 ? 'episode' : 'episodes') : (group.length === 1 ? 'track' : 'tracks')} · {formatDuration(group.reduce((sum, item) => sum + item.duration_seconds, 0))}</p></div><button type="button" class="accent-action rounded-full p-2.5" aria-label={`Play ${name}`} onclick={() => void playItems(group)}>▶</button></div>
						<div class="max-h-72 overflow-y-auto p-2">{#each group as item}{@render AudioRow(item, true)}{/each}</div>
					</section>
				{/each}
			</div>
		{:else}
			<div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
				{#each items as item}{@render AudioCard(item)}{/each}
			</div>
		{/if}
	</div>
</div>

{#snippet AudioRow(item: AudioItem, compact = false)}
	<div
		class="group flex items-center gap-2 rounded-lg px-2 py-2 transition {selectedSet.has(item.id) ? 'bg-[var(--color-primary-500)]/10 ring-1 ring-inset ring-[var(--color-primary-500)]' : 'hover:bg-[var(--color-surface-base)]'}"
		data-audio-id={item.id}
		onclick={handleAudioClick}
		onkeydown={handleAudioKeydown}
		onmousedown={handleAudioMouseDown}
		onmouseup={finishAudioPress}
		onmouseleave={finishAudioPress}
		ontouchstart={handleAudioTouchStart}
		ontouchmove={handleAudioTouchMove}
		ontouchend={finishAudioPress}
		ontouchcancel={finishAudioPress}
		role="button"
		tabindex="0"
	>
		<button type="button" data-audio-action class="bulk-select-checkbox h-5 w-5 shrink-0 rounded transition-colors" class:is-selected={selectedSet.has(item.id)} aria-pressed={selectedSet.has(item.id)} aria-label={selectedSet.has(item.id) ? `Deselect ${item.title}` : `Select ${item.title}`} onclick={(event) => toggleSelected(item.id, event)}>
			{#if selectedSet.has(item.id)}<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7"></path></svg>{/if}
		</button>
		<button type="button" data-audio-action class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-[var(--color-primary-300)] hover:bg-[var(--color-primary-500)]/15" disabled={item.unavailable} aria-label={`Play ${item.title}`} onclick={() => play(item)}>▶</button>
		<div class="min-w-0 flex-1"><div class="truncate text-sm font-medium text-[var(--color-surface-text)]">{item.track_number ? `${item.track_number}. ` : ''}{item.title}</div>{#if !compact}<div class="truncate text-xs text-[var(--color-surface-text-muted)]">{item.artists.join(', ') || item.filename}</div>{/if}</div>
		<span class="text-[10px] tabular-nums text-[var(--color-surface-text-muted)]">{formatDuration(item.duration_seconds)}</span>
		{#if activeTab === 'podcast'}<button type="button" data-audio-action class="rounded px-1.5 py-1 text-[10px] text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" title={item.status === 'played' ? 'Mark unplayed' : 'Mark played'} onclick={() => void setPodcastPlayed(item, item.status !== 'played')}>{item.status === 'played' ? 'Unplayed' : 'Played'}</button>{/if}
		<button type="button" data-audio-action class="rounded p-1.5 text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" disabled={item.unavailable} title="Play next" onclick={() => queue(item, 'next')}>+1</button>
	</div>
{/snippet}

{#snippet AudioCard(item: AudioItem)}
	<div
		class="relative flex gap-3 rounded-2xl border bg-[var(--color-surface-overlay)] p-3 shadow-sm transition {selectedSet.has(item.id) ? 'border-[var(--color-primary-500)] ring-1 ring-[var(--color-primary-500)]' : 'border-[var(--color-surface-border)] hover:border-[var(--color-surface-500)]'}"
		data-audio-id={item.id}
		onclick={handleAudioClick}
		onkeydown={handleAudioKeydown}
		onmousedown={handleAudioMouseDown}
		onmouseup={finishAudioPress}
		onmouseleave={finishAudioPress}
		ontouchstart={handleAudioTouchStart}
		ontouchmove={handleAudioTouchMove}
		ontouchend={finishAudioPress}
		ontouchcancel={finishAudioPress}
		role="button"
		tabindex="0"
	>
		<button type="button" data-audio-action class="bulk-select-checkbox mt-1 h-5 w-5 shrink-0 rounded transition-colors" class:is-selected={selectedSet.has(item.id)} aria-pressed={selectedSet.has(item.id)} aria-label={selectedSet.has(item.id) ? `Deselect ${item.title}` : `Select ${item.title}`} onclick={(event) => toggleSelected(item.id, event)}>
			{#if selectedSet.has(item.id)}<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7"></path></svg>{/if}
		</button>
		<BookCoverFrame src={`/api/covers/${item.book_id}/thumb?size=small`} alt={`${item.title} cover`} format={item.format} placeholderKind="audio" placeholderSize="xs" frameClass="h-24 w-16 flex-none rounded-lg shadow" />
		<div class="min-w-0 flex-1"><h2 class="line-clamp-2 font-semibold leading-tight text-[var(--color-surface-text)]">{item.title}</h2><p class="mt-1 truncate text-xs text-[var(--color-surface-text-muted)]">{item.artists.join(', ') || item.filename}</p>{#if item.chapter_count || item.bookmark_count}<p class="mt-1 text-[10px] text-[var(--color-surface-text-muted)]">{item.chapter_count ? `${item.chapter_count} chapters` : ''}{item.chapter_count && item.bookmark_count ? ' · ' : ''}{item.bookmark_count ? `${item.bookmark_count} bookmarks` : ''}</p>{/if}<div class="mt-3 flex flex-wrap gap-1.5"><button type="button" data-audio-action class="accent-action rounded-lg px-2.5 py-1.5 text-xs" disabled={item.unavailable} onclick={() => play(item)}>Play</button><button type="button" data-audio-action class="rounded-lg border border-[var(--color-surface-border)] px-2.5 py-1.5 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)] disabled:opacity-50" disabled={item.unavailable} onclick={() => queue(item)}>Queue</button><button type="button" data-audio-action class="rounded-lg px-2 py-1.5 text-xs text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)]" onclick={(event) => openAudioEditor(item, event)}>Edit</button></div></div>
	</div>
{/snippet}

{#if selectedIDs.length}
	<div class="fixed bottom-0 left-0 right-0 z-[10010] animate-slide-up" use:trackBulkActionBar>
		<div class="border-t border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl backdrop-blur-lg">
			<div class="mx-auto max-w-7xl px-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] pt-3 sm:px-4">
				<div class="bulk-action-layout">
					<div class="bulk-selection-cluster">
						<span class="text-sm font-medium text-[var(--color-surface-text)] sm:text-base">{selectedIDs.length} selected</span>
						<div class="bulk-selection-controls">
							{#if !allResultsSelected}
								<button type="button" onclick={selectAllResults} class="rounded-lg bg-[var(--color-surface-700)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]">
									Select All {items.length} {resultTypeLabel()}
								</button>
							{/if}
							<button type="button" onclick={deselectAll} class="rounded-lg bg-[var(--color-surface-700)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]">Deselect</button>
						</div>
					</div>

					<div class="bulk-primary-actions">
						<button type="button" disabled={saving || selectedItems.every((item) => item.unavailable)} onclick={() => void queueSelectedItems()} class="flex items-center justify-center gap-2 rounded-lg bg-[var(--color-surface-700)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none">
							<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M3 6h10M3 12h7M3 18h5"/><path d="M17 13v8m-4-4h8"/></svg>
							<span>Add to Queue</span>
						</button>
						<button type="button" disabled={saving} onclick={() => void openPlaylistPicker()} class="accent-action flex items-center justify-center gap-2 rounded-lg px-4 py-2 text-sm font-medium">
							<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M4 6h9M4 11h9M4 16h6"/><path d="M17 9v9m-3-3h6"/></svg>
							<span>Add to Playlist</span>
						</button>
						<div class="relative w-full sm:w-auto" bind:this={typeMenuContainer}>
							<button bind:this={typeMenuButton} type="button" disabled={saving} aria-haspopup="menu" aria-expanded={showTypeMenu} onclick={() => showTypeMenu = !showTypeMenu} class="flex w-full items-center justify-center gap-2 rounded-lg bg-[var(--color-surface-700)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] disabled:opacity-50 sm:w-auto">
								<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M4 7h10M4 12h7M4 17h4"/><path d="m15 14 3 3 3-3M18 17V7"/></svg>
								<span>Change Type</span>
							</button>
							{#if showTypeMenu}
								<div class="floating-surface bulk-menu-surface absolute bottom-full left-0 z-20 mb-2 w-64 overflow-hidden rounded-lg border sm:left-auto sm:right-0" role="menu">
									{#each ['audiobook', 'music', 'podcast'] as category}
										<button type="button" role="menuitem" disabled={activeTab === category || saving} onclick={() => void classify(category as Category)} class="block w-full px-4 py-3 text-left text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] disabled:cursor-default disabled:text-[var(--color-surface-text-muted)]">
											<span class="font-medium">{categoryLabel(category as Category)}</span>{#if activeTab === category}<span class="ml-2 text-xs">Current type</span>{/if}
										</button>
									{/each}
								</div>
							{/if}
						</div>
						{#if activeTab === 'audiobook'}
							<button type="button" disabled={saving || !canGroupAudiobook} title={canGroupAudiobook ? 'Group selected records into one audiobook' : 'Select tracks from at least two separate audiobook records'} onclick={() => { showTypeMenu = false; openGroupAudiobookModal(); }} class="flex items-center justify-center gap-2 rounded-lg bg-[var(--color-surface-700)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none">
								<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M8 7h12M8 12h12M8 17h8M4 7h.01M4 12h.01M4 17h.01"/></svg>
								<span>Group Audiobook</span>
							</button>
						{/if}
					</div>
				</div>
			</div>
		</div>
	</div>
{/if}

{#if showGroupAudiobookModal}
	<CombineBooksModal
		bookIds={groupAudiobookBookIDs}
		variant="audio"
		onClose={() => { showGroupAudiobookModal = false; groupAudiobookBookIDs = []; }}
		onCombined={handleAudiobookGrouped}
	/>
{/if}

{#if editing}
	<div class="fixed inset-0 z-[11000] flex items-center justify-center bg-black/70 p-4" role="presentation" onclick={(event) => { if (event.target === event.currentTarget) editing = null; }}>
		<form class="max-h-[90vh] w-full max-w-xl overflow-y-auto rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5 shadow-2xl backdrop-blur-xl" onsubmit={(event) => { event.preventDefault(); void saveMetadata(); }}>
			<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Edit audio metadata</h2>
			<div class="mt-4 grid gap-3 sm:grid-cols-2">
				<label class="sm:col-span-2 text-sm text-[var(--color-surface-text-muted)]">Title<input required bind:value={editing.title} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" /></label>
				<label class="text-sm text-[var(--color-surface-text-muted)]">Category<select bind:value={editing.category} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]"><option value="audiobook">Audiobook</option><option value="music">Music</option><option value="podcast">Podcast</option></select></label>
				<label class="text-sm text-[var(--color-surface-text-muted)]">Artists<input value={editing.artists.join(', ')} oninput={(event) => editing && (editing.artists = event.currentTarget.value.split(',').map((value) => value.trim()).filter(Boolean))} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" /></label>
				<label class="text-sm text-[var(--color-surface-text-muted)]">Album / collection<input bind:value={editing.album} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" /></label>
				<label class="text-sm text-[var(--color-surface-text-muted)]">Album artist<input bind:value={editing.album_artist} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" /></label>
				<label class="text-sm text-[var(--color-surface-text-muted)]">Show<input bind:value={editing.show_title} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" /></label>
				<label class="text-sm text-[var(--color-surface-text-muted)]">Genre<input bind:value={editing.genre} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" /></label>
				<label class="text-sm text-[var(--color-surface-text-muted)]">Track number<input type="number" min="0" value={editing.track_number ?? ''} oninput={(event) => editing && (editing.track_number = event.currentTarget.value ? Number(event.currentTarget.value) : null)} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" /></label>
				<label class="text-sm text-[var(--color-surface-text-muted)]">Disc number<input type="number" min="0" value={editing.disc_number ?? ''} oninput={(event) => editing && (editing.disc_number = event.currentTarget.value ? Number(event.currentTarget.value) : null)} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" /></label>
				<label class="text-sm text-[var(--color-surface-text-muted)]">Release date<input bind:value={editing.release_date} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" /></label>
				<label class="text-sm text-[var(--color-surface-text-muted)]">Episode number<input type="number" min="0" value={editing.episode_number ?? ''} oninput={(event) => editing && (editing.episode_number = event.currentTarget.value ? Number(event.currentTarget.value) : null)} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" /></label>
				<label class="text-sm text-[var(--color-surface-text-muted)] sm:col-span-2">Published date<input bind:value={editing.published_at} class="mt-1 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" /></label>
			</div>
			<div class="mt-5 flex justify-end gap-2"><button type="button" class="rounded-lg px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)]" onclick={() => editing = null}>Cancel</button><button type="submit" class="accent-action rounded-lg px-4 py-2 text-sm font-semibold" disabled={saving}>{saving ? 'Saving…' : 'Save'}</button></div>
		</form>
	</div>
{/if}

{#if showPlaylistForm}
	<div class="fixed inset-0 z-[11000] flex items-center justify-center bg-black/70 p-4" role="presentation" onclick={(event) => { if (event.target === event.currentTarget) showPlaylistForm = false; }}><form class="w-full max-w-sm rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5 shadow-2xl backdrop-blur-xl" onsubmit={(event) => { event.preventDefault(); void createPlaylist(); }}><h2 class="font-semibold text-[var(--color-surface-text)]">New playlist</h2><input bind:value={playlistName} class="mt-4 w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]" placeholder="Playlist name" /><p class="mt-2 text-xs text-[var(--color-surface-text-muted)]">{selectedIDs.length ? `${selectedIDs.length} selected items will be added.` : 'The playlist will start empty.'}</p><div class="mt-4 flex justify-end gap-2"><button type="button" class="rounded-lg px-3 py-2 text-sm text-[var(--color-surface-text-muted)]" onclick={() => showPlaylistForm = false}>Cancel</button><button type="submit" class="accent-action rounded-lg px-4 py-2 text-sm" disabled={saving || !playlistName.trim()}>Create</button></div></form></div>
{/if}

{#if showPlaylistPicker}
	<div class="fixed inset-0 z-[11000] flex items-center justify-center bg-black/70 p-4" role="presentation" onclick={(event) => { if (event.target === event.currentTarget) showPlaylistPicker = false; }}><div class="w-full max-w-sm rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4 shadow-2xl backdrop-blur-xl"><div class="flex items-center justify-between"><h2 class="font-semibold text-[var(--color-surface-text)]">Add to playlist</h2><button type="button" class="rounded p-1.5 text-[var(--color-surface-text-muted)]" onclick={() => showPlaylistPicker = false}>✕</button></div><div class="mt-3 max-h-72 space-y-1 overflow-y-auto">{#each playlists as playlist}<button type="button" class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left hover:bg-[var(--color-surface-700)]" onclick={() => void addSelectedToPlaylist(playlist)}><span class="truncate text-sm text-[var(--color-surface-text)]">{playlist.name}</span><span class="text-xs text-[var(--color-surface-text-muted)]">{playlist.item_count}</span></button>{/each}{#if !playlists.length}<p class="py-4 text-center text-sm text-[var(--color-surface-text-muted)]">No playlists yet.</p>{/if}</div><button type="button" class="accent-action mt-3 w-full rounded-lg px-3 py-2 text-sm" onclick={() => { showPlaylistPicker = false; showPlaylistForm = true; }}>Create new playlist</button></div></div>
{/if}

<style>
	@keyframes audio-bulk-slide-up {
		from { transform: translateY(100%); }
		to { transform: translateY(0); }
	}

	.animate-slide-up {
		animation: audio-bulk-slide-up 200ms ease-out;
	}
</style>

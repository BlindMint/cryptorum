<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import { audioPlayer } from '$lib/stores/audioPlayer';
	import { trackBulkActionBar } from '$lib/stores/bulkActionBar';

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

	const selectedSet = $derived(new Set(selectedIDs));
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
		selectedIDs = [];
		if (tab === 'podcast' && sort === 'title') sort = 'published';
		else if (tab !== 'podcast' && sort === 'published') sort = 'title';
		const url = new URL($page.url);
		url.searchParams.set('tab', tab);
		await goto(`${url.pathname}${url.search}`, { replaceState: true, noScroll: true, keepFocus: true });
		await load();
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
				if (query.trim()) params.set('q', query.trim());
				if (statusFilter) params.set('status', statusFilter);
				const response = await fetch(`/api/audio/items?${params}`, { credentials: 'same-origin' });
				if (!response.ok) throw new Error('Unable to load the audio library');
				const data = await response.json(); items = data.items ?? [];
			}
		} catch (reason) { error = reason instanceof Error ? reason.message : 'Unable to load audio'; }
		finally { loading = false; }
	}

	function toggleSelected(id: number) { selectedIDs = selectedSet.has(id) ? selectedIDs.filter((value) => value !== id) : [...selectedIDs, id]; }

	async function classify(category: Category) {
		if (!selectedIDs.length) return;
		saving = true;
		const response = await fetch('/api/audio/items/classify', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ids: selectedIDs, category }) });
		saving = false;
		if (!response.ok) { error = 'Unable to update audio categories'; return; }
		selectedIDs = []; await load();
	}

	async function groupAudiobook() {
		const selected = items.filter((item) => selectedSet.has(item.id));
		const bookIDs = [...new Set(selected.map((item) => item.book_id))];
		if (bookIDs.length < 2) { error = 'Select tracks from at least two separate book records to group them.'; return; }
		if (!confirm(`Group ${bookIDs.length} records under “${selected[0].title}”?`)) return;
		saving = true;
		const response = await fetch('/api/books/combine', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ primary_book_id: bookIDs[0], book_ids: bookIDs }) });
		saving = false;
		if (!response.ok) { error = await response.text() || 'Unable to group audiobook'; return; }
		selectedIDs = []; await load();
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

	async function saveMetadata() {
		if (!editing) return;
		saving = true; error = '';
		const response = await fetch(`/api/audio/items/${editing.id}`, { method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(editing) });
		saving = false;
		if (!response.ok) { error = 'Unable to save audio metadata'; return; }
		editing = null; await load();
	}

	async function createPlaylist(audioIDs = selectedIDs) {
		if (!playlistName.trim()) return;
		saving = true;
		const response = await fetch('/api/audio/playlists', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: playlistName.trim(), audio_ids: audioIDs }) });
		saving = false;
		if (!response.ok) { error = 'Unable to create playlist'; return; }
		playlistName = ''; showPlaylistForm = false; selectedIDs = [];
		if (activeTab === 'playlists') await load();
	}

	async function openPlaylistPicker() {
		const response = await fetch('/api/audio/playlists', { credentials: 'same-origin' });
		if (response.ok) playlists = await response.json();
		showPlaylistPicker = true;
	}

	async function addSelectedToPlaylist(playlist: Playlist) {
		let existing = playlistItems[playlist.id];
		if (!existing) { const response = await fetch(`/api/audio/playlists/${playlist.id}/items`, { credentials: 'same-origin' }); if (!response.ok) return; existing = await response.json(); }
		const selected = items.filter((item) => selectedSet.has(item.id));
		const next = [...existing, ...selected.filter((item) => !existing.some((entry) => entry.id === item.id))];
		await savePlaylistOrder(playlist.id, next); selectedIDs = []; showPlaylistPicker = false;
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

	onMount(() => {
		const tab = $page.url.searchParams.get('tab');
		if (tab === 'music' || tab === 'podcast' || tab === 'playlists') {
			activeTab = tab;
			if (tab === 'podcast') sort = 'published';
		}
		void load();
	});
</script>

<svelte:head><title>Audio · Cryptorum</title></svelte:head>

<div class="min-h-full bg-transparent px-3 py-4 sm:px-5 lg:px-7">
	<header class="mx-auto mb-5 flex max-w-7xl flex-wrap items-end justify-between gap-4">
		<div>
			<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--color-primary-400)]">Local library</p>
			<h1 class="mt-1 text-2xl font-semibold text-[var(--color-surface-text)] sm:text-3xl">Audio</h1>
			<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Audiobooks, music, and podcast files stored in your libraries.</p>
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
			{#each [['audiobook', 'Audiobooks'], ['music', 'Music'], ['podcast', 'Podcasts'], ['playlists', 'Playlists']] as option}
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
	<div class="group flex items-center gap-2 rounded-lg px-2 py-2 hover:bg-[var(--color-surface-base)]">
		<button type="button" class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-[var(--color-primary-300)] hover:bg-[var(--color-primary-500)]/15" disabled={item.unavailable} aria-label={`Play ${item.title}`} onclick={() => play(item)}>▶</button>
		<div class="min-w-0 flex-1"><div class="truncate text-sm font-medium text-[var(--color-surface-text)]">{item.track_number ? `${item.track_number}. ` : ''}{item.title}</div>{#if !compact}<div class="truncate text-xs text-[var(--color-surface-text-muted)]">{item.artists.join(', ') || item.filename}</div>{/if}</div>
		<span class="text-[10px] tabular-nums text-[var(--color-surface-text-muted)]">{formatDuration(item.duration_seconds)}</span>
		{#if activeTab === 'podcast'}<button type="button" class="rounded px-1.5 py-1 text-[10px] text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" title={item.status === 'played' ? 'Mark unplayed' : 'Mark played'} onclick={() => void setPodcastPlayed(item, item.status !== 'played')}>{item.status === 'played' ? 'Unplayed' : 'Played'}</button>{/if}
		<button type="button" class="rounded p-1.5 text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" disabled={item.unavailable} title="Play next" onclick={() => queue(item, 'next')}>+1</button>
	</div>
{/snippet}

{#snippet AudioCard(item: AudioItem)}
	<article class="relative flex gap-3 rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-3 shadow-sm transition hover:border-[var(--color-surface-500)]">
		<input type="checkbox" class="mt-1 accent-[var(--color-primary-500)]" checked={selectedSet.has(item.id)} onchange={() => toggleSelected(item.id)} aria-label={`Select ${item.title}`} />
		<BookCoverFrame src={`/api/covers/${item.book_id}/thumb?size=small`} alt={`${item.title} cover`} format={item.format} placeholderKind="audio" placeholderSize="xs" frameClass="h-24 w-16 flex-none rounded-lg shadow" />
		<div class="min-w-0 flex-1"><h2 class="line-clamp-2 font-semibold leading-tight text-[var(--color-surface-text)]">{item.title}</h2><p class="mt-1 truncate text-xs text-[var(--color-surface-text-muted)]">{item.artists.join(', ') || item.filename}</p>{#if item.chapter_count || item.bookmark_count}<p class="mt-1 text-[10px] text-[var(--color-surface-text-muted)]">{item.chapter_count ? `${item.chapter_count} chapters` : ''}{item.chapter_count && item.bookmark_count ? ' · ' : ''}{item.bookmark_count ? `${item.bookmark_count} bookmarks` : ''}</p>{/if}<div class="mt-3 flex flex-wrap gap-1.5"><button type="button" class="accent-action rounded-lg px-2.5 py-1.5 text-xs" disabled={item.unavailable} onclick={() => play(item)}>Play</button><button type="button" class="rounded-lg border border-[var(--color-surface-border)] px-2.5 py-1.5 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)] disabled:opacity-50" disabled={item.unavailable} onclick={() => queue(item)}>Queue</button><button type="button" class="rounded-lg px-2 py-1.5 text-xs text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)]" onclick={() => editing = structuredClone(item)}>Edit</button></div></div>
	</article>
{/snippet}

{#if selectedIDs.length}
	<div class="fixed bottom-3 left-1/2 z-[9990] flex max-w-[94vw] -translate-x-1/2 flex-wrap items-center justify-center gap-2 rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 shadow-2xl backdrop-blur-xl" use:trackBulkActionBar>
		<span class="text-sm font-semibold text-[var(--color-surface-text)]">{selectedIDs.length} selected</span>
		{#each ['audiobook', 'music', 'podcast'] as category}<button type="button" class="rounded-lg px-2.5 py-1.5 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)]" disabled={saving} onclick={() => void classify(category as Category)}>Move to {categoryLabel(category as Category)}</button>{/each}
		{#if activeTab === 'audiobook'}<button type="button" class="rounded-lg px-2.5 py-1.5 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)]" disabled={saving} onclick={() => void groupAudiobook()}>Group audiobook</button>{/if}
		<button type="button" class="rounded-lg px-2.5 py-1.5 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)]" onclick={() => void openPlaylistPicker()}>Add to playlist</button>
		<button type="button" class="rounded-lg p-1.5 text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)]" aria-label="Clear selection" onclick={() => selectedIDs = []}>✕</button>
	</div>
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

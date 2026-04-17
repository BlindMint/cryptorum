<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { flip } from 'svelte/animate';
	import { cubicOut } from 'svelte/easing';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import { showFormatOnCover, getFormatColor } from '$lib/stores';

 	let stats = $state({
 		books: 0,
 		libraries: 0,
 		reading: 0,
 		finished: 0
 	});

 	// Dashboard sections
  let continueReadingBooks = $state<any[]>([]);
  let recentBooks = $state<any[]>([]);
 	let discoverBooks = $state<any[]>([]);
 	let formatOnCover = $state(true);
 	let continueReadingRowEl = $state<HTMLDivElement | null>(null);
 	let recentBooksRowEl = $state<HTMLDivElement | null>(null);
 	let discoverBooksRowEl = $state<HTMLDivElement | null>(null);
 	let dashboardBookCardWidth = $state(148);
 	let dashboardBookRowGap = $state(16);
 	let visibleContinueReadingCount = $state(0);
 	let visibleRecentBooksCount = $state(0);
 	let visibleDiscoverBooksCount = $state(0);

 	// Dashboard configuration
 	let dashboardConfig = $state({
 		showContinueReading: true,
 		showRecentlyAdded: true,
 		showDiscover: true,
 		continueReadingLimit: 6,
 		recentlyAddedLimit: 6,
 		discoverLimit: 6
 	});

  	let showConfigModal = $state(false);

 	$effect(() => {
 		const unsub = showFormatOnCover.subscribe((v: boolean) => formatOnCover = v);
 		return unsub;
	});

	$effect(() => {
		continueReadingBooks.length;
		recentBooks.length;
		discoverBooks.length;
		dashboardConfig.continueReadingLimit;
		dashboardConfig.recentlyAddedLimit;
		dashboardConfig.discoverLimit;
		dashboardBookCardWidth;
		dashboardBookRowGap;
		updateDashboardRows();
	});

 	onMount(async () => {
 		showFormatOnCover.init();
 		try {
			const [libsRes, statsRes, continueRes, recentRes, discoverRes] = await Promise.all([
				fetch('/api/libraries'),
				fetch('/api/stats'),
				fetch('/api/books?status=reading&sort=last_read&limit=6'),
				fetch('/api/books?limit=6'),
				fetch('/api/books?sort=random&limit=6')
			]);

			if (libsRes.ok) {
				const libs = await libsRes.json();
				stats.libraries = libs.length;
			}

			if (statsRes.ok) {
				const s = await statsRes.json();
				stats.books = s.total_books;
				stats.reading = s.reading;
				stats.finished = s.finished;
			}

			if (continueRes.ok) {
				const data = await continueRes.json();
				continueReadingBooks = data.books || [];
			}

			if (recentRes.ok) {
				const data = await recentRes.json();
				recentBooks = (data.books || []).slice(0, 6);
				if (!statsRes.ok) stats.books = data.total || 0;
			}

			if (discoverRes.ok) {
				const data = await discoverRes.json();
				discoverBooks = data.books || [];
			}
			updateDashboardRows();
		} catch (e) {
			console.error('Failed to fetch data:', e);
		}
	});

	onMount(() => {
		const updateDimensions = () => {
			const viewportWidth = window.innerWidth;
			if (viewportWidth < 640) {
				dashboardBookCardWidth = 124;
				dashboardBookRowGap = 12;
			} else if (viewportWidth < 1024) {
				dashboardBookCardWidth = 136;
				dashboardBookRowGap = 14;
			} else {
				dashboardBookCardWidth = 148;
				dashboardBookRowGap = 16;
			}
			updateDashboardRows();
		};

		const resizeObserver = new ResizeObserver(() => {
			updateDashboardRows();
		});

		void tick().then(() => {
			if (continueReadingRowEl) resizeObserver.observe(continueReadingRowEl);
			if (recentBooksRowEl) resizeObserver.observe(recentBooksRowEl);
			if (discoverBooksRowEl) resizeObserver.observe(discoverBooksRowEl);
			updateDimensions();
		});

		window.addEventListener('resize', updateDimensions);

		return () => {
			resizeObserver.disconnect();
			window.removeEventListener('resize', updateDimensions);
		};
	});

	function toggleConfigModal() {
		showConfigModal = !showConfigModal;
	}

	function getVisibleBookCount(container: HTMLElement | null, totalCount: number) {
		if (!container || totalCount <= 0) return 0;
		const width = container.clientWidth;
		const slotWidth = dashboardBookCardWidth + dashboardBookRowGap;
		const fit = Math.floor((width + dashboardBookRowGap) / slotWidth);
		return Math.max(1, Math.min(totalCount, fit));
	}

	function updateDashboardRows() {
		visibleContinueReadingCount = getVisibleBookCount(
			continueReadingRowEl,
			Math.min(continueReadingBooks.length, dashboardConfig.continueReadingLimit)
		);
		visibleRecentBooksCount = getVisibleBookCount(
			recentBooksRowEl,
			Math.min(recentBooks.length, dashboardConfig.recentlyAddedLimit)
		);
		visibleDiscoverBooksCount = getVisibleBookCount(
			discoverBooksRowEl,
			Math.min(discoverBooks.length, dashboardConfig.discoverLimit)
		);
	}
</script>

<div class="flex h-full flex-col gap-4 overflow-hidden">
	<div class="hidden sm:grid grid-cols-2 lg:grid-cols-4 gap-3">
		<div class="bg-[var(--color-surface-overlay)] rounded-lg p-4 border border-[var(--color-surface-border)]">
			<div class="flex items-center justify-between">
				<div>
					<p class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">Total Books</p>
					<p class="text-2xl font-bold text-[var(--color-surface-text)] mt-1">{stats.books}</p>
				</div>
				<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center flex-shrink-0">
					<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
					</svg>
				</div>
			</div>
		</div>

		<div class="bg-[var(--color-surface-overlay)] rounded-lg p-4 border border-[var(--color-surface-border)]">
			<div class="flex items-center justify-between">
				<div>
					<p class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">Reading</p>
					<p class="text-2xl font-bold text-[var(--color-surface-text)] mt-1">{stats.reading}</p>
				</div>
				<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center flex-shrink-0">
					<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
					</svg>
				</div>
			</div>
		</div>

		<div class="bg-[var(--color-surface-overlay)] rounded-lg p-4 border border-[var(--color-surface-border)]">
			<div class="flex items-center justify-between">
				<div>
					<p class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">Finished</p>
					<p class="text-2xl font-bold text-[var(--color-surface-text)] mt-1">{stats.finished}</p>
				</div>
				<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center flex-shrink-0">
					<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path>
					</svg>
				</div>
			</div>
		</div>

		<div class="bg-[var(--color-surface-overlay)] rounded-lg p-4 border border-[var(--color-surface-border)]">
			<div class="flex items-center justify-between">
				<div>
					<p class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">Libraries</p>
					<p class="text-2xl font-bold text-[var(--color-surface-text)] mt-1">{stats.libraries}</p>
				</div>
				<div class="w-10 h-10 rounded-lg bg-[var(--color-primary-500)]/20 flex items-center justify-center flex-shrink-0">
					<svg class="w-5 h-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
					</svg>
				</div>
			</div>
		</div>
	</div>

	<!-- Continue Reading Section -->
	{#if dashboardConfig.showContinueReading}
	<div class="bg-[var(--color-surface-overlay)] rounded-lg p-4 border border-[var(--color-surface-border)] flex-none sm:flex-1 min-h-0 flex flex-col">
		<div class="flex items-center justify-between mb-3">
			<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Continue Reading</h2>
			<a href="/library?status=reading" class="text-sm text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">View all →</a>
		</div>
			{#if continueReadingBooks.length > 0}
				<div
					bind:this={continueReadingRowEl}
					class="dashboard-book-row"
					style="--dashboard-book-card-width: {dashboardBookCardWidth}px; --dashboard-book-row-gap: {dashboardBookRowGap}px;"
				>
					{#each continueReadingBooks.slice(0, visibleContinueReadingCount || 1) as book (book.id)}
						{@const readerUrl = (() => {
							if (book.format === 'pdf') return `/reader/pdf/${book.id}`;
							if (['cbz', 'cbr', 'cb7', 'cbt'].includes(book.format)) return `/reader/cbx/${book.id}`;
							if (['mp3', 'm4a', 'm4b', 'flac', 'ogg', 'wav'].includes(book.format)) return `/reader/audio/${book.id}`;
							return `/reader/epub/${book.id}`;
						})()}
						<div class="dashboard-book-item relative group min-w-0 self-start" animate:flip={{ duration: 90, easing: cubicOut }}>
							<a href="/book/{book.id}" class="flex min-w-0 flex-col">
								<div class="relative">
									<BookCoverFrame
										src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
										alt={book.title}
										mode="contain"
										frameClass="aspect-[2/3] mb-1.5"
										imageClass="group-hover:scale-105 transition-transform"
										placeholderSize="md"
									/>
									{#if book.status === 'reading'}
										<div class="absolute top-1 right-1 z-10 w-3 h-3 bg-blue-500 rounded-full"></div>
									{/if}
									{#if book.opened && book.percent > 0}
										<div class="absolute bottom-1.5 left-0 right-0 z-10 h-1 bg-slate-700">
											<div class="h-full bg-[var(--color-primary-500)] transition-all duration-300" style="width: {book.percent}%"></div>
										</div>
									{/if}
									{#if formatOnCover && book.format}
										{@const formatColor = getFormatColor(book.format)}
										<div
											class="absolute bottom-2 left-2 z-10 px-1.5 py-0.5 rounded text-[10px] font-medium uppercase border border-black/20 shadow-[0_1px_2px_rgba(0,0,0,0.35)]"
											style="background-color: {formatColor.bg}; color: {formatColor.text};"
										>
											{book.format}
										</div>
									{/if}
								</div>
								<div class="shrink-0">
									<h3 class="text-xs font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
									{#if book.authors && book.authors !== '[]'}
										{@const authorStr = (() => { try { const a = JSON.parse(book.authors); return Array.isArray(a) ? a.join(', ') : book.authors; } catch { return book.authors; } })()}
										<p class="text-[10px] text-[var(--color-surface-text-muted)] truncate">{authorStr}</p>
									{/if}
								</div>
							</a>
							<button 
								onclick={() => window.location.href = readerUrl}
								class="absolute bottom-12 left-1/2 -translate-x-1/2 opacity-0 group-hover:opacity-100 transition-opacity z-20 px-2 py-1 bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white text-[10px] rounded whitespace-nowrap"
							>
								Resume Reading
							</button>
						</div>
					{/each}
				</div>
			{:else}
				<div class="text-center py-12">
					<svg class="w-16 h-16 text-blue-400 mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
					</svg>
					<h3 class="text-lg font-medium text-[var(--color-surface-text)] mb-2">No books currently reading</h3>
					<p class="text-[var(--color-surface-text-muted)]">Books you're reading will appear here</p>
				</div>
			{/if}
		</div>
	{/if}

	<!-- Recently Added Section -->
	{#if dashboardConfig.showRecentlyAdded}
		<div class="bg-[var(--color-surface-overlay)] rounded-lg p-4 border border-[var(--color-surface-border)] flex-none sm:flex-1 min-h-0 flex flex-col">
			<div class="flex items-center justify-between mb-3">
				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Recently Added</h2>
				<a href="/library" class="text-sm text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">View all →</a>
			</div>
			{#if recentBooks.length > 0}
				<div
					bind:this={recentBooksRowEl}
					class="dashboard-book-row"
					style="--dashboard-book-card-width: {dashboardBookCardWidth}px; --dashboard-book-row-gap: {dashboardBookRowGap}px;"
				>
					{#each recentBooks.slice(0, visibleRecentBooksCount || 1) as book (book.id)}
						<a href="/book/{book.id}" class="dashboard-book-item group flex min-w-0 flex-col self-start" animate:flip={{ duration: 90, easing: cubicOut }}>
							<BookCoverFrame
								src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
								alt={book.title}
								mode="contain"
								frameClass="aspect-[2/3] mb-1.5"
								imageClass="group-hover:scale-105 transition-transform"
								placeholderSize="md"
							/>
							<div class="shrink-0">
								<h3 class="text-xs font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
								{#if book.authors && book.authors !== '[]'}
									{@const authorStr = (() => { try { const a = JSON.parse(book.authors); return Array.isArray(a) ? a.join(', ') : book.authors; } catch { return book.authors; } })()}
									<p class="text-[10px] text-[var(--color-surface-text-muted)] truncate">{authorStr}</p>
								{/if}
							</div>
						</a>
					{/each}
				</div>
			{:else}
 				<div class="text-center py-12">
 					<svg class="w-16 h-16 text-[var(--color-primary-400)] mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
 						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
 					</svg>
 					<h3 class="text-lg font-medium text-[var(--color-surface-text)] mb-2">No books yet</h3>
 					<p class="text-[var(--color-surface-text-muted)]">Add books to your library to get started</p>
 				</div>
 			{/if}
 		</div>
 	{/if}

 	<!-- Discover Section -->
	{#if dashboardConfig.showDiscover}
		<div class="bg-[var(--color-surface-overlay)] rounded-lg p-4 border border-[var(--color-surface-border)] flex-none sm:flex-1 min-h-0 flex flex-col">
			<div class="flex items-center justify-between mb-3">
				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Discover</h2>
				<button
					onclick={toggleConfigModal}
					class="p-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] transition-colors"
					title="Customize Dashboard"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
					</svg>
				</button>
			</div>
		{#if discoverBooks.length > 0}
				<div
					bind:this={discoverBooksRowEl}
					class="dashboard-book-row"
					style="--dashboard-book-card-width: {dashboardBookCardWidth}px; --dashboard-book-row-gap: {dashboardBookRowGap}px;"
				>
					{#each discoverBooks.slice(0, visibleDiscoverBooksCount || 1) as book (book.id)}
						<a href="/book/{book.id}" class="dashboard-book-item group flex min-w-0 flex-col self-start" animate:flip={{ duration: 90, easing: cubicOut }}>
							<BookCoverFrame
								src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
								alt={book.title}
								mode="contain"
								frameClass="aspect-[2/3] mb-1.5"
								imageClass="group-hover:scale-105 transition-transform"
								placeholderSize="md"
							/>
							<div class="shrink-0">
								<h3 class="text-xs font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
								{#if book.authors && book.authors !== '[]'}
									{@const authorStr = (() => { try { const a = JSON.parse(book.authors); return Array.isArray(a) ? a.join(', ') : book.authors; } catch { return book.authors; } })()}
									<p class="text-[10px] text-[var(--color-surface-text-muted)] truncate">{authorStr}</p>
								{/if}
							</div>
						</a>
					{/each}
				</div>
			{:else}
 				<div class="text-center py-12">
 					<svg class="w-16 h-16 text-[var(--color-primary-400)] mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
 						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
 					</svg>
 					<h3 class="text-lg font-medium text-[var(--color-surface-text)] mb-2">Discover new books</h3>
					<p class="text-[var(--color-surface-text-muted)]">Random books from your library will appear here</p>
				</div>
			{/if}
		</div>
	{/if}

	<!-- Dashboard Configuration Modal -->
	{#if showConfigModal}
		<div class="fixed inset-0 z-50 flex items-center justify-center p-4" role="dialog" aria-modal="true" tabindex="0" onkeydown={(e) => { if (e.key === 'Escape') toggleConfigModal(); }}>
			<button type="button" class="absolute inset-0 bg-black/80" aria-label="Close dashboard settings" onclick={toggleConfigModal}></button>
			<div class="relative z-10 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] w-full max-w-md max-h-[90vh] overflow-hidden shadow-2xl">
				<div class="px-6 py-4 border-b border-[var(--color-surface-border)]">
					<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Customize Dashboard</h3>
				</div>
				<div class="p-6 space-y-6 overflow-y-auto max-h-[calc(90vh-120px)]">
					<div class="space-y-4">
						<div class="flex items-center justify-between">
							<label class="text-sm font-medium text-[var(--color-surface-text)]" for="dashboard-show-continue">Show Continue Reading</label>
							<input id="dashboard-show-continue" type="checkbox" bind:checked={dashboardConfig.showContinueReading} class="rounded">
						</div>
						<div class="flex items-center justify-between">
							<label class="text-sm font-medium text-[var(--color-surface-text)]" for="dashboard-show-recent">Show Recently Added</label>
							<input id="dashboard-show-recent" type="checkbox" bind:checked={dashboardConfig.showRecentlyAdded} class="rounded">
						</div>
						<div class="flex items-center justify-between">
							<label class="text-sm font-medium text-[var(--color-surface-text)]" for="dashboard-show-discover">Show Discover</label>
							<input id="dashboard-show-discover" type="checkbox" bind:checked={dashboardConfig.showDiscover} class="rounded">
						</div>
					</div>

					{#if dashboardConfig.showContinueReading}
						<div>
							<label class="block text-sm font-medium text-[var(--color-surface-text)] mb-2" for="dashboard-continue-limit">Continue Reading Limit</label>
							<input
								id="dashboard-continue-limit"
								type="range"
								min="3"
								max="12"
								bind:value={dashboardConfig.continueReadingLimit}
								class="w-full h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer"
							>
							<div class="text-xs text-[var(--color-surface-text-muted)] mt-1">{dashboardConfig.continueReadingLimit} books</div>
						</div>
					{/if}

					{#if dashboardConfig.showRecentlyAdded}
						<div>
							<label class="block text-sm font-medium text-[var(--color-surface-text)] mb-2" for="dashboard-recent-limit">Recently Added Limit</label>
							<input
								id="dashboard-recent-limit"
								type="range"
								min="3"
								max="12"
								bind:value={dashboardConfig.recentlyAddedLimit}
								class="w-full h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer"
							>
							<div class="text-xs text-[var(--color-surface-text-muted)] mt-1">{dashboardConfig.recentlyAddedLimit} books</div>
						</div>
					{/if}

					{#if dashboardConfig.showDiscover}
						<div>
							<label class="block text-sm font-medium text-[var(--color-surface-text)] mb-2" for="dashboard-discover-limit">Discover Limit</label>
							<input
								id="dashboard-discover-limit"
								type="range"
								min="3"
								max="12"
								bind:value={dashboardConfig.discoverLimit}
								class="w-full h-2 bg-[var(--color-surface-700)] rounded-lg appearance-none cursor-pointer"
							>
							<div class="text-xs text-[var(--color-surface-text-muted)] mt-1">{dashboardConfig.discoverLimit} books</div>
						</div>
					{/if}
				</div>
				<div class="px-6 py-4 border-t border-[var(--color-surface-border)] flex justify-end">
					<button
						onclick={toggleConfigModal}
						class="px-4 py-2 bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white rounded-lg transition-colors"
					>
						Done
					</button>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.dashboard-book-row {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--dashboard-book-row-gap, 16px);
		width: 100%;
		min-width: 0;
		overflow: hidden;
	}

	.dashboard-book-item {
		flex: 0 0 var(--dashboard-book-card-width, 148px);
		width: var(--dashboard-book-card-width, 148px);
		min-width: 0;
		transition: transform 180ms ease, opacity 180ms ease;
	}

	.dashboard-book-item a {
		width: 100%;
	}
</style>

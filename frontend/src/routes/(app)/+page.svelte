<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { flip } from 'svelte/animate';
	import { cubicOut } from 'svelte/easing';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import BookCoverProgressChip from '$lib/components/BookCoverProgressChip.svelte';
	import { showFormatOnCover, showProgressChipOnCover, getFormatColor } from '$lib/stores';
	import { bookHasCoverProgress } from '$lib/utils/cover-progress';
	import { getBookReaderHref } from '$lib/utils/book-formats';

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
	let continueReadingLoading = $state(true);
	let recentBooksLoading = $state(true);
	let discoverBooksLoading = $state(true);
 	let formatOnCover = $state(true);
 	let progressChipOnCover = $state(true);
 	let continueReadingRowEl = $state<HTMLDivElement | null>(null);
 	let recentBooksRowEl = $state<HTMLDivElement | null>(null);
 	let discoverBooksRowEl = $state<HTMLDivElement | null>(null);
 	let dashboardBookCardWidth = $state(148);
 	let dashboardBookRowGap = $state(16);
 	const DASHBOARD_BOOK_MIN_WIDTH = 96;
 	const DASHBOARD_BOOK_MAX_WIDTH = 148;
 	const DASHBOARD_BOOK_TEXT_HEIGHT = 34;
 	let visibleContinueReadingCount = $state(0);
 	let visibleRecentBooksCount = $state(0);
 	let visibleDiscoverBooksCount = $state(0);

 	// Dashboard configuration
 	let dashboardConfig = $state({
 		showContinueReading: true,
 		showRecentlyAdded: true,
 		showDiscover: true,
 		continueReadingLimit: 12,
 		recentlyAddedLimit: 12,
 		discoverLimit: 12
 	});

  	let showConfigModal = $state(false);

 	$effect(() => {
 		const unsub = showFormatOnCover.subscribe((v: boolean) => formatOnCover = v);
 		return unsub;
	});

	$effect(() => {
		const unsub = showProgressChipOnCover.subscribe((v: boolean) => progressChipOnCover = v);
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

	function handleDashboardResponse(res: Response) {
		if (res.status === 401) {
			window.location.href = '/login';
			return false;
		}
		return res.ok;
	}

	async function loadDashboardSummary() {
		try {
			const res = await fetch('/api/dashboard/summary', { cache: 'no-store' });
			if (handleDashboardResponse(res)) {
				const s = await res.json();
				stats.books = s.total_books;
				stats.libraries = s.libraries;
				stats.reading = s.reading;
				stats.finished = s.finished;
			}
		} catch (e) {
			console.error('Failed to fetch dashboard summary:', e);
		}
	}

	async function loadContinueReading() {
		continueReadingLoading = true;
		try {
			const res = await fetch('/api/books?status=reading&standard_progress=true&sort=last_read&sort_dir=desc&limit=12&include_total=false', { cache: 'no-store' });
			if (handleDashboardResponse(res)) {
				const data = await res.json();
				continueReadingBooks = data.books || [];
			}
		} catch (e) {
			console.error('Failed to fetch continue reading:', e);
		} finally {
			continueReadingLoading = false;
		}
	}

	async function loadRecentBooks() {
		recentBooksLoading = true;
		try {
			const res = await fetch('/api/books?limit=12&include_total=false', { cache: 'no-store' });
			if (handleDashboardResponse(res)) {
				const data = await res.json();
				recentBooks = (data.books || []).slice(0, 12);
				if (!stats.books) stats.books = data.total || 0;
			}
		} catch (e) {
			console.error('Failed to fetch recently added books:', e);
		} finally {
			recentBooksLoading = false;
		}
	}

	async function loadDiscoverBooks() {
		discoverBooksLoading = true;
		try {
			const res = await fetch('/api/books/discover?limit=12', { cache: 'no-store' });
			if (handleDashboardResponse(res)) {
				const data = await res.json();
				discoverBooks = data.books || [];
			}
		} catch (e) {
			console.error('Failed to fetch discover books:', e);
		} finally {
			discoverBooksLoading = false;
		}
	}

 	onMount(async () => {
 		showFormatOnCover.init();
		showProgressChipOnCover.init();
		await Promise.allSettled([
			loadDashboardSummary(),
			loadContinueReading(),
			loadRecentBooks(),
			loadDiscoverBooks()
		]);
		updateDashboardRows();
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
			const measuredWidth = measureBookCardWidth();
			if (measuredWidth > 0) {
				dashboardBookCardWidth = measuredWidth;
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

	function measureBookCardWidth(): number {
		const rows = [continueReadingRowEl, recentBooksRowEl, discoverBooksRowEl].filter(Boolean) as HTMLDivElement[];
		const rowHeight = rows.reduce((minHeight, row) => {
			return Math.min(minHeight, row.clientHeight || minHeight);
		}, Number.POSITIVE_INFINITY);

		if (!Number.isFinite(rowHeight)) {
			return dashboardBookCardWidth;
		}

			const availableForCover = Math.max(0, rowHeight - DASHBOARD_BOOK_TEXT_HEIGHT);
		const heightBoundWidth = Math.floor(availableForCover / 1.5);
		return Math.max(
			DASHBOARD_BOOK_MIN_WIDTH,
			Math.min(DASHBOARD_BOOK_MAX_WIDTH, heightBoundWidth)
		);
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

	function getReaderUrl(book: any): string {
		return getBookReaderHref(book.id, book.format, '/', book.resume_file_id);
	}

	function handleDashboardRowWheel(event: WheelEvent) {
		const row = event.currentTarget as HTMLDivElement;
		if (!row || Math.abs(event.deltaX) > Math.abs(event.deltaY)) return;

		const maxScrollLeft = row.scrollWidth - row.clientWidth;
		if (maxScrollLeft <= 0) return;

		const atStart = row.scrollLeft <= 0;
		const atEnd = row.scrollLeft >= maxScrollLeft - 1;
		const scrollingRight = event.deltaY > 0;
		const scrollingLeft = event.deltaY < 0;

		if ((scrollingRight && atEnd) || (scrollingLeft && atStart)) return;

		event.preventDefault();
		row.scrollLeft = Math.max(0, Math.min(maxScrollLeft, row.scrollLeft + event.deltaY));
	}
</script>

<div class="flex min-h-full flex-col gap-3 overflow-x-hidden overflow-y-auto">
	<div class="hidden sm:grid grid-cols-2 lg:grid-cols-4 gap-2.5">
		<a href="/library" class="block rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-3 transition-colors hover:border-[var(--color-primary-500)]/50 hover:bg-[var(--color-surface-overlay)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]" aria-label="View all books">
			<div class="flex items-center justify-between">
				<div>
					<p class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">Total Books</p>
					<p class="text-2xl font-bold text-[var(--color-surface-text)] mt-1">{stats.books}</p>
				</div>
				<div class="w-8 h-8 rounded-md bg-[var(--color-primary-500)]/20 flex items-center justify-center flex-shrink-0">
					<svg class="w-4 h-4 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
					</svg>
				</div>
			</div>
		</a>

		<a href="/library?status=reading" class="block rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-3 transition-colors hover:border-[var(--color-primary-500)]/50 hover:bg-[var(--color-surface-overlay)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]" aria-label="View currently reading books">
			<div class="flex items-center justify-between">
				<div>
					<p class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">Reading</p>
					<p class="text-2xl font-bold text-[var(--color-surface-text)] mt-1">{stats.reading}</p>
				</div>
				<div class="w-8 h-8 rounded-md bg-[var(--color-primary-500)]/20 flex items-center justify-center flex-shrink-0">
					<svg class="w-4 h-4 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
					</svg>
				</div>
			</div>
		</a>

		<a href="/library?status=finished" class="block rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-3 transition-colors hover:border-[var(--color-primary-500)]/50 hover:bg-[var(--color-surface-overlay)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]" aria-label="View already read books">
			<div class="flex items-center justify-between">
				<div>
					<p class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">Finished</p>
					<p class="text-2xl font-bold text-[var(--color-surface-text)] mt-1">{stats.finished}</p>
				</div>
				<div class="w-8 h-8 rounded-md bg-[var(--color-primary-500)]/20 flex items-center justify-center flex-shrink-0">
					<svg class="w-4 h-4 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path>
					</svg>
				</div>
			</div>
		</a>

		<a href="/libraries" class="block rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-3 transition-colors hover:border-[var(--color-primary-500)]/50 hover:bg-[var(--color-surface-overlay)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]" aria-label="View libraries">
			<div class="flex items-center justify-between">
				<div>
					<p class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">Libraries</p>
					<p class="text-2xl font-bold text-[var(--color-surface-text)] mt-1">{stats.libraries}</p>
				</div>
				<div class="w-8 h-8 rounded-md bg-[var(--color-primary-500)]/20 flex items-center justify-center flex-shrink-0">
					<svg class="w-4 h-4 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
					</svg>
				</div>
			</div>
		</a>
	</div>

	<!-- Continue Reading Section -->
	{#if dashboardConfig.showContinueReading}
	<div class="bg-[var(--color-surface-overlay)] rounded-lg p-3 border border-[var(--color-surface-border)] flex-none flex flex-col">
		<div class="flex items-center justify-between mb-3">
			<h2 class="text-base font-semibold text-[var(--color-surface-text)]">Continue Reading</h2>
			<a href="/library?status=reading" class="text-sm text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">View all →</a>
		</div>
			{#if continueReadingBooks.length > 0}
				<div
					bind:this={continueReadingRowEl}
					class="dashboard-book-row"
					style="--dashboard-book-card-width: {dashboardBookCardWidth}px; --dashboard-book-row-gap: {dashboardBookRowGap}px;"
					onwheel={handleDashboardRowWheel}
				>
					{#each continueReadingBooks.slice(0, dashboardConfig.continueReadingLimit) as book (book.id)}
						{@const readerUrl = getReaderUrl(book)}
						<div class="dashboard-book-item relative group min-w-0 self-start" animate:flip={{ duration: 90, easing: cubicOut }}>
							<div class="flex min-w-0 flex-col">
								<div class="relative mb-1.5 overflow-hidden rounded-lg">
									<a href="/book/{book.id}" class="block" aria-label="View details for {book.title || 'book'}">
										<BookCoverFrame
											src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
											alt={book.title}
											format={book.format}
											mode="cover"
											frameClass="aspect-[2/3]"
											imageClass="group-hover:scale-105 transition-transform"
											placeholderSize="md"
											retryCount={2}
										/>
										{#if book.status === 'reading'}
											<div class="absolute bottom-2 right-2 z-10 h-3 w-3 rounded-full border border-black/30 bg-blue-500 shadow-[0_1px_2px_rgba(0,0,0,0.35)]"></div>
										{/if}
										{#if bookHasCoverProgress(book)}
											<div class="absolute bottom-0 left-0 right-0 z-10 h-1 bg-slate-700">
												<div class="h-full bg-[var(--color-primary-500)] transition-all duration-300" style="width: {book.percent}%"></div>
											</div>
										{/if}
										{#if formatOnCover && book.format}
											{@const formatColor = getFormatColor(book.format)}
											<div
												class="absolute right-2 top-2 z-10 px-1.5 py-0.5 rounded text-[10px] font-medium uppercase border border-black/20 shadow-[0_1px_2px_rgba(0,0,0,0.35)]"
												style="background-color: {formatColor.bg}; color: {formatColor.text};"
											>
												{book.format}
											</div>
										{/if}
									</a>
									{#if progressChipOnCover && bookHasCoverProgress(book)}
										<BookCoverProgressChip
											href={readerUrl}
											percent={book.percent}
											title={book.title}
											format={book.format}
										/>
									{/if}
								</div>
								<a href="/book/{book.id}" class="shrink-0 block">
									<h3 class="text-xs font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
									{#if book.authors && book.authors !== '[]'}
										{@const authorStr = (() => { try { const a = JSON.parse(book.authors); return Array.isArray(a) ? a.join(', ') : book.authors; } catch { return book.authors; } })()}
										<p class="text-[10px] text-[var(--color-surface-text-muted)] truncate">{authorStr}</p>
									{/if}
								</a>
							</div>
						</div>
					{/each}
				</div>
			{:else if continueReadingLoading}
				<div class="py-12 text-center text-sm text-[var(--color-surface-text-muted)]">Loading continue reading...</div>
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
		<div class="bg-[var(--color-surface-overlay)] rounded-lg p-3 border border-[var(--color-surface-border)] flex-none flex flex-col">
			<div class="flex items-center justify-between mb-3">
				<h2 class="text-base font-semibold text-[var(--color-surface-text)]">Recently Added</h2>
				<a href="/library" class="text-sm text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">View all →</a>
			</div>
			{#if recentBooks.length > 0}
				<div
					bind:this={recentBooksRowEl}
					class="dashboard-book-row"
					style="--dashboard-book-card-width: {dashboardBookCardWidth}px; --dashboard-book-row-gap: {dashboardBookRowGap}px;"
					onwheel={handleDashboardRowWheel}
				>
					{#each recentBooks.slice(0, dashboardConfig.recentlyAddedLimit) as book (book.id)}
						{@const readerUrl = getReaderUrl(book)}
						<div class="dashboard-book-item group flex min-w-0 flex-col self-start" animate:flip={{ duration: 90, easing: cubicOut }}>
							<div class="relative mb-1.5 overflow-hidden rounded-lg">
								<a href="/book/{book.id}" class="block" aria-label="View details for {book.title || 'book'}">
									<BookCoverFrame
										src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
										alt={book.title}
										format={book.format}
										mode="cover"
										frameClass="aspect-[2/3]"
										imageClass="group-hover:scale-105 transition-transform"
										placeholderSize="md"
										retryCount={2}
									/>
									{#if bookHasCoverProgress(book)}
										<div class="absolute bottom-0 left-0 right-0 z-10 h-1 bg-slate-700">
											<div class="h-full bg-[var(--color-primary-500)] transition-all duration-300" style="width: {book.percent}%"></div>
										</div>
									{/if}
									{#if formatOnCover && book.format}
										{@const formatColor = getFormatColor(book.format)}
										<div
											class="absolute right-2 top-2 z-10 px-1.5 py-0.5 rounded text-[10px] font-medium uppercase border border-black/20 shadow-[0_1px_2px_rgba(0,0,0,0.35)]"
											style="background-color: {formatColor.bg}; color: {formatColor.text};"
										>
											{book.format}
										</div>
									{/if}
								</a>
								{#if progressChipOnCover && bookHasCoverProgress(book)}
									<BookCoverProgressChip
										href={readerUrl}
										percent={book.percent}
										title={book.title}
										format={book.format}
									/>
								{/if}
							</div>
							<a href="/book/{book.id}" class="shrink-0 block">
								<h3 class="text-xs font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
								{#if book.authors && book.authors !== '[]'}
									{@const authorStr = (() => { try { const a = JSON.parse(book.authors); return Array.isArray(a) ? a.join(', ') : book.authors; } catch { return book.authors; } })()}
									<p class="text-[10px] text-[var(--color-surface-text-muted)] truncate">{authorStr}</p>
								{/if}
							</a>
						</div>
					{/each}
				</div>
			{:else if recentBooksLoading}
				<div class="py-12 text-center text-sm text-[var(--color-surface-text-muted)]">Loading recently added...</div>
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
		<div class="bg-[var(--color-surface-overlay)] rounded-lg p-3 border border-[var(--color-surface-border)] flex-none flex flex-col">
			<div class="flex items-center justify-between mb-3">
				<h2 class="text-base font-semibold text-[var(--color-surface-text)]">Discover</h2>
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
					onwheel={handleDashboardRowWheel}
				>
					{#each discoverBooks.slice(0, dashboardConfig.discoverLimit) as book (book.id)}
						{@const readerUrl = getReaderUrl(book)}
						<div class="dashboard-book-item group flex min-w-0 flex-col self-start" animate:flip={{ duration: 90, easing: cubicOut }}>
							<div class="relative mb-1.5 overflow-hidden rounded-lg">
								<a href="/book/{book.id}" class="block" aria-label="View details for {book.title || 'book'}">
									<BookCoverFrame
										src={book.cover_path ? `/api/covers/${book.id}/thumb` : null}
										alt={book.title}
										format={book.format}
										mode="cover"
										frameClass="aspect-[2/3]"
										imageClass="group-hover:scale-105 transition-transform"
										placeholderSize="md"
										retryCount={2}
									/>
									{#if bookHasCoverProgress(book)}
										<div class="absolute bottom-0 left-0 right-0 z-10 h-1 bg-slate-700">
											<div class="h-full bg-[var(--color-primary-500)] transition-all duration-300" style="width: {book.percent}%"></div>
										</div>
									{/if}
									{#if formatOnCover && book.format}
										{@const formatColor = getFormatColor(book.format)}
										<div
											class="absolute right-2 top-2 z-10 px-1.5 py-0.5 rounded text-[10px] font-medium uppercase border border-black/20 shadow-[0_1px_2px_rgba(0,0,0,0.35)]"
											style="background-color: {formatColor.bg}; color: {formatColor.text};"
										>
											{book.format}
										</div>
									{/if}
								</a>
								{#if progressChipOnCover && bookHasCoverProgress(book)}
									<BookCoverProgressChip
										href={readerUrl}
										percent={book.percent}
										title={book.title}
										format={book.format}
									/>
								{/if}
							</div>
							<a href="/book/{book.id}" class="shrink-0 block">
								<h3 class="text-xs font-medium text-[var(--color-surface-text)] truncate">{book.title || 'Untitled'}</h3>
								{#if book.authors && book.authors !== '[]'}
									{@const authorStr = (() => { try { const a = JSON.parse(book.authors); return Array.isArray(a) ? a.join(', ') : book.authors; } catch { return book.authors; } })()}
									<p class="text-[10px] text-[var(--color-surface-text-muted)] truncate">{authorStr}</p>
								{/if}
							</a>
						</div>
					{/each}
				</div>
			{:else if discoverBooksLoading}
				<div class="py-12 text-center text-sm text-[var(--color-surface-text-muted)]">Loading discover...</div>
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
						<div class="flex items-center justify-between gap-3">
							<div class="min-w-0">
								<label class="text-sm font-medium text-[var(--color-surface-text)]" for="dashboard-show-progress-chip">Show Progress Chip on Cover</label>
								<p class="mt-0.5 text-xs text-[var(--color-surface-text-muted)]">When a book has progress, a tappable % chip opens the reader.</p>
							</div>
							<input
								id="dashboard-show-progress-chip"
								type="checkbox"
								checked={progressChipOnCover}
								onchange={(event) => showProgressChipOnCover.set(event.currentTarget.checked)}
								class="rounded"
							>
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
						class="accent-action rounded-lg px-4 py-2 font-medium transition-colors"
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
		justify-content: flex-start;
		gap: var(--dashboard-book-row-gap, 16px);
		width: 100%;
		min-width: 0;
		overflow-x: auto;
		overflow-y: hidden;
		padding-bottom: 0.35rem;
		scrollbar-width: thin;
		scrollbar-color: var(--color-surface-border) transparent;
		-webkit-overflow-scrolling: touch;
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

<script lang="ts">
	import { onMount } from 'svelte';

	type MetadataCandidate = {
		provider: string;
		title: string;
		authors?: string[];
		series?: string;
		publisher?: string;
		pub_date?: string;
		description?: string;
		isbn?: string;
		asin?: string;
		cover_url?: string;
		page_count?: number;
		language?: string;
		rating?: number;
		genres?: string[];
		match_score?: number;
	};

	type BookSummary = {
		id: number;
		title?: string;
		authors?: string;
		series?: string;
		publisher?: string;
		description?: string;
		isbn?: string;
		asin?: string;
		cover_path?: string;
	};

	type LookupTarget = {
		bookId: number;
		title: string;
		authors: string[];
		isbn: string;
		asin: string;
		series: string;
		publisher: string;
		description: string;
		queryTitle: string;
		queryAuthors: string;
		queryIsbn: string;
		queryAsin: string;
		querySeries: string;
		queryPublisher: string;
		results: MetadataCandidate[];
		selectedIndex: number;
		loading: boolean;
		error: string | null;
	};

	interface Props {
		bookIds: number[];
		title?: string;
		onClose: () => void;
		onApplied?: () => Promise<void> | void;
	}

	let { bookIds = [], title = 'Metadata Lookup', onClose, onApplied }: Props = $props();

	let targets = $state<LookupTarget[]>([]);
	let activeBookId = $state<number | null>(null);
	let providers = $state<{ id: string; name: string }[]>([]);
	let selectedProvider = $state('');
	let includeCover = $state(true);
	let loading = $state(true);
	let applying = $state(false);
	let initialized = false;

	function parseAuthors(value: string | undefined): string[] {
		if (!value) return [];
		try {
			const parsed = JSON.parse(value);
			return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === 'string' && item.trim().length > 0) : [value];
		} catch {
			return value.split(',').map((item) => item.trim()).filter(Boolean);
		}
	}

	function summarizeAuthors(authors: string[]): string {
		return authors.filter(Boolean).join(', ');
	}

	function normalizeSearchText(value: string): string {
		return value.trim().toLowerCase().replace(/\s+/g, ' ');
	}

	function formatSearchError(status: number, detail?: string): string {
		const code = `E${status}`;
		const normalizedDetail = (detail || '').trim();
		const fallback = (() => {
			switch (status) {
				case 400:
					return 'bad request';
				case 401:
					return 'authentication required';
				case 403:
					return 'permission denied';
				case 404:
					return 'not found';
				case 429:
					return 'rate limited';
				case 500:
					return 'server error';
				case 503:
					return 'service unavailable';
				default:
					return 'request failed';
			}
		})();

		return normalizedDetail ? `Search failed (${code}): ${normalizedDetail}` : `Search failed (${code}): ${fallback}.`;
	}

	async function readErrorDetail(res: Response): Promise<string | undefined> {
		try {
			const text = await res.text();
			if (!text.trim()) return undefined;
			try {
				const parsed = JSON.parse(text);
				if (parsed && typeof parsed === 'object' && 'error' in parsed && typeof parsed.error === 'string') {
					return parsed.error;
				}
			} catch {
				// Fall through to raw text.
			}
			return text.trim();
		} catch {
			return undefined;
		}
	}

	function normalizeTarget(summary: BookSummary): LookupTarget {
		const authors = parseAuthors(summary.authors);
		const titleValue = summary.title?.trim() || '';
		return {
			bookId: summary.id,
			title: titleValue,
			authors,
			isbn: summary.isbn?.trim() || '',
			asin: summary.asin?.trim() || '',
			series: summary.series?.trim() || '',
			publisher: summary.publisher?.trim() || '',
			description: summary.description?.trim() || '',
			queryTitle: titleValue,
			queryAuthors: summarizeAuthors(authors),
			queryIsbn: summary.isbn?.trim() || '',
			queryAsin: summary.asin?.trim() || '',
			querySeries: summary.series?.trim() || '',
			queryPublisher: summary.publisher?.trim() || '',
			results: [],
			selectedIndex: -1,
			loading: false,
			error: null
		};
	}

	function activeTarget(): LookupTarget | null {
		return targets.find((target) => target.bookId === activeBookId) ?? null;
	}

	function activeTargetIndex(): number {
		return targets.findIndex((target) => target.bookId === activeBookId);
	}

	function goToTarget(offset: number) {
		if (targets.length === 0) return;
		const currentIndex = activeTargetIndex();
		const nextIndex = Math.min(Math.max(currentIndex + offset, 0), targets.length - 1);
		activeBookId = targets[nextIndex]?.bookId ?? activeBookId;
	}

	function updateTarget(bookId: number, updater: (target: LookupTarget) => LookupTarget) {
		targets = targets.map((target) => target.bookId === bookId ? updater({ ...target }) : target);
	}

	function queryFromTarget(target: LookupTarget): string {
		return [
			target.queryTitle,
			target.queryAuthors,
			target.queryIsbn,
			target.queryAsin,
			target.querySeries,
			target.queryPublisher
		]
			.map((value) => value.trim())
			.filter(Boolean)
			.join(' ');
	}

	function resultSummaryForTarget(target: LookupTarget): string {
		const title = target.queryTitle.trim();
		const authors = target.queryAuthors.trim();
		if (title && authors) return `${title} by ${authors}`;
		if (title) return title;
		if (authors) return authors;
		return `Book ${target.bookId}`;
	}

	function hasEditedSearchFields(target: LookupTarget): boolean {
		return (
			normalizeSearchText(target.queryTitle) !== normalizeSearchText(target.title) ||
			normalizeSearchText(target.queryAuthors) !== normalizeSearchText(summarizeAuthors(target.authors)) ||
			normalizeSearchText(target.queryIsbn) !== normalizeSearchText(target.isbn) ||
			normalizeSearchText(target.queryAsin) !== normalizeSearchText(target.asin || '') ||
			normalizeSearchText(target.querySeries) !== normalizeSearchText(target.series) ||
			normalizeSearchText(target.queryPublisher) !== normalizeSearchText(target.publisher)
		);
	}

	async function fetchProviders() {
		try {
			const res = await fetch('/api/providers');
			if (res.ok) {
				providers = await res.json();
			}
		} catch (error) {
			console.error('Failed to fetch providers:', error);
		}
	}

	async function fetchBookSummary(bookId: number): Promise<BookSummary | null> {
		try {
			const res = await fetch(`/api/books/${bookId}`);
			if (!res.ok) return null;
			return await res.json();
		} catch (error) {
			console.error('Failed to fetch book summary:', error);
			return null;
		}
	}

	async function initialize() {
		if (initialized) return;
		initialized = true;
		loading = true;
		try {
			await fetchProviders();
			const summaries = await Promise.all(bookIds.map((bookId) => fetchBookSummary(bookId)));
			targets = summaries.filter((summary): summary is BookSummary => !!summary).map((summary) => normalizeTarget(summary));
			activeBookId = targets[0]?.bookId ?? null;
		} finally {
			loading = false;
		}
	}

	async function searchTarget(bookId: number) {
		const target = targets.find((item) => item.bookId === bookId);
		if (!target) return;

		const query = queryFromTarget(target);
		if (!query) {
			updateTarget(bookId, (item) => ({ ...item, error: 'Add a title, author, ISBN, or ASIN before searching.' }));
			return;
		}

		updateTarget(bookId, (item) => ({ ...item, loading: true, error: null }));

		try {
			const params = new URLSearchParams();
			if (target.queryTitle.trim()) params.set('title', target.queryTitle.trim());
			if (target.queryAuthors.trim()) params.set('author', target.queryAuthors.trim());
			if (target.queryIsbn.trim()) params.set('isbn', target.queryIsbn.trim());
			if (target.queryAsin.trim()) params.set('asin', target.queryAsin.trim());
			if (target.querySeries.trim()) params.set('series', target.querySeries.trim());
			if (target.queryPublisher.trim()) params.set('publisher', target.queryPublisher.trim());
			if (selectedProvider) params.set('provider', selectedProvider);
			if (hasEditedSearchFields(target)) params.set('strict', '1');
			params.set('limit', '6');

			const res = await fetch(`/api/metadata/search?${params.toString()}`);
			if (!res.ok) {
				const detail = await readErrorDetail(res);
				throw new Error(formatSearchError(res.status, detail));
			}

			const results = await res.json();
			const safeResults = Array.isArray(results) ? results : [];
			updateTarget(bookId, (item) => ({
				...item,
				results: safeResults,
				selectedIndex: safeResults.length > 0 ? 0 : -1,
				loading: false,
				error: null
			}));
		} catch (error) {
			console.error('Failed to search metadata:', error);
			updateTarget(bookId, (item) => ({
				...item,
				loading: false,
				error: error instanceof Error ? error.message : 'Search failed (E000): unable to reach the metadata service.'
			}));
		}
	}

	async function searchAllTargets() {
		await Promise.all(targets.map((target) => searchTarget(target.bookId)));
	}

	async function applyMetadata(bookId: number, notifyParent = true) {
		const target = targets.find((item) => item.bookId === bookId);
		if (!target || target.selectedIndex < 0 || target.selectedIndex >= target.results.length) return;

		const selected = target.results[target.selectedIndex];
		const metadata = {
			...selected,
			cover_url: includeCover ? selected.cover_url : ''
		};

		updateTarget(bookId, (item) => ({ ...item, loading: true, error: null }));
		try {
			const res = await fetch('/api/metadata/apply', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					book_id: bookId,
					metadata
				})
			});

			if (!res.ok) {
				const text = await res.text();
				throw new Error(text || `Apply failed (${res.status})`);
			}

			updateTarget(bookId, (item) => ({ ...item, loading: false }));
			if (notifyParent) {
				await onApplied?.();
			}
		} catch (error) {
			console.error('Failed to apply metadata:', error);
			updateTarget(bookId, (item) => ({
				...item,
				loading: false,
				error: 'Unable to apply metadata.'
			}));
		}
	}

	async function applyAllSelected() {
		applying = true;
		try {
			const selectedItems = targets
				.filter((target) => target.selectedIndex >= 0 && target.results[target.selectedIndex])
				.map((target) => ({
					book_id: target.bookId,
					metadata: {
						...target.results[target.selectedIndex],
						cover_url: includeCover ? target.results[target.selectedIndex].cover_url : ''
					}
				}));

			if (selectedItems.length === 0) {
				return;
			}

			if (selectedItems.length === 1) {
				await applyMetadata(selectedItems[0].book_id, false);
				await onApplied?.();
				return;
			}

			const res = await fetch('/api/jobs/metadata-apply', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					items: selectedItems,
					include_cover: includeCover
				})
			});

			if (!res.ok) {
				const text = await res.text();
				throw new Error(text || `Queue failed (${res.status})`);
			}

			await onApplied?.();
		} finally {
			applying = false;
		}
	}

	function getSelectedResult(target: LookupTarget): MetadataCandidate | null {
		return target.selectedIndex >= 0 ? target.results[target.selectedIndex] ?? null : null;
	}

	onMount(() => {
		void initialize();
	});
</script>

<div class="fixed inset-0 z-[120] flex items-center justify-center p-4 relative">
	<button
		type="button"
		aria-label="Close metadata lookup modal"
		class="absolute inset-0 z-0 bg-black/70"
		onclick={onClose}
	></button>
	<div class="relative z-10 w-full max-w-6xl max-h-[92vh] overflow-hidden rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
		<div class="flex items-center justify-between gap-4 border-b border-[var(--color-surface-border)] px-6 py-4">
			<div>
				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">{title}</h2>
				<p class="text-sm text-[var(--color-surface-text-muted)]">
					Review selected books one at a time, search providers, then apply the best match.
				</p>
			</div>
			<div class="flex items-center gap-2">
				<button
					class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)]"
					onclick={searchAllTargets}
					disabled={loading || applying}
				>
					Search All
				</button>
				<button
					class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)]"
					onclick={onClose}
				>
					Close
				</button>
			</div>
		</div>

		{#if loading}
			<div class="flex items-center justify-center px-6 py-12">
				<div class="h-12 w-12 animate-spin rounded-full border-b-2 border-[var(--color-primary-500)]"></div>
			</div>
		{:else if targets.length > 0}
			{@const target = activeTarget()}
			{@const targetIndex = activeTargetIndex()}
			<div class="flex h-[calc(92vh-88px)] flex-col gap-4 overflow-hidden p-4 lg:p-6">
				{#if targets.length > 1}
					<div class="flex items-center justify-between gap-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-4 py-3">
						<button
							type="button"
							class="rounded-md border border-[var(--color-surface-border)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-overlay)] disabled:opacity-40"
							onclick={() => goToTarget(-1)}
							disabled={targetIndex <= 0}
						>
							&lt;
						</button>
						<div class="min-w-0 text-center">
							<div class="text-sm font-medium text-[var(--color-surface-text)]">
								{targetIndex + 1} / {targets.length}
							</div>
							<div class="truncate text-xs text-[var(--color-surface-text-muted)]">
								{target?.title || (target ? `Book ${target.bookId}` : 'Selected book')}
							</div>
						</div>
						<button
							type="button"
							class="rounded-md border border-[var(--color-surface-border)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-overlay)] disabled:opacity-40"
							onclick={() => goToTarget(1)}
							disabled={targetIndex >= targets.length - 1}
						>
							&gt;
						</button>
					</div>
				{/if}
				{#if target}
					<div class="grid min-h-0 flex-1 gap-4 overflow-hidden lg:grid-cols-[360px_minmax(0,1fr)]">
						<div class="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
							<div class="border-b border-[var(--color-surface-border)] px-4 py-3">
								<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Search Fields</h3>
								<p class="text-sm text-[var(--color-surface-text-muted)]">Refine the query for this book.</p>
							</div>
								<div class="min-h-0 flex-1 overflow-y-auto px-4 py-4 space-y-4">
									<div>
										<label for={`lookup-title-${target.bookId}`} class="mb-1 block text-sm font-medium text-[var(--color-surface-text-muted)]">Title</label>
										<input
											id={`lookup-title-${target.bookId}`}
											value={target.queryTitle}
											oninput={(event) => updateTarget(target.bookId, (item) => ({ ...item, queryTitle: (event.currentTarget as HTMLInputElement).value }))}
											class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
										/>
									</div>
									<div>
										<label for={`lookup-authors-${target.bookId}`} class="mb-1 block text-sm font-medium text-[var(--color-surface-text-muted)]">Authors</label>
										<input
											id={`lookup-authors-${target.bookId}`}
											value={target.queryAuthors}
											oninput={(event) => updateTarget(target.bookId, (item) => ({ ...item, queryAuthors: (event.currentTarget as HTMLInputElement).value }))}
											class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
										/>
									</div>
									<div>
										<label for={`lookup-isbn-${target.bookId}`} class="mb-1 block text-sm font-medium text-[var(--color-surface-text-muted)]">ISBN</label>
										<input
											id={`lookup-isbn-${target.bookId}`}
											value={target.queryIsbn}
											oninput={(event) => updateTarget(target.bookId, (item) => ({ ...item, queryIsbn: (event.currentTarget as HTMLInputElement).value }))}
											class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
										/>
									</div>
									<div>
										<label for={`lookup-asin-${target.bookId}`} class="mb-1 block text-sm font-medium text-[var(--color-surface-text-muted)]">ASIN</label>
										<input
											id={`lookup-asin-${target.bookId}`}
											value={target.queryAsin}
											oninput={(event) => updateTarget(target.bookId, (item) => ({ ...item, queryAsin: (event.currentTarget as HTMLInputElement).value }))}
											class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
										/>
									</div>
									<div>
										<label for={`lookup-series-${target.bookId}`} class="mb-1 block text-sm font-medium text-[var(--color-surface-text-muted)]">Series</label>
										<input
											id={`lookup-series-${target.bookId}`}
											value={target.querySeries}
											oninput={(event) => updateTarget(target.bookId, (item) => ({ ...item, querySeries: (event.currentTarget as HTMLInputElement).value }))}
											class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
										/>
									</div>
									<div>
										<label for={`lookup-publisher-${target.bookId}`} class="mb-1 block text-sm font-medium text-[var(--color-surface-text-muted)]">Publisher</label>
										<input
											id={`lookup-publisher-${target.bookId}`}
											value={target.queryPublisher}
											oninput={(event) => updateTarget(target.bookId, (item) => ({ ...item, queryPublisher: (event.currentTarget as HTMLInputElement).value }))}
											class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
										/>
									</div>
									<div>
										<label for={`lookup-provider-${target.bookId}`} class="mb-1 block text-sm font-medium text-[var(--color-surface-text-muted)]">Provider</label>
										<select bind:value={selectedProvider} class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]">
										<option value="">All providers</option>
										{#each providers as provider}
											<option value={provider.id}>{provider.name}</option>
										{/each}
									</select>
								</div>
								<div class="flex items-center gap-3">
									<input id="include-cover" type="checkbox" bind:checked={includeCover} class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]" />
									<label for="include-cover" class="text-sm text-[var(--color-surface-text)]">Update cover when available</label>
								</div>
								<div class="grid grid-cols-2 gap-2 pt-2">
									<button
										class="rounded-lg bg-[var(--color-primary-500)] px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-[var(--color-primary-600)] disabled:opacity-50"
										onclick={() => searchTarget(target.bookId)}
										disabled={target.loading || applying}
									>
										{target.loading ? 'Searching...' : 'Search'}
									</button>
									<button
										class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-overlay)] disabled:opacity-50"
										onclick={() => applyMetadata(target.bookId)}
										disabled={target.loading || applying || target.selectedIndex < 0}
									>
										Apply Current
									</button>
								</div>
								{#if targets.length > 1}
									<button
										class="w-full rounded-lg border border-[var(--color-primary-500)]/40 bg-[var(--color-primary-500)]/10 px-3 py-2 text-sm font-medium text-[var(--color-primary-300)] transition-colors hover:bg-[var(--color-primary-500)]/20 disabled:opacity-50"
										onclick={applyAllSelected}
										disabled={applying}
									>
										Queue Bulk Update
									</button>
								{/if}
								{#if target.error}
									<div class="rounded-lg border border-red-500/40 bg-red-500/10 px-3 py-2 text-sm text-red-300">
										{target.error}
									</div>
								{/if}
							</div>
						</div>

						<div class="min-h-0 overflow-hidden rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
							<div class="border-b border-[var(--color-surface-border)] px-4 py-3">
								<div class="flex items-center justify-between gap-3">
									<div>
										<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Results</h3>
										<p class="text-sm text-[var(--color-surface-text-muted)]">
											{target.results.length} result{target.results.length === 1 ? '' : 's'} for {resultSummaryForTarget(target)}
										</p>
									</div>
									<div class="text-sm text-[var(--color-surface-text-muted)]">
										Book ID {target.bookId}
									</div>
								</div>
							</div>

							<div class="min-h-0 overflow-y-auto p-4">
								{#if target.results.length === 0}
									<div class="flex h-56 items-center justify-center rounded-2xl border border-dashed border-[var(--color-surface-border)] text-sm text-[var(--color-surface-text-muted)]">
										No results yet. Search this book to begin.
									</div>
								{:else}
									<div class="space-y-3">
										{#each target.results as result, index}
											<button
												class="flex w-full gap-4 rounded-2xl border p-4 text-left transition-colors {target.selectedIndex === index ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] hover:border-[var(--color-primary-500)]/60 hover:bg-[var(--color-surface-overlay)]'}"
												onclick={() => updateTarget(target.bookId, (item) => ({ ...item, selectedIndex: index }))}
											>
												<div class="h-28 w-20 flex-shrink-0 overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
													{#if result.cover_url}
														<img src={result.cover_url} alt={result.title} class="h-full w-full object-cover" />
													{:else}
														<div class="flex h-full items-center justify-center px-2 text-center text-xs text-[var(--color-surface-text-muted)]">
															No cover
														</div>
													{/if}
												</div>
												<div class="min-w-0 flex-1">
													<div class="flex items-start justify-between gap-3">
														<div class="min-w-0">
															<h4 class="truncate text-base font-semibold text-[var(--color-surface-text)]">{result.title || 'Untitled'}</h4>
															<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">
																{#if result.authors?.length}
																	{result.authors.join(', ')}
																{:else}
																	No authors
																{/if}
															</p>
														</div>
														<div class="rounded-full border border-[var(--color-surface-border)] px-2 py-1 text-xs text-[var(--color-surface-text-muted)]">
															{Math.round(result.match_score ?? 0)}
														</div>
													</div>
													<div class="mt-3 grid gap-2 text-sm text-[var(--color-surface-text-muted)] sm:grid-cols-2">
														<div><span class="text-[var(--color-surface-text)]">Publisher:</span> {result.publisher || '-'}</div>
														<div><span class="text-[var(--color-surface-text)]">Published:</span> {result.pub_date || '-'}</div>
														<div><span class="text-[var(--color-surface-text)]">ISBN:</span> {result.isbn || '-'}</div>
														<div><span class="text-[var(--color-surface-text)]">ASIN:</span> {result.asin || '-'}</div>
														<div><span class="text-[var(--color-surface-text)]">Pages:</span> {result.page_count || '-'}</div>
													</div>
													{#if result.description}
														<p class="mt-3 line-clamp-3 text-sm text-[var(--color-surface-text-muted)]">{result.description}</p>
													{/if}
												</div>
											</button>
										{/each}
									</div>
								{/if}
							</div>
						</div>
					</div>
				{/if}
			</div>
		{:else}
			<div class="px-6 py-10 text-center text-[var(--color-surface-text-muted)]">
				No books available for metadata lookup.
			</div>
		{/if}
	</div>
</div>

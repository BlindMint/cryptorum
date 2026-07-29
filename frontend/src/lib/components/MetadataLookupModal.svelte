<script lang="ts">
	import { onMount } from 'svelte';
	import { appActivity } from '$lib/stores';
	import { addMetadataSuggestions, refreshMetadataSuggestions } from '$lib/stores/metadataSuggestions';
	import { confirmBulkAction } from '$lib/utils/bulk-confirm';

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
		tags?: string[];
		genres?: string[];
		match_score?: number;
	};

	type MetadataProviderDiagnostic = {
		provider: string;
		query?: string;
		mode?: string;
		status: string;
		count: number;
		error?: string;
	};

	type MetadataSearchResponse = {
		results?: MetadataCandidate[];
		diagnostics?: MetadataProviderDiagnostic[];
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
		diagnostics: MetadataProviderDiagnostic[];
		selectedIndex: number;
		loading: boolean;
		error: string | null;
	};

	interface Props {
		bookIds: number[];
		title?: string;
		onClose: () => void;
		onApplied?: () => Promise<void> | void;
		confirmBeforeApply?: boolean;
		hasUnsavedChanges?: boolean;
		onApplyCurrentEdits?: () => Promise<boolean> | boolean;
	}

	let {
		bookIds = [],
		title = 'Metadata Lookup',
		onClose,
		onApplied,
		confirmBeforeApply = false,
		hasUnsavedChanges = false,
		onApplyCurrentEdits
	}: Props = $props();

	let targets = $state<LookupTarget[]>([]);
	let activeBookId = $state<number | null>(null);
	let providers = $state<{ id: string; name: string }[]>([]);
	let selectedProvider = $state('');
	let coverSelections = $state<Record<string, boolean>>({});
	let loading = $state(true);
	let applying = $state(false);
	let pendingApply = $state<{ mode: 'single' | 'all'; bookId?: number } | null>(null);
	let overwriteProtectedFields = $state(false);
	let initialized = false;
	let searchControllers = new Map<number, AbortController>();

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
			diagnostics: [],
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

	function isAnyTargetSearching(): boolean {
		return targets.some((target) => target.loading);
	}

	function isAbortError(error: unknown): boolean {
		return error instanceof DOMException && error.name === 'AbortError';
	}

	function cancelSearch(bookId: number, message = 'Search cancelled.') {
		const controller = searchControllers.get(bookId);
		if (controller) {
			controller.abort();
			searchControllers.delete(bookId);
		}
		updateTarget(bookId, (item) => ({
			...item,
			loading: false,
			error: message
		}));
	}

	function cancelAllSearches() {
		const activeBookIds = Array.from(searchControllers.keys());
		for (const bookId of activeBookIds) {
			cancelSearch(bookId);
		}
	}

	function closeLookup() {
		cancelAllSearches();
		onClose();
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
		const isbn = target.queryIsbn.trim();
		const asin = target.queryAsin.trim();
		if (title && authors) return `${title} by ${authors}`;
		if (title) return title;
		if (authors) return authors;
		if (isbn) return `ISBN ${isbn}`;
		if (asin) return `ASIN ${asin}`;
		return `Book ${target.bookId}`;
	}

	function providerLabel(provider: string): string {
		const displayName = providers.find((item) => item.id === provider)?.name;
		return displayName ?? provider
			.split('_')
			.map((part) => part ? part[0].toUpperCase() + part.slice(1) : part)
			.join(' ');
	}

	function diagnosticLabel(diagnostic: MetadataProviderDiagnostic): string {
		const provider = providerLabel(diagnostic.provider);
		if (diagnostic.status === 'ok') return `${provider}: ${diagnostic.count} result${diagnostic.count === 1 ? '' : 's'}`;
		if (diagnostic.status === 'zero_results') return `${provider}: no results`;
		if (diagnostic.status === 'failed') return `${provider}: ${diagnostic.error || 'failed'}`;
		return `${provider}: ${diagnostic.status}`;
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

		searchControllers.get(bookId)?.abort();
		const controller = new AbortController();
		searchControllers.set(bookId, controller);
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
			params.set('include_diagnostics', '1');
			params.set('limit', '6');

			const res = await fetch(`/api/metadata/search?${params.toString()}`, { signal: controller.signal });
			if (!res.ok) {
				const detail = await readErrorDetail(res);
				throw new Error(formatSearchError(res.status, detail));
			}

			const payload: MetadataCandidate[] | MetadataSearchResponse = await res.json();
			const safeResults = Array.isArray(payload) ? payload : payload.results ?? [];
			const diagnostics = Array.isArray(payload) ? [] : payload.diagnostics ?? [];
			updateTarget(bookId, (item) => ({
				...item,
				results: safeResults,
				diagnostics,
				selectedIndex: safeResults.length > 0 ? 0 : -1,
				loading: false,
				error: null
			}));
		} catch (error) {
			if (isAbortError(error)) {
				updateTarget(bookId, (item) => ({
					...item,
					loading: false,
					error: 'Search cancelled.'
				}));
				return;
			}
			console.error('Failed to search metadata:', error);
			updateTarget(bookId, (item) => ({
				...item,
				loading: false,
				error: error instanceof Error ? error.message : 'Search failed (E000): unable to reach the metadata service.'
			}));
		} finally {
			if (searchControllers.get(bookId) === controller) {
				searchControllers.delete(bookId);
			}
		}
	}

	async function searchAllTargets() {
		await Promise.all(targets.map((target) => searchTarget(target.bookId)));
	}

	async function applyMetadata(bookId: number, notifyParent = true, overrideLocked = false) {
		const target = targets.find((item) => item.bookId === bookId);
		if (!target || target.selectedIndex < 0 || target.selectedIndex >= target.results.length) return;

		const selected = target.results[target.selectedIndex];
		const metadata = metadataWithSelectedCover(bookId, target.selectedIndex, selected);

		updateTarget(bookId, (item) => ({ ...item, loading: true, error: null }));
		try {
			const res = await fetch('/api/metadata/apply', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					book_id: bookId,
					metadata,
					override_locked: overrideLocked
				})
			});

			if (!res.ok) {
				const text = await res.text();
				throw new Error(text || `Apply failed (${res.status})`);
			}

			addCandidateSuggestions([metadata]);
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

	async function applyAllSelected(overrideLocked = false) {
		const selectedItems = targets
			.filter((target) => target.selectedIndex >= 0 && target.results[target.selectedIndex])
			.map((target) => ({
				book_id: target.bookId,
				metadata: metadataWithSelectedCover(target.bookId, target.selectedIndex, target.results[target.selectedIndex])
			}));

		if (selectedItems.length === 0) {
			return;
		}
		if (!confirmBulkAction({ action: 'apply metadata to', count: selectedItems.length })) {
			return;
		}

		applying = true;
		let pendingJob: string | null = null;
		try {
			if (selectedItems.length === 1) {
				await applyMetadata(selectedItems[0].book_id, false, overrideLocked);
				await onApplied?.();
				return;
			}

			pendingJob = appActivity.startPendingJob({
				job_type: 'metadata_apply',
				title: `Bulk metadata update (${selectedItems.length} books)`,
				total_items: selectedItems.length
			});
			const res = await fetch('/api/jobs/metadata-apply', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					items: selectedItems,
					include_cover: selectedItems.some((item) => !!item.metadata.cover_url),
					override_locked: overrideLocked
				})
			});

			if (!res.ok) {
				const text = await res.text();
				appActivity.failPendingJob(pendingJob, 'Unable to queue metadata update.');
				throw new Error(text || `Queue failed (${res.status})`);
			}
			const job = await res.json().catch(() => null);
			if (job?.id) {
				appActivity.confirmPendingJob(pendingJob, job);
			} else {
				appActivity.confirmPendingJob(pendingJob);
			}
			addCandidateSuggestions(selectedItems.map((item) => item.metadata));
			await appActivity.refresh();

			await onApplied?.();
		} catch (error) {
			if (pendingJob) {
				appActivity.failPendingJob(pendingJob, 'Unable to queue metadata update.');
			}
			console.error('Failed to apply selected metadata:', error);
		} finally {
			applying = false;
		}
	}

	function requestApplyMetadata(bookId: number) {
		if (confirmBeforeApply) {
			overwriteProtectedFields = false;
			pendingApply = { mode: 'single', bookId };
			return;
		}
		void applyMetadata(bookId);
	}

	function requestApplyAllSelected() {
		if (confirmBeforeApply) {
			overwriteProtectedFields = false;
			pendingApply = { mode: 'all' };
			return;
		}
		void applyAllSelected();
	}

	async function continuePendingApply(applyCurrentEditsFirst = false) {
		const action = pendingApply;
		if (!action) return;
		if (applyCurrentEditsFirst && onApplyCurrentEdits) {
			const saved = await onApplyCurrentEdits();
			if (!saved) return;
		}
		pendingApply = null;
		if (action.mode === 'single' && action.bookId) {
			await applyMetadata(action.bookId, true, overwriteProtectedFields);
		} else {
			await applyAllSelected(overwriteProtectedFields);
		}
		overwriteProtectedFields = false;
	}

	function getSelectedResult(target: LookupTarget): MetadataCandidate | null {
		return target.selectedIndex >= 0 ? target.results[target.selectedIndex] ?? null : null;
	}

	function candidateCoverKey(bookId: number, index: number): string {
		return `${bookId}:${index}`;
	}

	function shouldUpdateCover(bookId: number, index: number, candidate: MetadataCandidate): boolean {
		if (!candidate.cover_url) return false;
		return coverSelections[candidateCoverKey(bookId, index)] ?? true;
	}

	function setCandidateCover(bookId: number, index: number, value: boolean) {
		coverSelections = {
			...coverSelections,
			[candidateCoverKey(bookId, index)]: value
		};
	}

	function metadataWithSelectedCover(bookId: number, index: number, candidate: MetadataCandidate): MetadataCandidate {
		return {
			...candidate,
			cover_url: shouldUpdateCover(bookId, index, candidate) ? candidate.cover_url : ''
		};
	}

	function addCandidateSuggestions(candidates: MetadataCandidate[]) {
		addMetadataSuggestions({
			authors: candidates.flatMap((candidate) => candidate.authors ?? []),
			series: candidates.flatMap((candidate) => candidate.series ? [candidate.series] : []),
			publishers: candidates.flatMap((candidate) => candidate.publisher ? [candidate.publisher] : []),
			languages: candidates.flatMap((candidate) => candidate.language ? [candidate.language] : []),
			genres: candidates.flatMap((candidate) => candidate.genres ?? []),
			tags: candidates.flatMap((candidate) => candidate.tags ?? [])
		});
		void refreshMetadataSuggestions();
	}

	onMount(() => {
		void initialize();
		return () => {
			cancelAllSearches();
		};
	});
</script>

<div class="fixed inset-0 z-[120] flex items-center justify-center p-4">
	<button
		type="button"
		aria-label="Close metadata lookup modal"
		class="absolute inset-0 z-0 bg-black/70"
		onclick={closeLookup}
	></button>
	<div class="relative z-10 flex max-h-[92vh] w-full max-w-6xl flex-col overflow-hidden rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
		<div class="flex items-center justify-between gap-4 border-b border-[var(--color-surface-border)] px-5 py-3.5">
			<div>
				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">{title}</h2>
				<p class="text-sm text-[var(--color-surface-text-muted)]">
					Review selected books one at a time, search providers, then apply the best match.
				</p>
			</div>
			<div class="flex items-center gap-2">
					<button
						class="inline-flex items-center gap-2 rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)]"
						onclick={searchAllTargets}
						disabled={loading || applying}
					>
						{#if isAnyTargetSearching()}
							<span class="inline-flex h-3.5 w-3.5 items-center justify-center" aria-hidden="true">
								<span class="h-3.5 w-3.5 animate-spin rounded-full border-2 border-[var(--color-surface-border)] border-t-[var(--color-primary-500)]"></span>
							</span>
						{/if}
						{isAnyTargetSearching() ? 'Searching All...' : 'Search All'}
					</button>
					{#if isAnyTargetSearching()}
						<button
							type="button"
							class="rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm text-amber-200 transition-colors hover:bg-amber-500/20"
							onclick={cancelAllSearches}
						>
							Cancel Search
						</button>
					{/if}
				<button
					class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)]"
					onclick={closeLookup}
				>
					Close
				</button>
			</div>
		</div>

		{#if loading}
			<div class="flex h-[calc(92vh-88px)] flex-col gap-4 overflow-hidden p-4 lg:p-6">
				<div class="grid min-h-0 flex-1 gap-4 overflow-hidden lg:grid-cols-[360px_minmax(0,1fr)]">
					<div class="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
						<div class="border-b border-[var(--color-surface-border)] px-5 py-3.5">
							<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Search Fields</h3>
							<p class="text-sm text-[var(--color-surface-text-muted)]">Loading book details and providers...</p>
						</div>
						<div class="min-h-0 flex-1 overflow-y-auto px-4 py-4 space-y-4" aria-busy="true">
							{#each ['Title', 'Authors', 'ISBN', 'ASIN', 'Series', 'Publisher', 'Provider'] as label}
								<div>
									<div class="mb-1 text-sm font-medium text-[var(--color-surface-text-muted)]">{label}</div>
									<div class="h-[42px] rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)]">
										<div class="h-full w-2/3 animate-pulse rounded-lg bg-[var(--color-surface-700)]/70"></div>
									</div>
								</div>
							{/each}
							<div class="flex items-center gap-3">
								<div class="h-4 w-4 rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)]"></div>
								<div class="h-4 w-40 animate-pulse rounded bg-[var(--color-surface-700)]/70"></div>
							</div>
							<div class="grid grid-cols-2 gap-2 pt-2">
								<div class="h-9 rounded-lg bg-[var(--color-primary-500)]/40"></div>
								<div class="h-9 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)]"></div>
							</div>
						</div>
					</div>

					<div class="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
						<div class="border-b border-[var(--color-surface-border)] px-5 py-3.5">
							<div class="flex items-center justify-between gap-3">
								<div>
									<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Results</h3>
									<p class="text-sm text-[var(--color-surface-text-muted)]">Search results will appear here.</p>
								</div>
								<div class="h-4 w-16 animate-pulse rounded bg-[var(--color-surface-700)]/70"></div>
							</div>
						</div>
						<div class="min-h-0 flex-1 overflow-y-auto p-5">
							<div class="flex h-full min-h-56 items-center justify-center rounded-2xl border border-dashed border-[var(--color-surface-border)] text-center">
								<div>
									<div class="mx-auto h-10 w-10 animate-spin rounded-full border-2 border-[var(--color-surface-border)] border-t-[var(--color-primary-500)]"></div>
									<p class="mt-3 text-sm text-[var(--color-surface-text-muted)]">Preparing metadata lookup...</p>
								</div>
							</div>
						</div>
					</div>
				</div>
			</div>
		{:else if targets.length > 0}
			{@const target = activeTarget()}
			{@const targetIndex = activeTargetIndex()}
			<div class="flex h-[calc(92vh-88px)] flex-col gap-4 overflow-hidden p-4 lg:p-6">
				{#if targets.length > 1}
					<div class="flex items-center justify-between gap-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-5 py-3.5">
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
							<div class="border-b border-[var(--color-surface-border)] px-5 py-3.5">
								<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Search Fields</h3>
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
												<option value="" class="bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)]">All providers</option>
												{#each providers as provider}
													<option value={provider.id} class="bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)]">{provider.name}</option>
												{/each}
											</select>
										</div>
										<div class="grid grid-cols-2 gap-2 pt-2">
											<button
												class="accent-action inline-flex items-center justify-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors"
												onclick={() => searchTarget(target.bookId)}
												disabled={target.loading || applying}
											>
												{#if target.loading}
													<span class="inline-flex h-3.5 w-3.5 items-center justify-center" aria-hidden="true">
														<span class="h-3.5 w-3.5 animate-spin rounded-full border-2 border-[var(--color-primary-500)]/30 border-t-[var(--color-primary-400)]"></span>
													</span>
												{/if}
												{target.loading ? 'Searching...' : 'Search'}
											</button>
											<button
												class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-overlay)] disabled:opacity-50"
												onclick={() => requestApplyMetadata(target.bookId)}
												disabled={target.loading || applying || target.selectedIndex < 0}
											>
												Apply Current
											</button>
										</div>
								{#if targets.length > 1}
									<button
										class="w-full rounded-lg border border-[var(--color-primary-500)]/40 bg-[var(--color-primary-500)]/10 px-3 py-2 text-sm font-medium text-[var(--color-primary-300)] transition-colors hover:bg-[var(--color-primary-500)]/20 disabled:opacity-50"
										onclick={requestApplyAllSelected}
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

						<div class="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
							<div class="border-b border-[var(--color-surface-border)] px-5 py-3.5">
								<div class="flex items-center justify-between gap-3">
									<div>
										<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Results</h3>
										<p class="text-sm text-[var(--color-surface-text-muted)]">
											{target.results.length} result{target.results.length === 1 ? '' : 's'} for {resultSummaryForTarget(target)}
										</p>
									</div>
									<div class="flex flex-wrap items-center justify-end gap-2 text-sm text-[var(--color-surface-text-muted)]">
										{#if target.loading}
											<span class="inline-flex items-center gap-2 rounded-full border border-[var(--color-primary-500)]/35 bg-[var(--color-primary-500)]/10 px-2.5 py-1 text-xs font-medium text-[var(--color-primary-200)]">
												<span class="h-3 w-3 animate-spin rounded-full border-2 border-[var(--color-surface-border)] border-t-[var(--color-primary-500)]" aria-hidden="true"></span>
												Searching providers...
											</span>
											<button
												type="button"
												class="rounded-full border border-amber-500/40 bg-amber-500/10 px-2.5 py-1 text-xs font-medium text-amber-200 transition-colors hover:bg-amber-500/20"
												onclick={() => cancelSearch(target.bookId)}
											>
												Cancel
											</button>
										{/if}
										<span>Book ID {target.bookId}</span>
									</div>
								</div>
							</div>

							<div class="min-h-0 flex-1 overflow-y-auto p-5">
								{#if target.results.length === 0}
									<div class="flex min-h-56 flex-col items-center justify-center rounded-2xl border border-dashed border-[var(--color-surface-border)] px-4 py-6 text-center text-sm text-[var(--color-surface-text-muted)]">
										<p>{target.diagnostics.length > 0 ? 'No metadata matches were found.' : 'No results yet. Search this book to begin.'}</p>
										{#if target.diagnostics.length > 0}
											<div class="mt-4 w-full max-w-xl space-y-1 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-3 text-left text-xs">
												{#each target.diagnostics as diagnostic}
													<div class={diagnostic.status === 'failed' ? 'text-red-300' : 'text-[var(--color-surface-text-muted)]'}>
														{diagnosticLabel(diagnostic)}
													</div>
												{/each}
											</div>
										{/if}
									</div>
								{:else}
									<div class="space-y-3">
										{#each target.results as result, index}
											<div
												role="button"
												tabindex="0"
												class="flex w-full gap-4 rounded-2xl border p-4 text-left transition-colors {target.selectedIndex === index ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] hover:border-[var(--color-primary-500)]/60 hover:bg-[var(--color-surface-overlay)]'}"
												onclick={() => updateTarget(target.bookId, (item) => ({ ...item, selectedIndex: index }))}
												onkeydown={(event) => {
													if (event.key === 'Enter' || event.key === ' ') {
														event.preventDefault();
														updateTarget(target.bookId, (item) => ({ ...item, selectedIndex: index }));
													}
												}}
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
															<h4 class="truncate text-lg font-semibold text-[var(--color-surface-text)]">{result.title || 'Untitled'}</h4>
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
													<div class="mt-3 inline-flex items-center gap-2 text-sm text-[var(--color-surface-text)] {result.cover_url ? '' : 'opacity-50'}">
														<input
															type="checkbox"
															checked={shouldUpdateCover(target.bookId, index, result)}
															disabled={!result.cover_url}
															onclick={(event) => event.stopPropagation()}
															onkeydown={(event) => event.stopPropagation()}
															onchange={(event) => setCandidateCover(target.bookId, index, (event.currentTarget as HTMLInputElement).checked)}
															class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)] disabled:opacity-50"
														/>
														<span>Update cover</span>
													</div>
												</div>
											</div>
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

	{#if pendingApply}
		<div class="fixed inset-0 z-20 flex items-center justify-center p-4">
			<button
				type="button"
				class="absolute inset-0 bg-black/70"
				aria-label="Cancel metadata apply"
				onclick={() => { pendingApply = null; overwriteProtectedFields = false; }}
			></button>
			<div class="relative w-full max-w-lg rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5 shadow-2xl">
				<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Apply metadata result?</h3>
				<p class="mt-2 text-sm text-[var(--color-surface-text-muted)]">
					The result will update unprotected metadata for this book{pendingApply.mode === 'all' ? ' selection' : ''}. Protected user changes are preserved by default.
					{#if hasUnsavedChanges}
						Unsaved inline edits will be lost unless you apply them first.
					{/if}
				</p>
				<label class="mt-4 flex items-start gap-3 rounded-lg border border-amber-500/25 bg-amber-500/[0.07] p-3">
					<input
						type="checkbox"
						bind:checked={overwriteProtectedFields}
						class="mt-0.5 h-4 w-4 rounded border-[var(--color-surface-border)] bg-[var(--color-surface-700)] text-amber-500 focus:ring-amber-500"
					>
					<span>
						<span class="block text-sm font-medium text-amber-200">Overwrite protected fields</span>
						<span class="mt-0.5 block text-xs leading-5 text-[var(--color-surface-text-muted)]">Use this only when the selected result should intentionally replace prior user edits.</span>
					</span>
				</label>
				<div class="mt-5 flex flex-wrap justify-end gap-2">
					<button
						type="button"
						onclick={() => { pendingApply = null; overwriteProtectedFields = false; }}
						class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)]"
					>
						Cancel
					</button>
					{#if hasUnsavedChanges && onApplyCurrentEdits}
						<button
							type="button"
							onclick={() => continuePendingApply(true)}
							disabled={applying}
							class="rounded-lg border border-[var(--color-primary-500)]/40 bg-[var(--color-primary-500)]/10 px-4 py-2 text-sm font-medium text-[var(--color-primary-200)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-primary-500)]/20 disabled:opacity-50"
						>
							Apply Current Edits First
						</button>
						{/if}
						<button
							type="button"
							onclick={() => continuePendingApply(false)}
							disabled={applying}
							class="accent-action rounded-lg px-4 py-2 text-sm font-medium transition-all duration-200 ease-out hover:-translate-y-px"
						>
							Apply Metadata
						</button>
				</div>
			</div>
		</div>
	{/if}
</div>

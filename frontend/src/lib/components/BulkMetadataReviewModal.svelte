<script lang="ts">
	import { onMount } from 'svelte';
	import { appActivity, reviewedMetadataLookupJobs } from '$lib/stores';
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

	type CurrentMetadata = {
		book_id: number;
		title: string;
		authors?: string[];
		series?: string;
		publisher?: string;
		pub_date?: string;
		description?: string;
		isbn?: string;
		asin?: string;
		cover_path?: string;
		page_count?: number;
		language?: string;
	};

	type LookupItem = {
		book_id: number;
		current: CurrentMetadata;
		match?: MetadataCandidate;
		matches?: MetadataCandidate[];
		status: string;
		error?: string;
		query?: string;
		diagnostics?: MetadataProviderDiagnostic[];
	};

	type AdminJob = {
		id: number;
		job_type: string;
		title: string;
		status: string;
		result?: {
			items?: LookupItem[];
			completed?: number;
			failed?: number;
			total?: number;
		};
		total_items: number;
		completed_items: number;
		failed_items: number;
		created_at: number;
		error?: string;
		cancel_requested_at?: number;
	};

	interface Props {
		jobId?: number | null;
		bookIds?: number[];
		initialJob?: AdminJob | null;
		onClose: () => void;
		onApplied?: () => Promise<void> | void;
	}

	let { jobId = null, bookIds = [], initialJob = null, onClose, onApplied }: Props = $props();

	let job = $state<AdminJob | null>(null);
	let pendingItems = $state<LookupItem[]>([]);
	let selectedMatchIndexes = $state<Record<number, number>>({});
	let loading = $state(true);
	let loadingBooks = $state(false);
	let applying = $state(false);
	let cancelling = $state(false);
	let queueingLookup = $state(false);
	let showLookupConfirm = $state(false);
	let topMatchOnly = $state(false);
	let applyMessage = $state('');
	let overwriteProtectedFields = $state(false);
	let coverSelections = $state<Record<string, boolean>>({});
	let appliedBookIds = $state<Set<number>>(new Set());
	let pollTimer: number | null = null;
	let selectionTouched = false;

	const sourceItems = $derived(job?.result?.items ?? pendingItems);
	const items = $derived(sourceItems.filter((item) => !appliedBookIds.has(item.book_id)));
	const matchedItems = $derived(items.filter((item) => candidatesForItem(item).length > 0));
	const selectedItems = $derived(items
		.map((item) => {
			const candidates = candidatesForItem(item);
			const selectedIndex = selectedMatchIndexes[item.book_id];
			const metadata = selectedIndex >= 0 ? candidates[selectedIndex] : undefined;
			return metadata ? { item, metadata, selectedIndex } : null;
		})
		.filter((entry): entry is { item: LookupItem; metadata: MetadataCandidate; selectedIndex: number } => entry !== null));

	function authors(value: string[] | undefined): string {
		return value?.filter(Boolean).join(', ') || '-';
	}

	function score(value: number | undefined): string {
		if (value === undefined || value === null) return '-';
		return String(Math.round(value));
	}

	function providerLabel(provider: string): string {
		return provider
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

	function candidatesForItem(item: LookupItem): MetadataCandidate[] {
		if (item.matches?.length) return item.matches;
		return item.match ? [item.match] : [];
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

	function selectAllCovers() {
		const next = { ...coverSelections };
		for (const item of matchedItems) {
			for (const [index, candidate] of candidatesForItem(item).entries()) {
				if (candidate.cover_url) {
					next[candidateCoverKey(item.book_id, index)] = true;
				}
			}
		}
		coverSelections = next;
	}

	function clearAllCovers() {
		const next = { ...coverSelections };
		for (const item of matchedItems) {
			for (const [index, candidate] of candidatesForItem(item).entries()) {
				if (candidate.cover_url) {
					next[candidateCoverKey(item.book_id, index)] = false;
				}
			}
		}
		coverSelections = next;
	}

	function parseAuthors(value: string | string[] | undefined): string[] {
		if (Array.isArray(value)) return value.filter(Boolean);
		if (!value) return [];
		try {
			const parsed = JSON.parse(value);
			return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === 'string' && item.trim().length > 0) : [value];
		} catch {
			return value.split(',').map((item) => item.trim()).filter(Boolean);
		}
	}

	function isActiveLookupJob(value: AdminJob | null): boolean {
		return value?.status === 'queued' || value?.status === 'running' || value?.status === 'cancelling';
	}

	function lookupTitle(): string {
		if (job?.title) return job.title;
		const count = bookIds.length || pendingItems.length;
		return count > 0 ? `${count} selected book${count === 1 ? '' : 's'}` : 'No books selected';
	}

	function totalCount(): number {
		return job?.total_items ?? bookIds.length ?? pendingItems.length;
	}

	function selectCandidate(bookId: number, index: number) {
		selectionTouched = true;
		const next = { ...selectedMatchIndexes };
		if (next[bookId] === index) {
			delete next[bookId];
		} else {
			next[bookId] = index;
		}
		selectedMatchIndexes = next;
	}

	function selectAllMatches() {
		selectionTouched = true;
		const next: Record<number, number> = {};
		for (const item of matchedItems) {
			next[item.book_id] = 0;
		}
		selectedMatchIndexes = next;
	}

	function clearSelection() {
		selectionTouched = true;
		selectedMatchIndexes = {};
	}

	function syncDefaultSelection() {
		if (!selectionTouched && matchedItems.length > 0) {
			const next: Record<number, number> = {};
			for (const item of matchedItems) {
				next[item.book_id] = 0;
			}
			selectedMatchIndexes = next;
		}
	}

	async function loadJob(silent = false) {
		if (!jobId && !job?.id) {
			loading = false;
			return;
		}
		if (!silent) loading = true;
		try {
			const res = await fetch(`/api/jobs/${job?.id ?? jobId}`);
			if (res.ok) {
				job = await res.json();
				syncDefaultSelection();
			}
		} finally {
			loading = false;
		}
	}

	async function loadPendingBooks() {
		if (initialJob || jobId || bookIds.length === 0) {
			loading = false;
			return;
		}
		loadingBooks = true;
		loading = true;
		try {
			const summaries = await Promise.all(bookIds.map(async (bookId) => {
				try {
					const res = await fetch(`/api/books/${bookId}`);
					return res.ok ? await res.json() : null;
				} catch {
					return null;
				}
			}));
			pendingItems = summaries.map((summary, index) => {
				const fallbackId = bookIds[index];
				if (!summary) {
					return {
						book_id: fallbackId,
						current: {
							book_id: fallbackId,
							title: `Book ${fallbackId}`,
							authors: []
						},
						status: 'pending',
						error: 'Unable to load book summary'
					};
				}
				return {
					book_id: summary.id,
					current: {
						book_id: summary.id,
						title: summary.title ?? '',
						authors: parseAuthors(summary.authors),
						series: summary.series ?? '',
						publisher: summary.publisher ?? '',
						pub_date: summary.pub_date ?? '',
						description: summary.description ?? '',
						isbn: summary.isbn ?? '',
						asin: summary.asin ?? '',
						cover_path: summary.cover_path ?? '',
						page_count: summary.page_count ?? 0,
						language: summary.language ?? ''
					},
					status: 'pending'
				};
			});
		} finally {
			loadingBooks = false;
			loading = false;
		}
	}

	function startPolling() {
		if (pollTimer) window.clearInterval(pollTimer);
		pollTimer = window.setInterval(async () => {
			await loadJob(true);
			if (job && !isActiveLookupJob(job)) {
				if (pollTimer) window.clearInterval(pollTimer);
				pollTimer = null;
			}
		}, 2500);
	}

	async function queueLookupJob() {
		if (queueingLookup || isActiveLookupJob(job) || bookIds.length === 0) return;

		showLookupConfirm = false;
		queueingLookup = true;
		applyMessage = '';
		const selectedCount = bookIds.length;
		const pendingJob = appActivity.startPendingJob({
			job_type: 'metadata_lookup',
			title: `Bulk metadata lookup (${selectedCount} books)`,
			total_items: selectedCount
		});

		try {
			const res = await fetch('/api/jobs/metadata-lookup', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ book_ids: bookIds, top_match_only: topMatchOnly })
			});
			if (!res.ok) throw new Error(await res.text());
			job = await res.json();
			if (job?.id) {
				reviewedMetadataLookupJobs.mark(job.id);
				appActivity.confirmPendingJob(pendingJob, job);
			} else {
				appActivity.confirmPendingJob(pendingJob);
			}
			await appActivity.refresh();
			void loadJob(true);
			startPolling();
		} catch (error) {
			console.error('Failed to queue metadata lookup:', error);
			appActivity.failPendingJob(pendingJob, 'Unable to queue metadata lookup.');
			applyMessage = 'Unable to queue metadata lookup.';
		} finally {
			queueingLookup = false;
		}
	}

	async function cancelLookupJob() {
		if (!job || !isActiveLookupJob(job) || cancelling) return;
		if (!window.confirm('Cancel this metadata lookup? Completed matches will remain available for review.')) return;

		cancelling = true;
		applyMessage = '';
		try {
			const res = await fetch(`/api/jobs/${job.id}/cancel`, { method: 'POST' });
			if (!res.ok) throw new Error(await res.text());
			job = {
				...job,
				status: job.status === 'queued' ? 'cancelled' : 'cancelling',
				cancel_requested_at: Math.floor(Date.now() / 1000)
			};
			await appActivity.refresh();
			await loadJob(true);
			if (job && isActiveLookupJob(job) && !pollTimer) {
				startPolling();
			}
		} catch (error) {
			console.error('Failed to cancel metadata lookup:', error);
			applyMessage = 'Unable to cancel metadata lookup.';
		} finally {
			cancelling = false;
		}
	}

	async function applySelections(entriesToApply: { item: LookupItem; metadata: MetadataCandidate; selectedIndex: number }[]) {
		const validItems = entriesToApply.filter((entry) => entry.metadata);
		if (validItems.length === 0) return;
		if (!confirmBulkAction({ action: 'apply metadata to', count: validItems.length })) return;

		applying = true;
		applyMessage = '';
		let pendingJob: string | null = null;
		try {
			const payload = validItems.map(({ item, metadata, selectedIndex }) => ({
				book_id: item.book_id,
				metadata: metadataWithSelectedCover(item.book_id, selectedIndex, metadata)
			}));
			const includeCover = payload.some((entry) => !!entry.metadata.cover_url);

			const endpoint = payload.length === 1 ? '/api/metadata/apply' : '/api/jobs/metadata-apply';
			const body = payload.length === 1
				? { book_id: payload[0].book_id, metadata: payload[0].metadata, override_locked: overwriteProtectedFields }
				: { items: payload, include_cover: includeCover, override_locked: overwriteProtectedFields };
			if (payload.length > 1) {
				pendingJob = appActivity.startPendingJob({
					job_type: 'metadata_apply',
					title: `Apply metadata (${payload.length} books)`,
					total_items: payload.length
				});
			}

			const res = await fetch(endpoint, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body)
			});
			if (!res.ok) {
				if (pendingJob) appActivity.failPendingJob(pendingJob, 'Unable to queue metadata update.');
				throw new Error(await res.text());
			}
			if (pendingJob) {
				const queuedJob = await res.json();
				if (queuedJob?.id) {
					appActivity.confirmPendingJob(pendingJob, queuedJob);
				} else {
					appActivity.confirmPendingJob(pendingJob);
				}
				await appActivity.refresh();
			}

			const next = { ...selectedMatchIndexes };
			for (const { item } of validItems) delete next[item.book_id];
			selectionTouched = true;
			selectedMatchIndexes = next;
			const applied = new Set(appliedBookIds);
			for (const { item } of validItems) applied.add(item.book_id);
			appliedBookIds = applied;
			applyMessage = payload.length === 1 ? 'Metadata updated.' : 'Metadata update job queued.';
			await onApplied?.();
		} catch (error) {
			console.error('Failed to apply metadata:', error);
			if (pendingJob) appActivity.failPendingJob(pendingJob, 'Unable to queue metadata update.');
			applyMessage = 'Unable to update metadata.';
		} finally {
			applying = false;
		}
	}

	onMount(() => {
		reviewedMetadataLookupJobs.init();
		job = initialJob;
		loading = !initialJob;
		if (job?.id) {
			reviewedMetadataLookupJobs.mark(job.id);
		}
		syncDefaultSelection();
		if (job || jobId) {
			void loadJob(!!initialJob);
		} else {
			void loadPendingBooks();
		}
		if (job && isActiveLookupJob(job)) {
			startPolling();
		}
		return () => {
			if (pollTimer) window.clearInterval(pollTimer);
		};
	});
</script>

<div class="fixed inset-0 z-[125] flex items-center justify-center p-4">
	<button type="button" class="absolute inset-0 bg-black/70" aria-label="Close metadata review" onclick={onClose}></button>
	<div class="relative flex max-h-[92vh] w-full max-w-7xl flex-col overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
		<header class="flex items-center justify-between gap-4 border-b border-[var(--color-surface-border)] px-5 py-3.5">
			<div>
				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Bulk Metadata Lookup</h2>
				<p class="text-sm text-[var(--color-surface-text-muted)]">
					{lookupTitle()}
				</p>
			</div>
			<div class="flex items-center gap-2">
				<span class="rounded-full border border-[var(--color-surface-border)] px-3 py-1 text-xs uppercase tracking-normal text-[var(--color-surface-text-muted)]">
					{job?.status ?? 'not started'}
				</span>
				{#if isActiveLookupJob(job)}
					<button
						type="button"
						class="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm text-amber-200 transition-all duration-200 ease-out hover:-translate-y-px hover:bg-amber-500/20 hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-400 disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
						onclick={cancelLookupJob}
						disabled={cancelling || job?.status === 'cancelling'}
					>
						{cancelling || job?.status === 'cancelling' ? 'Cancelling...' : 'Cancel'}
					</button>
				{/if}
				<button
					type="button"
					class="rounded-md border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-base)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]"
					onclick={onClose}
				>
					Close
				</button>
			</div>
		</header>

		{#if loading}
			<div class="flex h-72 items-center justify-center">
				<div class="h-10 w-10 animate-spin rounded-full border-b-2 border-[var(--color-primary-500)]"></div>
			</div>
		{:else if !job && bookIds.length === 0 && pendingItems.length === 0}
			<div class="px-6 py-12 text-center text-sm text-[var(--color-surface-text-muted)]">Unable to load this job.</div>
		{:else}
			<div class="border-b border-[var(--color-surface-border)] px-6 py-3">
				<div class="flex flex-wrap items-center justify-between gap-3">
					<div class="text-sm text-[var(--color-surface-text-muted)]">
						{job?.completed_items ?? 0} / {totalCount()} checked, {matchedItems.length} match{matchedItems.length === 1 ? '' : 'es'}, {job?.failed_items ?? 0} failed
					</div>
					<div class="flex flex-wrap items-center gap-2">
						<label class="flex items-center gap-2 rounded-md border border-amber-500/20 bg-amber-500/[0.06] px-2 py-1 text-sm text-amber-200">
							<input type="checkbox" bind:checked={overwriteProtectedFields} class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-amber-500 focus:ring-amber-500" />
							Overwrite protected
						</label>
						<label class="flex items-center gap-2 rounded-md px-2 py-1 text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)]">
							<input type="checkbox" bind:checked={topMatchOnly} disabled={!!job || queueingLookup} class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)] disabled:opacity-50" />
							Top match only
						</label>
						<button type="button" class="rounded-md border border-[var(--color-surface-border)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-base)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]" onclick={selectAllCovers}>
							Select covers
						</button>
						<button type="button" class="rounded-md border border-[var(--color-surface-border)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-base)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]" onclick={clearAllCovers}>
							Clear covers
						</button>
						{#if !job}
							<button
								type="button"
								class="accent-action rounded-md px-3 py-1.5 text-sm font-medium transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:hover:translate-y-0 disabled:hover:shadow-none"
								onclick={() => showLookupConfirm = true}
								disabled={queueingLookup || bookIds.length === 0 || loadingBooks}
							>
								{queueingLookup ? 'Queueing...' : 'Lookup Metadata'}
							</button>
						{/if}
						<button type="button" class="rounded-md border border-[var(--color-surface-border)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-base)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]" onclick={selectAllMatches}>
							Select matches
						</button>
						<button type="button" class="rounded-md border border-[var(--color-surface-border)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-base)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]" onclick={clearSelection}>
							Clear
						</button>
						<button
							type="button"
							class="accent-action rounded-md px-3 py-1.5 text-sm font-medium transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:hover:translate-y-0 disabled:hover:shadow-none"
							onclick={() => applySelections(selectedItems)}
							disabled={applying || selectedItems.length === 0}
						>
							{applying ? 'Applying...' : `Apply selected (${selectedItems.length})`}
						</button>
					</div>
				</div>
				{#if applyMessage}
					<div class="mt-2 text-sm text-[var(--color-surface-text-muted)]">{applyMessage}</div>
				{/if}
			</div>

			<div class="min-h-0 flex-1 overflow-y-auto p-5">
				{#if items.length === 0}
					<div class="rounded-lg border border-dashed border-[var(--color-surface-border)] px-6 py-12 text-center text-sm text-[var(--color-surface-text-muted)]">
						{appliedBookIds.size > 0 ? 'All visible matches have been applied.' : 'No lookup results yet.'}
					</div>
				{:else}
					<div class="space-y-3">
						{#each items as item}
							<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4 transition-all duration-200 ease-out hover:border-[var(--color-surface-500)] hover:bg-[var(--color-surface-700)]/60 hover:shadow-sm">
								<div class="mb-3 flex items-center justify-between gap-3">
									<div class="min-w-0">
										<div class="truncate text-sm font-medium text-[var(--color-surface-text)]">
											{item.current.title || `Book ${item.book_id}`}
										</div>
										<div class="mt-1 text-xs text-[var(--color-surface-text-muted)]">
										{#if selectedMatchIndexes[item.book_id] !== undefined}
											Selection ready
										{:else if candidatesForItem(item).length > 0}
											Choose a match to apply
										{:else if !job}
											Ready to search
										{:else}
											No selectable match
										{/if}
										</div>
									</div>
									<div class="flex items-center gap-2">
										<span class="rounded-full border border-[var(--color-surface-border)] px-2 py-1 text-xs text-[var(--color-surface-text-muted)]">
											{#if candidatesForItem(item).length > 0}
												{candidatesForItem(item).length} match{candidatesForItem(item).length === 1 ? '' : 'es'}
											{:else}
												{item.status}
											{/if}
										</span>
										<button
											type="button"
											class="rounded-md border border-[var(--color-surface-border)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-overlay)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
											onclick={() => {
												const candidates = candidatesForItem(item);
												const selectedIndex = selectedMatchIndexes[item.book_id];
												const metadata = selectedIndex >= 0 ? candidates[selectedIndex] : undefined;
												if (metadata) applySelections([{ item, metadata, selectedIndex }]);
											}}
											disabled={applying || selectedMatchIndexes[item.book_id] === undefined}
										>
											Apply
										</button>
									</div>
								</div>

								<div class="grid gap-4 lg:grid-cols-2">
									<section class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4">
										<div class="mb-3 text-xs font-semibold uppercase tracking-normal text-[var(--color-surface-text-muted)]">Current</div>
										<div class="flex gap-4">
											<div class="h-32 w-24 shrink-0 overflow-hidden rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
												{#if item.current.cover_path}
													<img src="/api/covers/{item.book_id}" alt={item.current.title} class="h-full w-full object-cover" />
												{:else}
													<div class="flex h-full items-center justify-center px-2 text-center text-xs text-[var(--color-surface-text-muted)]">No cover</div>
												{/if}
											</div>
											<div class="min-w-0 text-sm">
												<div class="font-semibold text-[var(--color-surface-text)]">{item.current.title || '-'}</div>
												<div class="mt-1 text-[var(--color-surface-text-muted)]">{authors(item.current.authors)}</div>
												<div class="mt-3 grid gap-1 text-[var(--color-surface-text-muted)] sm:grid-cols-2">
													<div>Series: {item.current.series || '-'}</div>
													<div>Publisher: {item.current.publisher || '-'}</div>
													<div>Published: {item.current.pub_date || '-'}</div>
													<div>ISBN: {item.current.isbn || '-'}</div>
													<div>ASIN: {item.current.asin || '-'}</div>
												</div>
											</div>
										</div>
									</section>

									<section class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4">
										<div class="mb-3 text-xs font-semibold uppercase tracking-normal text-[var(--color-surface-text-muted)]">Matches</div>
										{#if candidatesForItem(item).length > 0}
											<div class="space-y-3">
												{#each candidatesForItem(item) as candidate, index}
													<div
														role="button"
														tabindex="0"
														class="flex w-full gap-4 rounded-lg border p-3 text-left transition-colors {selectedMatchIndexes[item.book_id] === index ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-base)] hover:border-[var(--color-primary-500)]/60 hover:bg-[var(--color-surface-700)]'}"
														onclick={() => selectCandidate(item.book_id, index)}
														onkeydown={(event) => {
															if (event.key === 'Enter' || event.key === ' ') {
																event.preventDefault();
																selectCandidate(item.book_id, index);
															}
														}}
													>
														<div class="h-28 w-20 shrink-0 overflow-hidden rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
															{#if candidate.cover_url}
																<img src={candidate.cover_url} alt={candidate.title} class="h-full w-full object-cover" />
															{:else}
																<div class="flex h-full items-center justify-center px-2 text-center text-xs text-[var(--color-surface-text-muted)]">No cover</div>
															{/if}
														</div>
														<div class="min-w-0 flex-1 text-sm">
															<div class="flex items-start justify-between gap-3">
																<div class="min-w-0">
																	<div class="truncate font-semibold text-[var(--color-surface-text)]">{candidate.title || '-'}</div>
																	<div class="mt-1 text-[var(--color-surface-text-muted)]">{authors(candidate.authors)}</div>
																</div>
																<div class="rounded-full border border-[var(--color-surface-border)] px-2 py-1 text-xs text-[var(--color-surface-text-muted)]">
																	{score(candidate.match_score)}
																</div>
															</div>
															<div class="mt-3 grid gap-1 text-[var(--color-surface-text-muted)] sm:grid-cols-2">
																<div>Series: {candidate.series || '-'}</div>
																<div>Publisher: {candidate.publisher || '-'}</div>
																<div>Published: {candidate.pub_date || '-'}</div>
																<div>ISBN: {candidate.isbn || '-'}</div>
																<div>ASIN: {candidate.asin || '-'}</div>
																<div>Provider: {candidate.provider || '-'}</div>
																<div>Pages: {candidate.page_count || '-'}</div>
															</div>
															<div class="mt-3 inline-flex items-center gap-2 text-sm text-[var(--color-surface-text)] {candidate.cover_url ? '' : 'opacity-50'}">
																<input
																	type="checkbox"
																	checked={shouldUpdateCover(item.book_id, index, candidate)}
																	disabled={!candidate.cover_url}
																	onclick={(event) => event.stopPropagation()}
																	onkeydown={(event) => event.stopPropagation()}
																	onchange={(event) => setCandidateCover(item.book_id, index, (event.currentTarget as HTMLInputElement).checked)}
																	class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)] disabled:opacity-50"
																/>
																<span>Update cover</span>
															</div>
														</div>
													</div>
												{/each}
											</div>
										{:else}
											<div class="flex min-h-32 flex-col justify-center rounded-md border border-dashed border-[var(--color-surface-border)] p-4 text-sm text-[var(--color-surface-text-muted)]">
												<div>{!job ? 'Lookup has not started.' : item.status === 'pending' || item.status === 'searching' ? 'Searching for matches...' : item.status === 'cancelled' ? 'Lookup cancelled.' : item.error || 'No match found'}</div>
												{#if item.diagnostics?.length}
													<div class="mt-3 space-y-1 text-xs">
														{#each item.diagnostics as diagnostic}
															<div class={diagnostic.status === 'failed' ? 'text-red-300' : ''}>
																{diagnosticLabel(diagnostic)}
															</div>
														{/each}
													</div>
												{/if}
											</div>
										{/if}
									</section>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>

	{#if showLookupConfirm}
		<div class="fixed inset-0 z-20 flex items-center justify-center p-4">
			<button type="button" class="absolute inset-0 bg-black/70" aria-label="Cancel lookup confirmation" onclick={() => showLookupConfirm = false}></button>
			<div class="relative w-full max-w-lg rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
				<div class="border-b border-[var(--color-surface-border)] px-5 py-4">
					<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Start metadata lookup?</h3>
					<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">
						This will search metadata providers for {bookIds.length} selected book{bookIds.length === 1 ? '' : 's'}.
					</p>
				</div>
				<div class="space-y-3 px-5 py-4 text-sm leading-6 text-[var(--color-surface-text-muted)]">
					<p>No metadata will be changed automatically. Results will populate here for review before you apply selections.</p>
					<p>{topMatchOnly ? 'Only the top match will be kept for each book.' : 'Multiple candidates may be shown for each book.'}</p>
				</div>
				<div class="flex flex-wrap items-center justify-end gap-2 border-t border-[var(--color-surface-border)] px-5 py-4">
					<button
						type="button"
						onclick={() => showLookupConfirm = false}
						disabled={queueingLookup}
						class="rounded-lg border border-[var(--color-surface-border)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-700)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
					>
						Cancel
					</button>
					<button
						type="button"
						onclick={queueLookupJob}
						disabled={queueingLookup}
						class="accent-action rounded-lg px-4 py-2 text-sm font-medium transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:hover:translate-y-0 disabled:hover:shadow-none"
					>
						{queueingLookup ? 'Queueing...' : 'Start Lookup'}
					</button>
				</div>
			</div>
		</div>
	{/if}
</div>

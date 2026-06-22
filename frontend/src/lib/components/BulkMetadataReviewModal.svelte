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
		genres?: string[];
		match_score?: number;
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
		status: string;
		error?: string;
		query?: string;
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
		error?: string;
	};

	interface Props {
		jobId: number;
		initialJob?: AdminJob | null;
		onClose: () => void;
		onApplied?: () => Promise<void> | void;
	}

	let { jobId, initialJob = null, onClose, onApplied }: Props = $props();

	let job = $state<AdminJob | null>(null);
	let selected = $state<Set<number>>(new Set());
	let loading = $state(true);
	let applying = $state(false);
	let applyMessage = $state('');
	let includeCover = $state(true);
	let appliedBookIds = $state<Set<number>>(new Set());
	let pollTimer: number | null = null;
	let selectionTouched = false;

	const items = $derived((job?.result?.items ?? []).filter((item) => !appliedBookIds.has(item.book_id)));
	const matchedItems = $derived(items.filter((item) => item.match));
	const selectedItems = $derived(items.filter((item) => item.match && selected.has(item.book_id)));

	function authors(value: string[] | undefined): string {
		return value?.filter(Boolean).join(', ') || '-';
	}

	function score(value: number | undefined): string {
		if (value === undefined || value === null) return '-';
		return String(Math.round(value));
	}

	function toggleSelected(bookId: number) {
		selectionTouched = true;
		const next = new Set(selected);
		if (next.has(bookId)) {
			next.delete(bookId);
		} else {
			next.add(bookId);
		}
		selected = next;
	}

	function selectAllMatches() {
		selectionTouched = true;
		selected = new Set(matchedItems.map((item) => item.book_id));
	}

	function clearSelection() {
		selectionTouched = true;
		selected = new Set();
	}

	function syncDefaultSelection() {
		if (!selectionTouched && matchedItems.length > 0) {
			selected = new Set(matchedItems.map((item) => item.book_id));
		}
	}

	async function loadJob(silent = false) {
		if (!silent) loading = true;
		try {
			const res = await fetch(`/api/jobs/${jobId}`);
			if (res.ok) {
				job = await res.json();
				syncDefaultSelection();
			}
		} finally {
			loading = false;
		}
	}

	function startPolling() {
		if (pollTimer) window.clearInterval(pollTimer);
		pollTimer = window.setInterval(async () => {
			await loadJob(true);
			if (job && job.status !== 'queued' && job.status !== 'running') {
				if (pollTimer) window.clearInterval(pollTimer);
				pollTimer = null;
			}
		}, 2500);
	}

	async function applyItems(itemsToApply: LookupItem[]) {
		const validItems = itemsToApply.filter((item) => item.match);
		if (validItems.length === 0) return;
		if (!confirmBulkAction({ action: 'apply metadata to', count: validItems.length })) return;

		applying = true;
		applyMessage = '';
		let pendingJob: string | null = null;
		try {
			const payload = validItems.map((item) => ({
				book_id: item.book_id,
				metadata: {
					...item.match,
					cover_url: includeCover ? item.match?.cover_url : ''
				}
			}));

			const endpoint = payload.length === 1 ? '/api/metadata/apply' : '/api/jobs/metadata-apply';
			const body = payload.length === 1
				? { book_id: payload[0].book_id, metadata: payload[0].metadata }
				: { items: payload, include_cover: includeCover };
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

			const next = new Set(selected);
			for (const item of validItems) next.delete(item.book_id);
			selectionTouched = true;
			selected = next;
			const applied = new Set(appliedBookIds);
			for (const item of validItems) applied.add(item.book_id);
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
		reviewedMetadataLookupJobs.mark(jobId);
		job = initialJob;
		loading = !initialJob;
		syncDefaultSelection();
		void loadJob(!!initialJob);
		if (!job || job.status === 'queued' || job.status === 'running') {
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
		<header class="flex items-center justify-between gap-4 border-b border-[var(--color-surface-border)] px-6 py-4">
			<div>
				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Review Metadata Matches</h2>
				<p class="text-sm text-[var(--color-surface-text-muted)]">
					{job?.title || 'Bulk metadata lookup'}
				</p>
			</div>
			<div class="flex items-center gap-2">
				{#if job}
					<span class="rounded-full border border-[var(--color-surface-border)] px-3 py-1 text-xs uppercase tracking-normal text-[var(--color-surface-text-muted)]">
						{job.status}
					</span>
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
		{:else if !job}
			<div class="px-6 py-12 text-center text-sm text-[var(--color-surface-text-muted)]">Unable to load this job.</div>
		{:else}
			<div class="border-b border-[var(--color-surface-border)] px-6 py-3">
				<div class="flex flex-wrap items-center justify-between gap-3">
					<div class="text-sm text-[var(--color-surface-text-muted)]">
						{job.completed_items} / {job.total_items} checked, {matchedItems.length} match{matchedItems.length === 1 ? '' : 'es'}, {job.failed_items} failed
					</div>
					<div class="flex flex-wrap items-center gap-2">
						<label class="flex items-center gap-2 rounded-md px-2 py-1 text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)]">
							<input type="checkbox" bind:checked={includeCover} class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]" />
							Update covers
						</label>
						<button type="button" class="rounded-md border border-[var(--color-surface-border)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-base)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]" onclick={selectAllMatches}>
							Select matches
						</button>
						<button type="button" class="rounded-md border border-[var(--color-surface-border)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-base)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]" onclick={clearSelection}>
							Clear
						</button>
						<button
							type="button"
							class="rounded-md bg-[var(--color-primary-500)] px-3 py-1.5 text-sm font-medium text-white transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-primary-600)] hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
							onclick={() => applyItems(selectedItems)}
							disabled={applying || selectedItems.length === 0}
						>
							{applying ? 'Applying...' : `Apply top match (${selectedItems.length})`}
						</button>
					</div>
				</div>
				{#if applyMessage}
					<div class="mt-2 text-sm text-[var(--color-surface-text-muted)]">{applyMessage}</div>
				{/if}
			</div>

			<div class="min-h-0 flex-1 overflow-y-auto p-4">
				{#if items.length === 0}
					<div class="rounded-lg border border-dashed border-[var(--color-surface-border)] px-6 py-12 text-center text-sm text-[var(--color-surface-text-muted)]">
						{appliedBookIds.size > 0 ? 'All visible matches have been applied.' : 'No lookup results yet.'}
					</div>
				{:else}
					<div class="space-y-3">
						{#each items as item}
							<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4 transition-all duration-200 ease-out hover:border-[var(--color-surface-500)] hover:bg-[var(--color-surface-700)]/60 hover:shadow-sm">
								<div class="mb-3 flex items-center justify-between gap-3">
									<label class="flex min-w-0 items-center gap-3">
										<input
											type="checkbox"
											checked={selected.has(item.book_id)}
											disabled={!item.match}
											onchange={() => toggleSelected(item.book_id)}
											class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)] disabled:opacity-40"
										/>
										<span class="truncate text-sm font-medium text-[var(--color-surface-text)]">
											{item.current.title || `Book ${item.book_id}`}
										</span>
									</label>
									<div class="flex items-center gap-2">
										<span class="rounded-full border border-[var(--color-surface-border)] px-2 py-1 text-xs text-[var(--color-surface-text-muted)]">
											{item.match ? `Score ${score(item.match.match_score)}` : item.status}
										</span>
										<button
											type="button"
											class="rounded-md border border-[var(--color-surface-border)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-overlay)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
											onclick={() => applyItems([item])}
											disabled={applying || !item.match}
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
										<div class="mb-3 text-xs font-semibold uppercase tracking-normal text-[var(--color-surface-text-muted)]">Top Match</div>
										{#if item.match}
											<div class="flex gap-4">
												<div class="h-32 w-24 shrink-0 overflow-hidden rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
													{#if item.match.cover_url}
														<img src={item.match.cover_url} alt={item.match.title} class="h-full w-full object-cover" />
													{:else}
														<div class="flex h-full items-center justify-center px-2 text-center text-xs text-[var(--color-surface-text-muted)]">No cover</div>
													{/if}
												</div>
												<div class="min-w-0 text-sm">
													<div class="font-semibold text-[var(--color-surface-text)]">{item.match.title || '-'}</div>
													<div class="mt-1 text-[var(--color-surface-text-muted)]">{authors(item.match.authors)}</div>
													<div class="mt-3 grid gap-1 text-[var(--color-surface-text-muted)] sm:grid-cols-2">
														<div>Series: {item.match.series || '-'}</div>
														<div>Publisher: {item.match.publisher || '-'}</div>
														<div>Published: {item.match.pub_date || '-'}</div>
														<div>ISBN: {item.match.isbn || '-'}</div>
														<div>ASIN: {item.match.asin || '-'}</div>
														<div>Provider: {item.match.provider || '-'}</div>
														<div>Pages: {item.match.page_count || '-'}</div>
													</div>
												</div>
											</div>
										{:else}
											<div class="flex h-32 items-center justify-center rounded-md border border-dashed border-[var(--color-surface-border)] text-sm text-[var(--color-surface-text-muted)]">
												{item.error || 'No match found'}
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
</div>

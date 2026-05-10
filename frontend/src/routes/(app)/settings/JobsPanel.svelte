<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import BulkMetadataReviewModal from '$lib/components/BulkMetadataReviewModal.svelte';
	import JobListItem from '$lib/components/JobListItem.svelte';
	import { notificationVisualIndicator } from '$lib/stores';

	type AdminJob = {
		id: number;
		job_type: string;
		title: string;
		status: string;
		total_items: number;
		completed_items: number;
		failed_items: number;
		payload?: Record<string, any>;
		result?: Record<string, any>;
		error?: string;
		created_at: number;
		started_at?: number;
		completed_at?: number;
		cancel_requested_at?: number;
	};

	type AdminNotification = {
		id: number;
		source?: 'notification' | 'job' | 'log';
		kind: string;
		title: string;
		message?: string;
		url?: string;
		read_at?: number;
		created_at: number;
		job?: AdminJob;
	};

	type JobDetailItem = {
		path?: string;
		title?: string;
		status: string;
		error?: string;
		book_id?: number;
	};

	const activeStatuses = ['queued', 'running', 'cancelling'];
	const typeOptions = [
		{ value: '', label: 'All types' },
		{ value: 'library_scan', label: 'Library scans' },
		{ value: 'cover_regenerate', label: 'Covers' },
		{ value: 'metadata_lookup,metadata_apply', label: 'Metadata' },
		{ value: 'database_backup', label: 'Backups' },
		{ value: 'auth', label: 'User login/logout' },
		{ value: 'logs', label: 'App events' }
	];

	let activeJobs = $state<AdminJob[]>([]);
	let jobs = $state<AdminJob[]>([]);
	let events = $state<AdminNotification[]>([]);
	let unreadCount = $state(0);
	let loading = $state(false);
	let error = $state('');
	let jobStatus = $state('');
	let jobType = $state('');
	let jobsExpanded = $state(false);
	let eventsExpanded = $state(false);
	let reviewJob = $state<AdminJob | null>(null);
	let detailJob = $state<AdminJob | null>(null);
	let detailLoading = $state(false);
	let refreshTimer: number | null = null;

	let historyJobs = $derived(jobs.filter((job) => !activeStatuses.includes(job.status)));
	let visibleEvents = $derived(events.filter((item) => item.source !== 'job'));

	function isEventOnlyFilter(value: string) {
		return ['auth', 'logs'].includes(value);
	}

	function canCancelJob(job: AdminJob): boolean {
		return job.status === 'queued' || (job.status === 'running' && job.job_type === 'library_scan');
	}

	function canReviewJob(job: AdminJob): boolean {
		return job.job_type === 'metadata_lookup' && !activeStatuses.includes(job.status);
	}

	function getJobItems(job: AdminJob | null): JobDetailItem[] {
		const raw = job?.result?.items ?? job?.result?.recent_items ?? [];
		return Array.isArray(raw) ? raw : [];
	}

	function getFailedJobItems(job: AdminJob | null): JobDetailItem[] {
		return getJobItems(job).filter((item) => item.status === 'failed');
	}

	function getSuccessfulJobItems(job: AdminJob | null): JobDetailItem[] {
		return getJobItems(job).filter((item) => item.status !== 'failed');
	}

	function itemLabel(item: JobDetailItem): string {
		return item.path ?? item.title ?? (item.book_id ? `Book ${item.book_id}` : 'Item');
	}

	function formatItemStatus(value: string): string {
		return value.replaceAll('_', ' ').replace(/\b\w/g, (char) => char.toUpperCase());
	}

	function formatTime(value: number) {
		return new Intl.DateTimeFormat(undefined, {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		}).format(new Date(value * 1000));
	}

	function sectionBodyClass(expanded: boolean): string {
		return expanded ? 'max-h-[70vh] overflow-auto' : 'max-h-[28rem] overflow-auto';
	}

	function sectionButtonLabel(expanded: boolean): string {
		return expanded ? 'Show less' : 'Show all';
	}

	function buildJobUrl(activeOnly = false) {
		const params = new URLSearchParams();
		params.set('limit', activeOnly ? '100' : '100');
		if (activeOnly) {
			params.set('status', activeStatuses.join(','));
		} else if (jobStatus.trim()) {
			params.set('status', jobStatus.trim());
		}
		if (!activeOnly && jobType.trim() && !isEventOnlyFilter(jobType)) {
			params.set('type', jobType.trim());
		}
		return `/api/jobs?${params.toString()}`;
	}

	function buildNotificationUrl(format?: 'json' | 'text') {
		const params = new URLSearchParams();
		params.set('limit', '100');
		if (jobType.trim()) params.set('type', jobType.trim());
		if (format && jobStatus.trim()) params.set('status', jobStatus.trim());
		if (format) params.set('format', format);
		return `/api/notifications?${params.toString()}`;
	}

	async function loadJobs(silent = false) {
		if (!silent) {
			loading = activeJobs.length === 0 && jobs.length === 0 && events.length === 0;
		}
		error = '';
		try {
			const activeUrl = buildJobUrl(true);
			const historyUrl = isEventOnlyFilter(jobType) ? null : buildJobUrl(false);
			const eventsUrl = buildNotificationUrl();
			const [activeRes, historyRes, eventsRes] = await Promise.all([
				fetch(activeUrl, { cache: 'no-store' }),
				historyUrl ? fetch(historyUrl, { cache: 'no-store' }) : Promise.resolve(null),
				fetch(eventsUrl, { cache: 'no-store' })
			]);

			if (activeRes.ok) {
				activeJobs = await activeRes.json();
			}
			if (historyRes && historyRes.ok) {
				jobs = await historyRes.json();
			} else if (!historyRes) {
				jobs = [];
			}
			if (eventsRes.ok) {
				const data = await eventsRes.json();
				events = data.items ?? [];
				unreadCount = data.unread_count ?? 0;
			}
		} catch (loadError) {
			console.error('Failed to load jobs:', loadError);
			error = 'Unable to load jobs.';
		} finally {
			loading = false;
		}
	}

	async function cancelJob(job: AdminJob) {
		const action = job.status === 'queued' ? 'Cancel this queued job?' : 'Request cancellation for this running job?';
		if (!confirm(action)) return;
		await fetch(`/api/jobs/${job.id}/cancel`, { method: 'POST' });
		await loadJobs(true);
	}

	async function deleteJob(job: AdminJob) {
		if (!confirm('Delete this job entry?')) return;
		await fetch(`/api/jobs/${job.id}`, { method: 'DELETE' });
		await loadJobs(true);
	}

	function openMetadataReview(job: AdminJob) {
		reviewJob = job;
	}

	async function openJobDetails(job: AdminJob) {
		detailJob = job;
		detailLoading = true;
		try {
			const res = await fetch(`/api/jobs/${job.id}`, { cache: 'no-store' });
			if (res.ok) {
				detailJob = await res.json();
			}
		} catch (detailError) {
			console.error('Failed to load job details:', detailError);
		} finally {
			detailLoading = false;
		}
	}

	async function markNotificationRead(notificationId: number) {
		if (notificationId < 0) return;
		await fetch(`/api/notifications/${notificationId}/read`, { method: 'POST' });
		await loadJobs(true);
	}

	async function deleteNotification(notificationId: number) {
		if (notificationId < 0) return;
		if (!confirm('Delete this notification?')) return;
		await fetch(`/api/notifications/${notificationId}`, { method: 'DELETE' });
		await loadJobs(true);
	}

	async function deleteAllNotifications() {
		if (!confirm('Dismiss all app notifications?')) return;
		await fetch('/api/notifications', { method: 'DELETE' });
		await loadJobs(true);
	}

	function openEvent(item: AdminNotification) {
		if (!item.read_at) {
			void markNotificationRead(item.id);
		}
		if (item.url) {
			goto(item.url);
		}
	}

	onMount(() => {
		notificationVisualIndicator.init();
		void loadJobs();
		refreshTimer = window.setInterval(() => void loadJobs(true), 5000);
		return () => {
			if (refreshTimer) window.clearInterval(refreshTimer);
		};
	});
</script>

<div class="space-y-6">
	<div class="flex flex-wrap items-end justify-between gap-3">
		<div>
			<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Jobs</h2>
			<p class="text-sm text-[var(--color-surface-text-muted)]">Background operations, active progress, and app activity</p>
		</div>
		<button
			onclick={() => loadJobs()}
			class="px-4 py-2 rounded-lg border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors"
		>
			Refresh
		</button>
	</div>

	<section class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] overflow-hidden">
		<div class="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--color-surface-border)] px-6 py-4">
			<div>
				<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Controls</h3>
				<p class="text-sm text-[var(--color-surface-text-muted)]">{unreadCount} active or unread items</p>
				<label class="mt-3 flex items-center gap-3 text-sm text-[var(--color-surface-text-muted)]">
					<input
						type="checkbox"
						checked={$notificationVisualIndicator}
						onchange={(e) => notificationVisualIndicator.set(e.currentTarget.checked)}
						class="h-4 w-4 rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)]"
					>
					<span>Show top-bar indicator for new notifications</span>
				</label>
			</div>
			<div class="flex flex-wrap items-center gap-2">
				<select
					value={jobStatus}
					onchange={async (e) => { jobStatus = e.currentTarget.value; await loadJobs(); }}
					class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)]"
				>
					<option value="">All job statuses</option>
					<option value="queued">Queued</option>
					<option value="running">Running</option>
					<option value="cancelling">Cancelling</option>
					<option value="cancelled">Cancelled</option>
					<option value="completed">Completed</option>
					<option value="failed">Failed</option>
				</select>
				<select
					value={jobType}
					onchange={async (e) => { jobType = e.currentTarget.value; await loadJobs(); }}
					class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)]"
				>
					{#each typeOptions as option}
						<option value={option.value}>{option.label}</option>
					{/each}
				</select>
				<a href={buildNotificationUrl('text')} download class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors">
					Export Text
				</a>
				<a href={buildNotificationUrl('json')} download class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors">
					Export JSON
				</a>
				{#if visibleEvents.some((item) => item.source === 'notification')}
					<button
						onclick={deleteAllNotifications}
						class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:border-red-500/50 hover:text-red-300 transition-colors"
					>
						Dismiss app notifications
					</button>
				{/if}
			</div>
		</div>
	</section>

	<section class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] overflow-hidden">
		<div class="border-b border-[var(--color-surface-border)] px-6 py-4">
			<div class="flex flex-wrap items-center justify-between gap-3">
				<div>
					<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Running Jobs</h3>
					<p class="text-sm text-[var(--color-surface-text-muted)]">Queued, running, and cancelling work stays pinned here.</p>
				</div>
				{#if activeJobs.length > 0}
					<div class="inline-flex items-center gap-2 rounded-full border border-[var(--color-primary-500)]/30 bg-[var(--color-primary-500)]/10 px-3 py-1 text-xs font-medium text-[var(--color-primary-300)]">
						<svg class="h-3.5 w-3.5 animate-scan-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
						</svg>
						{activeJobs.length} active
					</div>
				{/if}
			</div>
		</div>
		<div class="p-6 space-y-3">
			{#if loading}
				<div class="text-sm text-[var(--color-surface-text-muted)]">Loading jobs...</div>
			{:else if error}
				<div class="text-sm text-red-300">{error}</div>
			{:else if activeJobs.length === 0}
				<div class="text-sm text-[var(--color-surface-text-muted)]">No running jobs.</div>
			{:else}
				{#each activeJobs as job}
					<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]/70 p-3">
						<div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start">
							<button type="button" onclick={() => openJobDetails(job)} class="block min-w-0 text-left">
								<JobListItem {job} bare />
							</button>
							<div class="flex flex-wrap gap-2 lg:justify-end">
								<button
									onclick={() => openJobDetails(job)}
									class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]"
								>
									Details
								</button>
								{#if canCancelJob(job)}
									<button
										onclick={() => cancelJob(job)}
										class="rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-200 hover:bg-amber-500/20"
									>
										Cancel
									</button>
								{:else}
									<span class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text-muted)]">
										Active
									</span>
								{/if}
							</div>
						</div>
					</div>
				{/each}
			{/if}
		</div>
	</section>

	<section class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] overflow-hidden">
		<div class="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--color-surface-border)] px-6 py-4">
			<div>
				<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Job History</h3>
				<p class="text-sm text-[var(--color-surface-text-muted)]">Completed, failed, cancelled, and older background work</p>
			</div>
			<button
				onclick={() => jobsExpanded = !jobsExpanded}
				class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:border-[var(--color-primary-500)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] transition-colors"
			>
				{sectionButtonLabel(jobsExpanded)}
			</button>
		</div>
		<div class={sectionBodyClass(jobsExpanded) + ' p-6 space-y-3'}>
			{#if loading}
				<div class="text-sm text-[var(--color-surface-text-muted)]">Loading job history...</div>
			{:else if historyJobs.length === 0}
				<div class="text-sm text-[var(--color-surface-text-muted)]">No matching job history.</div>
			{:else}
				{#each historyJobs as job}
					<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]/70 p-3">
						<div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start">
							<button type="button" onclick={() => openJobDetails(job)} class="block min-w-0 text-left">
								<JobListItem {job} bare />
							</button>
							<div class="flex flex-wrap gap-2 lg:justify-end">
								<button
									onclick={() => openJobDetails(job)}
									class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]"
								>
									Details
								</button>
								{#if canReviewJob(job)}
									<button
										onclick={() => openMetadataReview(job)}
										class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]"
									>
										Review
									</button>
								{/if}
								<button
									onclick={() => deleteJob(job)}
									class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-red-300 hover:border-red-500/50"
								>
									Delete
								</button>
							</div>
						</div>
					</div>
				{/each}
			{/if}
		</div>
	</section>

	<section class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] overflow-hidden">
		<div class="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--color-surface-border)] px-6 py-4">
			<div>
				<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Activity Log</h3>
				<p class="text-sm text-[var(--color-surface-text-muted)]">Notifications and non-job events</p>
			</div>
			<button
				onclick={() => eventsExpanded = !eventsExpanded}
				class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:border-[var(--color-primary-500)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] transition-colors"
			>
				{sectionButtonLabel(eventsExpanded)}
			</button>
		</div>
		<div class={sectionBodyClass(eventsExpanded) + ' p-6 space-y-3'}>
			{#if loading}
				<div class="text-sm text-[var(--color-surface-text-muted)]">Loading activity...</div>
			{:else if visibleEvents.length === 0}
				<div class="text-sm text-[var(--color-surface-text-muted)]">No matching activity.</div>
			{:else}
				{#each visibleEvents as item}
					<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
						<div class="flex items-start justify-between gap-3">
							<button onclick={() => openEvent(item)} class="min-w-0 flex-1 text-left">
								<div class="flex flex-wrap items-center gap-2">
									<span class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">
										{item.source === 'log' ? `event · ${item.kind}` : item.kind}
									</span>
									{#if !item.read_at}
										<span class="rounded-full bg-[var(--color-primary-500)] px-2 py-0.5 text-[10px] font-semibold text-white">New</span>
									{/if}
								</div>
								<div class="mt-1 text-sm font-medium text-[var(--color-surface-text)]">{item.title}</div>
								{#if item.message}
									<div class="mt-1 text-xs text-[var(--color-surface-text-muted)]">{item.message}</div>
								{/if}
								<div class="mt-2 text-xs text-[var(--color-surface-text-muted)]">{formatTime(item.created_at)}</div>
							</button>
							<div class="flex flex-col gap-2">
								{#if item.source === 'log'}
									<span class="rounded-lg border border-[var(--color-surface-border)] px-2 py-1 text-xs text-[var(--color-surface-text-muted)]">
										Event
									</span>
								{:else}
									{#if !item.read_at}
										<button
											onclick={() => markNotificationRead(item.id)}
											class="rounded-lg border border-[var(--color-surface-border)] px-2 py-1 text-xs text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]"
										>
											Read
										</button>
									{/if}
									<button
										onclick={() => deleteNotification(item.id)}
										class="rounded-lg border border-[var(--color-surface-border)] px-2 py-1 text-xs text-red-300 hover:border-red-500/50"
									>
										Dismiss
									</button>
								{/if}
							</div>
						</div>
					</div>
				{/each}
			{/if}
		</div>
	</section>
</div>

{#if reviewJob}
	<BulkMetadataReviewModal
		jobId={reviewJob.id}
		initialJob={reviewJob}
		onClose={() => reviewJob = null}
		onApplied={() => loadJobs(true)}
	/>
{/if}

{#if detailJob}
	<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
		<button
			type="button"
			class="absolute inset-0 bg-black/75"
			aria-label="Close job details"
			onclick={() => detailJob = null}
		></button>
		<div class="relative z-10 flex max-h-[90vh] w-full max-w-4xl flex-col overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
			<div class="flex items-start justify-between gap-4 border-b border-[var(--color-surface-border)] px-6 py-4">
				<div class="min-w-0">
					<h3 class="truncate text-lg font-semibold text-[var(--color-surface-text)]">{detailJob.title}</h3>
					<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">
						{detailJob.status} · {detailJob.completed_items}/{detailJob.total_items} {detailJob.job_type === 'library_scan' ? 'scanned' : 'completed'} · {detailJob.failed_items} failed
					</p>
				</div>
				<button
					type="button"
					onclick={() => detailJob = null}
					class="rounded-lg p-2 text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)]"
					aria-label="Close"
				>
					<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
					</svg>
				</button>
			</div>
			<div class="min-h-0 flex-1 overflow-y-auto p-6">
				{#if detailLoading}
					<div class="text-sm text-[var(--color-surface-text-muted)]">Loading details...</div>
				{:else if getJobItems(detailJob).length === 0}
					<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4 text-sm text-[var(--color-surface-text-muted)]">
						This job only has aggregate totals. Per-item details are recorded for new scan jobs after this update.
					</div>
				{:else}
					<div class="grid gap-4 lg:grid-cols-2">
						<section class="min-w-0 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
							<div class="border-b border-[var(--color-surface-border)] px-4 py-3">
								<h4 class="text-sm font-semibold text-red-300">Failed Items ({getFailedJobItems(detailJob).length})</h4>
							</div>
							<div class="max-h-[50vh] overflow-y-auto p-3">
								{#if getFailedJobItems(detailJob).length === 0}
									<div class="text-sm text-[var(--color-surface-text-muted)]">No failed items.</div>
								{:else}
									<div class="space-y-2">
										{#each getFailedJobItems(detailJob) as item}
											<div class="rounded border border-red-500/20 bg-red-500/5 px-3 py-2">
												<div class="break-all font-mono text-xs text-[var(--color-surface-text)]">{itemLabel(item)}</div>
												{#if item.error}
													<div class="mt-1 text-xs text-red-300">{item.error}</div>
												{/if}
											</div>
										{/each}
									</div>
								{/if}
							</div>
						</section>

						<section class="min-w-0 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]">
							<div class="border-b border-[var(--color-surface-border)] px-4 py-3">
								<h4 class="text-sm font-semibold text-emerald-300">Successful Items ({getSuccessfulJobItems(detailJob).length})</h4>
							</div>
							<div class="max-h-[50vh] overflow-y-auto p-3">
								{#if getSuccessfulJobItems(detailJob).length === 0}
									<div class="text-sm text-[var(--color-surface-text-muted)]">No successful items recorded.</div>
								{:else}
									<div class="space-y-2">
										{#each getSuccessfulJobItems(detailJob) as item}
											<div class="rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2">
												<div class="flex flex-wrap items-start justify-between gap-2">
													<div class="break-all font-mono text-xs text-[var(--color-surface-text)]">{itemLabel(item)}</div>
													<span class="shrink-0 rounded-full border border-[var(--color-surface-border)] px-2 py-0.5 text-[10px] uppercase text-[var(--color-surface-text-muted)]">{formatItemStatus(item.status)}</span>
												</div>
											</div>
										{/each}
									</div>
								{/if}
							</div>
						</section>
					</div>
				{/if}
			</div>
		</div>
	</div>
{/if}

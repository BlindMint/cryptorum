<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import BulkMetadataReviewModal from '$lib/components/BulkMetadataReviewModal.svelte';
	import JobListItem from '$lib/components/JobListItem.svelte';
	import { appActivity, notificationEventPreferences, notificationVisualIndicator } from '$lib/stores';

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

	type ActivityScope = 'all' | 'jobs' | 'events';

	const activeStatuses = ['queued', 'running', 'cancelling'];
	const typeOptions = [
		{ value: '', label: 'All types' },
		{ value: 'library_scan', label: 'Library scans' },
		{ value: 'cover_regenerate', label: 'Covers' },
		{ value: 'metadata_lookup,metadata_apply,bulk_metadata_update', label: 'Metadata' },
		{ value: 'database_backup', label: 'Backups' },
		{ value: 'auth', label: 'User login/logout' },
		{ value: 'logs', label: 'App events' }
	];
	const notificationPreferenceRows: Array<{ key: keyof typeof $notificationEventPreferences; label: string; description: string }> = [
		{ key: 'appNotifications', label: 'App notifications', description: 'Messages created specifically for the notification panel.' },
		{ key: 'completedJobs', label: 'Completed jobs', description: 'Completed metadata jobs that may need review or confirmation.' },
		{ key: 'authEvents', label: 'User login/logout events', description: 'Authentication activity such as successful logins and logouts.' },
		{ key: 'appEvents', label: 'Other app events', description: 'General server activity and informational log events.' }
	];

	let activeJobs = $state<AdminJob[]>([]);
	let events = $state<AdminNotification[]>([]);
	let unreadCount = $state(0);
	let loading = $state(false);
	let error = $state('');
	let jobStatus = $state('');
	let eventType = $state('');
	let activityScope = $state<ActivityScope>('all');
	let searchQuery = $state('');
	let startDate = $state('');
	let endDate = $state('');
	let needsAttention = $state(false);
	let activityExpanded = $state(false);
	let showAdvancedFilters = $state(false);
	let showExportMenu = $state(false);
	let reviewJob = $state<AdminJob | null>(null);
	let detailJob = $state<AdminJob | null>(null);
	let detailLoading = $state(false);
	let refreshTimer: number | null = null;

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

	function dateStartTimestamp(value: string): string {
		if (!value) return '';
		const date = new Date(`${value}T00:00:00`);
		return Number.isNaN(date.getTime()) ? '' : String(Math.floor(date.getTime() / 1000));
	}

	function dateEndTimestamp(value: string): string {
		if (!value) return '';
		const date = new Date(`${value}T23:59:59`);
		return Number.isNaN(date.getTime()) ? '' : String(Math.floor(date.getTime() / 1000));
	}

	function buildJobUrl(activeOnly = false) {
		const params = new URLSearchParams();
		params.set('limit', '100');
		if (activeOnly) {
			params.set('status', activeStatuses.join(','));
		}
		return `/api/jobs?${params.toString()}`;
	}

	function buildActivityUrl(format?: 'json' | 'text') {
		const params = new URLSearchParams();
		params.set('limit', '200');
		if (eventType.trim()) params.set('type', eventType.trim());
		if (jobStatus.trim()) params.set('status', jobStatus.trim());
		if (searchQuery.trim()) params.set('q', searchQuery.trim());
		const start = dateStartTimestamp(startDate);
		const end = dateEndTimestamp(endDate);
		if (start) params.set('start', start);
		if (end) params.set('end', end);
		if (format) params.set('format', format);
		return `/api/notifications?${params.toString()}`;
	}

	function isAttentionItem(item: AdminNotification): boolean {
		if (item.source === 'job') {
			return item.job?.status === 'failed' || (item.job?.failed_items ?? 0) > 0 || canReviewJob(item.job as AdminJob);
		}
		return item.kind.toLowerCase().includes('error') || item.kind.toLowerCase().includes('warn');
	}

	function getActivityItems(): AdminNotification[] {
		return events.filter((item) => {
			if (activityScope === 'jobs' && item.source !== 'job') return false;
			if (activityScope === 'events' && item.source === 'job') return false;
			if (needsAttention && !isAttentionItem(item)) return false;
			return true;
		});
	}

	function clearFilters() {
		activityScope = 'all';
		jobStatus = '';
		eventType = '';
		searchQuery = '';
		startDate = '';
		endDate = '';
		needsAttention = false;
		void loadActivity();
	}

	async function loadActivity(silent = false) {
		if (!silent) {
			loading = activeJobs.length === 0 && events.length === 0;
		}
		error = '';
		try {
			const [activeRes, activityRes] = await Promise.all([
				fetch(buildJobUrl(true), { cache: 'no-store' }),
				fetch(buildActivityUrl(), { cache: 'no-store' })
			]);

			if (activeRes.ok) {
				activeJobs = await activeRes.json();
			}
			if (activityRes.ok) {
				const data = await activityRes.json();
				events = data.items ?? [];
				unreadCount = data.unread_count ?? 0;
			}
		} catch (loadError) {
			console.error('Failed to load activity:', loadError);
			error = 'Unable to load activity.';
		} finally {
			loading = false;
		}
	}

	async function cancelJob(job: AdminJob) {
		const action = job.status === 'queued' ? 'Cancel this queued job?' : 'Request cancellation for this running job?';
		if (!confirm(action)) return;
		await fetch(`/api/jobs/${job.id}/cancel`, { method: 'POST' });
		await loadActivity(true);
	}

	async function deleteJob(job: AdminJob) {
		if (!confirm('Delete this job entry?')) return;
		await fetch(`/api/jobs/${job.id}`, { method: 'DELETE' });
		await loadActivity(true);
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
		await loadActivity(true);
	}

	async function deleteNotification(notificationId: number) {
		if (notificationId < 0) return;
		if (!confirm('Delete this notification?')) return;
		await fetch(`/api/notifications/${notificationId}`, { method: 'DELETE' });
		await loadActivity(true);
	}

	async function deleteAllNotifications() {
		if (!confirm('Dismiss all app notifications?')) return;
		await fetch('/api/notifications', { method: 'DELETE' });
		await loadActivity(true);
	}

	function openEvent(item: AdminNotification) {
		if (!item.read_at) {
			void markNotificationRead(item.id);
		}
		if (item.url) {
			goto(item.url);
		}
	}

	function sectionButtonLabel(expanded: boolean): string {
		return expanded ? 'Show less' : 'Show all';
	}

	onMount(() => {
		notificationVisualIndicator.init();
		notificationEventPreferences.init();
		void loadActivity();
		refreshTimer = window.setInterval(() => void loadActivity(true), 5000);
		return () => {
			if (refreshTimer) window.clearInterval(refreshTimer);
		};
	});
</script>

<div class="space-y-6">
	<div class="flex flex-wrap items-end justify-between gap-3">
		<div>
			<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Activity</h2>
			<p class="text-sm text-[var(--color-surface-text-muted)]">Background operations, notifications, and app events</p>
		</div>
		<button
			onclick={() => loadActivity()}
			class="px-4 py-2 rounded-lg border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors"
		>
			Refresh
		</button>
	</div>

	{#if activeJobs.length > 0}
		<section class="rounded-lg border border-[var(--color-primary-500)]/30 bg-[var(--color-surface-overlay)] overflow-hidden">
			<div class="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--color-surface-border)] px-5 py-3">
				<div>
					<h3 class="text-sm font-semibold text-[var(--color-surface-text)]">Running Jobs</h3>
					<p class="text-xs text-[var(--color-surface-text-muted)]">Queued, running, and cancelling work.</p>
				</div>
				<div class="inline-flex items-center gap-2 rounded-full border border-[var(--color-primary-500)]/30 bg-[var(--color-primary-500)]/10 px-3 py-1 text-xs font-medium text-[var(--color-primary-300)]">
					<svg class="h-3.5 w-3.5 animate-scan-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
					</svg>
					{activeJobs.length} active
				</div>
			</div>
			<div class="space-y-2 p-4">
				{#each activeJobs as job}
					<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]/70 p-3">
						<div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start">
							<button type="button" onclick={() => openJobDetails(job)} class="block min-w-0 text-left">
								<JobListItem {job} bare />
							</button>
							<div class="flex flex-wrap gap-2 lg:justify-end">
								<button onclick={() => openJobDetails(job)} class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]">Details</button>
								{#if canCancelJob(job)}
									<button onclick={() => cancelJob(job)} class="rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-200 hover:bg-amber-500/20">Cancel</button>
								{:else}
									<span class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text-muted)]">Active</span>
								{/if}
							</div>
						</div>
					</div>
				{/each}
			</div>
		</section>
	{/if}

	<section class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] overflow-hidden">
		<div class="space-y-4 border-b border-[var(--color-surface-border)] px-6 py-4">
			<div class="flex flex-wrap items-start justify-between gap-4">
				<div>
					<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Activity Log</h3>
					<p class="text-sm text-[var(--color-surface-text-muted)]">
						Most recent jobs, notifications, and app events · {unreadCount} active or unread
					</p>
				</div>
				<div class="flex flex-wrap items-center gap-2">
					<div class="inline-flex rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-1">
						{#each [{ id: 'all', label: 'All' }, { id: 'jobs', label: 'Jobs' }, { id: 'events', label: 'Events' }] as option}
							<button
								type="button"
								onclick={() => activityScope = option.id as ActivityScope}
								class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors {activityScope === option.id ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
							>
								{option.label}
							</button>
						{/each}
					</div>
					<button onclick={() => activityExpanded = !activityExpanded} class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:border-[var(--color-primary-500)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] transition-colors">
						{sectionButtonLabel(activityExpanded)}
					</button>
				</div>
			</div>

			<div class="grid gap-3 xl:grid-cols-[minmax(16rem,1fr)_minmax(12rem,14rem)_auto]">
				<div class="flex min-w-0 gap-2">
					<input
						type="search"
						bind:value={searchQuery}
						onkeydown={(event) => { if (event.key === 'Enter') void loadActivity(); }}
						placeholder="Search title, message, or job ID..."
						class="min-w-0 flex-1 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
					>
					<button onclick={() => loadActivity()} class="accent-action rounded-lg px-3 py-2 text-sm font-medium transition-colors">Search</button>
				</div>
				<select
					bind:value={eventType}
					onchange={() => loadActivity()}
					class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)]"
				>
					{#each typeOptions as option}
						<option value={option.value} class="bg-[var(--color-surface-base)] text-[var(--color-surface-text)]">{option.label}</option>
					{/each}
				</select>
				<div class="flex flex-wrap items-center gap-2">
					<label class="flex items-center gap-2 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)]">
						<input type="checkbox" bind:checked={needsAttention} class="h-4 w-4 rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)]">
						<span>Needs attention</span>
					</label>
					<button
						onclick={() => showAdvancedFilters = !showAdvancedFilters}
						class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm transition-colors {showAdvancedFilters ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10 text-[var(--color-surface-text)]' : 'text-[var(--color-surface-text-muted)] hover:border-[var(--color-primary-500)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]'}"
					>
						Advanced
					</button>
					<div class="relative">
						<button
							onclick={() => showExportMenu = !showExportMenu}
							class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] transition-colors hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)]"
						>
							Export
						</button>
						{#if showExportMenu}
							<div class="absolute right-0 z-20 mt-2 w-44 overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-xl">
								<a href={buildActivityUrl('text')} download class="block px-3 py-2 text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]" onclick={() => showExportMenu = false}>Export Text</a>
								<a href={buildActivityUrl('json')} download class="block px-3 py-2 text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]" onclick={() => showExportMenu = false}>Export JSON</a>
							</div>
						{/if}
					</div>
				</div>
			</div>

			{#if showAdvancedFilters}
				<div class="grid gap-4 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]/70 p-4 xl:grid-cols-[minmax(0,1fr)_minmax(18rem,24rem)]">
					<div class="space-y-4">
						<div class="grid gap-3 md:grid-cols-3">
							<label class="space-y-1">
								<span class="text-xs font-semibold uppercase tracking-wide text-[var(--color-surface-text-muted)]">Job status</span>
								<select bind:value={jobStatus} class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)]">
									<option value="" class="bg-[var(--color-surface-base)] text-[var(--color-surface-text)]">All statuses</option>
									<option value="queued" class="bg-[var(--color-surface-base)] text-[var(--color-surface-text)]">Queued</option>
									<option value="running" class="bg-[var(--color-surface-base)] text-[var(--color-surface-text)]">Running</option>
									<option value="cancelling" class="bg-[var(--color-surface-base)] text-[var(--color-surface-text)]">Cancelling</option>
									<option value="cancelled" class="bg-[var(--color-surface-base)] text-[var(--color-surface-text)]">Cancelled</option>
									<option value="completed" class="bg-[var(--color-surface-base)] text-[var(--color-surface-text)]">Completed</option>
									<option value="failed" class="bg-[var(--color-surface-base)] text-[var(--color-surface-text)]">Failed</option>
								</select>
							</label>
							<label class="space-y-1">
								<span class="text-xs font-semibold uppercase tracking-wide text-[var(--color-surface-text-muted)]">Start date</span>
								<input type="date" bind:value={startDate} class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)]">
							</label>
							<label class="space-y-1">
								<span class="text-xs font-semibold uppercase tracking-wide text-[var(--color-surface-text-muted)]">End date</span>
								<input type="date" bind:value={endDate} class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)]">
							</label>
						</div>
						<div class="flex flex-wrap items-center gap-2">
							<button onclick={() => loadActivity()} class="accent-action rounded-lg px-3 py-2 text-sm font-medium transition-colors">Apply filters</button>
							<button onclick={clearFilters} class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-overlay)]">Clear filters</button>
							{#if events.some((item) => item.source === 'notification')}
								<button onclick={deleteAllNotifications} class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:border-red-500/50 hover:text-red-300 transition-colors">Dismiss app notifications</button>
							{/if}
						</div>
					</div>

					<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
						<h4 class="text-sm font-semibold text-[var(--color-surface-text)]">Notification Panel</h4>
						<p class="mt-1 text-xs text-[var(--color-surface-text-muted)]">Choose which completed activity also appears in the top-bar notification panel.</p>
						<label class="mt-4 flex items-start gap-3 text-sm text-[var(--color-surface-text-muted)]">
							<input
								type="checkbox"
								checked={$notificationVisualIndicator}
								onchange={(event) => notificationVisualIndicator.set(event.currentTarget.checked)}
								class="mt-0.5 h-4 w-4 rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)]"
							>
							<span>Show top-bar indicator for new notifications</span>
						</label>
						<div class="mt-4 space-y-3">
							{#each notificationPreferenceRows as row}
								<label class="flex items-start gap-3 text-sm">
									<input
										type="checkbox"
										checked={$notificationEventPreferences[row.key]}
										onchange={(event) => { notificationEventPreferences.setKey(row.key, event.currentTarget.checked); void appActivity.refresh(); }}
										class="mt-0.5 h-4 w-4 rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)]"
									>
									<span>
										<span class="block text-[var(--color-surface-text)]">{row.label}</span>
										<span class="block text-xs text-[var(--color-surface-text-muted)]">{row.description}</span>
									</span>
								</label>
							{/each}
						</div>
					</div>
				</div>
			{/if}
		</div>

		<div class={(activityExpanded ? 'max-h-[calc(100vh-18rem)] min-h-[32rem]' : 'max-h-[calc(100vh-22rem)] min-h-[28rem]') + ' overflow-auto p-6 space-y-3'}>
			{#if loading}
				<div class="text-sm text-[var(--color-surface-text-muted)]">Loading activity...</div>
			{:else if error}
				<div class="text-sm text-red-300">{error}</div>
			{:else if getActivityItems().length === 0}
				<div class="text-sm text-[var(--color-surface-text-muted)]">No matching activity.</div>
			{:else}
				{#each getActivityItems() as item (`${item.source}-${item.id}`)}
					{#if item.source === 'job' && item.job}
						<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]/70 p-3 {isAttentionItem(item) ? 'ring-1 ring-amber-500/30' : ''}">
							<div class="mb-2 flex flex-wrap items-center justify-between gap-2">
								<div class="flex flex-wrap items-center gap-2">
									<span class="rounded-full border border-[var(--color-primary-500)]/40 bg-[var(--color-primary-500)]/25 px-2 py-0.5 text-[10px] font-semibold uppercase text-white">Job</span>
									<span class="text-xs text-[var(--color-surface-text-muted)]">ID {item.job.id}</span>
									<span class="text-xs text-[var(--color-surface-text-muted)]">{formatTime(item.created_at)}</span>
								</div>
								{#if isAttentionItem(item)}
									<span class="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-semibold uppercase text-amber-200">Needs attention</span>
								{/if}
							</div>
							<div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start">
								<button type="button" onclick={() => openJobDetails(item.job as AdminJob)} class="block min-w-0 text-left">
									<JobListItem job={item.job} bare />
								</button>
								<div class="flex flex-wrap gap-2 lg:justify-end">
									<button onclick={() => openJobDetails(item.job as AdminJob)} class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]">Details</button>
									{#if canReviewJob(item.job)}
										<button onclick={() => openMetadataReview(item.job as AdminJob)} class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]">Review</button>
									{/if}
									{#if activeStatuses.includes(item.job.status)}
										{#if canCancelJob(item.job)}
											<button onclick={() => cancelJob(item.job as AdminJob)} class="rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-200 hover:bg-amber-500/20">Cancel</button>
										{/if}
									{:else}
										<button onclick={() => deleteJob(item.job as AdminJob)} class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-red-300 hover:border-red-500/50">Delete</button>
									{/if}
								</div>
							</div>
						</div>
					{:else}
						<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
							<div class="flex items-start justify-between gap-3">
								<button onclick={() => openEvent(item)} class="min-w-0 flex-1 text-left">
									<div class="flex flex-wrap items-center gap-2">
										<span class="rounded-full border border-[var(--color-surface-border)] px-2 py-0.5 text-[10px] font-semibold uppercase text-[var(--color-surface-text-muted)]">
											{item.source === 'log' ? 'Event' : 'Notification'}
										</span>
										<span class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">{item.kind}</span>
										{#if !item.read_at}
											<span class="rounded-full bg-[var(--color-primary-500)] px-2 py-0.5 text-[10px] font-semibold text-white">New</span>
										{/if}
										<span class="text-xs text-[var(--color-surface-text-muted)]">{formatTime(item.created_at)}</span>
									</div>
									<div class="mt-1 text-sm font-medium text-[var(--color-surface-text)]">{item.title}</div>
									{#if item.message}
										<div class="mt-1 text-xs text-[var(--color-surface-text-muted)]">{item.message}</div>
									{/if}
								</button>
								<div class="flex flex-col gap-2">
									{#if item.source === 'notification'}
										{#if !item.read_at}
											<button onclick={() => markNotificationRead(item.id)} class="rounded-lg border border-[var(--color-surface-border)] px-2 py-1 text-xs text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]">Read</button>
										{/if}
										<button onclick={() => deleteNotification(item.id)} class="rounded-lg border border-[var(--color-surface-border)] px-2 py-1 text-xs text-red-300 hover:border-red-500/50">Dismiss</button>
									{/if}
								</div>
							</div>
						</div>
					{/if}
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
		onApplied={() => loadActivity(true)}
	/>
{/if}

{#if detailJob}
	<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
		<button type="button" class="absolute inset-0 bg-black/75" aria-label="Close job details" onclick={() => detailJob = null}></button>
		<div class="relative z-10 flex max-h-[90vh] w-full max-w-4xl flex-col overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
			<div class="flex items-start justify-between gap-4 border-b border-[var(--color-surface-border)] px-6 py-4">
				<div class="min-w-0">
					<h3 class="truncate text-lg font-semibold text-[var(--color-surface-text)]">{detailJob.title}</h3>
					<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">
						{detailJob.status} · {detailJob.completed_items}/{detailJob.total_items} {detailJob.job_type === 'library_scan' ? 'scanned' : 'completed'} · {detailJob.failed_items} failed
					</p>
				</div>
				<button type="button" onclick={() => detailJob = null} class="rounded-lg p-2 text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)]" aria-label="Close">
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
						This job only has aggregate totals. Per-item details are recorded for newer jobs where item-level results are available.
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

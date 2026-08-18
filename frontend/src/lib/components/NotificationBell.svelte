<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import JobListItem from './JobListItem.svelte';
	import BulkMetadataReviewModal from './BulkMetadataReviewModal.svelte';
	import { appActivity, notificationVisualIndicator, reviewedMetadataLookupJobs } from '$lib/stores';

	type NotificationItem = {
		id: number;
		source?: 'notification' | 'job' | 'log';
		kind: string;
		title: string;
		message?: string;
		url?: string;
		read_at?: number;
		created_at: number;
		job?: JobItem;
	};

	type JobItem = {
		id: number;
		job_type: string;
		title: string;
		status: string;
		total_items: number;
		completed_items: number;
		failed_items: number;
		payload?: Record<string, any>;
		result?: Record<string, any>;
		created_at: number;
	};

	let open = $state(false);
	let reviewJob = $state<JobItem | null>(null);
	let notifications = $derived($appActivity.notifications as NotificationItem[]);
	let activeJobs = $derived($appActivity.activeJobs as JobItem[]);
	const runningJobStatuses = ['starting', 'queued', 'running', 'cancelling'];
	let runningJobCount = $derived(activeJobs.filter((job) => runningJobStatuses.includes(job.status)).length);
	let hasRecentReviewableMetadataJob = $derived(
		notifications.some((item) =>
			item.source === 'job' &&
			item.job?.job_type === 'metadata_lookup' &&
			item.job.status === 'completed' &&
			!$reviewedMetadataLookupJobs.has(item.job.id) &&
			Date.now() / 1000 - item.created_at < 900
		)
	);
	let unreadNotificationCount = $derived(
		notifications.filter((item) => item.source !== 'job' && !item.read_at).length + activeJobs.length
	);
	let hasUnreadNotifications = $derived(unreadNotificationCount > 0);
	let hasActiveJobs = $derived(runningJobCount > 0);
	let buttonRef = $state<HTMLButtonElement | null>(null);
	let panelRef = $state<HTMLDivElement | null>(null);
	let {
		mobileMenu = false,
		panelOnly = false,
		hideHeader = false,
		onClose
	} = $props<{
		mobileMenu?: boolean;
		panelOnly?: boolean;
		hideHeader?: boolean;
		onClose?: () => void;
	}>();

	function formatTime(value: number) {
		return new Intl.DateTimeFormat(undefined, {
			month: 'short',
			day: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		}).format(new Date(value * 1000));
	}

	async function markNotificationRead(notificationId: number) {
		if (notificationId < 0) return;
		await fetch(`/api/notifications/${notificationId}/read`, { method: 'POST' });
		await appActivity.refresh();
	}

	async function removeNotification(notificationId: number) {
		if (notificationId < 0) return;
		await fetch(`/api/notifications/${notificationId}`, { method: 'DELETE' });
		await appActivity.refresh();
	}

	async function dismissAllNotifications() {
		await fetch('/api/notifications', { method: 'DELETE' });
		await appActivity.refresh();
	}

	function closePanel() {
		open = false;
		onClose?.();
	}

	function openMetadataReview(job: JobItem) {
		reviewedMetadataLookupJobs.mark(job.id);
		reviewJob = job;
		closePanel();
	}

	function handleOpen(item: NotificationItem) {
		if (item.job?.job_type === 'metadata_lookup') {
			openMetadataReview(item.job);
			return;
		}
		if (!item.read_at) {
			markNotificationRead(item.id);
		}
		closePanel();
		if (item.url) {
			goto(item.url);
		}
	}

	function handleDocumentClick(event: MouseEvent) {
		if (panelOnly) return;
		const target = event.target as Node;
		if (!open) return;
		if (buttonRef?.contains(target) || panelRef?.contains(target)) return;
		closePanel();
	}

	onMount(() => {
		if (panelOnly) {
			open = true;
		}
		notificationVisualIndicator.init();
		reviewedMetadataLookupJobs.init();
		appActivity.init();
		if (!panelOnly) {
			document.addEventListener('click', handleDocumentClick);
		}
		return () => {
			if (!panelOnly) {
				document.removeEventListener('click', handleDocumentClick);
			}
		};
	});
</script>

<div class="relative">
	{#if !panelOnly}
		<button
			bind:this={buttonRef}
			onclick={() => open = !open}
			class={`relative rounded-md text-[var(--color-surface-text-muted)] transition-colors hover:text-[var(--color-surface-text)] ${hasActiveJobs ? '' : 'hover:bg-[var(--color-surface-overlay)]'} ${mobileMenu ? 'flex w-full items-center justify-between gap-3 px-0 py-0' : 'inline-flex h-9 w-9 items-center justify-center p-0'}`}
			title="Notifications"
			aria-label="Notifications"
		>
			{#if mobileMenu}
				<span class="flex items-center gap-3">
					<span class="notification-icon-frame" class:active={hasActiveJobs} class:reviewable={hasRecentReviewableMetadataJob && !hasActiveJobs}>
						<span class="notification-activity-ring" aria-hidden="true"></span>
						<svg class="notification-bell-icon h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C8.67 6.165 7 8.388 7 11v3.159c0 .538-.214 1.055-.595 1.436L5 17h5m5 0a3 3 0 11-6 0m6 0H9"></path>
						</svg>
						{#if $notificationVisualIndicator && hasUnreadNotifications}
							<span class="absolute -right-1 -top-1 h-2.5 w-2.5 rounded-full bg-[var(--color-primary-400)] ring-2 ring-[var(--color-surface-overlay)]"></span>
						{/if}
					</span>
					<span class="text-sm font-medium text-[var(--color-surface-text)]">Notifications</span>
				</span>
				{#if unreadNotificationCount > 0}
					<span class="min-w-5 h-5 px-1 rounded-full bg-[var(--color-primary-500)] text-white text-[10px] font-semibold flex items-center justify-center">{unreadNotificationCount}</span>
				{/if}
			{:else}
				<span class="notification-icon-frame" class:active={hasActiveJobs} class:reviewable={hasRecentReviewableMetadataJob && !hasActiveJobs}>
					<span class="notification-activity-ring" aria-hidden="true"></span>
					<svg class="notification-bell-icon h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C8.67 6.165 7 8.388 7 11v3.159c0 .538-.214 1.055-.595 1.436L5 17h5m5 0a3 3 0 11-6 0m6 0H9"></path>
					</svg>
				</span>
				{#if $notificationVisualIndicator && hasUnreadNotifications}
					<span class="absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full bg-[var(--color-primary-400)] ring-2 ring-[var(--color-surface-overlay)]"></span>
				{/if}
				{#if unreadNotificationCount > 0}
					<span class="absolute -top-0.5 -right-0.5 min-w-5 h-5 px-1 rounded-full bg-[var(--color-primary-500)] text-white text-[10px] font-semibold flex items-center justify-center">{unreadNotificationCount}</span>
				{/if}
			{/if}
		</button>
	{/if}

	{#if open}
		<div
			bind:this={panelRef}
			class={panelOnly
				? 'w-full overflow-hidden'
				: mobileMenu
					? 'fixed left-3 right-3 top-[calc(var(--app-topbar-height)+0.75rem)] max-h-[calc(100dvh-var(--app-topbar-height)-1.5rem)] rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl backdrop-blur-sm overflow-hidden z-[95]'
					: 'absolute right-0 mt-3 w-[28rem] max-w-[calc(100vw-2rem)] rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl backdrop-blur-sm overflow-hidden z-[80]'}
		>
			{#if !hideHeader}
				<div class="px-4 py-3 border-b border-[var(--color-surface-border)] flex items-center justify-between">
					<div>
						<div class="text-sm font-semibold text-[var(--color-surface-text)]">Notifications</div>
						<div class="text-xs text-[var(--color-surface-text-muted)]">{unreadNotificationCount} active or unread</div>
					</div>
					<div class="flex items-center gap-3">
						{#if notifications.some((item) => item.source === 'notification')}
							<button onclick={dismissAllNotifications} class="text-xs text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]">
								Dismiss all
							</button>
						{/if}
						<a href="/settings?tab=activity" class="text-xs text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">Activity</a>
					</div>
				</div>
			{:else}
				<div class="px-4 py-2 border-b border-[var(--color-surface-border)] flex items-center justify-between">
					<div class="text-xs text-[var(--color-surface-text-muted)]">{unreadNotificationCount} active or unread</div>
					<div class="flex items-center gap-3">
						{#if notifications.some((item) => item.source === 'notification')}
							<button onclick={dismissAllNotifications} class="text-xs text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]">
								Dismiss all
							</button>
						{/if}
						<a href="/settings?tab=activity" class="text-xs text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">Activity</a>
					</div>
				</div>
			{/if}
			<div class="{panelOnly ? '' : mobileMenu ? 'max-h-[calc(100dvh-13rem)]' : 'max-h-[32rem]'} overflow-auto">
				{#if activeJobs.length > 0}
					<div class="border-b border-[var(--color-surface-border)] bg-[var(--color-surface-base)]/50 px-4 py-3">
						<div class="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-primary-300)]">
							{#if hasActiveJobs}
								<svg class="h-3.5 w-3.5 animate-scan-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
								</svg>
							{/if}
							{hasActiveJobs ? 'Active Jobs' : 'Recent Jobs'}
						</div>
						<div class="space-y-2">
							{#each activeJobs as job}
								{#if job.job_type === 'metadata_lookup'}
									<button
										type="button"
										class="block w-full rounded-lg text-left transition-all duration-200 ease-out hover:-translate-y-px hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]"
										onclick={() => openMetadataReview(job)}
									>
										<JobListItem {job} compact />
									</button>
								{:else}
									<JobListItem {job} compact />
								{/if}
							{/each}
						</div>
					</div>
				{/if}

				{#if notifications.length === 0 && activeJobs.length === 0}
					<div class="px-4 py-6 text-sm text-[var(--color-surface-text-muted)]">No notifications.</div>
				{:else}
					{#each notifications as item}
						<div class="border-b border-[var(--color-surface-border)] last:border-b-0 px-4 py-3 hover:bg-[var(--color-surface-base)] transition-colors">
							<button class="w-full text-left" onclick={() => handleOpen(item)}>
								<div class="flex items-center justify-between gap-2">
									<span class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">{item.kind}</span>
									{#if !item.read_at}
										<span class="rounded-full bg-[var(--color-primary-500)]/15 px-2 py-0.5 text-[10px] font-semibold text-[var(--color-primary-300)]">New</span>
									{/if}
								</div>
								<div class="mt-1 text-sm font-medium text-[var(--color-surface-text)]">{item.title}</div>
								{#if item.message}
									<div class="mt-1 text-xs text-[var(--color-surface-text-muted)] line-clamp-2">{item.message}</div>
								{/if}
								<div class="mt-2 text-[11px] text-[var(--color-surface-text-muted)]">{formatTime(item.created_at)}</div>
							</button>
							<div class="mt-2 flex justify-end gap-2">
								{#if !item.read_at}
									<button onclick={() => markNotificationRead(item.id)} class="text-xs text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]">Read</button>
								{/if}
								{#if item.source !== 'job' && item.source !== 'log'}
									<button onclick={() => removeNotification(item.id)} class="text-xs text-red-300 hover:text-red-200">Dismiss</button>
								{/if}
							</div>
						</div>
					{/each}
				{/if}
			</div>
		</div>
	{/if}
</div>

{#if reviewJob}
	<BulkMetadataReviewModal
		jobId={reviewJob.id}
		initialJob={reviewJob}
		onClose={() => reviewJob = null}
		onApplied={async () => appActivity.refresh()}
	/>
{/if}

<style>
	.notification-icon-frame {
		position: relative;
		display: grid;
		width: 1.75rem;
		height: 1.75rem;
		flex-shrink: 0;
		place-items: center;
		color: inherit;
	}

	.notification-bell-icon {
		grid-area: 1 / 1;
		z-index: 1;
		transform-origin: center;
		transition:
			color 180ms ease,
			transform 180ms ease;
	}

	.notification-icon-frame.active .notification-bell-icon {
		color: var(--color-primary-400);
		transform: scale(0.9);
	}

	.notification-icon-frame.reviewable .notification-bell-icon {
		color: var(--color-primary-300);
	}

	.notification-activity-ring {
		grid-area: 1 / 1;
		width: 1.75rem;
		height: 1.75rem;
		border: 2px solid color-mix(in srgb, var(--color-surface-border, rgba(55, 65, 81, 0.6)) 70%, transparent);
		border-top-color: var(--color-primary-500, #f97316);
		border-radius: 999px;
		opacity: 0;
		pointer-events: none;
		transform: scale(0.88);
		transition:
			opacity 180ms ease,
			transform 180ms ease;
	}

	.notification-icon-frame.active .notification-activity-ring {
		opacity: 1;
		transform: scale(1);
		animation: notification-activity-spin 1s linear infinite;
	}

	.notification-icon-frame.reviewable .notification-activity-ring {
		opacity: 1;
		border-color: color-mix(in srgb, var(--color-primary-500, #f97316) 45%, transparent);
		transform: scale(1);
		animation: notification-review-pulse 1.8s ease-in-out infinite;
	}

	@keyframes notification-activity-spin {
		from {
			transform: scale(1) rotate(0deg);
		}

		to {
			transform: scale(1) rotate(360deg);
		}
	}

	@keyframes notification-review-pulse {
		0%,
		100% {
			opacity: 0.55;
			transform: scale(0.92);
		}

		50% {
			opacity: 1;
			transform: scale(1.05);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.notification-bell-icon,
		.notification-activity-ring {
			transition: none;
		}

		.notification-icon-frame.active .notification-activity-ring {
			animation: none;
			border-color: color-mix(in srgb, var(--color-primary-500, #f97316) 70%, transparent);
		}

		.notification-icon-frame.reviewable .notification-activity-ring {
			animation: none;
		}
	}
</style>

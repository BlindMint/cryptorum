<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { notificationVisualIndicator } from '$lib/stores';

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
	let notifications = $state<NotificationItem[]>([]);
	let runningJobs = $state<JobItem[]>([]);
	let unreadNotificationCount = $derived(
		notifications.filter((item) => item.source !== 'job' && !item.read_at).length
	);
	let hasUnreadNotifications = $derived(unreadNotificationCount > 0);
	let buttonRef: HTMLButtonElement | null = null;
	let panelRef = $state<HTMLDivElement | null>(null);
	let refreshTimer: number | null = null;
	let {
		mobileMenu = false
	} = $props<{
		mobileMenu?: boolean;
	}>();

	function formatTime(value: number) {
		return new Intl.DateTimeFormat(undefined, {
			month: 'short',
			day: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		}).format(new Date(value * 1000));
	}

	function formatJobDetail(job: JobItem) {
		if (job.status === 'queued') {
			return 'Waiting to start';
		}
		const result = job.result ?? {};
		if (job.job_type === 'library_scan') {
			const imported = result.imported_books ?? 0;
			const total = job.total_items || result.total_files || 0;
			const scanned = job.completed_items || result.scanned_files || 0;
			return total > 0
				? `${imported} books imported · ${scanned}/${total} files scanned`
				: `${imported} books imported · scanning files`;
		}
		if (job.total_items > 0) {
			return `${job.completed_items}/${job.total_items} completed · ${job.failed_items} failed`;
		}
		return job.status;
	}

	function jobStatusLabel(job: JobItem) {
		if (job.status === 'queued') return 'Queued';
		if (job.job_type === 'library_scan' && job.status === 'running') return 'Scanning';
		return job.status;
	}

	function jobStatusClass(job: JobItem) {
		return job.status === 'queued'
			? 'bg-[var(--color-surface-base)] text-[var(--color-surface-text-muted)]'
			: 'bg-[var(--color-primary-500)]/15 text-[var(--color-primary-300)]';
	}

	async function loadNotifications() {
		try {
			const res = await fetch('/api/notifications?limit=20', { cache: 'no-store' });
			if (res.ok) {
				const data = await res.json();
				const items: NotificationItem[] = data.items ?? [];
				runningJobs = items
					.filter((item) => item.source === 'job' && item.job && ['queued', 'running'].includes(item.job.status))
					.map((item) => item.job as JobItem);
				notifications = items.filter((item) => !(item.source === 'job' && item.job && ['queued', 'running'].includes(item.job.status)));
			}
		} catch (error) {
			console.error('Failed to load notifications:', error);
		}
	}

	async function refreshStatus() {
		await loadNotifications();
	}

	async function markNotificationRead(notificationId: number) {
		if (notificationId < 0) return;
		await fetch(`/api/notifications/${notificationId}/read`, { method: 'POST' });
		await loadNotifications();
	}

	async function removeNotification(notificationId: number) {
		if (notificationId < 0) return;
		await fetch(`/api/notifications/${notificationId}`, { method: 'DELETE' });
		await loadNotifications();
	}

	async function dismissAllNotifications() {
		await fetch('/api/notifications', { method: 'DELETE' });
		await loadNotifications();
	}

	function handleOpen(item: NotificationItem) {
		if (!item.read_at) {
			markNotificationRead(item.id);
		}
		open = false;
		if (item.url) {
			goto(item.url);
		}
	}

	function handleDocumentClick(event: MouseEvent) {
		const target = event.target as Node;
		if (!open) return;
		if (buttonRef?.contains(target) || panelRef?.contains(target)) return;
		open = false;
	}

	onMount(() => {
		notificationVisualIndicator.init();
		refreshStatus();
		refreshTimer = window.setInterval(refreshStatus, 5000);
		document.addEventListener('click', handleDocumentClick);
		return () => {
			if (refreshTimer) window.clearInterval(refreshTimer);
			document.removeEventListener('click', handleDocumentClick);
		};
	});
</script>

<div class="relative">
	<button
		bind:this={buttonRef}
		onclick={() => open = !open}
		class={`relative rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] transition-colors ${$notificationVisualIndicator && hasUnreadNotifications ? 'bg-[var(--color-primary-500)]/10 shadow-[0_0_18px_rgba(249,115,22,0.25)]' : ''} ${mobileMenu ? 'flex w-full items-center justify-between gap-3 px-0 py-0' : 'p-2'}`}
		title="Notifications"
		aria-label="Notifications"
	>
		{#if mobileMenu}
			<span class="flex items-center gap-3">
				<span class="relative">
					<svg class="h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C8.67 6.165 7 8.388 7 11v3.159c0 .538-.214 1.055-.595 1.436L5 17h5m5 0a3 3 0 11-6 0m6 0H9"></path>
			</svg>
			{#if $notificationVisualIndicator && hasUnreadNotifications}
				<span class="absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full bg-[var(--color-primary-400)] ring-2 ring-[var(--color-surface-overlay)]"></span>
			{/if}
			{#if unreadNotificationCount > 0}
				<span class="absolute -top-0.5 -right-0.5 min-w-5 h-5 px-1 rounded-full bg-[var(--color-primary-500)] text-white text-[10px] font-semibold flex items-center justify-center">{unreadNotificationCount}</span>
			{/if}
		{/if}
	</button>

	{#if open}
		<div
			bind:this={panelRef}
			class="absolute right-0 mt-3 w-80 rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl backdrop-blur-sm overflow-hidden z-[80]"
		>
			<div class="px-4 py-3 border-b border-[var(--color-surface-border)] flex items-center justify-between">
				<div>
					<div class="text-sm font-semibold text-[var(--color-surface-text)]">Notifications</div>
					<div class="text-xs text-[var(--color-surface-text-muted)]">{unreadNotificationCount} unread</div>
				</div>
				<div class="flex items-center gap-3">
					{#if notifications.length > 0}
						<button onclick={dismissAllNotifications} class="text-xs text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]">
							Dismiss all
						</button>
					{/if}
					<a href="/settings?tab=admin" class="text-xs text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">Admin</a>
				</div>
			</div>
			<div class="max-h-96 overflow-auto">
				{#if runningJobs.length > 0}
					<div class="border-b border-[var(--color-surface-border)] bg-[var(--color-surface-base)]/50 px-4 py-3">
						<div class="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-primary-300)]">
							<span class="h-2 w-2 rounded-full bg-[var(--color-primary-400)]"></span>
							Active Jobs
						</div>
						<div class="space-y-2">
							{#each runningJobs as job}
								<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2">
									<div class="flex items-center justify-between gap-2">
										<div class="truncate text-sm font-medium text-[var(--color-surface-text)]">{job.title}</div>
										<span class="shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase {jobStatusClass(job)}">{jobStatusLabel(job)}</span>
									</div>
									<div class="mt-1 text-xs text-[var(--color-surface-text-muted)]">{formatJobDetail(job)}</div>
									{#if job.status === 'running' && job.total_items > 0}
										<div class="mt-2 h-1.5 overflow-hidden rounded-full bg-[var(--color-surface-700)]">
											<div
												class="h-full rounded-full bg-[var(--color-primary-500)] transition-all"
												style:width={`${Math.min(100, Math.round((job.completed_items / job.total_items) * 100))}%`}
											></div>
										</div>
									{/if}
								</div>
							{/each}
						</div>
					</div>
				{/if}

				{#if notifications.length === 0 && runningJobs.length === 0}
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

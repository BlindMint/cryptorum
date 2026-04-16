<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import BulkMetadataReviewModal from '$lib/components/BulkMetadataReviewModal.svelte';

	type AdminJob = {
		id: number;
		job_type: string;
		title: string;
		status: string;
		total_items: number;
		completed_items: number;
		failed_items: number;
		result?: any;
		error?: string;
		created_at: number;
		started_at?: number;
		completed_at?: number;
	};

	type AdminNotification = {
		id: number;
		kind: string;
		title: string;
		message?: string;
		url?: string;
		read_at?: number;
		created_at: number;
	};

	type AdminLogEntry = {
		id: number;
		level: string;
		category: string;
		message: string;
		data?: any;
		created_at: number;
	};

	type BackupItem = {
		name: string;
		size: number;
		modified_at: number;
		download_url: string;
		restore_url: string;
		delete_url: string;
	};

	type BackupSettings = {
		enabled: boolean;
		cron: string;
		keep_last: number;
	};

	let jobs = $state<AdminJob[]>([]);
	let notifications = $state<AdminNotification[]>([]);
	let unreadCount = $state(0);
	let logs = $state<AdminLogEntry[]>([]);
	let jobsLoading = $state(false);
	let notificationsLoading = $state(false);
	let logsLoading = $state(false);
	let backupsLoading = $state(false);
	let jobError = $state('');
	let notificationError = $state('');
	let logError = $state('');
	let backupError = $state('');

	let jobStatus = $state('');
	let logLevel = $state('');
	let logCategory = $state('');
	let logQuery = $state('');
	let logFrom = $state('');
	let logTo = $state('');
	let backups = $state<BackupItem[]>([]);
	let backupSettings = $state<BackupSettings>({ enabled: true, cron: '', keep_last: 14 });
	let savingBackupSettings = $state(false);
	let creatingBackup = $state(false);
	let coverJobQueueing = $state<'all' | 'missing' | ''>('');
	let coverJobMessage = $state('');
	let backupsExpanded = $state(false);
	let jobsExpanded = $state(false);
	let notificationsExpanded = $state(false);
	let logsExpanded = $state(false);
	let reviewJob = $state<AdminJob | null>(null);

	function formatTime(value: number) {
		return new Intl.DateTimeFormat(undefined, {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		}).format(new Date(value * 1000));
	}

	function formatDateOnly(value: number) {
		return new Intl.DateTimeFormat(undefined, {
			month: 'short',
			day: 'numeric',
			year: 'numeric'
		}).format(new Date(value * 1000));
	}

	function formatDuration(startedAt?: number, completedAt?: number) {
		if (!startedAt) return 'Pending';
		if (!completedAt) return 'In progress';
		const durationMs = Math.max(0, (completedAt - startedAt) * 1000);
		const minutes = Math.floor(durationMs / 60000);
		const seconds = Math.floor((durationMs % 60000) / 1000);
		return `${minutes}m ${seconds}s`;
	}

	function formatFileSize(bytes: number) {
		if (!bytes || bytes <= 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB', 'TB'];
		let size = bytes;
		let unit = 0;
		while (size >= 1024 && unit < units.length - 1) {
			size /= 1024;
			unit += 1;
		}
		const digits = unit === 0 ? 0 : 1;
		return `${size.toFixed(digits)} ${units[unit]}`;
	}

	function statusClass(status: string) {
		switch (status) {
			case 'completed':
				return 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30';
			case 'running':
				return 'bg-blue-500/15 text-blue-300 border-blue-500/30';
			case 'queued':
				return 'bg-amber-500/15 text-amber-300 border-amber-500/30';
			case 'failed':
				return 'bg-red-500/15 text-red-300 border-red-500/30';
			default:
				return 'bg-[var(--color-surface-base)] text-[var(--color-surface-text-muted)] border-[var(--color-surface-border)]';
		}
	}

	function levelClass(level: string) {
		switch (level) {
			case 'error':
				return 'text-red-300 border-red-500/30 bg-red-500/10';
			case 'warn':
				return 'text-amber-300 border-amber-500/30 bg-amber-500/10';
			default:
				return 'text-[var(--color-surface-text)] border-[var(--color-surface-border)] bg-[var(--color-surface-base)]';
		}
	}

	function sectionBodyClass(expanded: boolean): string {
		return expanded ? 'max-h-[70vh] overflow-auto' : 'max-h-[22rem] overflow-auto';
	}

	function sectionButtonLabel(expanded: boolean): string {
		return expanded ? 'Show less' : 'Show all';
	}

	function buildLogExportUrl(format: 'json' | 'text') {
		const params = new URLSearchParams();
		params.set('format', format);
		params.set('limit', '200');
		if (logLevel.trim()) params.set('level', logLevel.trim());
		if (logCategory.trim()) params.set('category', logCategory.trim());
		if (logQuery.trim()) params.set('q', logQuery.trim());
		if (logFrom.trim()) params.set('from', logFrom.trim());
		if (logTo.trim()) params.set('to', logTo.trim());
		return `/api/logs?${params.toString()}`;
	}

	async function loadJobs(silent = false) {
		if (!silent) {
			jobsLoading = true;
		}
		jobError = '';
		try {
			const params = new URLSearchParams();
			params.set('limit', '25');
			if (jobStatus.trim()) params.set('status', jobStatus.trim());
			const res = await fetch(`/api/jobs?${params.toString()}`);
			if (res.ok) {
				jobs = await res.json();
			} else {
				jobError = 'Unable to load jobs.';
			}
		} catch (error) {
			console.error('Failed to load jobs:', error);
			jobError = 'Unable to load jobs.';
		} finally {
			if (!silent) {
				jobsLoading = false;
			}
		}
	}

	async function loadNotifications() {
		notificationsLoading = true;
		notificationError = '';
		try {
			const res = await fetch('/api/notifications?limit=25');
			if (res.ok) {
				const data = await res.json();
				notifications = data.items ?? [];
				unreadCount = data.unread_count ?? 0;
			} else {
				notificationError = 'Unable to load notifications.';
			}
		} catch (error) {
			console.error('Failed to load notifications:', error);
			notificationError = 'Unable to load notifications.';
		} finally {
			notificationsLoading = false;
		}
	}

	async function loadLogs() {
		logsLoading = true;
		logError = '';
		try {
			const params = new URLSearchParams();
			params.set('limit', '100');
			if (logLevel.trim()) params.set('level', logLevel.trim());
			if (logCategory.trim()) params.set('category', logCategory.trim());
			if (logQuery.trim()) params.set('q', logQuery.trim());
			if (logFrom.trim()) params.set('from', logFrom.trim());
			if (logTo.trim()) params.set('to', logTo.trim());
			const res = await fetch(`/api/logs?${params.toString()}`);
			if (res.ok) {
				logs = await res.json();
			} else {
				logError = 'Unable to load logs.';
			}
		} catch (error) {
			console.error('Failed to load logs:', error);
			logError = 'Unable to load logs.';
		} finally {
			logsLoading = false;
		}
	}

	async function loadBackups() {
		backupsLoading = true;
		backupError = '';
		try {
			const res = await fetch('/api/backups');
			if (res.ok) {
				const data = await res.json();
				backups = data.items ?? [];
				backupSettings = { ...backupSettings, ...(data.settings ?? {}) };
			} else {
				backupError = 'Unable to load backups.';
			}
		} catch (error) {
			console.error('Failed to load backups:', error);
			backupError = 'Unable to load backups.';
		} finally {
			backupsLoading = false;
		}
	}

	async function refreshAll() {
		await Promise.all([loadJobs(), loadNotifications(), loadLogs(), loadBackups()]);
	}

	async function deleteJob(jobId: number) {
		if (!confirm('Delete this job entry?')) return;
		await fetch(`/api/jobs/${jobId}`, { method: 'DELETE' });
		await loadJobs();
	}

	function openMetadataReview(job: AdminJob) {
		reviewJob = job;
	}

	async function markNotificationRead(notificationId: number) {
		await fetch(`/api/notifications/${notificationId}/read`, { method: 'POST' });
		await loadNotifications();
	}

	async function deleteNotification(notificationId: number) {
		if (!confirm('Delete this notification?')) return;
		await fetch(`/api/notifications/${notificationId}`, { method: 'DELETE' });
		await loadNotifications();
	}

	async function deleteAllNotifications() {
		if (!confirm('Dismiss all notifications?')) return;
		await fetch('/api/notifications', { method: 'DELETE' });
		await loadNotifications();
	}

	async function saveBackupSettings() {
		savingBackupSettings = true;
		backupError = '';
		try {
			const res = await fetch('/api/settings/backups', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					enabled: backupSettings.enabled,
					cron: backupSettings.cron,
					keep_last: backupSettings.keep_last
				})
			});
			if (!res.ok) {
				throw new Error('Failed to save backup settings');
			}
			const data = await res.json();
			backupSettings = data;
		} catch (error) {
			console.error('Failed to save backup settings:', error);
			backupError = 'Unable to save backup settings.';
		} finally {
			savingBackupSettings = false;
		}
	}

	async function createBackupNow() {
		creatingBackup = true;
		backupError = '';
		try {
			const res = await fetch('/api/backups', { method: 'POST' });
			if (!res.ok) {
				throw new Error('Failed to create backup');
			}
			await refreshAll();
		} catch (error) {
			console.error('Failed to create backup:', error);
			backupError = 'Unable to queue backup.';
		} finally {
			creatingBackup = false;
		}
	}

	async function queueCoverJob(mode: 'all' | 'missing') {
		coverJobQueueing = mode;
		jobError = '';
		coverJobMessage = '';
		try {
			const res = await fetch('/api/settings/book-covers/regenerate', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ mode })
			});
			if (!res.ok) {
				throw new Error('Failed to queue cover job');
			}
			coverJobMessage = mode === 'all'
				? 'Cover regeneration job queued.'
				: 'Missing cover regeneration job queued.';
			await Promise.all([loadJobs(), loadLogs(), loadNotifications()]);
		} catch (error) {
			console.error('Failed to queue cover job:', error);
			jobError = 'Unable to queue cover regeneration.';
		} finally {
			coverJobQueueing = '';
			setTimeout(() => coverJobMessage = '', 4000);
		}
	}

	async function restoreBackup(item: BackupItem) {
		if (!confirm(`Restore backup "${item.name}"? This will replace the live database.`)) return;
		try {
			const res = await fetch(item.restore_url, { method: 'POST' });
			if (!res.ok) {
				throw new Error('Failed to restore backup');
			}
			await refreshAll();
		} catch (error) {
			console.error('Failed to restore backup:', error);
			backupError = 'Unable to restore backup.';
		}
	}

	async function deleteBackup(item: BackupItem) {
		if (!confirm(`Delete backup "${item.name}"?`)) return;
		try {
			const res = await fetch(item.delete_url, { method: 'DELETE' });
			if (!res.ok) {
				throw new Error('Failed to delete backup');
			}
			await loadBackups();
		} catch (error) {
			console.error('Failed to delete backup:', error);
			backupError = 'Unable to delete backup.';
		}
	}

	function openNotification(item: AdminNotification) {
		if (item.url) {
			goto(item.url);
		}
	}

	onMount(() => {
		refreshAll();
		const interval = setInterval(() => {
			void loadJobs(true);
			void loadNotifications();
		}, 5000);
		return () => clearInterval(interval);
	});
</script>

<div class="space-y-6">
	<div class="flex flex-wrap items-end justify-between gap-3">
		<div>
			<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Admin</h2>
			<p class="text-sm text-[var(--color-surface-text-muted)]">Jobs, notifications, and logs for background operations</p>
		</div>
		<button
			onclick={refreshAll}
			class="px-4 py-2 rounded-lg border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors"
		>
			Refresh
		</button>
	</div>

	<section class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] overflow-hidden">
		<div class="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--color-surface-border)] px-6 py-4">
			<div>
				<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Backups</h3>
				<p class="text-sm text-[var(--color-surface-text-muted)]">Manual backups, restore actions, and scheduled backup settings</p>
			</div>
			<div class="flex flex-wrap items-center gap-2">
				<button
					onclick={() => backupsExpanded = !backupsExpanded}
					class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:border-[var(--color-primary-500)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] transition-colors"
				>
					{sectionButtonLabel(backupsExpanded)}
				</button>
				<button
					onclick={createBackupNow}
					disabled={creatingBackup}
					class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors disabled:opacity-50"
				>
					{creatingBackup ? 'Queueing...' : 'Backup Now'}
				</button>
				<button
					onclick={loadBackups}
					class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors"
				>
					Refresh
				</button>
			</div>
		</div>
		<div class="grid gap-4 border-b border-[var(--color-surface-border)] p-6 lg:grid-cols-4">
			<label class="space-y-2 lg:col-span-2">
				<span class="text-sm font-medium text-[var(--color-surface-text)]">Backup Cron</span>
				<input
					type="text"
					bind:value={backupSettings.cron}
					placeholder="0 4 * * 1"
					class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)]"
				>
			</label>
			<label class="space-y-2">
				<span class="text-sm font-medium text-[var(--color-surface-text)]">Automatic Backups</span>
				<div class="flex items-center gap-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2">
					<input
						type="checkbox"
						bind:checked={backupSettings.enabled}
						class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
					>
					<span class="text-sm text-[var(--color-surface-text)]">Enabled</span>
				</div>
			</label>
			<label class="space-y-2">
				<span class="text-sm font-medium text-[var(--color-surface-text)]">Keep Last</span>
				<input
					type="number"
					min="1"
					bind:value={backupSettings.keep_last}
					class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)]"
				>
			</label>
		</div>
		<div class="flex flex-wrap items-center justify-between gap-3 px-6 py-4 border-b border-[var(--color-surface-border)]">
			<div class="text-xs text-[var(--color-surface-text-muted)]">
				Backups are stored in the server data directory and can be restored from this panel.
			</div>
			<button
				onclick={saveBackupSettings}
				disabled={savingBackupSettings}
				class="rounded-lg bg-[var(--color-primary-500)] px-4 py-2 text-sm text-white hover:opacity-90 transition-opacity disabled:opacity-50"
			>
				{savingBackupSettings ? 'Saving...' : 'Save Backup Settings'}
			</button>
		</div>
		<div class={sectionBodyClass(backupsExpanded) + ' p-6 space-y-3'}>
			{#if backupsLoading}
				<div class="text-sm text-[var(--color-surface-text-muted)]">Loading backups...</div>
			{:else if backupError}
				<div class="text-sm text-red-300">{backupError}</div>
			{:else if backups.length === 0}
				<div class="text-sm text-[var(--color-surface-text-muted)]">No backups yet.</div>
			{:else}
				<div class="grid gap-3 lg:grid-cols-2">
					{#each backups as item}
						<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
							<div class="flex items-start justify-between gap-3">
								<div class="space-y-1">
									<div class="text-sm font-medium text-[var(--color-surface-text)]">{item.name}</div>
									<div class="text-xs text-[var(--color-surface-text-muted)]">
										{formatFileSize(item.size)} · {formatTime(item.modified_at)}
									</div>
								</div>
								<div class="flex flex-wrap gap-2">
									<a
										href={item.download_url}
										class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] transition-colors"
									>
										Download
									</a>
									<button
										onclick={() => restoreBackup(item)}
										class="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-200 hover:bg-amber-500/20 transition-colors"
									>
										Restore
									</button>
									<button
										onclick={() => deleteBackup(item)}
										class="rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-200 hover:bg-red-500/20 transition-colors"
									>
										Delete
									</button>
								</div>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</section>

	<div class="grid gap-6 xl:grid-cols-3">
		<section class="xl:col-span-2 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] overflow-hidden">
			<div class="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--color-surface-border)] px-6 py-4">
				<div>
					<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Jobs</h3>
					<p class="text-sm text-[var(--color-surface-text-muted)]">Background metadata and maintenance tasks</p>
				</div>
				<div class="flex flex-wrap items-center gap-2">
					<button
						onclick={() => jobsExpanded = !jobsExpanded}
						class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:border-[var(--color-primary-500)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] transition-colors"
					>
						{sectionButtonLabel(jobsExpanded)}
					</button>
					<button
						onclick={() => queueCoverJob('missing')}
						disabled={coverJobQueueing !== ''}
						class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors disabled:opacity-50"
					>
						{coverJobQueueing === 'missing' ? 'Queueing...' : 'Regenerate Missing Covers'}
					</button>
					<button
						onclick={() => queueCoverJob('all')}
						disabled={coverJobQueueing !== ''}
						class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors disabled:opacity-50"
					>
						{coverJobQueueing === 'all' ? 'Queueing...' : 'Regenerate Covers'}
					</button>
					<select
						value={jobStatus}
						onchange={async (e) => { jobStatus = e.currentTarget.value; await loadJobs(); }}
						class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)]"
					>
						<option value="">All statuses</option>
						<option value="queued">Queued</option>
						<option value="running">Running</option>
						<option value="completed">Completed</option>
						<option value="failed">Failed</option>
					</select>
				</div>
			</div>
			<div class={sectionBodyClass(jobsExpanded) + ' p-6 space-y-4'}>
				{#if coverJobMessage}
					<div class="text-sm text-emerald-300">{coverJobMessage}</div>
				{/if}
				{#if jobsLoading}
					<div class="text-sm text-[var(--color-surface-text-muted)]">Loading jobs...</div>
				{:else if jobError}
					<div class="text-sm text-red-300">{jobError}</div>
				{:else if jobs.length === 0}
					<div class="text-sm text-[var(--color-surface-text-muted)]">No jobs yet.</div>
				{:else}
					<div class="space-y-3">
						{#each jobs as job}
							<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
								<div class="flex items-start justify-between gap-4">
									<div class="space-y-2">
										<div class="flex flex-wrap items-center gap-2">
											<span class="rounded-full border px-2.5 py-1 text-xs font-medium {statusClass(job.status)}">{job.status}</span>
											<span class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">{job.job_type}</span>
										</div>
										<div class="text-sm font-medium text-[var(--color-surface-text)]">{job.title}</div>
										<div class="text-xs text-[var(--color-surface-text-muted)]">
											{job.completed_items}/{job.total_items} completed · {job.failed_items} failed
											{#if job.started_at || job.completed_at}
												· {formatDuration(job.started_at, job.completed_at)}
											{/if}
										</div>
										<div class="text-xs text-[var(--color-surface-text-muted)]">
											Created {formatTime(job.created_at)}
										</div>
										{#if job.error}
											<div class="text-xs text-red-300">{job.error}</div>
										{/if}
									</div>
									<div class="flex shrink-0 items-center gap-2">
										{#if job.job_type === 'metadata_lookup' && job.status !== 'queued' && job.status !== 'running'}
											<button
												onclick={() => openMetadataReview(job)}
												class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] transition-colors"
											>
												Review
											</button>
										{/if}
										<button
											onclick={() => deleteJob(job.id)}
											class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text-muted)] hover:border-red-500/50 hover:text-red-300 transition-colors"
										>
											Delete
										</button>
									</div>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</section>

		<section class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] overflow-hidden">
			<div class="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--color-surface-border)] px-6 py-4">
				<div>
					<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Notifications</h3>
					<p class="text-sm text-[var(--color-surface-text-muted)]">{unreadCount} unread</p>
				</div>
				<div class="flex items-center gap-2">
					{#if notifications.length > 0}
						<button
							onclick={deleteAllNotifications}
							class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:border-red-500/50 hover:text-red-300 transition-colors"
						>
							Dismiss all
						</button>
					{/if}
					<button
						onclick={() => notificationsExpanded = !notificationsExpanded}
						class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:border-[var(--color-primary-500)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] transition-colors"
					>
						{sectionButtonLabel(notificationsExpanded)}
					</button>
				</div>
			</div>
			<div class={sectionBodyClass(notificationsExpanded) + ' p-6 space-y-3'}>
				{#if notificationsLoading}
					<div class="text-sm text-[var(--color-surface-text-muted)]">Loading notifications...</div>
				{:else if notificationError}
					<div class="text-sm text-red-300">{notificationError}</div>
				{:else if notifications.length === 0}
					<div class="text-sm text-[var(--color-surface-text-muted)]">No notifications yet.</div>
				{:else}
					{#each notifications as item}
						<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
							<div class="flex items-start justify-between gap-3">
								<button onclick={() => openNotification(item)} class="text-left flex-1">
									<div class="flex items-center gap-2">
										<span class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">{item.kind}</span>
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
								</div>
							</div>
						</div>
					{/each}
				{/if}
			</div>
		</section>
	</div>

		<section class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] overflow-hidden">
			<div class="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--color-surface-border)] px-6 py-4">
				<div>
					<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Logs</h3>
					<p class="text-sm text-[var(--color-surface-text-muted)]">Searchable app events with export</p>
				</div>
				<div class="flex flex-wrap items-center gap-2">
					<button
						onclick={() => logsExpanded = !logsExpanded}
						class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:border-[var(--color-primary-500)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] transition-colors"
					>
						{sectionButtonLabel(logsExpanded)}
					</button>
					<a href={buildLogExportUrl('text')} download class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors">
						Export Text
					</a>
				<a href={buildLogExportUrl('json')} download class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors">
					Export JSON
				</a>
			</div>
		</div>
		<div class="grid gap-4 border-b border-[var(--color-surface-border)] p-6 lg:grid-cols-5">
			<input
				type="text"
				bind:value={logQuery}
				placeholder="Search message or data"
				class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)]"
			>
			<input
				type="text"
				bind:value={logCategory}
				placeholder="Category"
				class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)]"
			>
			<select bind:value={logLevel} class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)]">
				<option value="">All levels</option>
				<option value="info">Info</option>
				<option value="warn">Warn</option>
				<option value="error">Error</option>
			</select>
			<input
				type="text"
				bind:value={logFrom}
				placeholder="From (date or timestamp)"
				class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)]"
			>
			<div class="flex items-center gap-2">
				<input
					type="text"
					bind:value={logTo}
					placeholder="To (date or timestamp)"
					class="min-w-0 flex-1 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)]"
				>
				<button
					onclick={loadLogs}
					class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors"
				>
					Search
				</button>
			</div>
		</div>
		<div class={sectionBodyClass(logsExpanded) + ' p-6'}>
			{#if logsLoading}
				<div class="text-sm text-[var(--color-surface-text-muted)]">Loading logs...</div>
			{:else if logError}
				<div class="text-sm text-red-300">{logError}</div>
			{:else if logs.length === 0}
				<div class="text-sm text-[var(--color-surface-text-muted)]">No logs found.</div>
			{:else}
				<div class="space-y-3">
					{#each logs as item}
						<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
							<div class="flex flex-wrap items-center justify-between gap-3">
								<div class="flex flex-wrap items-center gap-2">
									<span class="rounded-full border px-2.5 py-1 text-xs font-medium {levelClass(item.level)}">{item.level}</span>
									<span class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">{item.category}</span>
								</div>
								<span class="text-xs text-[var(--color-surface-text-muted)]">{formatTime(item.created_at)}</span>
							</div>
							<div class="mt-2 text-sm text-[var(--color-surface-text)]">{item.message}</div>
							{#if item.data}
								<pre class="mt-3 overflow-auto rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-3 text-xs text-[var(--color-surface-text-muted)]">{JSON.stringify(item.data, null, 2)}</pre>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</section>
</div>

{#if reviewJob}
	<BulkMetadataReviewModal
		jobId={reviewJob.id}
		initialJob={reviewJob}
		onClose={() => reviewJob = null}
		onApplied={loadJobs}
	/>
{/if}

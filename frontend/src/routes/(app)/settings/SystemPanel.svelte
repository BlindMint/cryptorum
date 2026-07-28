<script lang="ts">
	import { onMount } from 'svelte';
	import { appActivity } from '$lib/stores';

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

	let backupsLoading = $state(false);
	let backupError = $state('');
	let backups = $state<BackupItem[]>([]);
	let backupSettings = $state<BackupSettings>({ enabled: true, cron: '', keep_last: 14 });
	let savingBackupSettings = $state(false);
	let creatingBackup = $state(false);
	let backupsExpanded = $state(false);

	function formatTime(value: number) {
		return new Intl.DateTimeFormat(undefined, {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		}).format(new Date(value * 1000));
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

	function sectionBodyClass(expanded: boolean): string {
		return expanded ? 'min-h-[16rem]' : 'min-h-[16rem] md:min-h-0 md:flex-1 md:overflow-auto';
	}

	function sectionButtonLabel(expanded: boolean): string {
		return expanded ? 'Show less' : 'Show all';
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
		const pendingJob = appActivity.startPendingJob({
			job_type: 'database_backup',
			title: 'Database backup',
			total_items: 1,
			payload: { trigger: 'manual' }
		});
		try {
			const res = await fetch('/api/backups', { method: 'POST' });
			if (!res.ok) {
				appActivity.failPendingJob(pendingJob, 'Unable to queue backup.');
				throw new Error('Failed to create backup');
			}
			const job = await res.json();
			if (job?.id) {
				appActivity.confirmPendingJob(pendingJob, job);
			} else {
				appActivity.confirmPendingJob(pendingJob);
			}
			await appActivity.refresh();
			await loadBackups();
		} catch (error) {
			console.error('Failed to create backup:', error);
			appActivity.failPendingJob(pendingJob, 'Unable to queue backup.');
			backupError = 'Unable to queue backup.';
		} finally {
			creatingBackup = false;
		}
	}

	async function restoreBackup(item: BackupItem) {
		if (!confirm(`Restore backup "${item.name}"? This will replace the live database.`)) return;
		try {
			const res = await fetch(item.restore_url, { method: 'POST' });
			if (!res.ok) {
				throw new Error('Failed to restore backup');
			}
			await loadBackups();
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

	onMount(loadBackups);
</script>

<div class="flex min-h-full flex-1 flex-col">
	<section class="flex min-h-[24rem] flex-1 flex-col rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)]">
		<div class="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--color-surface-border)] px-6 py-4">
			<div>
				<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Backups</h3>
				<p class="text-sm text-[var(--color-surface-text-muted)]">Manual backups, restore actions, and scheduled backup settings</p>
			</div>
			<div class="flex flex-wrap items-center gap-2">
				<button
					type="button"
					onclick={loadBackups}
					disabled={backupsLoading}
					class="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-[var(--color-surface-border)] text-[var(--color-surface-text-muted)] transition-colors hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)] disabled:cursor-wait disabled:opacity-60"
					title="Refresh backups"
					aria-label="Refresh backups"
				>
					<svg class="h-4 w-4 {backupsLoading ? 'animate-spin' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
					</svg>
				</button>
				<button
					onclick={() => backupsExpanded = !backupsExpanded}
					class="hidden rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text-muted)] transition-colors hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)] md:inline-flex"
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
				class="accent-action rounded-lg px-4 py-2 text-sm font-medium transition-colors"
			>
				{savingBackupSettings ? 'Saving...' : 'Save Backup Settings'}
			</button>
		</div>
		<div class={sectionBodyClass(backupsExpanded) + ' flex flex-col p-6 space-y-3'}>
			{#if backupsLoading}
				<div class="flex min-h-[12rem] flex-1 items-center justify-center text-sm text-[var(--color-surface-text-muted)]">Loading backups...</div>
			{:else if backupError}
				<div class="text-sm text-red-300">{backupError}</div>
			{:else if backups.length === 0}
				<div class="flex min-h-[12rem] flex-1 items-center justify-center text-sm text-[var(--color-surface-text-muted)]">No backups yet.</div>
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
</div>

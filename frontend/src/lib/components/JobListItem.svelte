<script lang="ts">
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
		error?: string;
		created_at: number;
		started_at?: number;
		completed_at?: number;
		cancel_requested_at?: number;
	};

	let { job, compact = false, bare = false } = $props<{
		job: JobItem;
		compact?: boolean;
		bare?: boolean;
	}>();

	const activeStatuses = ['starting', 'queued', 'running', 'cancelling'];

	function isActive(status: string) {
		return activeStatuses.includes(status);
	}

	function formatJobType(value: string) {
		switch (value) {
			case 'library_scan':
				return 'Library Scan';
			case 'cover_regenerate':
				return 'Covers';
			case 'metadata_lookup':
				return 'Metadata Lookup';
			case 'metadata_apply':
				return 'Metadata Apply';
			case 'bulk_metadata_update':
				return 'Bulk Metadata';
			case 'database_backup':
				return 'Backup';
			default:
				return value.replaceAll('_', ' ').replace(/\b\w/g, (char) => char.toUpperCase());
		}
	}

	function statusClass(status: string) {
		switch (status) {
			case 'starting':
				return 'bg-[var(--color-primary-500)]/15 text-[var(--color-primary-300)] border-[var(--color-primary-500)]/30';
			case 'completed':
				return 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30';
			case 'running':
				return 'bg-blue-500/15 text-blue-300 border-blue-500/30';
			case 'cancelling':
				return 'bg-orange-500/15 text-orange-300 border-orange-500/30';
			case 'queued':
				return 'bg-amber-500/15 text-amber-300 border-amber-500/30';
			case 'cancelled':
				return 'bg-[var(--color-surface-base)] text-[var(--color-surface-text-muted)] border-[var(--color-surface-border)]';
			case 'failed':
				return 'bg-red-500/15 text-red-300 border-red-500/30';
			default:
				return 'bg-[var(--color-surface-base)] text-[var(--color-surface-text-muted)] border-[var(--color-surface-border)]';
		}
	}

	function progressPercent(job: JobItem) {
		if (!job.total_items || job.total_items <= 0) return 0;
		return Math.max(0, Math.min(100, Math.round((job.completed_items / job.total_items) * 100)));
	}

	function getLibraryName(job: JobItem) {
		return job.result?.library_name ?? job.payload?.library_name ?? '';
	}

	function getDetail(job: JobItem) {
		const details: string[] = [];
		const libraryName = getLibraryName(job);
		if (libraryName) details.push(libraryName);
		if (job.job_type === 'library_scan') {
			const imported = job.result?.imported_books;
			const phase = job.result?.phase;
			const counts = job.result?.status_counts ?? {};
			if (typeof imported === 'number') details.push(`${imported} imported`);
			if (typeof counts.updated === 'number' && counts.updated > 0) details.push(`${counts.updated} updated`);
			if (typeof counts.unchanged === 'number' && counts.unchanged > 0) details.push(`${counts.unchanged} unchanged`);
			if (typeof counts.skipped === 'number' && counts.skipped > 0) details.push(`${counts.skipped} skipped`);
			if (phase) details.push(phase);
			if (job.total_items > 0) {
				details.push(`${job.completed_items}/${job.total_items} scanned`);
			}
			if (job.failed_items > 0) {
				details.push(`${job.failed_items} failed`);
			}
			return details.join(' · ');
		}
		if (job.job_type === 'cover_regenerate') {
			const updated = job.result?.updated;
			if (typeof updated === 'number') details.push(`${updated} updated`);
		}
		if (job.total_items > 0) {
			details.push(`${job.completed_items}/${job.total_items} completed`);
		}
		if (job.failed_items > 0) {
			details.push(`${job.failed_items} failed`);
		}
		return details.join(' · ');
	}

	function containerClass() {
		return bare
			? ''
			: 'rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)]/70 px-3 py-2';
	}
</script>

<div class={containerClass()}>
	<div class="flex items-start justify-between gap-3">
		<div class="min-w-0 flex-1">
			<div class="flex min-w-0 items-center gap-2">
				{#if isActive(job.status)}
					<svg class="h-4 w-4 shrink-0 animate-scan-spin text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
					</svg>
				{/if}
				<div class="truncate text-sm font-medium text-[var(--color-surface-text)]">{job.title}</div>
			</div>
			<div class="mt-1 flex flex-wrap items-center gap-2">
				<span class="rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase {statusClass(job.status)}">{job.status}</span>
				<span class="text-xs text-[var(--color-surface-text-muted)]">{formatJobType(job.job_type)}</span>
			</div>
			{#if getDetail(job)}
				<div class="mt-1 text-xs text-[var(--color-surface-text-muted)] {compact ? 'line-clamp-2' : 'break-all'}">{getDetail(job)}</div>
			{/if}
			{#if job.error}
				<div class="mt-1 text-xs text-red-300">{job.error}</div>
			{/if}
			{#if job.total_items > 0}
				<div class="mt-2 h-1.5 overflow-hidden rounded-full bg-[var(--color-surface-700)]">
					<div
						class="h-full rounded-full bg-[var(--color-primary-500)] transition-all"
						style:width={`${progressPercent(job)}%`}
					></div>
				</div>
			{/if}
		</div>
		{#if job.total_items > 0}
			<div class="shrink-0 text-xs tabular-nums text-[var(--color-surface-text-muted)]">{progressPercent(job)}%</div>
		{/if}
	</div>
</div>

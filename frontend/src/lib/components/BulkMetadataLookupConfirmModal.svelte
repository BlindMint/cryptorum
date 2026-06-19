<script lang="ts">
	interface Props {
		count: number;
		queueing?: boolean;
		onCancel: () => void;
		onProceed: () => void;
	}

	let { count, queueing = false, onCancel, onProceed }: Props = $props();
</script>

<div class="fixed inset-0 z-[120] flex items-center justify-center bg-black/65 p-4 backdrop-blur-sm">
	<button type="button" class="absolute inset-0" aria-label="Cancel bulk metadata lookup" onclick={onCancel}></button>
	<div class="relative w-full max-w-lg rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
		<div class="border-b border-[var(--color-surface-border)] px-5 py-4">
			<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Bulk Metadata Lookup</h2>
			<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">
				Search and download metadata candidates for {count} selected book{count === 1 ? '' : 's'}.
			</p>
		</div>
		<div class="space-y-3 px-5 py-4 text-sm leading-6 text-[var(--color-surface-text)]">
			<p>
				This queues a background lookup job. You can monitor progress from the notification panel or Settings &gt; Jobs.
			</p>
			<p class="text-[var(--color-surface-text-muted)]">
				No metadata will be changed automatically. When lookup completes, review the matches and apply the top matches you want to keep.
			</p>
		</div>
		<div class="flex flex-wrap items-center justify-end gap-2 border-t border-[var(--color-surface-border)] px-5 py-4">
			<button
				type="button"
				onclick={onCancel}
				disabled={queueing}
				class="rounded-lg border border-[var(--color-surface-border)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-700)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
			>
				Cancel
			</button>
			<button
				type="button"
				onclick={onProceed}
				disabled={queueing}
				class="rounded-lg bg-[var(--color-primary-500)] px-4 py-2 text-sm font-medium text-white transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-primary-600)] hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
			>
				{queueing ? 'Queueing...' : 'Start Lookup'}
			</button>
		</div>
	</div>
</div>

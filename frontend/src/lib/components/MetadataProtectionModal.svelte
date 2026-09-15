<script lang="ts">
	import MetadataProtectionPanel from '$lib/components/MetadataProtectionPanel.svelte';

	interface Props {
		bookId: number;
		lockedFields?: string[];
		libraryProtectionEnabled?: boolean;
		onClose: () => void;
		onChanged?: (fields: string[]) => void | Promise<void>;
		onRestored?: (book: any) => void | Promise<void>;
	}

	let {
		bookId,
		lockedFields = [],
		libraryProtectionEnabled = false,
		onClose,
		onChanged,
		onRestored
	}: Props = $props();
</script>

<div class="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
	<div class="flex max-h-[90dvh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
		<div class="flex items-start justify-between gap-4 border-b border-[var(--color-surface-border)] px-5 py-4">
			<div class="min-w-0">
				<div class="flex items-center gap-2">
					<svg class="h-5 w-5 shrink-0 text-[var(--color-primary-400)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
						<rect x="5" y="10" width="14" height="10" rx="2"></rect>
						<path d="M8 10V7a4 4 0 0 1 8 0v3"></path>
					</svg>
					<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Metadata protection</h2>
				</div>
				<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">
					Control whether scans, lookups, and refreshes can overwrite this book’s metadata.
				</p>
			</div>
			<button type="button" onclick={onClose} class="rounded-lg p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" aria-label="Close">
				<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
				</svg>
			</button>
		</div>

		<div class="min-h-0 flex-1 overflow-y-auto p-5">
			<MetadataProtectionPanel
				{bookId}
				{lockedFields}
				{libraryProtectionEnabled}
				heading={false}
				{onChanged}
				{onRestored}
			/>
		</div>

		<div class="flex items-center justify-end gap-2 border-t border-[var(--color-surface-border)] px-5 py-4">
			<button
				type="button"
				onclick={onClose}
				class="rounded-lg px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)]"
			>
				Done
			</button>
		</div>
	</div>
</div>

<script lang="ts">
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import { getCoverThumbUrl } from '$lib/utils/covers';
	import { getFormatDisplayLabel, uniqueBookFormats } from '$lib/utils/book-formats';
	import { parseAuthors } from '$lib/utils/metadata-edit';

	interface Props {
		book: any;
		files?: any[];
		positionLabel?: string;
		framed?: boolean;
		showCoverActions?: boolean;
		coverActionLabel?: string;
		coverActionBusy?: boolean;
		onEditCover?: () => void;
		onResetCover?: () => void;
	}

	let {
		book,
		files = [],
		positionLabel = '',
		framed = true,
		showCoverActions = false,
		coverActionLabel = 'Edit',
		coverActionBusy = false,
		onEditCover,
		onResetCover
	}: Props = $props();

	function getPrimaryFilePath(): string {
		return files[0]?.path || '';
	}
</script>

<aside class="space-y-4 {framed ? 'rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4' : ''}">
	<div class="relative mx-auto w-full max-w-[12rem]">
		<BookCoverFrame
			src={book?.cover_path ? getCoverThumbUrl(book.id, 'large', book.cover_updated_on) : null}
			alt={book?.title || 'Book cover'}
			format={book?.format}
			mode="contain"
			frameClass="aspect-[2/3] w-full"
			loading="eager"
		/>
		{#if showCoverActions}
			<div class="absolute bottom-2 right-2 z-20 flex gap-1.5">
				{#if book?.cover_source === 'custom' && onResetCover}
					<button
						type="button"
						onclick={onResetCover}
						disabled={coverActionBusy}
						class="rounded-full border border-[var(--color-surface-border)] bg-[var(--color-surface-900)]/90 px-2 py-1 text-xs font-medium text-[var(--color-surface-text)] shadow-lg backdrop-blur transition-colors hover:bg-[var(--color-surface-800)] disabled:opacity-50"
					>
						Reset
					</button>
				{/if}
				{#if onEditCover}
					<button
						type="button"
						onclick={onEditCover}
						disabled={coverActionBusy}
						class="rounded-full bg-[var(--color-primary-500)] px-2 py-1 text-xs font-medium text-white shadow-lg transition-colors hover:bg-[var(--color-primary-600)] disabled:opacity-50"
					>
						{coverActionLabel}
					</button>
				{/if}
			</div>
		{/if}
	</div>

	<div class="space-y-2">
		{#if positionLabel}
			<div class="text-xs font-medium uppercase tracking-[0.12em] text-[var(--color-primary-400)]">{positionLabel}</div>
		{/if}
		<div>
			<h2 class="break-words text-lg font-semibold text-[var(--color-surface-text)]">{book?.title || 'Untitled'}</h2>
			<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">{parseAuthors(book?.authors || '[]').join(', ')}</p>
		</div>
	</div>

	<div class="space-y-3 text-sm">
		{#if uniqueBookFormats(files).length > 0}
			<div>
				<div class="mb-1 text-xs uppercase tracking-[0.12em] text-[var(--color-surface-text-muted)]">Formats</div>
				<div class="flex flex-wrap gap-1.5">
					{#each uniqueBookFormats(files) as format}
						<span class="rounded-full border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-2 py-0.5 text-xs font-medium uppercase text-[var(--color-surface-text)]">
							{getFormatDisplayLabel(format)}
						</span>
					{/each}
				</div>
			</div>
		{/if}

		{#if book?.series}
			<div>
				<div class="text-xs uppercase tracking-[0.12em] text-[var(--color-surface-text-muted)]">Series</div>
				<div class="mt-1 text-[var(--color-surface-text)]">{book.series}</div>
			</div>
		{/if}

		{#if getPrimaryFilePath()}
			<div>
				<div class="text-xs uppercase tracking-[0.12em] text-[var(--color-surface-text-muted)]">Path</div>
				<div class="mt-1 break-all font-mono text-xs text-[var(--color-surface-text)]">{getPrimaryFilePath()}</div>
			</div>
		{/if}
	</div>
</aside>

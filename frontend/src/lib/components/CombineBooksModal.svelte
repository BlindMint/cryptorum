<script lang="ts">
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import { getFormatDisplayLabel } from '$lib/utils/book-formats';
	import {
		combineSelectionError,
		suggestedCombinePrimaryId,
		uniqueLibraryIds,
		type CombineBookSummary
	} from '$lib/utils/combine-books';
	import { getCoverThumbUrl } from '$lib/utils/covers';

	interface Props {
		bookIds: number[];
		onClose: () => void;
		onCombined: (primaryBookId: number) => void;
	}

	let { bookIds, onClose, onCombined }: Props = $props();

	let books = $state<CombineBookSummary[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let primaryBookId = $state<number | null>(null);

	let mixedLibraries = $derived(uniqueLibraryIds(books).length > 1);
	let selectionError = $derived(combineSelectionError(bookIds.length));
	let canSubmit = $derived(
		!loading &&
		!saving &&
		!selectionError &&
		!mixedLibraries &&
		primaryBookId !== null &&
		books.some((book) => book.id === primaryBookId)
	);

	$effect(() => {
		void loadBooks(bookIds);
	});

	async function loadBooks(ids: number[]) {
		loading = true;
		error = '';
		try {
			const loaded: CombineBookSummary[] = [];
			for (const id of ids) {
				const res = await fetch(`/api/books/${id}`, { credentials: 'same-origin' });
				if (!res.ok) {
					error = 'Unable to load one of the selected books.';
					continue;
				}
				const book = await res.json();
				loaded.push({
					id: book.id,
					title: book.title,
					authors: book.authors,
					cover_path: book.cover_path,
					percent: book.percent,
					status: book.status,
					library_id: book.library_id,
					format: book.resume_format
				});
			}
			books = loaded;
			primaryBookId = suggestedCombinePrimaryId(loaded);
			if (!error && uniqueLibraryIds(loaded).length > 1) {
				error = 'Books must belong to the same library.';
			}
		} catch (e) {
			console.error('Failed to load books for combine:', e);
			error = 'Unable to load the selected books.';
		} finally {
			loading = false;
		}
	}

	function parseAuthors(authorsJson?: string): string {
		if (!authorsJson) return '';
		try {
			const parsed = JSON.parse(authorsJson);
			return Array.isArray(parsed) ? parsed.join(', ') : authorsJson;
		} catch {
			return authorsJson;
		}
	}

	function progressLabel(book: CombineBookSummary): string {
		if (book.status === 'reading' || (book.percent || 0) > 0) {
			return `${Math.round(book.percent || 0)}% read`;
		}
		if (book.status === 'finished') return 'Finished';
		return 'Unread';
	}

	async function confirmCombine() {
		if (!canSubmit || primaryBookId === null) return;
		saving = true;
		error = '';
		try {
			const res = await fetch('/api/books/combine', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				credentials: 'same-origin',
				body: JSON.stringify({
					primary_book_id: primaryBookId,
					book_ids: bookIds
				})
			});
			const payload = await res.json().catch(() => ({}));
			if (!res.ok) {
				error = payload.error || 'Failed to combine books.';
				return;
			}
			onCombined(payload.primary_book_id || primaryBookId);
		} catch (e) {
			console.error('Failed to combine books:', e);
			error = 'Failed to combine books.';
		} finally {
			saving = false;
		}
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && !saving) onClose();
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="fixed inset-0 z-[60] flex items-center justify-center p-4">
	<button type="button" class="absolute inset-0 bg-black/60 backdrop-blur-sm" aria-label="Close combine books" onclick={() => { if (!saving) onClose(); }}></button>
	<div class="relative flex max-h-[90dvh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
		<div class="flex items-start justify-between gap-4 border-b border-[var(--color-surface-border)] px-5 py-4">
			<div>
				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Combine books</h2>
				<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">
					Choose the primary book. The others become extra files on that book and are removed from the library list. Files on disk are not deleted.
				</p>
			</div>
			<button type="button" onclick={onClose} class="rounded-lg p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" aria-label="Close">
				<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
				</svg>
			</button>
		</div>

		<div class="min-h-0 flex-1 overflow-y-auto p-5">
			{#if loading}
				<div class="flex items-center justify-center py-10 text-[var(--color-surface-text-muted)]">
					<svg class="mr-2 h-5 w-5 animate-spin text-[var(--color-primary-500)]" viewBox="0 0 24 24" fill="none">
						<circle class="opacity-25" cx="12" cy="12" r="9" stroke="currentColor" stroke-width="2"></circle>
						<path class="opacity-75" fill="currentColor" d="M12 3a9 9 0 0 1 9 9h-2.5a6.5 6.5 0 0 0-6.5-6.5V3z"></path>
					</svg>
					Loading selected books...
				</div>
			{:else}
				<div class="space-y-2">
					{#each books as book (book.id)}
						<label class="flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-3 transition-colors {primaryBookId === book.id ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10' : 'border-[var(--color-surface-border)] hover:border-[var(--color-surface-500)]'}">
							<input
								type="radio"
								name="combine-primary"
								class="accent-[var(--color-primary-500)]"
								checked={primaryBookId === book.id}
								onchange={() => primaryBookId = book.id}
							/>
							<BookCoverFrame
								src={book.cover_path ? getCoverThumbUrl(book.id, 'small') : null}
								alt={book.title || 'book'}
								format={book.format}
								mode="cover"
								frameClass="h-16 w-12 flex-shrink-0"
								placeholderSize="sm"
							/>
							<div class="min-w-0 flex-1">
								<div class="truncate text-sm font-medium text-[var(--color-surface-text)]">{book.title || 'Untitled'}</div>
								<div class="truncate text-xs text-[var(--color-surface-text-muted)]">{parseAuthors(book.authors) || 'Unknown author'}</div>
								<div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-[var(--color-surface-text-muted)]">
									{#if book.format}
										<span class="rounded-full bg-[var(--color-surface-700)] px-2 py-0.5 uppercase">{getFormatDisplayLabel(book.format)}</span>
									{/if}
									<span>{progressLabel(book)}</span>
								</div>
							</div>
							{#if primaryBookId === book.id}
								<span class="flex-shrink-0 text-xs font-medium text-[var(--color-primary-400)]">Primary</span>
							{/if}
						</label>
					{/each}
				</div>
			{/if}
			{#if error || selectionError}
				<p class="mt-4 text-sm text-red-400">{error || selectionError}</p>
			{/if}
		</div>

		<div class="flex items-center justify-end gap-2 border-t border-[var(--color-surface-border)] px-5 py-4">
			<button
				type="button"
				onclick={onClose}
				class="rounded-lg px-4 py-2 text-sm text-[var(--color-surface-text-muted)] transition-colors hover:text-[var(--color-surface-text)]"
			>
				Cancel
			</button>
			<button
				type="button"
				onclick={confirmCombine}
				disabled={!canSubmit}
				class="accent-action rounded-lg px-4 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50"
			>
				{saving ? 'Combining…' : 'Combine'}
			</button>
		</div>
	</div>
</div>

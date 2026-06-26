<script lang="ts">
	import AutocompleteInput from '$lib/components/AutocompleteInput.svelte';
	import { appActivity } from '$lib/stores';
	import { confirmBulkAction } from '$lib/utils/bulk-confirm';
	import { metadataStatusOptions, ratingStars, starsToRating } from '$lib/utils/metadata-edit';

	interface Props {
		bookIds: number[];
		filterParams?: Record<string, unknown> | null;
		selectionCount?: number;
		onClose: () => void;
		onSaved?: (job?: any) => void;
	}

	let { bookIds, filterParams = null, selectionCount = bookIds.length, onClose, onSaved }: Props = $props();

	let authors = $state('');
	let publisher = $state('');
	let language = $state('');
	let series = $state('');
	let seriesNumber = $state('');
	let status = $state('');
	let rating = $state(0);
	let hoveredRating = $state(0);
	let addTags = $state('');
	let removeTags = $state('');
	let clearFields = $state<Set<string>>(new Set());
	let saving = $state(false);
	let error = $state('');
	let queuedMessage = $state('');

	function toggleClear(field: string) {
		const next = new Set(clearFields);
		if (next.has(field)) next.delete(field);
		else next.add(field);
		clearFields = next;
	}

	function commaValues(value: string): string[] {
		return value.split(',').map((item) => item.trim()).filter(Boolean);
	}

	function setPayloadField(payload: Record<string, unknown>, key: string, value: string, transform: (value: string) => unknown = (v) => v.trim()) {
		if (value.trim()) payload[key] = transform(value);
	}

	function setRating(value: number) {
		rating = rating === value ? 0 : value;
	}

	function isRatingFilled(star: number): boolean {
		const activeRating = hoveredRating || rating;
		return star <= activeRating;
	}

	async function save(closeAfter: boolean) {
		if (saving || (bookIds.length === 0 && !filterParams)) return;
		if (!confirmBulkAction({ action: 'update metadata for', count: selectionCount })) return;
		saving = true;
		error = '';
		queuedMessage = '';
		const payload: Record<string, unknown> = {
			clear_fields: Array.from(clearFields)
		};
		if (filterParams) payload.filter = filterParams;
		else payload.book_ids = bookIds;
		setPayloadField(payload, 'authors', authors, commaValues);
		setPayloadField(payload, 'publisher', publisher);
		setPayloadField(payload, 'language', language);
		setPayloadField(payload, 'series', series);
		setPayloadField(payload, 'series_number', seriesNumber);
		if (status) payload.status = status;
		if (rating > 0) payload.rating = starsToRating(rating);
		setPayloadField(payload, 'add_tags', addTags, commaValues);
		setPayloadField(payload, 'remove_tags', removeTags, commaValues);

		const pendingJob = appActivity.startPendingJob({
			job_type: 'bulk_metadata_update',
			title: `Bulk metadata update (${selectionCount} books)`,
			total_items: selectionCount
		});
		try {
			const res = await fetch('/api/books/bulk-metadata', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(payload)
			});
			if (!res.ok) {
				error = await res.text();
				appActivity.failPendingJob(pendingJob, 'Unable to queue bulk metadata update.');
				return;
			}
			const job = await res.json().catch(() => null);
			if (job?.id) {
				appActivity.confirmPendingJob(pendingJob, job);
			} else {
				appActivity.confirmPendingJob(pendingJob);
			}
			await appActivity.refresh();
			queuedMessage = 'Bulk metadata update queued. You can monitor progress from the notification panel or Settings > Jobs.';
			onSaved?.(job);
			if (closeAfter) onClose();
		} catch (e) {
			console.error('Failed to save bulk metadata:', e);
			error = 'Failed to save bulk metadata.';
			appActivity.failPendingJob(pendingJob, 'Unable to queue bulk metadata update.');
		} finally {
			saving = false;
		}
	}
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
	<div class="flex max-h-[90dvh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
		<div class="flex items-start justify-between gap-4 border-b border-[var(--color-surface-border)] px-5 py-4">
			<div>
				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Edit Metadata in Bulk</h2>
				<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">{selectionCount} selected books</p>
			</div>
			<button type="button" onclick={onClose} class="rounded-lg p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" aria-label="Close">
				<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
				</svg>
			</button>
		</div>

		<div class="min-h-0 flex-1 overflow-y-auto p-5">
			<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
				<label class="space-y-1">
					<span class="block text-sm text-[var(--color-surface-text-muted)]">Authors</span>
					<input bind:value={authors} placeholder="Author One, Author Two" class="w-full rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
				</label>
				<div class="flex items-end gap-2">
					<label class="min-w-0 flex-1 space-y-1">
						<span class="block text-sm text-[var(--color-surface-text-muted)]">Publisher</span>
						<AutocompleteInput id="bulk-publisher" bind:value={publisher} field="publishers" multiple={false} onchange={(value) => publisher = value} />
					</label>
					<button type="button" onclick={() => toggleClear('publisher')} class="mb-0.5 rounded-lg border px-3 py-2 text-sm transition-colors {clearFields.has('publisher') ? 'border-red-400 bg-red-500/20 text-red-200' : 'border-[var(--color-surface-border)] text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}">Clear</button>
				</div>

				<div class="flex items-end gap-2">
					<label class="min-w-0 flex-1 space-y-1">
						<span class="block text-sm text-[var(--color-surface-text-muted)]">Language</span>
						<AutocompleteInput id="bulk-language" bind:value={language} field="languages" multiple={false} onchange={(value) => language = value} />
					</label>
					<button type="button" onclick={() => toggleClear('language')} class="mb-0.5 rounded-lg border px-3 py-2 text-sm transition-colors {clearFields.has('language') ? 'border-red-400 bg-red-500/20 text-red-200' : 'border-[var(--color-surface-border)] text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}">Clear</button>
				</div>
				<div class="flex items-end gap-2">
					<label class="min-w-0 flex-1 space-y-1">
						<span class="block text-sm text-[var(--color-surface-text-muted)]">Series</span>
						<AutocompleteInput id="bulk-series" bind:value={series} field="series" multiple={false} onchange={(value) => series = value} />
					</label>
					<input bind:value={seriesNumber} placeholder="#" class="mb-0.5 w-20 rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
					<button type="button" onclick={() => toggleClear('series')} class="mb-0.5 rounded-lg border px-3 py-2 text-sm transition-colors {clearFields.has('series') ? 'border-red-400 bg-red-500/20 text-red-200' : 'border-[var(--color-surface-border)] text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}">Clear</button>
				</div>

				<div class="space-y-1">
					<span class="block text-sm text-[var(--color-surface-text-muted)]">Status</span>
					<div class="grid grid-cols-2 gap-2 sm:grid-cols-4" role="radiogroup" aria-label="Bulk status">
						<button
							type="button"
							onclick={() => status = ''}
							class="rounded-lg border px-3 py-2 text-sm font-medium transition-all duration-200 ease-out hover:-translate-y-px focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] {status === '' ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)] text-white shadow-sm' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-700)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-600)] hover:shadow-sm'}"
						>
							No change
						</button>
						{#each metadataStatusOptions as option}
							<button
								type="button"
								onclick={() => status = option.value}
								class="rounded-lg border px-3 py-2 text-sm font-medium transition-all duration-200 ease-out hover:-translate-y-px focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] {status === option.value ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)] text-white shadow-sm' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-700)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-600)] hover:shadow-sm'}"
							>
								{option.label}
							</button>
						{/each}
					</div>
				</div>
				<div class="flex items-end gap-2">
					<div class="min-w-0 flex-1 space-y-1">
						<span class="block text-sm text-[var(--color-surface-text-muted)]">Rating</span>
						<div class="inline-flex max-w-full items-center gap-0.5 whitespace-nowrap rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2" role="radiogroup" aria-label="Rating" tabindex="-1" onmouseleave={() => hoveredRating = 0}>
							{#each ratingStars as star}
								<button
									type="button"
									onmouseenter={() => hoveredRating = star}
									onfocus={() => hoveredRating = star}
									onblur={() => hoveredRating = 0}
									onclick={() => setRating(star)}
									class="text-lg transition-all duration-200 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-yellow-400/40 focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] {isRatingFilled(star) ? 'text-yellow-400' : 'text-[var(--color-surface-600)]'} hover:-translate-y-px hover:scale-110"
									aria-label={rating === star ? 'Clear rating' : `Set rating to ${star} star${star === 1 ? '' : 's'}`}
								>
									&#9733;
								</button>
							{/each}
						</div>
					</div>
					<button type="button" onclick={() => toggleClear('rating')} class="mb-0.5 rounded-lg border px-3 py-2 text-sm transition-colors {clearFields.has('rating') ? 'border-red-400 bg-red-500/20 text-red-200' : 'border-[var(--color-surface-border)] text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}">Clear</button>
				</div>

				<label class="space-y-1">
					<span class="block text-sm text-[var(--color-surface-text-muted)]">Add Tags</span>
					<AutocompleteInput id="bulk-add-tags" bind:value={addTags} field="tags" onchange={(value) => addTags = value} />
				</label>
				<div class="flex items-end gap-2">
					<label class="min-w-0 flex-1 space-y-1">
						<span class="block text-sm text-[var(--color-surface-text-muted)]">Remove Tags</span>
						<AutocompleteInput id="bulk-remove-tags" bind:value={removeTags} field="tags" onchange={(value) => removeTags = value} />
					</label>
					<button type="button" onclick={() => toggleClear('tags')} class="mb-0.5 rounded-lg border px-3 py-2 text-sm transition-colors {clearFields.has('tags') ? 'border-red-400 bg-red-500/20 text-red-200' : 'border-[var(--color-surface-border)] text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}">Clear All</button>
				</div>
			</div>

			{#if error}
				<div class="mt-4 rounded-lg border border-red-500/50 bg-red-500/20 p-3 text-sm text-red-300">{error}</div>
			{/if}
			{#if queuedMessage}
				<div class="mt-4 rounded-lg border border-[var(--color-primary-500)]/40 bg-[var(--color-primary-500)]/10 p-3 text-sm text-[var(--color-surface-text)]">{queuedMessage}</div>
			{/if}
		</div>

		<div class="flex flex-wrap items-center justify-end gap-2 border-t border-[var(--color-surface-border)] px-5 py-4">
			<button type="button" onclick={onClose} disabled={saving} class="rounded-lg border border-[var(--color-surface-border)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)] disabled:opacity-50">Cancel</button>
			<button type="button" onclick={() => save(false)} disabled={saving} class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-600)] disabled:opacity-50">{saving ? 'Saving...' : 'Save'}</button>
			<button type="button" onclick={() => save(true)} disabled={saving} class="rounded-lg bg-[var(--color-primary-500)] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[var(--color-primary-600)] disabled:opacity-50">{saving ? 'Saving...' : 'Save & Close'}</button>
		</div>
	</div>
</div>

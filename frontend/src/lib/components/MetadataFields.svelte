<script lang="ts">
	import AutocompleteInput from '$lib/components/AutocompleteInput.svelte';
	import type { MetadataEditForm } from '$lib/utils/metadata-edit';
	import { metadataStatusOptions, prepareAuthorRows, ratingStars, removeAuthorRow, setAuthorRowValue } from '$lib/utils/metadata-edit';

	interface Props {
		editForm: MetadataEditForm;
		authorsList: string[];
		showDescription?: boolean;
		showStatus?: boolean;
		columns?: boolean;
	}

	let {
		editForm = $bindable(),
		authorsList = $bindable(),
		showDescription = true,
		showStatus = true,
		columns = true
	}: Props = $props();

	let hoveredRating = $state(0);

	function addAuthor() {
		authorsList = [...authorsList, ''];
	}

	function removeAuthor(index: number) {
		authorsList = removeAuthorRow(authorsList, index);
	}

	function updateAuthor(index: number, value: string) {
		authorsList = setAuthorRowValue(authorsList, index, value);
	}

	function setRating(value: number) {
		editForm.rating = editForm.rating === value ? 0 : value;
	}

	function isRatingFilled(star: number): boolean {
		const activeRating = hoveredRating || editForm.rating;
		return star <= activeRating;
	}

	function statusButtonClass(status: string): string {
		if (editForm.status === status) {
			if (status === 'reading') return 'border-blue-500/50 bg-blue-500/15 text-blue-300 shadow-sm';
			if (status === 'finished') return 'border-emerald-500/50 bg-emerald-500/15 text-emerald-300 shadow-sm';
			return 'border-[var(--color-primary-500)]/55 bg-[var(--color-primary-500)]/15 text-[var(--color-primary-400)] shadow-sm';
		}
		return 'border-[var(--color-surface-border)] bg-[var(--color-surface-700)] text-[var(--color-surface-text-muted)] hover:-translate-y-0.5 hover:bg-[var(--color-surface-600)] hover:text-[var(--color-surface-text)] hover:shadow-sm';
	}

	$effect(() => {
		authorsList = prepareAuthorRows(authorsList);
	});
</script>

<div class={columns ? 'grid grid-cols-1 gap-4 md:grid-cols-2' : 'space-y-4'}>
	{#if showStatus}
		<div>
			<div class="mb-2 block text-sm text-[var(--color-surface-text-muted)]">Status</div>
			<div class="flex gap-2">
				{#each metadataStatusOptions as option}
					<button
						type="button"
						onclick={() => editForm.status = option.value}
						class="flex-1 rounded-lg border px-3 py-2 text-sm font-medium transition-all duration-200 ease-out {statusButtonClass(option.value)} focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
						aria-pressed={editForm.status === option.value}
					>
						{option.label}
					</button>
				{/each}
			</div>
		</div>
	{/if}

	<div>
		<div class="mb-1 block text-sm text-[var(--color-surface-text-muted)]">Rating</div>
		<div class="inline-flex max-w-full items-center gap-0.5 whitespace-nowrap" role="radiogroup" aria-label="Rating" tabindex="-1" onmouseleave={() => hoveredRating = 0}>
			{#each ratingStars as star}
				<button
					type="button"
					onmouseenter={() => hoveredRating = star}
					onfocus={() => hoveredRating = star}
					onblur={() => hoveredRating = 0}
					onclick={() => setRating(star)}
					class="text-xl transition-all duration-200 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-yellow-400/40 focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] {isRatingFilled(star) ? 'text-yellow-400' : 'text-[var(--color-surface-600)]'} hover:-translate-y-0.5 hover:scale-110"
					aria-label={editForm.rating === star ? 'Clear rating' : `Set rating to ${star} star${star === 1 ? '' : 's'}`}
				>
					&#9733;
				</button>
			{/each}
		</div>
	</div>

	<div>
		<label class="mb-1 block text-sm text-[var(--color-surface-text-muted)]" for="book-title">Title</label>
		<input id="book-title" type="text" bind:value={editForm.title} placeholder="Untitled book" class="w-full rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
	</div>

	<div>
		<div class="mb-2 block text-sm text-[var(--color-surface-text-muted)]">Authors</div>
		<div class="space-y-2">
			{#each authorsList as author, i}
				<div class="flex items-center space-x-2">
					<div class="min-w-0 flex-1">
						<AutocompleteInput
							id={`book-author-${i}`}
							bind:value={authorsList[i]}
							placeholder="Author name"
							field="authors"
							multiple={false}
							onchange={(value) => updateAuthor(i, value)}
						/>
					</div>
					<button
						type="button"
						onclick={() => removeAuthor(i)}
						class="group rounded-md p-2 text-red-400 transition-all duration-200 ease-out hover:-translate-y-0.5 hover:bg-red-500/10 hover:text-red-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400/40"
						title="Remove author"
					>
						<svg class="h-4 w-4 transition-transform duration-200 ease-out group-hover:scale-110" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
						</svg>
					</button>
				</div>
			{/each}
			<button
				type="button"
				onclick={addAuthor}
				class="group inline-flex items-center rounded-lg border border-dashed border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-primary-400)] transition-all duration-200 ease-out hover:-translate-y-0.5 hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-primary-300)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
			>
				<svg class="mr-2 h-4 w-4 transition-transform duration-200 ease-out group-hover:scale-110" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
				</svg>
				Add Author
			</button>
		</div>
	</div>

	<div>
		<label class="mb-1 block text-sm text-[var(--color-surface-text-muted)]" for="book-publisher">Publisher</label>
		<AutocompleteInput id="book-publisher" bind:value={editForm.publisher} field="publishers" multiple={false} onchange={(value) => editForm.publisher = value} />
	</div>

	<div>
		<label class="mb-1 block text-sm text-[var(--color-surface-text-muted)]" for="book-pub-date">Published Date</label>
		<input id="book-pub-date" type="text" bind:value={editForm.pub_date} placeholder="YYYY-MM-DD" class="w-full rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
	</div>

	<div>
		<label class="mb-1 block text-sm text-[var(--color-surface-text-muted)]" for="book-language">Language</label>
		<AutocompleteInput id="book-language" bind:value={editForm.language} field="languages" multiple={false} onchange={(value) => editForm.language = value} />
	</div>

	<div>
		<label class="mb-1 block text-sm text-[var(--color-surface-text-muted)]" for="book-isbn">ISBN</label>
		<input id="book-isbn" type="text" bind:value={editForm.isbn} class="w-full rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
	</div>

	<div>
		<label class="mb-1 block text-sm text-[var(--color-surface-text-muted)]" for="book-asin">ASIN</label>
		<input id="book-asin" type="text" bind:value={editForm.asin} class="w-full rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
	</div>

	<div>
		<label class="mb-1 block text-sm text-[var(--color-surface-text-muted)]" for="book-pages">Pages</label>
		<input id="book-pages" type="number" bind:value={editForm.page_count} min="0" class="w-full rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
	</div>

	<div class={columns ? 'md:col-span-2' : ''}>
		<div class="mb-1 block text-sm text-[var(--color-surface-text-muted)]">Series</div>
		<div class="flex gap-2">
			<div class="min-w-0 flex-1">
				<AutocompleteInput id="book-series" bind:value={editForm.series} placeholder="Series name" field="series" multiple={false} onchange={(value) => editForm.series = value} />
			</div>
			<input id="book-series-number" type="text" bind:value={editForm.series_number} placeholder="#" class="w-24 rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
		</div>
	</div>

	<div class={columns ? 'md:col-span-2' : ''}>
		<label class="mb-1 block text-sm text-[var(--color-surface-text-muted)]" for="book-tags">Tags</label>
		<AutocompleteInput id="book-tags" bind:value={editForm.tags} placeholder="Fiction, Science Fiction.Space Opera, Favorite" field="tags" onchange={(value) => editForm.tags = value} />
		<p class="mt-1 text-xs text-[var(--color-surface-text-muted)]">Comma-separated. Use "Parent.Child" for hierarchies.</p>
	</div>

	{#if showDescription}
		<div class={columns ? 'md:col-span-2' : ''}>
			<label class="mb-1 block text-sm text-[var(--color-surface-text-muted)]" for="book-description">Description</label>
			<textarea id="book-description" bind:value={editForm.description} rows="4" class="w-full resize-y rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"></textarea>
		</div>
	{/if}
</div>

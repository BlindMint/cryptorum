<script lang="ts">
	import { lenientSearchMatch } from '$lib/utils/search';

	type FilterMode = 'AND' | 'OR' | 'NOT';
	type FilterOptionSort = 'count' | 'alpha';
	type FilterOption = { name: string; book_count?: number };
	type StatusFilterOption = { value: string; label: string };

	type Props = {
		authors: FilterOption[];
		series: FilterOption[];
		tags: FilterOption[];
		formats: FilterOption[];
		filterMode: FilterMode;
		valueFilterMode: FilterMode;
		hasActiveFilters: boolean;
		onClose: () => void;
		onClear: () => void;
		onSetFilterMode: (mode: FilterMode) => void;
		onSetValueFilterMode: (mode: FilterMode) => void;
		onToggleAuthor: (value: string) => void;
		onToggleSeries: (value: string) => void;
		onToggleTag: (value: string) => void;
		onToggleFormat: (value: string) => void;
		onToggleStatus: (value: string) => void;
		isAuthorSelected: (value: string) => boolean;
		isSeriesSelected: (value: string) => boolean;
		isTagSelected: (value: string) => boolean;
		isFormatSelected: (value: string) => boolean;
		isStatusSelected: (value: string) => boolean;
		showBackdrop?: boolean;
	};

	let {
		authors,
		series,
		tags,
		formats,
		filterMode,
		valueFilterMode,
		hasActiveFilters,
		onClose,
		onClear,
		onSetFilterMode,
		onSetValueFilterMode,
		onToggleAuthor,
		onToggleSeries,
		onToggleTag,
		onToggleFormat,
		onToggleStatus,
		isAuthorSelected,
		isSeriesSelected,
		isTagSelected,
		isFormatSelected,
		isStatusSelected,
		showBackdrop = false
	}: Props = $props();

	const FILTER_MODES: FilterMode[] = ['AND', 'OR', 'NOT'];
	const FILTER_OPTION_SORTS: { value: FilterOptionSort; label: string }[] = [
		{ value: 'alpha', label: 'A-Z' },
		{ value: 'count', label: 'Count' }
	];
	const STATUS_FILTER_OPTIONS: StatusFilterOption[] = [
		{ value: 'unread', label: 'Unread' },
		{ value: 'reading', label: 'Reading' },
		{ value: 'finished', label: 'Finished' }
	];

	let showSettings = $state(false);
	let filterOptionSort = $state<FilterOptionSort>('alpha');
	let authorsOpen = $state(true);
	let seriesOpen = $state(true);
	let tagsOpen = $state(true);
	let formatsOpen = $state(true);
	let statusOpen = $state(true);
	let authorSearch = $state('');
	let seriesSearch = $state('');
	let tagSearch = $state('');
	let formatSearch = $state('');
	let statusSearch = $state('');

	let visibleAuthors = $derived(visibleFilterOptions(authors, authorSearch, filterOptionSort));
	let visibleSeries = $derived(visibleFilterOptions(series, seriesSearch, filterOptionSort));
	let visibleTags = $derived(visibleFilterOptions(tags, tagSearch, filterOptionSort));
	let visibleFormats = $derived(visibleFilterOptions(formats, formatSearch, filterOptionSort));
	let visibleStatuses = $derived(visibleStatusOptions(statusSearch, filterOptionSort));

	function getModeIndex(mode: FilterMode): number {
		return FILTER_MODES.indexOf(mode);
	}

	function sortedFilterOptions<T extends FilterOption>(options: T[], sort: FilterOptionSort): T[] {
		if (sort === 'count') return options;
		return [...options].sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }));
	}

	function visibleFilterOptions<T extends FilterOption>(options: T[], query: string, sort: FilterOptionSort): T[] {
		const trimmed = query.trim();
		const filtered = trimmed
			? options.filter((option) => lenientSearchMatch(trimmed, option.name))
			: options;
		return sortedFilterOptions(filtered, sort);
	}

	function visibleStatusOptions(query: string, sort: FilterOptionSort): StatusFilterOption[] {
		const trimmed = query.trim();
		const filtered = trimmed
			? STATUS_FILTER_OPTIONS.filter((option) => lenientSearchMatch(trimmed, option.label))
			: STATUS_FILTER_OPTIONS;
		if (sort === 'count') return filtered;
		return [...filtered].sort((a, b) => a.label.localeCompare(b.label, undefined, { numeric: true, sensitivity: 'base' }));
	}
</script>

{#if showBackdrop}
	<button
		type="button"
		class="fixed inset-x-0 bottom-0 top-20 z-[35] bg-black/80 lg:bg-black/50"
		aria-label="Close filters"
		onclick={onClose}
	></button>
{/if}

<div class="fixed right-0 top-20 z-40 h-[calc(100dvh-5rem)] w-full max-w-80 translate-x-0 overflow-y-auto border-l border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-xl transition-transform duration-300 ease-out">
	<div class="sticky top-0 z-10 border-b border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)]">
		<div class="flex min-h-16 items-center justify-between gap-3 px-4 py-3">
			<div class="flex items-center gap-2">
				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Filters</h2>
				<button
					type="button"
					onclick={() => showSettings = !showSettings}
					aria-label="Filter settings"
					aria-expanded={showSettings}
					class="inline-flex items-center gap-1 rounded px-1.5 py-1 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)] {showSettings ? 'bg-[var(--color-surface-700)] text-[var(--color-surface-text)]' : ''}"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.607 2.296.07 2.572-1.065z"></path>
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
					</svg>
					<svg class="h-3 w-3 transition-transform {showSettings ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M19 9l-7 7-7-7"></path>
					</svg>
				</button>
			</div>
			<div class="flex items-center gap-2">
				<button
					type="button"
					onclick={onClear}
					disabled={!hasActiveFilters}
					class="rounded px-2 py-1 text-xs font-semibold text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)] disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent disabled:hover:text-[var(--color-surface-text-muted)]"
				>
					Clear
				</button>
				<button
					onclick={onClose}
					aria-label="Close filters"
					class="rounded p-1 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]"
				>
					<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
					</svg>
				</button>
			</div>
		</div>
		{#if showSettings}
			<div class="space-y-3 border-t border-[var(--color-surface-border)] px-4 pb-4 pt-3">
				<div>
					<div class="mb-1.5 text-xs font-semibold uppercase tracking-[0.12em] text-[var(--color-surface-text-muted)]">Across categories</div>
					<div class="relative grid grid-cols-3 rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-1">
						<span
							class="absolute bottom-1 left-1 top-1 rounded-lg bg-[var(--color-primary-500)] shadow-sm transition-transform duration-200 ease-out"
							style="width: calc((100% - 0.5rem) / 3); transform: translateX({getModeIndex(filterMode) * 100}%);"
						></span>
						{#each FILTER_MODES as mode}
							<button
								onclick={() => onSetFilterMode(mode)}
								class="relative z-10 rounded-lg px-2 py-1.5 text-[11px] font-semibold tracking-wide transition-colors {filterMode === mode ? 'text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
							>
								{mode}
							</button>
						{/each}
					</div>
				</div>
				<div>
					<div class="mb-1.5 text-xs font-semibold uppercase tracking-[0.12em] text-[var(--color-surface-text-muted)]">Within categories</div>
					<div class="relative grid grid-cols-3 rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-1">
						<span
							class="absolute bottom-1 left-1 top-1 rounded-lg bg-[var(--color-primary-500)] shadow-sm transition-transform duration-200 ease-out"
							style="width: calc((100% - 0.5rem) / 3); transform: translateX({getModeIndex(valueFilterMode) * 100}%);"
						></span>
						{#each FILTER_MODES as mode}
							<button
								onclick={() => onSetValueFilterMode(mode)}
								class="relative z-10 rounded-lg px-2 py-1.5 text-[11px] font-semibold tracking-wide transition-colors {valueFilterMode === mode ? 'text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
							>
								{mode}
							</button>
						{/each}
					</div>
				</div>
				<div>
					<div class="mb-1.5 text-xs font-semibold uppercase tracking-[0.12em] text-[var(--color-surface-text-muted)]">Filter sort</div>
					<div class="grid grid-cols-2 rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-1">
						{#each FILTER_OPTION_SORTS as option}
							<button
								type="button"
								onclick={() => filterOptionSort = option.value}
								class="rounded-lg px-2 py-1.5 text-xs font-semibold transition-colors {filterOptionSort === option.value ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]'}"
							>
								{option.label}
							</button>
						{/each}
					</div>
				</div>
			</div>
		{/if}
	</div>

	<div class="space-y-2 p-4">
		<div class="overflow-hidden rounded-lg border border-[var(--color-surface-border)]">
			<button onclick={() => authorsOpen = !authorsOpen} class="flex w-full items-center justify-between border-l-4 border-[var(--color-primary-500)]/50 bg-[var(--color-surface-base)] px-4 py-3 transition-colors hover:bg-[var(--color-surface-700)]">
				<span class="text-xs font-bold uppercase tracking-[0.14em] text-[var(--color-surface-text)]">Author</span>
				<div class="flex items-center space-x-2">
					<span class="rounded bg-[var(--color-surface-overlay)] px-2 py-0.5 text-xs text-[var(--color-surface-text-muted)]">{authors.length}</span>
					<svg class="h-4 w-4 text-[var(--color-surface-text-muted)] transition-transform {authorsOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
					</svg>
				</div>
			</button>
			{#if authorsOpen}
				<div class="border-b border-[var(--color-surface-border)] p-2">
					<input type="search" bind:value={authorSearch} placeholder="Search authors" class="h-8 w-full rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 text-sm text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:border-[var(--color-primary-500)] focus:outline-none" />
				</div>
				<div class="max-h-48 overflow-y-auto">
					{#each visibleAuthors as author}
						<button onclick={() => onToggleAuthor(author.name)} class="grid w-full grid-cols-[1rem_minmax(0,1fr)_auto] items-center gap-2 px-4 py-2 text-left text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)] {isAuthorSelected(author.name) ? 'bg-[var(--color-primary-500)]/20' : ''}">
							<span class="flex h-4 w-4 items-center justify-center">
								{#if isAuthorSelected(author.name)}
									<svg class="h-4 w-4 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
									</svg>
								{/if}
							</span>
							<span class="min-w-0 truncate">{author.name}</span>
							<span class="text-xs text-[var(--color-surface-text-muted)]">{author.book_count}</span>
						</button>
					{/each}
					{#if authors.length === 0}
						<p class="px-4 py-2 text-sm text-[var(--color-surface-text-muted)]">No authors found</p>
					{:else if visibleAuthors.length === 0}
						<p class="px-4 py-2 text-sm text-[var(--color-surface-text-muted)]">No matching authors</p>
					{/if}
				</div>
			{/if}
		</div>

		<div class="overflow-hidden rounded-lg border border-[var(--color-surface-border)]">
			<button onclick={() => seriesOpen = !seriesOpen} class="flex w-full items-center justify-between border-l-4 border-[var(--color-primary-500)]/50 bg-[var(--color-surface-base)] px-4 py-3 transition-colors hover:bg-[var(--color-surface-700)]">
				<span class="text-xs font-bold uppercase tracking-[0.14em] text-[var(--color-surface-text)]">Series</span>
				<div class="flex items-center space-x-2">
					<span class="rounded bg-[var(--color-surface-overlay)] px-2 py-0.5 text-xs text-[var(--color-surface-text-muted)]">{series.length}</span>
					<svg class="h-4 w-4 text-[var(--color-surface-text-muted)] transition-transform {seriesOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
					</svg>
				</div>
			</button>
			{#if seriesOpen}
				<div class="border-b border-[var(--color-surface-border)] p-2">
					<input type="search" bind:value={seriesSearch} placeholder="Search series" class="h-8 w-full rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 text-sm text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:border-[var(--color-primary-500)] focus:outline-none" />
				</div>
				<div class="max-h-48 overflow-y-auto">
					{#each visibleSeries as serie}
						<button onclick={() => onToggleSeries(serie.name)} class="grid w-full grid-cols-[1rem_minmax(0,1fr)_auto] items-center gap-2 px-4 py-2 text-left text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)] {isSeriesSelected(serie.name) ? 'bg-[var(--color-primary-500)]/20' : ''}">
							<span class="flex h-4 w-4 items-center justify-center">
								{#if isSeriesSelected(serie.name)}
									<svg class="h-4 w-4 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
									</svg>
								{/if}
							</span>
							<span class="min-w-0 truncate">{serie.name}</span>
							<span class="text-xs text-[var(--color-surface-text-muted)]">{serie.book_count}</span>
						</button>
					{/each}
					{#if series.length === 0}
						<p class="px-4 py-2 text-sm text-[var(--color-surface-text-muted)]">No series found</p>
					{:else if visibleSeries.length === 0}
						<p class="px-4 py-2 text-sm text-[var(--color-surface-text-muted)]">No matching series</p>
					{/if}
				</div>
			{/if}
		</div>

		<div class="overflow-hidden rounded-lg border border-[var(--color-surface-border)]">
			<button onclick={() => tagsOpen = !tagsOpen} class="flex w-full items-center justify-between border-l-4 border-[var(--color-primary-500)]/50 bg-[var(--color-surface-base)] px-4 py-3 transition-colors hover:bg-[var(--color-surface-700)]">
				<span class="text-xs font-bold uppercase tracking-[0.14em] text-[var(--color-surface-text)]">Tags</span>
				<div class="flex items-center space-x-2">
					<span class="rounded bg-[var(--color-surface-overlay)] px-2 py-0.5 text-xs text-[var(--color-surface-text-muted)]">{tags.length}</span>
					<svg class="h-4 w-4 text-[var(--color-surface-text-muted)] transition-transform {tagsOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
					</svg>
				</div>
			</button>
			{#if tagsOpen}
				<div class="border-b border-[var(--color-surface-border)] p-2">
					<input type="search" bind:value={tagSearch} placeholder="Search tags" class="h-8 w-full rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 text-sm text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:border-[var(--color-primary-500)] focus:outline-none" />
				</div>
				<div class="max-h-48 overflow-y-auto">
					{#each visibleTags as tag}
						<button onclick={() => onToggleTag(tag.name)} class="grid w-full grid-cols-[1rem_minmax(0,1fr)_auto] items-center gap-2 px-4 py-2 text-left text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)] {isTagSelected(tag.name) ? 'bg-[var(--color-primary-500)]/20' : ''}">
							<span class="flex h-4 w-4 items-center justify-center">
								{#if isTagSelected(tag.name)}
									<svg class="h-4 w-4 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
									</svg>
								{/if}
							</span>
							<span class="min-w-0 truncate">{tag.name}</span>
							<span class="text-xs text-[var(--color-surface-text-muted)]">{tag.book_count}</span>
						</button>
					{/each}
					{#if tags.length === 0}
						<p class="px-4 py-2 text-sm text-[var(--color-surface-text-muted)]">No tags found</p>
					{:else if visibleTags.length === 0}
						<p class="px-4 py-2 text-sm text-[var(--color-surface-text-muted)]">No matching tags</p>
					{/if}
				</div>
			{/if}
		</div>

		<div class="overflow-hidden rounded-lg border border-[var(--color-surface-border)]">
			<button onclick={() => formatsOpen = !formatsOpen} class="flex w-full items-center justify-between border-l-4 border-[var(--color-primary-500)]/50 bg-[var(--color-surface-base)] px-4 py-3 transition-colors hover:bg-[var(--color-surface-700)]">
				<span class="text-xs font-bold uppercase tracking-[0.14em] text-[var(--color-surface-text)]">Format</span>
				<div class="flex items-center space-x-2">
					<span class="rounded bg-[var(--color-surface-overlay)] px-2 py-0.5 text-xs text-[var(--color-surface-text-muted)]">{formats.length}</span>
					<svg class="h-4 w-4 text-[var(--color-surface-text-muted)] transition-transform {formatsOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
					</svg>
				</div>
			</button>
			{#if formatsOpen}
				<div class="border-b border-[var(--color-surface-border)] p-2">
					<input type="search" bind:value={formatSearch} placeholder="Search formats" class="h-8 w-full rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 text-sm text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:border-[var(--color-primary-500)] focus:outline-none" />
				</div>
				<div class="max-h-48 overflow-y-auto">
					{#each visibleFormats as format}
						<button onclick={() => onToggleFormat(format.name)} class="grid w-full grid-cols-[1rem_minmax(0,1fr)_auto] items-center gap-2 px-4 py-2 text-left text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)] {isFormatSelected(format.name) ? 'bg-[var(--color-primary-500)]/20' : ''}">
							<span class="flex h-4 w-4 items-center justify-center">
								{#if isFormatSelected(format.name)}
									<svg class="h-4 w-4 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
									</svg>
								{/if}
							</span>
							<span class="min-w-0 truncate uppercase tracking-[0.08em]">{format.name}</span>
							<span class="text-xs text-[var(--color-surface-text-muted)]">{format.book_count}</span>
						</button>
					{/each}
					{#if formats.length === 0}
						<p class="px-4 py-2 text-sm text-[var(--color-surface-text-muted)]">No formats found</p>
					{:else if visibleFormats.length === 0}
						<p class="px-4 py-2 text-sm text-[var(--color-surface-text-muted)]">No matching formats</p>
					{/if}
				</div>
			{/if}
		</div>

		<div class="overflow-hidden rounded-lg border border-[var(--color-surface-border)]">
			<button onclick={() => statusOpen = !statusOpen} class="flex w-full items-center justify-between border-l-4 border-[var(--color-primary-500)]/50 bg-[var(--color-surface-base)] px-4 py-3 transition-colors hover:bg-[var(--color-surface-700)]">
				<span class="text-xs font-bold uppercase tracking-[0.14em] text-[var(--color-surface-text)]">Reading Status</span>
				<svg class="h-4 w-4 text-[var(--color-surface-text-muted)] transition-transform {statusOpen ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
				</svg>
			</button>
			{#if statusOpen}
				<div class="border-b border-[var(--color-surface-border)] p-2">
					<input type="search" bind:value={statusSearch} placeholder="Search statuses" class="h-8 w-full rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 text-sm text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:border-[var(--color-primary-500)] focus:outline-none" />
				</div>
				<div class="py-1">
					{#each visibleStatuses as statusOption}
						<button onclick={() => onToggleStatus(statusOption.value)} class="grid w-full grid-cols-[1rem_minmax(0,1fr)] items-center gap-2 px-4 py-2 text-left text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)] {isStatusSelected(statusOption.value) ? 'bg-[var(--color-primary-500)]/20' : ''}">
							<span class="flex h-4 w-4 items-center justify-center">
								{#if isStatusSelected(statusOption.value)}
									<svg class="h-4 w-4 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
									</svg>
								{/if}
							</span>
							<span class="min-w-0 truncate">{statusOption.label}</span>
						</button>
					{/each}
					{#if visibleStatuses.length === 0}
						<p class="px-4 py-2 text-sm text-[var(--color-surface-text-muted)]">No matching statuses</p>
					{/if}
				</div>
			{/if}
		</div>
	</div>
</div>

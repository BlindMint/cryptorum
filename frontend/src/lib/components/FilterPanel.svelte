<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { filterPanelWidth } from '$lib/stores';
	import { lenientSearchMatch } from '$lib/utils/search';

	type FilterMode = 'AND' | 'OR' | 'NOT';
	type FilterOptionSort = 'count' | 'alpha';
	type FilterOption = { name: string; value?: string; book_count?: number };
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
		open?: boolean;
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
		open = true,
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
	let isResizing = $state(false);
	let activeResizePointerId: number | null = null;
	let tooltip = $state<{ text: string; top: number; right: number } | null>(null);
	let tooltipTimer: number | null = null;

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

	function filterOptionValue(option: FilterOption): string {
		return option.value || option.name;
	}

	function filterRowClass(selected: boolean, extra = ''): string {
		const selectedClasses = 'filter-option-selected border-l-2 border-[var(--color-primary-400)] bg-[var(--color-primary-500)]/25 hover:bg-[var(--color-primary-500)]/35';
		const unselectedClasses = 'border-l-2 border-transparent';
		return `filter-option-row grid w-full items-center gap-2 px-4 py-2 text-left text-[var(--color-surface-text)] transition-colors ${selected ? selectedClasses : unselectedClasses} ${extra}`;
	}

	function countClass(selected: boolean): string {
		return selected
			? 'text-xs font-semibold text-[var(--color-primary-200)]'
			: 'text-xs text-[var(--color-surface-text-muted)]';
	}

	function clearTooltipTimer() {
		if (tooltipTimer !== null) {
			window.clearTimeout(tooltipTimer);
			tooltipTimer = null;
		}
	}

	function hideTooltip() {
		clearTooltipTimer();
		tooltip = null;
	}

	function showTooltip(event: Event, text: string, delay = 300) {
		clearTooltipTimer();
		const target = event.currentTarget;
		if (!(target instanceof HTMLElement)) return;
		tooltipTimer = window.setTimeout(() => {
			const rect = target.getBoundingClientRect();
			tooltip = {
				text,
				top: Math.max(12, Math.min(window.innerHeight - 96, rect.bottom + 8)),
				right: Math.max(12, window.innerWidth - rect.right + 8)
			};
		}, delay);
	}

	function handleResizeMove(event: PointerEvent) {
		if (!isResizing || event.pointerId !== activeResizePointerId) return;
		filterPanelWidth.setTemporary(window.innerWidth - event.clientX);
	}

	function stopResize(event?: PointerEvent) {
		if (!isResizing) return;
		if (event && activeResizePointerId !== null && event.pointerId !== activeResizePointerId) return;

		isResizing = false;
		activeResizePointerId = null;
		window.removeEventListener('pointermove', handleResizeMove);
		window.removeEventListener('pointerup', stopResize);
		window.removeEventListener('pointercancel', stopResize);
		document.documentElement.style.cursor = '';
		document.body.style.userSelect = '';
		filterPanelWidth.set($filterPanelWidth);
	}

	function startResize(event: PointerEvent) {
		if (event.button !== 0) return;

		event.preventDefault();
		event.stopPropagation();
		hideTooltip();
		stopResize();

		isResizing = true;
		activeResizePointerId = event.pointerId;
		filterPanelWidth.setTemporary(window.innerWidth - event.clientX);
		document.documentElement.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		window.addEventListener('pointermove', handleResizeMove);
		window.addEventListener('pointerup', stopResize);
		window.addEventListener('pointercancel', stopResize);
	}

	onMount(() => {
		filterPanelWidth.init();
	});

	onDestroy(() => {
		hideTooltip();
		stopResize();
	});
</script>

{#if open && showBackdrop}
	<button
		type="button"
		class="fixed inset-x-0 bottom-0 top-20 z-[35] bg-black/70 lg:hidden"
		aria-label="Close filters"
		onclick={onClose}
	></button>
{/if}

<style>
	.filter-panel-shell {
		width: var(--filter-panel-width);
	}

	@media (max-width: 640px) {
		.filter-panel-shell {
			width: min(max(var(--filter-panel-width), 22.5rem), calc(100vw - 0.5rem));
			max-width: calc(100vw - 0.5rem);
		}
	}

	.filter-option-row:not(.filter-option-selected):hover,
	.filter-option-row:not(.filter-option-selected):focus-visible {
		background: color-mix(in srgb, var(--color-surface-text, #e2e8f0) 8%, transparent);
	}
</style>

<div
	class="filter-panel-shell fixed right-0 top-20 z-40 h-[calc(100dvh-5rem)] max-w-[calc(100vw-1rem)] overflow-y-auto border-l border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-xl {open ? 'translate-x-0' : 'invisible translate-x-full pointer-events-none'}"
	style={`--filter-panel-width: ${$filterPanelWidth}px;`}
	aria-hidden={!open}
	inert={!open}
	onscroll={hideTooltip}
>
	<div
		class="group absolute inset-y-0 left-0 z-20 hidden w-3 cursor-col-resize touch-none select-none lg:flex"
		title="Drag to resize filters"
		role="separator"
		aria-orientation="vertical"
		aria-label="Resize filters"
		onpointerdown={startResize}
	>
		<div class="h-full w-px bg-transparent transition-colors group-hover:bg-[var(--color-primary-500)]/55 {isResizing ? 'bg-[var(--color-primary-500)]/70' : ''}"></div>
	</div>
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
						{@const authorValue = filterOptionValue(author)}
						{@const selected = isAuthorSelected(authorValue)}
						<button
							onclick={() => onToggleAuthor(authorValue)}
							onmouseenter={(event) => showTooltip(event, author.name)}
							onfocus={(event) => showTooltip(event, author.name, 0)}
							onmouseleave={hideTooltip}
							onblur={hideTooltip}
							class={filterRowClass(selected, 'grid-cols-[1rem_minmax(0,1fr)_auto]')}
						>
							<span class="flex h-4 w-4 items-center justify-center">
								{#if selected}
									<svg class="h-4 w-4 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
									</svg>
								{/if}
							</span>
							<span class="min-w-0 truncate">{author.name}</span>
							<span class={countClass(selected)}>{author.book_count}</span>
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
						{@const selected = isSeriesSelected(serie.name)}
						<button
							onclick={() => onToggleSeries(serie.name)}
							onmouseenter={(event) => showTooltip(event, serie.name)}
							onfocus={(event) => showTooltip(event, serie.name, 0)}
							onmouseleave={hideTooltip}
							onblur={hideTooltip}
							class={filterRowClass(selected, 'grid-cols-[1rem_minmax(0,1fr)_auto]')}
						>
							<span class="flex h-4 w-4 items-center justify-center">
								{#if selected}
									<svg class="h-4 w-4 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
									</svg>
								{/if}
							</span>
							<span class="min-w-0 truncate">{serie.name}</span>
							<span class={countClass(selected)}>{serie.book_count}</span>
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
						{@const selected = isTagSelected(tag.name)}
						<button
							onclick={() => onToggleTag(tag.name)}
							onmouseenter={(event) => showTooltip(event, tag.name)}
							onfocus={(event) => showTooltip(event, tag.name, 0)}
							onmouseleave={hideTooltip}
							onblur={hideTooltip}
							class={filterRowClass(selected, 'grid-cols-[1rem_minmax(0,1fr)_auto]')}
						>
							<span class="flex h-4 w-4 items-center justify-center">
								{#if selected}
									<svg class="h-4 w-4 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
									</svg>
								{/if}
							</span>
							<span class="min-w-0 truncate">{tag.name}</span>
							<span class={countClass(selected)}>{tag.book_count}</span>
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
						{@const selected = isFormatSelected(format.name)}
						<button
							onclick={() => onToggleFormat(format.name)}
							onmouseenter={(event) => showTooltip(event, format.name)}
							onfocus={(event) => showTooltip(event, format.name, 0)}
							onmouseleave={hideTooltip}
							onblur={hideTooltip}
							class={filterRowClass(selected, 'grid-cols-[1rem_minmax(0,1fr)_auto]')}
						>
							<span class="flex h-4 w-4 items-center justify-center">
								{#if selected}
									<svg class="h-4 w-4 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
									</svg>
								{/if}
							</span>
							<span class="min-w-0 truncate uppercase tracking-[0.08em]">{format.name}</span>
							<span class={countClass(selected)}>{format.book_count}</span>
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
						{@const selected = isStatusSelected(statusOption.value)}
						<button
							onclick={() => onToggleStatus(statusOption.value)}
							onmouseenter={(event) => showTooltip(event, statusOption.label)}
							onfocus={(event) => showTooltip(event, statusOption.label, 0)}
							onmouseleave={hideTooltip}
							onblur={hideTooltip}
							class={filterRowClass(selected, 'grid-cols-[1rem_minmax(0,1fr)]')}
						>
							<span class="flex h-4 w-4 items-center justify-center">
								{#if selected}
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

{#if tooltip}
	<div
		class="pointer-events-none fixed z-[70] max-w-[min(28rem,calc(100vw-2rem))] rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-sm leading-snug text-[var(--color-surface-text)] shadow-2xl"
		style={`top: ${tooltip.top}px; right: ${tooltip.right}px;`}
		role="tooltip"
	>
		{tooltip.text}
	</div>
{/if}

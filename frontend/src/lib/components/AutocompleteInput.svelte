<script lang="ts">
	import { lenientSearchMatch } from '$lib/utils/search';

	type MetadataSuggestionField = 'authors' | 'series' | 'publishers' | 'languages' | 'genres' | 'tags';

	interface Props {
		value: string;
		placeholder?: string;
		field: MetadataSuggestionField;
		multiple?: boolean;
		onchange: (value: string) => void;
		id?: string;
	}

	let { value = $bindable(''), placeholder = '', field, multiple, onchange, id }: Props = $props();

	let suggestions: string[] = $state([]);
	let showDropdown = $state(false);
	let filteredSuggestions: string[] = $state([]);
	let selectedIndex = $state(-1);
	let inputElement: HTMLInputElement | null = $state(null);
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	async function fetchSuggestions() {
		try {
			const res = await fetch('/api/filter-options');
			if (res.ok) {
				const data = await res.json();
				suggestions = extractSuggestionNames(data[field]);
			}
		} catch {
			suggestions = [];
		}
	}

	function extractSuggestionNames(items: unknown): string[] {
		if (!Array.isArray(items)) return [];
		return items
			.map((item) => {
				if (typeof item === 'string') return item;
				const name = (item as { name?: unknown })?.name;
				return typeof name === 'string' ? name : '';
			})
			.map((item) => item.trim())
			.filter(Boolean);
	}

	function isMultipleValueField(): boolean {
		return multiple ?? (field === 'genres' || field === 'tags');
	}

	function getLastSegment(input: string): { before: string; segment: string; after: string } {
		if (!isMultipleValueField()) {
			return { before: '', segment: input, after: '' };
		}
		const lastCommaIndex = input.lastIndexOf(',');
		if (lastCommaIndex === -1) {
			return { before: '', segment: input, after: '' };
		}
		return {
			before: input.slice(0, lastCommaIndex + 1),
			segment: input.slice(lastCommaIndex + 1),
			after: ''
		};
	}

	function filterSuggestions(segment: string): string[] {
		if (!segment.trim()) {
			return suggestions.slice(0, 20);
		}
		const lower = segment.toLowerCase();
		return suggestions
			.filter(s => s.toLowerCase().includes(lower) || lenientSearchMatch(segment, s))
			.sort((a, b) => {
				const aLower = a.toLowerCase();
				const bLower = b.toLowerCase();
				const aStarts = aLower.startsWith(lower);
				const bStarts = bLower.startsWith(lower);
				if (aStarts !== bStarts) return aStarts ? -1 : 1;
				return a.localeCompare(b);
			})
			.slice(0, 20);
	}

	function handleInput(e: Event) {
		const target = e.target as HTMLInputElement;
		value = target.value;
		onchange(value);

		if (debounceTimer) {
			clearTimeout(debounceTimer);
		}
		debounceTimer = setTimeout(() => {
			const { segment } = getLastSegment(value);
			filteredSuggestions = filterSuggestions(segment);
			showDropdown = filteredSuggestions.length > 0;
			selectedIndex = -1;
		}, 150);
	}

	function handleFocus() {
		const { segment } = getLastSegment(value);
		filteredSuggestions = filterSuggestions(segment);
		showDropdown = filteredSuggestions.length > 0;
		selectedIndex = -1;
	}

	function handleBlur() {
		setTimeout(() => {
			showDropdown = false;
		}, 150);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (!showDropdown) return;

		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				selectedIndex = Math.min(selectedIndex + 1, filteredSuggestions.length - 1);
				break;
			case 'ArrowUp':
				e.preventDefault();
				selectedIndex = Math.max(selectedIndex - 1, -1);
				break;
			case 'Enter':
				e.preventDefault();
				if (selectedIndex >= 0 && selectedIndex < filteredSuggestions.length) {
					selectSuggestion(filteredSuggestions[selectedIndex]);
				}
				break;
			case 'Escape':
				showDropdown = false;
				selectedIndex = -1;
				break;
		}
	}

	function selectSuggestion(suggestion: string) {
		const { before } = getLastSegment(value);
		const separator = before && isMultipleValueField() ? ' ' : '';
		value = before + separator + suggestion;
		onchange(value);
		showDropdown = false;
		selectedIndex = -1;
		inputElement?.focus();
	}

	$effect(() => {
		fetchSuggestions();
	});
</script>

<div class="relative">
	<input
		bind:this={inputElement}
		{id}
		type="text"
		{value}
		{placeholder}
		oninput={handleInput}
		onfocus={handleFocus}
		onblur={handleBlur}
		onkeydown={handleKeydown}
		class="w-full bg-[var(--color-surface-700)] border border-[var(--color-surface-border)] rounded px-3 py-2 text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
	/>

	{#if showDropdown && filteredSuggestions.length > 0}
		<div class="absolute left-0 right-0 top-full mt-1 z-50 max-h-60 overflow-y-auto overflow-x-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-lg backdrop-blur-sm">
			<ul class="px-1 py-1">
				{#each filteredSuggestions as suggestion, i}
					<li>
						<button
							type="button"
							onclick={() => selectSuggestion(suggestion)}
							onmouseenter={() => selectedIndex = i}
							class="autocomplete-option w-full rounded-md px-3 py-2 text-left text-[var(--color-surface-text)] transition-all duration-150 ease-out focus-visible:outline-none {i === selectedIndex ? 'active' : ''}"
						>
							<span class="block truncate">{suggestion}</span>
						</button>
					</li>
				{/each}
			</ul>
		</div>
	{/if}
</div>

<style>
	.autocomplete-option:hover,
	.autocomplete-option.active {
		background: color-mix(in srgb, var(--color-surface-text, #e2e8f0) 8%, transparent);
		transform: translateX(2px);
	}
</style>

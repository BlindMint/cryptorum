<script lang="ts">
	interface Props {
		value: string;
		placeholder?: string;
		field: 'genres' | 'tags';
		onchange: (value: string) => void;
		id?: string;
	}

	let { value = $bindable(''), placeholder = '', field, onchange, id }: Props = $props();

	let suggestions: string[] = $state([]);
	let showDropdown = $state(false);
	let filteredSuggestions: string[] = $state([]);
	let selectedIndex = $state(-1);
	let inputElement: HTMLInputElement | null = $state(null);
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	async function fetchSuggestions() {
		try {
			const res = await fetch(`/api/metadata/suggestions?field=${field}`);
			if (res.ok) {
				suggestions = await res.json();
			}
		} catch {
			suggestions = [];
		}
	}

	function getLastSegment(input: string): { before: string; segment: string; after: string } {
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
			.filter(s => s.toLowerCase().includes(lower))
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
		value = before + suggestion;
		onchange(value);
		showDropdown = false;
		selectedIndex = -1;
		inputElement?.focus();
	}

	function highlightMatch(text: string): string {
		const { segment } = getLastSegment(value);
		if (!segment.trim()) return text;
		const lower = text.toLowerCase();
		const searchLower = segment.toLowerCase();
		const index = lower.indexOf(searchLower);
		if (index === -1) return text;

		return (
			text.slice(0, index) +
			'<mark class="bg-[var(--color-primary-500)]/30 text-[var(--color-primary-300)]">' +
			text.slice(index, index + segment.length) +
			'</mark>' +
			text.slice(index + segment.length)
		);
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
		<div class="absolute left-0 right-0 top-full mt-1 bg-[var(--color-surface-overlay)] backdrop-blur-sm border border-[var(--color-surface-border)] rounded-lg shadow-lg z-50 max-h-60 overflow-y-auto">
			<ul class="py-1">
				{#each filteredSuggestions as suggestion, i}
					<li>
						<button
							type="button"
							onclick={() => selectSuggestion(suggestion)}
							class="w-full px-3 py-2 text-left text-[var(--color-surface-text)] hover:bg-[var(--color-surface-hover)] transition-colors {i === selectedIndex ? 'bg-[var(--color-surface-hover)]' : ''}"
						>
							{@html highlightMatch(suggestion)}
						</button>
					</li>
				{/each}
			</ul>
		</div>
	{/if}
</div>

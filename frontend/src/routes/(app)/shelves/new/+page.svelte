<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';

	let shelfName = $state('');
	let shelfIcon = $state('bookmark');
	let isMagicShelf = $state(false);
	let conditions = $state<any[]>([
		{ field: 'status', operator: 'equals', value: 'unread' }
	]);
	let isSubmitting = $state(false);

	const fieldOptions = [
		{ value: 'status', label: 'Reading Status' },
		{ value: 'authors', label: 'Author' },
		{ value: 'series', label: 'Series' },
		{ value: 'genres', label: 'Genres' },
		{ value: 'publisher', label: 'Publisher' },
		{ value: 'language', label: 'Language' },
		{ value: 'rating', label: 'Rating' },
		{ value: 'page_count', label: 'Page Count' },
		{ value: 'added_at', label: 'Date Added' }
	];

	const operatorOptions = [
		{ value: 'equals', label: 'Equals' },
		{ value: 'not_equals', label: 'Not Equals' },
		{ value: 'contains', label: 'Contains' },
		{ value: 'not_contains', label: 'Does Not Contain' },
		{ value: 'greater_than', label: 'Greater Than' },
		{ value: 'less_than', label: 'Less Than' },
		{ value: 'between', label: 'Between' }
	];

	const statusOptions = [
		{ value: 'unread', label: 'Unread' },
		{ value: 'reading', label: 'Reading' },
		{ value: 'finished', label: 'Finished' }
	];

	onMount(() => {
		isMagicShelf = $page.url.searchParams.get('magic') === 'true';
	});

	function addCondition() {
		conditions = [...conditions, { field: 'status', operator: 'equals', value: 'unread' }];
	}

	function removeCondition(index: number) {
		conditions.splice(index, 1);
		conditions = [...conditions];
	}

	function updateCondition(index: number, key: string, value: any) {
		conditions[index] = { ...conditions[index], [key]: value };
		conditions = [...conditions];
	}

	async function createShelf() {
		if (!shelfName.trim()) return;

		isSubmitting = true;
		try {
			const response = await fetch('/api/shelves', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					name: shelfName.trim(),
					icon: shelfIcon,
					is_magic: isMagicShelf ? 1 : 0,
					rules_json: isMagicShelf ? JSON.stringify({ conditions }) : '',
					sort_by: isMagicShelf ? 'added_at' : 'name',
					sort_dir: isMagicShelf ? 'desc' : 'asc'
				})
			});

			if (response.ok) {
				goto('/shelves');
			} else {
				console.error('Failed to create shelf');
			}
		} catch (error) {
			console.error('Error creating shelf:', error);
		} finally {
			isSubmitting = false;
		}
	}

	function getValueInputType(field: string, operator: string) {
		if (field === 'status') return 'select';
		if (field === 'rating') return 'number';
		if (field === 'page_count') return 'number';
		if (field === 'added_at') return 'date';
		if (operator === 'between') return 'range';
		return 'text';
	}

	function getValueOptions(field: string) {
		if (field === 'status') return statusOptions;
		return [];
	}
</script>

<div class="space-y-6">
	<div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
		<div>
			<h1 class="text-2xl font-bold text-[var(--color-surface-text)]">
				{isMagicShelf ? 'Create Magic Shelf' : 'Create Shelf'}
			</h1>
			<p class="text-[var(--color-surface-text-muted)] mt-1">
				{isMagicShelf ? 'Automatically organize books based on rules' : 'Manually organize books into a collection'}
			</p>
		</div>

		<div class="inline-flex rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-1">
			<button
				type="button"
				onclick={() => isMagicShelf = false}
				class="rounded-md px-3 py-2 text-sm font-medium transition-colors {isMagicShelf ? 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]' : 'bg-[var(--color-primary-500)] text-white'}"
			>
				Regular Shelf
			</button>
			<button
				type="button"
				onclick={() => isMagicShelf = true}
				class="rounded-md px-3 py-2 text-sm font-medium transition-colors {isMagicShelf ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
			>
				Magic Shelf
			</button>
		</div>
	</div>

	<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] p-6">
		<div class="space-y-6">
				<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
					<div>
						<label for="shelf-name" class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">
							Shelf Name
						</label>
						<input
							id="shelf-name"
							type="text"
							bind:value={shelfName}
							placeholder="e.g., Fantasy Books"
							class="w-full px-3 py-2 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
						>
					</div>
					<div>
						<label for="shelf-icon" class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">
							Icon
						</label>
						<select
							id="shelf-icon"
							bind:value={shelfIcon}
							class="w-full px-3 py-2 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded-lg text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
						>
						<option value="bookmark">📚 Bookmark</option>
						<option value="star">⭐ Star</option>
						<option value="heart">❤️ Heart</option>
						<option value="fire">🔥 Fire</option>
						<option value="magic">✨ Magic</option>
					</select>
				</div>
			</div>

			{#if isMagicShelf}
				<div>
						<div class="flex items-center justify-between mb-4">
							<h3 class="text-lg font-medium text-[var(--color-surface-text)]">Rules</h3>
							<button
								type="button"
								onclick={addCondition}
								class="px-3 py-1 bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white rounded-lg text-sm transition-colors"
							>
							Add Rule
						</button>
					</div>

					<div class="space-y-3">
						{#each conditions as condition, index}
							<div class="flex flex-wrap items-center gap-3 p-3 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
								<span class="text-sm text-[var(--color-surface-text-muted)] font-medium">#{index + 1}</span>

								<select
									bind:value={condition.field}
									onchange={(e) => updateCondition(index, 'field', (e.target as HTMLSelectElement).value)}
									class="px-2 py-1 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
								>
									{#each fieldOptions as option}
										<option value={option.value}>{option.label}</option>
									{/each}
								</select>

								<select
									bind:value={condition.operator}
									onchange={(e) => updateCondition(index, 'operator', (e.target as HTMLSelectElement).value)}
									class="px-2 py-1 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
								>
									{#each operatorOptions as option}
										<option value={option.value}>{option.label}</option>
									{/each}
								</select>

								{#if getValueInputType(condition.field, condition.operator) === 'select'}
									<select
										bind:value={condition.value}
										onchange={(e) => updateCondition(index, 'value', (e.target as HTMLSelectElement).value)}
										class="px-2 py-1 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
									>
										{#each getValueOptions(condition.field) as option}
											<option value={option.value}>{option.label}</option>
										{/each}
									</select>
								{:else if getValueInputType(condition.field, condition.operator) === 'number'}
									<input
										type="number"
										bind:value={condition.value}
										oninput={(e) => updateCondition(index, 'value', (e.target as HTMLInputElement).value)}
										class="px-2 py-1 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
										placeholder="Enter value"
									>
								{:else if getValueInputType(condition.field, condition.operator) === 'date'}
									<input
										type="date"
										bind:value={condition.value}
										onchange={(e) => updateCondition(index, 'value', (e.target as HTMLInputElement).value)}
										class="px-2 py-1 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
									>
								{:else if getValueInputType(condition.field, condition.operator) === 'range'}
									<div class="flex items-center space-x-2">
										<input
											type="text"
											bind:value={condition.value.min}
											oninput={(e) => updateCondition(index, 'value', { ...condition.value, min: (e.target as HTMLInputElement).value })}
											class="px-2 py-1 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
											placeholder="Min"
										>
										<span class="text-[var(--color-surface-text-muted)]">to</span>
										<input
											type="text"
											bind:value={condition.value.max}
											oninput={(e) => updateCondition(index, 'value', { ...condition.value, max: (e.target as HTMLInputElement).value })}
											class="px-2 py-1 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
											placeholder="Max"
										>
									</div>
								{:else}
									<input
										type="text"
										bind:value={condition.value}
										oninput={(e) => updateCondition(index, 'value', (e.target as HTMLInputElement).value)}
										class="px-2 py-1 bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] rounded text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
										placeholder="Enter value"
									>
								{/if}

									<button
										type="button"
										onclick={() => removeCondition(index)}
										aria-label={`Remove rule ${index + 1}`}
										class="p-1 text-red-400 hover:text-red-300 transition-colors"
										disabled={conditions.length === 1}
									>
									<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
									</svg>
								</button>
							</div>
						{/each}
					</div>

					<p class="text-xs text-[var(--color-surface-text-muted)] mt-2">
						All conditions must be met (AND logic). Books will be automatically added and removed from this shelf.
					</p>
				</div>
			{:else}
				<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
					<p class="text-sm text-[var(--color-surface-text-muted)]">
						Regular shelves are manual collections. Add books to them from a book detail page or the library bulk picker.
					</p>
				</div>
			{/if}

			<div class="flex justify-end space-x-3">
					<a
						href="/shelves"
						class="px-4 py-2 border border-[var(--color-surface-border)] text-[var(--color-surface-text)] rounded-lg hover:bg-[var(--color-surface-overlay)] transition-colors"
					>
						Cancel
					</a>
					<button
						type="button"
						onclick={createShelf}
						disabled={!shelfName.trim() || isSubmitting}
						class="px-4 py-2 bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] disabled:opacity-50 disabled:cursor-not-allowed text-white rounded-lg transition-colors"
					>
					{#if isSubmitting}
						Creating...
					{:else if isMagicShelf}
						Create Magic Shelf
					{:else}
						Create Shelf
					{/if}
				</button>
			</div>
		</div>
	</div>
</div>

<script lang="ts">
	import { untrack } from 'svelte';
	import LibraryIconPicker from '$lib/components/LibraryIconPicker.svelte';
	import { parseLibraryIcon } from '$lib/utils/library-icons';

	interface ShelfCondition {
		field: string;
		operator: string;
		value: any;
	}

	interface SelectOption {
		value: string;
		label: string;
	}

	interface Shelf {
		id?: number;
		name?: string;
		icon?: string;
		is_magic?: number;
		rules_json?: string;
		sort_by?: string;
		sort_dir?: string;
		book_count?: number;
	}

	interface Props {
		open: boolean;
		mode?: 'create' | 'edit';
		shelf?: Shelf | null;
		initialMagic?: boolean;
		allowMagicToggle?: boolean;
		onClose?: () => void;
		onSaved?: (shelf: Shelf) => void;
	}

	let {
		open = false,
		mode = 'create',
		shelf = null,
		initialMagic = false,
		allowMagicToggle = true,
		onClose,
		onSaved
	}: Props = $props();

	let shelfName = $state('');
	let shelfIcon = $state('bookmark');
	let isMagicShelf = $state(false);
	let conditions = $state<ShelfCondition[]>([
		{ field: 'status', operator: 'equals', value: 'unread' }
	]);
	let isSubmitting = $state(false);
	let showShelfIconPicker = $state(false);
	let errorMessage = $state('');
	let currentShelfIcon = $derived(parseLibraryIcon(shelfIcon));
	let isEditing = $derived(mode === 'edit' && !!shelf?.id);
	let lastResetKey = '';

	const fieldOptions = [
		{ value: 'status', label: 'Reading Status' },
		{ value: 'authors', label: 'Author' },
		{ value: 'series', label: 'Series' },
		{ value: 'tags', label: 'Tags' },
		{ value: 'publisher', label: 'Publisher' },
		{ value: 'language', label: 'Language' },
		{ value: 'rating', label: 'Rating' },
		{ value: 'page_count', label: 'Page Count' }
	];

	const operatorOptionsByField: Record<string, SelectOption[]> = {
		status: [
			{ value: 'equals', label: 'Equals' },
			{ value: 'not_equals', label: 'Not Equals' }
		],
		authors: [{ value: 'contains', label: 'Contains' }],
		series: [
			{ value: 'equals', label: 'Equals' },
			{ value: 'contains', label: 'Contains' }
		],
		tags: [{ value: 'contains', label: 'Contains' }],
		publisher: [
			{ value: 'equals', label: 'Equals' },
			{ value: 'contains', label: 'Contains' }
		],
		language: [{ value: 'equals', label: 'Equals' }],
		rating: [
			{ value: 'equals', label: 'Equals' },
			{ value: 'greater_than', label: 'Greater Than' },
			{ value: 'less_than', label: 'Less Than' }
		],
		page_count: [
			{ value: 'equals', label: 'Equals' },
			{ value: 'greater_than', label: 'Greater Than' },
			{ value: 'less_than', label: 'Less Than' }
		]
	};

	const statusOptions = [
		{ value: 'unread', label: 'Unread' },
		{ value: 'reading', label: 'Reading' },
		{ value: 'finished', label: 'Finished' }
	];
	const shelfSortOptions = [
		{ value: 'title', label: 'Title' },
		{ value: 'authors', label: 'Author' },
		{ value: 'series', label: 'Series' },
		{ value: 'added_at', label: 'Date Added' },
		{ value: 'last_read', label: 'Last Read' }
	];
	let shelfSortBy = $state('added_at');
	let shelfSortDir = $state<'asc' | 'desc'>('desc');

	function getDefaultConditions(): ShelfCondition[] {
		return [{ field: 'status', operator: 'equals', value: 'unread' }];
	}

	function parseShelfConditions(rawRules: string | undefined): ShelfCondition[] {
		if (!rawRules?.trim()) return getDefaultConditions();
		try {
			const parsed = JSON.parse(rawRules);
			if (Array.isArray(parsed?.conditions) && parsed.conditions.length > 0) {
				return parsed.conditions.map((condition: any) => normalizeCondition(condition));
			}
		} catch (e) {
			console.error('Failed to parse shelf rules:', e);
		}
		return getDefaultConditions();
	}

	function normalizeCondition(condition: Partial<ShelfCondition>): ShelfCondition {
		const field = fieldOptions.some(option => option.value === condition.field) ? String(condition.field) : 'status';
		const operatorOptions = getOperatorOptions(field);
		const operator = operatorOptions.some(option => option.value === condition.operator)
			? String(condition.operator)
			: operatorOptions[0]?.value || 'equals';
		let value = condition.value ?? defaultValueForField(field);
		if (field === 'status' && !statusOptions.some(option => option.value === value)) {
			value = 'unread';
		}
		if ((field === 'rating' || field === 'page_count') && typeof value === 'object') {
			value = '';
		}
		return { field, operator, value };
	}

	function resetForm() {
		errorMessage = '';
		showShelfIconPicker = false;
		if (isEditing && shelf) {
			shelfName = shelf.name || '';
			shelfIcon = shelf.icon || (shelf.is_magic === 1 ? 'sparkles' : 'bookmark');
			isMagicShelf = shelf.is_magic === 1;
			conditions = parseShelfConditions(shelf.rules_json);
			shelfSortBy = normalizeShelfSortBy(shelf.sort_by);
			shelfSortDir = shelf.sort_dir === 'asc' ? 'asc' : 'desc';
			return;
		}

		shelfName = '';
		isMagicShelf = allowMagicToggle ? initialMagic : false;
		shelfIcon = isMagicShelf ? 'sparkles' : 'bookmark';
		conditions = getDefaultConditions();
		shelfSortBy = 'added_at';
		shelfSortDir = 'desc';
	}

	function closeModal() {
		if (isSubmitting) return;
		onClose?.();
	}

	function addCondition() {
		conditions = [...conditions, { field: 'status', operator: 'equals', value: 'unread' }];
	}

	function removeCondition(index: number) {
		if (conditions.length === 1) return;
		conditions.splice(index, 1);
		conditions = [...conditions];
	}

	function updateCondition(index: number, key: string, value: any) {
		conditions[index] = { ...conditions[index], [key]: value };
		conditions = [...conditions];
	}

	function updateConditionField(index: number, field: string) {
		const operator = getOperatorOptions(field)[0]?.value || 'equals';
		conditions[index] = { field, operator, value: defaultValueForField(field) };
		conditions = [...conditions];
	}

	function getOperatorOptions(field: string) {
		return operatorOptionsByField[field] || operatorOptionsByField.status;
	}

	function defaultValueForField(field: string) {
		if (field === 'status') return 'unread';
		return '';
	}

	function getValueInputType(field: string, operator: string) {
		if (field === 'status') return 'select';
		if (field === 'rating') return 'number';
		if (field === 'page_count') return 'number';
		return 'text';
	}

	function getValueOptions(field: string) {
		if (field === 'status') return statusOptions;
		return [];
	}

	function selectShelfIcon(iconValue: string) {
		shelfIcon = iconValue;
	}

	function clearShelfIcon() {
		shelfIcon = '';
	}

	function setShelfKind(nextIsMagic: boolean) {
		isMagicShelf = nextIsMagic;
		if (nextIsMagic) {
			if (!shelfIcon || shelfIcon === 'bookmark') shelfIcon = 'sparkles';
			return;
		}
		if (!shelfIcon || shelfIcon === 'sparkles') shelfIcon = 'bookmark';
	}

	function normalizeShelfSortBy(value: string | undefined) {
		const normalized = value || 'added_at';
		return shelfSortOptions.some(option => option.value === normalized) ? normalized : 'added_at';
	}

	async function saveShelf() {
		if (!shelfName.trim()) return;

		isSubmitting = true;
		errorMessage = '';
		const payload = {
			name: shelfName.trim(),
			icon: shelfIcon,
			is_magic: isMagicShelf ? 1 : 0,
			rules_json: isMagicShelf ? JSON.stringify({ conditions }) : '',
			sort_by: isMagicShelf ? shelfSortBy : (shelf?.sort_by || 'title'),
			sort_dir: isMagicShelf ? shelfSortDir : (shelf?.sort_dir || 'asc')
		};

		try {
			const response = await fetch(isEditing ? `/api/shelves/${shelf?.id}` : '/api/shelves', {
				method: isEditing ? 'PUT' : 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(payload)
			});

			if (!response.ok) {
				errorMessage = await response.text() || `Failed to ${isEditing ? 'update' : 'create'} shelf`;
				return;
			}

			const saved = isEditing ? { ...shelf, ...payload } : await response.json();
			await (window as any).refreshSidebar?.();
			onSaved?.(saved);
			onClose?.();
		} catch (error) {
			console.error('Failed to save shelf:', error);
			errorMessage = `Failed to ${isEditing ? 'update' : 'create'} shelf`;
		} finally {
			isSubmitting = false;
		}
	}

	$effect(() => {
		if (!open) {
			lastResetKey = '';
			return;
		}

		const resetKey = `${mode}:${shelf?.id ?? 'new'}:${initialMagic ? 'magic' : 'manual'}:${allowMagicToggle ? 'toggle' : 'fixed'}`;
		if (resetKey === lastResetKey) return;
		lastResetKey = resetKey;
		untrack(resetForm);
	});
</script>

{#if open}
	<div
		class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4"
		role="dialog"
		aria-modal="true"
		tabindex="-1"
		onkeydown={(event) => { if (event.key === 'Escape') closeModal(); }}
	>
		<button
			type="button"
			class="absolute inset-0 z-0"
			aria-label="Close shelf modal"
			onclick={closeModal}
		></button>

		<div class="relative z-10 flex max-h-[90vh] w-full max-w-3xl flex-col overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
			<div class="flex items-center justify-between gap-4 border-b border-[var(--color-surface-border)] px-6 py-4">
				<div>
					<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">{isEditing ? 'Edit Shelf' : isMagicShelf ? 'Create Magic Shelf' : 'Create Shelf'}</h3>
					<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">
						{isMagicShelf ? 'Automatically organize books based on rules.' : 'Manually organize books into a collection.'}
					</p>
				</div>
				<button
					type="button"
					class="inline-flex h-9 w-9 items-center justify-center rounded-md text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)]"
					aria-label="Close"
					onclick={closeModal}
				>
					<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<path d="M18 6 6 18"></path>
						<path d="m6 6 12 12"></path>
					</svg>
				</button>
			</div>

			<div class="min-h-0 flex-1 space-y-6 overflow-y-auto p-6">
				{#if !isEditing && allowMagicToggle}
					<div class="inline-flex rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-1">
						<button
							type="button"
							onclick={() => setShelfKind(false)}
							class="inline-flex items-center rounded-md border px-3 py-2 text-sm font-medium transition-colors {isMagicShelf ? 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]' : 'border-[var(--color-primary-500)]/45 bg-[var(--color-primary-500)]/10 text-[var(--color-primary-400)]'}"
						>
							Regular Shelf
						</button>
						<button
							type="button"
							onclick={() => setShelfKind(true)}
							class="inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm font-medium transition-colors {isMagicShelf ? 'border-[var(--color-primary-500)]/45 bg-[var(--color-primary-500)]/10 text-[var(--color-primary-400)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
						>
							<svg class="h-4 w-4 {isMagicShelf ? 'text-purple-300' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"></path>
							</svg>
							Magic Shelf
						</button>
					</div>
				{:else}
					<div class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-1 text-xs font-medium uppercase tracking-wide {isMagicShelf ? 'text-purple-300' : 'text-[var(--color-primary-400)]'}">
						{#if isMagicShelf}
							<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"></path>
							</svg>
							<span>Magic</span>
						{:else}
							<span>Manual</span>
						{/if}
					</div>
				{/if}

				<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
					<div>
						<label for="shelf-modal-name" class="mb-2 block text-sm font-medium text-[var(--color-surface-text)]">
							Shelf Name
						</label>
						<input
							id="shelf-modal-name"
							type="text"
							bind:value={shelfName}
							placeholder="e.g., Fantasy Books"
							class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
						>
					</div>

					<div>
						<div class="mb-2 block text-sm font-medium text-[var(--color-surface-text)]">Icon</div>
						<div class="flex gap-2">
							<button
								type="button"
								onclick={() => showShelfIconPicker = true}
								class="flex min-w-0 flex-1 items-center gap-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-left transition-colors hover:border-[var(--color-primary-500)]/60 hover:bg-[var(--color-surface-overlay)]"
							>
								<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-[var(--color-surface-overlay)] text-[var(--color-primary-400)]">
									{#if currentShelfIcon?.svg}
										<div class="shelf-icon-preview h-5 w-5">{@html currentShelfIcon.svg}</div>
									{:else}
										<span class="text-xs font-semibold uppercase">{(shelfName || '?').slice(0, 1)}</span>
									{/if}
								</div>
								<div class="min-w-0 flex-1">
									<div class="truncate text-sm font-medium text-[var(--color-surface-text)]">
										{currentShelfIcon?.label || currentShelfIcon?.name || 'Default icon'}
									</div>
									<div class="text-xs text-[var(--color-surface-text-muted)]">Click to choose a different icon</div>
								</div>
							</button>
							{#if shelfIcon && shelfIcon !== (isMagicShelf ? 'sparkles' : 'bookmark')}
								<button
									type="button"
									onclick={clearShelfIcon}
									class="inline-flex h-[58px] shrink-0 items-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 text-xs font-medium text-[var(--color-surface-text-muted)] transition-colors hover:border-[var(--color-primary-500)]/50 hover:text-[var(--color-surface-text)]"
								>
									Default
								</button>
							{/if}
						</div>
					</div>
				</div>

				{#if isMagicShelf}
					<div>
						<div class="mb-4 flex items-center justify-between">
							<h4 class="text-base font-medium text-[var(--color-surface-text)]">Rules</h4>
							<button
								type="button"
								onclick={addCondition}
								class="accent-action rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
							>
								Add Rule
							</button>
						</div>

						<div class="space-y-3">
							{#each conditions as condition, index}
								<div class="flex flex-wrap items-center gap-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-3">
									<span class="text-sm font-medium text-[var(--color-surface-text-muted)]">#{index + 1}</span>

									<select
										bind:value={condition.field}
										onchange={(e) => updateConditionField(index, (e.target as HTMLSelectElement).value)}
										class="rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-2 py-1 text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
									>
										{#each fieldOptions as option}
											<option value={option.value}>{option.label}</option>
										{/each}
									</select>

									<select
										bind:value={condition.operator}
										onchange={(e) => updateCondition(index, 'operator', (e.target as HTMLSelectElement).value)}
										class="rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-2 py-1 text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
									>
										{#each getOperatorOptions(condition.field) as option}
											<option value={option.value}>{option.label}</option>
										{/each}
									</select>

									{#if getValueInputType(condition.field, condition.operator) === 'select'}
										<select
											bind:value={condition.value}
											onchange={(e) => updateCondition(index, 'value', (e.target as HTMLSelectElement).value)}
											class="rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-2 py-1 text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
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
											class="rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-2 py-1 text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
											placeholder="Enter value"
										>
									{:else}
										<input
											type="text"
											bind:value={condition.value}
											oninput={(e) => updateCondition(index, 'value', (e.target as HTMLInputElement).value)}
											class="rounded border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-2 py-1 text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary-500)]"
											placeholder="Enter value"
										>
									{/if}

									<button
										type="button"
										onclick={() => removeCondition(index)}
										aria-label={`Remove rule ${index + 1}`}
										class="rounded p-1 text-red-400 transition-colors hover:bg-red-500/10 hover:text-red-300 disabled:cursor-not-allowed disabled:opacity-40"
										disabled={conditions.length === 1}
									>
										<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
										</svg>
									</button>
								</div>
							{/each}
						</div>

						<p class="mt-2 text-xs text-[var(--color-surface-text-muted)]">
							All conditions must be met. Books are added and removed automatically by the shelf rules.
						</p>
					</div>

					<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
						<div class="mb-3">
							<h4 class="text-base font-medium text-[var(--color-surface-text)]">Default Sort</h4>
							<p class="mt-1 text-xs text-[var(--color-surface-text-muted)]">Controls the initial order when this smart shelf opens.</p>
						</div>
						<div class="inline-flex max-w-full overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)]">
							<select
								bind:value={shelfSortBy}
								class="min-w-0 bg-transparent px-3 py-2 text-sm text-[var(--color-surface-text)] focus:outline-none"
								aria-label="Smart shelf default sort"
							>
								{#each shelfSortOptions as option}
									<option value={option.value}>{option.label}</option>
								{/each}
							</select>
							<button
								type="button"
								onclick={() => shelfSortDir = shelfSortDir === 'asc' ? 'desc' : 'asc'}
								class="border-l border-[var(--color-surface-border)] px-3 py-2 text-sm font-medium text-[var(--color-primary-400)] hover:bg-[var(--color-surface-base)]"
								aria-label={shelfSortDir === 'asc' ? 'Sort ascending' : 'Sort descending'}
								title={shelfSortDir === 'asc' ? 'Sort ascending' : 'Sort descending'}
							>
								{shelfSortDir === 'asc' ? 'Asc' : 'Desc'}
							</button>
						</div>
					</div>
				{:else}
					<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
						<p class="text-sm text-[var(--color-surface-text-muted)]">
							Manual shelves are edited from the shelf page. Use Add Books there to search, filter, and select books.
						</p>
					</div>
				{/if}

				{#if errorMessage}
					<div class="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">{errorMessage}</div>
				{/if}
			</div>

			<div class="flex justify-end gap-3 border-t border-[var(--color-surface-border)] px-6 py-4">
				<button
					type="button"
					onclick={closeModal}
					class="rounded-lg px-4 py-2 text-[var(--color-surface-text-muted)] transition-colors hover:text-[var(--color-surface-text)]"
				>
					Cancel
				</button>
				<button
					type="button"
					onclick={saveShelf}
					disabled={!shelfName.trim() || isSubmitting}
					class="accent-action rounded-lg px-4 py-2 font-medium transition-colors disabled:opacity-50"
				>
					{isSubmitting ? 'Saving...' : isEditing ? 'Save Shelf' : isMagicShelf ? 'Create Magic Shelf' : 'Create Shelf'}
				</button>
			</div>
		</div>
	</div>

	<LibraryIconPicker
		open={showShelfIconPicker}
		selectedIcon={shelfIcon}
		onSelect={selectShelfIcon}
		onClose={() => showShelfIconPicker = false}
	/>
{/if}

<style>
	.shelf-icon-preview :global(svg) {
		display: block;
		height: 100%;
		width: 100%;
		overflow: visible;
	}
</style>

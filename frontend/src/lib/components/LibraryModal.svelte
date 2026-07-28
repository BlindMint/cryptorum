<script lang="ts">
	import LibraryIconPicker from '$lib/components/LibraryIconPicker.svelte';
	import { parseLibraryIcon } from '$lib/utils/library-icons';

	type CoverMode = 'all' | 'missing';

	export interface LibraryModalLibrary {
		id?: number;
		name?: string;
		icon?: string;
		book_count?: number;
		exclude_from_suggestions?: boolean;
		comic_spread_fallback?: string;
		metadata_protection_enabled?: boolean;
		is_importing?: boolean;
		paths?: string[];
	}

	type Props = {
		open: boolean;
		library?: LibraryModalLibrary | null;
		isScanActive?: (library: LibraryModalLibrary) => boolean;
		onClose?: () => void;
		onSaved?: (result: { library: LibraryModalLibrary; isEditing: boolean; foldersChanged: boolean }) => void | Promise<void>;
		onScan?: (library: LibraryModalLibrary) => void | Promise<void>;
		onRegenerateCovers?: (library: LibraryModalLibrary, mode: CoverMode) => void | Promise<void>;
		onMetadataProtectionChanged?: (library: LibraryModalLibrary, enabled: boolean) => void | Promise<void>;
	};

	let {
		open,
		library = null,
		isScanActive,
		onClose,
		onSaved,
		onScan,
		onRegenerateCovers,
		onMetadataProtectionChanged
	}: Props = $props();

	const inheritedComicSpreadFallbackOptions = [
		{ value: 'inherit', label: 'Inherit' },
		{ value: 'right', label: 'Right side' },
		{ value: 'left', label: 'Left side' },
		{ value: 'disabled', label: 'Disabled' }
	];

	let form = $state({
		name: '',
		icon: '',
		exclude_from_suggestions: false,
		comic_spread_fallback: 'inherit',
		paths: ['']
	});
	let originalLibraryPaths = $state<string[]>([]);
	let isSaving = $state(false);
	let errorMessage = $state('');
	let showIconPicker = $state(false);
	let showDirectoryModal = $state(false);
	let currentDirectory = $state('/');
	let directoryContents = $state<any[]>([]);
	let directoryLoading = $state(false);
	let metadataProtectionEnabled = $state(false);
	let isSavingMetadataProtection = $state(false);
	let metadataProtectionMessage = $state('');

	let currentLibraryIcon = $derived(parseLibraryIcon(form.icon));
	let isEditing = $derived(!!library?.id);
	let libraryBookCount = $derived(Number(library?.book_count || 0));
	let libraryScanActive = $derived(!!library?.id && !!isScanActive?.(library));

	function resetForm() {
		const paths = library?.paths?.filter((path) => path.trim()) || [];
		form = {
			name: library?.name || '',
			icon: library?.icon || '',
			exclude_from_suggestions: !!library?.exclude_from_suggestions,
			comic_spread_fallback: library?.comic_spread_fallback || 'inherit',
			paths: paths.length ? [...paths] : ['']
		};
		originalLibraryPaths = normalizeLibraryPaths(paths);
		errorMessage = '';
		showIconPicker = false;
		showDirectoryModal = false;
		currentDirectory = '/';
		directoryContents = [];
		directoryLoading = false;
		metadataProtectionEnabled = !!library?.metadata_protection_enabled;
		isSavingMetadataProtection = false;
		metadataProtectionMessage = '';
	}

	function closeModal() {
		onClose?.();
	}

	function normalizeLibraryPaths(paths: string[]): string[] {
		return Array.from(new Set(paths.map((path) => path.trim()).filter(Boolean))).sort();
	}

	function libraryFolderSetChanged(nextPaths: string[]): boolean {
		const next = normalizeLibraryPaths(nextPaths);
		if (next.length !== originalLibraryPaths.length) return true;
		return next.some((path, index) => path !== originalLibraryPaths[index]);
	}

	function selectLibraryIcon(iconValue: string) {
		form.icon = iconValue;
	}

	function clearLibraryIcon() {
		form.icon = '';
	}

	function removePath(index: number) {
		form.paths = form.paths.filter((_, i) => i !== index);
		if (form.paths.length === 0) form.paths = [''];
	}

	async function openDirectoryModal() {
		showDirectoryModal = true;
		directoryContents = [];
		directoryLoading = true;
		try {
			const response = await fetch('/api/directories?path=/books');
			if (response.ok) {
				await loadDirectoryContents('/books');
			} else {
				await loadDirectoryContents('/');
			}
		} catch {
			await loadDirectoryContents('/');
		}
	}

	function closeDirectoryModal() {
		showDirectoryModal = false;
	}

	async function loadDirectoryContents(path: string) {
		currentDirectory = path;
		directoryLoading = true;
		directoryContents = [];
		try {
			const response = await fetch(`/api/directories?path=${encodeURIComponent(path)}`);
			directoryContents = response.ok ? await response.json() : [];
		} catch {
			directoryContents = [];
		} finally {
			directoryLoading = false;
		}
	}

	function selectDirectory(item: any) {
		if (item.type === 'directory') {
			void loadDirectoryContents(item.path);
		}
	}

	function addSelectedDirectory() {
		const newPath = currentDirectory;
		if (!form.paths.includes(newPath)) {
			form.paths = [...form.paths.filter((path) => path.trim()), newPath];
		}
		closeDirectoryModal();
	}

	async function saveLibrary() {
		if (isSaving) return;
		const filteredPaths = form.paths.map((path) => path.trim()).filter(Boolean);
		if (!form.name.trim() || filteredPaths.length === 0) return;

		isSaving = true;
		errorMessage = '';
		const wasEditing = isEditing;
		const foldersChanged = wasEditing && libraryFolderSetChanged(filteredPaths);
		const payload = {
			name: form.name.trim(),
			icon: form.icon.trim(),
			exclude_from_suggestions: form.exclude_from_suggestions,
			comic_spread_fallback: form.comic_spread_fallback,
			paths: filteredPaths
		};

		try {
			const response = await fetch(wasEditing ? `/api/libraries/${library?.id}` : '/api/libraries', {
				method: wasEditing ? 'PUT' : 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(payload)
			});

			if (!response.ok) {
				errorMessage = await response.text();
				return;
			}

			const responseData = await response.json().catch(() => null);
			const savedLibrary: LibraryModalLibrary = {
				...(library || {}),
				...(responseData || {}),
				...payload,
				id: Number(responseData?.id ?? library?.id),
				book_count: Number(responseData?.book_count ?? library?.book_count ?? 0)
			};
			await onSaved?.({ library: savedLibrary, isEditing: wasEditing, foldersChanged });
			closeModal();
		} catch (error) {
			console.error('Failed to save library:', error);
			errorMessage = 'Failed to save library.';
		} finally {
			isSaving = false;
		}
	}

	async function handleScanLibrary() {
		if (!library?.id || libraryScanActive) return;
		await onScan?.(library);
	}

	async function handleRegenerateCovers(mode: CoverMode) {
		if (!library?.id) return;
		await onRegenerateCovers?.(library, mode);
	}

	async function toggleMetadataProtection(event: Event) {
		const checkbox = event.currentTarget as HTMLInputElement;
		if (!library?.id || isSavingMetadataProtection) {
			checkbox.checked = metadataProtectionEnabled;
			return;
		}
		const enabled = checkbox.checked;
		if (!enabled && !confirm('Allow automatic metadata updates for this library? Fields changed by users will remain individually protected.')) {
			checkbox.checked = metadataProtectionEnabled;
			return;
		}
		const previousEnabled = metadataProtectionEnabled;
		metadataProtectionEnabled = enabled;
		isSavingMetadataProtection = true;
		metadataProtectionMessage = '';
		try {
			const response = await fetch(`/api/libraries/${library.id}/metadata-protection`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ enabled })
			});
			if (!response.ok) {
				metadataProtectionMessage = await response.text();
				metadataProtectionEnabled = previousEnabled;
				checkbox.checked = previousEnabled;
				return;
			}
			const result = await response.json();
			metadataProtectionEnabled = !!result.metadata_protection_enabled;
			metadataProtectionMessage = metadataProtectionEnabled
				? `Automatic metadata updates are disabled for ${libraryBookCount} existing books.`
				: 'Automatic metadata updates are allowed; user-edited fields remain protected.';
			await onMetadataProtectionChanged?.(
				{ ...library, metadata_protection_enabled: metadataProtectionEnabled },
				metadataProtectionEnabled
			);
		} catch (error) {
			console.error('Failed to update library metadata protection:', error);
			metadataProtectionMessage = 'Unable to update metadata protection.';
			metadataProtectionEnabled = previousEnabled;
			checkbox.checked = previousEnabled;
		} finally {
			isSavingMetadataProtection = false;
		}
	}

	$effect(() => {
		if (!open) return;
		resetForm();
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
			aria-label="Close library modal"
			onclick={closeModal}
		></button>

		<div class="relative z-10 flex max-h-[92vh] w-full max-w-3xl flex-col overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
			<div class="flex items-center justify-between gap-4 border-b border-[var(--color-surface-border)] px-4 py-3">
				<div>
					<h3 class="text-base font-semibold text-[var(--color-surface-text)]">{isEditing ? 'Edit Library' : 'Add Library'}</h3>
					<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">
						{isEditing ? 'Update library folders, covers, and display settings.' : 'Create a library from one or more book folders.'}
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

			<div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
				<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
					<div>
						<label for="library-modal-name" class="mb-1.5 block text-xs font-medium text-[var(--color-surface-text)]">
							Library Name
						</label>
						<input
							id="library-modal-name"
							type="text"
							bind:value={form.name}
							placeholder="Enter library name"
							class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
						>
					</div>

					<div>
						<div class="mb-1.5 block text-xs font-medium text-[var(--color-surface-text)]">Icon</div>
						<div class="flex gap-2">
							<button
								type="button"
								onclick={() => showIconPicker = true}
								class="flex min-w-0 flex-1 items-center gap-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-left transition-colors hover:border-[var(--color-primary-500)]/60 hover:bg-[var(--color-surface-overlay)]"
							>
								<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-[var(--color-surface-overlay)] text-[var(--color-primary-400)]">
									{#if currentLibraryIcon?.svg}
										<div class="library-icon-preview h-5 w-5">{@html currentLibraryIcon.svg}</div>
									{:else}
										<span class="text-xs font-semibold uppercase">{form.name.slice(0, 1) || '?'}</span>
									{/if}
								</div>
								<div class="min-w-0 flex-1">
									<div class="truncate text-sm font-medium text-[var(--color-surface-text)]">
										{currentLibraryIcon?.label || currentLibraryIcon?.name || 'Default icon'}
									</div>
									<div class="text-xs text-[var(--color-surface-text-muted)]">Click to choose a different icon</div>
								</div>
							</button>
							{#if form.icon}
								<button
									type="button"
									onclick={clearLibraryIcon}
									class="inline-flex h-[58px] shrink-0 items-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 text-xs font-medium text-[var(--color-surface-text-muted)] transition-colors hover:border-[var(--color-primary-500)]/50 hover:text-[var(--color-surface-text)]"
								>
									Default
								</button>
							{/if}
						</div>
					</div>
				</div>

				<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-4 py-3">
					<label for="library-exclude-suggestions" class="flex items-start justify-between gap-4">
						<span class="min-w-0">
							<span class="block text-xs font-medium text-[var(--color-surface-text)]">Exclude from discovery and recommendations</span>
							<span class="mt-1 block text-xs leading-5 text-[var(--color-surface-text-muted)]">
								Keep this library searchable and readable, but hide its books from dashboard discovery and similar book suggestions.
							</span>
						</span>
						<input
							id="library-exclude-suggestions"
							type="checkbox"
							bind:checked={form.exclude_from_suggestions}
							class="mt-1 h-4 w-4 shrink-0 rounded border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
						>
					</label>
				</div>

				<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
					<h4 class="text-xs font-semibold text-[var(--color-surface-text)]">Cover</h4>
					<div class="mt-3 space-y-3">
						<label for="library-comic-spread-fallback" class="block text-sm font-medium text-[var(--color-surface-text-muted)]">
							Wide Comic Fallback Cover
						</label>
						<select
							id="library-comic-spread-fallback"
							bind:value={form.comic_spread_fallback}
							class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
						>
							{#each inheritedComicSpreadFallbackOptions as option}
								<option value={option.value} class="bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)]">{option.label}</option>
							{/each}
						</select>

						{#if isEditing}
							<div class="grid gap-2 sm:grid-cols-2">
								<button
									type="button"
									onclick={() => handleRegenerateCovers('missing')}
									class="accent-action rounded-lg px-3 py-2 text-sm font-medium transition-colors"
								>
									Regenerate Missing Covers
								</button>
								<button
									type="button"
									onclick={() => handleRegenerateCovers('all')}
									class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-colors hover:border-[var(--color-primary-500)]/50 hover:text-[var(--color-primary-400)]"
								>
									Regenerate All Covers
								</button>
							</div>
						{/if}
					</div>
				</div>

				<div>
					<div class="mb-2 flex items-center justify-between gap-3">
						<div class="block text-xs font-medium text-[var(--color-surface-text)]">Book Folders</div>
						<button
							type="button"
							onclick={openDirectoryModal}
							class="accent-action rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
						>
							Add Folder
						</button>
					</div>
					<div class="min-h-[60px] space-y-2 rounded-lg border-2 border-dashed border-[var(--color-surface-border)] p-4">
						{#if form.paths.filter((path) => path.trim()).length === 0}
							<div class="py-4 text-center text-[var(--color-surface-text-muted)]">
								<svg class="mx-auto mb-2 h-8 w-8 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
								</svg>
								<p class="text-sm">No folders selected</p>
								<p class="mt-1 text-xs">Click Add Folder to select directories</p>
							</div>
						{:else}
							{#each form.paths.filter((path) => path.trim()) as path, index}
								<div class="flex items-center justify-between gap-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-3">
									<div class="flex min-w-0 items-center gap-3">
										<svg class="h-5 w-5 shrink-0 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
										</svg>
										<span class="min-w-0 truncate font-mono text-sm text-[var(--color-surface-text)]">{path}</span>
									</div>
									<button
										type="button"
										onclick={() => removePath(index)}
										class="rounded p-1 text-red-400 transition-colors hover:bg-red-500/10 hover:text-red-300"
										title="Remove folder"
										aria-label="Remove folder"
									>
										<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
										</svg>
									</button>
								</div>
							{/each}
						{/if}
					</div>
				</div>

				{#if isEditing}
					<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
						<div class="flex flex-wrap items-center justify-between gap-3">
							<div>
								<h4 class="text-xs font-semibold text-[var(--color-surface-text)]">Library Actions</h4>
								<p class="mt-1 text-xs text-[var(--color-surface-text-muted)]">Queue maintenance work for this library.</p>
							</div>
							<button
								type="button"
								onclick={handleScanLibrary}
								disabled={libraryScanActive}
								class="accent-action inline-flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors"
							>
								{#if libraryScanActive}
									<svg class="h-4 w-4 animate-scan-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
									</svg>
									Scan Running
								{:else}
									<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
									</svg>
									Scan Library
								{/if}
							</button>
						</div>
						<label
							class="mt-4 flex cursor-pointer items-start justify-between gap-4 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-3 transition-colors hover:border-[var(--color-primary-500)]/60"
							class:cursor-wait={isSavingMetadataProtection}
						>
							<span>
								<span class="block text-xs font-medium text-[var(--color-surface-text)]">Protect existing metadata</span>
								<span class="mt-1 block text-xs leading-5 text-[var(--color-surface-text-muted)]">
									Preserve current values during scans, refreshes, provider lookups, and cover regeneration. New books still receive their initial metadata.
								</span>
							</span>
							<input
								type="checkbox"
								checked={metadataProtectionEnabled}
								disabled={isSavingMetadataProtection}
								onchange={toggleMetadataProtection}
								class="mt-0.5 h-5 w-5 shrink-0 rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
							/>
						</label>
						{#if metadataProtectionMessage}
							<p class="mt-3 text-xs text-[var(--color-surface-text-muted)]">{metadataProtectionMessage}</p>
						{/if}
					</div>
				{/if}

				{#if errorMessage}
					<div class="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">{errorMessage}</div>
				{/if}
			</div>

			<div class="flex justify-end gap-3 border-t border-[var(--color-surface-border)] px-4 py-3">
				<button
					type="button"
					onclick={closeModal}
					class="rounded-lg px-4 py-2 text-[var(--color-surface-text-muted)] transition-colors hover:text-[var(--color-surface-text)]"
				>
					Cancel
				</button>
				<button
					type="button"
					onclick={saveLibrary}
					disabled={!form.name.trim() || form.paths.filter((path) => path.trim()).length === 0 || isSaving}
					class="accent-action rounded-lg px-4 py-2 font-medium transition-colors disabled:opacity-50"
				>
					{isSaving ? 'Saving...' : isEditing ? 'Save Library' : 'Create Library'}
				</button>
			</div>
		</div>

		<LibraryIconPicker
			open={showIconPicker}
			selectedIcon={form.icon}
			onSelect={selectLibraryIcon}
			onClose={() => showIconPicker = false}
		/>

		{#if showDirectoryModal}
			<div class="fixed inset-0 z-[140] flex items-center justify-center bg-black/50 p-4">
				<button
					type="button"
					class="absolute inset-0"
					aria-label="Close directory modal"
					onclick={closeDirectoryModal}
				></button>
				<div class="relative z-10 flex max-h-[80vh] w-full max-w-lg flex-col overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)]">
					<div class="flex-shrink-0 border-b border-[var(--color-surface-border)] px-4 py-3">
						<h3 class="text-base font-semibold text-[var(--color-surface-text)]">Select Directory</h3>
						<p class="mt-1 truncate font-mono text-sm text-[var(--color-surface-text-muted)]">{currentDirectory}</p>
					</div>
					<div class="flex-1 overflow-y-auto p-4">
						{#if directoryLoading}
							<div class="flex items-center justify-center py-8">
								<div class="h-8 w-8 animate-spin rounded-full border-b-2 border-[var(--color-primary-500)]"></div>
							</div>
						{:else}
							<div class="space-y-1">
								{#if currentDirectory !== '/'}
									<button
										type="button"
										onclick={() => loadDirectoryContents(currentDirectory.split('/').slice(0, -1).join('/') || '/')}
										class="flex w-full items-center rounded-lg px-3 py-2 text-left text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
									>
										<svg class="mr-3 h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
										</svg>
										..
									</button>
								{/if}
								{#each directoryContents as item}
									{#if item.type === 'directory'}
										<button
											type="button"
											onclick={() => selectDirectory(item)}
											class="flex w-full items-center rounded-lg px-3 py-2 text-left text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
										>
											<svg class="mr-3 h-5 w-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
											</svg>
											<span class="truncate">{item.name}</span>
										</button>
									{/if}
								{/each}
								{#if directoryContents.filter((item) => item.type === 'directory').length === 0}
									<div class="py-8 text-center text-[var(--color-surface-text-muted)]">No subdirectories found</div>
								{/if}
							</div>
						{/if}
					</div>
					<div class="flex justify-end gap-3 border-t border-[var(--color-surface-border)] px-4 py-3">
						<button
							type="button"
							onclick={closeDirectoryModal}
							class="rounded-lg px-4 py-2 text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]"
						>
							Cancel
						</button>
						<button
							type="button"
							onclick={addSelectedDirectory}
							class="accent-action rounded-lg px-4 py-2 font-medium transition-colors"
						>
							Add This Folder
						</button>
					</div>
				</div>
			</div>
		{/if}
	</div>
{/if}

<style>
	.library-icon-preview :global(svg) {
		display: block;
		height: 100%;
		width: 100%;
		overflow: visible;
	}
</style>

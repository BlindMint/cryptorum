<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import BookMetadataContext from '$lib/components/BookMetadataContext.svelte';
	import MetadataFields from '$lib/components/MetadataFields.svelte';
	import MetadataLookupModal from '$lib/components/MetadataLookupModal.svelte';
	import {
		buildMetadataPayload,
		comicSpreadFallbackOptions,
		createMetadataEditForm,
		parseAuthors,
		type MetadataEditForm
	} from '$lib/utils/metadata-edit';
	import {
		getMetadataEditSession,
		getMetadataEditUrl,
		type MetadataEditSelectionSession
	} from '$lib/utils/metadata-edit-session';

	let book = $state<any>(null);
	let files = $state<any[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let saveError = $state<string | null>(null);
	let regeneratingCover = $state(false);
	let coverFileInput: HTMLInputElement | null = $state(null);
	let coverUploading = $state(false);
	let showMetadataLookup = $state(false);
	let editForm = $state<MetadataEditForm>(createMetadataEditForm(null));
	let authorsList = $state<string[]>([]);
	let selectionSession = $state<MetadataEditSelectionSession | null>(null);

	type CoverMutationResponse = {
		cover_path?: string;
		cover_source?: string;
		cover_updated_on?: number;
	};

	$effect(() => {
		const bookId = $page.params.bookID;
		if (bookId) {
			loadSelectionSession();
			void fetchBook();
		}
	});

	function loadSelectionSession() {
		selectionSession = getMetadataEditSession($page.url.searchParams.get('selection'));
	}

	function getSelectionIndex(): number {
		const queryIndex = Number($page.url.searchParams.get('index') || 0);
		if (Number.isFinite(queryIndex) && queryIndex >= 0) return queryIndex;
		if (!selectionSession || !book?.id) return 0;
		return Math.max(0, selectionSession.bookIds.indexOf(book.id));
	}

	function hasSelectionNavigation(): boolean {
		return !!selectionSession && selectionSession.bookIds.length > 1 && (!book?.id || selectionSession.bookIds.includes(book.id));
	}

	function getPositionLabel(): string {
		if (!hasSelectionNavigation() || !selectionSession) return '';
		return `${Math.min(getSelectionIndex() + 1, selectionSession.bookIds.length)} of ${selectionSession.bookIds.length}`;
	}

	function initializeForm() {
		editForm = createMetadataEditForm(book);
		authorsList = parseAuthors(book?.authors || '[]');
	}

	async function fetchBook() {
		loading = true;
		saveError = null;
		const bookId = $page.params.bookID;
		try {
			const [bookResult, filesResult] = await Promise.allSettled([
				fetch(`/api/books/${bookId}`, { credentials: 'same-origin' }),
				fetch(`/api/books/${bookId}/files`, { credentials: 'same-origin' })
			]);
			const bookRes = bookResult.status === 'fulfilled' ? bookResult.value : null;
			const filesRes = filesResult.status === 'fulfilled' ? filesResult.value : null;

			if (bookRes?.status === 401 || filesRes?.status === 401) {
				await goto('/login');
				return;
			}
			if (bookRes?.ok) {
				book = await bookRes.json();
				initializeForm();
			} else {
				book = null;
			}
			files = filesRes?.ok ? await filesRes.json() : [];
		} catch (error) {
			console.error('Failed to load metadata editor:', error);
			book = null;
			files = [];
		} finally {
			loading = false;
		}
	}

	async function saveMetadata(): Promise<boolean> {
		if (!book?.id || saving) return false;
		saveError = null;
		saving = true;
		try {
			const res = await fetch(`/api/books/${book.id}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(buildMetadataPayload(editForm, authorsList))
			});
			if (!res.ok) {
				saveError = `Failed to save: ${res.status} ${await res.text()}`;
				return false;
			}
			await fetchBook();
			return true;
		} catch (error) {
			console.error('Failed to save metadata:', error);
			saveError = `Error: ${error}`;
			return false;
		} finally {
			saving = false;
		}
	}

	async function saveAndNext() {
		if (await saveMetadata()) {
			goToSelectionOffset(1);
		}
	}

	function goToSelectionOffset(offset: number) {
		if (!hasSelectionNavigation() || !selectionSession) return;
		const nextIndex = getSelectionIndex() + offset;
		if (nextIndex < 0 || nextIndex >= selectionSession.bookIds.length) return;
		const nextBookId = selectionSession.bookIds[nextIndex];
		goto(getMetadataEditUrl(nextBookId, selectionSession, nextIndex));
	}

	function backToSource() {
		if (hasSelectionNavigation() && selectionSession?.sourcePath) {
			goto(selectionSession.sourcePath);
			return;
		}
		if (book?.id) {
			goto(`/book/${book.id}`);
			return;
		}
		goto('/library');
	}

	function applyCoverMutation(update: CoverMutationResponse) {
		if (!book) return;
		book = {
			...book,
			cover_path: update.cover_path ?? book.cover_path,
			cover_source: update.cover_source ?? book.cover_source,
			cover_updated_on: update.cover_updated_on ?? book.cover_updated_on ?? Math.floor(Date.now() / 1000)
		};
	}

	async function regenerateCover() {
		if (!book?.id || regeneratingCover) return;
		regeneratingCover = true;
		try {
			const res = await fetch(`/api/books/${book.id}/cover/regenerate`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					comic_spread_fallback: editForm.comic_spread_fallback || book.comic_spread_fallback || 'inherit'
				})
			});
			if (res.ok) {
				applyCoverMutation(await res.json());
			} else {
				saveError = `Failed to regenerate cover: ${await res.text()}`;
			}
		} catch (error) {
			console.error('Failed to regenerate cover:', error);
			saveError = 'Failed to regenerate cover.';
		} finally {
			regeneratingCover = false;
		}
	}

	function openCoverPicker() {
		coverFileInput?.click();
	}

	async function uploadCustomCover(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file || !book?.id || coverUploading) return;
		coverUploading = true;
		saveError = null;
		try {
			const formData = new FormData();
			formData.append('cover', file);
			const res = await fetch(`/api/books/${book.id}/cover/custom`, {
				method: 'POST',
				body: formData
			});
			if (res.ok) {
				applyCoverMutation(await res.json());
			} else {
				saveError = await res.text();
			}
		} catch (error) {
			console.error('Failed to upload cover:', error);
			saveError = 'Failed to upload cover.';
		} finally {
			coverUploading = false;
			input.value = '';
		}
	}

	async function resetCustomCover() {
		if (!book?.id || coverUploading) return;
		if (!confirm('Remove the custom cover and restore the imported or generated cover?')) return;
		coverUploading = true;
		saveError = null;
		try {
			const res = await fetch(`/api/books/${book.id}/cover/custom`, { method: 'DELETE' });
			if (res.ok) {
				applyCoverMutation(await res.json());
			} else {
				saveError = await res.text();
			}
		} catch (error) {
			console.error('Failed to reset cover:', error);
			saveError = 'Failed to reset cover.';
		} finally {
			coverUploading = false;
		}
	}

	async function refreshAfterMetadataApply() {
		await fetchBook();
		showMetadataLookup = false;
	}
</script>

<div class="space-y-6">
	<div class="flex flex-wrap items-center justify-between gap-3">
		<button onclick={backToSource} class="group inline-flex items-center text-[var(--color-surface-text-muted)] transition-colors duration-200 ease-out hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]">
			<svg class="mr-2 h-4 w-4 transition-colors duration-200 ease-out group-hover:text-[var(--color-surface-text)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
			</svg>
			{hasSelectionNavigation() ? 'Back to Selection' : 'Back to Book Details'}
		</button>

		{#if hasSelectionNavigation() && selectionSession}
			<div class="flex items-center gap-2">
				<button
					type="button"
					onclick={() => goToSelectionOffset(-1)}
					disabled={getSelectionIndex() <= 0 || saving}
					class="inline-flex items-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
					aria-label="Previous selected book"
				>
					Prev
				</button>
				<span class="min-w-14 text-center text-xs font-medium uppercase tracking-[0.12em] text-[var(--color-surface-text-muted)]">{getPositionLabel()}</span>
				<button
					type="button"
					onclick={() => goToSelectionOffset(1)}
					disabled={getSelectionIndex() >= selectionSession.bookIds.length - 1 || saving}
					class="inline-flex items-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
					aria-label="Next selected book"
				>
					Next
				</button>
			</div>
		{/if}
	</div>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="h-12 w-12 animate-spin rounded-full border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if book}
		<div class="flex flex-col gap-6 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-6 lg:flex-row lg:items-start">
			<input bind:this={coverFileInput} type="file" accept="image/*" class="hidden" onchange={uploadCustomCover} />
			<div class="lg:w-72 lg:flex-shrink-0">
				<BookMetadataContext
					{book}
					{files}
					positionLabel={getPositionLabel()}
					framed={false}
					showCoverActions={true}
					coverActionLabel={coverUploading ? 'Saving...' : 'Edit'}
					coverActionBusy={coverUploading}
					onEditCover={openCoverPicker}
					onResetCover={resetCustomCover}
				/>
			</div>

			<div class="min-w-0 flex-1">
				<div class="mb-5 flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
					<div>
						<h1 class="text-2xl font-bold text-[var(--color-surface-text)]">Advanced Metadata</h1>
						<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">{book.title || 'Untitled'}</p>
					</div>
					<div class="flex flex-wrap items-center gap-2">
						<label class="flex items-center gap-2 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-1.5 text-sm text-[var(--color-surface-text)]">
							<span class="whitespace-nowrap text-[var(--color-surface-text-muted)]">Wide comic fallback</span>
							<select
								bind:value={editForm.comic_spread_fallback}
								class="rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-2 py-1 text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
							>
								{#each comicSpreadFallbackOptions as option}
									<option value={option.value} class="bg-[var(--color-surface-base)] text-[var(--color-surface-text)]">{option.label}</option>
								{/each}
							</select>
						</label>
						<button
							onclick={regenerateCover}
							type="button"
							disabled={regeneratingCover}
							class="group inline-flex items-center rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-1.5 text-sm text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-0.5 hover:bg-[var(--color-surface-600)] hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)] disabled:opacity-50"
						>
							{#if regeneratingCover}
								<svg class="mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
									<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
									<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
								</svg>
								Regenerating...
							{:else}
								<svg class="mr-2 h-4 w-4 transition-transform duration-200 ease-out group-hover:-rotate-45 group-hover:scale-110" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
								</svg>
								Regenerate Cover
							{/if}
						</button>
						<button
							onclick={() => showMetadataLookup = true}
							type="button"
							class="inline-flex items-center rounded-lg bg-[var(--color-primary-500)] px-3 py-1.5 text-sm text-white transition-all duration-200 ease-out hover:-translate-y-0.5 hover:bg-[var(--color-primary-600)] hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]"
						>
							Lookup Metadata
						</button>
					</div>
				</div>

				<MetadataFields bind:editForm bind:authorsList />

				{#if saveError}
					<div class="mt-4 rounded-lg border border-red-500/50 bg-red-500/20 p-3 text-sm text-red-400">
						{saveError}
					</div>
				{/if}

				<div class="mt-6 flex flex-wrap items-center justify-between gap-3">
					<div></div>
					<div class="flex items-center gap-2">
						<button onclick={backToSource} disabled={saving} class="rounded-lg bg-[var(--color-surface-700)] px-4 py-2 font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm disabled:opacity-50">Cancel</button>
						<button onclick={saveMetadata} disabled={saving} class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-4 py-2 font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-overlay)] disabled:opacity-50">
							{saving ? 'Saving...' : 'Save'}
						</button>
						{#if hasSelectionNavigation() && selectionSession}
							<button onclick={saveAndNext} disabled={saving || getSelectionIndex() >= selectionSession.bookIds.length - 1} class="rounded-lg bg-[var(--color-primary-500)] px-4 py-2 font-medium text-white transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-primary-600)] hover:shadow-md disabled:opacity-50">
								{saving ? 'Saving...' : 'Save & Next'}
							</button>
						{:else}
							<button onclick={async () => { if (await saveMetadata()) backToSource(); }} disabled={saving} class="rounded-lg bg-[var(--color-primary-500)] px-4 py-2 font-medium text-white transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-primary-600)] hover:shadow-md disabled:opacity-50">
								{saving ? 'Saving...' : 'Save & Close'}
							</button>
						{/if}
					</div>
				</div>
			</div>
		</div>
	{:else}
		<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-16 text-center">
			<p class="text-[var(--color-surface-text-muted)]">Book not found</p>
		</div>
	{/if}
</div>

{#if showMetadataLookup && book?.id}
	<MetadataLookupModal
		bookIds={[book.id]}
		title="Lookup Metadata"
		confirmBeforeApply={true}
		onClose={() => showMetadataLookup = false}
		onApplied={refreshAfterMetadataApply}
	/>
{/if}

<script lang="ts">
	interface Props {
		bookId: number;
		lockedFields?: string[];
		libraryProtectionEnabled?: boolean;
		onChanged?: (fields: string[]) => void | Promise<void>;
		onRestored?: (book: any) => void | Promise<void>;
	}

	let {
		bookId,
		lockedFields = [],
		libraryProtectionEnabled = false,
		onChanged,
		onRestored
	}: Props = $props();

	const allFields = [
		'title',
		'authors',
		'series',
		'series_number',
		'publisher',
		'pub_date',
		'description',
		'rating',
		'tags',
		'isbn',
		'asin',
		'language',
		'page_count',
		'cover'
	];

	const labels: Record<string, string> = {
		title: 'Title',
		authors: 'Authors',
		series: 'Series',
		series_number: 'Series number',
		publisher: 'Publisher',
		pub_date: 'Publication date',
		description: 'Description',
		rating: 'Rating',
		tags: 'Tags',
		isbn: 'ISBN',
		asin: 'ASIN',
		language: 'Language',
		page_count: 'Page count',
		cover: 'Cover'
	};

	let saving = $state(false);
	let error = $state('');
	let revisions = $state<{ id: number; changed_at: number; change_source: string; changed_fields: string[] }[]>([]);
	let protectedFields = $derived(Array.from(new Set(lockedFields)).filter((field) => allFields.includes(field)).sort());

	$effect(() => {
		if (!bookId) return;
		lockedFields.join('|');
		void loadRevisions(bookId);
	});

	async function loadRevisions(targetBookId: number) {
		try {
			const response = await fetch(`/api/books/${targetBookId}/metadata/revisions`);
			if (response.ok && targetBookId === bookId) {
				const result = await response.json();
				revisions = Array.isArray(result) ? result : [];
			}
		} catch {
			revisions = [];
		}
	}

	async function updateProtection(action: 'lock' | 'unlock', fields: string[]) {
		if (!bookId || fields.length === 0 || saving) return;
		saving = true;
		error = '';
		try {
			const response = await fetch(`/api/metadata/${action}`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ book_id: bookId, fields })
			});
			if (!response.ok) {
				error = await response.text();
				return;
			}
			const result = await response.json();
			await onChanged?.(Array.isArray(result.locked_fields) ? result.locked_fields : []);
		} catch (reason) {
			console.error('Failed to update metadata protection:', reason);
			error = 'Unable to update metadata protection.';
		} finally {
			saving = false;
		}
	}

	async function restoreLatestRevision() {
		const revision = revisions[0];
		if (!revision || saving) return;
		const fieldNames = revision.changed_fields.map((field) => labels[field] || field).join(', ');
		if (!confirm(`Restore the previous values for ${fieldNames || 'this metadata change'}?`)) return;
		saving = true;
		error = '';
		try {
			const response = await fetch(`/api/books/${bookId}/metadata/revisions/${revision.id}/restore`, { method: 'POST' });
			if (!response.ok) {
				error = await response.text();
				return;
			}
			const result = await response.json();
			await onRestored?.(result.book);
			await loadRevisions(bookId);
		} catch (reason) {
			console.error('Failed to restore metadata revision:', reason);
			error = 'Unable to restore the previous metadata values.';
		} finally {
			saving = false;
		}
	}

	function allowAutomaticUpdates() {
		if (protectedFields.length === 0) return;
		if (!confirm('Allow scans, extraction, and metadata refreshes to update all currently protected fields for this book?')) return;
		void updateProtection('unlock', protectedFields);
	}
</script>

<div class="rounded-xl border border-[var(--color-primary-500)]/25 bg-[var(--color-primary-500)]/[0.06] p-4">
	<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
		<div class="min-w-0">
			<div class="flex items-center gap-2">
				<svg class="h-4 w-4 shrink-0 text-[var(--color-primary-400)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
					<rect x="5" y="10" width="14" height="10" rx="2"></rect>
					<path d="M8 10V7a4 4 0 0 1 8 0v3"></path>
				</svg>
				<h3 class="text-sm font-semibold text-[var(--color-surface-text)]">Metadata protection</h3>
			</div>
			<p class="mt-1 text-xs leading-5 text-[var(--color-surface-text-muted)]">
				User-edited fields are protected automatically. Scans and metadata refreshes can only update unprotected fields.
			</p>
			{#if libraryProtectionEnabled}
				<p class="mt-2 text-xs font-medium text-[var(--color-primary-300)]">
					This book is protected by its library setting. Turn that setting off under General → Libraries before allowing automatic updates.
				</p>
			{/if}
			{#if protectedFields.length > 0}
				<p class="mt-2 text-xs text-[var(--color-primary-300)]">
					Individually protected: {protectedFields.map((field) => labels[field] || field).join(', ')}
				</p>
			{:else if libraryProtectionEnabled}
				<p class="mt-2 text-xs text-[var(--color-surface-text-muted)]">No additional field-level protections.</p>
			{:else}
				<p class="mt-2 text-xs text-amber-300">No fields are protected yet.</p>
			{/if}
		</div>
		<div class="flex shrink-0 flex-wrap gap-2">
			<button
				type="button"
				onclick={() => updateProtection('lock', allFields)}
				disabled={saving || protectedFields.length === allFields.length}
				class="rounded-lg border border-[var(--color-primary-500)]/40 bg-[var(--color-primary-500)]/10 px-3 py-2 text-xs font-semibold text-[var(--color-primary-200)] transition-colors hover:bg-[var(--color-primary-500)]/20 disabled:opacity-50"
			>
				Protect all
			</button>
			<button
				type="button"
				onclick={allowAutomaticUpdates}
				disabled={saving || protectedFields.length === 0 || libraryProtectionEnabled}
				class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-xs font-semibold text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-600)] hover:text-[var(--color-surface-text)] disabled:opacity-50"
				title={libraryProtectionEnabled ? 'Turn off library metadata protection first' : 'Allow automatic metadata updates'}
			>
				Allow automatic updates
			</button>
			{#if revisions.length > 0}
				<button
					type="button"
					onclick={restoreLatestRevision}
					disabled={saving}
					class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-2 text-xs font-semibold text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-600)] hover:text-[var(--color-surface-text)] disabled:opacity-50"
					title={`Restore ${revisions[0].changed_fields.map((field) => labels[field] || field).join(', ')}`}
				>
					Restore previous
				</button>
			{/if}
		</div>
	</div>
	{#if error}
		<p class="mt-2 text-xs text-red-300">{error}</p>
	{/if}
</div>

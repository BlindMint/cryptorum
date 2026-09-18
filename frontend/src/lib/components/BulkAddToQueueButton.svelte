<script lang="ts">
	import { onDestroy } from 'svelte';
	import { audioPlayer } from '$lib/stores/audioPlayer';

	let { bookIds, disabled = false, title = 'Add audio from this selection to the queue', label = 'Add to queue' } = $props<{
		bookIds: number[];
		disabled?: boolean;
		title?: string;
		label?: string;
	}>();
	let adding = $state(false);
	let resultLabel = $state('');
	let resultTimer: ReturnType<typeof setTimeout> | null = null;

	async function addSelectedAudio() {
		if (adding || disabled || bookIds.length === 0) return;
		adding = true;
		resultLabel = '';
		const result = await audioPlayer.addBooksToQueue(bookIds);
		adding = false;
		if (result.failed) {
			resultLabel = 'Unable to add';
		} else if (result.addedCount > 0) {
			resultLabel = `Added ${result.addedCount}${result.skippedCount > 0 ? ` · ${result.skippedCount} skipped` : ''}`;
		} else {
			resultLabel = result.skippedCount > 0 ? 'No new audio' : 'Nothing added';
		}
		if (resultTimer) clearTimeout(resultTimer);
		resultTimer = setTimeout(() => resultLabel = '', 4000);
	}

	onDestroy(() => {
		if (resultTimer) clearTimeout(resultTimer);
	});
</script>

<button
	type="button"
	onclick={addSelectedAudio}
	disabled={disabled || adding || bookIds.length === 0}
	{title}
	class="flex items-center gap-2 rounded-lg bg-[var(--color-surface-700)] px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] disabled:opacity-50 disabled:hover:translate-y-0 disabled:hover:shadow-none"
>
	{#if adding}
		<svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true"><circle class="opacity-25" cx="12" cy="12" r="9" stroke="currentColor" stroke-width="3"/><path class="opacity-80" fill="currentColor" d="M12 3a9 9 0 0 1 9 9h-3a6 6 0 0 0-6-6V3Z"/></svg>
	{:else}
		<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M3 6h10M3 12h7M3 18h5"/><path d="M17 13v8m-4-4h8"/></svg>
	{/if}
	<span>{adding ? 'Adding…' : resultLabel || label}</span>
</button>

<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { audioPlayer } from '$lib/stores/audioPlayer';

	let error = $state('');

	onMount(() => {
		let disposed = false;
		async function openPlayer() {
			const bookID = Number($page.params.bookID);
			const fileID = Number($page.url.searchParams.get('file_id')) || undefined;
			if (!Number.isInteger(bookID) || bookID <= 0) {
				error = 'This audio item could not be found.';
				return;
			}
			await audioPlayer.playBook(bookID, fileID);
			if (disposed) return;
			const requestedReturn = $page.url.searchParams.get('return');
			const destination = requestedReturn?.startsWith('/') && !requestedReturn.startsWith('//')
				? requestedReturn
				: `/book/${bookID}`;
			await goto(destination, { replaceState: true });
		}
		void openPlayer();
		return () => { disposed = true; };
	});
</script>

<div class="flex min-h-[100dvh] items-center justify-center bg-[var(--color-surface-base)] p-6">
	{#if error}
		<div class="max-w-md rounded-xl border border-red-500/30 bg-red-500/10 p-5 text-center">
			<p class="text-sm text-red-300">{error}</p>
			<a href="/library" class="mt-3 inline-block text-sm text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">Return to library</a>
		</div>
	{:else}
		<div class="text-center text-[var(--color-surface-text-muted)]" aria-live="polite">
			<div class="mx-auto mb-3 h-8 w-8 animate-spin rounded-full border-2 border-[var(--color-surface-border)] border-t-[var(--color-primary-500)]"></div>
			<p class="text-sm">Opening audio player…</p>
		</div>
	{/if}
</div>

<script lang="ts">
	import { onMount } from 'svelte';

	let visible = $state(false);
	let message = $state('');
	let reloadRemote: (() => void) | null = null;
	let takeOver: (() => Promise<void>) | null = null;
	let working = $state(false);

	onMount(() => {
		function handleConflict(event: Event) {
			const detail = (event as CustomEvent).detail;
			message = detail.message;
			reloadRemote = detail.reloadRemote;
			takeOver = detail.takeOver;
			visible = true;
		}
		window.addEventListener('cryptorum-progress-conflict', handleConflict);
		return () => window.removeEventListener('cryptorum-progress-conflict', handleConflict);
	});

	async function handleTakeOver() {
		if (!takeOver || working) return;
		working = true;
		try {
			await takeOver();
			visible = false;
		} finally {
			working = false;
		}
	}
</script>

{#if visible}
	<div class="fixed inset-x-3 top-3 z-[1000] mx-auto max-w-xl rounded-xl border border-amber-400/40 bg-[var(--color-surface-overlay)] p-4 text-[var(--color-surface-text)] shadow-2xl" role="alertdialog" aria-live="assertive">
		<p class="text-sm">{message}</p>
		<div class="mt-3 flex justify-end gap-2">
			<button class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]" type="button" onclick={() => reloadRemote?.()}>Reload Remote</button>
			<button class="accent-action rounded-lg px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-base)]" type="button" disabled={working} onclick={handleTakeOver}>
				{working ? 'Taking over…' : 'Take Over'}
			</button>
		</div>
	</div>
{/if}

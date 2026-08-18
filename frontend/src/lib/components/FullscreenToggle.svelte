<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { subscribeFullscreenState, toggleAppFullscreen } from '$lib/utils/fullscreen';

	let fullscreen = $state(false);
	let unavailable = $state(false);
	let unsubscribe: (() => void) | null = null;

	onMount(() => {
		unavailable = !document.fullscreenEnabled;
		unsubscribe = subscribeFullscreenState((enabled) => {
			fullscreen = enabled;
		});
	});

	onDestroy(() => {
		unsubscribe?.();
	});

	function toggleFullscreen() {
		toggleAppFullscreen(false).catch((error) => {
			console.error('Fullscreen toggle failed:', error);
		});
	}
</script>

<button
	type="button"
	onclick={toggleFullscreen}
	disabled={unavailable}
	class="shrink-0 rounded-md p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)] disabled:cursor-not-allowed disabled:opacity-40"
	aria-label={fullscreen ? 'Exit true fullscreen' : 'Enter true fullscreen'}
	title={fullscreen ? 'Exit true fullscreen' : 'Enter true fullscreen'}
>
	{#if fullscreen}
		<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 3v6H3M15 3v6h6M9 21v-6H3M15 21v-6h6"></path>
		</svg>
	{:else}
		<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5"></path>
		</svg>
	{/if}
</button>

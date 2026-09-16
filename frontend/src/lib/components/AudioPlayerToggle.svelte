<script lang="ts">
	import { audioPlayer } from '$lib/stores/audioPlayer';
</script>

<button
	type="button"
	class="relative rounded-md p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]/70"
	title="Audio player and queue"
	aria-label="Open audio player and queue"
	onclick={() => audioPlayer.expand(true)}
>
	<svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
		<path stroke-linecap="round" stroke-linejoin="round" d="M9 18V5l11-2v13"></path>
		<circle cx="6" cy="18" r="3"></circle>
		<circle cx="17" cy="16" r="3"></circle>
	</svg>
	{#if $audioPlayer.items.length > 0}
		<span class="absolute right-0 top-0 min-w-4 rounded-full bg-[var(--color-primary-500)] px-1 text-center text-[9px] font-bold leading-4 text-white">
			{$audioPlayer.items.length > 99 ? '99+' : $audioPlayer.items.length}
		</span>
	{/if}
</button>

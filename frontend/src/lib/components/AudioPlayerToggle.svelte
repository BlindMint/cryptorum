<script lang="ts">
	import { audioPlayer } from '$lib/stores/audioPlayer';

	const playerOpen = $derived($audioPlayer.expanded && !$audioPlayer.dismissed);
	const queueItemLabel = $derived(`${$audioPlayer.items.length} ${$audioPlayer.items.length === 1 ? 'item' : 'items'}`);
	const actionLabel = $derived(
		playerOpen
			? $audioPlayer.items.length > 0
				? `Minimize audio player and queue (${queueItemLabel})`
				: 'Close empty audio player and queue'
			: $audioPlayer.items.length > 0
				? `Open audio player and queue (${queueItemLabel})`
				: 'Open audio player and queue'
	);

	function toggleAudioPlayer() {
		if (!playerOpen) {
			audioPlayer.expand(true);
			return;
		}
		if ($audioPlayer.items.length === 0) {
			audioPlayer.dismiss();
		} else {
			audioPlayer.minimize();
		}
	}
</script>

<button
	type="button"
	class="relative rounded-md p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]/70"
	title={actionLabel}
	aria-label={actionLabel}
	aria-expanded={playerOpen}
	onclick={toggleAudioPlayer}
>
	<svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
		<path stroke-linecap="round" stroke-linejoin="round" d="M9 18V5l11-2v13"></path>
		<circle cx="6" cy="18" r="3"></circle>
		<circle cx="17" cy="16" r="3"></circle>
	</svg>
	{#if $audioPlayer.items.length > 0}
		<span class="passive-count-indicator absolute -right-0.5 -top-0.5" aria-hidden="true">
			{$audioPlayer.items.length > 99 ? '99+' : $audioPlayer.items.length}
		</span>
	{/if}
</button>

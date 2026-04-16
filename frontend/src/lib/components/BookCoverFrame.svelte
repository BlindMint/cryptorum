<script lang="ts">
	import BookCoverPlaceholder from '$lib/components/BookCoverPlaceholder.svelte';

	interface Props {
		src?: string | null;
		alt?: string;
		mode?: 'cover' | 'contain';
		frameClass?: string;
		imageClass?: string;
		placeholderSize?: 'sm' | 'md' | 'lg';
	}

	let {
		src = null,
		alt = '',
		mode = 'cover',
		frameClass = '',
		imageClass = '',
		placeholderSize = 'lg'
	}: Props = $props();

	let fitClass = $derived(mode === 'contain' ? 'object-contain p-3' : 'object-cover');
</script>

<div class={`relative overflow-hidden bg-[var(--color-surface-700)] rounded-lg border border-[var(--color-surface-border)] ${frameClass}`}>
	{#if src}
		<img src={src} alt={alt} class={`w-full h-full ${fitClass} ${imageClass}`} />
	{:else}
		<BookCoverPlaceholder size={placeholderSize} class="bg-transparent" />
	{/if}
</div>

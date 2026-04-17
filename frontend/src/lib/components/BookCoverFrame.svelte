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

	let fitClass = $derived(mode === 'contain' ? 'object-contain' : 'object-cover');
</script>

<div class={`relative overflow-hidden rounded-lg bg-[var(--color-surface-700)] ${frameClass}`}>
	{#if src}
		<img src={src} alt={alt} class={`w-full h-full ${fitClass} ${imageClass}`} />
	{:else}
		<BookCoverPlaceholder size={placeholderSize} />
	{/if}
</div>

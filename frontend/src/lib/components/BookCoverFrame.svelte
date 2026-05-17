<script lang="ts">
	import { onDestroy } from 'svelte';
	import BookCoverPlaceholder from '$lib/components/BookCoverPlaceholder.svelte';

	interface Props {
		src?: string | null;
		alt?: string;
		mode?: 'cover' | 'contain';
		frameClass?: string;
		imageClass?: string;
		placeholderSize?: 'sm' | 'md' | 'lg';
		loading?: 'eager' | 'lazy';
		decoding?: 'sync' | 'async' | 'auto';
		retryCount?: number;
		retryDelayMs?: number;
	}

	let {
		src = null,
		alt = '',
		mode = 'cover',
		frameClass = '',
		imageClass = '',
		placeholderSize = 'lg',
		loading = 'lazy',
		decoding = 'async',
		retryCount = 0,
		retryDelayMs = 500
	}: Props = $props();

	let fitClass = $derived(mode === 'contain' ? 'object-contain' : 'object-cover');
	let retryAttempt = $state(0);
	let imageFailed = $state(false);
	let retryTimer: ReturnType<typeof setTimeout> | null = null;
	let imageSrc = $derived(src && !imageFailed ? withRetryParam(src, retryAttempt) : null);

	function clearRetryTimer() {
		if (retryTimer) {
			clearTimeout(retryTimer);
			retryTimer = null;
		}
	}

	function withRetryParam(value: string, attempt: number) {
		if (attempt <= 0) return value;

		const [pathAndQuery, hash = ''] = value.split('#');
		const separator = pathAndQuery.includes('?') ? '&' : '?';
		const retryPath = `${pathAndQuery}${separator}cover_retry=${attempt}`;
		return hash ? `${retryPath}#${hash}` : retryPath;
	}

	function handleImageError() {
		clearRetryTimer();

		if (!src || retryAttempt >= retryCount) {
			imageFailed = true;
			return;
		}

		const nextAttempt = retryAttempt + 1;
		retryTimer = setTimeout(() => {
			retryAttempt = nextAttempt;
		}, retryDelayMs * nextAttempt);
	}

	$effect(() => {
		src;
		clearRetryTimer();
		retryAttempt = 0;
		imageFailed = false;

		return clearRetryTimer;
	});

	onDestroy(clearRetryTimer);
</script>

<div class={`relative overflow-hidden rounded-lg bg-[var(--color-surface-700)] ${frameClass}`}>
	{#if imageSrc}
		<img src={imageSrc} alt={alt} {loading} {decoding} class={`w-full h-full ${fitClass} ${imageClass}`} onerror={handleImageError} />
	{:else}
		<BookCoverPlaceholder size={placeholderSize} />
	{/if}
</div>

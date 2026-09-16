<script lang="ts">
	import type { BookMediaKind } from '$lib/utils/book-formats';

	interface Props {
		size?: 'xs' | 'sm' | 'md' | 'lg';
		kind?: BookMediaKind;
		class?: string;
	}

	let { size = 'md', kind = 'book', class: className = '' }: Props = $props();

	const sizeClasses = {
		xs: 'w-8 h-8',
		sm: 'w-12 h-12',
		md: 'w-16 h-16',
		lg: 'w-24 h-24'
	};
</script>

<div
	class="relative h-full w-full overflow-hidden {className}"
	style="background: linear-gradient(145deg, var(--color-cover-placeholder-accent), var(--color-cover-placeholder-base)); box-shadow: inset 0 0 0 1px var(--color-cover-placeholder-border);"
>
	<div
		class="absolute inset-0"
		style="background: radial-gradient(circle at 28% 18%, rgba(255, 255, 255, 0.14), transparent 46%), radial-gradient(circle at 72% 84%, rgba(0, 0, 0, 0.14), transparent 36%);"
	></div>
	<div class="relative flex h-full w-full items-center justify-center">
		{#if kind === 'audio'}
			<svg
				class="{sizeClasses[size]} text-[var(--color-cover-placeholder-icon)] drop-shadow-sm"
				fill="none"
				stroke="currentColor"
				viewBox="0 0 24 24"
				aria-hidden="true"
			>
				<path
					stroke-linecap="round"
					stroke-linejoin="round"
					stroke-width="1.75"
					d="M9 17.5V6l11-2v11.5"
				></path>
				<path
					stroke-linecap="round"
					stroke-linejoin="round"
					stroke-width="1.75"
					d="M9 10l11-2"
				></path>
				<ellipse cx="5.75" cy="18" rx="3.25" ry="2.5" transform="rotate(-12 5.75 18)" stroke-width="1.75"></ellipse>
				<ellipse cx="16.75" cy="16" rx="3.25" ry="2.5" transform="rotate(-12 16.75 16)" stroke-width="1.75"></ellipse>
			</svg>
		{:else}
			<svg
				class="{sizeClasses[size]} text-[var(--color-cover-placeholder-icon)] drop-shadow-sm"
				fill="none"
				stroke="currentColor"
				viewBox="0 0 24 24"
				aria-hidden="true"
			>
				<path
					stroke-linecap="round"
					stroke-linejoin="round"
					stroke-width="1.8"
					d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"
				></path>
			</svg>
		{/if}
	</div>
</div>

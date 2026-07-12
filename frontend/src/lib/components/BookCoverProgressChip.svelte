<script lang="ts">
	import { formatCoverProgressPercent, getProgressChipAriaLabel } from '$lib/utils/cover-progress';

	interface Props {
		href: string;
		percent: number;
		title?: string | null;
		format?: string | null;
		/** When true, chip is visible but not interactive (e.g. bulk select). */
		disabled?: boolean;
	}

	let { href, percent, title = null, format = null, disabled = false }: Props = $props();

	let rounded = $derived(formatCoverProgressPercent(percent));
	let ariaLabel = $derived(getProgressChipAriaLabel(title, percent, format));
</script>

{#if disabled}
	<span
		class="cover-progress-chip cover-progress-chip--disabled"
		aria-hidden="true"
	>
		{rounded}%
	</span>
{:else}
	<a
		{href}
		class="cover-progress-chip"
		aria-label={ariaLabel}
		onclick={(event) => event.stopPropagation()}
	>
		{rounded}%
	</a>
{/if}

<style>
	.cover-progress-chip {
		position: absolute;
		bottom: 0.4rem;
		left: 0.4rem;
		z-index: 20;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 2rem;
		min-height: 1.35rem;
		padding: 0.15rem 0.45rem;
		border-radius: 9999px;
		border: 1px solid rgba(255, 255, 255, 0.18);
		background: color-mix(in srgb, #0f172a 62%, transparent);
		color: #f8fafc;
		font-size: 10px;
		font-weight: 650;
		line-height: 1;
		letter-spacing: 0.01em;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.35);
		backdrop-filter: blur(6px);
		text-decoration: none;
		transition: background-color 140ms ease, border-color 140ms ease, transform 140ms ease;
	}

	a.cover-progress-chip:hover {
		background: color-mix(in srgb, #0f172a 78%, transparent);
		border-color: rgba(255, 255, 255, 0.28);
		transform: scale(1.04);
	}

	a.cover-progress-chip:focus-visible {
		outline: 2px solid var(--color-primary-500);
		outline-offset: 2px;
	}

	.cover-progress-chip--disabled {
		opacity: 0.72;
		pointer-events: none;
	}
</style>

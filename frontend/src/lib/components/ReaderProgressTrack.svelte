<script lang="ts">
	type Variant = 'top' | 'bottom';

	let {
		element = $bindable<HTMLElement | null>(null),
		progress = 0,
		visible = true,
		variant = 'top' as Variant,
		metaAlign = 'between' as 'between' | 'center',
		interactive = false,
		showThumb = false,
		label = '',
		detail = '',
		previewLabel = '',
		previewProgress = null as number | null,
		ariaLabel = 'Reading progress',
		ariaValueMin = 0,
		ariaValueMax = 100,
		ariaValueNow = 0,
		onpointerdown,
		onpointerenter,
		onpointerleave,
		onpointermove,
		onpointerup,
		onpointercancel,
		onkeydown
	}: {
		element?: HTMLElement | null;
		progress?: number;
		visible?: boolean;
		variant?: Variant;
		metaAlign?: 'between' | 'center';
		interactive?: boolean;
		showThumb?: boolean;
		label?: string;
		detail?: string;
		previewLabel?: string;
		previewProgress?: number | null;
		ariaLabel?: string;
		ariaValueMin?: number;
		ariaValueMax?: number;
		ariaValueNow?: number;
		onpointerdown?: (event: PointerEvent) => void;
		onpointerenter?: (event: PointerEvent) => void;
		onpointerleave?: (event: PointerEvent) => void;
		onpointermove?: (event: PointerEvent) => void;
		onpointerup?: (event: PointerEvent) => void;
		onpointercancel?: (event: PointerEvent) => void;
		onkeydown?: (event: KeyboardEvent) => void;
	} = $props();

	const clampedProgress = $derived(Math.max(0, Math.min(100, progress)));
	const clampedPreviewProgress = $derived(
		previewProgress === null ? null : Math.max(0, Math.min(100, previewProgress))
	);
	const showPreview = $derived(
		interactive && variant === 'bottom' && previewLabel && clampedPreviewProgress !== null
	);
</script>

{#if interactive}
	<div
		bind:this={element}
		class="reader-progress reader-progress-{variant} reader-progress-interactive"
		class:reader-progress-meta-center={metaAlign === 'center'}
		class:reader-progress-hidden={!visible}
		style="--reader-progress: {clampedProgress}%; --reader-preview-progress: {clampedPreviewProgress ?? 0}%;"
		role="slider"
		aria-label={ariaLabel}
		aria-valuemin={ariaValueMin}
		aria-valuemax={ariaValueMax}
		aria-valuenow={ariaValueNow}
		tabindex="0"
		onpointerdown={onpointerdown}
		onpointerenter={onpointerenter}
		onpointerleave={onpointerleave}
		onpointermove={onpointermove}
		onpointerup={onpointerup}
		onpointercancel={onpointercancel}
		onkeydown={onkeydown}
	>
		{#if variant === 'bottom' && (label || detail)}
			<div class="reader-progress-meta">
				<span>{label}</span>
				<span>{detail}</span>
			</div>
		{/if}
		<div class="reader-progress-track">
			<div class="reader-progress-fill"></div>
			{#if showPreview}
				<div class="reader-progress-preview-marker" role="presentation"></div>
				<div class="reader-progress-preview-tooltip" role="presentation">{previewLabel}</div>
			{/if}
			{#if showThumb}
				<div class="reader-progress-thumb" role="presentation"></div>
			{/if}
		</div>
	</div>
{:else}
	<div
		bind:this={element}
		class="reader-progress reader-progress-{variant}"
		class:reader-progress-meta-center={metaAlign === 'center'}
		class:reader-progress-hidden={!visible}
		style="--reader-progress: {clampedProgress}%;"
		role="progressbar"
		aria-label={ariaLabel}
		aria-valuemin={ariaValueMin}
		aria-valuemax={ariaValueMax}
		aria-valuenow={ariaValueNow}
	>
		{#if variant === 'bottom' && (label || detail)}
			<div class="reader-progress-meta">
				<span>{label}</span>
				<span>{detail}</span>
			</div>
		{/if}
		<div class="reader-progress-track">
			<div class="reader-progress-fill"></div>
			{#if showThumb}
				<div class="reader-progress-thumb" role="presentation"></div>
			{/if}
		</div>
	</div>
{/if}

<style>
	.reader-progress {
		--reader-progress-accent: var(--color-primary-500, #22c55e);
		--reader-progress-track: var(--color-surface-border, rgba(55, 65, 81, 0.65));
		transition: opacity 0.22s ease, transform 0.22s ease;
	}

	.reader-progress-top {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		height: 5px;
		pointer-events: none;
	}

	.reader-progress-bottom {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		z-index: 115;
		padding: 10px 18px calc(32px + env(safe-area-inset-bottom));
		background: var(--color-surface-base, #0f172a);
		border-top: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.6));
		box-shadow: 0 -8px 28px rgba(0, 0, 0, 0.28);
	}

	.reader-progress-hidden {
		opacity: 0;
		pointer-events: none;
	}

	.reader-progress-bottom.reader-progress-hidden {
		transform: translateY(100%);
	}

	.reader-progress-interactive {
		cursor: pointer;
		touch-action: none;
	}

	.reader-progress-meta {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		margin-bottom: 8px;
		color: var(--color-surface-text, #e2e8f0);
		font-size: 12px;
		font-variant-numeric: tabular-nums;
		min-width: 0;
	}

	.reader-progress-meta-center .reader-progress-meta {
		justify-content: center;
		text-align: center;
	}

	.reader-progress-meta span {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.reader-progress-track {
		position: relative;
		width: 100%;
		height: 16px;
		display: flex;
		align-items: center;
	}

	.reader-progress-track::before {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		height: 4px;
		border-radius: 999px;
		background: var(--reader-progress-track);
	}

	.reader-progress-top .reader-progress-track {
		height: 5px;
		align-items: flex-end;
	}

	.reader-progress-top .reader-progress-track::before {
		height: 3px;
		border-radius: 0;
	}

	.reader-progress-fill {
		position: absolute;
		left: 0;
		width: var(--reader-progress);
		height: 4px;
		border-radius: 999px;
		background: var(--reader-progress-accent);
		transition: width 0.1s ease;
	}

	.reader-progress-top .reader-progress-fill {
		bottom: 0;
		height: 3px;
		border-radius: 0;
	}

	.reader-progress-thumb {
		position: absolute;
		left: calc(var(--reader-progress) - 7px);
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: var(--reader-progress-accent);
		border: 2px solid var(--color-surface-base, #0f172a);
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.35);
		transition: left 0.05s ease, transform 0.1s ease;
	}

	.reader-progress-interactive:hover .reader-progress-thumb {
		transform: scale(1.12);
	}

	.reader-progress-preview-marker,
	.reader-progress-preview-tooltip {
		display: none;
	}

	@media (hover: hover) and (pointer: fine) {
		.reader-progress-preview-marker {
			position: absolute;
			left: var(--reader-preview-progress);
			width: 2px;
			height: 14px;
			border-radius: 999px;
			background: var(--color-surface-text, #e2e8f0);
			box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.35);
			transform: translateX(-1px);
			pointer-events: none;
			display: block;
			z-index: 2;
		}

		.reader-progress-preview-tooltip {
			position: absolute;
			left: clamp(64px, var(--reader-preview-progress), calc(100% - 64px));
			bottom: 24px;
			max-width: min(240px, calc(100vw - 32px));
			padding: 5px 8px;
			border: 1px solid var(--color-surface-border, rgba(55, 65, 81, 0.75));
			border-radius: 6px;
			background: var(--color-surface-overlay, rgba(15, 23, 42, 0.94));
			color: var(--color-surface-text, #e2e8f0);
			font-size: 12px;
			font-weight: 600;
			line-height: 1.2;
			white-space: nowrap;
			box-shadow: 0 8px 20px rgba(0, 0, 0, 0.35);
			transform: translateX(-50%);
			pointer-events: none;
			display: block;
			z-index: 3;
		}
	}

	@media (max-width: 768px) {
		.reader-progress-bottom {
			padding: 10px 14px calc(32px + env(safe-area-inset-bottom));
		}
	}
</style>

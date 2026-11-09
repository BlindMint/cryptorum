<script lang="ts">
	import { onDestroy } from 'svelte';

	type FitMode = 'fit-page' | 'fit-width' | 'fit-height' | 'actual-size';
	let {
		currentMode, disabled = false, onfit, fitHeight = false, showMenu = true,
		onzoomout, onzoomin, zoomPercent, onrotate, rotateDisabled = false, onopenchange
	}: {
		currentMode: string | number | null;
		disabled?: boolean;
		onfit: (mode: FitMode) => void;
		fitHeight?: boolean;
		showMenu?: boolean;
		onzoomout?: () => void;
		onzoomin?: () => void;
		zoomPercent?: number | null;
		onrotate?: () => void;
		rotateDisabled?: boolean;
		onopenchange?: () => void;
	} = $props();

	let menu = $state<HTMLDetailsElement>();
	let pressed = $state<FitMode | null>(null);
	let feedbackTimer: ReturnType<typeof setTimeout>;
	const modes: { id: FitMode; label: string }[] = [
		{ id: 'fit-page', label: 'Fit page' },
		{ id: 'fit-width', label: 'Fit width' },
		{ id: 'fit-height', label: 'Fit height' },
		{ id: 'actual-size', label: 'Actual size' }
	];

	function fit(mode: FitMode) {
		onfit(mode);
		pressed = mode;
		clearTimeout(feedbackTimer);
		feedbackTimer = setTimeout(() => pressed = null, 220);
		closeMenu();
	}

	function closeMenu() {
		if (!menu) return;
		menu.open = false;
		menu.querySelector('summary')?.focus();
	}

	function dismiss(event: PointerEvent) {
		if (menu && !event.composedPath().includes(menu)) menu.open = false;
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && menu?.open) {
			event.preventDefault();
			event.stopImmediatePropagation();
			menu.open = false;
			menu.querySelector('summary')?.focus();
		}
	}

	onDestroy(() => clearTimeout(feedbackTimer));
</script>

<svelte:window onpointerdown={dismiss} onkeydowncapture={handleKeydown} />

<div class="sizing-controls">
	{#if onzoomout && onzoomin}
		<button type="button" class="desktop" onclick={onzoomout} {disabled} aria-label="Zoom out" title="Zoom out">−</button>
		<span class="zoom desktop" aria-label="Zoom level">{zoomPercent == null ? 'Fit' : `${zoomPercent}%`}</span>
		<button type="button" class="desktop" onclick={onzoomin} {disabled} aria-label="Zoom in" title="Zoom in">+</button>
	{/if}
	<button type="button" class:pressed={pressed === 'fit-width'} onclick={() => fit('fit-width')} {disabled} title="Fit width" aria-label="Fit width">
		<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M4 3v18M20 3v18M7 12h10m-7-3-3 3 3 3m4-6 3 3-3 3" /></svg>
	</button>
	<button type="button" class="desktop" class:pressed={pressed === 'fit-page'} onclick={() => fit('fit-page')} {disabled} title="Fit page" aria-label="Fit page">
		<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><rect x="6" y="3" width="12" height="18" rx="1" /><path d="M3 8V3h3m12 0h3v5M3 16v5h3m12 0h3v-5" /></svg>
	</button>
	{#if showMenu}
	<details bind:this={menu} onfocusout={(event) => { if (event.relatedTarget instanceof Node && !menu?.contains(event.relatedTarget) && menu) menu.open = false; }} ontoggle={() => { if (menu?.open) onopenchange?.(); }}>
		<summary aria-label="More reader controls" title="More reader controls">•••</summary>
		<div class="options">
			<span class="caption">Page size</span>
			{#each modes.filter((mode) => fitHeight || mode.id !== 'fit-height') as mode}
				<button type="button" class="option" onclick={() => fit(mode.id)} {disabled}>
					<span>{mode.label}</span><span aria-label={currentMode === mode.id || (mode.id === 'actual-size' && currentMode === 1) ? 'Current size' : undefined}>{currentMode === mode.id || (mode.id === 'actual-size' && currentMode === 1) ? '✓' : ''}</span>
				</button>
			{/each}
			{#if onzoomout && onzoomin}
				<button type="button" class="option mobile" onclick={onzoomout} {disabled}>Zoom out</button>
				<button type="button" class="option mobile" onclick={onzoomin} {disabled}>Zoom in</button>
			{/if}
			{#if onrotate}
				<button type="button" class="option rotate" disabled={rotateDisabled} onclick={() => { onrotate?.(); closeMenu(); }}>Rotate clockwise</button>
			{/if}
		</div>
	</details>
	{/if}
</div>

<style>
	.sizing-controls { display: flex; align-items: center; gap: 4px; flex-shrink: 0; }
	button, summary { display: flex; align-items: center; justify-content: center; width: 44px; height: 44px; border: 0; border-radius: 6px; background: transparent; color: var(--color-surface-text); cursor: pointer; font: inherit; transition: background-color 0.15s, transform 0.15s; }
	button:hover, summary:hover { background: var(--color-surface-overlay); }
	button:active, button.pressed { background: color-mix(in srgb, var(--color-primary-500) 24%, transparent); transform: scale(0.94); }
	button:focus-visible, summary:focus-visible { outline: 2px solid var(--color-primary-500); outline-offset: 2px; }
	button:disabled { opacity: 0.3; cursor: not-allowed; }
	svg { width: 20px; height: 20px; }
	.zoom { font-size: 12px; color: var(--color-surface-text-muted); min-width: 38px; text-align: center; }
	details { position: relative; }
	summary { list-style: none; }
	summary::-webkit-details-marker { display: none; }
	.options { position: absolute; right: 0; top: calc(100% + 8px); width: 200px; max-height: calc(100dvh - 100px); overflow-y: auto; padding: 6px; border: 1px solid var(--color-surface-border); border-radius: 8px; background: var(--color-surface-base); box-shadow: 0 8px 24px #0004; }
	.caption { display: block; padding: 8px 10px; color: var(--color-surface-text-muted); font-size: 12px; }
	.option { width: 100%; justify-content: space-between; padding: 0 10px; font-size: 14px; }
	.rotate { border-top: 1px solid var(--color-surface-border); border-radius: 0; }
	.mobile { display: none; }
	@media (max-width: 1300px) { .desktop { display: none; } .mobile { display: flex; } }
	@media (prefers-reduced-motion: reduce) { button, summary { transition: none; } button:active, button.pressed { transform: none; } }
</style>

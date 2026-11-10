<script lang="ts">
	import {
		PRIME_LIBRARY_ICONS,
		SVG_LIBRARY_ICONS,
		TABLER_LIBRARY_ICONS,
		parseLibraryIcon,
		sanitizeSvgMarkup,
		serializeCustomLibraryIcon,
		serializePngLibraryIcon,
		type ParsedLibraryIcon
	} from '$lib/utils/library-icons';
	import { defaultSvgThemeColor, themeSvgMarkup, readPngIcon } from '$lib/utils/custom-icons';
	import { lenientSearchMatch } from '$lib/utils/search';

	type IconTab = 'prime' | 'svg' | 'tabler';

	interface Props {
		open: boolean;
		selectedIcon?: string;
		onSelect?: (value: string) => void;
		onClose?: () => void;
	}

	let {
		open = false,
		selectedIcon = '',
		onSelect,
		onClose
	}: Props = $props();

	let activeTab = $state<IconTab>('prime');
	let primeSearch = $state('');
	let svgSearch = $state('');
	let tablerSearch = $state('');
	let tablerPage = $state(0);
	const TABLER_PAGE_SIZE = 88;
	const selected = $derived(parseLibraryIcon(selectedIcon));
	const filteredTablerIcons = $derived(TABLER_LIBRARY_ICONS.filter(icon => !tablerSearch.trim() || lenientSearchMatch(tablerSearch.trim(), `${icon.name} ${icon.label}`)));
	const tablerPages = $derived(Math.max(1, Math.ceil(filteredTablerIcons.length / TABLER_PAGE_SIZE)));
	const tablerCurrentPage = $derived(Math.min(tablerPage, tablerPages - 1));
	const visibleTablerIcons = $derived(filteredTablerIcons.slice(tablerCurrentPage * TABLER_PAGE_SIZE, (tablerCurrentPage + 1) * TABLER_PAGE_SIZE));
	let showCustomEditor = $state(false);
	let customName = $state('');
	let customSvg = $state('');
	let customFormat = $state<'svg' | 'png'>('svg');
	let customUseThemeColor = $state<boolean | null>(null);
	let customPng = $state('');
	let pngLoading = $state(false);
	let pngError = $state('');
	let pngLoadToken = 0;
	const effectiveThemeColor = $derived(customUseThemeColor ?? defaultSvgThemeColor(customSvg));
	const customPreview = $derived(customFormat === 'png'
		? customPng ? parseLibraryIcon(serializePngLibraryIcon(customName || 'PNG', customPng))?.svg ?? '' : ''
		: effectiveThemeColor ? themeSvgMarkup(sanitizeSvgMarkup(customSvg)) : sanitizeSvgMarkup(customSvg));

	function populateCustomEditor(current: ParsedLibraryIcon | null) {
		pngLoadToken++;
		pngLoading = false;
		pngError = '';
		customName = current?.source === 'custom' || current?.source === 'image' ? current.name : '';
		customSvg = current?.source === 'custom' ? current.rawSvg ?? current.svg : '';
		customUseThemeColor = current?.source === 'custom' ? current.useThemeColor ?? null : null;
		customFormat = current?.source === 'image' ? 'png' : 'svg';
		customPng = current?.imageDataUrl ?? '';
	}
	async function uploadPng(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		const token = ++pngLoadToken;
		pngLoading = true;
		pngError = '';
		try {
			const dataUrl = await readPngIcon(file);
			if (token !== pngLoadToken) return;
			customPng = dataUrl;
			if (!customName.trim()) customName = file.name.replace(/\.png$/i, '');
		} catch (error) {
			if (token === pngLoadToken) pngError = error instanceof Error ? error.message : 'Could not load this PNG.';
		} finally {
			if (token === pngLoadToken) pngLoading = false;
			input.value = '';
		}
	}

	function closePicker() {
		onClose?.();
	}

	function chooseIcon(value: string) {
		onSelect?.(value);
		onClose?.();
	}

	function isSelected(value: string): boolean {
		return selected?.value === value;
	}

	function filteredPrimeIcons() {
		if (!primeSearch.trim()) return PRIME_LIBRARY_ICONS;
		const query = primeSearch.trim();
		return PRIME_LIBRARY_ICONS.filter(icon =>
			lenientSearchMatch(query, `${icon.name} ${icon.label}`)
		);
	}

	function filteredSvgIcons() {
		if (!svgSearch.trim()) return SVG_LIBRARY_ICONS;
		const query = svgSearch.trim();
		return SVG_LIBRARY_ICONS.filter(icon =>
			lenientSearchMatch(query, `${icon.name} ${icon.label}`)
		);
	}

	function addCustomIcon() {
		const name = customName.trim();
		if (!name || pngLoading) return;
		if (customFormat === 'png') {
			if (customPng) chooseIcon(serializePngLibraryIcon(name, customPng));
		} else {
			const svg = sanitizeSvgMarkup(customSvg);
			if (!/<svg\b/i.test(svg)) return;
			chooseIcon(serializeCustomLibraryIcon(name, svg, effectiveThemeColor));
		}
	}

	function openCustomEditor() {
		showCustomEditor = true;
		populateCustomEditor(parseLibraryIcon(selectedIcon));
	}

	$effect(() => {
		if (!open) return;
		const current = parseLibraryIcon(selectedIcon);
		if (current?.source === 'tabler') {
			activeTab = 'tabler';
			const index = TABLER_LIBRARY_ICONS.findIndex(icon => icon.value === current.value);
			tablerSearch = '';
			tablerPage = Math.max(0, Math.floor(index / TABLER_PAGE_SIZE));
		} else if (current?.source === 'svg' || current?.source === 'custom' || current?.source === 'image') {
			activeTab = 'svg';
		} else {
			activeTab = 'prime';
		}
		showCustomEditor = current?.source === 'custom' || current?.source === 'image';
		populateCustomEditor(current);
	});
</script>

{#if open}
	<div
		class="fixed inset-0 z-[130] flex items-center justify-center bg-black/70 p-4"
		role="dialog"
		aria-modal="true"
		tabindex="0"
		onkeydown={(e) => { if (e.key === 'Escape') closePicker(); }}
	>
		<button
			type="button"
			class="absolute inset-0"
			aria-label="Close icon picker"
			onclick={closePicker}
		></button>

		<div class="relative z-10 flex max-h-[90vh] w-full max-w-6xl flex-col overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
			<header class="flex items-center justify-between gap-4 border-b border-[var(--color-surface-border)] px-6 py-4">
				<div class="flex items-center gap-3">
					<div class="flex h-10 w-10 items-center justify-center rounded-md bg-[var(--color-primary-500)]/20 text-[var(--color-primary-400)]">
						<svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
							<path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 0 0 2.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 0 0 1.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 0 0-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 0 0-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 0 0-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 0 0-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 0 0 1.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.573-1.066z"/>
							<path d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0z"/>
						</svg>
					</div>
					<div>
						<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Choose an icon</h2>
						<p class="text-sm text-[var(--color-surface-text-muted)]">Pick from the built-in sets or add your own SVG or PNG</p>
					</div>
				</div>

				<button
					type="button"
					class="inline-flex h-9 w-9 items-center justify-center rounded-md text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)]"
					aria-label="Close"
					onclick={closePicker}
				>
					<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<path d="M18 6 6 18"></path>
						<path d="m6 6 12 12"></path>
					</svg>
				</button>
			</header>

			<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
				<div class="border-b border-[var(--color-surface-border)] px-4 sm:px-6">
					<div class="flex gap-2 overflow-x-auto">
						<button
							type="button"
							onclick={() => activeTab = 'prime'}
							class="library-icon-tab inline-flex items-center gap-2 px-4 py-3 text-sm font-medium transition-colors {activeTab === 'prime' ? 'active text-[var(--color-primary-400)]' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
						>
							<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
								<path d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
							</svg>
							Prime Icons
						</button>
						<button
							type="button"
							onclick={() => activeTab = 'svg'}
							class="library-icon-tab inline-flex items-center gap-2 px-4 py-3 text-sm font-medium transition-colors {activeTab === 'svg' ? 'active text-[var(--color-primary-400)]' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
						>
							<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
								<path d="M4 4h16v16H4z"></path>
								<path d="M8 4v16"></path>
								<path d="M4 8h16"></path>
							</svg>
							SVG & PNG
						</button>
						<button type="button" onclick={() => activeTab = 'tabler'} aria-pressed={activeTab === 'tabler'}
							class="library-icon-tab inline-flex shrink-0 items-center gap-2 px-4 py-3 text-sm font-medium transition-colors {activeTab === 'tabler' ? 'active text-[var(--color-primary-400)]' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}">Tabler</button>
					</div>
				</div>

				<div class="min-h-0 flex-1 overflow-y-auto px-4 py-4 sm:px-6">
					{#if activeTab === 'prime'}
						<div class="space-y-4">
							<div class="relative max-w-xl">
								<svg class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-surface-text-muted)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
									<circle cx="11" cy="11" r="7"></circle>
									<path d="m20 20-3.5-3.5"></path>
								</svg>
								<input
									type="text"
									bind:value={primeSearch}
									placeholder="Search Prime Icons"
									class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] py-2 pl-10 pr-3 text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								/>
							</div>

							<div class="grid grid-cols-[repeat(auto-fill,minmax(88px,1fr))] gap-3">
								{#each filteredPrimeIcons() as icon}
									<button
										type="button"
										onclick={() => chooseIcon(icon.value)}
										class="flex min-h-[92px] flex-col items-center justify-center gap-2 rounded-lg border p-3 text-center transition-colors {isSelected(icon.value) ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10 text-[var(--color-primary-400)]' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-overlay)]'}"
										title={icon.label}
									>
										<div class="library-icon-picker-svg flex h-8 w-8 items-center justify-center">
											{@html icon.svg}
										</div>
										<div class="text-[11px] leading-tight">{icon.label}</div>
									</button>
								{/each}
							</div>
						</div>
					{:else if activeTab === 'tabler'}
						<div class="space-y-4">
							<input type="search" bind:value={tablerSearch} oninput={() => tablerPage = 0}
								aria-label="Search Tabler icons" placeholder="Search Tabler icons"
								class="w-full max-w-xl rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]" />
							<div class="flex flex-wrap items-center justify-between gap-3 text-sm text-[var(--color-surface-text-muted)]">
								<span aria-live="polite">{filteredTablerIcons.length} icons · Page {tablerCurrentPage + 1} of {tablerPages}</span>
								<div class="flex gap-2">
									<button type="button" disabled={tablerCurrentPage === 0} onclick={() => tablerPage = tablerCurrentPage - 1}
										class="rounded-md border border-[var(--color-surface-border)] px-3 py-2 hover:bg-[var(--color-surface-base)] disabled:opacity-40">Previous</button>
									<button type="button" disabled={tablerCurrentPage >= tablerPages - 1} onclick={() => tablerPage = tablerCurrentPage + 1}
										class="rounded-md border border-[var(--color-surface-border)] px-3 py-2 hover:bg-[var(--color-surface-base)] disabled:opacity-40">Next</button>
								</div>
							</div>
							<div class="grid grid-cols-[repeat(auto-fill,minmax(88px,1fr))] gap-3">
								{#each visibleTablerIcons as icon (icon.name)}
									<button type="button" onclick={() => chooseIcon(icon.value)} aria-pressed={isSelected(icon.value)}
										class="flex min-h-[92px] flex-col items-center justify-center gap-2 rounded-lg border p-3 text-center transition-colors {isSelected(icon.value) ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10 text-[var(--color-primary-400)]' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-overlay)]'}"
										title={icon.label}>
										<div class="library-icon-picker-svg flex h-8 w-8 items-center justify-center">{@html icon.svg}</div>
										<div class="text-[11px] leading-tight">{icon.label}</div>
									</button>
								{:else}
									<p class="col-span-full text-sm text-[var(--color-surface-text-muted)]">No matching icons.</p>
								{/each}
							</div>
						</div>
					{:else}
						<div class="space-y-4">
							<div class="relative max-w-xl">
								<svg class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-surface-text-muted)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
									<circle cx="11" cy="11" r="7"></circle>
									<path d="m20 20-3.5-3.5"></path>
								</svg>
								<input
									type="text"
									bind:value={svgSearch}
									placeholder="Search SVG Library"
									class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] py-2 pl-10 pr-3 text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
								/>
							</div>

							<div class="flex flex-wrap items-center gap-2">
								<button
									type="button"
									onclick={openCustomEditor}
									class="inline-flex items-center gap-2 rounded-md border border-dashed border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-primary-400)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)]"
								>
									<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
										<path d="M12 5v14"></path>
										<path d="M5 12h14"></path>
									</svg>
									Add custom
								</button>
								{#if selected?.source === 'custom' || selected?.source === 'image'}
									<span class="text-xs text-[var(--color-surface-text-muted)]">Custom icon saved with the library.</span>
								{/if}
							</div>

							{#if showCustomEditor}
								<div class="space-y-4 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
									<div class="flex gap-2" role="group" aria-label="Custom icon format">
										<button type="button" aria-pressed={customFormat === 'svg'} onclick={() => customFormat = 'svg'} class="format-choice">SVG</button>
										<button type="button" aria-pressed={customFormat === 'png'} onclick={() => customFormat = 'png'} class="format-choice">PNG</button>
									</div>
									<div class="grid gap-4 sm:grid-cols-2">
										<div>
											<label class="mb-2 block text-sm font-medium text-[var(--color-surface-text)]" for="custom-icon-name">Icon name</label>
											<input
												id="custom-icon-name"
												type="text"
												bind:value={customName}
												placeholder="e.g. my-library-icon"
												class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
											/>
										</div>
										<div>
											{#if customFormat === 'svg'}
											<label class="mb-2 block text-sm font-medium text-[var(--color-surface-text)]" for="custom-icon-svg">SVG markup</label>
											<textarea
												id="custom-icon-svg"
												bind:value={customSvg}
												rows="6"
												placeholder="<svg ...>...</svg>"
												class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 font-mono text-xs text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
											></textarea>
											<label class="mt-3 flex items-center gap-2 text-sm text-[var(--color-surface-text)]">
												<input type="checkbox" checked={effectiveThemeColor} onchange={(event) => customUseThemeColor = event.currentTarget.checked} />
												Use theme color
											</label>
											<p class="mt-1 text-xs text-[var(--color-surface-text-muted)]">Matches the icon color to your theme. Transparent areas stay transparent.</p>
										{:else}
											<label class="mb-2 block text-sm font-medium text-[var(--color-surface-text)]" for="custom-icon-png">PNG image</label>
											<input id="custom-icon-png" type="file" accept="image/png,.png" onchange={uploadPng}
												class="w-full text-sm text-[var(--color-surface-text)]" />
											<p class="mt-2 text-xs text-[var(--color-surface-text-muted)]">PNG colors and transparency are preserved. Transparent backgrounds work best.</p>
											{#if pngLoading}<p role="status" class="mt-2 text-xs text-[var(--color-surface-text-muted)]">Preparing image…</p>{/if}
											{#if pngError}<p role="alert" class="mt-2 text-xs text-red-400">{pngError}</p>{/if}
										{/if}
										</div>
									</div>

									<div class="flex items-center gap-3">
										<button
											type="button"
											onclick={addCustomIcon}
											disabled={!customName.trim() || !customPreview || pngLoading}
											class="accent-action inline-flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors"
										>
											<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
												<path d="M12 5v14"></path>
												<path d="M5 12h14"></path>
											</svg>
											Add and select
										</button>
										<button
											type="button"
											onclick={() => showCustomEditor = false}
											class="inline-flex items-center gap-2 rounded-md border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)]"
										>
											Close
										</button>
									</div>

									{#if customPreview}
										<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-3">
											<div class="mb-2 text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">Preview</div>
											<div class="flex items-center gap-3">
												<div class="library-icon-picker-svg flex h-10 w-10 items-center justify-center text-[var(--color-primary-400)]">
													{@html customPreview}
												</div>
												<div>
													<div class="text-sm font-medium text-[var(--color-surface-text)]">{customName}</div>
													<div class="text-xs text-[var(--color-surface-text-muted)]">{customFormat === 'png' ? 'PNG image' : 'Custom SVG'}</div>
												</div>
											</div>
										</div>
									{/if}
								</div>
							{/if}

							<div class="grid grid-cols-[repeat(auto-fill,minmax(88px,1fr))] gap-3">
								{#each filteredSvgIcons() as icon}
									<button
										type="button"
										onclick={() => chooseIcon(icon.value)}
										class="flex min-h-[92px] flex-col items-center justify-center gap-2 rounded-lg border p-3 text-center transition-colors {isSelected(icon.value) ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-500)]/10 text-[var(--color-primary-400)]' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-overlay)]'}"
										title={icon.label}
									>
										<div class="library-icon-picker-svg flex h-8 w-8 items-center justify-center">
											{@html icon.svg}
										</div>
										<div class="text-[11px] leading-tight">{icon.label}</div>
									</button>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	.format-choice { border: 1px solid var(--color-surface-border); border-radius: 6px; padding: 6px 14px; font-size: 14px; color: var(--color-surface-text); }
	.format-choice[aria-pressed="true"] { border-color: var(--color-primary-500); background: color-mix(in srgb, var(--color-primary-500) 12%, transparent); }
	.format-choice:focus-visible { outline: 2px solid var(--color-primary-500); outline-offset: 2px; }
	:global(.library-icon-picker-svg svg) {
		display: block;
		width: 100%;
		height: 100%;
		overflow: visible;
	}

	.library-icon-tab {
		position: relative;
	}

	.library-icon-tab::after {
		content: '';
		position: absolute;
		left: 1rem;
		right: 1rem;
		bottom: 0;
		height: 2px;
		border-radius: 999px;
		background: var(--color-primary-500);
		opacity: 0;
		transform: scaleX(0.35);
		transition: opacity 140ms ease-out, transform 160ms ease-out;
	}

	.library-icon-tab.active::after {
		opacity: 1;
		transform: scaleX(1);
	}
</style>

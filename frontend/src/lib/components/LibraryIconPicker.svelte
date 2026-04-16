<script lang="ts">
	import {
		PRIME_LIBRARY_ICONS,
		SVG_LIBRARY_ICONS,
		parseLibraryIcon,
		sanitizeSvgMarkup,
		serializeCustomLibraryIcon
	} from '$lib/utils/library-icons';

	type IconTab = 'prime' | 'svg';

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
	let showCustomEditor = $state(false);
	let customName = $state('');
	let customSvg = $state('');

	function closePicker() {
		onClose?.();
	}

	function chooseIcon(value: string) {
		onSelect?.(value);
		onClose?.();
	}

	function isSelected(value: string): boolean {
		const current = parseLibraryIcon(selectedIcon);
		return current?.value === value;
	}

	function filteredPrimeIcons() {
		if (!primeSearch.trim()) return PRIME_LIBRARY_ICONS;
		const query = primeSearch.trim().toLowerCase();
		return PRIME_LIBRARY_ICONS.filter(icon =>
			icon.name.toLowerCase().includes(query) || icon.label.toLowerCase().includes(query)
		);
	}

	function filteredSvgIcons() {
		if (!svgSearch.trim()) return SVG_LIBRARY_ICONS;
		const query = svgSearch.trim().toLowerCase();
		return SVG_LIBRARY_ICONS.filter(icon =>
			icon.name.toLowerCase().includes(query) || icon.label.toLowerCase().includes(query)
		);
	}

	function addCustomIcon() {
		const name = customName.trim();
		const svg = sanitizeSvgMarkup(customSvg);
		if (!name || !svg || !svg.includes('<svg')) return;
		chooseIcon(serializeCustomLibraryIcon(name, svg));
	}

	function openCustomEditor() {
		showCustomEditor = true;
		const current = parseLibraryIcon(selectedIcon);
		if (current?.source === 'custom') {
			customName = current.name;
			customSvg = current.svg;
		} else {
			customName = '';
			customSvg = '';
		}
	}

	$effect(() => {
		if (!open) return;
		const current = parseLibraryIcon(selectedIcon);
		if (current?.source === 'svg' || current?.source === 'custom') {
			activeTab = 'svg';
		} else {
			activeTab = 'prime';
		}
		if (current?.source === 'custom') {
			showCustomEditor = true;
			customName = current.name;
			customSvg = current.svg;
		} else {
			showCustomEditor = false;
			customName = '';
			customSvg = '';
		}
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
						<p class="text-sm text-[var(--color-surface-text-muted)]">Pick from the built-in sets or add a custom SVG</p>
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
					<div class="flex gap-2">
						<button
							type="button"
							onclick={() => activeTab = 'prime'}
							class="inline-flex items-center gap-2 border-b-2 px-4 py-3 text-sm font-medium transition-colors {activeTab === 'prime' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-400)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
						>
							<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
								<path d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
							</svg>
							Prime Icons
						</button>
						<button
							type="button"
							onclick={() => activeTab = 'svg'}
							class="inline-flex items-center gap-2 border-b-2 px-4 py-3 text-sm font-medium transition-colors {activeTab === 'svg' ? 'border-[var(--color-primary-500)] text-[var(--color-primary-400)]' : 'border-transparent text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
						>
							<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
								<path d="M4 4h16v16H4z"></path>
								<path d="M8 4v16"></path>
								<path d="M4 8h16"></path>
							</svg>
							SVG Library
						</button>
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
										<div class="flex h-8 w-8 items-center justify-center">
											{@html icon.svg}
										</div>
										<div class="text-[11px] leading-tight">{icon.label}</div>
									</button>
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
								{#if parseLibraryIcon(selectedIcon)?.source === 'custom'}
									<span class="text-xs text-[var(--color-surface-text-muted)]">Custom SVG saved with the library.</span>
								{/if}
							</div>

							{#if showCustomEditor}
								<div class="space-y-4 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
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
											<label class="mb-2 block text-sm font-medium text-[var(--color-surface-text)]" for="custom-icon-svg">SVG markup</label>
											<textarea
												id="custom-icon-svg"
												bind:value={customSvg}
												rows="6"
												placeholder="<svg ...>...</svg>"
												class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 font-mono text-xs text-[var(--color-surface-text)] placeholder:text-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
											></textarea>
										</div>
									</div>

									<div class="flex items-center gap-3">
										<button
											type="button"
											onclick={addCustomIcon}
											class="inline-flex items-center gap-2 rounded-md bg-[var(--color-primary-500)] px-3 py-2 text-sm font-medium text-white hover:bg-[var(--color-primary-600)]"
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

									{#if customName.trim() && customSvg.trim()}
										<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-3">
											<div class="mb-2 text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">Preview</div>
											<div class="flex items-center gap-3">
												<div class="flex h-10 w-10 items-center justify-center text-[var(--color-primary-400)]">
													{@html sanitizeSvgMarkup(customSvg)}
												</div>
												<div>
													<div class="text-sm font-medium text-[var(--color-surface-text)]">{customName}</div>
													<div class="text-xs text-[var(--color-surface-text-muted)]">Custom SVG</div>
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
										<div class="flex h-8 w-8 items-center justify-center">
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

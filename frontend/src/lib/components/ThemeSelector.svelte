<script lang="ts">
	import { currentTheme, primaryColors, surfaceColors, addCustomTheme, removeCustomTheme, resetPrimaryToDefault, resetSurfaceToDefault, selectCustomTheme, generateId, type CustomTheme } from '$lib/stores/theme';
	import ThemePreviewSwatch from './ThemePreviewSwatch.svelte';

	let {
		mobileMenu = false
	} = $props<{
		mobileMenu?: boolean;
	}>();

	let showDropdown = $state(false);
	let showCustomThemeEditor = $state(false);
	let editingTheme = $state<CustomTheme | null>(null);
	let newThemeName = $state('');
	let newThemeFg = $state('#ffffff');
	let newThemeFgHexInput = $state('#ffffff');
	let newThemeFgRgb = $state('rgb(255, 255, 255)');
	let newThemeBg = $state('#111111');
	let newThemeBgHexInput = $state('#111111');
	let newThemeBgRgb = $state('rgb(17, 17, 17)');

	function normalizeHexColor(value: string): string | null {
		const trimmed = value.trim();
		const match = /^#?([a-f\d]{3}|[a-f\d]{6})$/i.exec(trimmed);
		if (!match) return null;
		let hex = match[1];
		if (hex.length === 3) {
			hex = hex.split('').map(char => char + char).join('');
		}
		return `#${hex.toLowerCase()}`;
	}

	function clampColorChannel(value: number): number {
		return Math.max(0, Math.min(255, Math.round(value)));
	}

	function hexToRgbComponents(hex: string): { r: number; g: number; b: number } | null {
		const normalized = normalizeHexColor(hex);
		if (!normalized) return null;
		const parts = normalized.slice(1).match(/.{2}/g);
		if (!parts || parts.length !== 3) return null;
		return {
			r: parseInt(parts[0], 16),
			g: parseInt(parts[1], 16),
			b: parseInt(parts[2], 16)
		};
	}

	function rgbComponentsToHex(r: number, g: number, b: number): string {
		return `#${[r, g, b].map(value => clampColorChannel(value).toString(16).padStart(2, '0')).join('')}`;
	}

	function rgbStringToHex(value: string): string | null {
		const trimmed = value.trim();
		const match = trimmed.match(/^rgb\s*\(\s*(\d{1,3})\s*[, ]\s*(\d{1,3})\s*[, ]\s*(\d{1,3})\s*\)$/i)
			?? trimmed.match(/^(\d{1,3})\s*[, ]\s*(\d{1,3})\s*[, ]\s*(\d{1,3})$/);
		if (!match) return null;
		const r = Number(match[1]);
		const g = Number(match[2]);
		const b = Number(match[3]);
		if ([r, g, b].some(value => Number.isNaN(value) || value < 0 || value > 255)) return null;
		return rgbComponentsToHex(r, g, b);
	}

	function rgbComponentsToString(hex: string): string {
		const rgb = hexToRgbComponents(hex);
		if (!rgb) return 'rgb(0, 0, 0)';
		return `rgb(${rgb.r}, ${rgb.g}, ${rgb.b})`;
	}

	function parseColorInput(value: string): string | null {
		return normalizeHexColor(value) ?? rgbStringToHex(value);
	}

	function resolvePreviewColor(value: string, fallback: string): string {
		return parseColorInput(value) ?? fallback;
	}

	function isValidThemeColor(value: string): boolean {
		return parseColorInput(value) !== null;
	}

	function setForegroundColorFromHex(value: string) {
		newThemeFgHexInput = value;
		const normalized = normalizeHexColor(value);
		if (normalized) {
			newThemeFg = normalized;
			newThemeFgHexInput = normalized;
			newThemeFgRgb = rgbComponentsToString(normalized);
		}
	}

	function setForegroundColorFromRgb(value: string) {
		newThemeFgRgb = value;
		const normalized = rgbStringToHex(value);
		if (normalized) {
			newThemeFg = normalized;
			newThemeFgHexInput = normalized;
			newThemeFgRgb = rgbComponentsToString(normalized);
		}
	}

	function setBackgroundColorFromHex(value: string) {
		newThemeBgHexInput = value;
		const normalized = normalizeHexColor(value);
		if (normalized) {
			newThemeBg = normalized;
			newThemeBgHexInput = normalized;
			newThemeBgRgb = rgbComponentsToString(normalized);
		}
	}

	function setBackgroundColorFromRgb(value: string) {
		newThemeBgRgb = value;
		const normalized = rgbStringToHex(value);
		if (normalized) {
			newThemeBg = normalized;
			newThemeBgHexInput = normalized;
			newThemeBgRgb = rgbComponentsToString(normalized);
		}
	}

	function selectPrimary(primary: string) {
		currentTheme.update(t => ({ ...t, primary }));
	}

	function selectSurface(surface: string) {
		currentTheme.update(t => ({ ...t, surface }));
	}

	function getPrimaryColorClass(color: string) {
		const colorMap: Record<string, string> = {
			red: 'bg-red-500',
			orange: 'bg-orange-500',
			yellow: 'bg-yellow-500',
			green: 'bg-green-500',
			teal: 'bg-teal-500',
			blue: 'bg-blue-500',
			indigo: 'bg-indigo-500',
			purple: 'bg-purple-500',
			pink: 'bg-pink-500',
			rose: 'bg-rose-500',
			'red-400': 'bg-red-400',
			'orange-400': 'bg-orange-400',
			'yellow-400': 'bg-yellow-400',
			'green-400': 'bg-green-400',
			'teal-400': 'bg-teal-400',
			'blue-400': 'bg-blue-400',
			'indigo-400': 'bg-indigo-400',
			'purple-400': 'bg-purple-400',
			'pink-400': 'bg-pink-400',
			'rose-400': 'bg-rose-400',
			'red-600': 'bg-red-600',
			'orange-600': 'bg-orange-600',
			'yellow-600': 'bg-yellow-600',
			'green-600': 'bg-green-600',
			'teal-600': 'bg-teal-600',
			'blue-600': 'bg-blue-600',
			'indigo-600': 'bg-indigo-600',
			'purple-600': 'bg-purple-600',
			'pink-600': 'bg-pink-600',
			'rose-600': 'bg-rose-600',
			'red-800': 'bg-red-800',
			'orange-800': 'bg-orange-800',
			'yellow-800': 'bg-yellow-800',
			'green-800': 'bg-green-800',
			'teal-800': 'bg-teal-800',
			'blue-800': 'bg-blue-800',
			'indigo-800': 'bg-indigo-800',
			'purple-800': 'bg-purple-800',
			'pink-800': 'bg-pink-800',
			'rose-800': 'bg-rose-800'
		};
		return colorMap[color] || 'bg-gray-500';
	}

	function getSurfaceColorClass(color: string) {
		const colorMap: Record<string, string> = {
			dark: 'bg-slate-800',
			light: 'bg-slate-200',
			darker: 'bg-slate-900',
			lighter: 'bg-white',
			slate: 'bg-slate-800',
			gray: 'bg-gray-800',
			zinc: 'bg-zinc-900',
			neutral: 'bg-neutral-900',
			stone: 'bg-stone-900',
			'red-surface': 'bg-red-900',
			'orange-surface': 'bg-orange-900',
			'yellow-surface': 'bg-yellow-900',
			'green-surface': 'bg-green-900',
			'teal-surface': 'bg-teal-900',
			'blue-surface': 'bg-blue-900',
			'indigo-surface': 'bg-indigo-900',
			'purple-surface': 'bg-purple-900',
			'pink-surface': 'bg-pink-900'
		};
		return colorMap[color] || 'bg-gray-800';
	}

	function clickOutside(node: HTMLElement, callback: () => void) {
		const handleClick = (event: MouseEvent) => {
			if (node && !node.contains(event.target as Node) && !event.defaultPrevented) {
				callback();
			}
		};

		document.addEventListener('click', handleClick, { capture: true });

		return {
			destroy() {
				document.removeEventListener('click', handleClick, { capture: true });
			}
		};
	}

	function handleButtonClick(event: MouseEvent) {
		event.stopPropagation();
	}

	function openAddCustomTheme() {
		editingTheme = null;
		newThemeName = '';
		newThemeFg = '#ffffff';
		newThemeFgHexInput = '#ffffff';
		newThemeFgRgb = 'rgb(255, 255, 255)';
		newThemeBg = '#111111';
		newThemeBgHexInput = '#111111';
		newThemeBgRgb = 'rgb(17, 17, 17)';
		showCustomThemeEditor = true;
	}

	function openEditCustomTheme(event: MouseEvent, theme: CustomTheme) {
		event.stopPropagation();
		editingTheme = theme;
		newThemeName = theme.name;
		newThemeFg = theme.foreground;
		newThemeFgHexInput = theme.foreground;
		newThemeFgRgb = rgbComponentsToString(theme.foreground);
		newThemeBg = theme.background;
		newThemeBgHexInput = theme.background;
		newThemeBgRgb = rgbComponentsToString(theme.background);
		showCustomThemeEditor = true;
	}

	function saveCustomTheme() {
		if (!newThemeName.trim()) return;
		const foreground = parseColorInput(newThemeFg);
		const background = parseColorInput(newThemeBg);
		if (!foreground || !background) return;
		
		if (editingTheme) {
			currentTheme.update(t => ({
				...t,
				appearance: {
					...t.appearance,
					customThemes: t.appearance.customThemes.map((ct: CustomTheme) => 
						ct.id === editingTheme!.id 
							? { ...ct, name: newThemeName.trim(), foreground, background }
							: ct
					)
				}
			}));
		} else {
			const newTheme: CustomTheme = {
				id: generateId(),
				name: newThemeName.trim(),
				foreground,
				background
			};
			addCustomTheme(newTheme);
		}
		
		showCustomThemeEditor = false;
	}

	function deleteCustomTheme(event: MouseEvent, id: string) {
		event.stopPropagation();
		removeCustomTheme(id);
	}

	function selectTheme(themeId: string | null) {
		selectCustomTheme(themeId);
	}

	let canSaveCustomTheme = $derived(
		newThemeName.trim().length > 0 &&
		isValidThemeColor(newThemeFgHexInput) &&
		isValidThemeColor(newThemeFgRgb) &&
		isValidThemeColor(newThemeBgHexInput) &&
		isValidThemeColor(newThemeBgRgb)
	);
</script>

<div class="relative">
	<button
		onclick={() => showDropdown = !showDropdown}
		class={`rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] transition-colors ${mobileMenu ? 'flex w-full items-center justify-between gap-3 px-0 py-0' : 'p-2'}`}
		aria-label="Change theme"
	>
		{#if mobileMenu}
			<span class="flex items-center gap-3">
				<svg class="h-5 w-5" viewBox="0 -960 960 960" fill="currentColor">
					<path d="M480-80q-82 0-155-31.5t-127.5-86Q143-252 111.5-325T80-480q0-83 32.5-156t88-127Q256-817 330-848.5T488-880q80 0 151 27.5t124.5 76q53.5 48.5 85 115T880-518q0 115-70 176.5T640-280h-74q-9 0-12.5 5t-3.5 11q0 12 15 34.5t15 51.5q0 50-27.5 74T480-80Zm0-400Zm-177 23q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17Zm120-160q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17Zm200 0q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17Zm120 160q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17ZM480-160q9 0 14.5-5t5.5-13q0-14-15-33t-15-57q0-42 29-67t71-25h70q66 0 113-38.5T800-518q0-121-92.5-201.5T488-800q-136 0-232 93t-96 227q0 133 93.5 226.5T480-160Z"/>
				</svg>
				<span class="text-sm font-medium text-[var(--color-surface-text)]">Theme</span>
			</span>
		{:else}
			<svg class="w-5 h-5" viewBox="0 -960 960 960" fill="currentColor">
				<path d="M480-80q-82 0-155-31.5t-127.5-86Q143-252 111.5-325T80-480q0-83 32.5-156t88-127Q256-817 330-848.5T488-880q80 0 151 27.5t124.5 76q53.5 48.5 85 115T880-518q0 115-70 176.5T640-280h-74q-9 0-12.5 5t-3.5 11q0 12 15 34.5t15 51.5q0 50-27.5 74T480-80Zm0-400Zm-177 23q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17Zm120-160q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17Zm200 0q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17Zm120 160q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17ZM480-160q9 0 14.5-5t5.5-13q0-14-15-33t-15-57q0-42 29-67t71-25h70q66 0 113-38.5T800-518q0-121-92.5-201.5T488-800q-136 0-232 93t-96 227q0 133 93.5 226.5T480-160Z"/>
			</svg>
		{/if}
	</button>

	{#if showDropdown}
		<div
			class="absolute right-0 top-full mt-2 w-96 bg-[var(--color-surface-overlay)] backdrop-blur-sm border border-[var(--color-surface-border)] rounded-lg shadow-lg z-50 p-4"
			use:clickOutside={() => { showDropdown = false; showCustomThemeEditor = false; }}
		>
			<div class="space-y-4">
				{#if showCustomThemeEditor}
					<div class="space-y-3">
						<div class="flex items-center justify-between">
								<h4 class="text-sm font-medium text-[var(--color-surface-text)]">
									{editingTheme ? 'Edit Custom Theme' : 'Add Custom Theme'}
								</h4>
								<button
									type="button"
									onclick={() => showCustomThemeEditor = false}
									aria-label="Close custom theme editor"
									class="text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]"
								>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
								</svg>
							</button>
							</div>
							<div>
								<div class="block text-xs text-[var(--color-surface-text-muted)] mb-1">Theme Name</div>
								<input
									type="text"
									bind:value={newThemeName}
									placeholder="My Theme"
									class="w-full px-3 py-2 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] text-sm"
								/>
							</div>
							<div class="grid grid-cols-2 gap-3">
								<div>
									<div class="block text-xs text-[var(--color-surface-text-muted)] mb-2">Text Color</div>
									<div class="flex items-start gap-3">
							<div class="relative flex h-12 w-12 flex-shrink-0 items-center justify-center overflow-hidden rounded-full border border-[var(--color-surface-border)] shadow-inner" style="background-color: {resolvePreviewColor(newThemeFg, '#ffffff')};">
										<span class="absolute inset-0 ring-1 ring-inset ring-black/10"></span>
										<input
											type="color"
											value={resolvePreviewColor(newThemeFg, '#ffffff')}
											oninput={(e) => setForegroundColorFromHex(e.currentTarget.value)}
											class="absolute inset-0 h-full w-full cursor-pointer opacity-0"
											aria-label="Pick text color"
										/>
						</div>
									<div class="flex-1 space-y-2">
										<input
											type="text"
											value={newThemeFgHexInput}
											oninput={(e) => setForegroundColorFromHex(e.currentTarget.value)}
											placeholder="#ffffff"
											class="w-full px-3 py-2 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] text-sm font-mono"
										/>
										<input
											type="text"
											value={newThemeFgRgb}
											oninput={(e) => setForegroundColorFromRgb(e.currentTarget.value)}
											placeholder="rgb(255, 255, 255)"
											class="w-full px-3 py-2 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] text-sm font-mono"
										/>
									</div>
									</div>
								</div>
								<div>
									<div class="block text-xs text-[var(--color-surface-text-muted)] mb-2">Background Color</div>
									<div class="flex items-start gap-3">
							<div class="relative flex h-12 w-12 flex-shrink-0 items-center justify-center overflow-hidden rounded-full border border-[var(--color-surface-border)] shadow-inner" style="background-color: {resolvePreviewColor(newThemeBg, '#111111')};">
										<span class="absolute inset-0 ring-1 ring-inset ring-black/10"></span>
										<input
											type="color"
											value={resolvePreviewColor(newThemeBg, '#111111')}
											oninput={(e) => setBackgroundColorFromHex(e.currentTarget.value)}
											class="absolute inset-0 h-full w-full cursor-pointer opacity-0"
											aria-label="Pick background color"
										/>
						</div>
									<div class="flex-1 space-y-2">
										<input
											type="text"
											value={newThemeBgHexInput}
											oninput={(e) => setBackgroundColorFromHex(e.currentTarget.value)}
											placeholder="#111111"
											class="w-full px-3 py-2 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] text-sm font-mono"
										/>
										<input
											type="text"
											value={newThemeBgRgb}
											oninput={(e) => setBackgroundColorFromRgb(e.currentTarget.value)}
											placeholder="rgb(17, 17, 17)"
											class="w-full px-3 py-2 rounded bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] text-sm font-mono"
										/>
									</div>
								</div>
							</div>
						</div>
						<p class="text-xs text-[var(--color-surface-text-muted)]">
							Use HEX like <span class="font-mono">#ffffff</span> or RGB like <span class="font-mono">rgb(255, 255, 255)</span>.
						</p>
							<div class="flex justify-end gap-2">
								<button
									type="button"
									onclick={() => showCustomThemeEditor = false}
									class="px-3 py-1.5 text-sm rounded bg-[var(--color-surface-base)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-700)] transition-colors"
								>
								Cancel
								</button>
								<button
									type="button"
									onclick={saveCustomTheme}
									disabled={!canSaveCustomTheme}
									class="px-3 py-1.5 text-sm rounded bg-[var(--color-primary-500)] text-white hover:bg-[var(--color-primary-600)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
								>
								{editingTheme ? 'Update' : 'Add'} Theme
							</button>
						</div>
					</div>
				{:else}
					{#if $currentTheme.appearance.customThemes && $currentTheme.appearance.customThemes.length > 0}
						<div>
								<div class="flex items-center justify-between mb-2">
									<div class="text-xs font-medium text-[var(--color-surface-text-muted)] uppercase tracking-wider">Custom Themes</div>
									<button
										type="button"
										onclick={openAddCustomTheme}
										class="text-xs text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)] font-medium"
									>
									+ Add
								</button>
							</div>
							<div class="grid grid-cols-2 gap-2">
									{#each $currentTheme.appearance.customThemes as theme}
										<div class="relative group">
											<button
												type="button"
												onclick={() => selectTheme(theme.id)}
												class="w-full flex items-center gap-2 p-2 rounded-lg border-2 transition-all {$currentTheme.appearance.selectedCustomThemeId === theme.id ? 'border-[var(--color-primary-500)]' : 'border-[var(--color-surface-border)] hover:border-[var(--color-surface-500)]'}"
											>
											<ThemePreviewSwatch background={theme.background} foreground={theme.foreground} />
											<div class="flex-1 min-w-0 text-left">
												<div class="text-sm text-[var(--color-surface-text)] truncate">{theme.name}</div>
											</div>
										</button>
											<div class="absolute top-1 right-1 flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
												<button
													type="button"
													onclick={(e) => openEditCustomTheme(e, theme)}
													aria-label={`Edit ${theme.name} theme`}
													class="p-1 rounded bg-[var(--color-surface-700)] hover:bg-[var(--color-surface-600)] text-[var(--color-surface-text-muted)]"
											>
												<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
													<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z"></path>
												</svg>
												</button>
												<button
													type="button"
													onclick={(e) => deleteCustomTheme(e, theme.id)}
													aria-label={`Delete ${theme.name} theme`}
													class="p-1 rounded bg-red-500/20 hover:bg-red-500/40 text-red-400"
											>
												<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
													<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
												</svg>
											</button>
										</div>
									</div>
								{/each}
							</div>
						</div>
					{/if}

		<div>
			<div class="mb-3 flex items-center justify-between">
				<div class="text-xs font-medium text-[var(--color-surface-text-muted)] uppercase tracking-wider">Primary</div>
				<button
					type="button"
					onclick={resetPrimaryToDefault}
					class="inline-flex h-5 w-5 items-center justify-center rounded text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
					title="Reset to default colors"
					aria-label="Reset primary color to default"
				>
					<svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" aria-hidden="true">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12a8 8 0 1 1-2.343-5.657"></path>
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 4v4h-4"></path>
					</svg>
				</button>
			</div>
				<div class="grid grid-cols-10 gap-2">
					{#each primaryColors as color}
						<button
										type="button"
										onclick={(e) => { handleButtonClick(e); selectPrimary(color); }}
										class="w-6 h-6 rounded-full border border-[var(--color-surface-border)] {getPrimaryColorClass(color)} {$currentTheme.primary === color ? 'ring-2 ring-[var(--color-surface-text)]' : ''} transition-all hover:scale-110"
										aria-label={`Select ${color} primary color`}
								></button>
							{/each}
						</div>
					</div>

		<div>
			<div class="mb-3 flex items-center justify-between">
				<div class="text-xs font-medium text-[var(--color-surface-text-muted)] uppercase tracking-wider">Surface</div>
				<button
					type="button"
					onclick={resetSurfaceToDefault}
					class="inline-flex h-5 w-5 items-center justify-center rounded text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
					title="Reset to default colors"
					aria-label="Reset surface color to default"
				>
					<svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" aria-hidden="true">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12a8 8 0 1 1-2.343-5.657"></path>
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 4v4h-4"></path>
					</svg>
				</button>
			</div>
				<div class="grid grid-cols-6 gap-2">
					{#each surfaceColors as color}
									<button
										type="button"
										onclick={(e) => { handleButtonClick(e); selectSurface(color); }}
										class="w-8 h-8 rounded-lg border border-[var(--color-surface-border)] {getSurfaceColorClass(color)} {$currentTheme.surface === color ? 'ring-2 ring-[var(--color-surface-text)]' : ''} transition-all hover:scale-105"
										aria-label={`Select ${color} surface`}
								></button>
							{/each}
						</div>
					</div>

						{#if !$currentTheme.appearance.customThemes || $currentTheme.appearance.customThemes.length === 0}
							<button
								type="button"
								onclick={openAddCustomTheme}
								class="w-full py-2 text-sm text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)] border border-dashed border-[var(--color-surface-border)] hover:border-[var(--color-primary-500)] rounded-lg transition-colors"
							>
							+ Add Custom Theme
						</button>
					{/if}
				{/if}
			</div>
		</div>
	{/if}
</div>

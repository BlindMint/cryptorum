import { writable } from 'svelte/store';

export interface CustomTheme {
  id: string;
  name: string;
  foreground: string;
  background: string;
}

export interface Theme {
  primary: string;
  surface: string;
}

export interface ThemeColors {
  foreground: string;
  background: string;
}

export type BackgroundImageDisplay = 'fill' | 'fit' | 'center' | 'stretch' | 'tile';

export interface AppearanceSettings {
  glowEnabled: boolean;
  glowAutoMode: boolean;
  glowColor: string;
  glowIntensity: number;
  bgImageEnabled: boolean;
  bgImageTransparency: number;
  bgImageDisplay: BackgroundImageDisplay;
  backgroundImages: string[];
  selectedBgImageIndex: number;
  customThemes: CustomTheme[];
  selectedCustomThemeId: string | null;
}

export interface FullTheme extends Theme {
  appearance: AppearanceSettings;
}

export const DEFAULT_THEME_PRIMARY = 'orange';
export const DEFAULT_THEME_SURFACE = 'dark';

// Primary colors (40 options like Booklore)
export const primaryColors = [
	'red', 'orange', 'yellow', 'green', 'teal', 'blue', 'indigo', 'purple', 'pink', 'rose',
	'red-400', 'orange-400', 'yellow-400', 'green-400', 'teal-400', 'blue-400', 'indigo-400', 'purple-400', 'pink-400', 'rose-400',
	'red-600', 'orange-600', 'yellow-600', 'green-600', 'teal-600', 'blue-600', 'indigo-600', 'purple-600', 'pink-600', 'rose-600',
	'red-800', 'orange-800', 'yellow-800', 'green-800', 'teal-800', 'blue-800', 'indigo-800', 'purple-800', 'pink-800', 'rose-800'
];

// Surface colors (18 options like Booklore) - organized by darkness, colored tints, then light
export const surfaceColors = [
	'darker', 'dark', 'zinc', 'neutral', 'stone', 'slate', 'gray',
	'red-surface', 'orange-surface', 'yellow-surface', 'green-surface', 'teal-surface', 'blue-surface', 'indigo-surface', 'purple-surface', 'pink-surface',
	'lighter', 'light'
];

export const themes: Theme[] = [
	{ primary: DEFAULT_THEME_PRIMARY, surface: DEFAULT_THEME_SURFACE }
];

function createDefaultAppearance(): AppearanceSettings {
	return {
		glowEnabled: true,
		glowAutoMode: true,
		glowColor: '#f97316',
		glowIntensity: 10,
		bgImageEnabled: false,
		bgImageTransparency: 50,
		bgImageDisplay: 'fill',
		backgroundImages: [],
		selectedBgImageIndex: 0,
		customThemes: [],
		selectedCustomThemeId: null
	};
}

export function createDefaultTheme(): FullTheme {
	return {
		primary: DEFAULT_THEME_PRIMARY,
		surface: DEFAULT_THEME_SURFACE,
		appearance: createDefaultAppearance()
	};
}

const defaultTheme: FullTheme = createDefaultTheme();

// Load from localStorage if available
let initialTheme = defaultTheme;
if (typeof localStorage !== 'undefined') {
	const saved = localStorage.getItem('cryptorum-theme');
	if (saved) {
		try {
			const parsed = JSON.parse(saved);
			initialTheme = { ...defaultTheme, ...parsed };
			if (!initialTheme.appearance) {
				initialTheme.appearance = defaultTheme.appearance;
			}
			// Ensure new appearance properties exist
			if (!initialTheme.appearance.customThemes) {
				initialTheme.appearance.customThemes = defaultTheme.appearance.customThemes;
			}
			if (initialTheme.appearance.selectedCustomThemeId === undefined) {
				initialTheme.appearance.selectedCustomThemeId = defaultTheme.appearance.selectedCustomThemeId;
			}
			if (!initialTheme.appearance.bgImageDisplay) {
				initialTheme.appearance.bgImageDisplay = defaultTheme.appearance.bgImageDisplay;
			}
		} catch (e) {
			console.warn('Invalid theme in localStorage, using default');
		}
	}
}

export const currentTheme = writable<FullTheme>(initialTheme);

// Save to localStorage when theme changes
currentTheme.subscribe((theme) => {
	if (typeof localStorage !== 'undefined') {
		localStorage.setItem('cryptorum-theme', JSON.stringify(theme));
	}
	// Update CSS variables
	updateThemeColors(theme);
});

export function resetPrimaryToDefault() {
	currentTheme.update((theme) => ({
		...theme,
		primary: DEFAULT_THEME_PRIMARY
	}));
}

export function resetSurfaceToDefault() {
	currentTheme.update((theme) => ({
		...theme,
		surface: DEFAULT_THEME_SURFACE
	}));
}

function updateThemeColors(theme: FullTheme) {
	if (typeof document !== 'undefined') {
		const root = document.documentElement;

		// Define color mappings
		const primaryColors = {
			green: { 500: '#22c55e', 600: '#16a34a', 400: '#4ade80' },
			blue: { 500: '#3b82f6', 600: '#2563eb', 400: '#60a5fa' },
			purple: { 500: '#a855f7', 600: '#9333ea', 400: '#c084fc' },
			red: { 500: '#ef4444', 600: '#dc2626', 400: '#f87171' },
			orange: { 500: '#f97316', 600: '#ea580c', 400: '#fb923c' },
			pink: { 500: '#ec4899', 600: '#db2777', 400: '#f9a8d4' }
		};

		const primaryColorMap = {
			red: { 500: '#ef4444', 600: '#dc2626', 400: '#f87171' },
			orange: { 500: '#f97316', 600: '#ea580c', 400: '#fb923c' },
			yellow: { 500: '#eab308', 600: '#ca8a04', 400: '#facc15' },
			green: { 500: '#22c55e', 600: '#16a34a', 400: '#4ade80' },
			teal: { 500: '#14b8a6', 600: '#0f766e', 400: '#2dd4bf' },
			blue: { 500: '#3b82f6', 600: '#2563eb', 400: '#60a5fa' },
			indigo: { 500: '#6366f1', 600: '#4f46e5', 400: '#818cf8' },
			purple: { 500: '#a855f7', 600: '#9333ea', 400: '#c084fc' },
			pink: { 500: '#ec4899', 600: '#db2777', 400: '#f9a8d4' },
			rose: { 500: '#f43f5e', 600: '#e11d48', 400: '#fb7185' },

			'red-400': { 500: '#f87171', 600: '#ef4444', 400: '#fca5a5' },
			'orange-400': { 500: '#fb923c', 600: '#f97316', 400: '#fdba74' },
			'yellow-400': { 500: '#facc15', 600: '#eab308', 400: '#fde047' },
			'green-400': { 500: '#4ade80', 600: '#22c55e', 400: '#86efac' },
			'teal-400': { 500: '#2dd4bf', 600: '#14b8a6', 400: '#5eead4' },
			'blue-400': { 500: '#60a5fa', 600: '#3b82f6', 400: '#93c5fd' },
			'indigo-400': { 500: '#818cf8', 600: '#6366f1', 400: '#a5b4fc' },
			'purple-400': { 500: '#c084fc', 600: '#a855f7', 400: '#d8b4fe' },
			'pink-400': { 500: '#f9a8d4', 600: '#ec4899', 400: '#f0abfc' },
			'rose-400': { 500: '#fb7185', 600: '#f43f5e', 400: '#fecdd3' },

			'red-600': { 500: '#dc2626', 600: '#b91c1c', 400: '#ef4444' },
			'orange-600': { 500: '#ea580c', 600: '#c2410c', 400: '#f97316' },
			'yellow-600': { 500: '#ca8a04', 600: '#a16207', 400: '#eab308' },
			'green-600': { 500: '#16a34a', 600: '#15803d', 400: '#22c55e' },
			'teal-600': { 500: '#0f766e', 600: '#115e59', 400: '#14b8a6' },
			'blue-600': { 500: '#2563eb', 600: '#1d4ed8', 400: '#3b82f6' },
			'indigo-600': { 500: '#4f46e5', 600: '#4338ca', 400: '#6366f1' },
			'purple-600': { 500: '#9333ea', 600: '#7c3aed', 400: '#a855f7' },
			'pink-600': { 500: '#db2777', 600: '#be185d', 400: '#ec4899' },
			'rose-600': { 500: '#e11d48', 600: '#dc2626', 400: '#f43f5e' },

			'red-800': { 500: '#991b1b', 600: '#7f1d1d', 400: '#dc2626' },
			'orange-800': { 500: '#9a3412', 600: '#7c2d12', 400: '#ea580c' },
			'yellow-800': { 500: '#a16207', 600: '#854d0e', 400: '#ca8a04' },
			'green-800': { 500: '#166534', 600: '#14532d', 400: '#16a34a' },
			'teal-800': { 500: '#134e4a', 600: '#115e59', 400: '#0f766e' },
			'blue-800': { 500: '#1e40af', 600: '#1e3a8a', 400: '#2563eb' },
			'indigo-800': { 500: '#3730a3', 600: '#312e81', 400: '#4f46e5' },
			'purple-800': { 500: '#6b21a8', 600: '#581c87', 400: '#9333ea' },
			'pink-800': { 500: '#9d174d', 600: '#831843', 400: '#db2777' },
			'rose-800': { 500: '#9f1239', 600: '#881337', 400: '#e11d48' }
		};

		const surfaceColorMap = {
			dark: {
				base: '#0f172a',
				overlay: 'rgba(15, 23, 42, 0.85)',
				border: 'rgba(55, 65, 81, 0.6)',
				text: '#e2e8f0',
				textMuted: '#94a3b8',
				glow: 'rgba(15, 23, 42, 0.8)'
			},
			light: {
				base: '#f8fafc',
				overlay: 'rgba(248, 250, 252, 0.9)',
				border: 'rgba(203, 213, 225, 0.7)',
				text: '#1e293b',
				textMuted: '#64748b',
				glow: 'rgba(248, 250, 252, 0.7)'
			},
			darker: {
				base: '#020617',
				overlay: 'rgba(2, 6, 23, 0.9)',
				border: 'rgba(30, 41, 59, 0.7)',
				text: '#f1f5f9',
				textMuted: '#94a3b8',
				glow: 'rgba(2, 6, 23, 0.9)'
			},
			lighter: {
				base: '#ffffff',
				overlay: 'rgba(255, 255, 255, 0.95)',
				border: 'rgba(226, 232, 240, 0.8)',
				text: '#0f172a',
				textMuted: '#64748b',
				glow: 'rgba(255, 255, 255, 0.8)'
			},
			slate: {
				base: '#0f172a',
				overlay: 'rgba(15, 23, 42, 0.85)',
				border: 'rgba(51, 65, 85, 0.6)',
				text: '#e2e8f0',
				textMuted: '#94a3b8',
				glow: 'rgba(15, 23, 42, 0.8)'
			},
			gray: {
				base: '#111827',
				overlay: 'rgba(17, 24, 39, 0.85)',
				border: 'rgba(55, 65, 81, 0.6)',
				text: '#f3f4f6',
				textMuted: '#9ca3af',
				glow: 'rgba(17, 24, 39, 0.8)'
			},
			zinc: {
				base: '#09090b',
				overlay: 'rgba(9, 9, 11, 0.9)',
				border: 'rgba(39, 39, 42, 0.7)',
				text: '#fafafa',
				textMuted: '#a1a1aa',
				glow: 'rgba(9, 9, 11, 0.9)'
			},
			neutral: {
				base: '#171717',
				overlay: 'rgba(23, 23, 23, 0.85)',
				border: 'rgba(64, 64, 64, 0.6)',
				text: '#fafafa',
				textMuted: '#a3a3a3',
				glow: 'rgba(23, 23, 23, 0.8)'
			},
			stone: {
				base: '#1c1917',
				overlay: 'rgba(28, 25, 23, 0.85)',
				border: 'rgba(68, 64, 60, 0.6)',
				text: '#fafaf9',
				textMuted: '#a8a29e',
				glow: 'rgba(28, 25, 23, 0.8)'
			},
			'red-surface': {
				base: '#0f0a0a',
				overlay: 'rgba(31, 10, 10, 0.85)',
				border: 'rgba(69, 29, 29, 0.6)',
				text: '#f1f1f1',
				textMuted: '#a1a1aa',
				glow: 'rgba(31, 10, 10, 0.9)'
			},
			'orange-surface': {
				base: '#0f0d07',
				overlay: 'rgba(31, 20, 7, 0.85)',
				border: 'rgba(69, 49, 18, 0.6)',
				text: '#f1f1f1',
				textMuted: '#a1a1aa',
				glow: 'rgba(31, 20, 7, 0.9)'
			},
			'yellow-surface': {
				base: '#0f0f06',
				overlay: 'rgba(31, 31, 6, 0.85)',
				border: 'rgba(69, 49, 15, 0.6)',
				text: '#f1f1f1',
				textMuted: '#a1a1aa',
				glow: 'rgba(31, 31, 6, 0.9)'
			},
			'green-surface': {
				base: '#0a0f0a',
				overlay: 'rgba(10, 31, 10, 0.85)',
				border: 'rgba(29, 69, 29, 0.6)',
				text: '#f1f1f1',
				textMuted: '#a1a1aa',
				glow: 'rgba(10, 31, 10, 0.9)'
			},
			'teal-surface': {
				base: '#0a0f0f',
				overlay: 'rgba(10, 31, 31, 0.85)',
				border: 'rgba(20, 69, 69, 0.6)',
				text: '#f1f1f1',
				textMuted: '#a1a1aa',
				glow: 'rgba(10, 31, 31, 0.9)'
			},
			'blue-surface': {
				base: '#0a0d0f',
				overlay: 'rgba(10, 20, 31, 0.85)',
				border: 'rgba(29, 49, 69, 0.6)',
				text: '#f1f1f1',
				textMuted: '#a1a1aa',
				glow: 'rgba(10, 20, 31, 0.9)'
			},
			'indigo-surface': {
				base: '#0d0a0f',
				overlay: 'rgba(20, 10, 31, 0.85)',
				border: 'rgba(49, 29, 69, 0.6)',
				text: '#f1f1f1',
				textMuted: '#a1a1aa',
				glow: 'rgba(20, 10, 31, 0.9)'
			},
			'purple-surface': {
				base: '#0f0a0f',
				overlay: 'rgba(31, 10, 31, 0.85)',
				border: 'rgba(69, 29, 69, 0.6)',
				text: '#f1f1f1',
				textMuted: '#a1a1aa',
				glow: 'rgba(31, 10, 31, 0.9)'
			},
			'pink-surface': {
				base: '#0f0a0d',
				overlay: 'rgba(31, 10, 20, 0.85)',
				border: 'rgba(69, 29, 49, 0.6)',
				text: '#f1f1f1',
				textMuted: '#a1a1aa',
				glow: 'rgba(31, 10, 20, 0.9)'
			}
		};

		const primary = primaryColorMap[theme.primary as keyof typeof primaryColorMap];
		const surface = surfaceColorMap[theme.surface as keyof typeof surfaceColorMap];

		// Set CSS variables
		root.style.setProperty('--color-primary-500', primary[500]);
		root.style.setProperty('--color-primary-600', primary[600]);
		root.style.setProperty('--color-primary-400', primary[400]);

		// Set surface colors for transparent overlays
			root.style.setProperty('--color-surface-base', surface.base);
			root.style.setProperty('--color-surface-overlay', surface.overlay);
			root.style.setProperty('--color-surface-border', surface.border);
			root.style.setProperty('--color-surface-text', surface.text);
			root.style.setProperty('--color-surface-text-muted', surface.textMuted);

			const surfaceIsLight = (luminanceFromHex(surface.base) ?? 0) > 0.6;
			const placeholderBase = mixHexColors(
				surface.base,
				primary[500],
				surfaceIsLight ? 0.08 : 0.18
			) ?? surface.base;
			const placeholderAccent = mixHexColors(
				surface.base,
				primary[400],
				surfaceIsLight ? 0.14 : 0.28
			) ?? placeholderBase;

			root.style.setProperty('--color-cover-placeholder-base', placeholderBase);
			root.style.setProperty('--color-cover-placeholder-accent', placeholderAccent);
			root.style.setProperty('--color-cover-placeholder-border', surface.border);
			root.style.setProperty('--color-cover-placeholder-icon', surface.textMuted);

			// Set RGB values for gradients (now using surface glow color)
			const surfaceGlowRgb = hexToRgb(surface.glow);

		if (surfaceGlowRgb) root.style.setProperty('--color-surface-glow-rgb', surfaceGlowRgb);

		// Set glow color based on auto mode or custom color
		let glowColorForUse: string;
		if (theme.appearance.glowAutoMode) {
			glowColorForUse = primary[500];
		} else {
			glowColorForUse = theme.appearance.glowColor;
		}
		
		const glowRgb = hexToRgb(glowColorForUse);
		const intensity = theme.appearance.glowIntensity / 100;
		if (glowRgb) {
			root.style.setProperty('--color-glow-rgb', glowRgb);
			root.style.setProperty('--color-glow-intensity', String(intensity));
		}

		// Apply glow only if enabled
		if (theme.appearance.glowEnabled) {
			root.style.setProperty('--glow-opacity', '1');
		} else {
			root.style.setProperty('--glow-opacity', '0');
		}

		// Apply background images if enabled and any exist
		if (theme.appearance.bgImageEnabled && theme.appearance.backgroundImages && theme.appearance.backgroundImages.length > 0) {
			const transparency = theme.appearance.bgImageTransparency / 100;
			const selectedIndex = theme.appearance.selectedBgImageIndex ?? 0;
			const bgImage = theme.appearance.backgroundImages[selectedIndex] || theme.appearance.backgroundImages[0];
			const display = getBackgroundDisplayCss(theme.appearance.bgImageDisplay);
			root.style.setProperty('--color-bg-image', `url(${bgImage})`);
			root.style.setProperty('--color-bg-image-opacity', String(transparency));
			root.style.setProperty('--color-bg-image-position', display.position);
			root.style.setProperty('--color-bg-image-repeat', display.repeat);
			root.style.setProperty('--color-bg-image-size', display.size);
		} else {
			root.style.removeProperty('--color-bg-image');
			root.style.removeProperty('--color-bg-image-opacity');
			root.style.removeProperty('--color-bg-image-position');
			root.style.removeProperty('--color-bg-image-repeat');
			root.style.removeProperty('--color-bg-image-size');
		}
	}
}

function getBackgroundDisplayCss(display: BackgroundImageDisplay | undefined): { position: string; repeat: string; size: string } {
	switch (display) {
		case 'fit':
			return { position: 'center center', repeat: 'no-repeat', size: 'contain' };
		case 'center':
			return { position: 'center center', repeat: 'no-repeat', size: 'auto' };
		case 'stretch':
			return { position: 'center center', repeat: 'no-repeat', size: '100% 100%' };
		case 'tile':
			return { position: 'top left', repeat: 'repeat', size: 'auto' };
		case 'fill':
		default:
			return { position: 'center center', repeat: 'no-repeat', size: 'cover' };
	}
}

function hexToRgb(hex: string): string | null {
	const result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex);
	return result ? `${parseInt(result[1], 16)}, ${parseInt(result[2], 16)}, ${parseInt(result[3], 16)}` : null;
}

function hexToRgbTuple(hex: string): [number, number, number] | null {
	const result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex);
	if (!result) return null;
	return [
		parseInt(result[1], 16),
		parseInt(result[2], 16),
		parseInt(result[3], 16)
	];
}

function luminanceFromHex(hex: string): number | null {
	const rgb = hexToRgbTuple(hex);
	if (!rgb) return null;

	const transform = (value: number) => (
		value <= 0.03928 ? value / 12.92 : Math.pow((value + 0.055) / 1.055, 2.4)
	);

	const [r8, g8, b8] = rgb.map((value) => value / 255);
	const r = transform(r8);
	const g = transform(g8);
	const b = transform(b8);
	return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function mixHexColors(baseHex: string, accentHex: string, accentWeight: number): string | null {
	const base = hexToRgbTuple(baseHex);
	const accent = hexToRgbTuple(accentHex);
	if (!base || !accent) return null;

	const weight = Math.max(0, Math.min(1, accentWeight));
	const mix = (baseValue: number, accentValue: number) => Math.round(
		baseValue * (1 - weight) + accentValue * weight
	);

	return `#${[mix(base[0], accent[0]), mix(base[1], accent[1]), mix(base[2], accent[2])]
		.map((value) => value.toString(16).padStart(2, '0'))
		.join('')}`;
}

// Helper functions to update appearance settings
export function updateGlowAutoMode(autoMode: boolean) {
	currentTheme.update(t => ({
		...t,
		appearance: { ...t.appearance, glowAutoMode: autoMode }
	}));
}

export function updateGlowColor(color: string) {
	currentTheme.update(t => ({
		...t,
		appearance: { ...t.appearance, glowColor: color }
	}));
}

export function updateGlowIntensity(intensity: number) {
	currentTheme.update(t => ({
		...t,
		appearance: { ...t.appearance, glowIntensity: Math.max(0, Math.min(100, intensity)) }
	}));
}

export function addBackgroundImage(imageData: string) {
	currentTheme.update(t => ({
		...t,
		appearance: {
			...t.appearance,
			backgroundImages: [...t.appearance.backgroundImages, imageData]
		}
	}));
}

export function removeBackgroundImage(index: number) {
	currentTheme.update(t => ({
		...t,
		appearance: {
			...t.appearance,
			backgroundImages: t.appearance.backgroundImages.filter((_, i) => i !== index)
		}
	}));
}

export function updateGlowEnabled(enabled: boolean) {
	currentTheme.update(t => ({
		...t,
		appearance: { ...t.appearance, glowEnabled: enabled }
	}));
}

export function updateBgImageEnabled(enabled: boolean) {
	currentTheme.update(t => ({
		...t,
		appearance: { ...t.appearance, bgImageEnabled: enabled }
	}));
}

export function updateBgImageTransparency(transparency: number) {
	currentTheme.update(t => ({
		...t,
		appearance: { ...t.appearance, bgImageTransparency: Math.max(0, Math.min(100, transparency)) }
	}));
}

export function updateBgImageDisplay(display: BackgroundImageDisplay) {
	currentTheme.update(t => ({
		...t,
		appearance: { ...t.appearance, bgImageDisplay: display }
	}));
}

export function updateSelectedBgImage(index: number) {
  currentTheme.update(t => ({
    ...t,
    appearance: { ...t.appearance, selectedBgImageIndex: index }
  }));
}

export function addCustomTheme(theme: CustomTheme) {
  currentTheme.update(t => ({
    ...t,
    appearance: {
      ...t.appearance,
      customThemes: [...t.appearance.customThemes, theme]
    }
  }));
}

export function updateCustomTheme(id: string, updates: Partial<CustomTheme>) {
  currentTheme.update(t => ({
    ...t,
    appearance: {
      ...t.appearance,
      customThemes: t.appearance.customThemes.map(ct => 
        ct.id === id ? { ...ct, ...updates } : ct
      )
    }
  }));
}

export function removeCustomTheme(id: string) {
  currentTheme.update(t => ({
    ...t,
    appearance: {
      ...t.appearance,
      customThemes: t.appearance.customThemes.filter(ct => ct.id !== id),
      selectedCustomThemeId: t.appearance.selectedCustomThemeId === id ? null : t.appearance.selectedCustomThemeId
    }
  }));
}

export function selectCustomTheme(id: string | null) {
  currentTheme.update(t => ({
    ...t,
    appearance: { ...t.appearance, selectedCustomThemeId: id }
  }));
}

export function getCustomThemeColors(themeId: string | null, themes: CustomTheme[]): { foreground: string; background: string } | null {
  if (!themeId) return null;
  const customTheme = themes.find(t => t.id === themeId);
  if (!customTheme) return null;
  return {
    foreground: customTheme.foreground,
    background: customTheme.background
  };
}

export function resolveThemeColors(
  themeId: string | null,
  builtInThemes: Array<{ id: string; bg: string; text: string }>,
  customThemes: CustomTheme[],
  fallback: ThemeColors = { foreground: '#e5e7eb', background: '#111111' }
): ThemeColors {
  if (!themeId) return fallback;

  const builtIn = builtInThemes.find(theme => theme.id === themeId);
  if (builtIn) {
    return {
      foreground: builtIn.text,
      background: builtIn.bg
    };
  }

  const customTheme = customThemes.find(theme => theme.id === themeId);
  if (customTheme) {
    return {
      foreground: customTheme.foreground,
      background: customTheme.background
    };
  }

  return fallback;
}

export function generateId(): string {
  return 'theme_' + Date.now() + '_' + Math.random().toString(36).substr(2, 9);
}

// Initialize theme colors
updateThemeColors(initialTheme);

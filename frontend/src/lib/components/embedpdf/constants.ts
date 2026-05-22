export const EMBEDPDF_DOCUMENT_ID = 'cryptorum-pdf';

export const readingOnlyDisabledCategories = [
	'mode',
	'annotation',
	'redaction',
	'insert',
	'form',
	'stamp',
	'stamps',
	'signature',
	'history'
];

export const embedPdfSelectors = {
	leftGroup: '[data-epdf-i="left-group"]',
	centerGroup: '[data-epdf-i="center-group"]',
	rightGroup: '[data-epdf-i="right-group"]',
	searchInput: [
		'input[type="search"]',
		'input[placeholder="Search"]',
		'input[aria-label="Search"]',
		'input[aria-label*="search" i]',
		'[data-epdf-i="search-input"] input',
		'[data-epdf-cat~="panel-search"] input',
		'[data-epdf-i="search-panel"] input'
	].join(', '),
	searchToggle: [
		'[data-epdf-i="toggle-search"]',
		'button[data-epdf-cat~="panel-search"]',
		'[role="button"][data-epdf-cat~="panel-search"]',
		'button[aria-label*="search" i]',
		'button[title*="search" i]'
	].join(', '),
	chromeOrSidebar: [
		'button',
		'a',
		'input',
		'textarea',
		'select',
		'label',
		'[contenteditable="true"]',
		'[role="button"]',
		'[data-epdf-i="left-group"]',
		'[data-epdf-i="center-group"]',
		'[data-epdf-i="right-group"]',
		'[data-epdf-i="sidebar-panel"]',
		'[data-epdf-i="search-panel"]',
		'[data-epdf-cat~="panel-sidebar"]',
		'[data-epdf-cat~="panel-search"]'
	].join(', '),
	hiddenChrome: [
		'[data-epdf-i="comment-button"]',
		'[data-epdf-cat~="panel-comment"]',
		'[data-epdf-i="document-menu-button"]',
		'[data-epdf-cat~="document-menu"]',
		'button[aria-label="Document Menu"]',
		'[data-epdf-i="divider-1"]'
	].join(', '),
	pageControls: [
		'[data-overlay-id="page-controls"]',
		'[data-epdf-i="page-controls"]',
		'[data-epdf-cat~="page-controls"]'
	].join(', ')
};

export const embedPdfTheme = {
	preference: 'dark' as const,
	light: {
		background: {
			app: 'var(--color-surface-base, #0f172a)',
			surface: 'var(--color-surface-base, #0f172a)',
			surfaceAlt: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
			elevated: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
			overlay: 'rgba(0, 0, 0, 0.6)',
			input: 'var(--color-surface-base, #0f172a)'
		},
		foreground: {
			primary: 'var(--color-surface-text, #e2e8f0)',
			secondary: 'var(--color-surface-text, #e2e8f0)',
			muted: 'var(--color-surface-text-muted, #94a3b8)',
			disabled: 'color-mix(in srgb, var(--color-surface-text-muted, #94a3b8) 55%, transparent)',
			onAccent: '#ffffff'
		},
		border: {
			default: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
			subtle: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
			strong: 'var(--color-primary-500, #f97316)'
		},
		accent: {
			primary: 'var(--color-primary-500, #f97316)',
			primaryHover: 'var(--color-primary-600, #ea580c)',
			primaryActive: 'var(--color-primary-600, #ea580c)',
			primaryLight: 'color-mix(in srgb, var(--color-primary-500, #f97316) 20%, transparent)',
			primaryForeground: '#ffffff'
		},
		interactive: {
			hover: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
			active: 'color-mix(in srgb, var(--color-primary-500, #f97316) 24%, transparent)',
			selected: 'color-mix(in srgb, var(--color-primary-500, #f97316) 18%, transparent)',
			focus: 'var(--color-primary-500, #f97316)',
			focusRing: 'color-mix(in srgb, var(--color-primary-500, #f97316) 35%, transparent)'
		},
		scrollbar: {
			track: 'transparent',
			thumb: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
			thumbHover: 'var(--color-primary-500, #f97316)'
		},
		tooltip: {
			background: 'var(--color-surface-base, #0f172a)',
			foreground: 'var(--color-surface-text, #e2e8f0)'
		}
	},
	dark: {
		background: {
			app: 'var(--color-surface-base, #0f172a)',
			surface: 'var(--color-surface-base, #0f172a)',
			surfaceAlt: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
			elevated: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
			overlay: 'rgba(0, 0, 0, 0.6)',
			input: 'var(--color-surface-base, #0f172a)'
		},
		foreground: {
			primary: 'var(--color-surface-text, #e2e8f0)',
			secondary: 'var(--color-surface-text, #e2e8f0)',
			muted: 'var(--color-surface-text-muted, #94a3b8)',
			disabled: 'color-mix(in srgb, var(--color-surface-text-muted, #94a3b8) 55%, transparent)',
			onAccent: '#ffffff'
		},
		border: {
			default: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
			subtle: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
			strong: 'var(--color-primary-500, #f97316)'
		},
		accent: {
			primary: 'var(--color-primary-500, #f97316)',
			primaryHover: 'var(--color-primary-600, #ea580c)',
			primaryActive: 'var(--color-primary-600, #ea580c)',
			primaryLight: 'color-mix(in srgb, var(--color-primary-500, #f97316) 20%, transparent)',
			primaryForeground: '#ffffff'
		},
		interactive: {
			hover: 'var(--color-surface-overlay, rgba(15, 23, 42, 0.85))',
			active: 'color-mix(in srgb, var(--color-primary-500, #f97316) 24%, transparent)',
			selected: 'color-mix(in srgb, var(--color-primary-500, #f97316) 18%, transparent)',
			focus: 'var(--color-primary-500, #f97316)',
			focusRing: 'color-mix(in srgb, var(--color-primary-500, #f97316) 35%, transparent)'
		},
		scrollbar: {
			track: 'transparent',
			thumb: 'var(--color-surface-border, rgba(55, 65, 81, 0.6))',
			thumbHover: 'var(--color-primary-500, #f97316)'
		},
		tooltip: {
			background: 'var(--color-surface-base, #0f172a)',
			foreground: 'var(--color-surface-text, #e2e8f0)'
		}
	}
};

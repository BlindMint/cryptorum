<script lang="ts">
	interface Props {
		path: string;
		libraryPaths?: string[];
		className?: string;
	}

	let { path, libraryPaths = [], className = '' }: Props = $props();
	let copied = $state(false);
	let copyTimer: ReturnType<typeof setTimeout> | null = null;

	function normalizePath(value: string): string {
		return value.replace(/\\/g, '/').replace(/\/+$/g, '');
	}

	function displayPath(): string {
		const fullPath = path.trim();
		if (!fullPath) return '';
		const normalizedFullPath = normalizePath(fullPath);
		const roots = libraryPaths
			.map((root) => normalizePath(root.trim()))
			.filter(Boolean)
			.sort((a, b) => b.length - a.length);

		for (const root of roots) {
			if (normalizedFullPath === root) {
				return fullPath.split(/[/\\]/).pop() || fullPath;
			}
			if (normalizedFullPath.startsWith(root + '/')) {
				return normalizedFullPath.slice(root.length + 1);
			}
		}
		return fullPath;
	}

	async function copyFullPath() {
		if (!path) return;
		try {
			await navigator.clipboard.writeText(path);
			copied = true;
			if (copyTimer) clearTimeout(copyTimer);
			copyTimer = setTimeout(() => copied = false, 1200);
		} catch (error) {
			console.error('Failed to copy path:', error);
		}
	}
</script>

<div class={`flex min-w-0 items-center gap-2 ${className}`}>
	<span class="line-clamp-3 min-w-0 flex-1 break-all font-mono text-xs leading-relaxed text-[var(--color-surface-text)]" title={path}>
		{displayPath()}
	</span>
	<button
		type="button"
		onclick={copyFullPath}
		class="inline-flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] text-[var(--color-surface-text-muted)] transition-all duration-200 ease-out hover:-translate-y-px hover:bg-[var(--color-surface-600)] hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]"
		title={copied ? 'Copied full path' : 'Copy full path'}
		aria-label={copied ? 'Copied full path' : 'Copy full path'}
	>
		{#if copied}
			<svg class="h-3.5 w-3.5 text-[var(--color-primary-300)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7"></path>
			</svg>
		{:else}
			<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 8h10a2 2 0 012 2v8a2 2 0 01-2 2H8a2 2 0 01-2-2V10a2 2 0 012-2z"></path>
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 8V6a2 2 0 00-2-2H6a2 2 0 00-2 2v8a2 2 0 002 2h2"></path>
			</svg>
		{/if}
	</button>
</div>

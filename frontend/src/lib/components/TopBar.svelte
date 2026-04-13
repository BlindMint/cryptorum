<script lang="ts">
	import { goto } from '$app/navigation';
	import { mobileMenuOpen } from '$lib/stores';
	import ThemeSelector from './ThemeSelector.svelte';
	import NotificationBell from './NotificationBell.svelte';

	let searchQuery = $state('');

	function handleSearch(e: Event) {
		e.preventDefault();
		if (searchQuery.trim()) {
			goto(`/search?q=${encodeURIComponent(searchQuery.trim())}`);
			searchQuery = '';
		}
	}
</script>

<header class="h-16 bg-[var(--color-surface-overlay)] backdrop-blur-sm border-b border-[var(--color-surface-border)] flex items-center justify-between px-4 lg:px-6 z-50 relative">
	<div class="flex items-center space-x-3">
		<button
			class="lg:hidden p-2 rounded-lg text-gray-400 hover:text-white hover:bg-gray-700 transition-colors"
			aria-label="Toggle navigation menu"
			onclick={() => $mobileMenuOpen = !$mobileMenuOpen}
		>
			<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
			</svg>
		</button>

	{#if $mobileMenuOpen}
		<div
			class="lg:hidden fixed inset-0 bg-black/50 z-40"
			onclick={() => $mobileMenuOpen = false}
			role="button"
			tabindex="-1"
			onkeydown={(e) => e.key === 'Escape' && ($mobileMenuOpen = false)}
		></div>
	{/if}

		<div class="w-10 h-10 rounded-xl bg-gradient-to-br from-[var(--color-primary-400)] to-[var(--color-primary-600)] flex items-center justify-center">
			<svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
			</svg>
		</div>
		<div>
			<h1 class="text-lg font-bold text-[var(--color-surface-text)]">Cryptorum</h1>
			<p class="text-xs text-[var(--color-surface-text-muted)]">Personal Library</p>
		</div>
	</div>
	<div class="flex items-center flex-1 justify-center">
		<form onsubmit={handleSearch} class="w-full max-w-md">
			<div class="relative">
				<input
					type="text"
					bind:value={searchQuery}
					placeholder="Search books..."
					class="w-full pl-10 pr-4 py-2 rounded-lg bg-[var(--color-surface-overlay)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)] focus:border-transparent transition-all"
				/>
				<svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
				</svg>
			</div>
		</form>
	</div>

	<div class="flex items-center space-x-3">
		<NotificationBell />
		<ThemeSelector />

		<a
			href="/history"
			class="p-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] transition-colors"
			title="Reading History"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
			</svg>
		</a>

		<a
			href="/stats"
			class="p-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] transition-colors"
			title="Statistics"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"></path>
			</svg>
		</a>

		<button
			onclick={async () => {
				try {
					await fetch('/api/scan', { method: 'POST' });
				} catch (e) {
					console.error('Scan failed:', e);
				}
			}}
			class="p-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] transition-colors"
			title="Scan Library"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
			</svg>
		</button>

		<a
			href="/settings"
			class="p-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] transition-colors"
			title="Settings"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
			</svg>
		</a>
	</div>
</header>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { mobileMenuOpen } from '$lib/stores';
	import ThemeSelector from './ThemeSelector.svelte';
	import NotificationBell from './NotificationBell.svelte';

	let searchQuery = $state('');
	let showMobileActions = $state(false);

	function handleSearch(e: Event) {
		e.preventDefault();
		if (searchQuery.trim()) {
			goto(`/search?q=${encodeURIComponent(searchQuery.trim())}`);
			searchQuery = '';
			showMobileActions = false;
		}
	}

	function closeMobileActions() {
		showMobileActions = false;
	}
</script>

<header class="relative z-50 overflow-visible border-b border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] backdrop-blur-sm">
	<div class="flex items-center gap-3 px-3 py-3 lg:px-6 h-16">
		<button
			class="lg:hidden shrink-0 rounded-lg p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
			aria-label="Toggle navigation menu"
			onclick={() => $mobileMenuOpen = !$mobileMenuOpen}
		>
			<svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
			</svg>
		</button>

		<div class="hidden items-center space-x-3 shrink-0 lg:flex">
			<div class="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-br from-[var(--color-primary-400)] to-[var(--color-primary-600)]">
				<svg class="h-5 w-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
				</svg>
			</div>
			<div>
				<h1 class="text-lg font-bold text-[var(--color-surface-text)]">Cryptorum</h1>
				<p class="text-xs text-[var(--color-surface-text-muted)]">Personal Library</p>
			</div>
		</div>

		<form onsubmit={handleSearch} class="min-w-0 flex-1 lg:flex lg:justify-center">
			<div class="relative w-full lg:max-w-md">
				<input
					type="text"
					bind:value={searchQuery}
					placeholder="Search books..."
					autocomplete="off"
					autocapitalize="none"
					autocorrect="off"
					spellcheck="false"
					class="w-full min-w-0 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] py-2.5 pl-10 pr-4 text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] transition-all focus:border-transparent focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
				/>
				<svg class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
				</svg>
			</div>
		</form>

		<div class="hidden items-center space-x-3 lg:flex">
			<NotificationBell />
			<ThemeSelector />

			<a
				href="/history"
				class="rounded-lg p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
				title="Reading History"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
				</svg>
			</a>

			<a
				href="/stats"
				class="rounded-lg p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
				title="Statistics"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
				class="rounded-lg p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
				title="Scan Library"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
				</svg>
			</button>

			<a
				href="/settings"
				class="rounded-lg p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
				title="Settings"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
				</svg>
			</a>
		</div>

		<button
			type="button"
			class="lg:hidden shrink-0 rounded-lg p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
			aria-label="Open quick actions"
			aria-expanded={showMobileActions}
			onclick={() => showMobileActions = !showMobileActions}
		>
			<svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6h.01M12 12h.01M12 18h.01"></path>
			</svg>
		</button>
	</div>

	{#if showMobileActions}
		<button
			type="button"
			class="fixed inset-0 z-40 lg:hidden"
			aria-label="Close quick actions"
			onclick={closeMobileActions}
		></button>
		<div class="absolute right-3 top-[calc(100%-0.25rem)] z-50 lg:hidden w-[min(22rem,calc(100vw-1.5rem))] overflow-hidden rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl backdrop-blur-sm">
			<div class="grid gap-2 p-3">
				<div class="mobile-action-row">
					<NotificationBell mobileMenu />
				</div>
				<div class="mobile-action-row">
					<ThemeSelector mobileMenu />
				</div>
				<a
					href="/history"
					onclick={closeMobileActions}
					class="mobile-action-link flex items-center gap-3 rounded-lg px-3 py-2 text-sm text-[var(--color-surface-text)] transition-all"
				>
					<svg class="h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
					</svg>
					<span>Reading History</span>
				</a>
				<a
					href="/stats"
					onclick={closeMobileActions}
					class="mobile-action-link flex items-center gap-3 rounded-lg px-3 py-2 text-sm text-[var(--color-surface-text)] transition-all"
				>
					<svg class="h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"></path>
					</svg>
					<span>Statistics</span>
				</a>
				<button
					onclick={async () => {
						closeMobileActions();
						try {
							await fetch('/api/scan', { method: 'POST' });
						} catch (e) {
							console.error('Scan failed:', e);
						}
					}}
					class="mobile-action-link flex items-center gap-3 rounded-lg px-3 py-2 text-left text-sm text-[var(--color-surface-text)] transition-all"
				>
					<svg class="h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
					</svg>
					<span>Scan Library</span>
				</button>
				<a
					href="/settings"
					onclick={closeMobileActions}
					class="mobile-action-link flex items-center gap-3 rounded-lg px-3 py-2 text-sm text-[var(--color-surface-text)] transition-all"
				>
					<svg class="h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
					</svg>
					<span>Settings</span>
				</a>
			</div>
		</div>
	{/if}
</header>

<style>
	.mobile-action-row {
		display: flex;
		align-items: center;
		justify-content: flex-start;
		gap: 0.5rem;
		border: 1px solid var(--color-surface-border);
		border-radius: 0.5rem;
		background: var(--color-surface-base);
		padding: 0.5rem 0.75rem;
		transition: border-color 160ms ease, box-shadow 160ms ease, background-color 160ms ease;
	}

	.mobile-action-row:hover {
		border-color: color-mix(in srgb, var(--color-primary-500) 55%, var(--color-surface-border));
		box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-primary-500) 28%, transparent);
	}

	.mobile-action-link {
		border: 1px solid transparent;
	}

	.mobile-action-link:hover {
		background: var(--color-surface-base);
		border-color: color-mix(in srgb, var(--color-primary-500) 55%, var(--color-surface-border));
		box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-primary-500) 22%, transparent);
	}
</style>

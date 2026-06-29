<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { desktopSidebarCollapsed, mobileMenuOpen } from '$lib/stores';
	import AppLogo from '$lib/components/AppLogo.svelte';
	import FullscreenToggle from './FullscreenToggle.svelte';
	import ThemeSelector from './ThemeSelector.svelte';
	import NotificationBell from './NotificationBell.svelte';

	type MobileActionsView = 'menu' | 'notifications' | 'theme';

	let searchQuery = $state('');
	let showMobileActions = $state(false);
	let mobileActionsView = $state<MobileActionsView>('menu');
	let lastRouteKey = '';

	function handleSearch(e: Event) {
		e.preventDefault();
		if (searchQuery.trim()) {
			const params = new URLSearchParams();
			params.set('q', searchQuery.trim());
			closeAllMobilePanels();
			goto(`/library?${params.toString()}`);
			searchQuery = '';
		}
	}

	function closeMobileActions() {
		showMobileActions = false;
		mobileActionsView = 'menu';
	}

	function closeAllMobilePanels() {
		$mobileMenuOpen = false;
		closeMobileActions();
	}

	function openMobileNotifications() {
		mobileActionsView = 'notifications';
	}

	function openMobileTheme() {
		mobileActionsView = 'theme';
	}

	function toggleMobileActions() {
		if (showMobileActions) {
			closeMobileActions();
		} else {
			$mobileMenuOpen = false;
			mobileActionsView = 'menu';
			showMobileActions = true;
		}
	}

	function toggleMobileNavigation() {
		if ($mobileMenuOpen) {
			$mobileMenuOpen = false;
		} else {
			closeMobileActions();
			$mobileMenuOpen = true;
		}
	}

	function getMobileActionsTitle() {
		switch (mobileActionsView) {
			case 'notifications':
				return 'Notifications';
			case 'theme':
				return 'Theme';
			default:
				return 'Quick Actions';
		}
	}

	function toggleDesktopSidebar() {
		desktopSidebarCollapsed.update((collapsed) => !collapsed);
	}

	$effect(() => {
		const routeKey = `${$page.url.pathname}${$page.url.search}`;
		if (lastRouteKey && routeKey !== lastRouteKey) {
			closeMobileActions();
		}
		lastRouteKey = routeKey;
	});

</script>

<header class="relative z-50 overflow-visible border-b border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] backdrop-blur-sm">
	<div class="flex h-[4.75rem] items-center gap-3 px-3 py-4 lg:h-20 lg:px-6">
		<button
			class="lg:hidden shrink-0 rounded-lg p-3 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
			aria-label="Toggle navigation menu"
			onclick={toggleMobileNavigation}
		>
			<svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
			</svg>
		</button>

		<div class="hidden items-center space-x-3 shrink-0 lg:flex">
			<AppLogo sizeClass="h-10 w-10" roundedClass="rounded-xl" />
			<div>
				<h1 class="text-lg font-bold text-[var(--color-surface-text)]">Cryptorum</h1>
				<p class="text-xs text-[var(--color-surface-text-muted)]">Personal Library</p>
			</div>
		</div>

		<button
			type="button"
			class={`hidden shrink-0 rounded-lg p-3 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]/70 lg:inline-flex ${$desktopSidebarCollapsed ? 'bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)]' : ''}`}
			aria-controls="app-sidebar"
			aria-expanded={!$desktopSidebarCollapsed}
			aria-label={$desktopSidebarCollapsed ? 'Open sidebar' : 'Collapse sidebar'}
			title={$desktopSidebarCollapsed ? 'Open sidebar' : 'Collapse sidebar'}
			onclick={toggleDesktopSidebar}
		>
			<svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
			</svg>
		</button>

		<form onsubmit={handleSearch} class="min-w-0 flex-1 lg:flex">
			<div class="relative w-full">
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
			<FullscreenToggle />
			<NotificationBell />
			<ThemeSelector />

			<a
				href="/history"
				class="rounded-lg p-3 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
				title="Reading History"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
				</svg>
			</a>

			<a
				href="/stats"
				class="rounded-lg p-3 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
				title="Statistics"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"></path>
				</svg>
			</a>

			<a
				href="/settings"
				class="rounded-lg p-3 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
				title="Settings"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
				</svg>
			</a>
		</div>

		<div class="lg:hidden">
			<FullscreenToggle />
		</div>

		<button
			type="button"
			class="lg:hidden shrink-0 rounded-lg p-3 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
			aria-label="Open quick actions"
			aria-expanded={showMobileActions}
			onclick={toggleMobileActions}
		>
			<svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6h.01M12 12h.01M12 18h.01"></path>
			</svg>
		</button>
	</div>

</header>

{#if showMobileActions}
	<button
		type="button"
		class="fixed inset-0 z-[80] bg-black/80 lg:hidden"
		aria-label="Close quick actions"
		onclick={closeMobileActions}
	></button>
	<div class="mobile-actions-panel {mobileActionsView === 'menu' ? 'mobile-actions-panel-menu' : 'mobile-actions-panel-detail'}">
		{#if mobileActionsView === 'menu'}
			<div class="grid gap-2 p-3">
				<button type="button" class="mobile-action-row w-full" onclick={openMobileNotifications}>
					<span class="flex items-center gap-3">
						<svg class="h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C8.67 6.165 7 8.388 7 11v3.159c0 .538-.214 1.055-.595 1.436L5 17h5m5 0a3 3 0 11-6 0m6 0H9"></path>
						</svg>
						<span class="text-sm font-medium text-[var(--color-surface-text)]">Notifications</span>
					</span>
				</button>
				<button type="button" class="mobile-action-row w-full" onclick={openMobileTheme}>
					<span class="flex items-center gap-3">
						<svg class="h-5 w-5 text-[var(--color-surface-text-muted)]" viewBox="0 -960 960 960" fill="currentColor">
							<path d="M480-80q-82 0-155-31.5t-127.5-86Q143-252 111.5-325T80-480q0-83 32.5-156t88-127Q256-817 330-848.5T488-880q80 0 151 27.5t124.5 76q53.5 48.5 85 115T880-518q0 115-70 176.5T640-280h-74q-9 0-12.5 5t-3.5 11q0 12 15 34.5t15 51.5q0 50-27.5 74T480-80Zm0-400Zm-177 23q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17Zm120-160q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17Zm200 0q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17Zm120 160q17-17 17-43t-17-43q-17-17-43-17t-43 17q-17 17-17 43t17 43q17 17 43 17t43-17ZM480-160q9 0 14.5-5t5.5-13q0-14-15-33t-15-57q0-42 29-67t71-25h70q66 0 113-38.5T800-518q0-121-92.5-201.5T488-800q-136 0-232 93t-96 227q0 133 93.5 226.5T480-160Z"/>
						</svg>
						<span class="text-sm font-medium text-[var(--color-surface-text)]">Theme</span>
					</span>
				</button>
				<a
					href="/history"
					onclick={closeAllMobilePanels}
					class="mobile-action-link flex items-center gap-3 rounded-lg px-3 py-2 text-sm text-[var(--color-surface-text)] transition-all"
				>
					<svg class="h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
					</svg>
					<span>Reading History</span>
				</a>
				<a
					href="/stats"
					onclick={closeAllMobilePanels}
					class="mobile-action-link flex items-center gap-3 rounded-lg px-3 py-2 text-sm text-[var(--color-surface-text)] transition-all"
				>
					<svg class="h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"></path>
					</svg>
					<span>Statistics</span>
				</a>
				<a
					href="/settings"
					onclick={closeAllMobilePanels}
					class="mobile-action-link flex items-center gap-3 rounded-lg px-3 py-2 text-sm text-[var(--color-surface-text)] transition-all"
				>
					<svg class="h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
					</svg>
					<span>Settings</span>
				</a>
			</div>
		{:else}
			<div class="mobile-actions-header">
				<button type="button" class="mobile-actions-back" onclick={() => mobileActionsView = 'menu'}>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
					</svg>
					<span>Back</span>
				</button>
				<div class="text-sm font-semibold text-[var(--color-surface-text)]">{getMobileActionsTitle()}</div>
				<button type="button" class="mobile-actions-close" aria-label="Close quick actions" onclick={closeMobileActions}>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
					</svg>
				</button>
			</div>
			<div class="mobile-actions-content">
				{#if mobileActionsView === 'notifications'}
					<NotificationBell mobileMenu panelOnly hideHeader onClose={closeMobileActions} />
				{:else if mobileActionsView === 'theme'}
					<ThemeSelector mobileMenu panelOnly />
				{/if}
			</div>
		{/if}
	</div>
{/if}

<style>
	.mobile-actions-panel {
		position: fixed;
		right: 0.75rem;
		top: 4.25rem;
		z-index: 90;
		width: min(22rem, calc(100vw - 1.5rem));
		max-height: calc(100dvh - 5rem);
		overflow: hidden;
		border: 1px solid var(--color-surface-border);
		border-radius: 0.75rem;
		background: var(--color-surface-overlay);
		box-shadow: 0 25px 50px -12px rgb(0 0 0 / 0.55);
		backdrop-filter: blur(10px);
	}

	.mobile-actions-panel-detail {
		display: flex;
		width: min(30rem, calc(100vw - 1.5rem));
		flex-direction: column;
	}

	.mobile-actions-header {
		display: flex;
		flex-shrink: 0;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		border-bottom: 1px solid var(--color-surface-border);
		padding: 0.75rem;
	}

	.mobile-actions-back,
	.mobile-actions-close {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border-radius: 0.5rem;
		color: var(--color-surface-text-muted);
		transition: background-color 160ms ease, color 160ms ease;
	}

	.mobile-actions-back {
		gap: 0.25rem;
		padding: 0.375rem 0.5rem;
		font-size: 0.8125rem;
		font-weight: 500;
	}

	.mobile-actions-close {
		height: 2rem;
		width: 2rem;
	}

	.mobile-actions-back:hover,
	.mobile-actions-close:hover {
		background: var(--color-surface-base);
		color: var(--color-surface-text);
	}

	.mobile-actions-content {
		min-height: 0;
		overflow-y: auto;
	}

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
		border: 1px solid var(--color-surface-border);
		background: var(--color-surface-base);
	}

	.mobile-action-link:hover {
		background: var(--color-surface-base);
		border-color: color-mix(in srgb, var(--color-primary-500) 55%, var(--color-surface-border));
		box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-primary-500) 22%, transparent);
	}

	@media (max-width: 30rem) {
		.mobile-actions-panel {
			left: 0.75rem;
			right: 0.75rem;
			width: auto;
		}
	}
</style>

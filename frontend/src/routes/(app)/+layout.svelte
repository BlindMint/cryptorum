<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import TopBar from '$lib/components/TopBar.svelte';
	import { mobileMenuOpen } from '$lib/stores';
	import { readerSettings } from '$lib/stores/readerSettings';

	let { children } = $props();
	let authenticated = $state(false);
	let loading = $state(true);
	let wakeLock: WakeLockSentinel | null = null;
	let wakeLockRequestInFlight = false;
	let lastRouteKey = '';

	const isReaderPage = $derived($page.url.pathname.includes('/reader/'));
	const isLibraryPage = $derived($page.url.pathname === '/library');
	const keepScreenOn = $derived(
		authenticated && (
			$readerSettings.keepScreenOnWhileAppOpen ||
			(isReaderPage && $readerSettings.keepScreenOnWhileReading)
		)
	);

	async function requestWakeLock() {
		if (
			!keepScreenOn ||
			wakeLock ||
			wakeLockRequestInFlight ||
			!('wakeLock' in navigator) ||
			document.visibilityState !== 'visible'
		) {
			return;
		}

		wakeLockRequestInFlight = true;
		try {
			wakeLock = await navigator.wakeLock.request('screen');
			wakeLock.addEventListener('release', () => {
				wakeLock = null;
			});
		} catch (e) {
			console.warn('Failed to keep screen awake:', e);
		} finally {
			wakeLockRequestInFlight = false;
		}
	}

	function releaseWakeLock() {
		if (!wakeLock) return;
		const lock = wakeLock;
		wakeLock = null;
		lock.release().catch(() => undefined);
	}

	function syncWakeLock() {
		if (keepScreenOn) {
			void requestWakeLock();
		} else {
			releaseWakeLock();
		}
	}

	function handleVisibilityChange() {
		if (document.visibilityState === 'visible') {
			syncWakeLock();
		} else {
			releaseWakeLock();
		}
	}

	$effect(() => {
		if (authenticated) {
			syncWakeLock();
		}
	});

	$effect(() => {
		const routeKey = `${$page.url.pathname}${$page.url.search}`;
		if (lastRouteKey && routeKey !== lastRouteKey) {
			$mobileMenuOpen = false;
		}
		if (typeof window !== 'undefined' && authenticated) {
			try {
				const stack = JSON.parse(sessionStorage.getItem('cryptorumRouteStack') || '[]') as string[];
				const nextStack = stack[stack.length - 1] === routeKey ? stack : [...stack, routeKey].slice(-30);
				sessionStorage.setItem('cryptorumRouteStack', JSON.stringify(nextStack));
			} catch {
				sessionStorage.setItem('cryptorumRouteStack', JSON.stringify([routeKey]));
			}
		}
		lastRouteKey = routeKey;
	});

	onMount(() => {
		let disposed = false;

		async function init() {
			try {
				const res = await fetch('/api/auth/check', { credentials: 'same-origin' });
				const data = await res.json();
				if (!data.authenticated) {
					window.location.href = '/login';
					return;
				}
				if (disposed) return;
				authenticated = true;
				await readerSettings.syncWithBackend();
				if (disposed) return;
				document.addEventListener('visibilitychange', handleVisibilityChange);
			} catch (e) {
				window.location.href = '/login';
				return;
			}
			loading = false;
		}

		void init();

		return () => {
			disposed = true;
			document.removeEventListener('visibilitychange', handleVisibilityChange);
			releaseWakeLock();
		};
	});
 </script>

{#if loading}
	<div class="min-h-[100dvh] bg-[var(--color-surface-base)] flex items-center justify-center overflow-x-hidden">
		<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
	</div>
{:else if authenticated}
	<div class="h-[100dvh] bg-transparent relative overflow-x-hidden">
		<!-- Subtle background texture -->
		<div class="absolute inset-0 opacity-[0.02] pointer-events-none" style="background-image: radial-gradient(circle at 25% 25%, rgba(255,255,255,0.1) 1px, transparent 1px), radial-gradient(circle at 75% 75%, rgba(255,255,255,0.05) 1px, transparent 1px); background-size: 20px 20px;"></div>

		{#if !isReaderPage}
			<TopBar />
		{/if}
		<div class="flex {isReaderPage ? 'h-dvh' : 'h-[calc(100dvh-4rem-1px)]'} relative min-w-0 overflow-x-hidden">
			{#if $mobileMenuOpen}
				<button
					type="button"
					class="fixed inset-0 bg-black/80 z-30 lg:hidden"
					aria-label="Close menu"
					onclick={() => $mobileMenuOpen = false}
				></button>
			{/if}
			{#if !isReaderPage}
				<Sidebar />
			{/if}
				<div class="flex-1 min-w-0 flex flex-col overflow-hidden">
					<main class="flex-1 min-h-0 min-w-0 overflow-y-auto overflow-x-hidden {isReaderPage || isLibraryPage ? '!p-0' : 'p-6'}">
						{@render children()}
					</main>
				</div>
		</div>
	</div>
 {/if}

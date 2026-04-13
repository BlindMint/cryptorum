<script lang="ts">
 	import { onMount } from 'svelte';
 	import { page } from '$app/stores';
 	import Sidebar from '$lib/components/Sidebar.svelte';
 	import TopBar from '$lib/components/TopBar.svelte';
 	import { mobileMenuOpen } from '$lib/stores';

 	let { children } = $props();
 	let authenticated = $state(false);
 	let loading = $state(true);

 	const isReaderPage = $derived($page.url.pathname.includes('/reader/'));
 	const isLibraryPage = $derived($page.url.pathname === '/library');

 	onMount(async () => {
 		try {
 			const res = await fetch('/api/auth/check');
 			const data = await res.json();
 			if (!data.authenticated) {
 				window.location.href = '/login';
 				return;
 			}
 			authenticated = true;
 		} catch (e) {
 			window.location.href = '/login';
 			return;
 		}
 		loading = false;
 	});
 </script>

 {#if loading}
 	<div class="min-h-screen bg-[var(--color-surface-base)] flex items-center justify-center">
 		<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
 	</div>
 {:else if authenticated}
 	<div class="h-screen bg-transparent relative">
 		<!-- Subtle background texture -->
 		<div class="absolute inset-0 opacity-[0.02] pointer-events-none" style="background-image: radial-gradient(circle at 25% 25%, rgba(255,255,255,0.1) 1px, transparent 1px), radial-gradient(circle at 75% 75%, rgba(255,255,255,0.05) 1px, transparent 1px); background-size: 20px 20px;"></div>

		{#if !isReaderPage}
			<TopBar />
  		{/if}
  		<div class="flex {isReaderPage ? 'h-screen' : 'h-[calc(100vh-4rem)]'} relative">
  			{#if $mobileMenuOpen}
  				<button
  					type="button"
  					class="fixed inset-0 bg-black/50 z-30 lg:hidden"
  					aria-label="Close menu"
  					onclick={() => $mobileMenuOpen = false}
  				></button>
  			{/if}
  			{#if !isReaderPage}
  				<Sidebar />
  			{/if}
  			<div class="flex-1 flex flex-col overflow-hidden">
 				<main class="flex-1 overflow-y-auto {isReaderPage || isLibraryPage ? '!p-0' : 'p-6'}">
  					{@render children()}
  				</main>
  			</div>
  		</div>
 	</div>
 {/if}

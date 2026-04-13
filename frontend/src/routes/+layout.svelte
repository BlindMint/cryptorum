<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import '../app.css';

	let { children } = $props();
	let authenticated = $state(false);
	let authDisabled = $state(false);
	let checking = $state(true);

	onMount(async () => {
		await checkAuth();
	});

	async function checkAuth() {
		try {
			const res = await fetch('/api/auth/check');
			const data = await res.json();
			authenticated = data.authenticated || false;
			authDisabled = data.auth_disabled || false;
		} catch (e) {
			authenticated = false;
		}
		checking = false;
	}
</script>

{#if checking}
	<div class="min-h-screen bg-[var(--color-surface-base)] flex items-center justify-center">
		<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
	</div>
{:else}
	{@render children()}
{/if}

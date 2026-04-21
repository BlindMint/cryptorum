<script lang="ts">
	import { onMount } from 'svelte';
	import AppLogo from '$lib/components/AppLogo.svelte';

	let username = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);
	let checked = $state(false);

	onMount(async () => {
		try {
			const res = await fetch('/api/auth/check');
			const data = await res.json();
			if (data.authenticated) {
				window.location.href = '/';
			}
		} catch (e) {
			// Not logged in, show login page
		}
		checked = true;
	});

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			const res = await fetch('/api/auth/login', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ username, password })
			});

			const data = await res.json();

			if (!res.ok || data.error) {
				error = data.error || 'Login failed';
				loading = false;
				return;
			}

			window.location.href = '/';
		} catch (e) {
			error = 'Network error. Please try again.';
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Login - Cryptorum</title>
</svelte:head>

{#if checked}
<div class="min-h-[100dvh] overflow-x-hidden bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900 flex items-center justify-center px-4 py-4 sm:py-8">
	<div class="w-full max-w-md">
		<div class="text-center mb-5 sm:mb-6">
			<div class="mb-3 inline-flex items-center justify-center">
				<AppLogo sizeClass="h-16 w-16 sm:h-20 sm:w-20" roundedClass="rounded-2xl" class="ring-1 ring-white/10" />
			</div>
			<h1 class="text-3xl sm:text-4xl font-bold text-white mb-2">Cryptorum</h1>
			<p class="text-slate-400">Your personal digital library</p>
		</div>

		<div class="bg-slate-800/50 backdrop-blur-sm rounded-2xl p-6 sm:p-8 shadow-xl border border-slate-700/50">
			<h2 class="text-xl font-semibold text-white mb-4 sm:mb-5">Sign in to your library</h2>

			{#if error}
				<div class="mb-4 p-3 rounded-lg bg-red-500/20 border border-red-500/50 text-red-200 text-sm">
					{error}
				</div>
			{/if}

			<form onsubmit={handleSubmit}>
				<div class="mb-4">
					<label for="username" class="block text-sm font-medium text-slate-300 mb-2">Username</label>
					<input
						type="text"
						id="username"
						bind:value={username}
						class="w-full px-4 py-3 rounded-lg bg-slate-900/50 border border-slate-600 text-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-amber-500 focus:border-transparent transition-all"
						placeholder="Enter username"
						required
					/>
				</div>

				<div class="mb-6">
					<label for="password" class="block text-sm font-medium text-slate-300 mb-2">Password</label>
					<input
						type="password"
						id="password"
						bind:value={password}
						class="w-full px-4 py-3 rounded-lg bg-slate-900/50 border border-slate-600 text-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-amber-500 focus:border-transparent transition-all"
						placeholder="Enter password"
						required
					/>
				</div>

				<button
					type="submit"
					disabled={loading}
					class="w-full py-3 px-4 rounded-lg bg-gradient-to-r from-amber-500 to-orange-600 text-white font-semibold hover:from-amber-600 hover:to-orange-700 focus:outline-none focus:ring-2 focus:ring-amber-500 focus:ring-offset-2 focus:ring-offset-slate-800 disabled:opacity-50 disabled:cursor-not-allowed transition-all"
				>
					{#if loading}
						<span class="inline-flex items-center">
							<svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" fill="none" viewBox="0 0 24 24">
								<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
								<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
							</svg>
							Signing in...
						</span>
					{:else}
						Sign in
					{/if}
				</button>
			</form>
		</div>

		<p class="hidden sm:block text-center text-slate-500 text-sm mt-5">
			Cryptorum - Personal Digital Library
		</p>
	</div>
</div>
{/if}

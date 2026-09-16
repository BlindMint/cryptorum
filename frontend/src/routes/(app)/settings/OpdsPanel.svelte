<script lang="ts">
	import { onMount } from 'svelte';

	type OpdsSettings = {
		enabled: boolean;
		catalog_title: string;
		catalog_path: string;
		authentication_required: boolean;
	};

	let settings = $state<OpdsSettings>({
		enabled: true,
		catalog_title: 'Cryptorum Catalog',
		catalog_path: '/opds/',
		authentication_required: false
	});
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let saved = $state(false);
	let copied = $state(false);
	let origin = $state('');
	let catalogUrl = $derived(`${origin}${settings.catalog_path}`);

	async function loadSettings() {
		loading = true;
		error = '';
		try {
			const response = await fetch('/api/settings/opds');
			if (!response.ok) throw new Error('Failed to load OPDS settings');
			settings = { ...settings, ...(await response.json()) };
		} catch (loadError) {
			console.error('Failed to load OPDS settings:', loadError);
			error = 'Unable to load OPDS settings.';
		} finally {
			loading = false;
		}
	}

	async function saveSettings() {
		saving = true;
		error = '';
		saved = false;
		try {
			const response = await fetch('/api/settings/opds', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ enabled: settings.enabled, catalog_title: settings.catalog_title })
			});
			if (!response.ok) {
				const body = await response.json().catch(() => null);
				throw new Error(body?.error || 'Failed to save OPDS settings');
			}
			settings = { ...settings, ...(await response.json()) };
			saved = true;
			window.setTimeout(() => (saved = false), 2500);
		} catch (saveError) {
			console.error('Failed to save OPDS settings:', saveError);
			error = saveError instanceof Error ? saveError.message : 'Unable to save OPDS settings.';
		} finally {
			saving = false;
		}
	}

	async function copyCatalogUrl() {
		try {
			let copiedSuccessfully = false;
			if (navigator.clipboard?.writeText) {
				try {
					await navigator.clipboard.writeText(catalogUrl);
					copiedSuccessfully = true;
				} catch {
					// The Clipboard API can be unavailable on non-HTTPS local deployments.
				}
			}
			if (!copiedSuccessfully) {
				const input = document.createElement('textarea');
				input.value = catalogUrl;
				input.style.position = 'fixed';
				input.style.opacity = '0';
				document.body.appendChild(input);
				input.select();
				copiedSuccessfully = document.execCommand('copy');
				input.remove();
				if (!copiedSuccessfully) throw new Error('Copy command failed');
			}
			copied = true;
			window.setTimeout(() => (copied = false), 2000);
		} catch (copyError) {
			console.error('Failed to copy catalog URL:', copyError);
			error = 'Unable to copy the catalog URL.';
		}
	}

	onMount(() => {
		origin = window.location.origin;
		void loadSettings();
	});
</script>

<div class="space-y-5">
	<section class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)]">
		<div class="flex items-center gap-3 border-b border-[var(--color-surface-border)] px-5 py-3.5">
			<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary-500)]/20">
				<svg class="h-5 w-5 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path></svg>
			</div>
			<div class="min-w-0 space-y-0.5">
				<h2 class="text-lg font-semibold leading-6 text-[var(--color-surface-text)]">OPDS Catalog</h2>
				<p class="text-sm leading-5 text-[var(--color-surface-text-muted)]">Share your library with reading apps that support OPDS 2.</p>
			</div>
		</div>

		{#if loading}
			<div class="flex min-h-48 items-center justify-center p-6">
				<div class="h-8 w-8 animate-spin rounded-full border-b-2 border-[var(--color-primary-500)]"></div>
			</div>
		{:else}
			<form class="space-y-5 p-6" onsubmit={(event) => { event.preventDefault(); void saveSettings(); }}>
				<label class="flex items-center justify-between gap-4 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4">
					<span>
						<span class="block text-sm font-medium text-[var(--color-surface-text)]">Enable OPDS catalog</span>
						<span class="mt-0.5 block text-sm leading-5 text-[var(--color-surface-text-muted)]">When disabled, catalog, cover, and download URLs return not found.</span>
					</span>
					<input type="checkbox" bind:checked={settings.enabled} class="settings-switch">
				</label>

				<label class="block space-y-2">
					<span class="text-sm font-medium text-[var(--color-surface-text)]">Catalog title</span>
					<input
						type="text"
						bind:value={settings.catalog_title}
						maxlength="120"
						required
						class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] focus:border-[var(--color-primary-500)] focus:outline-none"
					>
				</label>

				<div class="space-y-2">
					<span class="text-sm font-medium text-[var(--color-surface-text)]">Catalog URL</span>
					<div class="flex flex-col gap-2 sm:flex-row">
						<input
							type="text"
							value={catalogUrl}
							readonly
							class="min-w-0 flex-1 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 font-mono text-sm text-[var(--color-surface-text)]"
						>
						<button type="button" onclick={copyCatalogUrl} class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-sm text-[var(--color-surface-text)] transition-colors hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)]">
							{copied ? 'Copied' : 'Copy URL'}
						</button>
						<a href={settings.catalog_path} target="_blank" rel="noreferrer" aria-disabled={!settings.enabled} class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-center text-sm text-[var(--color-surface-text)] transition-colors hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] {!settings.enabled ? 'pointer-events-none opacity-50' : ''}">Open catalog</a>
					</div>
				</div>

				<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] p-4 text-sm">
					<p class="font-medium text-[var(--color-surface-text)]">Authentication</p>
					<p class="mt-1 text-[var(--color-surface-text-muted)]">
						{settings.authentication_required ? 'Use the same username and password configured for Cryptorum.' : 'Authentication is disabled in the server configuration.'}
					</p>
					<p class="mt-3 text-[var(--color-surface-text-muted)]">Use HTTPS when accessing the catalog outside a trusted local network so credentials and downloads remain encrypted in transit.</p>
				</div>

				{#if error}<p class="text-sm text-red-500" role="alert">{error}</p>{/if}
				{#if saved}<p class="text-sm text-green-500" role="status">OPDS settings saved.</p>{/if}

				<div class="flex justify-end">
					<button type="submit" disabled={saving} class="accent-action rounded-lg px-4 py-2 font-medium focus-visible:outline-2">
						{saving ? 'Saving…' : 'Save OPDS Settings'}
					</button>
				</div>
			</form>
		{/if}
	</section>
</div>

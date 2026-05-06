<script lang="ts">
	import { onMount } from 'svelte';

	interface Shelf {
		id: number;
		name: string;
		icon: string;
		is_magic: number;
		book_count: number;
	}

	let shelves = $state<Shelf[]>([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			const res = await fetch('/api/shelves');
			if (res.ok) shelves = await res.json();
		} catch (e) {
			console.error('Failed to fetch shelves:', e);
		} finally {
			loading = false;
		}
	});
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold text-[var(--color-surface-text)]">Shelves</h1>
			<p class="text-[var(--color-surface-text-muted)] mt-1">Organize your books into collections</p>
		</div>
		<div class="flex flex-wrap gap-3">
			<a
				href="/shelves/new"
				class="inline-flex items-center px-4 py-2 bg-[var(--color-surface-overlay)] hover:bg-[var(--color-surface-700)] text-[var(--color-surface-text)] font-medium rounded-lg transition-colors border border-[var(--color-surface-border)]"
			>
				<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
				</svg>
				Create Shelf
			</a>
			<a
				href="/shelves/new?magic=true"
				class="inline-flex items-center px-4 py-2 bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white font-medium rounded-lg transition-colors"
			>
				<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"></path>
				</svg>
				Create Magic Shelf
			</a>
		</div>
	</div>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if shelves.length === 0}
		<div class="text-center py-16 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
			<svg class="w-16 h-16 text-[var(--color-primary-400)] mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
			</svg>
			<h3 class="text-lg font-medium text-[var(--color-surface-text)] mb-2">No shelves yet</h3>
			<p class="text-[var(--color-surface-text-muted)]">Create shelves to organize your books</p>
		</div>
	{:else}
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each shelves as shelf}
				<a href="/shelves/{shelf.id}" class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] p-6 hover:border-[var(--color-primary-500)]/50 transition-colors group relative">
					{#if shelf.is_magic === 1}
						<div class="absolute top-3 right-3 flex items-center space-x-1 bg-purple-500/20 text-purple-400 px-2 py-1 rounded-full text-xs font-medium">
							<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"></path>
							</svg>
							<span>Magic</span>
						</div>
					{/if}
					<div class="flex items-center space-x-4">
						<div class="w-12 h-12 rounded-lg {shelf.is_magic === 1 ? 'bg-purple-500/20' : 'bg-[var(--color-primary-500)]/20'} flex items-center justify-center">
							{#if shelf.is_magic === 1}
								<svg class="w-6 h-6 text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"></path>
								</svg>
							{:else}
								<svg class="w-6 h-6 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
								</svg>
							{/if}
						</div>
						<div class="flex-1">
							<h3 class="font-semibold text-[var(--color-surface-text)] group-hover:text-[var(--color-primary-400)] transition-colors">{shelf.name}</h3>
							<p class="text-sm text-[var(--color-surface-text-muted)]">
								{shelf.book_count} books • {shelf.is_magic === 1 ? 'Smart rules' : 'Manual'}
							</p>
						</div>
						<svg class="w-5 h-5 text-[var(--color-surface-500)] group-hover:text-[var(--color-primary-400)] transition-colors" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
						</svg>
					</div>
				</a>
			{/each}
		</div>
	{/if}
</div>

<script lang="ts">
	import { onMount } from 'svelte';
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import { getCoverThumbUrl } from '$lib/utils/covers';
	
	let history = $state<any[]>([]);
	let loading = $state(true);
	
	onMount(async () => {
		await fetchHistory();
	});
	
	async function fetchHistory() {
		try {
			const res = await fetch('/api/history');
			history = await res.json();
		} catch (e) {
			console.error('Failed to fetch history:', e);
		} finally {
			loading = false;
		}
	}

	async function deleteHistoryItem(item: any) {
		if (!confirm('Delete this reading session?')) return;
		try {
			const res = await fetch(`/api/books/${item.book_id}/sessions/${item.session_id}`, {
				method: 'DELETE'
			});
			if (res.ok) {
				history = history.filter(entry => entry.session_id !== item.session_id);
			}
		} catch (e) {
			console.error('Failed to delete history item:', e);
		}
	}
	
	function formatDate(timestamp: number) {
		const date = new Date(timestamp * 1000);
		const now = new Date();
		const diff = now.getTime() - date.getTime();
		const days = Math.floor(diff / (1000 * 60 * 60 * 24));
		
		if (days === 0) return 'Today';
		if (days === 1) return 'Yesterday';
		if (days < 7) return `${days} days ago`;
		return date.toLocaleDateString();
	}
	
	function formatTime(timestamp: number) {
		return new Date(timestamp * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}
	
	function formatSessionDuration(item: any): string {
		if (!item.activity_tracked) return 'Legacy (untracked)';
		const seconds = Math.max(0, Number(item.active_seconds) || 0);
		if (seconds < 60) return `${seconds}s`;
		const minutes = Math.floor(seconds / 60);
		if (minutes < 60) return `${minutes}m`;
		const hours = Math.floor(minutes / 60);
		const remainingMinutes = minutes % 60;
		return `${hours}h ${remainingMinutes}m`;
	}
	
	function groupByDate(history: any[]) {
		const groups: Record<string, any[]> = {};
		
		for (const item of history) {
			const key = formatDate(item.started_at);
			if (!groups[key]) {
				groups[key] = [];
			}
			groups[key].push(item);
		}
		
		return groups;
	}
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold text-[var(--color-surface-text)]">Reading History</h1>
		<p class="text-[var(--color-surface-text-muted)] mt-1">Your recent reading activity</p>
	</div>

	{#if loading}
		<div class="flex justify-center py-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if history.length === 0}
		<div class="text-center py-16 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)]">
			<svg class="w-16 h-16 text-[var(--color-primary-400)] mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
			</svg>
			<h3 class="text-lg font-medium text-[var(--color-surface-text)] mb-2">No reading history</h3>
			<p class="text-[var(--color-surface-text-muted)]">Start reading to see your history here.</p>
		</div>
	{:else}
		<div class="space-y-8">
			{#each Object.entries(groupByDate(history)) as [date, items]}
				<div>
					<h2 class="text-lg font-semibold text-[var(--color-surface-text)] mb-4">{date}</h2>
					<div class="space-y-3">
						{#each items as item}
							<div class="block bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] p-4 hover:border-[var(--color-surface-border)] transition-colors">
								<div class="flex items-start space-x-4">
									<BookCoverFrame
										src={item.cover_path ? getCoverThumbUrl(item.book_id, 'small') : null}
										alt={item.title}
										placeholderKind={item.reader_type === 'audio' ? 'audio' : 'book'}
										frameClass="h-24 w-16 flex-shrink-0 rounded"
										placeholderSize="sm"
									/>
									<div class="flex-1 min-w-0">
										<div class="flex items-start justify-between">
											<div>
												<a href="/book/{item.book_id}" class="font-medium text-[var(--color-surface-text)] truncate hover:text-[var(--color-primary-400)] transition-colors">{item.title || 'Untitled'}</a>
										<div class="flex flex-wrap items-center gap-2 mt-1">
											<span class="text-xs text-[var(--color-surface-text-muted)]">
												{formatTime(item.started_at)} → {item.ended_at ? formatTime(item.ended_at) : 'In progress'}
											</span>
											<span class="text-[var(--color-surface-600)]">·</span>
											<span class="text-xs text-[var(--color-primary-400)]">
														{formatSessionDuration(item)}{item.activity_tracked ? ' active' : ''}
											</span>
											<span class="passive-status-indicator">
												{item.reader_type === 'speed' ? 'Speed Reader' : item.reader_type === 'epub' || item.reader_type === 'normal' ? 'Normal Reader' : item.reader_type}
											</span>
										</div>
											</div>
											<button
												onclick={() => deleteHistoryItem(item)}
												class="p-2 rounded-lg bg-red-500/10 text-red-400 hover:bg-red-500/20 transition-colors flex-shrink-0"
												title="Delete history item"
											>
												<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
													<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
												</svg>
											</button>
										</div>
										{#if item.percent > 0}
											<div class="mt-3">
												<div class="w-full bg-[var(--color-surface-base)] rounded-full h-1.5">
													<div class="bg-[var(--color-primary-500)] h-1.5 rounded-full" style="width: {item.percent}%"></div>
												</div>
												<span class="text-xs text-[var(--color-surface-text-muted)]">{Math.round(item.percent)}% complete</span>
											</div>
										{/if}
									</div>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	type NotificationItem = {
		id: number;
		kind: string;
		title: string;
		message?: string;
		url?: string;
		read_at?: number;
		created_at: number;
	};

	let open = $state(false);
	let unreadCount = $state(0);
	let notifications = $state<NotificationItem[]>([]);
	let buttonRef: HTMLButtonElement | null = null;
	let panelRef = $state<HTMLDivElement | null>(null);
	let refreshTimer: number | null = null;
	let {
		mobileMenu = false
	} = $props<{
		mobileMenu?: boolean;
	}>();

	function formatTime(value: number) {
		return new Intl.DateTimeFormat(undefined, {
			month: 'short',
			day: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		}).format(new Date(value * 1000));
	}

	async function loadNotifications() {
		try {
			const res = await fetch('/api/notifications?limit=8');
			if (res.ok) {
				const data = await res.json();
				notifications = data.items ?? [];
				unreadCount = data.unread_count ?? 0;
			}
		} catch (error) {
			console.error('Failed to load notifications:', error);
		}
	}

	async function markNotificationRead(notificationId: number) {
		await fetch(`/api/notifications/${notificationId}/read`, { method: 'POST' });
		await loadNotifications();
	}

	async function removeNotification(notificationId: number) {
		await fetch(`/api/notifications/${notificationId}`, { method: 'DELETE' });
		await loadNotifications();
	}

	async function dismissAllNotifications() {
		await fetch('/api/notifications', { method: 'DELETE' });
		await loadNotifications();
	}

	function handleOpen(item: NotificationItem) {
		if (!item.read_at) {
			markNotificationRead(item.id);
		}
		open = false;
		if (item.url) {
			goto(item.url);
		}
	}

	function handleDocumentClick(event: MouseEvent) {
		const target = event.target as Node;
		if (!open) return;
		if (buttonRef?.contains(target) || panelRef?.contains(target)) return;
		open = false;
	}

	onMount(() => {
		loadNotifications();
		refreshTimer = window.setInterval(loadNotifications, 30000);
		document.addEventListener('click', handleDocumentClick);
		return () => {
			if (refreshTimer) window.clearInterval(refreshTimer);
			document.removeEventListener('click', handleDocumentClick);
		};
	});
</script>

<div class="relative">
	<button
		bind:this={buttonRef}
		onclick={() => open = !open}
		class={`relative rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] hover:bg-[var(--color-surface-overlay)] transition-colors ${mobileMenu ? 'flex w-full items-center justify-between gap-3 px-0 py-0' : 'p-2'}`}
		title="Notifications"
		aria-label="Notifications"
	>
		{#if mobileMenu}
			<span class="flex items-center gap-3">
				<svg class="h-5 w-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C8.67 6.165 7 8.388 7 11v3.159c0 .538-.214 1.055-.595 1.436L5 17h5m5 0a3 3 0 11-6 0m6 0H9"></path>
				</svg>
				<span class="text-sm font-medium text-[var(--color-surface-text)]">Notifications</span>
			</span>
			{#if unreadCount > 0}
				<span class="min-w-5 h-5 px-1 rounded-full bg-[var(--color-primary-500)] text-white text-[10px] font-semibold flex items-center justify-center">{unreadCount}</span>
			{/if}
		{:else}
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C8.67 6.165 7 8.388 7 11v3.159c0 .538-.214 1.055-.595 1.436L5 17h5m5 0a3 3 0 11-6 0m6 0H9"></path>
			</svg>
			{#if unreadCount > 0}
				<span class="absolute -top-0.5 -right-0.5 min-w-5 h-5 px-1 rounded-full bg-[var(--color-primary-500)] text-white text-[10px] font-semibold flex items-center justify-center">{unreadCount}</span>
			{/if}
		{/if}
	</button>

	{#if open}
		<div
			bind:this={panelRef}
			class="absolute right-0 mt-3 w-80 rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl backdrop-blur-sm overflow-hidden z-[80]"
		>
			<div class="px-4 py-3 border-b border-[var(--color-surface-border)] flex items-center justify-between">
				<div>
					<div class="text-sm font-semibold text-[var(--color-surface-text)]">Notifications</div>
					<div class="text-xs text-[var(--color-surface-text-muted)]">{unreadCount} unread</div>
				</div>
				<div class="flex items-center gap-3">
					{#if notifications.length > 0}
						<button onclick={dismissAllNotifications} class="text-xs text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]">
							Dismiss all
						</button>
					{/if}
					<a href="/settings?tab=admin" class="text-xs text-[var(--color-primary-400)] hover:text-[var(--color-primary-300)]">Admin</a>
				</div>
			</div>
			<div class="max-h-96 overflow-auto">
				{#if notifications.length === 0}
					<div class="px-4 py-6 text-sm text-[var(--color-surface-text-muted)]">No notifications.</div>
				{:else}
					{#each notifications as item}
						<div class="border-b border-[var(--color-surface-border)] last:border-b-0 px-4 py-3 hover:bg-[var(--color-surface-base)] transition-colors">
							<button class="w-full text-left" onclick={() => handleOpen(item)}>
								<div class="flex items-center justify-between gap-2">
									<span class="text-xs uppercase tracking-wide text-[var(--color-surface-text-muted)]">{item.kind}</span>
									{#if !item.read_at}
										<span class="rounded-full bg-[var(--color-primary-500)]/15 px-2 py-0.5 text-[10px] font-semibold text-[var(--color-primary-300)]">New</span>
									{/if}
								</div>
								<div class="mt-1 text-sm font-medium text-[var(--color-surface-text)]">{item.title}</div>
								{#if item.message}
									<div class="mt-1 text-xs text-[var(--color-surface-text-muted)] line-clamp-2">{item.message}</div>
								{/if}
								<div class="mt-2 text-[11px] text-[var(--color-surface-text-muted)]">{formatTime(item.created_at)}</div>
							</button>
							<div class="mt-2 flex justify-end gap-2">
								{#if !item.read_at}
									<button onclick={() => markNotificationRead(item.id)} class="text-xs text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]">Read</button>
								{/if}
								<button onclick={() => removeNotification(item.id)} class="text-xs text-red-300 hover:text-red-200">Dismiss</button>
							</div>
						</div>
					{/each}
				{/if}
			</div>
		</div>
	{/if}
</div>

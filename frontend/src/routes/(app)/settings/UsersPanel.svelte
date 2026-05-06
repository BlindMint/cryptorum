<script lang="ts">
	import { onMount } from 'svelte';

	type AppUser = {
		id: number;
		username: string;
		is_admin: boolean;
		is_bootstrap_admin: boolean;
		permissions: string[];
		created_at: number;
		updated_at: number;
	};

	const allPermissions = [
		'manage_users',
		'manage_libraries',
		'manage_metadata',
		'view_admin',
		'view_logs',
		'manage_jobs',
		'download_books',
		'restore_backups',
		'create_backups'
	];

	let users = $state<AppUser[]>([]);
	let loading = $state(false);
	let error = $state('');
	let showForm = $state(false);
	let editingUser = $state<AppUser | null>(null);
	let username = $state('');
	let password = $state('');
	let isAdmin = $state(false);
	let permissions = $state<string[]>([]);

	function resetForm(user: AppUser | null = null) {
		editingUser = user;
		username = user?.username ?? '';
		password = '';
		isAdmin = user?.is_admin ?? false;
		permissions = user?.permissions?.length ? [...user.permissions] : [];
		showForm = true;
	}

	function togglePermission(permission: string) {
		if (permissions.includes(permission)) {
			permissions = permissions.filter((item) => item !== permission);
		} else {
			permissions = [...permissions, permission];
		}
	}

	async function loadUsers() {
		loading = true;
		error = '';
		try {
			const res = await fetch('/api/users');
			if (!res.ok) {
				throw new Error('Unable to load users');
			}
			users = await res.json();
		} catch (err) {
			console.error(err);
			error = 'Unable to load users.';
		} finally {
			loading = false;
		}
	}

	async function saveUser() {
		const payload: Record<string, unknown> = {
			username: username.trim(),
			is_admin: isAdmin,
			permissions
		};
		if (password.trim()) {
			payload.password = password.trim();
		}

		const endpoint = editingUser ? `/api/users/${editingUser.id}` : '/api/users';
		const method = editingUser ? 'PUT' : 'POST';

		try {
			const res = await fetch(endpoint, {
				method,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(payload)
			});
			if (!res.ok) {
				throw new Error('Failed to save user');
			}
			showForm = false;
			editingUser = null;
			username = '';
			password = '';
			isAdmin = false;
			permissions = [];
			await loadUsers();
		} catch (err) {
			console.error(err);
			error = 'Unable to save user.';
		}
	}

	async function removeUser(user: AppUser) {
		if (user.is_bootstrap_admin) return;
		if (!confirm(`Delete user "${user.username}"?`)) return;
		try {
			const res = await fetch(`/api/users/${user.id}`, { method: 'DELETE' });
			if (!res.ok) {
				throw new Error('Failed to delete user');
			}
			await loadUsers();
		} catch (err) {
			console.error(err);
			error = 'Unable to delete user.';
		}
	}

	function cancelForm() {
		showForm = false;
		editingUser = null;
		username = '';
		password = '';
		isAdmin = false;
		permissions = [];
	}

	function formatTime(value: number) {
		return new Intl.DateTimeFormat(undefined, {
			month: 'short',
			day: 'numeric',
			year: 'numeric'
		}).format(new Date(value * 1000));
	}

	onMount(loadUsers);
</script>

<div class="space-y-6">
	<div class="flex flex-wrap items-end justify-between gap-3">
		<div>
			<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Users</h2>
			<p class="text-sm text-[var(--color-surface-text-muted)]">Manage accounts and coarse permissions</p>
		</div>
		<div class="flex items-center gap-2">
			<button
				onclick={loadUsers}
				class="px-4 py-2 rounded-lg border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-base)] transition-colors"
			>
				Refresh
			</button>
			<button
				onclick={() => resetForm(null)}
				class="px-4 py-2 rounded-lg bg-[var(--color-primary-500)] text-white hover:opacity-90 transition-opacity"
			>
				Add User
			</button>
		</div>
	</div>

	{#if showForm}
		<div class="rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-6 space-y-4">
			<div class="flex items-center justify-between gap-3">
				<div>
					<h3 class="text-base font-semibold text-[var(--color-surface-text)]">{editingUser ? 'Edit User' : 'New User'}</h3>
					<p class="text-sm text-[var(--color-surface-text-muted)]">Bootstrap admin cannot be removed or demoted.</p>
				</div>
				<button
					onclick={cancelForm}
					class="text-sm text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]"
				>
					Close
				</button>
			</div>

			<div class="grid gap-4 md:grid-cols-2">
				<label class="space-y-2">
					<span class="text-sm font-medium text-[var(--color-surface-text)]">Username</span>
					<input
						bind:value={username}
						disabled={editingUser?.is_bootstrap_admin}
						class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)] disabled:opacity-60"
						placeholder="username"
					>
				</label>
				<label class="space-y-2">
					<span class="text-sm font-medium text-[var(--color-surface-text)]">Password {editingUser ? '(optional)' : ''}</span>
					<input
						bind:value={password}
						type="password"
						class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2 text-[var(--color-surface-text)]"
						placeholder="password"
					>
				</label>
			</div>

			<div class="flex flex-wrap items-center gap-4">
				<label class="flex items-center gap-2 text-sm text-[var(--color-surface-text)]">
					<input bind:checked={isAdmin} type="checkbox" disabled={editingUser?.is_bootstrap_admin} class="rounded border-[var(--color-surface-border)]">
					Admin
				</label>
				<span class="text-xs text-[var(--color-surface-text-muted)]">Selecting admin grants the default full permission set.</span>
			</div>

			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each allPermissions as permission}
					<label class="flex items-start gap-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2">
						<input
							type="checkbox"
							checked={permissions.includes(permission)}
							disabled={isAdmin || editingUser?.is_bootstrap_admin}
							onchange={() => togglePermission(permission)}
							class="mt-1 rounded border-[var(--color-surface-border)]"
						>
						<div>
							<div class="text-sm text-[var(--color-surface-text)] font-medium">{permission.replaceAll('_', ' ')}</div>
						</div>
					</label>
				{/each}
			</div>

			<div class="flex items-center gap-3">
				<button
					onclick={saveUser}
					class="px-4 py-2 rounded-lg bg-[var(--color-primary-500)] text-white hover:opacity-90 transition-opacity"
				>
					{editingUser ? 'Save User' : 'Create User'}
				</button>
				<button
					onclick={cancelForm}
					class="px-4 py-2 rounded-lg border border-[var(--color-surface-border)] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] transition-colors"
				>
					Cancel
				</button>
			</div>
		</div>
	{/if}

	{#if loading}
		<div class="rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-6 text-sm text-[var(--color-surface-text-muted)]">
			Loading users...
		</div>
	{:else if error}
		<div class="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200">{error}</div>
	{:else}
		<div class="grid gap-4">
			{#each users as user}
				<div class="rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-4 flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
					<div class="space-y-2">
						<div class="flex items-center gap-2 flex-wrap">
							<h3 class="text-base font-semibold text-[var(--color-surface-text)]">{user.username}</h3>
							{#if user.is_bootstrap_admin}
								<span class="text-xs rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-amber-300">Bootstrap admin</span>
							{/if}
							{#if user.is_admin}
								<span class="text-xs rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 text-emerald-300">Admin</span>
							{/if}
						</div>
						<div class="text-xs text-[var(--color-surface-text-muted)]">Created {formatTime(user.created_at)}</div>
						<div class="flex flex-wrap gap-2">
							{#each user.permissions as permission}
								<span class="text-xs rounded-full border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-2 py-0.5 text-[var(--color-surface-text-muted)]">
									{permission.replaceAll('_', ' ')}
								</span>
							{/each}
						</div>
					</div>
					<div class="flex items-center gap-2">
						<button
							onclick={() => resetForm(user)}
							class="px-3 py-2 rounded-lg border border-[var(--color-surface-border)] text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] transition-colors"
						>
							Edit
						</button>
						<button
							onclick={() => removeUser(user)}
							disabled={user.is_bootstrap_admin}
							class="px-3 py-2 rounded-lg border border-red-500/30 bg-red-500/10 text-sm text-red-200 hover:bg-red-500/20 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
						>
							Delete
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

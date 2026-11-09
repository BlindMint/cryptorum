<script lang="ts">
	let { job, onchanged }: { job: { id: number; job_type: string; status: string }; onchanged: () => void | Promise<void> } = $props();
	let pending = $state(false);
	let error = $state('');

	async function control(action: 'pause' | 'resume') {
		pending = true;
		error = '';
		try {
			const response = await fetch(`/api/jobs/${job.id}/${action}`, { method: 'POST' });
			if (!response.ok) {
				const body = await response.json();
				throw new Error(body.error || 'Unable to change scan state.');
			}
			await onchanged();
		} catch (failure) {
			error = failure instanceof Error ? failure.message : 'Unable to reach the server.';
		} finally {
			pending = false;
		}
	}
</script>

{#if job.job_type === 'library_scan'}
	{#if job.status === 'queued' || job.status === 'running'}
		<button type="button" disabled={pending} onclick={() => control('pause')} class="rounded-lg border border-[var(--color-surface-border)] px-3 py-2 text-xs text-[var(--color-surface-text)] focus-visible:outline-2 disabled:opacity-50">{pending ? 'Requesting…' : 'Pause'}</button>
	{:else if ['paused', 'failed', 'cancelled'].includes(job.status)}
		<button type="button" disabled={pending} onclick={() => control('resume')} class="accent-action rounded-lg px-3 py-2 text-xs focus-visible:outline-2">{pending ? 'Requesting…' : 'Resume'}</button>
	{:else if job.status === 'pausing'}
		<span class="px-3 py-2 text-xs text-[var(--color-surface-text-muted)]">Finishing current file…</span>
	{/if}
	{#if error}<p role="alert" class="w-full text-xs text-[var(--color-surface-text)]">{error}</p>{/if}
{/if}

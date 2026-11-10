export function sidebarDropPosition(clientY: number, top: number, height: number): 'before' | 'after' {
	return clientY < top + height / 2 ? 'before' : 'after';
}

// Refresh metadata without replacing an optimistic order with an older response.
export function reconcileSidebarItems<T extends { id: number }>(current: T[], incoming: T[]): T[] {
	const byId = new Map(incoming.map(item => [item.id, item]));
	const existing = current.flatMap(item => {
		const refreshed = byId.get(item.id);
		byId.delete(item.id);
		return refreshed ? [refreshed] : [];
	});
	return [...existing, ...byId.values()];
}

export function createSidebarOrderQueue() {
	let tail = Promise.resolve();
	return (save: () => Promise<void>): Promise<void> => {
		const result = tail.then(save);
		tail = result.catch(() => {});
		return result;
	};
}

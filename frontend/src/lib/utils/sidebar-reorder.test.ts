import { describe, expect, it } from 'vitest';
import { createSidebarOrderQueue, reconcileSidebarItems, sidebarDropPosition } from './sidebar-reorder';
import { reorderSidebarEntry, withSidebarDividers } from './sidebar-dividers';

describe('sidebar pointer reorder', () => {
	it('uses final pointer coordinates on either side of the target midpoint', () => {
		expect(sidebarDropPosition(104, 100, 32)).toBe('before');
		expect(sidebarDropPosition(128, 100, 32)).toBe('after');
	});

	it('moves an item across a divider even when the item order stays the same', () => {
		const items = [{ id: 1 }, { id: 2 }];
		const dividers = [{ id: 'd', label: 'Group', beforeId: 2 }];
		const next = reorderSidebarEntry(items, dividers, 'item:1', 'divider:d', 'after');
		expect(next.items).toEqual(items);
		expect(withSidebarDividers(next.items, next.dividers).map(row => row.type === 'item' ? row.item.id : row.divider.id)).toEqual(['d', 1, 2]);
	});

	it('moves distant items and keeps divider anchors attached to the resulting layout', () => {
		const next = reorderSidebarEntry([{ id: 1 }, { id: 2 }, { id: 3 }], [{ id: 'd', label: '', beforeId: 2 }], 'item:3', 'item:1', 'before');
		expect(next.items.map(item => item.id)).toEqual([3, 1, 2]);
		expect(next.dividers[0].beforeId).toBe(2);
	});
});

describe('sidebar order persistence', () => {
	it('serializes rapid writes so the newest order is saved last', async () => {
		const enqueue = createSidebarOrderQueue();
		const calls: number[] = [];
		let release!: () => void;
		const pending = new Promise<void>(resolve => release = resolve);
		const first = enqueue(async () => { calls.push(1); await pending; });
		const second = enqueue(async () => { calls.push(2); });
		await Promise.resolve();
		expect(calls).toEqual([1]);
		release();
		await Promise.all([first, second]);
		expect(calls).toEqual([1, 2]);
	});

	it('allows subsequent saves after a failed request', async () => {
		const enqueue = createSidebarOrderQueue();
		const first = enqueue(async () => { throw new Error('offline'); });
		let saved = false;
		const second = enqueue(async () => { saved = true; });
		await expect(first).rejects.toThrow('offline');
		await second;
		expect(saved).toBe(true);
	});

	it('refreshes metadata without replacing the local order with stale server ordering', () => {
		const local = [{ id: 2, name: 'old' }, { id: 1, name: 'first' }, { id: 9, name: 'removed' }];
		const incoming = [{ id: 1, name: 'first' }, { id: 2, name: 'updated' }, { id: 3, name: 'new' }];
		expect(reconcileSidebarItems(local, incoming)).toEqual([{ id: 2, name: 'updated' }, { id: 1, name: 'first' }, { id: 3, name: 'new' }]);
	});
});

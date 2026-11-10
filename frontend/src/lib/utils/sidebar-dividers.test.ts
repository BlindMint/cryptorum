import { describe, expect, it } from 'vitest';
import { parseSidebarDividers, withSidebarDividers, moveSidebarDivider, dropSidebarDivider, type SidebarDivider } from './sidebar-dividers';

const items = [{ id: 1 }, { id: 2 }, { id: 3 }];
const divider: SidebarDivider = { id: 'd1', label: 'Reference', beforeId: null };
const names = (dividers: SidebarDivider[]) => withSidebarDividers(items, dividers).map(row => row.type === 'item' ? row.item.id : row.divider.id);

describe('sidebar dividers', () => {
	it('moves a divider through items without reordering or removing them', () => {
		let dividers = [divider];
		expect(names(dividers)).toEqual([1, 2, 3, 'd1']);
		dividers = moveSidebarDivider(items, dividers, 'd1', -1);
		expect(names(dividers)).toEqual([1, 2, 'd1', 3]);
		dividers = moveSidebarDivider(items, dividers, 'd1', -1);
		expect(names(dividers)).toEqual([1, 'd1', 2, 3]);
		dividers = moveSidebarDivider(items, dividers, 'd1', 1);
		expect(names(dividers)).toEqual([1, 2, 'd1', 3]);
		expect(names([])).toEqual([1, 2, 3]);
	});
	it('moves adjacent dividers and handles boundaries', () => {
		const second = { ...divider, id: 'd2', label: '' };
		const moved = moveSidebarDivider(items, [divider, second], 'd2', -1);
		expect(names(moved)).toEqual([1, 2, 3, 'd2', 'd1']);
		expect(names(moveSidebarDivider(items, moved, 'd1', 1))).toEqual(names(moved));
		const first = [{ ...divider, beforeId: 1 }];
		expect(names(moveSidebarDivider(items, first, 'd1', -1))).toEqual(names(first));
	});
	it('keeps anchors across item reordering and retains dividers after anchor deletion', () => {
		const anchored = [{ ...divider, beforeId: 2 }];
		const reordered = withSidebarDividers([{ id: 3 }, { id: 2 }, { id: 1 }], anchored);
		expect(reordered.map(row => row.type === 'item' ? row.item.id : row.divider.id)).toEqual([3, 'd1', 2, 1]);
		const missing = withSidebarDividers([{ id: 1 }], anchored);
		expect(missing.at(-1)?.type).toBe('divider');
	});
	it('restores section preferences and tolerates malformed browser storage', () => {
		const raw = JSON.stringify({ libraries: [divider], audio: [{ ...divider, label: 'Podcasts' }], collections: [] });
		expect(parseSidebarDividers(raw).audio[0].label).toBe('Podcasts');
		expect(parseSidebarDividers(raw).libraries[0].label).toBe('Reference');
		expect(parseSidebarDividers('broken')).toEqual({ libraries: [], audio: [], collections: [] });
		const invalid = { libraries: [divider, divider, null, { id: 'bad', label: 'Bad', beforeId: '2' }] };
		expect(parseSidebarDividers(JSON.stringify(invalid)).libraries).toEqual([divider]);
	});
});


describe('dragging sidebar dividers', () => {
	it('drops before and after distant items while retaining item order', () => {
		const before = dropSidebarDivider(items, [divider], 'd1', 'item:1', 'before');
		expect(names(before)).toEqual(['d1', 1, 2, 3]);
		const after = dropSidebarDivider(items, before, 'd1', 'item:2', 'after');
		expect(names(after)).toEqual([1, 2, 'd1', 3]);
		expect(names(dropSidebarDivider(items, after, 'd1', 'item:3', 'after'))).toEqual([1, 2, 3, 'd1']);
	});
	it('drops beside other dividers in the same gap', () => {
		const second = { ...divider, id: 'd2', label: '' };
		const swapped = dropSidebarDivider(items, [divider, second], 'd2', 'divider:d1', 'before');
		expect(names(swapped)).toEqual([1, 2, 3, 'd2', 'd1']);
		expect(names(dropSidebarDivider(items, swapped, 'd2', 'divider:d1', 'after'))).toEqual([1, 2, 3, 'd1', 'd2']);
	});
	it('ignores self drops and unknown sources or targets', () => {
		for (const [source, target] of [['d1', 'divider:d1'], ['missing', 'item:1'], ['d1', 'item:99']]) {
			expect(dropSidebarDivider(items, [divider], source, target, 'before')).toEqual([divider]);
		}
	});
});

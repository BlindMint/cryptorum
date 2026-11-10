export type SidebarSection = 'libraries' | 'audio' | 'collections';
export interface SidebarDivider {
	id: string;
	label: string;
	beforeId: number | null;
}
export type SidebarDividers = Record<SidebarSection, SidebarDivider[]>;
export type SidebarEntry<T> = { type: 'item'; item: T } | { type: 'divider'; divider: SidebarDivider };

export function parseSidebarDividers(raw: string | null): SidebarDividers {
	const result: SidebarDividers = { libraries: [], audio: [], collections: [] };
	try {
		const value = JSON.parse(raw ?? '{}');
		for (const section of Object.keys(result) as SidebarSection[]) {
			if (!Array.isArray(value?.[section])) continue;
			const ids = new Set<string>();
			result[section] = value[section].filter((entry: SidebarDivider) => {
				if (!entry || typeof entry.id !== 'string' || ids.has(entry.id) || typeof entry.label !== 'string'
					|| !(entry.beforeId === null || Number.isSafeInteger(entry.beforeId))) return false;
				ids.add(entry.id);
				return true;
			});
		}
	} catch { /* Ignore malformed browser preferences. */ }
	return result;
}

export function withSidebarDividers<T extends { id: number }>(items: T[], dividers: SidebarDivider[]): SidebarEntry<T>[] {
	const ids = new Set(items.map(item => item.id));
	const rows: SidebarEntry<T>[] = [];
	for (const item of items) {
		for (const divider of dividers) if (divider.beforeId === item.id) rows.push({ type: 'divider', divider });
		rows.push({ type: 'item', item });
	}
	for (const divider of dividers) {
		if (divider.beforeId === null || !ids.has(divider.beforeId)) rows.push({ type: 'divider', divider });
	}
	return rows;
}

export function moveSidebarDivider<T extends { id: number }>(items: T[], dividers: SidebarDivider[], id: string, direction: -1 | 1): SidebarDivider[] {
	const rows = withSidebarDividers(items, dividers);
	const from = rows.findIndex(row => row.type === 'divider' && row.divider.id === id);
	const to = from + direction;
	if (from < 0 || to < 0 || to >= rows.length) return dividers;
	[rows[from], rows[to]] = [rows[to], rows[from]];
	return rows.flatMap((row, index) => {
		if (row.type !== 'divider') return [];
		const nextItem = rows.slice(index + 1).find(next => next.type === 'item');
		return [{ ...row.divider, beforeId: nextItem?.type === 'item' ? nextItem.item.id : null }];
	});
}

export function sidebarEntryKey<T extends { id: number }>(row: SidebarEntry<T>): string {
	return row.type === 'divider' ? 'divider:' + row.divider.id : 'item:' + row.item.id;
}

export function dropSidebarDivider<T extends { id: number }>(items: T[], dividers: SidebarDivider[], id: string, targetKey: string, position: 'before' | 'after'): SidebarDivider[] {
	const rows = withSidebarDividers(items, dividers);
	const source = rows.findIndex(row => row.type === 'divider' && row.divider.id === id);
	if (source < 0 || targetKey === 'divider:' + id || !rows.some(row => sidebarEntryKey(row) === targetKey)) return dividers;
	const [moved] = rows.splice(source, 1);
	const target = rows.findIndex(row => sidebarEntryKey(row) === targetKey);
	rows.splice(target + (position === 'after' ? 1 : 0), 0, moved);
	return rows.flatMap((row, index) => {
		if (row.type !== 'divider') return [];
		const next = rows.slice(index + 1).find(item => item.type === 'item');
		return [{ ...row.divider, beforeId: next?.type === 'item' ? next.item.id : null }];
	});
}

export function reorderSidebarEntry<T extends { id: number }>(items: T[], dividers: SidebarDivider[], sourceKey: string, targetKey: string, position: 'before' | 'after'): { items: T[]; dividers: SidebarDivider[] } {
	const rows = withSidebarDividers(items, dividers);
	const source = rows.findIndex(row => sidebarEntryKey(row) === sourceKey);
	if (source < 0 || sourceKey === targetKey || !rows.some(row => sidebarEntryKey(row) === targetKey)) return { items, dividers };
	const [moved] = rows.splice(source, 1);
	const target = rows.findIndex(row => sidebarEntryKey(row) === targetKey);
	rows.splice(target + (position === 'after' ? 1 : 0), 0, moved);
	return {
		items: rows.flatMap(row => row.type === 'item' ? [row.item] : []),
		dividers: rows.flatMap((row, index) => {
			if (row.type !== 'divider') return [];
			const next = rows.slice(index + 1).find(item => item.type === 'item');
			return [{ ...row.divider, beforeId: next?.type === 'item' ? next.item.id : null }];
		})
	};
}

export function comicPageGroups(total: number, spread: 'off' | 'even' | 'odd' | 'auto'): number[][] {
	const groups: number[][] = [];
	let page = 1;
	if (spread === 'even' && total > 0) { groups.push([1]); page = 2; }
	const size = spread === 'even' || spread === 'odd' ? 2 : 1;
	for (; page <= total; page += size) groups.push(Array.from({ length: Math.min(size, total - page + 1) }, (_, index) => page + index));
	return groups;
}

export function comicGroupIndex(groups: number[][], page: number): number {
	return Math.max(0, groups.findIndex(group => group.includes(page)));
}

export function comicSnapIndex(offset: number, width: number, count: number): number {
	return width > 0 ? Math.max(0, Math.min(count - 1, Math.round(Math.abs(offset) / width))) : 0;
}

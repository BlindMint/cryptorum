export const LARGE_BULK_ACTION_THRESHOLD = 250;

interface BulkActionConfirmationOptions {
	action: string;
	count: number;
	destructive?: boolean;
	alwaysConfirm?: boolean;
	threshold?: number;
}

export function confirmBulkAction({
	action,
	count,
	destructive = false,
	alwaysConfirm = false,
	threshold = LARGE_BULK_ACTION_THRESHOLD
}: BulkActionConfirmationOptions): boolean {
	if (count <= 0) return false;
	if (!alwaysConfirm && count < threshold) return true;

	const actionText = action.includes('{count}')
		? action.replace('{count}', String(count))
		: `${action} ${count} books`;
	const warning = count >= threshold
		? `This will ${actionText}.`
		: `${actionText.charAt(0).toUpperCase()}${actionText.slice(1)}?`;
	const consequence = destructive ? '\n\nThis cannot be undone.' : '';
	return confirm(`${warning}${consequence}\n\nContinue?`);
}

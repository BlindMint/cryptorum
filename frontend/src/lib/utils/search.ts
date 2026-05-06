const STOP_WORDS = new Set([
	'a',
	'an',
	'and',
	'are',
	'as',
	'at',
	'by',
	'for',
	'from',
	'in',
	'into',
	'is',
	'of',
	'on',
	'or',
	'the',
	'to',
	'with'
]);

export function lenientSearchMatch(query: string, candidate: string): boolean {
	const queryTokens = searchTokens(query);
	if (queryTokens.length === 0) return true;

	const candidateTokens = searchTokens(candidate);
	if (candidateTokens.length === 0) return false;

	const matched = queryTokens.filter((queryToken) =>
		candidateTokens.some((candidateToken) => tokenMatchScore(queryToken, candidateToken) >= 0.62)
	).length;

	return matched / queryTokens.length >= requiredCoverage(queryTokens.length);
}

function searchTokens(value: string): string[] {
	const normalized = value
		.toLowerCase()
		.replace(/[^\p{L}\p{N}]+/gu, ' ')
		.trim();
	if (!normalized) return [];

	const seen = new Set<string>();
	const tokens: string[] = [];
	for (const token of normalized.split(/\s+/)) {
		if (token.length < 2 || STOP_WORDS.has(token) || seen.has(token)) continue;
		seen.add(token);
		tokens.push(token);
	}
	return tokens;
}

function tokenMatchScore(queryToken: string, candidateToken: string): number {
	if (queryToken === candidateToken) return 1;
	if (singularToken(queryToken) === singularToken(candidateToken)) return 0.96;
	if (candidateToken.startsWith(queryToken) || queryToken.startsWith(candidateToken)) return 0.9;
	if (
		queryToken.length >= 4 &&
		(candidateToken.includes(queryToken) || queryToken.includes(candidateToken))
	) {
		return 0.78;
	}

	const distance = damerauLevenshteinDistance(queryToken, candidateToken);
	const maxLength = Math.max(queryToken.length, candidateToken.length);
	if (maxLength <= 4 && distance === 1) return 0.7;
	if (maxLength <= 7 && distance <= 1) return 0.82;
	if (maxLength > 7 && distance <= 2) return 0.74;
	return 0;
}

function requiredCoverage(tokenCount: number): number {
	if (tokenCount <= 2) return 1;
	if (tokenCount === 3) return 0.67;
	return 0.6;
}

function singularToken(token: string): string {
	if (token.length <= 3) return token;
	if (token.endsWith('ies') && token.length > 4) return token.slice(0, -3) + 'y';
	if (token.endsWith('es') && token.length > 4) return token.slice(0, -2);
	if (token.endsWith('s') && !token.endsWith('ss')) return token.slice(0, -1);
	return token;
}

function damerauLevenshteinDistance(left: string, right: string): number {
	if (left.length === 0) return right.length;
	if (right.length === 0) return left.length;

	const dist = Array.from({ length: left.length + 1 }, () => Array(right.length + 1).fill(0));
	for (let i = 0; i <= left.length; i++) dist[i][0] = i;
	for (let j = 0; j <= right.length; j++) dist[0][j] = j;

	for (let i = 1; i <= left.length; i++) {
		for (let j = 1; j <= right.length; j++) {
			const cost = left[i - 1] === right[j - 1] ? 0 : 1;
			dist[i][j] = Math.min(
				dist[i - 1][j] + 1,
				dist[i][j - 1] + 1,
				dist[i - 1][j - 1] + cost
			);

			if (i > 1 && j > 1 && left[i - 1] === right[j - 2] && left[i - 2] === right[j - 1]) {
				dist[i][j] = Math.min(dist[i][j], dist[i - 2][j - 2] + 1);
			}
		}
	}
	return dist[left.length][right.length];
}

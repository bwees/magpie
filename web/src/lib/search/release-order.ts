export interface RankedRelease {
	release: string;
	/** Position among accepted releases when it was sent, from 1; 0 for rejected ones. */
	rank: number;
}

/**
 * The best-first order of accepted releases after new ones arrive. The server ranks each new batch
 * against everything so far, and earlier releases never change their order among themselves, so a new
 * release's rank is its position in the merged list.
 */
export function mergeRanked(order: string[], incoming: RankedRelease[]): string[] {
	const known = new Set(order);
	const fresh = incoming
		.filter((r) => r.rank > 0 && !known.has(r.release))
		.toSorted((a, b) => a.rank - b.rank);
	const earlier = [...order];
	const merged: string[] = [];
	for (const next of fresh) {
		while (merged.length < next.rank - 1 && earlier.length > 0) {
			merged.push(earlier.shift() as string);
		}
		merged.push(next.release);
	}
	return [...merged, ...earlier];
}

/** The items whose release isn't among known. */
export function unseen<T extends { release: string }>(
	known: { release: string }[],
	items: T[]
): T[] {
	const ids = new Set(known.map((k) => k.release));
	return items.filter((item) => !ids.has(item.release));
}

/** Releases sorted by their place in order, best first; releases not in it go last. */
export function bestFirst<T extends { release: string }>(order: string[], releases: T[]): T[] {
	const position = new Map(order.map((id, index) => [id, index]));
	const placeOf = (r: T) => position.get(r.release) ?? Number.POSITIVE_INFINITY;
	return releases.toSorted((a, b) => placeOf(a) - placeOf(b));
}

/** The order a snapshot's current ranks describe. */
export function rankedOrder(releases: RankedRelease[]): string[] {
	return releases
		.filter((r) => r.rank > 0)
		.toSorted((a, b) => a.rank - b.rank)
		.map((r) => r.release);
}

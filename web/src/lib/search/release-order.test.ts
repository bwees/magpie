import { describe, expect, it } from 'vitest';
import { mergeRanked, rankedOrder } from './release-order';

describe('mergeRanked', () => {
	it('places each new release at its rank and keeps earlier ones in order', () => {
		const order = ['a', 'b', 'c'];
		const merged = mergeRanked(order, [
			{ release: 'x', rank: 1 },
			{ release: 'y', rank: 4 },
			{ release: 'z', rank: 0 }
		]);
		expect(merged).toEqual(['x', 'a', 'b', 'y', 'c']);
	});

	it('ignores releases it already has, as a replay sends them again', () => {
		expect(mergeRanked(['a', 'b'], [{ release: 'a', rank: 2 }])).toEqual(['a', 'b']);
	});

	it('appends when ranks run past the known list', () => {
		expect(mergeRanked(['a'], [{ release: 'x', rank: 5 }])).toEqual(['a', 'x']);
	});
});

describe('rankedOrder', () => {
	it('orders accepted releases by rank and leaves out rejected ones', () => {
		const order = rankedOrder([
			{ release: 'b', rank: 2 },
			{ release: 'r', rank: 0 },
			{ release: 'a', rank: 1 }
		]);
		expect(order).toEqual(['a', 'b']);
	});
});

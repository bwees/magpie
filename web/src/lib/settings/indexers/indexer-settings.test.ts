import { describe, expect, it } from 'vitest';
import {
	formatResponseTime,
	formatSince,
	indexerDraftError,
	optionsDraftError,
	SeedingOverride,
	seedingGoalsApply,
	updateRequest,
	type IndexerDraft
} from './indexer-settings';

const VALID: IndexerDraft = {
	priority: 25,
	minSeeders: 1,
	seedingOverride: SeedingOverride.USE_GLOBAL,
	seedRatio: null,
	seedDays: null
};

describe('indexerDraftError', () => {
	it('accepts defaults and empty seeding goals', () => {
		expect(indexerDraftError(VALID)).toBeUndefined();
	});

	it.each([
		[{ priority: 0 }, 'Priority must be 1–50'],
		[{ priority: 51 }, 'Priority must be 1–50'],
		[{ priority: null }, 'Priority must be 1–50'],
		[{ minSeeders: 101 }, 'Minimum seeders must be 0–100'],
		[{ seedRatio: 100.5 }, 'Seed ratio must be 0–100'],
		[{ seedDays: 366 }, 'Seed days must be 0–365']
	])('rejects %o', (change, message) => {
		expect(indexerDraftError({ ...VALID, ...change })).toBe(message);
	});
});

describe('optionsDraftError', () => {
	it('follows 12-settings ranges', () => {
		const valid = { searchTimeoutSeconds: 60, resultCacheMinutes: 0, syncIntervalMinutes: 60 };
		expect(optionsDraftError(valid)).toBeUndefined();
		expect(optionsDraftError({ ...valid, searchTimeoutSeconds: 9 })).toBe(
			'Search timeout must be 10–120'
		);
		expect(optionsDraftError({ ...valid, resultCacheMinutes: 121 })).toBe(
			'Result cache must be 0–120'
		);
		expect(optionsDraftError({ ...valid, syncIntervalMinutes: 5 })).toBe(
			'Sync interval must be 15–1440'
		);
	});
});

describe('updateRequest', () => {
	it('leaves empty seeding goals out so the global goal applies', () => {
		expect(updateRequest(VALID)).toEqual({
			priority: 25,
			minSeeders: 1,
			seedingOverride: SeedingOverride.USE_GLOBAL,
			seedRatio: undefined,
			seedDays: undefined
		});
		expect(updateRequest({ ...VALID, seedRatio: 1.5, seedDays: 7 })).toMatchObject({
			seedRatio: 1.5,
			seedDays: 7
		});
	});
});

describe('formatting', () => {
	it('shows response times in ms below a second', () => {
		expect(formatResponseTime(0)).toBe('—');
		expect(formatResponseTime(420)).toBe('420 ms');
		expect(formatResponseTime(2210)).toBe('2.2 s');
	});

	it('shows how long ago an indexer was OK', () => {
		const now = new Date('2026-10-11T12:00:00Z');
		expect(formatSince(undefined, now)).toBe('—');
		expect(formatSince('2026-10-11T11:59:30Z', now)).toBe('just now');
		expect(formatSince('2026-10-11T11:55:00Z', now)).toMatch(/5 minutes ago/);
		expect(formatSince('2026-10-11T09:00:00Z', now)).toMatch(/3 hours ago/);
	});

	it("disables seeding goals for Don't seed", () => {
		expect(seedingGoalsApply(SeedingOverride.DONT_SEED)).toBe(false);
		expect(seedingGoalsApply(SeedingOverride.USE_GLOBAL)).toBe(true);
	});
});

import type {
	IndexerOptionsRequest,
	IndexerOptionsResponse,
	IndexerResponse,
	UpdateIndexerRequest
} from '$lib/sdk';

export enum IndexerStatus {
	OK = 'OK',
	FAILING = 'FAILING',
	GONE = 'GONE'
}

export enum IndexerPrivacy {
	PUBLIC = 'PUBLIC',
	SEMI_PRIVATE = 'SEMI_PRIVATE',
	PRIVATE = 'PRIVATE'
}

export enum SeedingOverride {
	USE_GLOBAL = 'USE_GLOBAL',
	SEED_UNTIL_GOALS = 'SEED_UNTIL_GOALS',
	DONT_SEED = 'DONT_SEED'
}

export const SEEDING_OVERRIDE_LABELS: Record<SeedingOverride, string> = {
	[SeedingOverride.USE_GLOBAL]: 'Use global',
	[SeedingOverride.SEED_UNTIL_GOALS]: 'Seed until goals',
	[SeedingOverride.DONT_SEED]: "Don't seed"
};

export const PRIVACY_LABELS: Record<IndexerPrivacy, string> = {
	[IndexerPrivacy.PUBLIC]: 'Public',
	[IndexerPrivacy.SEMI_PRIVATE]: 'Semi-private',
	[IndexerPrivacy.PRIVATE]: 'Private'
};

/** Ranges from 12-settings › Indexers; the server checks the same ones. */
export const LIMITS = {
	PRIORITY: { MIN: 1, MAX: 50 },
	MIN_SEEDERS: { MIN: 0, MAX: 100 },
	SEED_RATIO: { MIN: 0, MAX: 100, STEP: 0.1 },
	SEED_DAYS: { MIN: 0, MAX: 365 },
	SEARCH_TIMEOUT_SECONDS: { MIN: 10, MAX: 120 },
	RESULT_CACHE_MINUTES: { MIN: 0, MAX: 120 },
	SYNC_INTERVAL_MINUTES: { MIN: 15, MAX: 1440 }
} as const;

interface Range {
	MIN: number;
	MAX: number;
}

/** A number input's value: Svelte binds an empty number input to null. */
type NumberField = number | null;

export interface IndexerDraft {
	priority: NumberField;
	minSeeders: NumberField;
	seedingOverride: SeedingOverride;
	seedRatio: NumberField;
	seedDays: NumberField;
}

export interface OptionsDraft {
	searchTimeoutSeconds: NumberField;
	resultCacheMinutes: NumberField;
	syncIntervalMinutes: NumberField;
}

export function indexerDraft(indexer: IndexerResponse): IndexerDraft {
	return {
		priority: indexer.priority,
		minSeeders: indexer.minSeeders,
		seedingOverride: indexer.seedingOverride as SeedingOverride,
		seedRatio: indexer.seedRatio ?? null,
		seedDays: indexer.seedDays ?? null
	};
}

export function optionsDraft(options: IndexerOptionsResponse): OptionsDraft {
	return { ...options };
}

function outside(value: NumberField, range: Range, required: boolean): boolean {
	if (value === null || Number.isNaN(value)) {
		return required;
	}
	return value < range.MIN || value > range.MAX;
}

function rangeMessage(label: string, range: Range): string {
	return `${label} must be ${range.MIN}–${range.MAX}`;
}

/** The first problem with a draft, or undefined when it can be saved. */
export function indexerDraftError(draft: IndexerDraft): string | undefined {
	if (outside(draft.priority, LIMITS.PRIORITY, true)) {
		return rangeMessage('Priority', LIMITS.PRIORITY);
	}
	if (outside(draft.minSeeders, LIMITS.MIN_SEEDERS, true)) {
		return rangeMessage('Minimum seeders', LIMITS.MIN_SEEDERS);
	}
	if (outside(draft.seedRatio, LIMITS.SEED_RATIO, false)) {
		return rangeMessage('Seed ratio', LIMITS.SEED_RATIO);
	}
	if (outside(draft.seedDays, LIMITS.SEED_DAYS, false)) {
		return rangeMessage('Seed days', LIMITS.SEED_DAYS);
	}
	return undefined;
}

export function optionsDraftError(draft: OptionsDraft): string | undefined {
	if (outside(draft.searchTimeoutSeconds, LIMITS.SEARCH_TIMEOUT_SECONDS, true)) {
		return rangeMessage('Search timeout', LIMITS.SEARCH_TIMEOUT_SECONDS);
	}
	if (outside(draft.resultCacheMinutes, LIMITS.RESULT_CACHE_MINUTES, true)) {
		return rangeMessage('Result cache', LIMITS.RESULT_CACHE_MINUTES);
	}
	if (outside(draft.syncIntervalMinutes, LIMITS.SYNC_INTERVAL_MINUTES, true)) {
		return rangeMessage('Sync interval', LIMITS.SYNC_INTERVAL_MINUTES);
	}
	return undefined;
}

function optional(value: NumberField): number | undefined {
	return value === null || Number.isNaN(value) ? undefined : value;
}

/** Only call with a draft that has no error. */
export function updateRequest(draft: IndexerDraft): UpdateIndexerRequest {
	return {
		priority: draft.priority ?? LIMITS.PRIORITY.MIN,
		minSeeders: draft.minSeeders ?? LIMITS.MIN_SEEDERS.MIN,
		seedingOverride: draft.seedingOverride,
		seedRatio: optional(draft.seedRatio),
		seedDays: optional(draft.seedDays)
	};
}

/** Only call with a draft that has no error. */
export function optionsRequest(draft: OptionsDraft): IndexerOptionsRequest {
	return {
		searchTimeoutSeconds: draft.searchTimeoutSeconds ?? LIMITS.SEARCH_TIMEOUT_SECONDS.MIN,
		resultCacheMinutes: draft.resultCacheMinutes ?? LIMITS.RESULT_CACHE_MINUTES.MIN,
		syncIntervalMinutes: draft.syncIntervalMinutes ?? LIMITS.SYNC_INTERVAL_MINUTES.MIN
	};
}

/** Seeding goals only matter when this indexer's releases can seed. */
export function seedingGoalsApply(override: SeedingOverride): boolean {
	return override !== SeedingOverride.DONT_SEED;
}

const MS_PER_SECOND = 1000;

export function formatResponseTime(ms: number): string {
	if (ms <= 0) {
		return '—';
	}
	if (ms < MS_PER_SECOND) {
		return `${ms} ms`;
	}
	return `${(ms / MS_PER_SECOND).toFixed(1)} s`;
}

const RELATIVE_UNITS: { unit: Intl.RelativeTimeFormatUnit; seconds: number }[] = [
	{ unit: 'day', seconds: 86_400 },
	{ unit: 'hour', seconds: 3_600 },
	{ unit: 'minute', seconds: 60 }
];

const relativeFormat = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });

/** "5 minutes ago" style text for a past timestamp, or "—" when there is none. */
export function formatSince(iso: string | undefined, now: Date): string {
	if (!iso) {
		return '—';
	}
	const elapsedSeconds = (now.getTime() - new Date(iso).getTime()) / MS_PER_SECOND;
	const match = RELATIVE_UNITS.find(({ seconds }) => elapsedSeconds >= seconds);
	if (!match) {
		return 'just now';
	}
	return relativeFormat.format(-Math.floor(elapsedSeconds / match.seconds), match.unit);
}

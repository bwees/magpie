import type { SchedulesRequest, SchedulesResponse } from '$lib/sdk';

/** Ranges from 12-settings › Schedules; the server checks the same ones. */
export const LIMITS = {
	RSS_MINUTES: { MIN: 10, MAX: 120 },
	MISSING_SWEEP_DAYS: { MIN: 1, MAX: 7 },
	CUTOFF_SWEEP_DAYS: { MIN: 1, MAX: 30 }
} as const;

/** A number input's value: Svelte binds an empty number input to null. */
type NumberField = number | null;

export interface SchedulesDraft {
	rssIntervalMinutes: NumberField;
	missingSweepDays: NumberField;
	cutoffSweepDays: NumberField;
}

export function schedulesDraft(saved: SchedulesResponse): SchedulesDraft {
	return {
		rssIntervalMinutes: saved.rssIntervalMinutes,
		missingSweepDays: saved.missingSweepDays,
		cutoffSweepDays: saved.cutoffSweepDays
	};
}

function outside(value: NumberField, range: { MIN: number; MAX: number }): boolean {
	return value === null || !Number.isInteger(value) || value < range.MIN || value > range.MAX;
}

export function schedulesError(draft: SchedulesDraft): string | undefined {
	if (outside(draft.rssIntervalMinutes, LIMITS.RSS_MINUTES)) {
		return `RSS sync must be every ${LIMITS.RSS_MINUTES.MIN}–${LIMITS.RSS_MINUTES.MAX} minutes`;
	}
	if (outside(draft.missingSweepDays, LIMITS.MISSING_SWEEP_DAYS)) {
		return `The missing sweep must run every ${LIMITS.MISSING_SWEEP_DAYS.MIN}–${LIMITS.MISSING_SWEEP_DAYS.MAX} days`;
	}
	if (outside(draft.cutoffSweepDays, LIMITS.CUTOFF_SWEEP_DAYS)) {
		return `The cutoff-unmet sweep must run every ${LIMITS.CUTOFF_SWEEP_DAYS.MIN}–${LIMITS.CUTOFF_SWEEP_DAYS.MAX} days`;
	}
	return undefined;
}

/** Only call with a draft that has no error. */
export function schedulesRequest(draft: SchedulesDraft): SchedulesRequest {
	return {
		rssIntervalMinutes: draft.rssIntervalMinutes ?? LIMITS.RSS_MINUTES.MIN,
		missingSweepDays: draft.missingSweepDays ?? LIMITS.MISSING_SWEEP_DAYS.MIN,
		cutoffSweepDays: draft.cutoffSweepDays ?? LIMITS.CUTOFF_SWEEP_DAYS.MIN
	};
}

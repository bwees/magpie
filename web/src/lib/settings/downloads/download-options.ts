import type { DownloadOptionsRequest, DownloadOptionsResponse } from '$lib/sdk';

/** Mirrors the Go `RegexPatternPrefix`. */
export const REGEX_PREFIX = 'regex:';

const MINUTES_PER_HOUR = 60;

/** From 12-settings › Downloads. */
export const LIMITS = {
	METADATA_TIMEOUT_MINUTES: { MIN: 1, MAX: 60 },
	STALLED_LIMIT_HOURS: { MIN: 0.5, MAX: 72 }
} as const;

export const STALLED_LIMIT_STEP_HOURS = 0.5;

export interface TimeoutsDraft {
	metadataTimeoutMinutes: number | null;
	stalledLimitHours: number | null;
}

export function timeoutsDraft(options: DownloadOptionsResponse): TimeoutsDraft {
	return {
		metadataTimeoutMinutes: options.metadataTimeoutMinutes,
		stalledLimitHours: options.stalledLimitMinutes / MINUTES_PER_HOUR
	};
}

function outside(value: number | null, range: { MIN: number; MAX: number }): boolean {
	return value === null || Number.isNaN(value) || value < range.MIN || value > range.MAX;
}

export function timeoutsError(draft: TimeoutsDraft): string | undefined {
	const metadata = LIMITS.METADATA_TIMEOUT_MINUTES;
	if (outside(draft.metadataTimeoutMinutes, metadata)) {
		return `Metadata timeout must be ${metadata.MIN}–${metadata.MAX} minutes`;
	}
	if (outside(draft.stalledLimitHours, LIMITS.STALLED_LIMIT_HOURS)) {
		return 'Stalled limit must be 30 minutes to 72 hours';
	}
	return undefined;
}

/** The PUT body: the stored options with the given changes. */
export function optionsRequest(
	saved: DownloadOptionsResponse,
	change: Partial<DownloadOptionsRequest>
): DownloadOptionsRequest {
	return {
		blockedFilesEnabled: saved.blockedFilesEnabled,
		blockedPatterns: saved.blockedPatterns,
		metadataTimeoutMinutes: saved.metadataTimeoutMinutes,
		stalledLimitMinutes: saved.stalledLimitMinutes,
		...change
	};
}

export function timeoutsChange(draft: TimeoutsDraft): Partial<DownloadOptionsRequest> {
	return {
		metadataTimeoutMinutes: draft.metadataTimeoutMinutes ?? LIMITS.METADATA_TIMEOUT_MINUTES.MIN,
		stalledLimitMinutes: Math.round(
			(draft.stalledLimitHours ?? LIMITS.STALLED_LIMIT_HOURS.MIN) * MINUTES_PER_HOUR
		)
	};
}

/** Why a new pattern can't be added, or undefined. The server checks again. */
export function patternError(pattern: string, existing: string[]): string | undefined {
	if (pattern === '') {
		return 'Enter a pattern, such as *.exe';
	}
	if (existing.some((p) => p.toLowerCase() === pattern.toLowerCase())) {
		return `${pattern} is already on the list`;
	}
	if (pattern.startsWith(REGEX_PREFIX)) {
		try {
			new RegExp(pattern.slice(REGEX_PREFIX.length));
		} catch {
			return 'That regular expression doesn’t compile';
		}
		return undefined;
	}
	if (pattern.includes('/')) {
		return 'Patterns match file names only, not folders';
	}
	return undefined;
}

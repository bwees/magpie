import type {
	CoverageResponse,
	QuotaResponse,
	SubtitleResponse,
	SubtitleSettingsBody
} from '$lib/sdk';

/** Mirrors the Go `api.Source` values. */
export enum SubtitleSource {
	EMBEDDED = 'EMBEDDED',
	RELEASE = 'RELEASE',
	PROVIDER = 'PROVIDER',
	WHISPER = 'WHISPER'
}

/** Mirrors the Go `api.CoverageStatus` values. */
export enum CoverageStatus {
	SATISFIED = 'SATISFIED',
	PENDING = 'PENDING',
	MISSING = 'MISSING'
}

/** Mirrors the Go `api.MissingReason` values. */
export enum MissingReason {
	NO_MATCH = 'NO_MATCH',
	QUOTA_EXHAUSTED = 'QUOTA_EXHAUSTED',
	WHISPER_FAILED = 'WHISPER_FAILED',
	NOT_ORIGINAL_LANGUAGE = 'NOT_ORIGINAL_LANGUAGE'
}

export enum SubtitlesEvent {
	SUBTITLE_ADDED = 'subtitles.SubtitleAdded',
	SUBTITLE_MISSING = 'subtitles.SubtitleMissing',
	COVERAGE_SETTLED = 'subtitles.CoverageSettled'
}

export const SUBTITLE_EVENTS = Object.values(SubtitlesEvent);

/** Mirrors the Go `subtitles.WhisperModels`, as whisper.cpp names them. */
export const WHISPER_MODELS = ['large-v3-turbo', 'large-v3', 'medium', 'small'] as const;

export const SOURCE_LABELS: Record<string, string> = {
	[SubtitleSource.EMBEDDED]: 'Embedded',
	[SubtitleSource.RELEASE]: 'Release',
	[SubtitleSource.PROVIDER]: 'OpenSubtitles',
	[SubtitleSource.WHISPER]: 'Whisper'
};

export const COVERAGE_LABELS: Record<string, string> = {
	[CoverageStatus.SATISFIED]: 'Found',
	[CoverageStatus.PENDING]: 'Searching',
	[CoverageStatus.MISSING]: 'Missing'
};

export const REASON_LABELS: Record<string, string> = {
	[MissingReason.NO_MATCH]: 'no good match',
	[MissingReason.QUOTA_EXHAUSTED]: 'out of downloads',
	[MissingReason.WHISPER_FAILED]: 'Whisper failed',
	[MissingReason.NOT_ORIGINAL_LANGUAGE]: 'no match, not the original language'
};

/** Ranges from 12-settings › Subtitles; the server checks the same ones. */
export const LIMITS = {
	MIN_SCORE: { MIN: 50, MAX: 100 },
	UPGRADE_WINDOW_DAYS: { MIN: 0, MAX: 30 },
	UPGRADE_CHECK_HOURS: { MIN: 1, MAX: 24 }
} as const;

const MS_PER_SECOND = 1000;
const OFFSET_DECIMALS = 1;

const languageNames = new Intl.DisplayNames(['en'], { type: 'language' });

/** "English", or the code itself when the browser doesn't know it. */
export function languageName(code: string): string {
	try {
		return languageNames.of(code) ?? code;
	} catch {
		return code;
	}
}

/** "English", "English · forced" or "English · SDH". */
export function trackLabel(track: { language: string; forced: boolean; sdh?: boolean }): string {
	const parts = [languageName(track.language)];
	if (track.forced) {
		parts.push('forced');
	} else if (track.sdh) {
		parts.push('SDH');
	}
	return parts.join(' · ');
}

/** "synced +1.2 s", "synced" for no shift, or "not synced". Embedded tracks have no sync. */
export function syncLabel(sub: SubtitleResponse): string | undefined {
	if (sub.source === SubtitleSource.EMBEDDED || sub.source === SubtitleSource.RELEASE) {
		return undefined;
	}
	if (!sub.synced) {
		return 'not synced';
	}
	const ms = sub.syncOffsetMs ?? 0;
	if (ms === 0) {
		return 'synced';
	}
	const sign = ms > 0 ? '+' : '−';
	return `synced ${sign}${(Math.abs(ms) / MS_PER_SECOND).toFixed(OFFSET_DECIMALS)} s`;
}

/** The status line of one wanted track: "Found · Embedded", "Missing · no good match", "Waiting for quota". */
export function coverageLabel(c: CoverageResponse): string {
	if (c.status === CoverageStatus.PENDING && c.waitingForQuota) {
		return 'Waiting for quota';
	}
	if (c.status === CoverageStatus.PENDING && c.waitingForWhisper) {
		return 'Generating with Whisper';
	}
	const status = COVERAGE_LABELS[c.status] ?? c.status;
	if (c.status === CoverageStatus.SATISFIED && c.source) {
		return `${status} · ${SOURCE_LABELS[c.source] ?? c.source}`;
	}
	if (c.status === CoverageStatus.MISSING && c.reason) {
		return `${status} · ${REASON_LABELS[c.reason] ?? c.reason}`;
	}
	return status;
}

/** The file name without its folders. */
export function fileName(path: string): string {
	return path.split('/').at(-1) ?? path;
}

/** The distinct wanted languages of a file, in wanted order, for the candidate picker. */
export function wantedLanguages(coverage: CoverageResponse[]): string[] {
	return [...new Set(coverage.map((c) => c.language))];
}

/** "17 left today", "VIP · 940 left today", or undefined before OpenSubtitles first said. */
export function quotaLabel(q: QuotaResponse): string | undefined {
	if (q.remaining === null) {
		return undefined;
	}
	const left = `${q.remaining} left today`;
	return q.vip ? `VIP · ${left}` : left;
}

/** A number input's value: Svelte binds an empty number input to null. */
type NumberField = number | null;

export interface SettingsDraft {
	whisperEnabled: boolean;
	whisperModel: string;
	autoSync: boolean;
	minScoreEpisode: NumberField;
	minScoreMovie: NumberField;
	upgradeWindowDays: NumberField;
	upgradeCheckHours: NumberField;
}

export function settingsDraft(saved: SubtitleSettingsBody): SettingsDraft {
	return { ...saved };
}

function outside(value: NumberField, range: { MIN: number; MAX: number }): boolean {
	return value === null || !Number.isInteger(value) || value < range.MIN || value > range.MAX;
}

export function settingsError(draft: SettingsDraft): string | undefined {
	if (
		outside(draft.minScoreEpisode, LIMITS.MIN_SCORE) ||
		outside(draft.minScoreMovie, LIMITS.MIN_SCORE)
	) {
		return `Minimum scores must be ${LIMITS.MIN_SCORE.MIN}–${LIMITS.MIN_SCORE.MAX}%`;
	}
	if (outside(draft.upgradeWindowDays, LIMITS.UPGRADE_WINDOW_DAYS)) {
		return `The upgrade window must be ${LIMITS.UPGRADE_WINDOW_DAYS.MIN}–${LIMITS.UPGRADE_WINDOW_DAYS.MAX} days`;
	}
	if (outside(draft.upgradeCheckHours, LIMITS.UPGRADE_CHECK_HOURS)) {
		return `The upgrade check must run every ${LIMITS.UPGRADE_CHECK_HOURS.MIN}–${LIMITS.UPGRADE_CHECK_HOURS.MAX} hours`;
	}
	return undefined;
}

/** Only call with a draft that has no error. */
export function settingsRequest(draft: SettingsDraft): SubtitleSettingsBody {
	return {
		whisperEnabled: draft.whisperEnabled,
		whisperModel: draft.whisperModel,
		autoSync: draft.autoSync,
		minScoreEpisode: draft.minScoreEpisode ?? LIMITS.MIN_SCORE.MAX,
		minScoreMovie: draft.minScoreMovie ?? LIMITS.MIN_SCORE.MAX,
		upgradeWindowDays: draft.upgradeWindowDays ?? LIMITS.UPGRADE_WINDOW_DAYS.MIN,
		upgradeCheckHours: draft.upgradeCheckHours ?? LIMITS.UPGRADE_CHECK_HOURS.MIN
	};
}

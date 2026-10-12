import type { TranscodeSettingsResponse, UpdateTranscodeSettingsRequest } from '$lib/sdk';

/** Mirrors the Go `api.TargetCodec` values. */
export enum TargetCodec {
	HEVC_10 = 'HEVC_10',
	AV1_10 = 'AV1_10'
}

export const TARGET_LABELS: Record<TargetCodec, string> = {
	[TargetCodec.HEVC_10]: 'HEVC 10-bit',
	[TargetCodec.AV1_10]: 'AV1 10-bit'
};

export const TARGET_HINTS: Record<TargetCodec, string> = {
	[TargetCodec.HEVC_10]: 'Plays almost everywhere',
	[TargetCodec.AV1_10]: 'Smaller files; only agents that passed an AV1 test encode take these jobs'
};

/** Mirrors the Go `api.Preset` values, from the highest quality to the smallest files. */
export enum Preset {
	ARCHIVE = 'ARCHIVE',
	HIGH = 'HIGH',
	BALANCED = 'BALANCED',
	SMALL = 'SMALL'
}

export const PRESET_LABELS: Record<Preset, string> = {
	[Preset.ARCHIVE]: 'Archive',
	[Preset.HIGH]: 'High',
	[Preset.BALANCED]: 'Balanced',
	[Preset.SMALL]: 'Small'
};

/** Mirrors the Go `api.ResolutionClass` values, largest first. */
export enum ResolutionClass {
	UHD = '2160P',
	FHD = '1080P',
	HD = '720P',
	SD = 'SD'
}

export const RESOLUTION_LABELS: Record<ResolutionClass, string> = {
	[ResolutionClass.UHD]: '2160p',
	[ResolutionClass.FHD]: '1080p',
	[ResolutionClass.HD]: '720p',
	[ResolutionClass.SD]: 'SD'
};

/** Mirrors the Go `api.SkipReason` values. */
export enum SkipReason {
	EXCLUDED = 'EXCLUDED',
	HDR_DYNAMIC = 'HDR_DYNAMIC',
	ALREADY_TARGET = 'ALREADY_TARGET',
	LOW_BITRATE = 'LOW_BITRATE',
	NOT_WORTH_IT = 'NOT_WORTH_IT',
	NO_CAPABLE_AGENT = 'NO_CAPABLE_AGENT'
}

export const SKIP_LABELS: Record<SkipReason, string> = {
	[SkipReason.EXCLUDED]: 'Excluded',
	[SkipReason.HDR_DYNAMIC]: 'Dolby Vision or HDR10+',
	[SkipReason.ALREADY_TARGET]: 'Already the target',
	[SkipReason.LOW_BITRATE]: 'Low bitrate',
	[SkipReason.NOT_WORTH_IT]: 'Not worth it',
	[SkipReason.NO_CAPABLE_AGENT]: 'No capable agent'
};

/** Bounds from 12-settings › Transcode. */
export const BITRATE_CAP_MBPS = { MIN: 1, MAX: 200 } as const;
export const LOW_BITRATE_PCT = { MIN: 10, MAX: 100 } as const;
export const MIN_SAVING_PCT = { MIN: 0, MAX: 90 } as const;

export interface Bounds {
	MIN: number;
	MAX: number;
}

export function inRange(value: number, bounds: Bounds): boolean {
	return Number.isInteger(value) && value >= bounds.MIN && value <= bounds.MAX;
}

function sameRecord(a: Record<string, number>, b: Record<string, number>): boolean {
	const keys = Object.keys(a);
	return keys.length === Object.keys(b).length && keys.every((key) => a[key] === b[key]);
}

/** Whether two PUT bodies store the same settings. */
export function sameSettings(
	a: UpdateTranscodeSettingsRequest,
	b: UpdateTranscodeSettingsRequest
): boolean {
	return (
		a.enabled === b.enabled &&
		a.targetCodec === b.targetCodec &&
		a.preset === b.preset &&
		sameRecord(a.bitrateCapsMbps, b.bitrateCapsMbps) &&
		a.skipAlreadyTarget === b.skipAlreadyTarget &&
		a.skipLowBitrate === b.skipLowBitrate &&
		a.lowBitratePct === b.lowBitratePct &&
		a.minSavingPct === b.minSavingPct &&
		sameRecord(a.encoderOverrides, b.encoderOverrides) &&
		a.transferCap === b.transferCap &&
		a.defaultSlotsPerGpu === b.defaultSlotsPerGpu
	);
}

/** The PUT body that stores settings as they are; the calibration table is read-only. */
export function toUpdate(settings: TranscodeSettingsResponse): UpdateTranscodeSettingsRequest {
	return {
		enabled: settings.enabled,
		targetCodec: settings.targetCodec,
		preset: settings.preset,
		bitrateCapsMbps: { ...settings.bitrateCapsMbps },
		skipAlreadyTarget: settings.skipAlreadyTarget,
		skipLowBitrate: settings.skipLowBitrate,
		lowBitratePct: settings.lowBitratePct,
		minSavingPct: settings.minSavingPct,
		encoderOverrides: { ...settings.encoderOverrides },
		transferCap: settings.transferCap,
		defaultSlotsPerGpu: settings.defaultSlotsPerGpu
	};
}

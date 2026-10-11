import type {
	AcquisitionSettingsRequest,
	AcquisitionSettingsResponse,
	FormatScoreResponse,
	ProfileItemResponse,
	ProfileRequest,
	ProfileResponse,
	ProfileSummaryResponse
} from '$lib/sdk';

export enum QualityPreset {
	P720 = '720P',
	P1080 = '1080P',
	P2160 = '2160P'
}

export enum PropersMode {
	PREFER_AND_UPGRADE = 'PREFER_AND_UPGRADE',
	DO_NOT_UPGRADE = 'DO_NOT_UPGRADE',
	DO_NOT_PREFER = 'DO_NOT_PREFER'
}

export const PROPERS_LABELS: Record<PropersMode, string> = {
	[PropersMode.PREFER_AND_UPGRADE]: 'Prefer and upgrade',
	[PropersMode.DO_NOT_UPGRADE]: "Don't upgrade",
	[PropersMode.DO_NOT_PREFER]: "Don't prefer"
};

export interface PresetSpec {
	preset: QualityPreset;
	label: string;
	description: string;
}

/** The first-run resolution presets: every source at one resolution, which is also the cutoff. */
export const PRESETS: PresetSpec[] = [
	{ preset: QualityPreset.P720, label: '720p', description: 'Small files, fine on phones.' },
	{ preset: QualityPreset.P1080, label: '1080p', description: 'Full HD. The usual choice.' },
	{ preset: QualityPreset.P2160, label: '2160p', description: '4K for big screens.' }
];

/** Ranges from 12-settings › Quality; the server checks the same ones. */
export const LIMITS = {
	DELAY_MINUTES: { MIN: 0, MAX: 1440 },
	SETTLE_SECONDS: { MIN: 5, MAX: 120 }
} as const;

/** TRaSH's "never grab" score, which "Block term" gives its format. */
export const BLOCK_SCORE = -10000;

const MS_PER_MINUTE = 60_000;
const MINUTES_PER_HOUR = 60;
const HOURS_PER_DAY = 24;

/** A number input's value: Svelte binds an empty number input to null. */
type NumberField = number | null;

export interface SettingsDraft {
	defaultProfileId: string;
	defaultDelayMinutes: NumberField;
	settleWindowSeconds: NumberField;
	trashSyncEnabled: boolean;
}

export function settingsDraft(settings: AcquisitionSettingsResponse): SettingsDraft {
	return { ...settings };
}

function outside(value: NumberField, range: { MIN: number; MAX: number }): boolean {
	return value === null || Number.isNaN(value) || value < range.MIN || value > range.MAX;
}

export function settingsDraftError(draft: SettingsDraft): string | undefined {
	if (!draft.defaultProfileId) {
		return 'Pick a default profile';
	}
	if (outside(draft.defaultDelayMinutes, LIMITS.DELAY_MINUTES)) {
		return `Delay must be ${LIMITS.DELAY_MINUTES.MIN}–${LIMITS.DELAY_MINUTES.MAX} minutes`;
	}
	if (outside(draft.settleWindowSeconds, LIMITS.SETTLE_SECONDS)) {
		return `Settle window must be ${LIMITS.SETTLE_SECONDS.MIN}–${LIMITS.SETTLE_SECONDS.MAX} seconds`;
	}
	return undefined;
}

/** Only call with a draft that has no error. */
export function settingsRequest(draft: SettingsDraft): AcquisitionSettingsRequest {
	return {
		defaultProfileId: draft.defaultProfileId,
		defaultDelayMinutes: draft.defaultDelayMinutes ?? LIMITS.DELAY_MINUTES.MIN,
		settleWindowSeconds: draft.settleWindowSeconds ?? LIMITS.SETTLE_SECONDS.MIN,
		trashSyncEnabled: draft.trashSyncEnabled
	};
}

export function presetProfile(
	profiles: ProfileSummaryResponse[],
	preset: QualityPreset
): ProfileSummaryResponse | undefined {
	return profiles.find((profile) => profile.preset === preset);
}

export interface ItemDraft {
	/** The group's name, or '' for a single quality. */
	groupName: string;
	qualityIds: number[];
	label: string;
	allowed: boolean;
	minMbPerMin: NumberField;
	preferredMbPerMin: NumberField;
	maxMbPerMin: NumberField;
}

export interface ProfileDraft {
	name: string;
	upgradeAllowed: boolean;
	items: ItemDraft[];
	cutoff: number;
	minFormatScore: NumberField;
	cutoffFormatScore: NumberField;
	minUpgradeFormatScore: NumberField;
	propersMode: PropersMode;
	formatScores: FormatScoreResponse[];
}

function itemLabel(item: ProfileItemResponse): string {
	return item.group?.name ?? item.qualities[0]?.name ?? '';
}

export function profileDraft(profile: ProfileResponse): ProfileDraft {
	return {
		name: profile.name,
		upgradeAllowed: profile.upgradeAllowed,
		items: profile.items.map((item) => ({
			groupName: item.group?.name ?? '',
			qualityIds: item.qualities.map((quality) => quality.id),
			label: itemLabel(item),
			allowed: item.allowed,
			minMbPerMin: item.minMbPerMin,
			preferredMbPerMin: item.preferredMbPerMin,
			maxMbPerMin: item.maxMbPerMin
		})),
		cutoff: profile.cutoff,
		minFormatScore: profile.minFormatScore,
		cutoffFormatScore: profile.cutoffFormatScore,
		minUpgradeFormatScore: profile.minUpgradeFormatScore,
		propersMode: profile.propersMode as PropersMode,
		formatScores: profile.formatScores.map((score) => ({ ...score }))
	};
}

function blank(value: NumberField): boolean {
	return value === null || Number.isNaN(value);
}

/** The first problem with a profile draft, or undefined when it can be saved. */
export function profileDraftError(draft: ProfileDraft): string | undefined {
	if (!draft.name.trim()) {
		return 'The profile needs a name';
	}
	if (!draft.items[draft.cutoff]?.allowed) {
		return 'The cutoff must be an allowed quality';
	}
	for (const item of draft.items) {
		const { minMbPerMin: min, preferredMbPerMin: preferred, maxMbPerMin: max } = item;
		if (blank(min) || blank(preferred) || blank(max)) {
			return `${item.label}: fill in every size`;
		}
		if (min! < 0 || min! > preferred! || preferred! > max!) {
			return `${item.label}: sizes must be 0 ≤ min ≤ preferred ≤ max`;
		}
	}
	if (
		[draft.minFormatScore, draft.cutoffFormatScore, draft.minUpgradeFormatScore].some(blank) ||
		draft.formatScores.some((score) => blank(score.score))
	) {
		return 'Fill in every score';
	}
	return undefined;
}

/** Only call with a draft that has no error. */
export function profileRequest(draft: ProfileDraft): ProfileRequest {
	return {
		name: draft.name.trim(),
		upgradeAllowed: draft.upgradeAllowed,
		items: draft.items.map((item) => ({
			groupName: item.groupName || undefined,
			qualityIds: item.qualityIds,
			allowed: item.allowed,
			minMbPerMin: item.minMbPerMin ?? 0,
			preferredMbPerMin: item.preferredMbPerMin ?? 0,
			maxMbPerMin: item.maxMbPerMin ?? 0
		})),
		cutoff: draft.cutoff,
		minFormatScore: draft.minFormatScore ?? 0,
		cutoffFormatScore: draft.cutoffFormatScore ?? 0,
		minUpgradeFormatScore: draft.minUpgradeFormatScore ?? 0,
		propersMode: draft.propersMode,
		formatScores: draft.formatScores.map((score) => ({
			formatId: score.formatId,
			score: score.score
		}))
	};
}

/** "3 hours ago"-style text for the last TRaSH sync. */
export function formatSyncedAt(iso: string | undefined, now: Date): string {
	if (!iso) {
		return 'Never synced';
	}
	const minutes = Math.round((now.getTime() - new Date(iso).getTime()) / MS_PER_MINUTE);
	if (minutes < 1) {
		return 'Synced just now';
	}
	if (minutes < MINUTES_PER_HOUR) {
		return `Synced ${minutes} min ago`;
	}
	const hours = Math.round(minutes / MINUTES_PER_HOUR);
	if (hours < HOURS_PER_DAY) {
		return `Synced ${hours} h ago`;
	}
	return `Synced ${Math.round(hours / HOURS_PER_DAY)} d ago`;
}

import type {
	AttentionItemResponse,
	DiscoverDetailResponse,
	IssueResponse,
	MediaFileResponse,
	PipelineRowResponse,
	RequestResponse,
	TitleCoverageResponse,
	TitleResponse,
	TranscodeSettingsResponse
} from '$lib/sdk';
import { Stage } from '$lib/activity/pipeline';
import { MediaKind } from '$lib/catalog/title-page.svelte';
import { Preset, ResolutionClass, TargetCodec } from '$lib/transcode/transcode-settings';

// Complete API responses for manager tests; each takes the fields a test cares about.

export const AT = '2026-10-11T12:00:00Z';

export function title(overrides: Partial<TitleResponse> = {}): TitleResponse {
	return {
		id: 'T1',
		kind: MediaKind.MOVIE,
		name: 'Dune',
		originalName: 'Dune',
		overview: '',
		year: 2021,
		tmdbId: 1,
		tvdbId: null,
		imdbId: null,
		addedAt: AT,
		episodeGroupId: '',
		images: { poster: null, backdrop: null, logo: null },
		languagesOverride: null,
		monitored: true,
		monitorFutureSeasons: false,
		origin: 'REQUEST',
		originalLanguage: 'en',
		profileId: 'P1',
		releaseDates: { theatrical: null, digital: null, physical: null },
		released: true,
		releasedAt: null,
		requestedBy: null,
		runtimeMinutes: 0,
		seasonCount: 0,
		seasons: [],
		seriesStatus: '',
		wantedEpisodeCount: 0,
		...overrides
	};
}

export function pipelineRow(overrides: Partial<PipelineRowResponse> = {}): PipelineRowResponse {
	return {
		titleId: 'T1',
		episodeId: null,
		season: 0,
		number: 0,
		downloadId: null,
		stage: Stage.REQUESTED,
		failed: false,
		detail: '',
		updatedAt: AT,
		...overrides
	};
}

export function mediaFile(overrides: Partial<MediaFileResponse> = {}): MediaFileResponse {
	return {
		id: 'F1',
		title: 'T1',
		available: true,
		episodes: [],
		importedAt: AT,
		jellyfinOverdue: false,
		mediaInfo: {
			bitrateKbps: 0,
			container: 'mkv',
			durationSeconds: 0,
			hdr: 'NONE',
			height: 0,
			streams: [],
			width: 0
		},
		origin: 'DOWNLOAD',
		path: '/media/movies/Dune.mkv',
		quality: { id: 0, name: 'Bluray-1080p', resolution: 1080, revision: 1, source: 'BLURAY' },
		releaseName: '',
		rootFolder: '/media/movies',
		size: 0,
		transcoded: false,
		...overrides
	};
}

export function coverage(overrides: Partial<TitleCoverageResponse> = {}): TitleCoverageResponse {
	return { title: 'T1', episodesWithFile: 0, episodesAvailable: 0, sizeBytes: 0, ...overrides };
}

export function attentionItem(
	overrides: Partial<AttentionItemResponse> = {}
): AttentionItemResponse {
	return {
		id: 'A1',
		kind: 'PIPELINE_FAILED',
		ref: 'T1',
		titleId: 'T1',
		text: 'No accepted release',
		action: 'SEARCH_AGAIN',
		openedAt: AT,
		...overrides
	};
}

export function request(overrides: Partial<RequestResponse> = {}): RequestResponse {
	return {
		id: 'R1',
		kind: MediaKind.MOVIE,
		tmdbId: 1,
		titleId: null,
		scope: 'MOVIE',
		seasons: [],
		status: 'PENDING',
		profileId: 'P1',
		requester: { id: 'U1', displayName: 'Ana' },
		availableItems: 0,
		totalItems: 0,
		createdAt: AT,
		decidedAt: null,
		decidedBy: null,
		declineReason: '',
		failureReason: '',
		...overrides
	};
}

export function issue(overrides: Partial<IssueResponse> = {}): IssueResponse {
	return {
		id: 'I1',
		reporter: { id: 'U1', displayName: 'Ana' },
		titleId: 'T1',
		titleName: 'Severance',
		episodeId: 'E1',
		episode: { id: 'E1', season: 1, number: 2, name: 'Half Loop' },
		kind: 'AUDIO',
		text: 'The audio drifts',
		status: 'OPEN',
		action: '',
		comments: 0,
		createdAt: AT,
		resolvedAt: null,
		resolvedBy: '',
		...overrides
	};
}

export function media(overrides: Partial<DiscoverDetailResponse> = {}): DiscoverDetailResponse {
	return {
		kind: MediaKind.MOVIE,
		tmdbId: 1,
		name: 'Dune',
		year: 2021,
		overview: '',
		posterUrl: null,
		inLibrary: false,
		titleId: null,
		seasons: [],
		...overrides
	};
}

export function transcodeSettings(
	overrides: Partial<TranscodeSettingsResponse> = {}
): TranscodeSettingsResponse {
	return {
		enabled: true,
		targetCodec: TargetCodec.HEVC_10,
		preset: Preset.HIGH,
		bitrateCapsMbps: {
			[ResolutionClass.UHD]: 25,
			[ResolutionClass.FHD]: 10,
			[ResolutionClass.HD]: 5,
			[ResolutionClass.SD]: 2
		},
		skipAlreadyTarget: true,
		skipLowBitrate: true,
		lowBitratePct: 50,
		minSavingPct: 15,
		encoderOverrides: {},
		transferCap: 2,
		defaultSlotsPerGpu: 1,
		encoders: [
			{
				encoder: 'hevc_nvenc',
				qualityArg: 'cq',
				min: 1,
				max: 51,
				presets: {
					[Preset.ARCHIVE]: 19,
					[Preset.HIGH]: 23,
					[Preset.BALANCED]: 26,
					[Preset.SMALL]: 29
				}
			},
			{
				encoder: 'libsvtav1',
				qualityArg: 'crf',
				min: 1,
				max: 63,
				presets: {
					[Preset.ARCHIVE]: 22,
					[Preset.HIGH]: 28,
					[Preset.BALANCED]: 32,
					[Preset.SMALL]: 38
				}
			}
		],
		...overrides
	};
}

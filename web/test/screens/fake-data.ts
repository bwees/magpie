import type {
	AgentResponse,
	AttentionItemResponse,
	CutoffStatusResponse,
	DiscoverDetailResponse,
	DiscoverResponse,
	DownloadResponse,
	EpisodeGroupResponse,
	EpisodeResponse,
	FileSubtitlesResponse,
	IndexerListResponse,
	IndexerOptionsResponse,
	IndexerResponse,
	JobListResponse,
	JobResponse,
	MeResponse,
	MediaFileResponse,
	NotificationListResponse,
	PipelineRowResponse,
	ProfileSummaryResponse,
	Quality,
	ReleaseDecision,
	RequestListResponse,
	RequestResponse,
	SearchResponse,
	SeasonResponse,
	TimelineEntryResponse,
	TitleCoverageResponse,
	TitleResponse,
	WantedLanguagesResponse
} from '../../src/lib/sdk';
import { AttentionAction, AttentionKind } from '../../src/lib/activity/attention';
import { Severity, Stage, SubsState, TranscodeState } from '../../src/lib/activity/pipeline';
import { DownloadState, SeedingMode } from '../../src/lib/downloads/downloads';
import { MediaKind } from '../../src/lib/library/library';
import {
	IndexerPrivacy,
	IndexerStatus,
	SeedingOverride
} from '../../src/lib/settings/indexers/indexer-settings';
import { Capability, TransferMode } from '../../src/lib/transcode/agents';
import { JobKind, JobOrigin, JobState } from '../../src/lib/transcode/gpu-jobs';

/** The moment every screenshot is taken at, so relative times never change. */
export const NOW = '2026-10-11T12:00:00Z';

const MINUTE_MS = 60_000;
const HOUR_MS = 60 * MINUTE_MS;
const DAY_MS = 24 * HOUR_MS;
const GIB = 1024 ** 3;
const MIB = 1024 ** 2;

function ago(ms: number): string {
	return new Date(Date.parse(NOW) - ms).toISOString();
}

const ADMIN_ROLE = 'ADMIN';
const REQUEST_ORIGIN = 'REQUEST';
const ENGLISH = 'en';

/** Mirrors $lib/requests/requests, which loads the SDK client and so can't run outside Vite. */
enum RequestStatus {
	PENDING = 'PENDING',
	APPROVED = 'APPROVED',
	AVAILABLE = 'AVAILABLE',
	DECLINED = 'DECLINED'
}

enum RequestScope {
	MOVIE = 'MOVIE',
	SEASONS = 'SEASONS'
}

/** Values the Go side owns but the web app has no enum for. */
const SEARCH_INDEXER = { DONE: 'DONE', PENDING: 'PENDING', FAILED: 'FAILED' } as const;
const SEARCH_TRIGGER_INTERACTIVE = 'INTERACTIVE';
const GRAB_TRIGGER = 'AUTOMATIC';
const NOTIFICATION_KIND = 'TITLE_AVAILABLE';

export const ME: MeResponse = {
	id: 'U1',
	displayName: 'Brandon',
	username: 'admin',
	localAdmin: true,
	role: ADMIN_ROLE,
	permissions: {
		autoApproveMovies: true,
		autoApproveTv: true,
		reportIssues: true,
		viewOthersRequests: true
	}
};

const REQUESTERS = [
	{ id: 'U2', displayName: 'Maya Lin' },
	{ id: 'U3', displayName: 'Theo Grant' }
];

/** A poster colour per title, so the generated posters look different. */
export const POSTER_COLOURS: Record<string, string> = {
	M1: '#b07a3c',
	M2: '#3c5a7a',
	M3: '#6b3c7a',
	M4: '#7a3c3c',
	M5: '#3c7a5e',
	M6: '#7a6b3c',
	S1: '#2f6f73',
	S2: '#5e4a7a',
	S3: '#7a5a3c',
	S4: '#3c4f7a'
};

function poster(id: string): string {
	return `/api/catalog/images/${id}/poster`;
}

function movie(id: string, name: string, year: number, overview = ''): TitleResponse {
	return {
		id,
		kind: MediaKind.MOVIE,
		name,
		originalName: name,
		overview,
		year,
		tmdbId: year,
		tvdbId: null,
		imdbId: null,
		addedAt: ago(DAY_MS),
		episodeGroupId: '',
		images: { poster: poster(id), backdrop: null, logo: null },
		languagesOverride: null,
		monitored: true,
		monitorFutureSeasons: false,
		origin: REQUEST_ORIGIN,
		originalLanguage: ENGLISH,
		profileId: 'P1',
		releaseDates: { theatrical: null, digital: null, physical: null },
		released: true,
		releasedAt: null,
		requestedBy: null,
		runtimeMinutes: 0,
		seasonCount: 0,
		seasons: [],
		seriesStatus: '',
		wantedEpisodeCount: 0
	};
}

const EPISODE_NAMES = [
	'Good News About Hell',
	'Half Loop',
	'In Perpetuity',
	'The You You Are',
	'The Grim Barbarity of Optics and Design',
	'Hide and Seek',
	'Defiant Jazz',
	"What's for Dinner?",
	'The We We Are'
];

function episodes(
	seriesId: string,
	season: number,
	count: number,
	released: number
): EpisodeResponse[] {
	return EPISODE_NAMES.slice(0, count).map((name, index) => ({
		id: `${seriesId}-S${season}E${index + 1}`,
		name,
		number: index + 1,
		season,
		airedNumber: index + 1,
		airedSeason: season,
		airDate: ago((count - index) * 7 * DAY_MS),
		monitored: true,
		released: index < released,
		runtimeMinutes: 55
	}));
}

function series(
	id: string,
	name: string,
	year: number,
	status: string,
	seasons: SeasonResponse[],
	overview = ''
): TitleResponse {
	const wanted = seasons.flatMap((season) => season.episodes.filter((e) => e.released)).length;
	return {
		...movie(id, name, year, overview),
		kind: MediaKind.SERIES,
		seriesStatus: status,
		seasonCount: seasons.length,
		seasons,
		wantedEpisodeCount: wanted,
		monitorFutureSeasons: true
	};
}

const CONTINUING = 'CONTINUING';
const ENDED = 'ENDED';

export const MOVIES: TitleResponse[] = [
	{
		...movie(
			'M1',
			'Dune: Part Two',
			2024,
			'Paul Atreides unites with Chani and the Fremen while on a path of revenge against the conspirators who destroyed his family. Facing a choice between the love of his life and the fate of the known universe, he endeavors to prevent a terrible future only he can foresee.'
		),
		requestedBy: 'U2'
	},
	movie('M2', 'Oppenheimer', 2023),
	movie('M3', 'Past Lives', 2023),
	movie('M4', 'Aftersun', 2022),
	movie('M5', 'Perfect Days', 2023),
	movie('M6', 'The Holdovers', 2023)
];

export const SERIES: TitleResponse[] = [
	{
		...series(
			'S1',
			'Severance',
			2022,
			CONTINUING,
			[
				{ number: 1, monitored: true, episodes: episodes('S1', 1, 9, 9) },
				{ number: 2, monitored: true, episodes: episodes('S1', 2, 6, 4) }
			],
			'Mark leads a team of office workers whose memories have been surgically divided between their work and personal lives. When a mysterious colleague appears outside of work, it begins a journey to discover the truth about their jobs.'
		),
		nextEpisode: { season: 2, number: 5, airDate: '2026-10-17' }
	},
	series('S2', 'Slow Horses', 2022, CONTINUING, [
		{ number: 1, monitored: true, episodes: episodes('S2', 1, 6, 6) }
	]),
	series('S3', 'Chernobyl', 2019, ENDED, [
		{ number: 1, monitored: true, episodes: episodes('S3', 1, 5, 5) }
	]),
	series('S4', 'Station Eleven', 2021, ENDED, [
		{ number: 1, monitored: true, episodes: episodes('S4', 1, 9, 9) }
	])
];

export const TITLES = [...MOVIES, ...SERIES];

export const MOVIE_ID = 'M1';
export const SERIES_ID = 'S1';

function row(overrides: Partial<PipelineRowResponse>): PipelineRowResponse {
	return {
		titleId: MOVIE_ID,
		episodeId: null,
		season: 0,
		number: 0,
		downloadId: null,
		stage: Stage.REQUESTED,
		failed: false,
		detail: '',
		updatedAt: ago(HOUR_MS),
		...overrides
	};
}

function episodeRows(seriesId: string, season: number, stages: Stage[]): PipelineRowResponse[] {
	return stages.map((stage, index) =>
		row({
			titleId: seriesId,
			episodeId: `${seriesId}-S${season}E${index + 1}`,
			season,
			number: index + 1,
			stage,
			detail: stage === Stage.DOWNLOADING ? '12.4 MB/s · 6 min left' : '',
			subs:
				stage === Stage.AVAILABLE
					? { state: SubsState.SETTLED, satisfied: [ENGLISH], missing: [] }
					: undefined
		})
	);
}

export const PIPELINE: PipelineRowResponse[] = [
	row({
		titleId: 'M1',
		stage: Stage.AVAILABLE,
		updatedAt: ago(20 * MINUTE_MS),
		subs: { state: SubsState.SETTLED, satisfied: [ENGLISH], missing: ['es'] },
		transcode: { state: TranscodeState.RUNNING, jobId: 'J1', reason: '', savedPercent: null }
	}),
	row({
		titleId: 'M2',
		stage: Stage.DOWNLOADING,
		downloadId: 'D1',
		detail: '18.2 MB/s · 4 min left',
		updatedAt: ago(2 * MINUTE_MS)
	}),
	row({
		titleId: 'M3',
		stage: Stage.SEARCHING,
		detail: '7 of 9 trackers answered',
		updatedAt: ago(MINUTE_MS)
	}),
	row({
		titleId: 'M4',
		stage: Stage.SEARCHING,
		failed: true,
		detail: 'No release met the profile',
		updatedAt: ago(3 * HOUR_MS)
	}),
	row({ titleId: 'M5', stage: Stage.REQUESTED, updatedAt: ago(5 * HOUR_MS) }),
	row({ titleId: 'M6', stage: Stage.AVAILABLE, updatedAt: ago(2 * DAY_MS) }),
	...episodeRows('S1', 1, Array(9).fill(Stage.AVAILABLE)),
	...episodeRows('S1', 2, [Stage.AVAILABLE, Stage.AVAILABLE, Stage.DOWNLOADING, Stage.SEARCHING]),
	...episodeRows('S2', 1, Array(6).fill(Stage.AVAILABLE)),
	...episodeRows('S3', 1, [Stage.AVAILABLE, Stage.AVAILABLE, Stage.IMPORTING])
];

/** Rows still moving through the pipeline: what "Live now" and the Activity badge show. */
export const ACTIVE_PIPELINE = PIPELINE.filter(
	(entry) => entry.stage !== Stage.AVAILABLE && entry.stage !== Stage.REQUESTED && !entry.failed
);

export const ATTENTION: AttentionItemResponse[] = [
	{
		id: 'A1',
		kind: AttentionKind.REQUEST_PENDING,
		action: AttentionAction.APPROVE,
		ref: 'R1',
		text: 'Maya Lin requested it',
		titleId: null,
		openedAt: ago(30 * MINUTE_MS)
	},
	{
		id: 'A2',
		kind: AttentionKind.PIPELINE_FAILED,
		action: AttentionAction.SEARCH_AGAIN,
		ref: 'M4',
		text: 'No release met the profile',
		titleId: 'M4',
		openedAt: ago(3 * HOUR_MS)
	},
	{
		id: 'A3',
		kind: AttentionKind.AGENT_OFFLINE,
		action: AttentionAction.VIEW_AGENT,
		ref: 'G2',
		text: 'basement-gpu has been offline for 2 hours',
		titleId: null,
		openedAt: ago(2 * HOUR_MS)
	}
];

function request(overrides: Partial<RequestResponse>): RequestResponse {
	return {
		id: 'R1',
		kind: MediaKind.MOVIE,
		tmdbId: 1,
		titleId: null,
		scope: RequestScope.MOVIE,
		seasons: [],
		profileId: 'P1',
		requester: REQUESTERS[0],
		status: RequestStatus.PENDING,
		createdAt: ago(30 * MINUTE_MS),
		decidedAt: null,
		decidedBy: null,
		declineReason: '',
		failureReason: '',
		availableItems: 0,
		totalItems: 1,
		...overrides
	};
}

export const REQUESTS: RequestResponse[] = [
	request({ id: 'R1', tmdbId: 101 }),
	request({
		id: 'R2',
		kind: MediaKind.SERIES,
		tmdbId: 102,
		scope: RequestScope.SEASONS,
		seasons: [1, 2],
		requester: REQUESTERS[1],
		createdAt: ago(4 * HOUR_MS),
		totalItems: 16
	}),
	request({
		id: 'R3',
		tmdbId: 103,
		requester: REQUESTERS[1],
		createdAt: ago(DAY_MS)
	})
];

/** The approved request behind Dune's "Requested by" chip. */
export const TITLE_REQUEST = request({
	id: 'R4',
	tmdbId: 104,
	titleId: MOVIE_ID,
	status: RequestStatus.APPROVED,
	createdAt: ago(DAY_MS)
});

const REQUEST_COUNTS = [
	{ status: RequestStatus.PENDING, count: REQUESTS.length },
	{ status: RequestStatus.APPROVED, count: 4 },
	{ status: RequestStatus.AVAILABLE, count: 12 },
	{ status: RequestStatus.DECLINED, count: 1 }
];

export function requestList(params: URLSearchParams): RequestListResponse {
	const titleId = params.get('titleId');
	const items = titleId ? [TITLE_REQUEST].filter((r) => r.titleId === titleId) : REQUESTS;
	return { items, total: items.length, counts: REQUEST_COUNTS };
}

export function requestById(id: string): RequestResponse | undefined {
	return [...REQUESTS, TITLE_REQUEST].find((r) => r.id === id);
}

const REQUESTED_MEDIA: Record<number, Pick<DiscoverDetailResponse, 'name' | 'year' | 'kind'>> = {
	101: { name: 'The Zone of Interest', year: 2023, kind: MediaKind.MOVIE },
	102: { name: 'Shōgun', year: 2024, kind: MediaKind.SERIES },
	103: { name: 'Anatomy of a Fall', year: 2023, kind: MediaKind.MOVIE },
	104: { name: 'Dune: Part Two', year: 2024, kind: MediaKind.MOVIE }
};

export const TRENDING: DiscoverResponse = {
	trending: true,
	items: [...MOVIES, ...SERIES].map((title, index) => ({
		tmdbId: index + 1,
		kind: title.kind,
		name: title.name,
		year: title.year,
		overview: '',
		inLibrary: true,
		titleId: title.id,
		posterUrl: poster(title.id)
	}))
};

export const WANTED_LANGUAGES: WantedLanguagesResponse = {
	available: [
		{ code: ENGLISH, name: 'English' },
		{ code: 'es', name: 'Spanish' }
	],
	languages: [{ language: ENGLISH, forced: false, hearingImpaired: false }]
};

export const EPISODE_GROUPS: EpisodeGroupResponse[] = [];

export function fileSubtitles(mediaFileId: string): FileSubtitlesResponse {
	return { mediaFile: mediaFileId, coverage: [], subtitles: [] };
}

export function discoverDetail(tmdbId: number): DiscoverDetailResponse {
	const media = REQUESTED_MEDIA[tmdbId];
	return {
		...media,
		tmdbId,
		overview: '',
		inLibrary: false,
		titleId: null,
		posterUrl: null,
		seasons: []
	};
}

const WEB_DL_2160: Quality = {
	id: 19,
	name: 'WEBDL-2160p',
	resolution: 2160,
	revision: 1,
	source: 'WEBDL'
};
const BLURAY_1080: Quality = {
	id: 7,
	name: 'Bluray-1080p',
	resolution: 1080,
	revision: 1,
	source: 'BLURAY'
};
const WEB_DL_1080: Quality = {
	id: 3,
	name: 'WEBDL-1080p',
	resolution: 1080,
	revision: 1,
	source: 'WEBDL'
};

function download(overrides: Partial<DownloadResponse>): DownloadResponse {
	return {
		id: 'D1',
		title: 'M2',
		titleName: 'Oppenheimer',
		releaseName: 'Oppenheimer.2023.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT',
		indexer: 'BeyondHD',
		infoHash: 'abc',
		episodes: [],
		state: DownloadState.DOWNLOADING,
		progress: 0.42,
		speedBps: 18.2 * MIB,
		etaSeconds: 240,
		ratio: 0.1,
		sizeBytes: 64 * GIB,
		seedingMode: SeedingMode.SEED_UNTIL_GOALS,
		quality: WEB_DL_2160,
		cfScore: 1500,
		override: false,
		grabbedAt: ago(10 * MINUTE_MS),
		trigger: GRAB_TRIGGER,
		...overrides
	};
}

export const DOWNLOADS: DownloadResponse[] = [
	download({}),
	download({
		id: 'D2',
		title: 'S1',
		titleName: 'Severance',
		releaseName: 'Severance.S02E03.Who.Is.Alive.1080p.ATVP.WEB-DL.DDP5.1.Atmos.H.264-FLUX',
		indexer: 'TorrentLeech',
		episodes: ['S1-S2E3'],
		progress: 0.71,
		speedBps: 12.4 * MIB,
		etaSeconds: 360,
		sizeBytes: 3.2 * GIB,
		quality: WEB_DL_1080
	}),
	download({
		id: 'D3',
		title: 'M6',
		titleName: 'The Holdovers',
		releaseName: 'The.Holdovers.2023.1080p.BluRay.x264-SPARKS',
		indexer: 'IPTorrents',
		state: DownloadState.SEEDING,
		progress: 1,
		speedBps: 0,
		etaSeconds: 0,
		ratio: 0.8,
		sizeBytes: 11 * GIB,
		quality: BLURAY_1080,
		completedAt: ago(DAY_MS)
	})
];

function device(name: string, ok: boolean): AgentResponse['devices'][number] {
	return {
		id: name,
		name,
		vendor: 'NVIDIA',
		driver: '560.35',
		healthy: true,
		maxSessions: 3,
		slots: 2,
		capabilities: [Capability.ENC_HEVC_10, Capability.ENC_AV1_10, Capability.WHISPER_VULKAN].map(
			(capability) => ({ capability, ok, error: ok ? '' : 'not supported', testedAt: ago(DAY_MS) })
		)
	};
}

export const AGENTS: AgentResponse[] = [
	{
		id: 'G1',
		hostname: 'studio-pc',
		version: '1.0.0',
		ffmpegVersion: '7.1',
		online: true,
		draining: false,
		lastSeenAt: NOW,
		lifetimeSavedBytes: 812 * GIB,
		mode: TransferMode.DOWNLOAD_FIRST,
		scratchFreeBytes: 420 * GIB,
		slotsOverride: null,
		throughputBps: 110 * MIB,
		devices: [device('RTX 4080', true)]
	},
	{
		id: 'G2',
		hostname: 'basement-gpu',
		version: '1.0.0',
		ffmpegVersion: '7.1',
		online: false,
		draining: false,
		lastSeenAt: ago(2 * HOUR_MS),
		lifetimeSavedBytes: 96 * GIB,
		mode: TransferMode.DIRECT_READ,
		scratchFreeBytes: 80 * GIB,
		slotsOverride: 1,
		throughputBps: 40 * MIB,
		devices: [device('GTX 1660', false)]
	}
];

function job(overrides: Partial<JobResponse>): JobResponse {
	return {
		id: 'J1',
		kind: JobKind.TRANSCODE,
		state: JobState.ENCODING,
		title: 'M1',
		titleName: 'Dune: Part Two',
		fileName: 'Dune Part Two (2024).mkv',
		episodes: '',
		mediaFile: 'F1',
		agent: 'G1',
		agentName: 'studio-pc',
		attempt: 1,
		maxAttempts: 3,
		error: '',
		finishedAt: null,
		notBefore: null,
		origin: JobOrigin.DOWNLOAD,
		prefetched: false,
		progress: 0.41,
		queuedAt: ago(HOUR_MS),
		targetCodec: 'HEVC',
		language: '',
		...overrides
	};
}

export const JOBS: JobListResponse = {
	backlog: 38,
	skipped: {},
	counts: {},
	jobs: [
		job({}),
		job({
			id: 'J2',
			state: JobState.QUEUED,
			title: 'M6',
			titleName: 'The Holdovers',
			fileName: 'The Holdovers (2023).mkv',
			agent: null,
			agentName: '',
			progress: 0
		})
	]
};

const PROFILE_1080 = 'P1';

export const PROFILES: ProfileSummaryResponse[] = [
	{ id: PROFILE_1080, name: '1080p', upgradeAllowed: true },
	{ id: 'P2', name: '2160p', upgradeAllowed: true }
];

function indexer(overrides: Partial<IndexerResponse>): IndexerResponse {
	return {
		id: 1,
		name: 'BeyondHD',
		status: IndexerStatus.OK,
		privacy: IndexerPrivacy.PRIVATE,
		averageResponseMs: 420,
		priority: 25,
		minSeeders: 1,
		lastError: '',
		seedingOverride: SeedingOverride.USE_GLOBAL,
		caps: { categories: [], movieParams: [], movieSearch: true, tvParams: [], tvSearch: true },
		...overrides
	};
}

export const INDEXERS: IndexerListResponse = {
	prowlarrUrl: 'http://prowlarr:9696',
	lastSyncedAt: ago(HOUR_MS),
	indexers: [
		indexer({}),
		indexer({ id: 2, name: 'TorrentLeech', averageResponseMs: 880 }),
		indexer({
			id: 3,
			name: 'IPTorrents',
			status: IndexerStatus.FAILING,
			lastError: 'HTTP 503 from the tracker',
			failingSince: ago(2 * HOUR_MS),
			averageResponseMs: 2400
		})
	]
};

export const INDEXER_OPTIONS: IndexerOptionsResponse = {
	resultCacheMinutes: 30,
	searchTimeoutSeconds: 30,
	syncIntervalMinutes: 60
};

export const NOTIFICATIONS: NotificationListResponse = {
	unreadCount: 1,
	notifications: [
		{
			id: 'N1',
			kind: NOTIFICATION_KIND,
			title: 'The Holdovers is available',
			body: '',
			link: '/titles/M6',
			read: false,
			createdAt: ago(2 * DAY_MS)
		}
	]
};

export const TIMELINE: TimelineEntryResponse[] = [
	{
		eventId: 'E3',
		at: ago(20 * MINUTE_MS),
		type: 'library.ItemAvailable',
		text: 'Available in Jellyfin',
		detail: '',
		episodes: [],
		severity: Severity.INFO
	},
	{
		eventId: 'E2',
		at: ago(2 * HOUR_MS),
		type: 'acquisition.ReleaseRejected',
		text: 'Rejected: blocked file inside the release',
		detail: 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.rar',
		episodes: [],
		severity: Severity.WARN
	},
	{
		eventId: 'E1',
		at: ago(DAY_MS),
		type: 'requests.RequestApproved',
		text: 'Requested by Maya Lin',
		detail: '',
		episodes: [],
		severity: Severity.INFO
	}
];

export const COVERAGE: TitleCoverageResponse[] = [
	{ title: 'S1', episodesWithFile: 11, episodesAvailable: 11, sizeBytes: 58 * GIB },
	{ title: 'S2', episodesWithFile: 6, episodesAvailable: 6, sizeBytes: 24 * GIB },
	{ title: 'S3', episodesWithFile: 2, episodesAvailable: 2, sizeBytes: 9 * GIB },
	{ title: 'S4', episodesWithFile: 9, episodesAvailable: 9, sizeBytes: 31 * GIB }
];

export function mediaFile(titleId: string): MediaFileResponse {
	return {
		id: `F-${titleId}`,
		title: titleId,
		available: true,
		episodes: [],
		importedAt: ago(DAY_MS),
		jellyfinOverdue: false,
		mediaInfo: {
			bitrateKbps: 18_000,
			container: 'mkv',
			durationSeconds: 9_960,
			hdr: 'HDR10',
			height: 2160,
			width: 3840,
			streams: []
		},
		origin: REQUEST_ORIGIN,
		path: `/media/movies/${titleId}.mkv`,
		quality: WEB_DL_2160,
		releaseName: 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.HDR.H.265-FLUX',
		rootFolder: '/media/movies',
		size: 21 * GIB,
		transcoded: false
	};
}

/** Movies with a file: the available ones. */
export const FILED_MOVIES = ['M1', 'M6'];

export function cutoffStatus(titleId: string): CutoffStatusResponse[] {
	return FILED_MOVIES.filter((id) => id === titleId).map((id) => ({
		file: mediaFile(id).id,
		quality: WEB_DL_2160,
		cfScore: 1800,
		cutoffMet: true,
		upgradesAllowed: true
	}));
}

function release(overrides: Partial<ReleaseDecision>): ReleaseDecision {
	return {
		release: 'X1',
		name: 'Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC.TrueHD.Atmos.7.1-FGT',
		indexer: 'BeyondHD',
		infoHash: 'x1',
		quality: WEB_DL_2160,
		cfScore: 2100,
		formats: ['DV', 'HDR10', 'Atmos'],
		freeleech: false,
		rank: 1,
		rejections: null,
		seeders: 128,
		sizeBytes: 78 * GIB,
		publishedAt: ago(DAY_MS),
		...overrides
	};
}

export const SEARCH_ID = 'SEARCH1';

export const SEARCH: SearchResponse = {
	searchId: SEARCH_ID,
	title: MOVIE_ID,
	trigger: SEARCH_TRIGGER_INTERACTIVE,
	startedAt: NOW,
	cancelled: false,
	completed: false,
	profile: {
		id: PROFILE_1080,
		name: '2160p',
		cutoff: 'WEBDL-2160p',
		penalizedFormats: ['LQ'],
		buckets: [
			{ name: 'WEBDL-2160p', qualityIds: [WEB_DL_2160.id], meetsCutoff: true },
			{ name: 'Bluray-1080p', qualityIds: [BLURAY_1080.id], meetsCutoff: false },
			{ name: 'WEBDL-1080p', qualityIds: [WEB_DL_1080.id], meetsCutoff: false }
		]
	},
	indexers: [
		{ name: 'BeyondHD', status: SEARCH_INDEXER.DONE, message: '' },
		{ name: 'TorrentLeech', status: SEARCH_INDEXER.DONE, message: '' },
		{ name: 'IPTorrents', status: SEARCH_INDEXER.FAILED, message: 'HTTP 503' },
		{ name: 'PassThePopcorn', status: SEARCH_INDEXER.PENDING, message: '' }
	],
	releases: [
		release({}),
		release({
			release: 'X2',
			name: 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX',
			indexer: 'TorrentLeech',
			cfScore: 1800,
			formats: ['DV', 'HDR10'],
			freeleech: true,
			rank: 2,
			seeders: 412,
			sizeBytes: 24 * GIB
		}),
		release({
			release: 'X3',
			name: 'Dune.Part.Two.2024.1080p.BluRay.x264-SPARKS',
			quality: BLURAY_1080,
			cfScore: 400,
			formats: ['x264'],
			rank: 3,
			seeders: 96,
			sizeBytes: 14 * GIB
		}),
		release({
			release: 'X4',
			name: 'Dune.Part.Two.2024.1080p.WEBRip.x265-YTS',
			quality: WEB_DL_1080,
			cfScore: -10000,
			formats: ['LQ'],
			rank: 4,
			seeders: 2048,
			sizeBytes: 2.1 * GIB,
			rejections: [
				{ reason: 'FORMAT_SCORE', message: 'Format score -10000 below minimum 0', temporary: false }
			]
		})
	]
};

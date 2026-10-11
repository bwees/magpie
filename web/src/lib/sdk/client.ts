/**
 * Magpie
 * 1.0.0
 * DO NOT MODIFY - This file has been generated using oazapfts.
 * See https://www.npmjs.com/package/oazapfts
 */
import * as Oazapfts from "@oazapfts/runtime";
import * as QS from "@oazapfts/runtime/query";
export const defaults: Oazapfts.Defaults<Oazapfts.CustomHeaders> = {
    headers: {},
    baseUrl: "/"
};
const oazapfts = Oazapfts.runtime(defaults);
export const servers = {};
export type IndexerOptionsResponse = {
    resultCacheMinutes: number;
    searchTimeoutSeconds: number;
    syncIntervalMinutes: number;
};
export type ErrorItem = {
    /** Additional information about the error */
    more?: {
        [key: string]: any;
    } | {
        [key: string]: any;
    };
    /** For example, name of the parameter that caused the error */
    name: string;
    /** Human readable error message */
    reason: string;
};
export type HttpError = {
    /** Human readable error message */
    detail?: string;
    errors?: ErrorItem[] | null;
    instance?: string;
    /** HTTP status code */
    status?: number;
    /** Short title of the error */
    title?: string;
    /** URL of the error type. Can be used to lookup the error in a documentation */
    "type"?: string;
};
export type IndexerOptionsRequest = {
    resultCacheMinutes: number;
    searchTimeoutSeconds: number;
    syncIntervalMinutes: number;
};
export type IndexerCapsResponse = {
    categories: number[];
    movieParams: string[];
    movieSearch: boolean;
    tvParams: string[];
    tvSearch: boolean;
};
export type IndexerResponse = {
    averageResponseMs: number;
    caps: IndexerCapsResponse;
    failingSince?: string;
    id: number;
    lastError: string;
    lastOkAt?: string;
    minSeeders: number;
    name: string;
    priority: number;
    privacy: string;
    seedDays?: number;
    seedRatio?: number;
    seedingOverride: string;
    status: string;
};
export type IndexerListResponse = {
    indexers: IndexerResponse[];
    lastSyncedAt?: string;
    prowlarrUrl: string;
    syncError?: string;
};
export type UpdateIndexerRequest = {
    minSeeders: number;
    priority: number;
    seedDays?: number;
    seedRatio?: number;
    seedingOverride: string;
};
export type AttentionItemResponse = {
    action: string;
    id: string;
    kind: string;
    openedAt: string;
    ref: string;
    text: string;
    titleId: string | null;
};
export type HistoryEntryResponse = {
    at: string;
    detail: string;
    episodes: string[];
    eventId: string;
    severity: string;
    text: string;
    titleId: string | null;
    titleName: string;
    "type": string;
};
export type SubsChipResponse = {
    missing: string[];
    satisfied: string[];
    state: string;
};
export type TranscodeChipResponse = {
    jobId: string | null;
    reason: string;
    savedPercent: number | null;
    state: string;
};
export type PipelineRowResponse = {
    detail: string;
    downloadId: string | null;
    episodeId: string | null;
    failed: boolean;
    stage: string;
    subs?: SubsChipResponse;
    titleId: string;
    transcode?: TranscodeChipResponse;
    updatedAt: string;
};
export type TimelineEntryResponse = {
    at: string;
    detail: string;
    episodes: string[];
    eventId: string;
    severity: string;
    text: string;
    "type": string;
};
export type AttributionResponse = {
    link: string;
    logoUrl: string;
    notice: string;
};
export type DiscoverItemResponse = {
    inLibrary: boolean;
    kind: string;
    name: string;
    overview: string;
    posterUrl: string | null;
    titleId: string | null;
    tmdbId: number;
    year: number;
};
export type DiscoverResponse = {
    items: DiscoverItemResponse[];
    trending: boolean;
};
export type String = string;
export type CatalogSettingsResponse = {
    language: string;
    releaseRegion: string;
};
export type UpdateCatalogSettingsRequest = {
    language: string;
    releaseRegion: string;
};
export type TitleImagesResponse = {
    backdrop: string | null;
    logo: string | null;
    poster: string | null;
};
export type ReleaseDatesResponse = {
    digital: string | null;
    physical: string | null;
    theatrical: string | null;
};
export type EpisodeResponse = {
    airDate: string | null;
    airedNumber: number;
    airedSeason: number;
    id: string;
    monitored: boolean;
    name: string;
    "number": number;
    released: boolean;
    runtimeMinutes: number;
    season: number;
};
export type SeasonResponse = {
    episodes: EpisodeResponse[];
    monitored: boolean;
    "number": number;
};
export type TitleResponse = {
    addedAt: string;
    id: string;
    images: TitleImagesResponse;
    imdbId: string | null;
    kind: string;
    monitorFutureSeasons: boolean;
    monitored: boolean;
    name: string;
    origin: string;
    originalLanguage: string;
    originalName: string;
    overview: string;
    profileId: string;
    releaseDates: ReleaseDatesResponse;
    released: boolean;
    releasedAt: string | null;
    requestedBy: string | null;
    runtimeMinutes: number;
    seasons: SeasonResponse[];
    seriesStatus: string;
    tmdbId: number;
    tvdbId: string | null;
    year: number;
};
export type RemoveTitleResponse = {
    deleteFiles: boolean;
    id: string;
};
export type UpdateTitleRequest = {
    monitorFutureSeasons?: boolean;
    monitored?: boolean;
    profileId?: string;
};
export type RefreshTitleResponse = {
    queued: boolean;
};
export type HealthResponse = {
    status: string;
};
export type ChangePasswordRequest = {
    currentPassword: string;
    newPassword: string;
};
export type PasswordChangedResponse = {
    changed: boolean;
};
export type CredentialsRequest = {
    password: string;
    username: string;
};
export type Permissions = {
    autoApproveMovies: boolean;
    autoApproveTv: boolean;
    reportIssues: boolean;
    viewOthersRequests: boolean;
};
export type MeResponse = {
    displayName: string;
    id: string;
    localAdmin: boolean;
    permissions: Permissions;
    role: string;
    username: string;
};
export type SignedOutResponse = {
    signedOut: boolean;
};
export type BackupResponse = {
    createdAt: string;
    kind: string;
    name: string;
    size: number;
};
export type RestoreResponse = {
    restarting: boolean;
};
export type ServiceHealthResponse = {
    checkedAt?: string;
    detail: string;
    error: string;
    service: string;
    status: string;
    version: string;
};
export type JellyfinConnection = {
    apiKey: string;
    url: string;
};
export type OpenSubtitlesConnection = {
    password: string;
    username: string;
};
export type ProwlarrConnection = {
    apiKey: string;
    url: string;
};
export type QBittorrentConnection = {
    apiKey: string;
    category: string;
    url: string;
};
export type TmdbConnection = {
    apiKey: string;
};
export type ConnectionsResponse = {
    health: ServiceHealthResponse[];
    jellyfin: JellyfinConnection;
    openSubtitles: OpenSubtitlesConnection;
    prowlarr: ProwlarrConnection;
    qbittorrent: QBittorrentConnection;
    tmdb: TmdbConnection;
};
export type UpdateConnectionsRequest = {
    jellyfin?: JellyfinConnection;
    openSubtitles?: OpenSubtitlesConnection;
    prowlarr?: ProwlarrConnection;
    qbittorrent?: QBittorrentConnection;
    tmdb?: TmdbConnection;
};
export type ParkedEventResponse = {
    aggregateId: string;
    attempts: number;
    eventId: string;
    eventType: string;
    handler: string;
    lastError: string;
    parkedAt: string;
};
export type RetryParkedEventRequest = {
    eventId: string;
    handler: string;
};
export type AgentCapabilityResponse = {
    capability: string;
    error: string;
    ok: boolean;
    testedAt: string;
};
export type AgentDeviceResponse = {
    capabilities: AgentCapabilityResponse[];
    driver: string;
    healthy: boolean;
    id: string;
    maxSessions: number;
    name: string;
    slots: number;
    vendor: string;
};
export type AgentResponse = {
    devices: AgentDeviceResponse[];
    draining: boolean;
    ffmpegVersion: string;
    hostname: string;
    id: string;
    lastSeenAt: string;
    lifetimeSavedBytes: number;
    mode: string;
    online: boolean;
    scratchFreeBytes: number;
    slotsOverride: number | null;
    throughputBps: number;
    version: string;
};
export type UpdateAgentRequest = {
    draining: boolean;
    mode: "DOWNLOAD_FIRST" | "DIRECT_READ";
    slotsOverride: number | null;
};
export type RetestResponse = {
    agent: string;
};
export type AgentSecretResponse = {
    configured: boolean;
    masked: string;
    secret: string;
};
export type TranscodeSettingsResponse = {
    defaultSlotsPerGpu: number;
    transferCap: number;
};
export type UpdateTranscodeSettingsRequest = {
    defaultSlotsPerGpu: number;
    transferCap: number;
};
export type AgentOffline = {
    agent: string;
    since: string;
};
export type AgentOnline = {
    agent: string;
    hostname: string;
};
export type AttentionChanged = {
    count: number;
    item: string;
    kind: string;
    open: boolean;
    ref: string;
};
export type CoverageSettled = {
    mediaFile: string;
    missing: string[] | null;
    satisfied: string[] | null;
    title: string;
};
export type Quality = {
    id: number;
    name: string;
    resolution: number;
    revision: number;
    source: string;
};
export type CutoffMet = {
    cfScore: number;
    episodes: string[] | null;
    file: string;
    origin: string;
    quality: Quality;
    title: string;
};
export type DownloadFile = {
    path: string;
    size: number;
};
export type DownloadCompleted = {
    cfScore: number;
    contentPath: string;
    download: string;
    episodes: string[] | null;
    files: DownloadFile[] | null;
    quality: Quality;
    releaseName: string;
    seedingMode: string;
    title: string;
};
export type DownloadFailed = {
    download: string;
    reason: string;
    title: string;
};
export type DownloadProgressed = {
    download: string;
    eta: number;
    progress: number;
    ratio: number;
    speedBps: number;
    state: string;
};
export type EpisodeMove = {
    episode: string;
    fromNo: number;
    fromSeason: number;
    toNo: number;
    toSeason: number;
};
export type EpisodeGroupChanged = {
    groupId: string;
    moves: EpisodeMove[] | null;
    title: string;
};
export type EpisodeRef = {
    episode: string;
    "number": number;
    season: number;
};
export type EpisodesAnnounced = {
    episodes: EpisodeRef[] | null;
    title: string;
};
export type ExternalIDs = {
    imdb?: string;
    tmdb: number;
    tvdb?: string;
};
export type FileDeleted = {
    episodes: EpisodeRef[] | null;
    mediaFile: string;
    path: string;
    reason: string;
    title: string;
};
export type Stream = {
    bitDepth: number;
    bitrateKbps: number;
    channels: number;
    codec: string;
    "default": boolean;
    forced: boolean;
    image: boolean;
    index: number;
    kind: string;
    language: string;
};
export type MediaInfo = {
    bitrateKbps: number;
    container: string;
    duration: number;
    hdr: string;
    height: number;
    streams: Stream[] | null;
    width: number;
};
export type Sidecar = {
    forced: boolean;
    language: string;
    path: string;
    sdh: boolean;
};
export type FileImported = {
    download: string | null;
    episodes: EpisodeRef[] | null;
    mediaFile: string;
    mediaInfo: MediaInfo;
    origin: string;
    osHash: string;
    path: string;
    quality: Quality;
    releaseName: string;
    replaces: string | null;
    sidecars: Sidecar[] | null;
    size: number;
    title: string;
};
export type FileTranscoded = {
    mediaFile: string;
    newSize: number;
    oldSize: number;
    path: string;
};
export type FolderFound = {
    kind: string;
    parsedTitle: string;
    path: string;
    tmdbHint: number | null;
    year: number | null;
};
export type ImportFailed = {
    download: string;
    reason: string;
    retryable: boolean;
};
export type IssueActionRequested = {
    action: string;
    episode: string | null;
    issue: string;
    title: string;
};
export type IssueCommented = {
    author: string;
    issue: string;
    reporter: string;
};
export type IssueReported = {
    episode: string | null;
    issue: string;
    kind: string;
    title: string;
    user: string;
};
export type IssueResolved = {
    by: string;
    issue: string;
    reporter: string;
};
export type ItemAvailable = {
    episodes: string[] | null;
    jellyfinItem: string;
    title: string;
};
export type JobFailed = {
    final: boolean;
    job: string;
    kind: string;
    mediaFile: string;
    reason: string;
};
export type JobProgressed = {
    eta: number;
    job: string;
    mediaFile: string;
    progress: number;
    speed: number;
    stage: string;
};
export type JobQueued = {
    job: string;
    kind: string;
    mediaFile: string;
    origin: string;
    title: string;
};
export type JobStarted = {
    agent: string;
    attempt: number;
    job: string;
    kind: string;
    mediaFile: string;
};
export type WantedLanguage = {
    forced: boolean;
    hearingImpaired: boolean;
    language: string;
};
export type LanguagesChanged = {
    languages: WantedLanguage[] | null;
    title: string | null;
};
export type MonitoringChanged = {
    added: string[] | null;
    movie: boolean | null;
    removed: string[] | null;
    requestId: string | null;
    title: string;
};
export type NotificationCreated = {
    kind: string;
    link: string;
    notification: string;
    title: string;
    user: string;
};
export type ProfileChanged = {
    "from": string;
    title: string;
    to: string;
};
export type ProjectionUpdated = {
    projection: string;
    title: string | null;
};
export type Rejection = {
    message: string;
    reason: string;
    temporary: boolean;
};
export type ReleaseDecision = {
    cfScore: number;
    formats: string[] | null;
    freeleech: boolean;
    indexer: string;
    infoHash: string;
    name: string;
    publishedAt: string;
    quality: Quality;
    rank: number;
    rejections: Rejection[] | null;
    release: string;
    seeders: number;
    sizeBytes: number;
};
export type ReleaseGrabbed = {
    cfScore: number;
    download: string;
    episodes: string[] | null;
    indexer: string;
    infoHash: string;
    "override": boolean;
    quality: Quality;
    release: string;
    title: string;
    trigger: string;
};
export type ReleaseRejected = {
    blockedFile: string | null;
    indexer: string;
    infoHash: string;
    next: string | null;
    reason: string;
    release: string;
    title: string;
};
export type RequestApproved = {
    approvedBy: string;
    auto: boolean;
    kind: string;
    profile: string;
    request: string;
    scope: string;
    seasons: number[] | null;
    tmdb: number;
    user: string;
};
export type RequestAvailable = {
    request: string;
    title: string;
    user: string;
};
export type RequestCreated = {
    kind: string;
    request: string;
    scope: string;
    seasons: number[] | null;
    tmdb: number;
    user: string;
};
export type RequestDeclined = {
    reason: string;
    request: string;
    user: string;
};
export type RunAgentJob = {
    kind: string;
    language: string;
    mediaFile: string;
    model: string;
};
export type ScanFolderMatched = {
    folder: string;
    title: string;
};
export type SearchCompleted = {
    accepted: number;
    failedIndexers: string[] | null;
    grabbed: string | null;
    outcome: string;
    search: string;
    title: string;
    total: number;
};
export type SearchResultsReceived = {
    indexer: string;
    message: string;
    releases: ReleaseDecision[] | null;
    search: string;
    status: string;
};
export type SearchStarted = {
    episodes: string[] | null;
    indexers: string[] | null;
    interactive: boolean;
    search: string;
    title: string;
    trigger: string;
};
export type ServiceHealthChanged = {
    error: string;
    healthy: boolean;
    service: string;
};
export type SubtitleAdded = {
    forced: boolean;
    language: string;
    mediaFile: string;
    replaces: boolean;
    sdh: boolean;
    source: string;
    synced: boolean;
};
export type SubtitleMissing = {
    forced: boolean;
    language: string;
    mediaFile: string;
    reason: string;
};
export type TitleAddFailed = {
    kind: string;
    reason: string;
    requestId: string | null;
    tmdb: number;
};
export type TitleAdded = {
    external: ExternalIDs;
    kind: string;
    monitored: string[] | null;
    origin: string;
    profile: string;
    released: boolean;
    requestId: string | null;
    requestedBy: string | null;
    scanFolder: string;
    title: string;
};
export type TitleRemoved = {
    deleteFiles: boolean;
    title: string;
};
export type TranscodeSkipped = {
    detail: string;
    mediaFile: string;
    reason: string;
};
export type TranscodeStats = {
    agent: string;
    bitDepth: number;
    droppedAudio: number[] | null;
    droppedSubs: number[] | null;
    durationSec: number;
    encoder: string;
    newSize: number;
    oldSize: number;
    videoCodec: string;
    vmafMean: number | null;
};
export type TranscodeSwapRejected = {
    job: string;
    mediaFile: string;
    reason: string;
};
export type TranscodeVerified = {
    job: string;
    mediaFile: string;
    sha256: string;
    sourceETag: string;
    stagedPath: string;
    stats: TranscodeStats;
};
export type UserCreated = {
    role: string;
    source: string;
    user: string;
};
export type WantedReleased = {
    episodes: string[] | null;
    title: string;
};
export type WhisperCompleted = {
    job: string;
    language: string;
    mediaFile: string;
    stagedPath: string;
};
export type UnknownInterface = any;
/**
 * Sync interval, search timeout and result cache for all indexers (admin)
 */
export function getIndexerOptions({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: IndexerOptionsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/indexer-options", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Change the options that apply to all indexers (admin)
 */
export function setIndexerOptions(indexerOptionsRequest: IndexerOptionsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: IndexerOptionsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/indexer-options", oazapfts.json({
        ...opts,
        method: "PUT",
        body: indexerOptionsRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * List Prowlarr's indexers with Magpie's settings (admin)
 */
export function listIndexers({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: IndexerListResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/indexers", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Pull the indexer list from Prowlarr now (admin)
 */
export function syncIndexers({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: IndexerListResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/indexers/sync", {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Replace Magpie's settings for one indexer (admin)
 */
export function updateIndexer(id: number, updateIndexerRequest: UpdateIndexerRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: IndexerResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/indexers/${encodeURIComponent(id)}`, oazapfts.json({
        ...opts,
        method: "PUT",
        body: updateIndexerRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Open attention items, oldest first (admin)
 */
export function getAttention({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: AttentionItemResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/activity/attention", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * The global history, newest first (admin)
 */
export function getHistory({ severity, before, limit, accept }: {
    severity?: string;
    before?: string;
    limit?: number;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: HistoryEntryResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/activity/history${QS.query(QS.explode({
        severity,
        before,
        limit
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Pipeline rows: one title's rows, each listed title's summary row, or the active rows
 */
export function getPipeline({ title, titles, active, accept }: {
    title?: string;
    titles?: string[];
    active?: boolean;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: PipelineRowResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/activity/pipeline${QS.query(QS.explode({
        title,
        titles,
        active
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * A title's timeline, newest first
 */
export function getTimeline(title: string, { before, limit, accept }: {
    before?: string;
    limit?: number;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: TimelineEntryResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/activity/timeline${QS.query(QS.explode({
        title,
        before,
        limit
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * TMDB logo and notice for the About page
 */
export function getAttribution({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: AttributionResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/catalog/attribution", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Search TMDB, or list this week's trending titles for an empty query
 */
export function discoverTitles({ q, accept }: {
    q?: string;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: DiscoverResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/catalog/discover${QS.query(QS.explode({
        q
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Serve a cached poster, backdrop or logo, resized to the width asked for
 */
export function getTitleImage(titleId: string, kind: string, { w }: {
    w?: number;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: String;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/catalog/images/${encodeURIComponent(titleId)}/${encodeURIComponent(kind)}${QS.query(QS.explode({
        w
    }))}`, {
        ...opts
    }));
}
/**
 * Get the metadata language and release region
 */
export function getCatalogSettings({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: CatalogSettingsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/catalog/settings", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Set the metadata language and release region (admin)
 */
export function updateCatalogSettings(updateCatalogSettingsRequest: UpdateCatalogSettingsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: CatalogSettingsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/catalog/settings", oazapfts.json({
        ...opts,
        method: "PUT",
        body: updateCatalogSettingsRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * List Titles in the library
 */
export function listTitles({ q, kind, monitored, released, sort, accept }: {
    q?: string;
    kind?: string;
    monitored?: boolean;
    released?: boolean;
    sort?: string;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: TitleResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/catalog/titles${QS.query(QS.explode({
        q,
        kind,
        monitored,
        released,
        sort
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Remove a Title from Magpie (admin)
 */
export function removeTitle(id: string, { deleteFiles, accept }: {
    deleteFiles?: boolean;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: RemoveTitleResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/catalog/titles/${encodeURIComponent(id)}${QS.query(QS.explode({
        deleteFiles
    }))}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Get one Title with its seasons and episodes
 */
export function getTitle(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: TitleResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/catalog/titles/${encodeURIComponent(id)}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Change a Title's profile or monitoring (admin)
 */
export function updateTitle(id: string, updateTitleRequest: UpdateTitleRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: TitleResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/catalog/titles/${encodeURIComponent(id)}`, oazapfts.json({
        ...opts,
        method: "PATCH",
        body: updateTitleRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Refresh a Title's metadata and images from TMDB now (admin)
 */
export function refreshTitle(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: RefreshTitleResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/catalog/titles/${encodeURIComponent(id)}/refresh`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Report that the server is up
 */
export function getHealth({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: HealthResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/health", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Change the local admin's password (local admin)
 */
export function changeLocalAdminPassword(changePasswordRequest: ChangePasswordRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: PasswordChangedResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/local-admin/password", oazapfts.json({
        ...opts,
        method: "PUT",
        body: changePasswordRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Sign in as the local admin
 */
export function signInLocal(credentialsRequest: CredentialsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MeResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/login/local", oazapfts.json({
        ...opts,
        method: "POST",
        body: credentialsRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Delete the current session
 */
export function signOut({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: SignedOutResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/logout", {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * The signed-in user and their permissions
 */
export function getMe({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MeResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/me", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Create the local admin while no admin exists, and sign them in
 */
export function setupAdmin(credentialsRequest: CredentialsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MeResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/setup/admin", oazapfts.json({
        ...opts,
        method: "POST",
        body: credentialsRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * List database backups, newest first (admin)
 */
export function listBackups({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: BackupResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/backups", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Back up the database now (admin)
 */
export function createBackup({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 201;
        data: BackupResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/backups", {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Restore a backup and restart the server (admin)
 */
export function restoreBackup(name: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: RestoreResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/platform/backups/${encodeURIComponent(name)}/restore`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * External service connections with masked secrets and health (admin)
 */
export function getConnections({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ConnectionsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/connections", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Change the connections named in the body and re-check them (admin)
 */
export function updateConnections(updateConnectionsRequest: UpdateConnectionsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ConnectionsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/connections", oazapfts.json({
        ...opts,
        method: "PUT",
        body: updateConnectionsRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Probe one stored connection now (admin)
 */
export function testConnection(service: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ServiceHealthResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/platform/connections/${encodeURIComponent(service)}/test`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * List events a handler gave up on (admin)
 */
export function listParkedEvents({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ParkedEventResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/parked-events", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Re-queue a parked event for its handler (admin)
 */
export function retryParkedEvent(retryParkedEventRequest: RetryParkedEventRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ParkedEventResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/parked-events/retry", oazapfts.json({
        ...opts,
        method: "POST",
        body: retryParkedEventRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Agents with devices and tested capabilities, by hostname (admin)
 */
export function listAgents({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: AgentResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/transcode/agents", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Set an agent's slots override, transfer mode and draining (admin)
 */
export function updateAgent(id: string, updateAgentRequest: UpdateAgentRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: AgentResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/transcode/agents/${encodeURIComponent(id)}`, oazapfts.json({
        ...opts,
        method: "PATCH",
        body: updateAgentRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Ask a connected agent to re-run its capability tests (admin)
 */
export function retestAgent(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: RetestResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/transcode/agents/${encodeURIComponent(id)}/retest`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * The agent shared secret from MAGPIE_AGENT_SECRET, masked and for copying (admin)
 */
export function getAgentSecret({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: AgentSecretResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/transcode/secret", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Transcode settings (admin)
 */
export function getTranscodeSettings({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: TranscodeSettingsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/transcode/settings", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Update transcode settings (admin)
 */
export function updateTranscodeSettings(updateTranscodeSettingsRequest: UpdateTranscodeSettingsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: TranscodeSettingsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/transcode/settings", oazapfts.json({
        ...opts,
        method: "PUT",
        body: updateTranscodeSettingsRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}

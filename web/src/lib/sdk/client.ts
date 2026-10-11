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
export type HealthResponse = {
    status: string;
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

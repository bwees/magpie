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
export type BlocklistEntryResponse = {
    at: string;
    expiresAt?: string;
    id: string;
    indexer: string;
    infoHash: string;
    reason: string;
    releaseName: string;
    title: string;
    titleName: string;
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
export type UnknownInterface = any;
export type FormatSpecDto = {
    kind: string;
    max: number;
    min: number;
    name: string;
    negate: boolean;
    required: boolean;
    value: string;
};
export type CustomFormatResponse = {
    id: string;
    includeWhenRenaming: boolean;
    name: string;
    specs: FormatSpecDto[];
    trashId?: string;
};
export type CustomFormatRequest = {
    includeWhenRenaming: boolean;
    name: string;
    specs: FormatSpecDto[];
};
export type BlockTermRequest = {
    term: string;
};
export type Quality = {
    id: number;
    name: string;
    resolution: number;
    revision: number;
    source: string;
};
export type CutoffStatusResponse = {
    cfScore: number;
    cutoffMet: boolean;
    file: string;
    quality: Quality;
    upgradesAllowed: boolean;
};
export type DownloadOptionsResponse = {
    blockedFilesEnabled: boolean;
    blockedPatterns: string[];
    metadataTimeoutMinutes: number;
    seedDays: number;
    seedRatio: number;
    seedingMode: string;
    stalledLimitMinutes: number;
};
export type DownloadOptionsRequest = {
    blockedFilesEnabled: boolean;
    blockedPatterns: string[];
    metadataTimeoutMinutes: number;
    seedDays: number;
    seedRatio: number;
    seedingMode: string;
    stalledLimitMinutes: number;
};
export type DownloadResponse = {
    cfScore: number;
    completedAt?: string;
    episodes: string[];
    etaSeconds: number;
    grabbedAt: string;
    id: string;
    indexer: string;
    infoHash: string;
    "override": boolean;
    progress: number;
    quality: Quality;
    ratio: number;
    releaseName: string;
    search?: string;
    seedingMode: string;
    sizeBytes: number;
    speedBps: number;
    state: string;
    title: string;
    titleName: string;
    trigger: string;
};
export type IndexerOptionsResponse = {
    resultCacheMinutes: number;
    searchTimeoutSeconds: number;
    syncIntervalMinutes: number;
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
export type ProfileSummaryResponse = {
    id: string;
    name: string;
    preset?: string;
    trashProfileId?: string;
    upgradeAllowed: boolean;
};
export type FormatScoreRequest = {
    formatId: string;
    score: number;
};
export type ProfileItemRequest = {
    allowed: boolean;
    groupName?: string;
    maxMbPerMin: number;
    minMbPerMin: number;
    preferredMbPerMin: number;
    qualityIds: number[];
};
export type ProfileRequest = {
    cutoff: number;
    cutoffFormatScore: number;
    formatScores: FormatScoreRequest[];
    items: ProfileItemRequest[];
    minFormatScore: number;
    minUpgradeFormatScore: number;
    name: string;
    propersMode: string;
    upgradeAllowed: boolean;
};
export type FormatScoreResponse = {
    formatId: string;
    name: string;
    score: number;
    userOverride: boolean;
};
export type QualityGroupResponse = {
    id: number;
    name: string;
};
export type QualityResponse = {
    id: number;
    name: string;
    resolution: number;
    source: string;
};
export type ProfileItemResponse = {
    allowed: boolean;
    group?: QualityGroupResponse;
    maxMbPerMin: number;
    minMbPerMin: number;
    preferredMbPerMin: number;
    qualities: QualityResponse[];
};
export type ProfileResponse = {
    cutoff: number;
    cutoffFormatScore: number;
    formatScores: FormatScoreResponse[];
    id: string;
    items: ProfileItemResponse[];
    minFormatScore: number;
    minUpgradeFormatScore: number;
    name: string;
    preset?: string;
    propersMode: string;
    trashProfileId?: string;
    trashScoreSet?: string;
    upgradeAllowed: boolean;
};
export type FromPresetRequest = {
    preset: string;
};
export type SchedulesResponse = {
    cutoffSweepDays: number;
    missingSweepDays: number;
    pendingCheckMinutes: number;
    rssIntervalMinutes: number;
};
export type SchedulesRequest = {
    cutoffSweepDays: number;
    missingSweepDays: number;
    rssIntervalMinutes: number;
};
export type SearchOptionsResponse = {
    searchOnAdd: boolean;
};
export type SearchOptionsRequest = {
    searchOnAdd: boolean;
};
export type StartSearchRequest = {
    episodes?: string[] | null;
    interactive: boolean;
    title: string;
};
export type SearchIndexerResponse = {
    message: string;
    name: string;
    status: string;
};
export type QualityBucketResponse = {
    meetsCutoff: boolean;
    name: string;
    qualityIds: number[];
};
export type SearchProfileResponse = {
    buckets: QualityBucketResponse[];
    cutoff: string;
    id: string;
    name: string;
    penalizedFormats: string[];
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
export type SearchResponse = {
    cancelled: boolean;
    completed: boolean;
    indexers: SearchIndexerResponse[];
    profile: SearchProfileResponse;
    releases: ReleaseDecision[];
    searchId: string;
    startedAt: string;
    title: string;
    trigger: string;
};
export type GrabRequest = {
    "override": boolean;
    releaseId: string;
};
export type GrabResponse = {
    blockedFile?: string;
    download?: string;
    next?: string;
    status: string;
};
export type AcquisitionSettingsResponse = {
    defaultDelayMinutes: number;
    defaultProfileId: string;
    settleWindowSeconds: number;
    trashSyncEnabled: boolean;
};
export type AcquisitionSettingsRequest = {
    defaultDelayMinutes: number;
    defaultProfileId: string;
    settleWindowSeconds: number;
    trashSyncEnabled: boolean;
};
export type TrashProfileResponse = {
    app: string;
    cutoff: string;
    formatCount: number;
    name: string;
    qualities: string[];
    scoreSet: string;
    syncedAt: string;
    trashId: string;
};
export type TrashStatusResponse = {
    enabled: boolean;
    lastAttemptAt?: string;
    lastError?: string;
    lastSyncedAt?: string;
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
export type NotificationKindResponse = {
    kind: string;
    label: string;
    optIn: boolean;
    scope: string;
};
export type NotificationUrlResponse = {
    createdAt: string;
    id: string;
    kinds: string[];
    label: string;
    lastError: string;
    lastSentAt: string | null;
    maskedUrl: string;
};
export type NotificationUrlRequest = {
    kinds: string[] | null;
    label: string;
    url: string;
};
export type NotificationUrlChangeRequest = {
    kinds: string[] | null;
    label?: string;
};
export type NotificationResponse = {
    body: string;
    createdAt: string;
    id: string;
    kind: string;
    link: string;
    read: boolean;
    title: string;
};
export type NotificationListResponse = {
    notifications: NotificationResponse[];
    unreadCount: number;
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
    "number": number;
    season: number;
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
export type DiscoverSeasonResponse = {
    episodeCount: number;
    monitored: boolean;
    "number": number;
};
export type DiscoverDetailResponse = {
    inLibrary: boolean;
    kind: string;
    name: string;
    overview: string;
    posterUrl: string | null;
    seasons: DiscoverSeasonResponse[];
    titleId: string | null;
    tmdbId: number;
    year: number;
};
export type String = string;
export type LanguageOptionResponse = {
    code: string;
    name: string;
};
export type WantedLanguageEntry = {
    forced: boolean;
    hearingImpaired: boolean;
    language: string;
};
export type WantedLanguagesResponse = {
    available: LanguageOptionResponse[];
    languages: WantedLanguageEntry[];
};
export type UpdateWantedLanguagesRequest = {
    languages: WantedLanguageEntry[];
};
export type ScanCandidateResponse = {
    kind: string;
    name: string;
    overview: string;
    posterUrl: string | null;
    score: number;
    tmdbId: number;
    year: number;
};
export type ScanReviewResponse = {
    candidates: ScanCandidateResponse[];
    createdAt: string;
    folderPath: string;
    id: string;
    kind: string;
    parsedTitle: string;
    parsedYear: number | null;
    state: string;
    titleId: string | null;
    tmdbHint: number | null;
};
export type MatchScanReviewRequest = {
    tmdbId: number;
};
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
export type NextEpisodeResponse = {
    airDate: string;
    "number": number;
    season: number;
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
    episodeGroupId: string;
    id: string;
    images: TitleImagesResponse;
    imdbId: string | null;
    kind: string;
    languagesOverride: WantedLanguageEntry[] | null;
    monitorFutureSeasons: boolean;
    monitored: boolean;
    name: string;
    nextEpisode?: NextEpisodeResponse;
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
    seasonCount: number;
    seasons: SeasonResponse[];
    seriesStatus: string;
    tmdbId: number;
    tvdbId: string | null;
    wantedEpisodeCount: number;
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
export type SelectEpisodeGroupRequest = {
    groupId: string;
};
export type EpisodeGroupResponse = {
    episodeCount: number;
    groupCount: number;
    id: string;
    name: string;
    "type": string;
};
export type UpdateMonitoredRequest = {
    monitored: boolean;
};
export type UpdateTitleLanguagesRequest = {
    languages: WantedLanguageEntry[] | null;
};
export type RefreshTitleResponse = {
    queued: boolean;
};
export type HealthResponse = {
    status: string;
};
export type Permissions = {
    autoApproveMovies: boolean;
    autoApproveTv: boolean;
    reportIssues: boolean;
    viewOthersRequests: boolean;
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
export type MeResponse = {
    displayName: string;
    id: string;
    localAdmin: boolean;
    permissions: Permissions;
    role: string;
    username: string;
};
export type SignInMethodsResponse = {
    jellyfin: boolean;
    jellyfinReachable: boolean;
    quickConnect: boolean;
};
export type QuickConnectStartResponse = {
    code: string;
    expiresAt: string;
    handle: string;
};
export type QuickConnectPollResponse = {
    status: string;
    user?: MeResponse;
};
export type SignedOutResponse = {
    signedOut: boolean;
};
export type SignInSettings = {
    allowJellyfin: boolean;
};
export type UserResponse = {
    createdAt: string;
    disabled: boolean;
    displayName: string;
    id: string;
    lastLoginAt?: string;
    localAdmin: boolean;
    permissions: Permissions;
    role: string;
    source: string;
    username: string;
};
export type UpdateUserRequest = {
    autoApproveMovies?: boolean;
    autoApproveTv?: boolean;
    disabled?: boolean;
    reportIssues?: boolean;
    role?: string;
    viewOthersRequests?: boolean;
};
export type TitleCoverageResponse = {
    episodesAvailable: number;
    episodesWithFile: number;
    sizeBytes: number;
    title: string;
};
export type EpisodeRefResponse = {
    episode: string;
    "number": number;
    season: number;
};
export type StreamResponse = {
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
export type MediaInfoResponse = {
    bitrateKbps: number;
    container: string;
    durationSeconds: number;
    hdr: string;
    height: number;
    streams: StreamResponse[];
    width: number;
};
export type MediaFileResponse = {
    available: boolean;
    download?: string;
    episodes: EpisodeRefResponse[];
    id: string;
    importedAt: string;
    mediaInfo: MediaInfoResponse;
    origin: string;
    path: string;
    quality: Quality;
    releaseName: string;
    renameProblem?: string;
    rootFolder: string;
    size: number;
    title: string;
    transcoded: boolean;
};
export type RootFolderResponse = {
    id: string;
    kind: string;
    link: string;
    path: string;
};
export type HardlinkCheckResponse = {
    downloadPath: string;
    problem?: string;
    rootFolders: RootFolderResponse[];
};
export type ImportEpisodeResponse = {
    id: string;
    name: string;
    "number": number;
    season: number;
};
export type ImportFileResponse = {
    episodes: EpisodeRefResponse[];
    kind: string;
    path: string;
    sample: boolean;
    size: number;
};
export type ImportResponse = {
    attempts: number;
    download: string;
    episodes: ImportEpisodeResponse[];
    files: ImportFileResponse[];
    kind: string;
    reason: string;
    releaseName: string;
    status: string;
    title: string;
    titleName: string;
    updatedAt: string;
};
export type FileMappingRequest = {
    episodes: string[];
    path: string;
};
export type ManualImportRequest = {
    files: FileMappingRequest[];
};
export type AddRootFolderRequest = {
    kind: string;
    path: string;
};
export type LibraryScanResponse = {
    error: string;
    finishedAt: string | null;
    found: number;
    ignored: number;
    linked: number;
    renameMatched: boolean;
    reported: number;
    running: boolean;
    startedAt: string | null;
};
export type ScanOptionsRequest = {
    renameMatched: boolean;
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
    localPath?: string;
    remotePath?: string;
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
export type MigrationOptions = {
    importAliases: boolean;
    renameFolders: boolean;
};
export type MigrationPath = {
    access: string;
    app: string;
    "from": string;
    to: string;
};
export type MigrationProgress = {
    done: number;
    step: string;
    total: number;
};
export type MigrationAppReport = {
    app: string;
    matched: number;
    review: number;
    skipped: number;
};
export type MigrationReport = {
    apps: MigrationAppReport[];
    blocklistWithHash: number;
    blocklistWithoutHash: number;
    files: number;
    formatClashes: number;
    formats: number;
    history: number;
    missingFiles: number;
    numberingMismatches: number;
    profileClashes: number;
    profiles: number;
    releaseProfileFormats: number;
    unmappedEpisodes: number;
    untranslatedSpecs: number;
    warnings: string[];
};
export type MigrationClash = {
    choice: string;
    id: string;
    kind: string;
    name: string;
    radarr: string[];
    sonarr: string[];
};
export type MigrationGroupOption = {
    id: string;
    name: string;
    score: number;
    "type": number;
};
export type MigrationSeasonCount = {
    episodes: number;
    season: number;
};
export type MigrationNumberingReview = {
    decision: string;
    group: string;
    groups: MigrationGroupOption[];
    id: string;
    name: string;
    sonarrSeasons: MigrationSeasonCount[];
    suggested: string;
    tmdbSeasons: MigrationSeasonCount[];
};
export type MigrationQueueCount = {
    pending: number;
    queue: string;
    total: number;
};
export type MigrationSeriesTypeReview = {
    decision: string;
    id: string;
    name: string;
    "type": string;
};
export type MigrationSpecReview = {
    app: string;
    decision: string;
    formatName: string;
    id: string;
    kind: string;
    options: string[];
    rawValue: string;
    reason: string;
    specName: string;
    value: string;
};
export type MigrationCandidate = {
    kind: string;
    name: string;
    tmdb: number;
    year: number;
};
export type MigrationTitleReview = {
    app: string;
    candidates: MigrationCandidate[];
    decision: string;
    id: string;
    kind: string;
    name: string;
    tmdb: number[];
    year: number;
};
export type MigrationReviews = {
    clashes: MigrationClash[];
    numbering: MigrationNumberingReview[];
    queues: MigrationQueueCount[];
    seriesTypes: MigrationSeriesTypeReview[];
    specs: MigrationSpecReview[];
    titles: MigrationTitleReview[];
};
export type MigrationSource = {
    app: string;
    rootFolders: string[];
    skipped: boolean;
    url: string;
    version: string;
};
export type MigrationSummaryStep = {
    imported: number;
    step: string;
};
export type MigrationSummary = {
    missingFiles: string[];
    skippedTitles: string[];
    steps: MigrationSummaryStep[];
    unmappedFiles: string[];
};
export type MigrationResponse = {
    canCommit: boolean;
    cutoverConfirmed: boolean;
    error: string;
    failedStep: string;
    options: MigrationOptions;
    paths: MigrationPath[];
    phase: string;
    progress: MigrationProgress;
    report?: MigrationReport;
    reviews?: MigrationReviews;
    sources: MigrationSource[];
    summary?: MigrationSummary;
};
export type MigrationConnectRequest = {
    apiKey: string;
    app: string;
    skip: boolean;
    url: string;
};
export type MigrationCutoverRequest = {
    confirmed: boolean;
};
export type MigrationOptionsRequest = {
    importAliases: boolean;
    renameFolders: boolean;
};
export type MigrationPathRequest = {
    app: string;
    "from": string;
    to: string;
};
export type MigrationPathsRequest = {
    paths: MigrationPathRequest[];
};
export type MigrationPathCheckRequest = {
    path: string;
};
export type MigrationPathCheck = {
    access: string;
    path: string;
};
export type MigrationDecisionRequest = {
    clash?: string;
    group?: string;
    id: string;
    numbering?: string;
    queue: string;
    seriesType?: string;
    spec?: string;
    title?: string;
    tmdb?: number[] | null;
    value?: string;
};
export type MigrationSkipQueueRequest = {
    queue: string;
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
export type StatusCountResponse = {
    count: number;
    status: string;
};
export type RequesterResponse = {
    displayName: string;
    id: string;
};
export type RequestResponse = {
    availableItems: number;
    createdAt: string;
    decidedAt: string | null;
    decidedBy: string | null;
    declineReason: string;
    failureReason: string;
    id: string;
    kind: string;
    profileId: string;
    requester: RequesterResponse;
    scope: string;
    seasons: number[];
    status: string;
    titleId: string | null;
    tmdbId: number;
    totalItems: number;
};
export type RequestListResponse = {
    counts: StatusCountResponse[];
    items: RequestResponse[];
    total: number;
};
export type CreateRequestRequest = {
    kind: string;
    scope: string;
    seasons?: number[] | null;
    tmdbId: number;
};
export type IssueEpisodeResponse = {
    id: string;
    name: string;
    "number": number;
    season: number;
};
export type IssueResponse = {
    action: string;
    comments: number;
    createdAt: string;
    episode?: IssueEpisodeResponse;
    episodeId: string | null;
    id: string;
    kind: string;
    reporter: RequesterResponse;
    resolvedAt: string | null;
    resolvedBy: string;
    status: string;
    text: string;
    titleId: string;
    titleName: string;
};
export type IssueListResponse = {
    items: IssueResponse[];
    total: number;
};
export type ReportIssueRequest = {
    episodeId?: string;
    kind: string;
    text: string;
    titleId: string;
};
export type IssueCommentResponse = {
    author: RequesterResponse;
    body: string;
    createdAt: string;
    id: string;
};
export type IssueThreadResponse = {
    comments: IssueCommentResponse[];
    issue: IssueResponse;
};
export type IssueActionRequest = {
    action: string;
    waitingRemoved?: boolean;
};
export type CommentIssueRequest = {
    body: string;
};
export type ApproveRequestRequest = {
    profileId?: string;
};
export type DeclineRequestRequest = {
    reason?: string;
};
export type CoverageResponse = {
    forced: boolean;
    language: string;
    nextAttemptAt?: string;
    reason?: string;
    source?: string;
    status: string;
    waitingForQuota: boolean;
    waitingForWhisper: boolean;
};
export type SubtitleResponse = {
    addedAt: string;
    forced: boolean;
    id: string;
    language: string;
    path: string;
    releaseName: string;
    score?: number;
    sdh: boolean;
    source: string;
    syncOffsetMs?: number;
    synced: boolean;
    upgradeUntil?: string;
};
export type FileSubtitlesResponse = {
    coverage: CoverageResponse[];
    mediaFile: string;
    subtitles: SubtitleResponse[];
};
export type CandidateResponse = {
    acceptable: boolean;
    aiTranslated: boolean;
    fileId: number;
    forced: boolean;
    hashMatch: boolean;
    hearingImpaired: boolean;
    language: string;
    maxPoints: number;
    points: number;
    releaseName: string;
    score: number;
};
export type DownloadSubtitleRequest = {
    fileId: number;
    forced: boolean;
    language: string;
};
export type QuotaResponse = {
    pausedUntil: string | null;
    problem: string;
    remaining: number | null;
    resetAt: string | null;
    vip: boolean;
};
export type SubtitleSettingsBody = {
    autoSync: boolean;
    minScoreEpisode: number;
    minScoreMovie: number;
    upgradeCheckHours: number;
    upgradeWindowDays: number;
    whisperEnabled: boolean;
    whisperModel: string;
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
export type BacklogResponse = {
    backlog: number;
    eligible: number;
    skipped: {
        [key: string]: number;
    };
};
export type ExclusionResponse = {
    ref: string;
    scope: string;
};
export type JobResponse = {
    agent: string | null;
    agentName: string;
    attempt: number;
    episodes: string;
    error: string;
    fileName: string;
    finishedAt: string | null;
    id: string;
    kind: string;
    language: string;
    maxAttempts: number;
    mediaFile: string;
    notBefore: string | null;
    origin: string;
    prefetched: boolean;
    progress: number;
    queuedAt: string;
    state: string;
    targetCodec: string;
    title: string;
    titleName: string;
};
export type JobListResponse = {
    backlog: number;
    counts: {
        [key: string]: number;
    };
    jobs: JobResponse[];
    skipped: {
        [key: string]: number;
    };
};
export type AgentSecretResponse = {
    configured: boolean;
    masked: string;
    secret: string;
};
export type EncoderCalibrationResponse = {
    encoder: string;
    max: number;
    min: number;
    presets: {
        [key: string]: number;
    };
    qualityArg: string;
};
export type TranscodeSettingsResponse = {
    bitrateCapsMbps: {
        [key: string]: number;
    };
    defaultSlotsPerGpu: number;
    enabled: boolean;
    encoderOverrides: {
        [key: string]: number;
    };
    encoders: EncoderCalibrationResponse[];
    lowBitratePct: number;
    minSavingPct: number;
    preset: string;
    skipAlreadyTarget: boolean;
    skipLowBitrate: boolean;
    targetCodec: string;
    transferCap: number;
};
export type UpdateTranscodeSettingsRequest = {
    bitrateCapsMbps: {
        [key: string]: number;
    };
    defaultSlotsPerGpu: number;
    enabled: boolean;
    encoderOverrides: {
        [key: string]: number;
    } | {
        [key: string]: number;
    };
    lowBitratePct: number;
    minSavingPct: number;
    preset: string;
    skipAlreadyTarget: boolean;
    skipLowBitrate: boolean;
    targetCodec: string;
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
    infoHash: string;
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
export type DownloadRemoved = {
    download: string;
    title: string;
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
export type EpisodesUnreleased = {
    episodes: string[] | null;
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
    startTime: number;
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
export type SidecarMove = {
    newPath: string;
    oldPath: string;
};
export type FileRenamed = {
    episodes: EpisodeRef[] | null;
    mediaFile: string;
    newPath: string;
    oldPath: string;
    sidecars: SidecarMove[] | null;
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
    manual: boolean;
    reason: string;
    retryable: boolean;
};
export type ImportRemoved = {
    download: string;
    releaseName: string;
    title: string;
};
export type IssueActionRequested = {
    action: string;
    episode: string | null;
    issue: string;
    removedDownloads: string[] | null;
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
export type JobCancelled = {
    job: string;
    mediaFile: string;
    reason: string;
};
export type JobFailed = {
    final: boolean;
    job: string;
    kind: string;
    language: string;
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
export type MigrationProgressed = {
    done: number;
    phase: string;
    step: string;
    total: number;
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
export type ProfileDeleted = {
    profile: string;
    replacedBy: string;
};
export type ProjectionUpdated = {
    projection: string;
    title: string | null;
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
    download: string | null;
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
export type RequestCancelled = {
    request: string;
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
    episodes: string[] | null;
    forced: boolean;
    language: string;
    mediaFile: string;
    replaces: boolean;
    sdh: boolean;
    source: string;
    synced: boolean;
    title: string;
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
/**
 * Every blocklist entry, newest first, expired temporary ones included (admin)
 */
export function listBlocklist({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: BlocklistEntryResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/blocklist", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Remove a blocklist entry so its release can be grabbed again (admin)
 */
export function removeBlocklistEntry(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/blocklist/${encodeURIComponent(id)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Custom formats ordered by name (admin)
 */
export function listCustomFormats({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: CustomFormatResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/custom-formats", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Create a custom format (admin)
 */
export function createCustomFormat(customFormatRequest: CustomFormatRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 201;
        data: CustomFormatResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/custom-formats", oazapfts.json({
        ...opts,
        method: "POST",
        body: customFormatRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Create a format for a release-name term scored -10000 in every profile (admin)
 */
export function blockTerm(blockTermRequest: BlockTermRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 201;
        data: CustomFormatResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/custom-formats/block-term", oazapfts.json({
        ...opts,
        method: "POST",
        body: blockTermRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Delete a custom format and its scores (admin)
 */
export function deleteCustomFormat(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/custom-formats/${encodeURIComponent(id)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Replace a custom format's name and specs (admin)
 */
export function updateCustomFormat(id: string, customFormatRequest: CustomFormatRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: CustomFormatResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/custom-formats/${encodeURIComponent(id)}`, oazapfts.json({
        ...opts,
        method: "PUT",
        body: customFormatRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Whether each of a title's files meets its profile's cutoff and may still be upgraded (admin)
 */
export function listCutoffStatus(title: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: CutoffStatusResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/cutoff${QS.query(QS.explode({
        title
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Blocked-file check, blocked patterns, metadata timeout and stalled limit (admin)
 */
export function getDownloadOptions({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: DownloadOptionsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/download-options", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Change the blocked-file check and download timeouts (admin)
 */
export function setDownloadOptions(downloadOptionsRequest: DownloadOptionsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: DownloadOptionsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/download-options", oazapfts.json({
        ...opts,
        method: "PUT",
        body: downloadOptionsRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Downloads still in qBittorrent with their last polled progress, newest first (admin)
 */
export function listDownloads({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: DownloadResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/downloads", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Remove a download from qBittorrent, with its data when deleteData is set (admin)
 */
export function removeDownload(id: string, { deleteData, accept }: {
    deleteData?: boolean;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/downloads/${encodeURIComponent(id)}${QS.query(QS.explode({
        deleteData
    }))}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
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
 * Quality profiles ordered by name, for profile pickers
 */
export function listProfiles({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ProfileSummaryResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/profiles", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Create a quality profile (admin)
 */
export function createProfile(profileRequest: ProfileRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 201;
        data: ProfileResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/profiles", oazapfts.json({
        ...opts,
        method: "POST",
        body: profileRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Create a 720P, 1080P or 2160P resolution preset (admin)
 */
export function createProfileFromPreset(fromPresetRequest: FromPresetRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 201;
        data: ProfileResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/profiles/from-preset", oazapfts.json({
        ...opts,
        method: "POST",
        body: fromPresetRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Delete a quality profile other than the default (admin)
 */
export function deleteProfile(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/profiles/${encodeURIComponent(id)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * A profile with items, groups, sizes and format scores (admin)
 */
export function getProfile(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ProfileResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/profiles/${encodeURIComponent(id)}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Replace a quality profile (admin)
 */
export function updateProfile(id: string, profileRequest: ProfileRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ProfileResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/profiles/${encodeURIComponent(id)}`, oazapfts.json({
        ...opts,
        method: "PUT",
        body: profileRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Radarr's 30 qualities, used for movies and TV
 */
export function listQualities({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: QualityResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/qualities", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * RSS sync, backlog sweep and pending check intervals (admin)
 */
export function getAcquisitionSchedules({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: SchedulesResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/schedules", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Change the RSS sync and backlog sweep intervals (admin)
 */
export function setAcquisitionSchedules(schedulesRequest: SchedulesRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: SchedulesResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/schedules", oazapfts.json({
        ...opts,
        method: "PUT",
        body: schedulesRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Whether a requested or added title is searched at once (admin)
 */
export function getSearchOptions({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: SearchOptionsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/search-options", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Turn searching on title add on or off (admin)
 */
export function setSearchOptions(searchOptionsRequest: SearchOptionsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: SearchOptionsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/search-options", oazapfts.json({
        ...opts,
        method: "PUT",
        body: searchOptionsRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Start a search; results stream on the live topic search:<id>, and a non-interactive one grabs on its own (admin)
 */
export function startSearch(startSearchRequest: StartSearchRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 201;
        data: SearchResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/searches", oazapfts.json({
        ...opts,
        method: "POST",
        body: startSearchRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Cancel a search's in-flight indexer queries (admin)
 */
export function cancelSearch(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/searches/${encodeURIComponent(id)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * A search's cached state, for reconnect and refresh (admin)
 */
export function getSearch(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: SearchResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/searches/${encodeURIComponent(id)}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Grab a release from the search's cached results; override grabs a rejected one (admin)
 */
export function grabRelease(id: string, grabRequest: GrabRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: GrabResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/acquisition/searches/${encodeURIComponent(id)}/grab`, oazapfts.json({
        ...opts,
        method: "POST",
        body: grabRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Default profile, delay, settle window and TRaSH sync (admin)
 */
export function getAcquisitionSettings({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: AcquisitionSettingsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/settings", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Change the default profile, delay, settle window and TRaSH sync (admin)
 */
export function updateAcquisitionSettings(acquisitionSettingsRequest: AcquisitionSettingsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: AcquisitionSettingsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/settings", oazapfts.json({
        ...opts,
        method: "PUT",
        body: acquisitionSettingsRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * TRaSH guide profiles as of the last sync (admin)
 */
export function listTrashProfiles({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: TrashProfileResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/trash/profiles", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * TRaSH sync setting and last attempt (admin)
 */
export function getTrashStatus({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: TrashStatusResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/trash/status", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Queue a TRaSH Guides sync now (admin)
 */
export function syncTrash({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: TrashStatusResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/acquisition/trash/sync", {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
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
 * Dismiss every open attention item (admin)
 */
export function clearAttention({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/activity/attention/clear", {
        ...opts,
        method: "POST",
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
 * The notification kinds your role may subscribe a URL to
 */
export function listNotificationKinds({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: NotificationKindResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/activity/notification-kinds", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Your notification URLs, masked
 */
export function listNotificationUrls({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: NotificationUrlResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/activity/notification-urls", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Add a shoutrrr URL (at most 10 per user)
 */
export function addNotificationUrl(notificationUrlRequest: NotificationUrlRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 201;
        data: NotificationUrlResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/activity/notification-urls", oazapfts.json({
        ...opts,
        method: "POST",
        body: notificationUrlRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Delete a notification URL
 */
export function deleteNotificationUrl(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/activity/notification-urls/${encodeURIComponent(id)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Change a URL's label or event kinds
 */
export function updateNotificationUrl(id: string, notificationUrlChangeRequest: NotificationUrlChangeRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: NotificationUrlResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/activity/notification-urls/${encodeURIComponent(id)}`, oazapfts.json({
        ...opts,
        method: "PATCH",
        body: notificationUrlChangeRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Send a test message to a URL now
 */
export function testNotificationUrl(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/activity/notification-urls/${encodeURIComponent(id)}/test`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Your in-app notifications, newest first, and how many are unread
 */
export function listNotifications({ unread, before, limit, accept }: {
    unread?: boolean;
    before?: string;
    limit?: number;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: NotificationListResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/activity/notifications${QS.query(QS.explode({
        unread,
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
 * Mark all your notifications read
 */
export function markAllNotificationsRead({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/activity/notifications/read-all", {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Mark one of your notifications read
 */
export function markNotificationRead(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/activity/notifications/${encodeURIComponent(id)}/read`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Pipeline rows: one title's rows, each listed title's summary row, the active rows, or the recently active titles' summary rows
 */
export function getPipeline({ title, titles, active, recent, accept }: {
    title?: string;
    titles?: string[];
    active?: boolean;
    recent?: boolean;
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
        active,
        recent
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
 * One movie or series with its seasons, from the library when tracked, else from TMDB
 */
export function getDiscoverDetail(kind: string, tmdbId: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: DiscoverDetailResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/catalog/discover/${encodeURIComponent(kind)}/${encodeURIComponent(tmdbId)}`, {
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
 * Get the global wanted languages and the languages a list may hold
 */
export function getWantedLanguages({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: WantedLanguagesResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/catalog/languages", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Replace the global wanted languages, most wanted first (admin)
 */
export function updateWantedLanguages(updateWantedLanguagesRequest: UpdateWantedLanguagesRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: WantedLanguagesResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/catalog/languages", oazapfts.json({
        ...opts,
        method: "PUT",
        body: updateWantedLanguagesRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Library folders a scan couldn't match with certainty, oldest first (admin)
 */
export function listScanReview({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ScanReviewResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/catalog/scan-review", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Leave a folder out of the library; later scans don't ask again (admin)
 */
export function ignoreScanReview(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ScanReviewResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/catalog/scan-review/${encodeURIComponent(id)}/ignore`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Match a folder to a TMDB title; creates the title if it's new (admin)
 */
export function matchScanReview(id: string, matchScanReviewRequest: MatchScanReviewRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ScanReviewResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/catalog/scan-review/${encodeURIComponent(id)}/match`, oazapfts.json({
        ...opts,
        method: "POST",
        body: matchScanReviewRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
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
export function listTitles({ q, kind, monitored, released, ids, sort, accept }: {
    q?: string;
    kind?: string;
    monitored?: boolean;
    released?: boolean;
    ids?: string[];
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
        ids,
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
 * Number a series by a TMDB episode group, or by aired order for an empty ID (admin)
 */
export function selectEpisodeGroup(id: string, selectEpisodeGroupRequest: SelectEpisodeGroupRequest, { accept }: {
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
    }>(`/api/catalog/titles/${encodeURIComponent(id)}/episode-group`, oazapfts.json({
        ...opts,
        method: "PUT",
        body: selectEpisodeGroupRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * List TMDB's episode groups for a series
 */
export function listEpisodeGroups(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: EpisodeGroupResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/catalog/titles/${encodeURIComponent(id)}/episode-groups`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Monitor or unmonitor one episode (admin)
 */
export function updateEpisode(id: string, episodeId: string, updateMonitoredRequest: UpdateMonitoredRequest, { accept }: {
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
    }>(`/api/catalog/titles/${encodeURIComponent(id)}/episodes/${encodeURIComponent(episodeId)}`, oazapfts.json({
        ...opts,
        method: "PATCH",
        body: updateMonitoredRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Set or clear a Title's wanted-languages override (admin)
 */
export function updateTitleLanguages(id: string, updateTitleLanguagesRequest: UpdateTitleLanguagesRequest, { accept }: {
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
    }>(`/api/catalog/titles/${encodeURIComponent(id)}/languages`, oazapfts.json({
        ...opts,
        method: "PUT",
        body: updateTitleLanguagesRequest,
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
 * Monitor or unmonitor a season and all its episodes (admin)
 */
export function updateSeason(id: string, n: string, updateMonitoredRequest: UpdateMonitoredRequest, { accept }: {
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
    }>(`/api/catalog/titles/${encodeURIComponent(id)}/seasons/${encodeURIComponent(n)}`, oazapfts.json({
        ...opts,
        method: "PATCH",
        body: updateMonitoredRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
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
 * Permissions new Jellyfin users start with (admin)
 */
export function getUserDefaults({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: Permissions;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/defaults", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Set the permissions new Jellyfin users start with (admin)
 */
export function setUserDefaults(permissions: Permissions, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: Permissions;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/defaults", oazapfts.json({
        ...opts,
        method: "PUT",
        body: permissions,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
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
 * Sign in with a Jellyfin username and password
 */
export function signInJellyfin(credentialsRequest: CredentialsRequest, { accept }: {
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
    }>("/api/identity/login/jellyfin", oazapfts.json({
        ...opts,
        method: "POST",
        body: credentialsRequest,
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
 * What the sign-in page offers besides the local admin
 */
export function getSignInMethods({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: SignInMethodsResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/login/methods", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Start a Jellyfin Quick Connect sign-in; returns the code to approve and a handle to poll
 */
export function startQuickConnect({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: QuickConnectStartResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/login/quick-connect", {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Poll a Quick Connect sign-in: 202 while pending, a session once approved, 410 once expired
 */
export function pollQuickConnect(handle: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: QuickConnectPollResponse;
    } | {
        status: 202;
        data: QuickConnectPollResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/identity/login/quick-connect/${encodeURIComponent(handle)}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
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
 * Whether Jellyfin users may sign in (admin)
 */
export function getSignInSettings({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: SignInSettings;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/sign-in-settings", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Allow or stop Jellyfin sign-in; the local admin always stays available (admin)
 */
export function setSignInSettings(signInSettings: SignInSettings, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: SignInSettings;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/sign-in-settings", oazapfts.json({
        ...opts,
        method: "PUT",
        body: signInSettings,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Every user ordered by username (admin)
 */
export function listUsers({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: UserResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/identity/users", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Change a user's role, permissions or disabled flag (admin)
 */
export function updateUser(id: string, updateUserRequest: UpdateUserRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: UserResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/identity/users/${encodeURIComponent(id)}`, oazapfts.json({
        ...opts,
        method: "PATCH",
        body: updateUserRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Episodes with a file, episodes available and size per title, in input order (admin)
 */
export function getCoverage({ titles, accept }: {
    titles?: string[];
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: TitleCoverageResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/library/coverage${QS.query(QS.explode({
        titles
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * One title's media files, or several titles' files in input order; newest first per title (admin)
 */
export function listMediaFiles({ title, titles, transcoded, accept }: {
    title?: string;
    titles?: string[];
    transcoded?: boolean;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MediaFileResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/library/files${QS.query(QS.explode({
        title,
        titles,
        transcoded
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Delete a media file from disk; other contexts drop what they keep for it (admin)
 */
export function deleteMediaFile(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/library/files/${encodeURIComponent(id)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * One media file with its media info (admin)
 */
export function getMediaFile(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MediaFileResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/library/files/${encodeURIComponent(id)}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Whether qBittorrent's download folder shares a filesystem with each root folder (admin)
 */
export function getHardlinkCheck({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: HardlinkCheckResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/library/hardlink-check", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Imports in the given statuses, most recently changed first (admin)
 */
export function listImports(status: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ImportResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/library/imports${QS.query(QS.explode({
        status
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Remove & blocklist a failed or manual import; Acquisition drops its download and grabs another (admin)
 */
export function removeImport(downloadId: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/library/imports/${encodeURIComponent(downloadId)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Import a failed or manual import with a file → episodes mapping (admin)
 */
export function importManually(downloadId: string, manualImportRequest: ManualImportRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: ImportResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/library/imports/${encodeURIComponent(downloadId)}/manual`, oazapfts.json({
        ...opts,
        method: "POST",
        body: manualImportRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Run a failed or manual import again (admin)
 */
export function retryImport(downloadId: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: ImportResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/library/imports/${encodeURIComponent(downloadId)}/retry`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Root folders with their hardlink check (admin)
 */
export function listRootFolders({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: RootFolderResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/library/root-folders", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Add a root folder; it must exist and be writable (admin)
 */
export function addRootFolder(addRootFolderRequest: AddRootFolderRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 201;
        data: RootFolderResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/library/root-folders", oazapfts.json({
        ...opts,
        method: "POST",
        body: addRootFolderRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Forget a root folder that holds no media files (admin)
 */
export function removeRootFolder(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/library/root-folders/${encodeURIComponent(id)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * The last library scan and the folders scans have found (admin)
 */
export function getLibraryScan({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: LibraryScanResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/library/scan", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Scan the root folders for folders Magpie doesn't know yet; nothing happens while one runs (admin)
 */
export function startLibraryScan({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: LibraryScanResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/library/scan", {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Choose whether matched folders are renamed to the fixed naming (admin)
 */
export function setLibraryScanOptions(scanOptionsRequest: ScanOptionsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: LibraryScanResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/library/scan/options", oazapfts.json({
        ...opts,
        method: "PUT",
        body: scanOptionsRequest,
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
 * Forget the import to start over; anything committed stays (admin)
 */
export function resetMigration({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/migration", {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * The Sonarr/Radarr import: connections, paths, dry-run report, review queues and summary (admin)
 */
export function getMigration({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MigrationResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/migration", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Import the plan, or resume a failed commit; runs in the background (admin)
 */
export function startMigrationCommit({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: MigrationResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/migration/commit", {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Connect or skip Sonarr or Radarr; checks the version (Sonarr ≥ 4.0) and reads its root folders (admin)
 */
export function connectMigrationSource(migrationConnectRequest: MigrationConnectRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MigrationResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/migration/connections", oazapfts.json({
        ...opts,
        method: "PUT",
        body: migrationConnectRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Confirm the manual cutover checklist; Commit needs it (admin)
 */
export function setMigrationCutover(migrationCutoverRequest: MigrationCutoverRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MigrationResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/migration/cutover", oazapfts.json({
        ...opts,
        method: "PUT",
        body: migrationCutoverRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Read both apps and plan the import without writing anything; runs in the background (admin)
 */
export function startMigrationDryRun({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: MigrationResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/migration/dry-run", {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Choose folder renames and alias import (admin)
 */
export function setMigrationOptions(migrationOptionsRequest: MigrationOptionsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MigrationResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/migration/options", oazapfts.json({
        ...opts,
        method: "PUT",
        body: migrationOptionsRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Set where each app's root folder is as the server sees it (admin)
 */
export function setMigrationPaths(migrationPathsRequest: MigrationPathsRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MigrationResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/migration/paths", oazapfts.json({
        ...opts,
        method: "PUT",
        body: migrationPathsRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Check that the server can read and write a folder (admin)
 */
export function checkMigrationPath(migrationPathCheckRequest: MigrationPathCheckRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MigrationPathCheck;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/migration/paths/check", oazapfts.json({
        ...opts,
        method: "POST",
        body: migrationPathCheckRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Settle one review item (admin)
 */
export function decideMigrationReview(migrationDecisionRequest: MigrationDecisionRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MigrationResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/migration/reviews", oazapfts.json({
        ...opts,
        method: "POST",
        body: migrationDecisionRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Settle every open item of a queue with its default (admin)
 */
export function skipMigrationReviewQueue(migrationSkipQueueRequest: MigrationSkipQueueRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MigrationResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/platform/migration/reviews/skip", oazapfts.json({
        ...opts,
        method: "POST",
        body: migrationSkipQueueRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Search TMDB for a title to match an unmatched one to (admin)
 */
export function searchMigrationTitle(kind: string, q: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: MigrationCandidate[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/platform/migration/search${QS.query(QS.explode({
        kind,
        q
    }))}`, {
        ...opts,
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
 * Requests the viewer may see, newest first: their own unless they may view others'
 */
export function listRequests({ status, userId, titleId, count, accept }: {
    status?: string[];
    userId?: string;
    titleId?: string;
    count?: boolean;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: RequestListResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/requests${QS.query(QS.explode({
        status,
        userId,
        titleId,
        count
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Request a movie, a whole series or chosen seasons; admins and auto-approved users are approved at once
 */
export function createRequest(createRequestRequest: CreateRequestRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 201;
        data: RequestResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/requests", oazapfts.json({
        ...opts,
        method: "POST",
        body: createRequestRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Issues the viewer may see, newest first: their own unless they may view others' requests
 */
export function listIssues({ status, userId, titleId, count, accept }: {
    status?: string[];
    userId?: string;
    titleId?: string;
    count?: boolean;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: IssueListResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/requests/issues${QS.query(QS.explode({
        status,
        userId,
        titleId,
        count
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Report a problem with a Title or one episode (needs the report-issues toggle)
 */
export function reportIssue(reportIssueRequest: ReportIssueRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 201;
        data: IssueResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/requests/issues", oazapfts.json({
        ...opts,
        method: "POST",
        body: reportIssueRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * One issue with its comment thread
 */
export function getIssue(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: IssueThreadResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/requests/issues/${encodeURIComponent(id)}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Fix an open issue: blocklist and re-grab, or re-run subtitles (admin)
 */
export function requestIssueAction(id: string, issueActionRequest: IssueActionRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: IssueResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/requests/issues/${encodeURIComponent(id)}/actions`, oazapfts.json({
        ...opts,
        method: "POST",
        body: issueActionRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Reply on an issue (its reporter or an admin)
 */
export function commentIssue(id: string, commentIssueRequest: CommentIssueRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 201;
        data: IssueCommentResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/requests/issues/${encodeURIComponent(id)}/comments`, oazapfts.json({
        ...opts,
        method: "POST",
        body: commentIssueRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Close an issue by hand (its reporter or an admin)
 */
export function resolveIssue(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: IssueResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/requests/issues/${encodeURIComponent(id)}/resolve`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Cancel your own pending request
 */
export function cancelRequest(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/requests/${encodeURIComponent(id)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * One request with its item progress
 */
export function getRequest(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: RequestResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/requests/${encodeURIComponent(id)}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Approve a pending request, optionally with another profile (admin)
 */
export function approveRequest(id: string, approveRequestRequest: ApproveRequestRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: RequestResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/requests/${encodeURIComponent(id)}/approve`, oazapfts.json({
        ...opts,
        method: "POST",
        body: approveRequestRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Decline a pending request (admin)
 */
export function declineRequest(id: string, declineRequestRequest: DeclineRequestRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: RequestResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/requests/${encodeURIComponent(id)}/decline`, oazapfts.json({
        ...opts,
        method: "POST",
        body: declineRequestRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Approve a failed request again (admin)
 */
export function retryRequest(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: RequestResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/requests/${encodeURIComponent(id)}/retry`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * A media file's subtitles and wanted-language coverage (admin)
 */
export function getFileSubtitles(mediaFileId: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: FileSubtitlesResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/subtitles/files/${encodeURIComponent(mediaFileId)}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Scored OpenSubtitles results for one language of the file, best first (admin)
 */
export function listSubtitleCandidates(mediaFileId: string, language: string, { forced, accept }: {
    forced?: boolean;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: CandidateResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/subtitles/files/${encodeURIComponent(mediaFileId)}/candidates${QS.query(QS.explode({
        language,
        forced
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Download a candidate, sync it and store it as the file's sidecar (admin)
 */
export function downloadSubtitle(mediaFileId: string, downloadSubtitleRequest: DownloadSubtitleRequest, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: FileSubtitlesResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/subtitles/files/${encodeURIComponent(mediaFileId)}/download`, oazapfts.json({
        ...opts,
        method: "POST",
        body: downloadSubtitleRequest,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Search the file's missing languages now, skipping the backoff (admin)
 */
export function searchFileSubtitles(mediaFileId: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: FileSubtitlesResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/subtitles/files/${encodeURIComponent(mediaFileId)}/search`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * OpenSubtitles downloads left today (admin)
 */
export function getSubtitleQuota({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: QuotaResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/subtitles/quota", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Whisper, auto-sync, score thresholds and the upgrade window (admin)
 */
export function getSubtitleSettings({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: SubtitleSettingsBody;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/subtitles/settings", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Replace the subtitle settings (admin)
 */
export function setSubtitleSettings(subtitleSettingsBody: SubtitleSettingsBody, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: SubtitleSettingsBody;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/subtitles/settings", oazapfts.json({
        ...opts,
        method: "PUT",
        body: subtitleSettingsBody,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    })));
}
/**
 * Remove a sidecar subtitle (admin)
 */
export function deleteSubtitle(subtitleId: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/subtitles/${encodeURIComponent(subtitleId)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
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
 * Queued backlog jobs, eligible files and skips per reason (admin)
 */
export function getTranscodeBacklog({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: BacklogResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/transcode/backlog", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Re-evaluate every eligible and skipped file against the current policy, in the background (admin)
 */
export function sweepTranscodeBacklog({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 202;
        data: BacklogResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/transcode/backlog/sweep", {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Titles and root folders kept out of transcoding (admin)
 */
export function listTranscodeExclusions({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ExclusionResponse[];
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/transcode/exclusions", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * End an exclusion and evaluate its files again (admin)
 */
export function includeInTranscoding(scope: string, ref: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/transcode/exclusions/${encodeURIComponent(scope)}/${encodeURIComponent(ref)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Keep a title or root folder out of transcoding; its queued jobs are cancelled (admin)
 */
export function excludeFromTranscoding(scope: string, ref: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: ExclusionResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/transcode/exclusions/${encodeURIComponent(scope)}/${encodeURIComponent(ref)}`, {
        ...opts,
        method: "PUT",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Jobs agents hold, then queued jobs in offer order, then finished ones (admin)
 */
export function listTranscodeJobs({ state, kind, limit, accept }: {
    state?: string[];
    kind?: string;
    limit?: number;
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: JobListResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/transcode/jobs${QS.query(QS.explode({
        state,
        kind,
        limit
    }))}`, {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Delete a FAILED or NOT_WORTH_IT job (admin)
 */
export function dismissTranscodeJob(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 204;
        data: UnknownInterface;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/transcode/jobs/${encodeURIComponent(id)}`, {
        ...opts,
        method: "DELETE",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Cancel an unfinished job; its agent drops it (admin)
 */
export function cancelTranscodeJob(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: JobResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/transcode/jobs/${encodeURIComponent(id)}/cancel`, {
        ...opts,
        method: "POST",
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
/**
 * Queue a FAILED or NOT_WORTH_IT job again from its first attempt (admin)
 */
export function retryTranscodeJob(id: string, { accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: JobResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>(`/api/transcode/jobs/${encodeURIComponent(id)}/retry`, {
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
 * Update transcode settings; a policy change re-evaluates the backlog (admin)
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

// Mirrors server/internal/platform/migrate: phases, apps, review queues and their decisions.

export enum MigrationPhase {
	SETUP = 'SETUP',
	DRY_RUN_RUNNING = 'DRY_RUN_RUNNING',
	DRY_RUN_FAILED = 'DRY_RUN_FAILED',
	REVIEW = 'REVIEW',
	COMMIT_RUNNING = 'COMMIT_RUNNING',
	COMMIT_FAILED = 'COMMIT_FAILED',
	DONE = 'DONE'
}

export const RUNNING_PHASES: string[] = [
	MigrationPhase.DRY_RUN_RUNNING,
	MigrationPhase.COMMIT_RUNNING
];

export enum ArrApp {
	SONARR = 'SONARR',
	RADARR = 'RADARR'
}

export const ARR_APPS = [ArrApp.SONARR, ArrApp.RADARR];

export const APP_LABELS: Record<ArrApp, string> = {
	[ArrApp.SONARR]: 'Sonarr',
	[ArrApp.RADARR]: 'Radarr'
};

export const APP_PLACEHOLDERS: Record<ArrApp, string> = {
	[ArrApp.SONARR]: 'http://sonarr:8989',
	[ArrApp.RADARR]: 'http://radarr:7878'
};

export enum PathAccess {
	OK = 'OK',
	MISSING = 'MISSING',
	NOT_A_FOLDER = 'NOT_A_FOLDER',
	NOT_WRITABLE = 'NOT_WRITABLE',
	NOT_ABSOLUTE = 'NOT_ABSOLUTE'
}

export const PATH_ACCESS_LABELS: Record<PathAccess, string> = {
	[PathAccess.OK]: 'Reachable',
	[PathAccess.MISSING]: 'Not found',
	[PathAccess.NOT_A_FOLDER]: 'Not a folder',
	[PathAccess.NOT_WRITABLE]: 'Read-only',
	[PathAccess.NOT_ABSOLUTE]: 'Not an absolute path'
};

export enum ReviewQueue {
	TITLES = 'TITLES',
	NUMBERING = 'NUMBERING',
	CLASHES = 'CLASHES',
	SPECS = 'SPECS',
	SERIES_TYPES = 'SERIES_TYPES'
}

export const QUEUE_LABELS: Record<ReviewQueue, string> = {
	[ReviewQueue.TITLES]: 'Unmatched titles',
	[ReviewQueue.NUMBERING]: 'Episode numbering',
	[ReviewQueue.CLASHES]: 'Profile and format clashes',
	[ReviewQueue.SPECS]: 'Untranslated conditions',
	[ReviewQueue.SERIES_TYPES]: 'Daily and anime series'
};

/** What "Skip the rest" does in each queue. */
export const QUEUE_SKIP_LABELS: Record<ReviewQueue, string> = {
	[ReviewQueue.TITLES]: 'Skip the rest',
	[ReviewQueue.NUMBERING]: 'Keep the suggestions',
	[ReviewQueue.CLASHES]: 'Keep both versions',
	[ReviewQueue.SPECS]: 'Drop the rest',
	[ReviewQueue.SERIES_TYPES]: 'Skip the rest'
};

export enum TitleDecision {
	PENDING = 'PENDING',
	MATCH = 'MATCH',
	SPLIT = 'SPLIT',
	SKIP = 'SKIP'
}

export enum NumberingDecision {
	PENDING = 'PENDING',
	GROUP = 'GROUP',
	AIRED = 'AIRED'
}

export enum ClashChoice {
	PENDING = 'PENDING',
	SONARR = 'SONARR',
	RADARR = 'RADARR',
	KEEP_BOTH = 'KEEP_BOTH'
}

export enum ClashKind {
	FORMAT = 'FORMAT',
	PROFILE = 'PROFILE'
}

export enum SpecDecision {
	PENDING = 'PENDING',
	DROP = 'DROP',
	EDIT = 'EDIT'
}

export enum SeriesTypeDecision {
	PENDING = 'PENDING',
	IMPORT = 'IMPORT',
	SKIP = 'SKIP'
}

export enum SeriesType {
	DAILY = 'daily',
	ANIME = 'anime'
}

export enum MediaKind {
	MOVIE = 'MOVIE',
	SERIES = 'SERIES'
}

export enum CommitStep {
	CUSTOM_FORMATS = 'CUSTOM_FORMATS',
	QUALITY_PROFILES = 'QUALITY_PROFILES',
	DELAY_PROFILES = 'DELAY_PROFILES',
	TITLES = 'TITLES',
	ROOT_FOLDERS = 'ROOT_FOLDERS',
	MEDIA_FILES = 'MEDIA_FILES',
	BLOCKLIST = 'BLOCKLIST',
	HISTORY = 'HISTORY'
}

export const STEP_LABELS: Record<string, string> = {
	READING_SONARR: 'Reading Sonarr',
	READING_RADARR: 'Reading Radarr',
	MATCHING: 'Matching titles on TMDB',
	[CommitStep.CUSTOM_FORMATS]: 'Custom formats',
	[CommitStep.QUALITY_PROFILES]: 'Quality profiles',
	[CommitStep.DELAY_PROFILES]: 'Delay profiles',
	[CommitStep.TITLES]: 'Titles',
	[CommitStep.ROOT_FOLDERS]: 'Root folders',
	[CommitStep.MEDIA_FILES]: 'Media files',
	[CommitStep.BLOCKLIST]: 'Blocklist',
	[CommitStep.HISTORY]: 'History'
};

export const MIGRATION_PROGRESSED_EVENT = 'platform.MigrationProgressed';

/** The wizard's page under Settings → General. */
export const MIGRATION_PATH = '/settings/general/import';

/** The live progress event's payload. */
export interface ProgressEvent {
	phase: string;
	step: string;
	done: number;
	total: number;
}

const FULL_PERCENT = 100;

/** Percent done, or undefined while the total isn't known yet. */
export function progressPercent(done: number, total: number): number | undefined {
	if (total <= 0) return undefined;
	return Math.min(FULL_PERCENT, Math.round((done / total) * FULL_PERCENT));
}

/** A group's score as a whole percent. */
export function scorePercent(score: number): number {
	return Math.round(score * FULL_PERCENT);
}

/** The wizard's steps, in the order of 11-migration. */
export enum WizardStep {
	CONNECT = 'CONNECT',
	PATHS = 'PATHS',
	CUTOVER = 'CUTOVER',
	DRY_RUN = 'DRY_RUN',
	REVIEW = 'REVIEW',
	OPTIONS = 'OPTIONS',
	COMMIT = 'COMMIT'
}

export const WIZARD_STEPS: { step: WizardStep; label: string }[] = [
	{ step: WizardStep.CONNECT, label: 'Connect' },
	{ step: WizardStep.PATHS, label: 'Paths' },
	{ step: WizardStep.CUTOVER, label: 'Cutover' },
	{ step: WizardStep.DRY_RUN, label: 'Dry run' },
	{ step: WizardStep.REVIEW, label: 'Review' },
	{ step: WizardStep.OPTIONS, label: 'Options' },
	{ step: WizardStep.COMMIT, label: 'Commit' }
];

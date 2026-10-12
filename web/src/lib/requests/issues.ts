import { apiErrorMessage, apiProblemInstance, apiProblemType, type IssueResponse } from '$lib/sdk';
import { ACTIVITY_TAB_QUERY, ActivityTab } from '$lib/activity/activity-tabs';
import { Route } from '$lib/navigation/nav';

// Mirrors server/internal/requests/api: issue kinds, statuses, actions and events.

export enum IssueKind {
	VIDEO = 'VIDEO',
	AUDIO = 'AUDIO',
	SUBTITLE = 'SUBTITLE',
	OTHER = 'OTHER'
}

export const ISSUE_KINDS: IssueKind[] = Object.values(IssueKind);

export const KIND_LABELS: Record<IssueKind, string> = {
	[IssueKind.VIDEO]: 'Video',
	[IssueKind.AUDIO]: 'Audio',
	[IssueKind.SUBTITLE]: 'Subtitle',
	[IssueKind.OTHER]: 'Other'
};

export enum IssueStatus {
	OPEN = 'OPEN',
	ACTION_PENDING = 'ACTION_PENDING',
	RESOLVED = 'RESOLVED'
}

export enum IssueAction {
	REGRAB = 'REGRAB',
	RERUN_SUBTITLES = 'RERUN_SUBTITLES'
}

export const ACTION_LABELS: Record<IssueAction, string> = {
	[IssueAction.REGRAB]: 'Blocklist & re-grab',
	[IssueAction.RERUN_SUBTITLES]: 'Re-run subtitles'
};

/** What a pending issue shows while its fix runs. */
export const PENDING_LABELS: Record<IssueAction, string> = {
	[IssueAction.REGRAB]: 'Re-grabbing',
	[IssueAction.RERUN_SUBTITLES]: 'Re-running subtitles'
};

/** IssueResponse.resolvedBy besides a user ID. */
export enum ResolvedBy {
	ACTION = 'ACTION',
	TITLE_REMOVED = 'TITLE_REMOVED'
}

export enum IssueEvent {
	REPORTED = 'requests.IssueReported',
	COMMENTED = 'requests.IssueCommented',
	ACTION_REQUESTED = 'requests.IssueActionRequested',
	RESOLVED = 'requests.IssueResolved'
}

export const ISSUE_EVENTS: IssueEvent[] = Object.values(IssueEvent);

/** Events that change how many issues are open. */
export const OPEN_COUNT_EVENTS: IssueEvent[] = [IssueEvent.REPORTED, IssueEvent.RESOLVED];

/** The chips on the Issues tab. */
export enum IssueFilter {
	OPEN = 'OPEN',
	RESOLVED = 'RESOLVED'
}

export const ISSUE_FILTERS: IssueFilter[] = Object.values(IssueFilter);

export const ISSUE_FILTER_LABELS: Record<IssueFilter, string> = {
	[IssueFilter.OPEN]: 'Open',
	[IssueFilter.RESOLVED]: 'Resolved'
};

export const ISSUE_FILTER_STATUSES: Record<IssueFilter, IssueStatus[]> = {
	[IssueFilter.OPEN]: [IssueStatus.OPEN, IssueStatus.ACTION_PENDING],
	[IssueFilter.RESOLVED]: [IssueStatus.RESOLVED]
};

/** The query parameter that opens an issue's thread on the Requests page; notifications link with it. */
export const ISSUE_QUERY = 'issue';

/** Activity → Imports, where a waiting download is mapped. */
export const IMPORTS_PATH = `${Route.ACTIVITY}?${ACTIVITY_TAB_QUERY}=${ActivityTab.IMPORTS}`;

export function issuePath(id: string): string {
	return `${Route.REQUESTS}?${ISSUE_QUERY}=${id}`;
}

/** Problem types of the issue routes. Mirrors the Go controller constants. */
export enum IssueProblem {
	IMPORT_WAITING = 'urn:magpie:problem:import-waiting'
}

/** A download whose import waits for its files to be mapped, blocking a REGRAB. */
export interface WaitingImport {
	downloadId: string;
	/** The server's sentence naming the release. */
	message: string;
}

/** The waiting download a REGRAB was refused for, or undefined for any other error. */
export function waitingImportOf(error: unknown): WaitingImport | undefined {
	if (apiProblemType(error) !== IssueProblem.IMPORT_WAITING) return undefined;
	const downloadId = apiProblemInstance(error)?.split('/').pop();
	if (!downloadId) return undefined;
	return { downloadId, message: apiErrorMessage(error) };
}

const EPISODE_NUMBER_WIDTH = 2;

/** "S01E02". */
export function episodeCode(season: number, number: number): string {
	const pad = (n: number) => String(n).padStart(EPISODE_NUMBER_WIDTH, '0');
	return `S${pad(season)}E${pad(number)}`;
}

/** "Severance · S01E02", or just the title for a movie or a whole-series issue. */
export function issueSubject(issue: Pick<IssueResponse, 'titleName' | 'episode'>): string {
	const title = issue.titleName || 'Removed title';
	if (!issue.episode) return title;
	return `${title} · ${episodeCode(issue.episode.season, issue.episode.number)}`;
}

/** Whether a user ID resolved the issue, rather than an action or a removed title. */
export function resolvedByPerson(resolvedBy: string): boolean {
	return resolvedBy !== '' && !Object.values<string>(ResolvedBy).includes(resolvedBy);
}

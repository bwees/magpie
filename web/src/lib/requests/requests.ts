import { apiErrorStatus, apiProblemType, type StatusCountResponse } from '$lib/sdk';

// Mirrors server/internal/requests/api: status and scope values, and the request events.

export enum RequestStatus {
	PENDING = 'PENDING',
	APPROVED = 'APPROVED',
	PROCESSING = 'PROCESSING',
	PARTIALLY_AVAILABLE = 'PARTIALLY_AVAILABLE',
	AVAILABLE = 'AVAILABLE',
	DECLINED = 'DECLINED',
	FAILED = 'FAILED'
}

export enum RequestScope {
	MOVIE = 'MOVIE',
	ALL_SEASONS = 'ALL_SEASONS',
	SEASONS = 'SEASONS'
}

export enum RequestEvent {
	CREATED = 'requests.RequestCreated',
	APPROVED = 'requests.RequestApproved',
	DECLINED = 'requests.RequestDeclined',
	CANCELLED = 'requests.RequestCancelled',
	AVAILABLE = 'requests.RequestAvailable'
}

export const REQUEST_EVENTS: RequestEvent[] = Object.values(RequestEvent);

/** Events that change how many requests are pending. */
export const PENDING_EVENTS: RequestEvent[] = [
	RequestEvent.CREATED,
	RequestEvent.APPROVED,
	RequestEvent.DECLINED,
	RequestEvent.CANCELLED
];

/** Problem types of POST /api/requests conflicts. Mirrors the Go controller constants. */
export enum RequestProblem {
	ALREADY_REQUESTED = 'urn:magpie:problem:already-requested',
	ALREADY_MONITORED = 'urn:magpie:problem:already-monitored'
}

/** The status chips on the Requests page. */
export enum StatusFilter {
	PENDING = 'PENDING',
	APPROVED = 'APPROVED',
	AVAILABLE = 'AVAILABLE',
	DECLINED = 'DECLINED',
	ALL = 'ALL'
}

export const STATUS_FILTERS: StatusFilter[] = Object.values(StatusFilter);

export const FILTER_LABELS: Record<StatusFilter, string> = {
	[StatusFilter.PENDING]: 'Pending',
	[StatusFilter.APPROVED]: 'Approved',
	[StatusFilter.AVAILABLE]: 'Available',
	[StatusFilter.DECLINED]: 'Declined',
	[StatusFilter.ALL]: 'All'
};

/** The statuses each chip lists. Approved covers everything on its way, including failures to retry. */
export const FILTER_STATUSES: Record<StatusFilter, RequestStatus[]> = {
	[StatusFilter.PENDING]: [RequestStatus.PENDING],
	[StatusFilter.APPROVED]: [
		RequestStatus.APPROVED,
		RequestStatus.PROCESSING,
		RequestStatus.PARTIALLY_AVAILABLE,
		RequestStatus.FAILED
	],
	[StatusFilter.AVAILABLE]: [RequestStatus.AVAILABLE],
	[StatusFilter.DECLINED]: [RequestStatus.DECLINED],
	[StatusFilter.ALL]: []
};

export const STATUS_LABELS: Record<RequestStatus, string> = {
	[RequestStatus.PENDING]: 'Pending',
	[RequestStatus.APPROVED]: 'Approved',
	[RequestStatus.PROCESSING]: 'Processing',
	[RequestStatus.PARTIALLY_AVAILABLE]: 'Partly available',
	[RequestStatus.AVAILABLE]: 'Available',
	[RequestStatus.DECLINED]: 'Declined',
	[RequestStatus.FAILED]: 'Failed'
};

/** Requests that still cover their scope, so the search shows them as Requested. */
export const OPEN_STATUSES: RequestStatus[] = [
	RequestStatus.PENDING,
	RequestStatus.APPROVED,
	RequestStatus.PROCESSING,
	RequestStatus.PARTIALLY_AVAILABLE,
	RequestStatus.AVAILABLE
];

/** How many requests a chip holds. ALL counts every status. */
export function filterCount(counts: StatusCountResponse[], filter: StatusFilter): number {
	const statuses = FILTER_STATUSES[filter];
	return counts
		.filter((c) => statuses.length === 0 || statuses.includes(c.status as RequestStatus))
		.reduce((sum, c) => sum + c.count, 0);
}

const SEASON_SEPARATOR = ', ';

/** "Movie", "All seasons", "Season 2" or "Seasons 1, 3". */
export function scopeLabel(scope: string, seasons: number[]): string {
	switch (scope) {
		case RequestScope.MOVIE:
			return 'Movie';
		case RequestScope.ALL_SEASONS:
			return 'All seasons';
		default:
			return seasons.length === 1
				? `Season ${seasons[0]}`
				: `Seasons ${seasons.join(SEASON_SEPARATOR)}`;
	}
}

/** Identifies a movie or series across TMDB results and requests. */
export function mediaKey(kind: string, tmdbId: number): string {
	return `${kind}:${tmdbId}`;
}

const HTTP_CONFLICT = 409;

/** The problem type of a 409 from POST /api/requests, or undefined for any other error. */
export function conflictOf(error: unknown): RequestProblem | undefined {
	if (apiErrorStatus(error) !== HTTP_CONFLICT) return undefined;
	const type = apiProblemType(error);
	return Object.values<string>(RequestProblem).includes(type ?? '')
		? (type as RequestProblem)
		: undefined;
}

const INITIALS_LIMIT = 2;

/** Up to two initials for a requester's avatar. */
export function initials(name: string): string {
	return name
		.split(/\s+/)
		.filter(Boolean)
		.slice(0, INITIALS_LIMIT)
		.map((part) => part[0].toUpperCase())
		.join('');
}

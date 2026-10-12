import {
	approveRequest,
	cancelRequest,
	createRequest,
	declineRequest,
	getAcquisitionSettings,
	getDiscoverDetail,
	listRequests,
	retryRequest,
	type CreateRequestRequest,
	type DiscoverDetailResponse,
	type RequestListResponse,
	type RequestResponse
} from '$lib/sdk';
import type { RequestStatus } from './requests';

/** Where the request views read and act. The SDK in the app, a fake in tests. */
export interface RequestsApi {
	list(
		statuses: RequestStatus[],
		options?: { count?: boolean; titleId?: string }
	): Promise<RequestListResponse>;
	create(request: CreateRequestRequest): Promise<RequestResponse>;
	approve(id: string, profileId?: string): Promise<RequestResponse>;
	decline(id: string, reason: string): Promise<RequestResponse>;
	retry(id: string): Promise<RequestResponse>;
	cancel(id: string): Promise<unknown>;
	/** The default profile for new requests, from the admin-only acquisition settings. */
	defaultProfile(): Promise<string>;
	/** Name, year, poster and seasons of a movie or series, from Catalog. */
	media(kind: string, tmdbId: number): Promise<DiscoverDetailResponse>;
}

export const SDK_REQUESTS_API: RequestsApi = {
	list: (status, { count, titleId } = {}) => listRequests({ status, count, titleId }),
	create: (request) => createRequest(request),
	approve: (id, profileId) => approveRequest(id, { profileId }),
	decline: (id, reason) => declineRequest(id, { reason }),
	retry: (id) => retryRequest(id),
	cancel: (id) => cancelRequest(id),
	defaultProfile: async () => (await getAcquisitionSettings()).defaultProfileId,
	media: (kind, tmdbId) => getDiscoverDetail(kind, String(tmdbId))
};

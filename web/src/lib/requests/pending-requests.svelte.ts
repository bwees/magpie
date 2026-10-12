import type { LiveEvents } from '$lib/activity/activity-source';
import { SDK_REQUESTS_API, type RequestsApi } from './requests-api';
import { PENDING_EVENTS, RequestStatus } from './requests';

/** The pending-requests badge on the Requests nav item: the viewer's visible pending requests. */
export class PendingRequests {
	count = $state(0);

	readonly #api: RequestsApi;

	constructor(api: RequestsApi = SDK_REQUESTS_API) {
		this.#api = api;
	}

	async load(): Promise<void> {
		try {
			this.count = (await this.#api.list([RequestStatus.PENDING], { count: true })).total;
		} catch {
			// The badge keeps its last count; the next event or reconnect tries again.
		}
	}

	watch(live: LiveEvents): () => void {
		const stops = PENDING_EVENTS.map((type) => live.on(type, () => void this.load()));
		stops.push(live.onReset(() => void this.load()));
		return () => stops.forEach((stop) => stop());
	}
}

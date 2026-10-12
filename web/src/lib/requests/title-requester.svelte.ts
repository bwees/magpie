import { SDK_REQUESTS_API, type RequestsApi } from './requests-api';

/** Who requested a Title, for the title hero: the first request linked to it that the viewer may see. */
export class TitleRequester {
	name = $state<string>();

	readonly #api: RequestsApi;

	constructor(api: RequestsApi = SDK_REQUESTS_API) {
		this.#api = api;
	}

	async load(titleId: string): Promise<void> {
		try {
			const linked = await this.#api.list([], { titleId });
			this.name = linked.items.at(-1)?.requester.displayName || undefined;
		} catch {
			this.name = undefined;
		}
	}
}

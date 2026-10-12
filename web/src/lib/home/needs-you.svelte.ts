import { toast } from 'svelte-sonner';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import {
	apiErrorMessage,
	approveRequest,
	getDiscoverDetail,
	getRequest,
	type AttentionItemResponse,
	type DiscoverDetailResponse,
	type RequestResponse
} from '$lib/sdk';
import { AttentionAction, AttentionKind, KIND_SUBJECTS } from '$lib/activity/attention';
import type { AttentionList } from '$lib/activity/attention-list.svelte';
import type { TitleDirectory } from '$lib/catalog/title-directory.svelte';
import { titlePath } from '$lib/catalog/images';
import { Route } from '$lib/navigation/nav';
import { AutomaticSearch } from '$lib/search/automatic-search.svelte';
import { SettingsSection, settingsPath } from '$lib/settings/sections';

/** Where each attention action goes. The SDK in the app, a fake in tests. */
export interface NeedsYouApi {
	approve(requestId: string): Promise<unknown>;
	searchAgain(titleId: string, label: string): Promise<void>;
	request(id: string): Promise<RequestResponse>;
	media(kind: string, tmdbId: number): Promise<DiscoverDetailResponse>;
}

const automatic = new AutomaticSearch();

export const SDK_NEEDS_YOU_API: NeedsYouApi = {
	approve: (id) => approveRequest(id, {}),
	searchAgain: (titleId, label) => automatic.search({ titleId, label }),
	request: (id) => getRequest(id),
	media: (kind, tmdbId) => getDiscoverDetail(kind, String(tmdbId))
};

export const AGENTS_PATH = `${Route.ACTIVITY}/agents`;
export const CONNECTIONS_PATH = settingsPath(SettingsSection.CONNECTIONS);

/** A row's button: a call to the item's owner, or a link. */
export interface AttentionButton {
	label: string;
	/** Primary buttons ask for a decision; the rest fix or open something. */
	primary: boolean;
	href?: string;
	run?: () => Promise<void>;
}

/**
 * "Needs you" on Home: each open attention item with what it is about and one action that calls the
 * item's owner. Issue re-grab and transcode retry open the title until their owners' endpoints exist.
 */
export class NeedsYou {
	readonly list: AttentionList;
	readonly directory: TitleDirectory;
	/** Items with an action in flight. */
	readonly busy = new SvelteSet<string>();
	/** Pending requests' movie or series names, by request ID. */
	readonly requestNames = new SvelteMap<string, string>();

	readonly #api: NeedsYouApi;

	constructor(
		list: AttentionList,
		directory: TitleDirectory,
		api: NeedsYouApi = SDK_NEEDS_YOU_API
	) {
		this.list = list;
		this.directory = directory;
		this.#api = api;
	}

	get items(): AttentionItemResponse[] {
		return this.list.items;
	}

	/** Looks up the titles and requests the rows name. */
	async resolve(): Promise<void> {
		const titles = this.items.flatMap((item) => (item.titleId ? [item.titleId] : []));
		const requests = this.items.filter(
			(item) => item.kind === AttentionKind.REQUEST_PENDING && !this.requestNames.has(item.ref)
		);
		await Promise.all([
			this.directory.ensure(titles),
			...requests.map(async (item) => {
				try {
					const request = await this.#api.request(item.ref);
					const media = await this.#api.media(request.kind, request.tmdbId);
					this.requestNames.set(item.ref, media.name);
				} catch {
					// The row keeps its generic subject.
				}
			})
		]);
	}

	subject(item: AttentionItemResponse): string {
		if (item.titleId) {
			return this.directory.get(item.titleId)?.name ?? KIND_SUBJECTS[item.kind] ?? item.kind;
		}
		return this.requestNames.get(item.ref) ?? KIND_SUBJECTS[item.kind] ?? item.kind;
	}

	button(item: AttentionItemResponse): AttentionButton {
		const titleLink = item.titleId ? titlePath(item.titleId) : undefined;
		switch (item.action) {
			case AttentionAction.APPROVE:
				return {
					label: 'Approve',
					primary: true,
					run: () => this.#act(item, () => this.#api.approve(item.ref), 'Approved')
				};
			case AttentionAction.SEARCH_AGAIN:
				return {
					label: 'Search again',
					primary: false,
					run: () =>
						this.#act(item, () =>
							this.#api.searchAgain(item.titleId ?? item.ref, this.subject(item))
						)
				};
			case AttentionAction.VIEW_AGENT:
				return { label: 'View agent', primary: false, href: AGENTS_PATH };
			case AttentionAction.OPEN_SETTINGS:
				return { label: 'Open', primary: false, href: CONNECTIONS_PATH };
			default:
				return { label: 'Open', primary: false, href: titleLink ?? Route.HOME };
		}
	}

	async #act(
		item: AttentionItemResponse,
		action: () => Promise<unknown>,
		done?: string
	): Promise<void> {
		this.busy.add(item.id);
		try {
			await action();
			if (done) toast.success(`${done}: ${this.subject(item)}`);
			await this.list.block.refresh();
		} catch (error) {
			toast.error(apiErrorMessage(error));
		} finally {
			this.busy.delete(item.id);
		}
	}
}

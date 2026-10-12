import type { Page, Route } from '@playwright/test';
import * as data from './fake-data';

enum Method {
	GET = 'GET',
	POST = 'POST'
}

interface Endpoint {
	method?: Method;
	path: RegExp;
	/** Extra condition on the query string, for paths that serve several lists. */
	when?: (params: URLSearchParams) => boolean;
	respond: (url: URL, match: RegExpMatchArray) => unknown;
}

const NOT_FOUND = 404;
const SVG_TYPE = 'image/svg+xml';
const POSTER_PATH = /^\/api\/catalog\/images\/([^/]+)\/poster$/;

function byIds<T>(items: T[], params: URLSearchParams, key: keyof T, param: string): T[] {
	const ids = params.getAll(param).flatMap((value) => value.split(','));
	return items.filter((item) => ids.includes(String(item[key])));
}

const ENDPOINTS: Endpoint[] = [
	{ path: /^\/api\/identity\/me$/, respond: () => data.ME },
	{ path: /^\/api\/activity\/attention$/, respond: () => data.ATTENTION },
	{
		path: /^\/api\/activity\/notifications$/,
		respond: () => data.NOTIFICATIONS
	},
	{
		path: /^\/api\/activity\/pipeline$/,
		when: (params) => params.has('active'),
		respond: () => data.ACTIVE_PIPELINE
	},
	{
		path: /^\/api\/activity\/pipeline$/,
		when: (params) => params.has('recent'),
		respond: () => data.PIPELINE.filter((row) => row.episodeId === null)
	},
	{
		path: /^\/api\/activity\/pipeline$/,
		when: (params) => params.has('title'),
		respond: (url) => data.PIPELINE.filter((row) => row.titleId === url.searchParams.get('title'))
	},
	{
		path: /^\/api\/activity\/pipeline$/,
		respond: (url) => byIds(data.PIPELINE, url.searchParams, 'titleId', 'titles')
	},
	{ path: /^\/api\/activity\/timeline$/, respond: () => data.TIMELINE },
	{ path: /^\/api\/activity\/history$/, respond: () => data.TIMELINE },
	{
		path: /^\/api\/catalog\/titles\/([^/]+)$/,
		respond: (_, match) => data.TITLES.find((title) => title.id === match[1])
	},
	{
		path: /^\/api\/catalog\/titles$/,
		when: (params) => params.has('ids'),
		respond: (url) => byIds(data.TITLES, url.searchParams, 'id', 'ids')
	},
	{
		path: /^\/api\/catalog\/titles$/,
		respond: (url) => data.TITLES.filter((title) => title.kind === url.searchParams.get('kind'))
	},
	{
		path: /^\/api\/catalog\/discover\/[^/]+\/(\d+)$/,
		respond: (_, match) => data.discoverDetail(Number(match[1]))
	},
	{
		path: /^\/api\/library\/coverage$/,
		respond: (url) => byIds(data.COVERAGE, url.searchParams, 'title', 'titles')
	},
	{
		path: /^\/api\/library\/files$/,
		respond: (url) => {
			const params = url.searchParams;
			const wanted = params.has('title') ? [params.get('title') ?? ''] : params.getAll('titles');
			const ids = wanted.flatMap((value) => value.split(','));
			return data.FILED_MOVIES.filter((id) => ids.includes(id)).map(data.mediaFile);
		}
	},
	{ path: /^\/api\/library\/imports$/, respond: () => [] },
	{ path: /^\/api\/requests\/issues$/, respond: (url) => data.issueList(url.searchParams) },
	{
		path: /^\/api\/requests\/issues\/([^/]+)$/,
		respond: (_, match) => data.issueThread(match[1])
	},
	{ path: /^\/api\/requests$/, respond: (url) => data.requestList(url.searchParams) },
	{ path: /^\/api\/requests\/([^/]+)$/, respond: (_, match) => data.requestById(match[1]) },
	{ path: /^\/api\/catalog\/discover$/, respond: () => data.TRENDING },
	{ path: /^\/api\/catalog\/languages$/, respond: () => data.WANTED_LANGUAGES },
	{ path: /^\/api\/catalog\/titles\/[^/]+\/episode-groups$/, respond: () => data.EPISODE_GROUPS },
	{
		path: /^\/api\/subtitles\/files\/([^/]+)$/,
		respond: (_, match) => data.fileSubtitles(match[1])
	},
	{ path: /^\/api\/acquisition\/profiles$/, respond: () => data.PROFILES },
	{ path: /^\/api\/acquisition\/downloads$/, respond: () => data.DOWNLOADS },
	{ path: /^\/api\/acquisition\/blocklist$/, respond: () => [] },
	{
		path: /^\/api\/acquisition\/cutoff$/,
		respond: (url) => data.cutoffStatus(url.searchParams.get('title') ?? '')
	},
	{ path: /^\/api\/acquisition\/indexers$/, respond: () => data.INDEXERS },
	{ path: /^\/api\/acquisition\/indexer-options$/, respond: () => data.INDEXER_OPTIONS },
	{ method: Method.POST, path: /^\/api\/acquisition\/searches$/, respond: () => data.SEARCH },
	{ path: /^\/api\/acquisition\/searches\/[^/]+$/, respond: () => data.SEARCH },
	{ path: /^\/api\/transcode\/agents$/, respond: () => data.AGENTS },
	{ path: /^\/api\/transcode\/jobs$/, respond: () => data.JOBS },
	{ path: /^\/api\/transcode\/settings$/, respond: () => data.TRANSCODE_SETTINGS },
	{ path: /^\/api\/transcode\/backlog$/, respond: () => data.TRANSCODE_BACKLOG },
	{ path: /^\/api\/transcode\/exclusions$/, respond: () => [] }
];

function posterSvg(id: string): string {
	const colour = data.POSTER_COLOURS[id] ?? '#444444';
	return `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="300"><rect width="200" height="300" fill="${colour}"/></svg>`;
}

/**
 * Serves every /api request from fake-data and keeps the live socket open but silent.
 * Requests it has no answer for are recorded, so a test can fail when a screen calls something new.
 */
export class FakeApi {
	readonly unhandled: string[] = [];

	async install(page: Page): Promise<void> {
		await page.routeWebSocket(/\/api\/live$/, () => {});
		await page.route(/\/api\//, (route) => this.#answer(route));
	}

	async #answer(route: Route): Promise<void> {
		const request = route.request();
		const url = new URL(request.url());
		const poster = url.pathname.match(POSTER_PATH);
		if (poster) {
			return route.fulfill({ contentType: SVG_TYPE, body: posterSvg(poster[1]) });
		}
		for (const endpoint of ENDPOINTS) {
			const match = url.pathname.match(endpoint.path);
			if (
				match &&
				(endpoint.method ?? Method.GET) === request.method() &&
				(endpoint.when?.(url.searchParams) ?? true)
			) {
				return route.fulfill({ json: endpoint.respond(url, match) });
			}
		}
		this.unhandled.push(`${request.method()} ${url.pathname}${url.search}`);
		return route.fulfill({ status: NOT_FOUND, json: { title: 'Not in the fake API' } });
	}
}

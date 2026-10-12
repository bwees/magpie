import type {
	ConnectionsResponse,
	ServiceHealthResponse,
	UpdateConnectionsRequest
} from '$lib/sdk';

/** Mirrors the Go `platformapi.Service` values. */
export enum Service {
	TMDB = 'TMDB',
	QBITTORRENT = 'QBITTORRENT',
	PROWLARR = 'PROWLARR',
	JELLYFIN = 'JELLYFIN',
	OPENSUBTITLES = 'OPENSUBTITLES'
}

/** Mirrors the Go `health.Status` values. */
export enum HealthStatus {
	UNKNOWN = 'UNKNOWN',
	HEALTHY = 'HEALTHY',
	UNHEALTHY = 'UNHEALTHY',
	NOT_CONFIGURED = 'NOT_CONFIGURED'
}

/** The server sends this in place of a stored secret; sending it back keeps the secret. */
export const SECRET_MASK = '••••••••';

export const SERVICE_HEALTH_CHANGED = 'platform.ServiceHealthChanged';

export enum FieldKey {
	URL = 'url',
	API_KEY = 'apiKey',
	CATEGORY = 'category',
	USERNAME = 'username',
	PASSWORD = 'password',
	REMOTE_PATH = 'remotePath',
	LOCAL_PATH = 'localPath'
}

export interface FieldSpec {
	key: FieldKey;
	label: string;
	placeholder: string;
	secret: boolean;
}

type ConnectionKey = keyof UpdateConnectionsRequest & keyof ConnectionsResponse;

export interface ServiceSpec {
	service: Service;
	label: string;
	description: string;
	/** The body and response field that holds this service's config. */
	key: ConnectionKey;
	/** The `{service}` segment of the test route. */
	path: string;
	fields: FieldSpec[];
}

export type FieldValues = Partial<Record<FieldKey, string>>;

const URL_FIELD = (placeholder: string): FieldSpec => ({
	key: FieldKey.URL,
	label: 'URL',
	placeholder,
	secret: false
});
const API_KEY_FIELD: FieldSpec = {
	key: FieldKey.API_KEY,
	label: 'API key',
	placeholder: 'API key',
	secret: true
};

export const SERVICES: Record<Service, ServiceSpec> = {
	[Service.TMDB]: {
		service: Service.TMDB,
		label: 'TMDB',
		description: 'Titles, artwork and release dates. Get a free API key at themoviedb.org.',
		key: 'tmdb',
		path: 'tmdb',
		fields: [API_KEY_FIELD]
	},
	[Service.QBITTORRENT]: {
		service: Service.QBITTORRENT,
		label: 'qBittorrent',
		description: 'Downloads torrents. Needs qBittorrent 5.2 or newer with an API key.',
		key: 'qbittorrent',
		path: 'qbittorrent',
		fields: [
			URL_FIELD('http://qbittorrent:8080'),
			API_KEY_FIELD,
			{ key: FieldKey.CATEGORY, label: 'Category', placeholder: 'magpie', secret: false },
			{
				key: FieldKey.REMOTE_PATH,
				label: 'Download path in qBittorrent (optional)',
				placeholder: '/data/torrents',
				secret: false
			},
			{
				key: FieldKey.LOCAL_PATH,
				label: 'Same path in Magpie (optional)',
				placeholder: '/downloads',
				secret: false
			}
		]
	},
	[Service.PROWLARR]: {
		service: Service.PROWLARR,
		label: 'Prowlarr',
		description: 'Lists your indexers. Magpie searches each one through Prowlarr.',
		key: 'prowlarr',
		path: 'prowlarr',
		fields: [URL_FIELD('http://prowlarr:9696'), API_KEY_FIELD]
	},
	[Service.JELLYFIN]: {
		service: Service.JELLYFIN,
		label: 'Jellyfin',
		description: 'Sign-in for your users and library refreshes. Needs Jellyfin 12.',
		key: 'jellyfin',
		path: 'jellyfin',
		fields: [URL_FIELD('http://jellyfin:8096'), API_KEY_FIELD]
	},
	[Service.OPENSUBTITLES]: {
		service: Service.OPENSUBTITLES,
		label: 'OpenSubtitles',
		description: 'Your OpenSubtitles.com login, for subtitle downloads.',
		key: 'openSubtitles',
		path: 'opensubtitles',
		fields: [
			{ key: FieldKey.USERNAME, label: 'Username', placeholder: 'Username', secret: false },
			{ key: FieldKey.PASSWORD, label: 'Password', placeholder: 'Password', secret: true }
		]
	}
};

export const SERVICE_ORDER: Service[] = [
	Service.TMDB,
	Service.QBITTORRENT,
	Service.PROWLARR,
	Service.JELLYFIN,
	Service.OPENSUBTITLES
];

export function storedValues(spec: ServiceSpec, connections: ConnectionsResponse): FieldValues {
	return { ...connections[spec.key] } as FieldValues;
}

/** The PUT body that changes only this service. */
export function updateFor(spec: ServiceSpec, values: FieldValues): UpdateConnectionsRequest {
	return { [spec.key]: values } as UpdateConnectionsRequest;
}

export function unknownHealth(service: Service): ServiceHealthResponse {
	return { service, status: HealthStatus.UNKNOWN, version: '', detail: '', error: '' };
}

/** The toast line for a passing test: detected version, then what the probe found. */
export function testSummary(health: ServiceHealthResponse): string {
	return [health.version && `Version ${health.version}`, health.detail].filter(Boolean).join(' · ');
}

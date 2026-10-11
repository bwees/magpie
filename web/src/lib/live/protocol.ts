// Mirrors server/internal/platform/live/protocol.go. The socket is not REST, so it is not in the OpenAPI spec.

export const LIVE_PATH = '/api/live';

export enum LiveControlType {
	HEARTBEAT = 'live.Heartbeat',
	RESET = 'live.Reset'
}

export enum LiveOp {
	RESUME = 'resume',
	SUBSCRIBE = 'subscribe',
	UNSUBSCRIBE = 'unsubscribe',
	PONG = 'pong'
}

export const SEARCH_TOPIC_PREFIX = 'search:';

export interface LiveMessage {
	seq: number;
	epoch: string;
	type: string;
	topic: string;
	data: unknown;
}

export interface LiveClientMessage {
	op: LiveOp;
	epoch?: string;
	seq?: number;
	topics?: string[];
}

export function searchTopic(searchId: string): string {
	return SEARCH_TOPIC_PREFIX + searchId;
}

/** The socket URL: same host as the API, with ws or wss to match http or https. */
export function liveUrl(apiBaseUrl: string, pageUrl: string): string {
	const url = new URL(LIVE_PATH, apiBaseUrl || pageUrl);
	url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
	return url.toString();
}

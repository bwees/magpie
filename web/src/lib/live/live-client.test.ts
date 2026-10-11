import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	INITIAL_RECONNECT_DELAY_MS,
	MAX_RECONNECT_DELAY_MS,
	nextReconnectDelay,
	withJitter
} from './backoff';
import { LiveClient, LiveStatus } from './live-client.svelte';
import {
	LiveControlType,
	LiveOp,
	liveUrl,
	searchTopic,
	type LiveClientMessage,
	type LiveMessage
} from './protocol';

const URL = 'ws://magpie.test/api/live';
const EPOCH = '01JEPOCH';
const FULL_DELAY = () => 1;
const EVENT_TYPE = 'library.FileImported';

class FakeSocket {
	sent: LiveClientMessage[] = [];
	onopen: (() => void) | null = null;
	onmessage: ((event: { data: string }) => void) | null = null;
	onclose: (() => void) | null = null;

	send(data: string) {
		this.sent.push(JSON.parse(data));
	}

	close() {
		this.onclose?.();
	}

	open() {
		this.onopen?.();
	}

	receive(message: Partial<LiveMessage>) {
		this.onmessage?.({
			data: JSON.stringify({ seq: 0, epoch: EPOCH, topic: '', data: null, ...message })
		});
	}
}

function setup() {
	const sockets: FakeSocket[] = [];
	const client = new LiveClient({
		url: URL,
		random: FULL_DELAY,
		createSocket: () => {
			const socket = new FakeSocket();
			sockets.push(socket);
			return socket as unknown as WebSocket;
		}
	});
	const latest = () => sockets[sockets.length - 1];
	return { client, sockets, latest };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe('LiveClient', () => {
	it('resumes from nothing on first connect', () => {
		const { client, latest } = setup();
		client.connect();
		latest().open();

		expect(client.status).toBe(LiveStatus.OPEN);
		expect(latest().sent).toEqual([{ op: LiveOp.RESUME, epoch: '', seq: 0 }]);
	});

	it('dispatches messages by event type', () => {
		const { client, latest } = setup();
		const imported = vi.fn();
		const other = vi.fn();
		client.on(EVENT_TYPE, imported);
		const stop = client.on('catalog.TitleAdded', other);
		stop();
		client.connect();
		latest().open();

		latest().receive({ type: EVENT_TYPE, seq: 1, data: { title: 't1' } });
		latest().receive({ type: 'catalog.TitleAdded', seq: 2 });

		expect(imported).toHaveBeenCalledWith(
			expect.objectContaining({ seq: 1, data: { title: 't1' } })
		);
		expect(other).not.toHaveBeenCalled();
	});

	it('answers heartbeats with pong', () => {
		const { client, latest } = setup();
		client.connect();
		latest().open();

		latest().receive({ type: LiveControlType.HEARTBEAT, seq: 9 });

		expect(latest().sent.at(-1)).toEqual({ op: LiveOp.PONG });
	});

	it('notifies views on reset and resumes from the last message after reconnecting', () => {
		const { client, latest } = setup();
		const refetch = vi.fn();
		client.onReset(refetch);
		client.connect();
		latest().open();

		latest().receive({ type: LiveControlType.RESET, seq: 4 });
		latest().receive({ type: EVENT_TYPE, seq: 5 });
		latest().receive({ type: LiveControlType.HEARTBEAT, seq: 7 });
		expect(refetch).toHaveBeenCalledOnce();

		latest().close();
		vi.advanceTimersByTime(INITIAL_RECONNECT_DELAY_MS);
		latest().open();

		expect(latest().sent).toEqual([{ op: LiveOp.RESUME, epoch: EPOCH, seq: 5 }]);
	});

	it('reconnects with doubling delays up to the cap and shows reconnecting', () => {
		const { client, sockets, latest } = setup();
		client.connect();
		latest().open();

		const expected = [1_000, 2_000, 4_000, 8_000, 16_000, 30_000, 30_000];
		for (const delay of expected) {
			const before = sockets.length;
			latest().close();
			expect(client.status).toBe(LiveStatus.RECONNECTING);
			vi.advanceTimersByTime(delay - 1);
			expect(sockets.length).toBe(before);
			vi.advanceTimersByTime(1);
			expect(sockets.length).toBe(before + 1);
		}

		latest().open();
		const before = sockets.length;
		latest().close();
		vi.advanceTimersByTime(INITIAL_RECONNECT_DELAY_MS);
		expect(sockets.length).toBe(before + 1);
	});

	it('stays connecting, not reconnecting, until the first connection opens', () => {
		const { client, latest } = setup();
		client.connect();

		latest().close();

		expect(client.status).toBe(LiveStatus.CONNECTING);
	});

	it('subscribes to topics and restores them before resuming', () => {
		const { client, latest } = setup();
		const topic = searchTopic('01JSEARCH');
		client.connect();
		latest().open();

		client.subscribe(topic);
		expect(latest().sent.at(-1)).toEqual({ op: LiveOp.SUBSCRIBE, topics: [topic] });

		latest().close();
		vi.advanceTimersByTime(INITIAL_RECONNECT_DELAY_MS);
		latest().open();
		expect(latest().sent).toEqual([
			{ op: LiveOp.SUBSCRIBE, topics: [topic] },
			{ op: LiveOp.RESUME, epoch: '', seq: 0 }
		]);

		client.unsubscribe(topic);
		expect(latest().sent.at(-1)).toEqual({ op: LiveOp.UNSUBSCRIBE, topics: [topic] });
	});

	it('stops reconnecting once closed', () => {
		const { client, sockets, latest } = setup();
		client.connect();
		latest().open();

		client.close();
		vi.advanceTimersByTime(MAX_RECONNECT_DELAY_MS);

		expect(client.status).toBe(LiveStatus.CLOSED);
		expect(sockets).toHaveLength(1);
	});
});

describe('backoff', () => {
	it('doubles from 1 s and caps at 30 s', () => {
		const delays = [INITIAL_RECONNECT_DELAY_MS];
		while (delays.length < 7) delays.push(nextReconnectDelay(delays[delays.length - 1]));
		expect(delays).toEqual([1_000, 2_000, 4_000, 8_000, 16_000, 30_000, 30_000]);
	});

	it('jitters between half and all of the delay', () => {
		expect(withJitter(10_000, () => 0)).toBe(5_000);
		expect(withJitter(10_000, () => 1)).toBe(10_000);
	});
});

describe('liveUrl', () => {
	it('follows the page scheme and host when no API base is set', () => {
		expect(liveUrl('', 'https://magpie.example.com/movies')).toBe(
			'wss://magpie.example.com/api/live'
		);
		expect(liveUrl('', 'http://localhost:5173/')).toBe('ws://localhost:5173/api/live');
	});

	it('uses the API base when set', () => {
		expect(liveUrl('http://127.0.0.1:3000', 'http://localhost:5173/')).toBe(
			'ws://127.0.0.1:3000/api/live'
		);
	});
});

import { INITIAL_RECONNECT_DELAY_MS, nextReconnectDelay, withJitter } from './backoff';
import { LiveControlType, LiveOp, type LiveClientMessage, type LiveMessage } from './protocol';

export enum LiveStatus {
	CONNECTING = 'CONNECTING',
	OPEN = 'OPEN',
	RECONNECTING = 'RECONNECTING',
	CLOSED = 'CLOSED'
}

export type LiveHandler = (message: LiveMessage) => void;
export type ResetListener = () => void;

export interface LiveConnectionOptions {
	url: string;
	createSocket?: (url: string) => WebSocket;
	random?: () => number;
}

/**
 * The app-wide live socket. It resumes from the last seen message after a reconnect,
 * hands each message to the handlers for its type, and tells views to refetch on a reset.
 */
export class LiveConnection {
	readonly #url: string;
	readonly #createSocket: (url: string) => WebSocket;
	readonly #random: () => number;
	readonly #handlers = new Map<string, Set<LiveHandler>>();
	readonly #resetListeners = new Set<ResetListener>();
	readonly #topics = new Set<string>();

	#status = LiveStatus.CONNECTING;
	#socket: WebSocket | undefined;
	#retryTimer: ReturnType<typeof setTimeout> | undefined;
	#delay = INITIAL_RECONNECT_DELAY_MS;
	#epoch = '';
	#seq = 0;
	#hasConnected = false;
	#stopped = false;

	constructor(options: LiveConnectionOptions) {
		this.#url = options.url;
		this.#createSocket = options.createSocket ?? ((url) => new WebSocket(url));
		this.#random = options.random ?? Math.random;
	}

	get status(): LiveStatus {
		return this.#status;
	}

	protected set status(status: LiveStatus) {
		this.#status = status;
	}

	connect(): void {
		this.#stopped = false;
		this.#open();
	}

	close(): void {
		this.#stopped = true;
		clearTimeout(this.#retryTimer);
		this.#socket?.close();
		this.#socket = undefined;
		this.status = LiveStatus.CLOSED;
	}

	/** Calls handler for every message of type until the returned function is called. */
	on(type: string, handler: LiveHandler): () => void {
		const handlers = this.#handlers.get(type) ?? new Set<LiveHandler>();
		handlers.add(handler);
		this.#handlers.set(type, handlers);
		return () => handlers.delete(handler);
	}

	/** Calls listener when the server could not replay what was missed; views should refetch. */
	onReset(listener: ResetListener): () => void {
		this.#resetListeners.add(listener);
		return () => this.#resetListeners.delete(listener);
	}

	subscribe(topic: string): void {
		this.#topics.add(topic);
		this.#send({ op: LiveOp.SUBSCRIBE, topics: [topic] });
	}

	unsubscribe(topic: string): void {
		this.#topics.delete(topic);
		this.#send({ op: LiveOp.UNSUBSCRIBE, topics: [topic] });
	}

	#open(): void {
		const socket = this.#createSocket(this.#url);
		this.#socket = socket;
		socket.onopen = () => this.#opened();
		socket.onmessage = (event) => this.#received(event.data);
		socket.onclose = () => this.#closed(socket);
	}

	#opened(): void {
		this.status = LiveStatus.OPEN;
		this.#hasConnected = true;
		this.#delay = INITIAL_RECONNECT_DELAY_MS;
		// Topics go first so the server filters the replay for them.
		if (this.#topics.size > 0) {
			this.#send({ op: LiveOp.SUBSCRIBE, topics: [...this.#topics] });
		}
		this.#send({ op: LiveOp.RESUME, epoch: this.#epoch, seq: this.#seq });
	}

	#received(raw: unknown): void {
		const message = JSON.parse(String(raw)) as LiveMessage;
		switch (message.type) {
			case LiveControlType.HEARTBEAT:
				this.#send({ op: LiveOp.PONG });
				return;
			case LiveControlType.RESET:
				this.#remember(message);
				for (const listener of this.#resetListeners) listener();
				return;
			default:
				this.#remember(message);
				for (const handler of this.#handlers.get(message.type) ?? []) handler(message);
		}
	}

	#remember(message: LiveMessage): void {
		this.#epoch = message.epoch;
		this.#seq = message.seq;
	}

	#closed(socket: WebSocket): void {
		if (socket !== this.#socket || this.#stopped) return;
		this.#socket = undefined;
		this.status = this.#hasConnected ? LiveStatus.RECONNECTING : LiveStatus.CONNECTING;
		this.#retryTimer = setTimeout(() => this.#open(), withJitter(this.#delay, this.#random));
		this.#delay = nextReconnectDelay(this.#delay);
	}

	#send(message: LiveClientMessage): void {
		if (this.status === LiveStatus.OPEN) {
			this.#socket?.send(JSON.stringify(message));
		}
	}
}

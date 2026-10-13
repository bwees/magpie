import { apiErrorMessage, type NotificationCreated, type NotificationResponse } from '$lib/sdk';
import type { LiveEvents } from '$lib/activity/activity-source';
import { LoadStatus } from '$lib/load-status';
import { NotificationEvent, type NotificationSource } from './notification-source';

/** The bell: the user's latest notifications and unread count, refetched when a new one arrives. */
export class NotificationInbox {
	items = $state<NotificationResponse[]>([]);
	unreadCount = $state(0);
	status = $state(LoadStatus.LOADING);
	error = $state('');

	readonly #source: NotificationSource;
	#latest: Promise<unknown> | undefined;

	constructor(source: NotificationSource) {
		this.#source = source;
	}

	get hasUnread(): boolean {
		return this.unreadCount > 0;
	}

	get empty(): boolean {
		return this.status === LoadStatus.READY && this.items.length === 0;
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		await this.refresh();
	}

	async refresh(): Promise<void> {
		const request = this.#source.list();
		this.#latest = request;
		try {
			const list = await request;
			if (this.#latest !== request) return;
			this.items = list.notifications;
			this.unreadCount = list.unreadCount;
			this.error = '';
			this.status = LoadStatus.READY;
		} catch (error) {
			if (this.#latest !== request) return;
			this.error = apiErrorMessage(error, 'Could not load notifications.');
			if (this.status === LoadStatus.LOADING || this.items.length === 0) {
				this.status = LoadStatus.ERROR;
			}
		}
	}

	async markRead(item: NotificationResponse): Promise<void> {
		if (item.read) return;
		item.read = true;
		this.unreadCount = Math.max(0, this.unreadCount - 1);
		try {
			await this.#source.markRead(item.id);
		} catch {
			await this.refresh();
		}
	}

	async markAllRead(): Promise<void> {
		if (!this.hasUnread) return;
		for (const item of this.items) item.read = true;
		this.unreadCount = 0;
		try {
			await this.#source.markAllRead();
		} catch {
			await this.refresh();
		}
	}

	/** Refetches on every new notification and on a live reset. Calls onCreated with each new one. */
	connect(live: LiveEvents, onCreated?: (created: NotificationCreated) => void): () => void {
		const stops = [
			live.on(NotificationEvent.CREATED, (message) => {
				onCreated?.(message.data as NotificationCreated);
				void this.refresh();
			}),
			live.onReset(() => void this.refresh())
		];
		return () => stops.forEach((stop) => stop());
	}
}

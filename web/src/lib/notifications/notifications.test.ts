import { describe, expect, it, vi } from 'vitest';
import type { LiveHandler, ResetListener } from '$lib/live/live-connection';
import type { LiveMessage } from '$lib/live/protocol';
import { LoadStatus } from '$lib/load-status';
import type {
	NotificationKindResponse,
	NotificationResponse,
	NotificationUrlResponse
} from '$lib/sdk';
import type { LiveEvents } from '$lib/activity/activity-source';
import { NotificationInbox } from './notification-inbox.svelte';
import {
	KindScope,
	MAX_NOTIFICATION_URLS,
	NotificationEvent,
	type NotificationSource
} from './notification-source';
import { groupKinds, NotificationSettings, TestOutcome } from './notification-settings.svelte';

const AT = '2026-10-11T12:00:00Z';
const URL_ID = '01URL';
const NTFY = 'ntfy://ntfy.sh/topic';
const IMPORT_FAILED = 'IMPORT_FAILED';
const REQUEST_AVAILABLE = 'REQUEST_AVAILABLE';
const GRABBED = 'GRABBED';

class FakeLive implements LiveEvents {
	readonly handlers = new Map<string, Set<LiveHandler>>();
	readonly resets = new Set<ResetListener>();

	on(type: string, handler: LiveHandler): () => void {
		const handlers = this.handlers.get(type) ?? new Set<LiveHandler>();
		handlers.add(handler);
		this.handlers.set(type, handlers);
		return () => handlers.delete(handler);
	}

	onReset(listener: ResetListener): () => void {
		this.resets.add(listener);
		return () => this.resets.delete(listener);
	}

	emit(type: string, data: unknown): void {
		const message: LiveMessage = { seq: 1, epoch: 'E', type, topic: '', data };
		this.handlers.get(type)?.forEach((handler) => handler(message));
	}
}

function notification(id: string, read = false): NotificationResponse {
	return {
		id,
		kind: REQUEST_AVAILABLE,
		title: id,
		body: '',
		link: '/titles/1',
		createdAt: AT,
		read
	};
}

function stored(overrides: Partial<NotificationUrlResponse> = {}): NotificationUrlResponse {
	return {
		id: URL_ID,
		label: 'Ntfy',
		maskedUrl: 'ntfy://ntfy.sh…',
		kinds: [],
		createdAt: AT,
		lastSentAt: null,
		lastError: '',
		...overrides
	};
}

function kind(name: string, scope: KindScope, optIn = false): NotificationKindResponse {
	return { kind: name, label: name, scope, optIn };
}

function source(overrides: Partial<NotificationSource> = {}): NotificationSource {
	return {
		list: vi.fn(async () => ({ notifications: [], unreadCount: 0 })),
		markRead: vi.fn(async () => undefined),
		markAllRead: vi.fn(async () => undefined),
		kinds: vi.fn(async () => []),
		urls: vi.fn(async () => []),
		addUrl: vi.fn(async () => stored()),
		updateUrl: vi.fn(async () => stored()),
		deleteUrl: vi.fn(async () => undefined),
		testUrl: vi.fn(async () => undefined),
		...overrides
	};
}

function problem(status: number, detail: string) {
	return { status, data: { detail } };
}

describe('NotificationInbox', () => {
	it('loads the newest notifications and the unread count', async () => {
		const inbox = new NotificationInbox(
			source({
				list: async () => ({
					notifications: [notification('a'), notification('b', true)],
					unreadCount: 1
				})
			})
		);
		await inbox.load();
		expect(inbox.status).toBe(LoadStatus.READY);
		expect(inbox.items.map((item) => item.id)).toEqual(['a', 'b']);
		expect(inbox.hasUnread).toBe(true);
	});

	it('is empty when there are no notifications', async () => {
		const inbox = new NotificationInbox(source());
		await inbox.load();
		expect(inbox.empty).toBe(true);
		expect(inbox.hasUnread).toBe(false);
	});

	it('shows an error when the first load fails', async () => {
		const inbox = new NotificationInbox(
			source({ list: async () => Promise.reject(problem(500, 'database gone')) })
		);
		await inbox.load();
		expect(inbox.status).toBe(LoadStatus.ERROR);
		expect(inbox.error).toBe('database gone');
	});

	it('marks one read once', async () => {
		const markRead = vi.fn(async () => undefined);
		const inbox = new NotificationInbox(
			source({
				list: async () => ({ notifications: [notification('a')], unreadCount: 1 }),
				markRead
			})
		);
		await inbox.load();
		await inbox.markRead(inbox.items[0]);
		await inbox.markRead(inbox.items[0]);
		expect(markRead).toHaveBeenCalledTimes(1);
		expect(inbox.items[0].read).toBe(true);
		expect(inbox.hasUnread).toBe(false);
	});

	it('marks all read', async () => {
		const markAllRead = vi.fn(async () => undefined);
		const inbox = new NotificationInbox(
			source({
				list: async () => ({
					notifications: [notification('a'), notification('b')],
					unreadCount: 2
				}),
				markAllRead
			})
		);
		await inbox.load();
		await inbox.markAllRead();
		expect(markAllRead).toHaveBeenCalledOnce();
		expect(inbox.items.every((item) => item.read)).toBe(true);
		expect(inbox.unreadCount).toBe(0);
	});

	it('refetches and reports each new notification from the live socket', async () => {
		const list = vi.fn(async () => ({ notifications: [], unreadCount: 0 }));
		const inbox = new NotificationInbox(source({ list }));
		const live = new FakeLive();
		const created = vi.fn();
		const stop = inbox.connect(live, created);

		live.emit(NotificationEvent.CREATED, { title: 'Dune is available' });
		expect(created).toHaveBeenCalledWith({ title: 'Dune is available' });
		expect(list).toHaveBeenCalledOnce();
		live.resets.forEach((reset) => reset());
		expect(list).toHaveBeenCalledTimes(2);

		stop();
		live.emit(NotificationEvent.CREATED, {});
		expect(list).toHaveBeenCalledTimes(2);
	});
});

describe('groupKinds', () => {
	const kinds = [
		kind(REQUEST_AVAILABLE, KindScope.REQUESTER),
		kind(IMPORT_FAILED, KindScope.ADMIN),
		kind(GRABBED, KindScope.ADMIN, true)
	];

	it('groups admin kinds and drops empty groups for requesters', () => {
		expect(groupKinds(kinds, true).map((group) => group.kinds.map((k) => k.kind))).toEqual([
			[REQUEST_AVAILABLE],
			[IMPORT_FAILED],
			[GRABBED]
		]);
		expect(groupKinds(kinds.slice(0, 1), false)).toHaveLength(1);
	});
});

describe('NotificationSettings', () => {
	it('adds a URL and clears the field', async () => {
		const addUrl = vi.fn(async () => stored());
		const settings = new NotificationSettings(source({ addUrl }), false);
		await settings.load();
		settings.newUrl = ` ${NTFY} `;
		await settings.add();
		expect(addUrl).toHaveBeenCalledWith({ label: '', url: NTFY, kinds: [] });
		expect(settings.cards).toHaveLength(1);
		expect(settings.newUrl).toBe('');
	});

	it('keeps the field and shows the error when the URL is invalid', async () => {
		const settings = new NotificationSettings(
			source({ addUrl: async () => Promise.reject(problem(422, 'not a URL Magpie can use')) }),
			false
		);
		await settings.load();
		settings.newUrl = 'gopher://x';
		await settings.add();
		expect(settings.addError).toBe('not a URL Magpie can use');
		expect(settings.newUrl).toBe('gopher://x');
		expect(settings.cards).toHaveLength(0);
	});

	it('stops adding at the limit', async () => {
		const urls = Array.from({ length: MAX_NOTIFICATION_URLS }, (_, i) => stored({ id: String(i) }));
		const addUrl = vi.fn(async () => stored());
		const settings = new NotificationSettings(source({ urls: async () => urls, addUrl }), true);
		await settings.load();
		settings.newUrl = NTFY;
		await settings.add();
		expect(settings.atLimit).toBe(true);
		expect(addUrl).not.toHaveBeenCalled();
	});

	it('prefills a scheme only into an empty field', () => {
		const settings = new NotificationSettings(source(), false);
		settings.useScheme('discord://');
		settings.useScheme('ntfy://');
		expect(settings.newUrl).toBe('discord://');
	});

	it('ticks a kind and rolls back when the save fails', async () => {
		const updateUrl = vi.fn(async () => stored({ kinds: [REQUEST_AVAILABLE] }));
		const settings = new NotificationSettings(
			source({ urls: async () => [stored()], updateUrl }),
			false
		);
		await settings.load();
		const [card] = settings.cards;
		await settings.setKind(card, REQUEST_AVAILABLE, true);
		expect(updateUrl).toHaveBeenCalledWith(URL_ID, { kinds: [REQUEST_AVAILABLE] });
		expect(card.subscribes(REQUEST_AVAILABLE)).toBe(true);

		updateUrl.mockRejectedValueOnce(problem(422, 'not available to your account'));
		await settings.setKind(card, IMPORT_FAILED, true);
		expect(card.subscribes(IMPORT_FAILED)).toBe(false);
		expect(card.error).toBe('not available to your account');
	});

	it('saves a changed label without touching the kinds', async () => {
		const updateUrl = vi.fn(async () => stored({ label: 'Phone' }));
		const settings = new NotificationSettings(
			source({ urls: async () => [stored()], updateUrl }),
			false
		);
		await settings.load();
		const [card] = settings.cards;
		await settings.saveLabel(card);
		expect(updateUrl).not.toHaveBeenCalled();
		card.label = ' Phone ';
		await settings.saveLabel(card);
		expect(updateUrl).toHaveBeenCalledWith(URL_ID, { label: 'Phone', kinds: null });
		expect(card.label).toBe('Phone');
	});

	it('reports the test outcome', async () => {
		const testUrl = vi.fn(async () => undefined);
		const settings = new NotificationSettings(
			source({ urls: async () => [stored()], testUrl }),
			false
		);
		await settings.load();
		const [card] = settings.cards;
		await settings.test(card);
		expect(card.testOutcome).toBe(TestOutcome.SENT);

		testUrl.mockRejectedValueOnce(problem(502, "couldn't be reached"));
		await settings.test(card);
		expect(card.testOutcome).toBe(TestOutcome.FAILED);
		expect(card.testMessage).toBe("couldn't be reached");
	});

	it('removes a deleted card', async () => {
		const settings = new NotificationSettings(source({ urls: async () => [stored()] }), false);
		await settings.load();
		await settings.remove(settings.cards[0]);
		expect(settings.cards).toHaveLength(0);
	});
});

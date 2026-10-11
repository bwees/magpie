import {
	addNotificationUrl,
	deleteNotificationUrl,
	listNotificationKinds,
	listNotifications,
	listNotificationUrls,
	markAllNotificationsRead,
	markNotificationRead,
	testNotificationUrl,
	updateNotificationUrl,
	type NotificationKindResponse,
	type NotificationListResponse,
	type NotificationUrlChangeRequest,
	type NotificationUrlRequest,
	type NotificationUrlResponse
} from '$lib/sdk';

/** Mirrors the Go `activity.MaxNotificationURLs`. */
export const MAX_NOTIFICATION_URLS = 10;

/** How many notifications the bell shows. */
export const BELL_PAGE_SIZE = 20;

/** Services the server accepts, as the add field suggests them. Mirrors the Go `shoutrrr.Schemes`. */
export const EXAMPLE_SCHEMES = ['discord://', 'telegram://', 'ntfy://', 'pushover://', 'smtp://'];

export const SHOUTRRR_DOCS_URL = 'https://shoutrrr.nickfedor.com/';

export enum NotificationEvent {
	CREATED = 'activity.NotificationCreated'
}

/** Mirrors the Go `activity.KindScope` values. */
export enum KindScope {
	REQUESTER = 'REQUESTER',
	ADMIN = 'ADMIN'
}

/** Where the bell and Settings → Notifications read and write. The SDK in the app, a fake in tests. */
export interface NotificationSource {
	list(): Promise<NotificationListResponse>;
	markRead(id: string): Promise<unknown>;
	markAllRead(): Promise<unknown>;
	kinds(): Promise<NotificationKindResponse[]>;
	urls(): Promise<NotificationUrlResponse[]>;
	addUrl(request: NotificationUrlRequest): Promise<NotificationUrlResponse>;
	updateUrl(id: string, change: NotificationUrlChangeRequest): Promise<NotificationUrlResponse>;
	deleteUrl(id: string): Promise<unknown>;
	testUrl(id: string): Promise<unknown>;
}

export const SDK_NOTIFICATIONS: NotificationSource = {
	list: () => listNotifications({ limit: BELL_PAGE_SIZE }),
	markRead: (id) => markNotificationRead(id),
	markAllRead: () => markAllNotificationsRead(),
	kinds: () => listNotificationKinds(),
	urls: () => listNotificationUrls(),
	addUrl: (request) => addNotificationUrl(request),
	updateUrl: (id, change) => updateNotificationUrl(id, change),
	deleteUrl: (id) => deleteNotificationUrl(id),
	testUrl: (id) => testNotificationUrl(id)
};

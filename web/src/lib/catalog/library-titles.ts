import { listTitles, type TitleResponse } from '$lib/sdk';
import type { MediaKind } from './title-page.svelte';

/** Mirrors the Go `catalog.TitleSort` values. ADDED and YEAR list the newest first. */
export enum TitleSort {
	NAME = 'NAME',
	ADDED = 'ADDED',
	YEAR = 'YEAR'
}

/** Lists the library's titles of one kind. The SDK in the app, a fake in tests. */
export type TitleLister = (kind: MediaKind, sort: TitleSort) => Promise<TitleResponse[]>;

export const SDK_TITLE_LISTER: TitleLister = (kind, sort) => listTitles({ kind, sort });

/** Catalog events that add, remove or reshape titles in the library lists. */
export enum CatalogEvent {
	TITLE_ADDED = 'catalog.TitleAdded',
	TITLE_REMOVED = 'catalog.TitleRemoved',
	PROFILE_CHANGED = 'catalog.ProfileChanged',
	MONITORING_CHANGED = 'catalog.MonitoringChanged',
	EPISODES_ANNOUNCED = 'catalog.EpisodesAnnounced',
	WANTED_RELEASED = 'catalog.WantedReleased',
	EPISODE_GROUP_CHANGED = 'catalog.EpisodeGroupChanged'
}

export const CATALOG_EVENTS: CatalogEvent[] = Object.values(CatalogEvent);

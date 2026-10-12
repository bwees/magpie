import { createContext } from 'svelte';
import { IsMobile } from '$lib/hooks/is-mobile.svelte';
import { SIDEBAR_COOKIE_NAME } from '$lib/components/ui/sidebar/constants';
import { TitleSearch } from '$lib/catalog/title-search.svelte';
import { PendingRequests } from '$lib/requests/pending-requests.svelte';
import { SearchRequests } from '$lib/requests/search-requests.svelte';
import { BadgeSource, NavAction, navItemFor, type NavItem } from '$lib/navigation/nav';
import type { LiveEvents } from '$lib/activity/activity-source';
import { AttentionList } from '$lib/activity/attention-list.svelte';
import { LivePipeline } from '$lib/activity/live-pipeline.svelte';

export enum ToastPosition {
	DESKTOP = 'bottom-right',
	PHONE = 'top-center'
}

const SIDEBAR_EXPANDED = 'true';

function readSidebarExpanded(): boolean {
	return document.cookie
		.split('; ')
		.some((cookie) => cookie === `${SIDEBAR_COOKIE_NAME}=${SIDEBAR_EXPANDED}`);
}

export class ShellState {
	readonly #isMobile = new IsMobile();

	sidebarOpen = $state(readSidebarExpanded());
	readonly pendingRequests = new PendingRequests();
	/** "Needs you" and the Home badge; admins only. */
	readonly attention = new AttentionList();
	/** "Live now" and the Activity badge; admins only. */
	readonly livePipeline = new LivePipeline();
	badges: Record<BadgeSource, number> = $derived({
		[BadgeSource.ATTENTION]: this.attention.count,
		[BadgeSource.PENDING_REQUESTS]: this.pendingRequests.count,
		[BadgeSource.LIVE_JOBS]: this.livePipeline.count
	});

	/** Loads the badges and follows them until the returned function is called. */
	watchBadges(live: LiveEvents, admin: boolean): () => void {
		void this.pendingRequests.load();
		const stops = [this.pendingRequests.watch(live)];
		if (admin) {
			void this.attention.load();
			void this.livePipeline.load();
			stops.push(this.attention.watch(live), this.livePipeline.watch(live));
		}
		return () => stops.forEach((stop) => stop());
	}

	get isMobile(): boolean {
		return this.#isMobile.current;
	}

	get toastPosition(): ToastPosition {
		return this.isMobile ? ToastPosition.PHONE : ToastPosition.DESKTOP;
	}

	/** The top-bar search, shared so the requester's Search tab can open it. */
	readonly search = new TitleSearch();
	/** Add and Request on the search's TMDB cards. */
	readonly searchRequests = new SearchRequests();

	run(action: NavAction): void {
		switch (action) {
			case NavAction.SEARCH:
				this.search.show();
				break;
		}
	}

	badgeFor(item: NavItem): number {
		return item.badge ? this.badges[item.badge] : 0;
	}

	pageFor(pathname: string): NavItem | undefined {
		return navItemFor(pathname);
	}
}

export const [getShell, setShell] = createContext<ShellState>();

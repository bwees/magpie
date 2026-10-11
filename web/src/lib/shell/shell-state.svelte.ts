import { createContext } from 'svelte';
import { IsMobile } from '$lib/hooks/is-mobile.svelte';
import { SIDEBAR_COOKIE_NAME } from '$lib/components/ui/sidebar/constants';
import { TitleSearch } from '$lib/catalog/title-search.svelte';
import { BadgeSource, NavAction, navItemFor, type NavItem } from '$lib/navigation/nav';

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
	badges = $state<Record<BadgeSource, number>>({
		[BadgeSource.ATTENTION]: 0,
		[BadgeSource.PENDING_REQUESTS]: 0,
		[BadgeSource.LIVE_JOBS]: 0
	});

	get isMobile(): boolean {
		return this.#isMobile.current;
	}

	get toastPosition(): ToastPosition {
		return this.isMobile ? ToastPosition.PHONE : ToastPosition.DESKTOP;
	}

	/** The top-bar search, shared so the requester's Search tab can open it. */
	readonly search = new TitleSearch();

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

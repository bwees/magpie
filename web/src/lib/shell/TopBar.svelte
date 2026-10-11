<script lang="ts">
	import { page } from '$app/state';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import SearchOverlay from '$lib/catalog/SearchOverlay.svelte';
	import TopSearch from '$lib/catalog/TopSearch.svelte';
	import { getLive, LiveStatus } from '$lib/live/live-client.svelte';
	import NotificationBell from '$lib/notifications/NotificationBell.svelte';
	import AvatarMenu from './AvatarMenu.svelte';
	import { getShell } from './shell-state.svelte';

	const shell = getShell();
	const live = getLive();
	const current = $derived(shell.pageFor(page.url.pathname));
	const search = shell.search;
</script>

<header class="sticky top-0 z-30 border-b bg-background/80 backdrop-blur">
	<div class="flex h-14 items-center gap-3 px-4 md:gap-4 md:px-6">
		{#if current}
			<h1 class="truncate text-sm font-medium">{current.label}</h1>
		{:else}
			<button
				type="button"
				class="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
				onclick={() => history.back()}
			>
				<ChevronLeft class="size-4" />
				Back
			</button>
		{/if}

		<div class="ml-auto flex items-center gap-1 md:gap-3">
			{#if live.status === LiveStatus.RECONNECTING}
				<span
					class="flex items-center gap-1.5 text-xs text-muted-foreground"
					role="status"
					aria-live="polite"
				>
					<LoaderCircle class="size-3.5 animate-spin" />
					<span class="hidden sm:inline">Reconnecting…</span>
				</span>
			{/if}
			{#if shell.isMobile}
				<SearchOverlay {search} />
			{:else}
				<TopSearch {search} />
			{/if}

			<NotificationBell />

			<AvatarMenu />
		</div>
	</div>
</header>

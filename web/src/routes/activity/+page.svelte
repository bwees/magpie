<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { Badge } from '$lib/components/ui/badge';
	import * as Tabs from '$lib/components/ui/tabs';
	import HistoryTab from '$lib/activity/HistoryTab.svelte';
	import BlocklistTab from '$lib/downloads/BlocklistTab.svelte';
	import DownloadsTab from '$lib/downloads/DownloadsTab.svelte';
	import ImportsTab from '$lib/library/ImportsTab.svelte';
	import { ImportsManager } from '$lib/library/imports-manager.svelte';
	import AgentsTab from '$lib/transcode/AgentsTab.svelte';
	import GpuJobsTab from '$lib/transcode/GpuJobsTab.svelte';
	import {
		activityTabsFor,
		ACTIVITY_TAB_QUERY,
		ActivityTab,
		TAB_LABELS
	} from '$lib/activity/activity-tabs';
	import { getLive } from '$lib/live/live-client.svelte';
	import { NAV_ITEMS } from '$lib/navigation/nav';
	import { getSession } from '$lib/session/session.svelte';

	const session = getSession();
	const tabs = activityTabsFor(session.isAdmin);
	const linked = page.url.searchParams.get(ACTIVITY_TAB_QUERY) as ActivityTab | null;
	let tab = $state(linked && tabs.includes(linked) ? linked : ActivityTab.DOWNLOADS);

	const imports = new ImportsManager();
	const live = getLive();

	onMount(() => {
		if (!session.isAdmin) return;
		void imports.load();
		return imports.watch(live);
	});
</script>

<svelte:head>
	<title>{NAV_ITEMS.ACTIVITY.label} · Magpie</title>
</svelte:head>

<Tabs.Root bind:value={tab} class="gap-4">
	<div class="-mx-4 overflow-x-auto px-4 md:mx-0 md:px-0">
		<Tabs.List>
			{#each tabs as value (value)}
				<Tabs.Trigger {value}>
					{TAB_LABELS[value]}
					{#if value === ActivityTab.IMPORTS && imports.count > 0}
						<Badge
							class="ml-1 h-5 min-w-5 px-1.5 tabular-nums"
							aria-label="{imports.count} to review"
						>
							{imports.count}
						</Badge>
					{/if}
				</Tabs.Trigger>
			{/each}
		</Tabs.List>
	</div>
	{#each tabs as value (value)}
		<Tabs.Content {value}>
			{#if value === ActivityTab.DOWNLOADS}
				<DownloadsTab />
			{:else if value === ActivityTab.IMPORTS}
				<ImportsTab manager={imports} />
			{:else if value === ActivityTab.BLOCKLIST}
				<BlocklistTab />
			{:else if value === ActivityTab.HISTORY}
				<HistoryTab />
			{:else if value === ActivityTab.AGENTS}
				<AgentsTab />
			{:else if value === ActivityTab.GPU_JOBS}
				<GpuJobsTab />
			{:else}
				<p class="py-8 text-center text-sm text-muted-foreground">Nothing here yet.</p>
			{/if}
		</Tabs.Content>
	{/each}
</Tabs.Root>

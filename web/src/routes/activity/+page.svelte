<script lang="ts">
	import * as Tabs from '$lib/components/ui/tabs';
	import HistoryTab from '$lib/activity/HistoryTab.svelte';
	import BlocklistTab from '$lib/downloads/BlocklistTab.svelte';
	import DownloadsTab from '$lib/downloads/DownloadsTab.svelte';
	import AgentsTab from '$lib/transcode/AgentsTab.svelte';
	import { ACTIVITY_TABS, ActivityTab, TAB_LABELS } from '$lib/activity/activity-tabs';
	import { NAV_ITEMS } from '$lib/navigation/nav';

	let tab = $state(ActivityTab.DOWNLOADS);
</script>

<svelte:head>
	<title>{NAV_ITEMS.ACTIVITY.label} · Magpie</title>
</svelte:head>

<Tabs.Root bind:value={tab} class="gap-4">
	<div class="-mx-4 overflow-x-auto px-4 md:mx-0 md:px-0">
		<Tabs.List>
			{#each ACTIVITY_TABS as value (value)}
				<Tabs.Trigger {value}>{TAB_LABELS[value]}</Tabs.Trigger>
			{/each}
		</Tabs.List>
	</div>
	{#each ACTIVITY_TABS as value (value)}
		<Tabs.Content {value}>
			{#if value === ActivityTab.DOWNLOADS}
				<DownloadsTab />
			{:else if value === ActivityTab.BLOCKLIST}
				<BlocklistTab />
			{:else if value === ActivityTab.HISTORY}
				<HistoryTab />
			{:else if value === ActivityTab.AGENTS}
				<AgentsTab />
			{:else}
				<p class="py-8 text-center text-sm text-muted-foreground">Nothing here yet.</p>
			{/if}
		</Tabs.Content>
	{/each}
</Tabs.Root>

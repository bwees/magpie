<script lang="ts">
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import FilterChips from '$lib/components/FilterChips.svelte';
	import { LoadStatus } from '$lib/load-status';
	import IssueRow from './IssueRow.svelte';
	import IssueThreadSheet from './IssueThreadSheet.svelte';
	import RegrabWarningDialog from './RegrabWarningDialog.svelte';
	import { ISSUE_FILTERS, ISSUE_FILTER_LABELS, IssueFilter } from './issues';
	import type { IssuesTab } from './issues-tab.svelte';

	let { tab }: { tab: IssuesTab } = $props();

	const SKELETON_ROWS = ['first', 'second', 'third'];
</script>

<div class="flex flex-col gap-4">
	<FilterChips
		filters={ISSUE_FILTERS}
		selected={tab.filter}
		labels={ISSUE_FILTER_LABELS}
		count={(filter) => (filter === IssueFilter.OPEN ? tab.openCount : undefined)}
		onSelect={(filter) => tab.select(filter)}
	/>

	{#if tab.status === LoadStatus.LOADING}
		<div class="flex flex-col gap-3" role="status" aria-label="Loading issues">
			{#each SKELETON_ROWS as key (key)}
				<Skeleton class="h-20 w-full" />
			{/each}
		</div>
	{:else if tab.status === LoadStatus.ERROR}
		<Alert.Root variant="destructive">
			<CircleAlert />
			<Alert.Title>Could not load issues</Alert.Title>
			<Alert.Description>
				<p>{tab.error}</p>
				<Button variant="outline" size="sm" class="mt-2" onclick={() => tab.load()}>Retry</Button>
			</Alert.Description>
		</Alert.Root>
	{:else if tab.empty}
		<p class="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
			{tab.emptyText}
		</p>
	{:else}
		<ul class="divide-y rounded-lg border bg-card px-4">
			{#each tab.items as issue (issue.id)}
				<IssueRow {issue} {tab} />
			{/each}
		</ul>
	{/if}
</div>

<IssueThreadSheet {tab} />
<RegrabWarningDialog fix={tab.fix} />

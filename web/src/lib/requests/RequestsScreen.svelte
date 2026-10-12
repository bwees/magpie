<script lang="ts">
	import { page as route } from '$app/state';
	import { onMount, untrack } from 'svelte';
	import { Badge } from '$lib/components/ui/badge';
	import * as Tabs from '$lib/components/ui/tabs';
	import { getLive } from '$lib/live/live-client.svelte';
	import IssuesView from './IssuesView.svelte';
	import RequestsView from './RequestsView.svelte';
	import { ISSUE_QUERY } from './issues';
	import { IssuesTab } from './issues-tab.svelte';
	import { REQUESTS_TAB_QUERY, RequestsTab, StatusFilter } from './requests';
	import { RequestsPage, type Viewer } from './requests-page.svelte';

	let { viewer }: { viewer: Viewer } = $props();

	// svelte-ignore state_referenced_locally
	const requests = new RequestsPage(viewer);
	// svelte-ignore state_referenced_locally
	const issues = new IssuesTab(viewer);
	const live = getLive();

	let picked = $state<RequestsTab>();
	const linkedIssue = $derived(route.url.searchParams.get(ISSUE_QUERY));
	const linkedTab = $derived(
		linkedIssue
			? RequestsTab.ISSUES
			: (route.url.searchParams.get(REQUESTS_TAB_QUERY) as RequestsTab)
	);
	const tab = $derived(picked ?? linkedTab ?? RequestsTab.REQUESTS);
	const pending = $derived(requests.count(StatusFilter.PENDING));

	onMount(() => {
		void requests.load();
		void issues.load();
		const stopRequests = requests.watch(live);
		const stopIssues = issues.watch(live);
		return () => {
			stopRequests();
			stopIssues();
		};
	});

	$effect(() => {
		const id = linkedIssue;
		if (!id) return;
		untrack(() => {
			picked = undefined;
			void issues.open(id);
		});
	});
</script>

<Tabs.Root value={tab} onValueChange={(value) => (picked = value as RequestsTab)} class="gap-4">
	<Tabs.List>
		<Tabs.Trigger value={RequestsTab.REQUESTS}>
			Requests
			{#if viewer.admin && pending > 0}
				<Badge class="ml-1 h-5 min-w-5 px-1.5 tabular-nums" aria-label="{pending} pending">
					{pending}
				</Badge>
			{/if}
		</Tabs.Trigger>
		<Tabs.Trigger value={RequestsTab.ISSUES}>
			Issues
			{#if issues.openCount > 0}
				<Badge
					variant="secondary"
					class="ml-1 h-5 min-w-5 px-1.5 tabular-nums"
					aria-label="{issues.openCount} open"
				>
					{issues.openCount}
				</Badge>
			{/if}
		</Tabs.Trigger>
	</Tabs.List>
	<Tabs.Content value={RequestsTab.REQUESTS}>
		<RequestsView page={requests} />
	</Tabs.Content>
	<Tabs.Content value={RequestsTab.ISSUES}>
		<IssuesView tab={issues} />
	</Tabs.Content>
</Tabs.Root>

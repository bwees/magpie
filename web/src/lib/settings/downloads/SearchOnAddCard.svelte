<script lang="ts">
	import { onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Switch } from '$lib/components/ui/switch';
	import { LoadStatus } from '$lib/load-status';
	import { SearchOptionsManager } from './search-options-manager.svelte';

	const manager = new SearchOptionsManager();

	onMount(() => {
		void manager.load();
	});
</script>

<Card.Root>
	<Card.Header>
		<div class="flex items-start justify-between gap-4">
			<div class="grid gap-1">
				<Card.Title><label for="search-on-add">Search when a title is added</label></Card.Title>
				<Card.Description>
					Requested and manually added titles are searched at once for what is already released.
					Titles found by a library scan or a migration wait for the backlog sweeps.
				</Card.Description>
			</div>
			{#if manager.status === LoadStatus.LOADING}
				<Skeleton class="h-5 w-9 rounded-full" />
			{:else if manager.saved}
				<Switch
					id="search-on-add"
					checked={manager.saved.searchOnAdd}
					disabled={manager.saving}
					onCheckedChange={(checked) => manager.setSearchOnAdd(checked)}
				/>
			{/if}
		</div>
	</Card.Header>
	{#if manager.status === LoadStatus.ERROR}
		<Card.Content class="flex flex-wrap items-center gap-3">
			<p class="text-sm text-destructive" role="alert">{manager.error}</p>
			<Button size="sm" variant="outline" onclick={() => manager.load()}>Retry</Button>
		</Card.Content>
	{/if}
</Card.Root>

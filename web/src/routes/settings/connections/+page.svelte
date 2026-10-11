<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import ConnectionCard from '$lib/connections/ConnectionCard.svelte';
	import { ConnectionsState } from '$lib/connections/connections-state.svelte';
	import { LoadStatus } from '$lib/load-status';
	import { SERVICE_ORDER } from '$lib/connections/services';
	import { getLive } from '$lib/live/live-client.svelte';

	const connections = new ConnectionsState(getLive());

	onMount(() => {
		connections.load();
	});
	onDestroy(() => connections.destroy());
</script>

{#if connections.status === LoadStatus.LOADING}
	<div class="grid gap-4" aria-busy="true">
		{#each SERVICE_ORDER as service (service)}
			<Skeleton class="h-40 w-full rounded-xl" />
		{/each}
	</div>
{:else if connections.status === LoadStatus.ERROR}
	<div class="flex flex-col items-start gap-3 rounded-xl border border-destructive/40 p-4">
		<p class="text-sm text-destructive">{connections.error}</p>
		<Button size="sm" variant="outline" onclick={() => connections.load()}>Retry</Button>
	</div>
{:else}
	<div class="grid gap-4">
		{#each SERVICE_ORDER as service (service)}
			<ConnectionCard {connections} {service} />
		{/each}
	</div>
{/if}

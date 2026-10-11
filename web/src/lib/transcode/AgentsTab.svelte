<script lang="ts">
	import { onMount } from 'svelte';
	import Cpu from '@lucide/svelte/icons/cpu';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { getLive } from '$lib/live/live-client.svelte';
	import { SettingsSection, settingsPath } from '$lib/settings/sections';
	import AgentCard from './AgentCard.svelte';
	import { AgentsManager, LoadState } from './agents-manager.svelte';
	import { dockerRunCommand } from './docker-run';

	const SKELETON_CARDS = 2;

	const manager = new AgentsManager();
	const live = getLive();

	onMount(() => {
		void manager.load();
		return manager.watch(live);
	});
</script>

{#if manager.loadState === LoadState.LOADING}
	<div class="grid gap-4 xl:grid-cols-2" aria-busy="true" aria-label="Loading agents">
		{#each { length: SKELETON_CARDS }, i (i)}
			<Skeleton class="h-64 rounded-xl" />
		{/each}
	</div>
{:else if manager.loadState === LoadState.ERROR}
	<section
		class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center"
		role="alert"
	>
		<p class="text-sm text-muted-foreground">{manager.error}</p>
		<Button variant="outline" size="sm" onclick={() => manager.load()}>Retry</Button>
	</section>
{:else if manager.agents.length === 0}
	<section class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center">
		<Cpu class="size-8 text-muted-foreground" />
		<h2 class="text-lg font-semibold">No agents yet</h2>
		<p class="max-w-prose text-sm text-muted-foreground">
			Start an agent on a machine with a GPU. It shows up here once it connects. The shared secret
			is in
			<a
				class="text-primary underline-offset-4 hover:underline"
				href={settingsPath(SettingsSection.AGENTS)}
			>
				Settings → Agents</a
			>.
		</p>
		<pre
			class="max-w-full overflow-x-auto rounded-md bg-muted p-3 text-left font-mono text-xs">{dockerRunCommand(
				location.origin
			)}</pre>
	</section>
{:else}
	<div class="grid gap-4 xl:grid-cols-2">
		{#each manager.agents as agent (agent.id)}
			<AgentCard {agent} {manager} />
		{/each}
	</div>
{/if}

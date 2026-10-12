<script lang="ts">
	import { onMount } from 'svelte';
	import { TitleDirectory } from '$lib/catalog/title-directory.svelte';
	import { getLive } from '$lib/live/live-client.svelte';
	import { getShell } from '$lib/shell/shell-state.svelte';
	import LiveNowBlock from './LiveNowBlock.svelte';
	import NeedsYouBlock from './NeedsYouBlock.svelte';
	import { NeedsYou } from './needs-you.svelte';
	import RecentlyActiveBlock from './RecentlyActiveBlock.svelte';
	import { RecentlyActive } from './recently-active.svelte';

	const shell = getShell();
	const live = getLive();
	const directory = new TitleDirectory();
	const recent = new RecentlyActive(directory);
	const needsYou = new NeedsYou(shell.attention, directory);

	onMount(() => {
		void recent.load();
		return recent.watch(live);
	});
</script>

<div class="grid grid-cols-1 gap-8 lg:grid-cols-[minmax(0,1fr)_380px]">
	<div class="order-last min-w-0 lg:order-none">
		<RecentlyActiveBlock {recent} />
	</div>
	<aside class="flex min-w-0 flex-col gap-8">
		<NeedsYouBlock {needsYou} />
		<LiveNowBlock pipeline={shell.livePipeline} {directory} />
	</aside>
</div>

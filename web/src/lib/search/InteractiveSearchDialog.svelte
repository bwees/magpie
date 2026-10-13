<script lang="ts">
	import { untrack } from 'svelte';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import RotateCw from '@lucide/svelte/icons/rotate-cw';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { getLive } from '$lib/live/live-client.svelte';
	import { SettingsSection, settingsPath } from '$lib/settings/sections';
	import QualityBuckets from './QualityBuckets.svelte';
	import TrackerPill from './TrackerPill.svelte';
	import { InteractiveSearch, SearchPhase } from './interactive-search.svelte';
	import type { SearchTarget } from './search-target';

	let { target, open = $bindable(false) }: { target: SearchTarget; open?: boolean } = $props();

	const live = getLive();
	const search = new InteractiveSearch(live);
	const indexersPath = settingsPath(SettingsSection.INDEXERS);

	$effect(() => {
		if (!open) return;
		untrack(() => void search.start(target));
		return () => search.stop();
	});
</script>

<Dialog.Root bind:open>
	<Dialog.Content
		class="top-[env(safe-area-inset-top)] flex h-[calc(100dvh-env(safe-area-inset-top))] w-screen max-w-none translate-y-0 flex-col gap-0 rounded-none p-0 sm:max-w-none md:top-1/2 md:h-[90vh] md:w-[94vw] md:max-w-[94vw] md:-translate-y-1/2 md:rounded-xl"
		onOpenAutoFocus={(event) => event.preventDefault()}
	>
		<div class="flex flex-wrap items-center gap-3 border-b px-4 py-3 pr-12 md:px-6 md:py-4">
			<Dialog.Title class="w-full truncate text-lg md:w-auto">Search · {target.label}</Dialog.Title>
			<Dialog.Description class="sr-only">
				Releases from every indexer, grouped by quality as they arrive.
			</Dialog.Description>
			{#if search.profile}
				<Badge variant="outline" class="hidden md:inline-flex">
					{search.profile.name} · cutoff {search.profile.cutoff}
				</Badge>
			{/if}
			<TrackerPill {search} />
			<span class="hidden text-sm text-muted-foreground tabular-nums md:inline">
				{search.accepted.length} accepted · {search.rejected.length} rejected
			</span>
			<Button
				size="sm"
				variant="outline"
				class="ml-auto md:mr-8"
				disabled={search.phase === SearchPhase.STARTING}
				onclick={() => search.start(target)}
			>
				<RotateCw /><span class="hidden sm:inline">Search again</span>
			</Button>
		</div>
		<div class="min-h-0 flex-1 overflow-y-auto p-3 md:p-6">
			{#if search.phase === SearchPhase.FAILED}
				<Alert.Root variant="destructive">
					<CircleAlert />
					<Alert.Title>The search didn't run</Alert.Title>
					<Alert.Description>{search.error}</Alert.Description>
				</Alert.Root>
			{:else if search.phase === SearchPhase.STARTING && !search.profile}
				<div class="flex flex-col gap-3" aria-busy="true">
					<Skeleton class="h-5 w-32" />
					<Skeleton class="h-14 w-full" />
					<Skeleton class="h-14 w-full" />
				</div>
			{:else}
				{#if search.allFailed}
					<Alert.Root variant="destructive" class="mb-6">
						<CircleAlert />
						<Alert.Title>No trackers answered. Check Prowlarr</Alert.Title>
						<Alert.Description>
							<a class="underline" href={indexersPath}>Open Settings → Indexers</a>
						</Alert.Description>
					</Alert.Root>
				{:else if search.noIndexers}
					<Alert.Root class="mb-6">
						<CircleAlert />
						<Alert.Title>No indexer can search this title</Alert.Title>
						<Alert.Description>
							<a class="underline" href={indexersPath}>Add indexers in Prowlarr, then sync them</a>
						</Alert.Description>
					</Alert.Root>
				{/if}
				<QualityBuckets {search} />
			{/if}
		</div>
	</Dialog.Content>
</Dialog.Root>

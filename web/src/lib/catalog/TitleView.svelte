<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import FileQuestion from '@lucide/svelte/icons/file-question';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Tabs from '$lib/components/ui/tabs';
	import { SDK_SOURCE } from '$lib/activity/activity-source';
	import Timeline from '$lib/activity/Timeline.svelte';
	import { TitlePipeline } from '$lib/activity/title-pipeline.svelte';
	import EpisodesTab from '$lib/catalog/EpisodesTab.svelte';
	import TitleHero from '$lib/catalog/TitleHero.svelte';
	import TitleSettings from '$lib/catalog/TitleSettings.svelte';
	import { TIMELINE_COLUMN_QUERY, TitleTab } from '$lib/catalog/layout';
	import FileList from '$lib/library/FileList.svelte';
	import { TitleFiles } from '$lib/library/title-files.svelte';
	import { PageStatus, TitlePage } from '$lib/catalog/title-page.svelte';
	import { getLive } from '$lib/live/live-client.svelte';
	import { Route } from '$lib/navigation/nav';

	/** The parent keys this view on the id, so one view shows one title for its whole life. */
	let { id }: { id: string } = $props();

	const wide = new MediaQuery(TIMELINE_COLUMN_QUERY);
	const live = getLive();

	// svelte-ignore state_referenced_locally
	const title = new TitlePage(id);
	/** The stage bar and the episode rows share one pipeline. */
	// svelte-ignore state_referenced_locally
	const pipeline = new TitlePipeline(id, SDK_SOURCE);
	// svelte-ignore state_referenced_locally
	const files = new TitleFiles(id);
	/** The tab the user picked; until then the page opens on its default tab. */
	let picked = $state<TitleTab | undefined>();
	const defaultTab = $derived(
		title.isSeries ? TitleTab.EPISODES : wide.current ? TitleTab.FILES : TitleTab.TIMELINE
	);
	const tab = $derived(picked ?? defaultTab);

	onMount(() => {
		void title.load();
		return title.watch(live);
	});

	onMount(() => {
		void pipeline.load();
		return pipeline.connect(live);
	});

	onMount(() => {
		void files.load();
		return files.watch(live);
	});

	$effect(() => {
		if (wide.current && picked === TitleTab.TIMELINE) {
			picked = undefined;
		}
	});
</script>

<svelte:head>
	<title>{title.title?.name ?? 'Title'} · Magpie</title>
</svelte:head>

{#if title.status === PageStatus.LOADING}
	<div class="flex flex-col gap-6" aria-busy="true">
		<Skeleton class="h-72 w-full rounded-xl" />
		<Skeleton class="h-9 w-64" />
		<Skeleton class="h-40 w-full" />
	</div>
{:else if title.status === PageStatus.NOT_FOUND}
	<section
		class="flex min-h-64 flex-col items-center justify-center gap-3 rounded-xl border border-dashed p-8 text-center"
	>
		<FileQuestion class="size-8 text-muted-foreground" />
		<h2 class="text-lg font-semibold">Title not found</h2>
		<p class="text-sm text-muted-foreground">It may have been removed from Magpie.</p>
		<Button href={Route.HOME} variant="secondary">Go home</Button>
	</section>
{:else if title.status === PageStatus.FAILED || !title.title}
	<section class="flex flex-col items-center gap-3 rounded-xl border p-8 text-center" role="alert">
		<p class="text-sm text-muted-foreground">{title.error}</p>
		<Button variant="secondary" onclick={() => title.load()}>Retry</Button>
	</section>
{:else}
	<div class="flex flex-col gap-6">
		<TitleHero title={title.title} {pipeline} posterUrl={title.posterUrl} tint={title.tint} />

		<div class="grid grid-cols-1 gap-8 lg:grid-cols-[1fr_340px]">
			<Tabs.Root
				value={tab}
				onValueChange={(value) => (picked = value as TitleTab)}
				class="min-w-0"
			>
				<Tabs.List>
					{#if title.isSeries}
						<Tabs.Trigger value={TitleTab.EPISODES}>Episodes</Tabs.Trigger>
					{/if}
					<Tabs.Trigger value={TitleTab.TIMELINE} class="lg:hidden">Timeline</Tabs.Trigger>
					{#if !title.isSeries}
						<Tabs.Trigger value={TitleTab.FILES}>Files</Tabs.Trigger>
					{/if}
					<Tabs.Trigger value={TitleTab.SETTINGS}>Settings</Tabs.Trigger>
				</Tabs.List>
				{#if title.isSeries}
					<Tabs.Content value={TitleTab.EPISODES} class="pt-4">
						<EpisodesTab page={title} {pipeline} {files} />
					</Tabs.Content>
				{/if}
				<Tabs.Content value={TitleTab.TIMELINE} class="pt-4 lg:hidden">
					<Timeline titleId={title.id} />
				</Tabs.Content>
				{#if !title.isSeries}
					<Tabs.Content value={TitleTab.FILES} class="pt-4">
						<FileList {files} />
					</Tabs.Content>
				{/if}
				<Tabs.Content value={TitleTab.SETTINGS} class="pt-4">
					<TitleSettings page={title} onRemoved={() => goto(Route.HOME)} />
				</Tabs.Content>
			</Tabs.Root>
			<aside class="hidden w-[340px] lg:block">
				<h2 class="mb-3 text-sm font-semibold">Timeline</h2>
				<Timeline titleId={title.id} />
			</aside>
		</div>
	</div>
{/if}

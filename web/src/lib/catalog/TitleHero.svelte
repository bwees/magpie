<script lang="ts">
	import { onMount } from 'svelte';
	import type { TitleResponse } from '$lib/sdk';
	import { ProfileOptions } from '$lib/settings/quality/profile-options.svelte';
	import StageBar from '$lib/activity/StageBar.svelte';
	import type { TitlePipeline } from '$lib/activity/title-pipeline.svelte';
	import RequestedBy from '$lib/requests/RequestedBy.svelte';
	import Flag from '@lucide/svelte/icons/flag';
	import Search from '@lucide/svelte/icons/search';
	import ReportIssueDialog from '$lib/requests/ReportIssueDialog.svelte';
	import { ReportIssue } from '$lib/requests/report-issue.svelte';
	import InteractiveSearchDialog from '$lib/search/InteractiveSearchDialog.svelte';
	import { AutomaticSearch } from '$lib/search/automatic-search.svelte';
	import { getSession } from '$lib/session/session.svelte';
	import PosterImage from './PosterImage.svelte';
	import { Button } from '$lib/components/ui/button';

	let {
		title,
		pipeline,
		posterUrl,
		tint
	}: { title: TitleResponse; pipeline: TitlePipeline; posterUrl?: string; tint?: string } =
		$props();

	/** A dark teal used until the poster colour is known. */
	const DEFAULT_TINT = '24 72 80';

	const background = $derived(
		`linear-gradient(120deg, rgb(${tint ?? DEFAULT_TINT} / 0.75), rgb(${tint ?? DEFAULT_TINT} / 0.25) 55%, var(--card))`
	);
	const chip = 'rounded-md bg-white/15 px-2 py-0.5';

	const profiles = new ProfileOptions();
	const profileName = $derived(profiles.name(title.profileId));

	onMount(() => {
		profiles.load();
	});

	const session = getSession();
	const automatic = new AutomaticSearch();
	const target = $derived({ titleId: title.id, label: title.name });
	let searching = $state(false);
	// svelte-ignore state_referenced_locally
	const report = new ReportIssue(title.id);
</script>

<section class="overflow-hidden rounded-xl border bg-card" style:background-image={background}>
	<div class="flex flex-col gap-4 p-4 text-white md:p-8 lg:flex-row lg:items-end lg:gap-6 lg:pt-12">
		<div class="flex items-end gap-4 md:gap-6 lg:contents">
			<PosterImage
				src={posterUrl}
				alt="{title.name} poster"
				class="w-24 shrink-0 ring-2 ring-white/20 md:w-32 lg:w-36"
			/>
			<h1 class="text-2xl font-semibold tracking-tight md:text-4xl lg:hidden">{title.name}</h1>
		</div>
		<div class="min-w-0 flex-1">
			<h1 class="hidden text-4xl font-semibold tracking-tight lg:block">{title.name}</h1>
			{#if title.overview}
				<p class="mt-2 line-clamp-3 max-w-3xl text-sm text-white/80 md:line-clamp-none">
					{title.overview}
				</p>
			{/if}
			<div class="mt-3 flex flex-wrap gap-2 text-xs">
				{#if title.year}
					<span class={chip}>{title.year}</span>
				{/if}
				{#if profileName}
					<span class={chip}>{profileName}</span>
				{/if}
				{#if title.requestedBy}
					<RequestedBy titleId={title.id} class={chip} />
				{/if}
				{#if !title.monitored}
					<span class={chip}>Not monitored</span>
				{/if}
			</div>
		</div>
		{#if session.isAdmin || session.canReportIssues}
			<div class="grid grid-cols-2 gap-2 lg:flex lg:shrink-0">
				{#if session.isAdmin}
					<Button
						variant="secondary"
						disabled={automatic.isStarting(target)}
						onclick={() => automatic.search(target)}
					>
						<Search />Search
					</Button>
					<Button
						variant="outline"
						class="border-white/30 bg-transparent text-white hover:bg-white/10"
						onclick={() => (searching = true)}
					>
						Interactive search
					</Button>
				{/if}
				{#if session.canReportIssues}
					<Button
						variant="outline"
						class="col-span-2 border-white/30 bg-transparent text-white hover:bg-white/10"
						onclick={() => report.start()}
					>
						<Flag />Report issue
					</Button>
				{/if}
			</div>
		{/if}
	</div>
	<div class="px-4 pb-5 md:px-8 md:pb-6">
		<StageBar {pipeline} class="text-white" />
	</div>
</section>

{#if session.canReportIssues}
	<ReportIssueDialog {report} titleName={title.name} seasons={title.seasons} />
{/if}

{#if session.isAdmin}
	<InteractiveSearchDialog bind:open={searching} titleId={title.id} titleName={title.name} />
{/if}

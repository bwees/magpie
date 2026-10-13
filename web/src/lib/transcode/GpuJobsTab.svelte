<script lang="ts">
	import { onMount } from 'svelte';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import Cpu from '@lucide/svelte/icons/cpu';
	import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
	import SkipForward from '@lucide/svelte/icons/skip-forward';
	import HoverPopover from '$lib/components/HoverPopover.svelte';
	import X from '@lucide/svelte/icons/x';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Table from '$lib/components/ui/table';
	import * as Tabs from '$lib/components/ui/tabs';
	import ProgressBar from '$lib/downloads/ProgressBar.svelte';
	import { getLive } from '$lib/live/live-client.svelte';
	import { LoadStatus } from '$lib/load-status';
	import type { JobResponse } from '$lib/sdk';
	import { SettingsSection, settingsPath } from '$lib/settings/sections';
	import { SKIP_LABELS } from './transcode-settings';
	import { GpuJobsManager } from './gpu-jobs-manager.svelte';
	import {
		JobListTab,
		JobState,
		attemptLabel,
		detailLabel,
		jobName,
		kindLabel,
		originLabel,
		stateLabel
	} from './gpu-jobs';

	const SKELETON_ROWS = 3;
	const RUNNING_COLUMNS = ['Title', 'Kind', 'State', 'Progress', 'Detail', 'Agent', ''];

	const manager = new GpuJobsManager();
	const live = getLive();

	onMount(() => {
		void manager.load();
		return manager.watch(live);
	});

	const empty = $derived(
		manager.running.length === 0 &&
			manager.queuedCount === 0 &&
			manager.failed.length === 0 &&
			manager.skipReasons.length === 0
	);
</script>

{#snippet stateBadge(job: JobResponse)}
	{#if job.state === JobState.FAILED}
		<Badge variant="destructive">{stateLabel(job)}</Badge>
	{:else if job.state === JobState.NOT_WORTH_IT || job.state === JobState.QUEUED}
		<Badge variant="secondary">{stateLabel(job)}</Badge>
	{:else}
		<Badge variant="outline" class="border-primary/50 text-primary">{stateLabel(job)}</Badge>
	{/if}
{/snippet}

{#snippet cancelButton(job: JobResponse)}
	<Button
		size="icon-sm"
		variant="ghost"
		title="Cancel"
		aria-label={`Cancel ${jobName(job)}`}
		disabled={manager.busy.has(job.id)}
		onclick={() => manager.cancel(job)}
	>
		<X />
	</Button>
{/snippet}

{#snippet countBadge(count: number, variant: 'secondary' | 'destructive')}
	{#if count > 0}
		<Badge {variant} class="ml-1 h-5 min-w-5 px-1.5 tabular-nums">{count}</Badge>
	{/if}
{/snippet}

{#snippet sectionTitle(title: string, count: number)}
	<h2 class="flex items-center gap-2 text-sm font-semibold">
		{title}
		<span class="font-normal text-muted-foreground tabular-nums">{count}</span>
	</h2>
{/snippet}

{#if manager.status === LoadStatus.LOADING}
	<div class="flex flex-col gap-2" aria-busy="true" aria-label="Loading GPU jobs">
		{#each { length: SKELETON_ROWS }, i (i)}
			<Skeleton class="h-12 w-full rounded-lg" />
		{/each}
	</div>
{:else if manager.status === LoadStatus.ERROR}
	<section
		class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center"
		role="alert"
	>
		<p class="text-sm text-muted-foreground">{manager.error}</p>
		<Button variant="outline" size="sm" onclick={() => manager.load()}>Retry</Button>
	</section>
{:else if empty}
	<section class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center">
		<Cpu class="size-8 text-muted-foreground" />
		<p class="text-sm text-muted-foreground">
			No GPU jobs. Files are queued here once they meet their quality cutoff.
		</p>
	</section>
{:else}
	<div class="flex flex-col gap-6">
		{#if manager.waitingForAgent > 0}
			<Alert.Root>
				<CircleAlert />
				<Alert.Title>
					{manager.waitingForAgent}
					{manager.waitingForAgent === 1 ? 'file waits' : 'files wait'} for a capable agent
				</Alert.Title>
				<Alert.Description>
					No agent has passed a test encode for the transcode target. These files queue as soon as
					one does.
				</Alert.Description>
			</Alert.Root>
		{/if}

		<section class="flex flex-col gap-3" aria-label="Running">
			{@render sectionTitle('Running', manager.running.length)}
			{#if manager.running.length === 0}
				<p class="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">
					Nothing is running. Queued jobs start when an agent has a free slot.
				</p>
			{:else}
				<div class="hidden rounded-xl border md:block">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								{#each RUNNING_COLUMNS as column, i (i)}
									<Table.Head class="whitespace-nowrap">{column}</Table.Head>
								{/each}
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each manager.running as job (job.id)}
								<Table.Row>
									<Table.Cell class="max-w-56 truncate font-medium" title={job.fileName}>
										{jobName(job)}
									</Table.Cell>
									<Table.Cell class="whitespace-nowrap text-muted-foreground"
										>{kindLabel(job)}</Table.Cell
									>
									<Table.Cell>{@render stateBadge(job)}</Table.Cell>
									<Table.Cell class="w-40"><ProgressBar progress={job.progress} /></Table.Cell>
									<Table.Cell class="whitespace-nowrap text-xs text-muted-foreground tabular-nums">
										{detailLabel(job, manager.details.get(job.id))}
									</Table.Cell>
									<Table.Cell class="whitespace-nowrap">{job.agentName || '—'}</Table.Cell>
									<Table.Cell class="text-right">{@render cancelButton(job)}</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
				<div class="flex flex-col gap-3 md:hidden">
					{#each manager.running as job (job.id)}
						<article class="flex flex-col gap-2 rounded-xl border p-3">
							<div class="flex items-start justify-between gap-2">
								<div class="min-w-0">
									<h3 class="truncate text-sm font-medium">{jobName(job)}</h3>
									<p class="text-xs text-muted-foreground">{kindLabel(job)}</p>
								</div>
								{@render cancelButton(job)}
							</div>
							<div class="flex items-center gap-2">
								{@render stateBadge(job)}
								<span class="truncate text-xs text-muted-foreground">{job.agentName}</span>
							</div>
							<ProgressBar progress={job.progress} />
							<p class="text-xs text-muted-foreground tabular-nums">
								{detailLabel(job, manager.details.get(job.id))}
							</p>
						</article>
					{/each}
				</div>
			{/if}
		</section>

		<Tabs.Root
			value={manager.tab}
			onValueChange={(value) => (manager.tab = value as JobListTab)}
			class="gap-3"
		>
			<div class="flex flex-wrap items-center gap-3">
				<Tabs.List>
					<Tabs.Trigger value={JobListTab.QUEUED}>
						Queued
						{@render countBadge(manager.queuedCount, 'secondary')}
					</Tabs.Trigger>
					<Tabs.Trigger value={JobListTab.FAILED}>
						Failed
						{@render countBadge(manager.failed.length, 'destructive')}
					</Tabs.Trigger>
				</Tabs.List>
				{#if manager.tab === JobListTab.QUEUED && manager.backlog > 0}
					<Badge variant="secondary" class="tabular-nums">
						{manager.backlog} from the library backlog
					</Badge>
				{/if}
				{#if manager.skipReasons.length > 0}
					<div class="ml-auto">
						<HoverPopover class="w-64">
							{#snippet trigger(props)}
								<Button
									{...props}
									variant="ghost"
									size="sm"
									class="text-muted-foreground tabular-nums"
									aria-label="{manager.skippedCount} files skipped"
								>
									<SkipForward />
									{manager.skippedCount} skipped
								</Button>
							{/snippet}
							<div class="flex flex-col gap-2 text-xs">
								<dl class="grid grid-cols-[1fr_auto] gap-x-3 gap-y-1.5">
									{#each manager.skipReasons as reason (reason)}
										<dt class="text-muted-foreground">{SKIP_LABELS[reason]}</dt>
										<dd class="text-right tabular-nums">{manager.skipped[reason]}</dd>
									{/each}
								</dl>
								<a
									href={settingsPath(SettingsSection.TRANSCODE)}
									class="text-primary underline-offset-4 hover:underline">Transcode settings</a
								>
							</div>
						</HoverPopover>
					</div>
				{/if}
			</div>
			<Tabs.Content value={JobListTab.QUEUED} class="flex flex-col gap-3">
				{#if manager.queued.length === 0}
					<p class="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">
						Nothing is waiting.
					</p>
				{:else}
					<ul class="divide-y rounded-xl border">
						{#each manager.queuedOnPage as job (job.id)}
							<li class="flex items-center gap-3 px-4 py-2.5">
								<div class="min-w-0 flex-1">
									<p class="truncate text-sm font-medium">{jobName(job)}</p>
									<p class="truncate text-xs text-muted-foreground">
										{kindLabel(job)} · {originLabel(job.origin)}
										{#if job.attempt > 1}
											· {attemptLabel(job)}
										{/if}
									</p>
								</div>
								{@render cancelButton(job)}
							</li>
						{/each}
					</ul>
					{#if manager.queuedPages > 1}
						<div class="flex items-center justify-end gap-2">
							<span class="text-xs text-muted-foreground tabular-nums">
								Page {manager.queuedPage + 1} of {manager.queuedPages}
							</span>
							<Button
								size="icon-sm"
								variant="outline"
								aria-label="Previous page"
								disabled={manager.queuedPage === 0}
								onclick={() => manager.previousQueuedPage()}
							>
								<ChevronLeft />
							</Button>
							<Button
								size="icon-sm"
								variant="outline"
								aria-label="Next page"
								disabled={manager.queuedPage === manager.queuedPages - 1}
								onclick={() => manager.nextQueuedPage()}
							>
								<ChevronRight />
							</Button>
						</div>
					{/if}
				{/if}
			</Tabs.Content>

			<Tabs.Content value={JobListTab.FAILED}>
				{#if manager.failed.length === 0}
					<p class="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">
						Nothing has failed.
					</p>
				{:else}
					<ul class="divide-y rounded-xl border">
						{#each manager.failed as job (job.id)}
							<li class="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:gap-3">
								<div class="min-w-0 flex-1">
									<div class="flex flex-wrap items-center gap-2">
										<p class="truncate text-sm font-medium">{jobName(job)}</p>
										{@render stateBadge(job)}
									</div>
									<p
										class="text-xs break-words {job.state === JobState.FAILED
											? 'text-destructive'
											: 'text-muted-foreground'}"
									>
										{job.error || 'No reason recorded'}
									</p>
									<p class="text-xs text-muted-foreground">
										{kindLabel(job)} · {attemptLabel(job)}{job.agentName
											? ` · ${job.agentName}`
											: ''}
									</p>
								</div>
								<div class="flex items-center gap-1 self-start sm:self-center">
									<Button
										variant="outline"
										size="sm"
										disabled={manager.busy.has(job.id)}
										onclick={() => manager.retry(job)}
									>
										<RotateCcw />
										Retry
									</Button>
									<Button
										size="icon-sm"
										variant="ghost"
										title="Dismiss"
										aria-label={`Dismiss ${jobName(job)}`}
										disabled={manager.busy.has(job.id)}
										onclick={() => manager.dismiss(job)}
									>
										<X />
									</Button>
								</div>
							</li>
						{/each}
					</ul>
				{/if}
			</Tabs.Content>
		</Tabs.Root>
	</div>
{/if}

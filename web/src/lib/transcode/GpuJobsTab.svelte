<script lang="ts">
	import { onMount } from 'svelte';
	import Cpu from '@lucide/svelte/icons/cpu';
	import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
	import X from '@lucide/svelte/icons/x';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Table from '$lib/components/ui/table';
	import ProgressBar from '$lib/downloads/ProgressBar.svelte';
	import { getLive } from '$lib/live/live-client.svelte';
	import { LoadStatus } from '$lib/load-status';
	import type { JobResponse } from '$lib/sdk';
	import { GpuJobsManager } from './gpu-jobs-manager.svelte';
	import {
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
		manager.running.length === 0 && manager.queuedCount === 0 && manager.failed.length === 0
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

		<section class="flex flex-col gap-3" aria-label="Queued">
			<div class="flex flex-wrap items-center gap-3">
				{@render sectionTitle('Queued', manager.queuedCount)}
				{#if manager.backlog > 0}
					<Badge variant="secondary" class="tabular-nums">
						{manager.backlog} from the library backlog
					</Badge>
				{/if}
			</div>
			{#if manager.queued.length === 0}
				<p class="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">
					Nothing is waiting.
				</p>
			{:else}
				<ul class="divide-y rounded-xl border">
					{#each manager.queued as job (job.id)}
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
			{/if}
		</section>

		{#if manager.failed.length > 0}
			<section class="flex flex-col gap-3" aria-label="Failed">
				{@render sectionTitle('Failed', manager.failed.length)}
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
									{kindLabel(job)} · {attemptLabel(job)}{job.agentName ? ` · ${job.agentName}` : ''}
								</p>
							</div>
							<Button
								variant="outline"
								size="sm"
								class="self-start sm:self-center"
								disabled={manager.busy.has(job.id)}
								onclick={() => manager.retry(job)}
							>
								<RotateCcw />
								Retry
							</Button>
						</li>
					{/each}
				</ul>
			</section>
		{/if}
	</div>
{/if}

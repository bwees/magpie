<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as Collapsible from '$lib/components/ui/collapsible';
	import { cn } from '$lib/utils';
	import JobProgress from './JobProgress.svelte';
	import { APP_LABELS, MigrationPhase, type ArrApp } from './migration';
	import type { MigrationWizard } from './migration-wizard.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();

	const report = $derived(wizard.state?.report);
	const failed = $derived(wizard.phase === MigrationPhase.DRY_RUN_FAILED);
	const running = $derived(wizard.phase === MigrationPhase.DRY_RUN_RUNNING);
	/** The report's counts; a detail row qualifies the row above it. */
	const rows = $derived(
		report
			? [
					{ label: 'Series with other numbering', value: report.numberingMismatches },
					{ label: 'Episodes without a match', value: report.unmappedEpisodes },
					{ label: 'Custom formats', value: report.formats },
					{ label: 'from release profiles', value: report.releaseProfileFormats, detail: true },
					{ label: 'with a clash', value: report.formatClashes, detail: true },
					{ label: 'conditions to review', value: report.untranslatedSpecs, detail: true },
					{ label: 'Quality profiles', value: report.profiles },
					{ label: 'with a clash', value: report.profileClashes, detail: true },
					{ label: 'Files', value: report.files },
					{ label: 'not found on disk', value: report.missingFiles, detail: true },
					{ label: 'Blocklist rows with a hash', value: report.blocklistWithHash },
					{ label: 'without one', value: report.blocklistWithoutHash, detail: true },
					{ label: 'History rows', value: report.history }
				]
			: []
	);
</script>

<div class="grid gap-4">
	<p class="text-sm text-muted-foreground">
		The dry run reads both apps and matches every title on TMDB. It writes nothing; a big library
		takes a few minutes.
	</p>

	{#if running && wizard.state}
		<JobProgress progress={wizard.state.progress} label="Dry run" />
	{/if}
	{#if failed}
		<p class="text-sm text-destructive" role="alert">The dry run stopped: {wizard.state?.error}</p>
	{/if}

	{#if report && !running}
		<div class="grid gap-3">
			<div class="grid gap-3 sm:grid-cols-2">
				{#each report.apps as app (app.app)}
					<div class="rounded-lg border p-3">
						<p class="text-sm font-medium">{APP_LABELS[app.app as ArrApp]} titles</p>
						<dl class="mt-2 grid grid-cols-3 gap-2 text-center">
							<div>
								<dt class="text-xs text-muted-foreground">Matched</dt>
								<dd class="text-lg tabular-nums">{app.matched}</dd>
							</div>
							<div>
								<dt class="text-xs text-muted-foreground">To review</dt>
								<dd class="text-lg tabular-nums">{app.review}</dd>
							</div>
							<div>
								<dt class="text-xs text-muted-foreground">Skipped</dt>
								<dd class="text-lg tabular-nums">{app.skipped}</dd>
							</div>
						</dl>
					</div>
				{/each}
			</div>
			<dl class="grid gap-y-1.5 rounded-lg border p-3 text-sm">
				{#each rows as row, index (index)}
					<div class={cn('flex justify-between gap-3', row.detail && 'pl-4')}>
						<dt class="text-muted-foreground">{row.label}</dt>
						<dd class="tabular-nums">{row.value}</dd>
					</div>
				{/each}
			</dl>
			{#if report.warnings.length > 0}
				<Collapsible.Root>
					<Collapsible.Trigger
						class="text-sm text-muted-foreground underline-offset-4 hover:underline"
					>
						{report.warnings.length} things that won't carry over
					</Collapsible.Trigger>
					<Collapsible.Content>
						<ul class="mt-2 grid list-disc gap-1 pl-5 text-xs text-muted-foreground">
							{#each report.warnings as warning (warning)}
								<li>{warning}</li>
							{/each}
						</ul>
					</Collapsible.Content>
				</Collapsible.Root>
			{/if}
		</div>
	{/if}

	<div class="flex flex-wrap justify-end gap-2">
		<Button
			variant={report ? 'outline' : 'default'}
			disabled={wizard.busy || running}
			onclick={() => wizard.startDryRun()}
		>
			{running ? 'Running…' : report ? 'Run again' : 'Run the dry run'}
		</Button>
		{#if report && !running}
			<Button onclick={() => wizard.next()}>Review</Button>
		{/if}
	</div>
</div>

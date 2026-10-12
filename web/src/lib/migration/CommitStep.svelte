<script lang="ts">
	import Check from '@lucide/svelte/icons/check';
	import X from '@lucide/svelte/icons/x';
	import { Button } from '$lib/components/ui/button';
	import { settingsPath, SettingsSection } from '$lib/settings/sections';
	import JobProgress from './JobProgress.svelte';
	import { MigrationPhase, STEP_LABELS, WizardStep } from './migration';
	import type { MigrationWizard } from './migration-wizard.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();

	const state = $derived(wizard.state);
	const summary = $derived(state?.summary);
	const checks = $derived([
		{
			label: 'Every review queue is settled',
			ok: wizard.pendingReviews === 0,
			step: WizardStep.REVIEW
		},
		{
			label: 'The cutover checklist is ticked',
			ok: state?.cutoverConfirmed ?? false,
			step: WizardStep.CUTOVER
		}
	]);
	const leftovers = $derived(
		summary
			? [
					{ label: 'Titles not imported', items: summary.skippedTitles },
					{ label: "Files Magpie couldn't find", items: summary.missingFiles },
					{ label: "Files whose episodes didn't map", items: summary.unmappedFiles }
				].filter((group) => group.items.length > 0)
			: []
	);
</script>

<div class="grid gap-4">
	{#if wizard.phase === MigrationPhase.DONE && summary}
		<p class="text-sm">
			The import finished. Titles were added without a search; the daily and weekly sweeps pick them
			up.
		</p>
		<dl class="grid grid-cols-2 gap-x-6 gap-y-1.5 rounded-lg border p-3 text-sm sm:grid-cols-4">
			{#each summary.steps as step (step.step)}
				<div class="flex justify-between gap-2 sm:grid sm:gap-0.5">
					<dt class="text-muted-foreground">{STEP_LABELS[step.step]}</dt>
					<dd class="tabular-nums">{step.imported}</dd>
				</div>
			{/each}
		</dl>
		{#each leftovers as group (group.label)}
			<section class="grid gap-1.5">
				<h3 class="text-sm font-medium">{group.label} ({group.items.length})</h3>
				<ul
					class="grid max-h-48 gap-0.5 overflow-y-auto rounded-md border p-2 font-mono text-xs break-all"
				>
					{#each group.items as item (item)}
						<li>{item}</li>
					{/each}
				</ul>
			</section>
		{/each}
		<p class="text-sm text-muted-foreground">
			Files left behind can be matched with the
			<a
				class="text-primary underline-offset-4 hover:underline"
				href={settingsPath(SettingsSection.LIBRARY)}>library scan</a
			>.
		</p>
	{:else}
		<ul class="grid gap-1.5 text-sm">
			{#each checks as check (check.label)}
				<li class="flex items-center gap-2">
					{#if check.ok}<Check class="size-4 text-primary" />{:else}<X
							class="size-4 text-destructive"
						/>{/if}
					<span class={check.ok ? '' : 'text-muted-foreground'}>{check.label}</span>
					{#if !check.ok && wizard.canVisit(check.step)}
						<Button
							size="sm"
							variant="link"
							class="h-auto p-0"
							onclick={() => wizard.visit(check.step)}>Go there</Button
						>
					{/if}
				</li>
			{/each}
		</ul>
		{#if wizard.phase === MigrationPhase.COMMIT_RUNNING && state}
			<JobProgress progress={state.progress} label="Importing" />
		{/if}
		{#if wizard.phase === MigrationPhase.COMMIT_FAILED}
			<p class="text-sm text-destructive" role="alert">
				The import stopped at {STEP_LABELS[state?.failedStep ?? ''] ?? 'a step'}: {state?.error}.
				Resuming skips what's done.
			</p>
		{/if}
		<div class="flex justify-end">
			<Button disabled={!wizard.canCommit} onclick={() => wizard.startCommit()}>
				{wizard.phase === MigrationPhase.COMMIT_FAILED
					? 'Resume import'
					: wizard.running
						? 'Importing…'
						: 'Commit'}
			</Button>
		</div>
	{/if}
</div>

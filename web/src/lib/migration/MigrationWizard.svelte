<script lang="ts">
	import { onMount } from 'svelte';
	import Check from '@lucide/svelte/icons/check';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { LoadStatus } from '$lib/load-status';
	import { cn } from '$lib/utils';
	import CommitStep from './CommitStep.svelte';
	import ConnectStep from './ConnectStep.svelte';
	import CutoverStep from './CutoverStep.svelte';
	import DryRunStep from './DryRunStep.svelte';
	import { WIZARD_STEPS, WizardStep } from './migration';
	import { MigrationWizard, type MigrationLive } from './migration-wizard.svelte';
	import OptionsStep from './OptionsStep.svelte';
	import PathsStep from './PathsStep.svelte';
	import ReviewStep from './ReviewStep.svelte';

	let { live }: { live?: MigrationLive } = $props();

	const wizard = new MigrationWizard();
	const SKELETON_ROWS = 3;

	onMount(() => {
		void wizard.load();
		return wizard.watch(live);
	});

	const currentIndex = $derived(WIZARD_STEPS.findIndex((s) => s.step === wizard.step));
</script>

{#if wizard.status === LoadStatus.LOADING}
	<div class="grid gap-2" aria-busy="true">
		{#each { length: SKELETON_ROWS }, row (row)}
			<Skeleton class="h-10 w-full" />
		{/each}
	</div>
{:else if wizard.status === LoadStatus.ERROR}
	<div class="flex flex-wrap items-center gap-3">
		<p class="text-sm text-destructive">{wizard.error}</p>
		<Button size="sm" variant="outline" onclick={() => wizard.load()}>Retry</Button>
	</div>
{:else}
	<div class="grid min-w-0 gap-5 [&>*]:min-w-0">
		<ol
			class="-mx-1 flex gap-1 overflow-x-auto px-1 pb-1 text-sm sm:flex-wrap"
			aria-label="Import steps"
		>
			{#each WIZARD_STEPS as spec, index (spec.step)}
				{@const active = spec.step === wizard.step}
				<li class="shrink-0">
					<button
						type="button"
						class={cn(
							'flex items-center gap-2 rounded-md px-2.5 py-1.5 whitespace-nowrap',
							active ? 'bg-muted font-medium' : 'text-muted-foreground',
							wizard.canVisit(spec.step) && !active && 'hover:bg-muted/60 hover:text-foreground'
						)}
						aria-current={active ? 'step' : undefined}
						disabled={!wizard.canVisit(spec.step)}
						onclick={() => wizard.visit(spec.step)}
					>
						<span
							class={cn(
								'flex size-5 items-center justify-center rounded-full border text-[10px]',
								index < currentIndex && 'border-primary bg-primary text-primary-foreground',
								active && 'border-primary text-primary'
							)}
						>
							{#if index < currentIndex}<Check class="size-3" />{:else}{index + 1}{/if}
						</span>
						{spec.label}
					</button>
				</li>
			{/each}
		</ol>

		{#if wizard.step === WizardStep.CONNECT}
			<ConnectStep {wizard} />
		{:else if wizard.step === WizardStep.PATHS}
			<PathsStep {wizard} />
		{:else if wizard.step === WizardStep.CUTOVER}
			<CutoverStep {wizard} />
		{:else if wizard.step === WizardStep.DRY_RUN}
			<DryRunStep {wizard} />
		{:else if wizard.step === WizardStep.REVIEW}
			<ReviewStep {wizard} />
		{:else if wizard.step === WizardStep.OPTIONS}
			<OptionsStep {wizard} />
		{:else}
			<CommitStep {wizard} />
		{/if}

		{#if wizard.actionError}
			<p class="text-sm text-destructive" role="alert">{wizard.actionError}</p>
		{/if}
	</div>
{/if}

<script lang="ts">
	import Check from '@lucide/svelte/icons/check';
	import SkipForward from '@lucide/svelte/icons/skip-forward';
	import { cn } from '$lib/utils';
	import type { SetupWizard } from './setup-wizard.svelte';
	import { StepOutcome, WIZARD_STEPS } from './steps';

	let { wizard }: { wizard: SetupWizard } = $props();
</script>

<ol class="grid gap-0.5" aria-label="Setup steps">
	{#each WIZARD_STEPS as spec, index (spec.step)}
		{@const outcome = wizard.outcomes[spec.step]}
		{@const active = index === wizard.index}
		<li>
			<button
				type="button"
				class={cn(
					'flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left text-sm',
					active ? 'bg-muted font-medium' : 'text-muted-foreground',
					wizard.canVisit(index) && !active && 'hover:bg-muted/60 hover:text-foreground'
				)}
				aria-current={active ? 'step' : undefined}
				disabled={!wizard.canVisit(index)}
				onclick={() => wizard.visit(index)}
			>
				<span
					class={cn(
						'flex size-5 shrink-0 items-center justify-center rounded-full border text-[10px]',
						outcome === StepOutcome.DONE && 'border-primary bg-primary text-primary-foreground',
						active && outcome === StepOutcome.PENDING && 'border-primary text-primary'
					)}
				>
					{#if outcome === StepOutcome.DONE}
						<Check class="size-3" />
					{:else if outcome === StepOutcome.SKIPPED}
						<SkipForward class="size-3" />
					{:else}
						{index + 1}
					{/if}
				</span>
				<span class="min-w-0 flex-1 truncate">{spec.label}</span>
				{#if outcome === StepOutcome.SKIPPED}
					<span class="text-xs text-muted-foreground">Skipped</span>
				{/if}
			</button>
		</li>
	{/each}
</ol>

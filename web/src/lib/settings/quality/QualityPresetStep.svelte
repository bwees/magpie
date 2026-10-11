<script lang="ts">
	import { onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { LoadStatus } from '$lib/load-status';
	import { PresetStep } from './preset-step.svelte';
	import { PRESETS } from './quality';

	let { isLast, onDone, onSkip }: { isLast: boolean; onDone: () => void; onSkip: () => void } =
		$props();

	const step = new PresetStep();

	onMount(() => {
		step.load();
	});

	async function save() {
		if (await step.save()) onDone();
	}
</script>

<Card.Content class="grid gap-3">
	{#if step.status === LoadStatus.LOADING}
		<Skeleton class="h-24 w-full" />
	{:else if step.status === LoadStatus.ERROR}
		<div class="flex flex-wrap items-center gap-3">
			<p class="text-sm text-destructive">{step.error}</p>
			<Button size="sm" variant="outline" onclick={() => step.load()}>Retry</Button>
		</div>
	{:else}
		<div class="grid gap-2 sm:grid-cols-3" role="radiogroup" aria-label="Default quality preset">
			{#each PRESETS as spec (spec.preset)}
				<button
					type="button"
					role="radio"
					aria-checked={step.selected === spec.preset}
					class="rounded-lg border p-3 text-left transition-colors hover:bg-muted {step.selected ===
					spec.preset
						? 'border-primary bg-muted'
						: ''}"
					onclick={() => (step.selected = spec.preset)}
				>
					<div class="font-medium">{spec.label}</div>
					<div class="text-xs text-muted-foreground">{spec.description}</div>
				</button>
			{/each}
		</div>
		<p class="text-xs text-muted-foreground">
			Every source at the chosen resolution is accepted. TRaSH Guides scores decide between them.
		</p>
		{#if step.error}
			<p class="text-sm text-destructive" role="alert">{step.error}</p>
		{/if}
	{/if}
</Card.Content>
<Card.Footer class="flex-wrap justify-end gap-2">
	<Button variant="ghost" disabled={step.saving} onclick={onSkip}>Skip</Button>
	<Button disabled={step.saving || step.status !== LoadStatus.READY} onclick={save}>
		{step.saving ? 'Saving…' : isLast ? 'Finish' : 'Continue'}
	</Button>
</Card.Footer>

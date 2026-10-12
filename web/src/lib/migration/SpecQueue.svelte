<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import * as Select from '$lib/components/ui/select';
	import DecisionBadge from './DecisionBadge.svelte';
	import { APP_LABELS, ReviewQueue, SpecDecision, type ArrApp } from './migration';
	import type { MigrationWizard } from './migration-wizard.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();

	const specs = $derived(wizard.state?.reviews?.specs ?? []);
	let values = $state<Record<string, string>>({});

	const DECISION_LABELS: Partial<Record<SpecDecision, string>> = {
		[SpecDecision.DROP]: 'Dropped',
		[SpecDecision.EDIT]: 'Edited'
	};

	function decide(id: string, spec: SpecDecision, value = '') {
		void wizard.decide({ queue: ReviewQueue.SPECS, id, spec, value });
	}
</script>

{#if specs.length === 0}
	<p class="text-sm text-muted-foreground">Every custom-format condition translated.</p>
{:else}
	<ul class="grid gap-3">
		{#each specs as review (review.id)}
			{@const value = values[review.id] ?? review.value}
			<li class="grid gap-2 rounded-lg border p-3">
				<div class="flex flex-wrap items-center gap-2">
					<span class="font-medium">{review.formatName}</span>
					<span class="text-sm text-muted-foreground"
						>{review.specName} · {APP_LABELS[review.app as ArrApp]}</span
					>
					<DecisionBadge label={DECISION_LABELS[review.decision as SpecDecision]} />
				</div>
				<p class="text-sm text-muted-foreground">{review.reason}</p>
				<div class="flex flex-wrap items-center gap-2">
					{#if review.kind !== ''}
						{#if review.options.length > 0}
							<Select.Root type="single" {value} onValueChange={(v) => (values[review.id] = v)}>
								<Select.Trigger class="w-48" aria-label="New value for {review.specName}"
									>{value || 'Choose a value'}</Select.Trigger
								>
								<Select.Content>
									{#each review.options as option (option)}
										<Select.Item value={option} label={option} />
									{/each}
								</Select.Content>
							</Select.Root>
						{:else}
							<Input
								class="w-48"
								{value}
								oninput={(event) => (values[review.id] = event.currentTarget.value)}
								aria-label="New value for {review.specName}"
							/>
						{/if}
						<Button
							size="sm"
							variant="secondary"
							disabled={wizard.busy || !value}
							onclick={() => decide(review.id, SpecDecision.EDIT, value)}>Use this</Button
						>
					{/if}
					<Button
						size="sm"
						variant="ghost"
						disabled={wizard.busy}
						onclick={() => decide(review.id, SpecDecision.DROP)}>Drop</Button
					>
				</div>
			</li>
		{/each}
	</ul>
{/if}

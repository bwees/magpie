<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import DecisionBadge from './DecisionBadge.svelte';
	import { APP_LABELS, ArrApp, ClashChoice, ClashKind, ReviewQueue } from './migration';
	import type { MigrationWizard } from './migration-wizard.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();

	const clashes = $derived(wizard.state?.reviews?.clashes ?? []);

	const KIND_LABELS: Record<ClashKind, string> = {
		[ClashKind.FORMAT]: 'Custom format',
		[ClashKind.PROFILE]: 'Quality profile'
	};
	const CHOICE_LABELS: Partial<Record<ClashChoice, string>> = {
		[ClashChoice.SONARR]: "Sonarr's",
		[ClashChoice.RADARR]: "Radarr's",
		[ClashChoice.KEEP_BOTH]: 'Both kept'
	};
	const SIDES = [
		{ app: ArrApp.SONARR, choice: ClashChoice.SONARR },
		{ app: ArrApp.RADARR, choice: ClashChoice.RADARR }
	];

	function choose(id: string, clash: ClashChoice) {
		void wizard.decide({ queue: ReviewQueue.CLASHES, id, clash });
	}
</script>

{#if clashes.length === 0}
	<p class="text-sm text-muted-foreground">
		Formats and profiles named alike in both apps are the same, or merge cleanly.
	</p>
{:else}
	<ul class="grid gap-3">
		{#each clashes as clash (clash.id)}
			<li class="grid gap-3 rounded-lg border p-3">
				<div class="flex flex-wrap items-center gap-2">
					<span class="font-medium">{clash.name}</span>
					<span class="text-sm text-muted-foreground">{KIND_LABELS[clash.kind as ClashKind]}</span>
					<DecisionBadge label={CHOICE_LABELS[clash.choice as ClashChoice]} />
				</div>
				<div class="grid gap-3 md:grid-cols-2">
					{#each SIDES as side (side.app)}
						{@const lines = side.app === ArrApp.SONARR ? clash.sonarr : clash.radarr}
						{@const other = side.app === ArrApp.SONARR ? clash.radarr : clash.sonarr}
						<div class="grid min-w-0 content-start gap-2 rounded-md bg-muted/40 p-2.5">
							<p class="text-xs font-medium text-muted-foreground">{APP_LABELS[side.app]}</p>
							<ul class="grid gap-0.5 font-mono text-xs break-words">
								{#each lines as line, index (index)}
									<li class={other.includes(line) ? 'text-muted-foreground' : 'text-foreground'}>
										{line}
									</li>
								{/each}
							</ul>
							<Button
								size="sm"
								variant={clash.choice === side.choice ? 'default' : 'secondary'}
								disabled={wizard.busy}
								onclick={() => choose(clash.id, side.choice)}
							>
								Use {APP_LABELS[side.app]}'s
							</Button>
						</div>
					{/each}
				</div>
				<div class="flex justify-end">
					<Button
						size="sm"
						variant={clash.choice === ClashChoice.KEEP_BOTH ? 'default' : 'ghost'}
						disabled={wizard.busy}
						onclick={() => choose(clash.id, ClashChoice.KEEP_BOTH)}
					>
						Keep both as "(TV)" and "(Movies)"
					</Button>
				</div>
			</li>
		{/each}
	</ul>
{/if}

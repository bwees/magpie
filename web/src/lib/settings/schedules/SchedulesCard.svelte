<script lang="ts">
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import * as Collapsible from '$lib/components/ui/collapsible';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { LoadStatus } from '$lib/load-status';
	import { LIMITS } from './schedules';
	import { SchedulesManager } from './schedules-manager.svelte';

	const manager = new SchedulesManager();

	$effect(() => {
		if (manager.open && manager.status === LoadStatus.LOADING) {
			void manager.load();
		}
	});
</script>

<Card.Root>
	<Collapsible.Root bind:open={manager.open}>
		<Card.Header>
			<Card.Title>Schedules</Card.Title>
			<Card.Description>How often Magpie looks for releases without being asked.</Card.Description>
			<Card.Action>
				<Collapsible.Trigger>
					{#snippet child({ props })}
						<Button {...props} variant="ghost" size="sm">
							Advanced
							<ChevronDown class={manager.open ? 'rotate-180' : ''} />
						</Button>
					{/snippet}
				</Collapsible.Trigger>
			</Card.Action>
		</Card.Header>
		<Collapsible.Content>
			<Card.Content class="pt-4">
				{#if manager.status === LoadStatus.LOADING}
					<Skeleton class="h-40 w-full" aria-busy="true" />
				{:else if manager.status === LoadStatus.ERROR}
					<div class="flex flex-wrap items-center gap-3">
						<p class="text-sm text-destructive" role="alert">{manager.error}</p>
						<Button size="sm" variant="outline" onclick={() => manager.load()}>Retry</Button>
					</div>
				{:else}
					<form
						class="grid gap-4"
						onsubmit={(event) => {
							event.preventDefault();
							manager.save();
						}}
					>
						<div class="grid gap-4 sm:grid-cols-2">
							<div class="grid gap-1.5">
								<Label for="rss-interval">RSS sync per indexer</Label>
								<div class="flex items-center gap-2">
									<Input
										id="rss-interval"
										type="number"
										class="w-24"
										min={LIMITS.RSS_MINUTES.MIN}
										max={LIMITS.RSS_MINUTES.MAX}
										bind:value={manager.draft.rssIntervalMinutes}
									/>
									<span class="text-sm text-muted-foreground">minutes</span>
								</div>
							</div>
							<div class="grid gap-1.5">
								<span class="text-sm font-medium">Pending (delay) queue check</span>
								<span class="flex h-9 items-center text-sm text-muted-foreground">
									Every {manager.saved?.pendingCheckMinutes} min (fixed)
								</span>
							</div>
							<div class="grid gap-1.5">
								<Label for="missing-sweep">Missing backlog sweep</Label>
								<div class="flex items-center gap-2">
									<Input
										id="missing-sweep"
										type="number"
										class="w-24"
										min={LIMITS.MISSING_SWEEP_DAYS.MIN}
										max={LIMITS.MISSING_SWEEP_DAYS.MAX}
										bind:value={manager.draft.missingSweepDays}
									/>
									<span class="text-sm text-muted-foreground">days</span>
								</div>
							</div>
							<div class="grid gap-1.5">
								<Label for="cutoff-sweep">Cutoff-unmet sweep</Label>
								<div class="flex items-center gap-2">
									<Input
										id="cutoff-sweep"
										type="number"
										class="w-24"
										min={LIMITS.CUTOFF_SWEEP_DAYS.MIN}
										max={LIMITS.CUTOFF_SWEEP_DAYS.MAX}
										bind:value={manager.draft.cutoffSweepDays}
									/>
									<span class="text-sm text-muted-foreground">days</span>
								</div>
							</div>
						</div>
						<p class="text-xs text-muted-foreground">
							Sweep searches are spaced so each indexer gets at most one query every 2 seconds.
						</p>
						<div class="flex items-center gap-3">
							<Button type="submit" size="sm" disabled={manager.saving}>Save</Button>
							{#if manager.draftError}
								<span class="text-sm text-destructive" role="alert">{manager.draftError}</span>
							{/if}
						</div>
					</form>
				{/if}
			</Card.Content>
		</Collapsible.Content>
	</Collapsible.Root>
</Card.Root>

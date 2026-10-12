<script lang="ts">
	import { onMount } from 'svelte';
	import Plus from '@lucide/svelte/icons/plus';
	import X from '@lucide/svelte/icons/x';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Switch } from '$lib/components/ui/switch';
	import { LoadStatus } from '$lib/load-status';
	import { LIMITS, REGEX_PREFIX, STALLED_LIMIT_STEP_HOURS } from './download-options';
	import { DownloadOptionsManager } from './download-options-manager.svelte';
	import SeedingModeSelect from './SeedingModeSelect.svelte';

	const manager = new DownloadOptionsManager();

	onMount(() => {
		void manager.load();
	});
</script>

<div class="grid gap-6">
	<p class="text-sm text-muted-foreground">
		Magpie checks every torrent's file list before any content downloads.
	</p>

	{#if manager.status === LoadStatus.LOADING}
		<div class="grid gap-4" aria-busy="true" aria-label="Loading download settings">
			<Skeleton class="h-40 rounded-xl" />
			<Skeleton class="h-32 rounded-xl" />
		</div>
	{:else if manager.status === LoadStatus.ERROR || !manager.saved}
		<section
			class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center"
			role="alert"
		>
			<p class="text-sm text-muted-foreground">{manager.error}</p>
			<Button variant="outline" size="sm" onclick={() => manager.load()}>Retry</Button>
		</section>
	{:else}
		<Card.Root>
			<Card.Header>
				<div class="flex items-start justify-between gap-4">
					<div class="grid gap-1">
						<Card.Title
							><label for="blocked-files">Reject releases with blocked files</label></Card.Title
						>
						<Card.Description>
							Any matching file rejects the whole release, which is blocklisted for good. Patterns
							match file names, not folders, and ignore case. Prefix a regular expression with
							<code class="font-mono">{REGEX_PREFIX}</code>.
						</Card.Description>
					</div>
					<Switch
						id="blocked-files"
						checked={manager.saved.blockedFilesEnabled}
						disabled={manager.saving}
						onCheckedChange={(checked) => manager.setEnabled(checked)}
					/>
				</div>
			</Card.Header>
			<Card.Content class="grid gap-3">
				{#if manager.saved.blockedPatterns.length === 0}
					<p class="text-sm text-muted-foreground">No patterns. Every file passes the check.</p>
				{:else}
					<ul class="flex flex-wrap gap-1.5" aria-label="Blocked file patterns">
						{#each manager.saved.blockedPatterns as pattern (pattern)}
							<li
								class={[
									'flex items-center gap-1 rounded-md border py-0.5 pr-0.5 pl-2 font-mono text-xs',
									!manager.saved.blockedFilesEnabled && 'opacity-50'
								]}
							>
								{pattern}
								<button
									type="button"
									class="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
									aria-label={`Remove ${pattern}`}
									disabled={manager.saving}
									onclick={() => manager.removePattern(pattern)}
								>
									<X class="size-3" />
								</button>
							</li>
						{/each}
					</ul>
				{/if}
				<form
					class="flex flex-col gap-1.5"
					onsubmit={(event) => {
						event.preventDefault();
						manager.addPattern();
					}}
				>
					<div class="flex gap-2">
						<Input
							class="max-w-xs font-mono"
							placeholder="*.exe or regex:^setup\."
							aria-label="New blocked pattern"
							aria-invalid={manager.patternError ? true : undefined}
							bind:value={manager.newPattern}
						/>
						<Button type="submit" size="sm" variant="outline" disabled={manager.saving}>
							<Plus />Add
						</Button>
					</div>
					{#if manager.patternError}
						<span class="text-sm text-destructive" role="alert">{manager.patternError}</span>
					{/if}
				</form>
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header>
				<Card.Title>Seeding</Card.Title>
				<Card.Description>
					A seeding download is removed with its data once it reaches the ratio or the days,
					whichever comes first. An indexer's own goal in Settings → Indexers wins over this one.
				</Card.Description>
			</Card.Header>
			<Card.Content class="grid gap-4">
				<div class="grid gap-1.5">
					<Label for="seeding-mode">After download</Label>
					<SeedingModeSelect {manager} id="seeding-mode" />
				</div>
				<form
					class="grid gap-4"
					onsubmit={(event) => {
						event.preventDefault();
						manager.saveSeedingGoal();
					}}
				>
					<div class="grid gap-4 sm:grid-cols-2">
						<div class="grid gap-1.5">
							<Label for="seed-ratio">Seed ratio</Label>
							<Input
								id="seed-ratio"
								type="number"
								class="w-24"
								step={LIMITS.SEED_RATIO.STEP}
								min={LIMITS.SEED_RATIO.MIN}
								max={LIMITS.SEED_RATIO.MAX}
								bind:value={manager.seedingGoal.seedRatio}
							/>
						</div>
						<div class="grid gap-1.5">
							<Label for="seed-days">Seed time</Label>
							<div class="flex items-center gap-2">
								<Input
									id="seed-days"
									type="number"
									class="w-24"
									min={LIMITS.SEED_DAYS.MIN}
									max={LIMITS.SEED_DAYS.MAX}
									bind:value={manager.seedingGoal.seedDays}
								/>
								<span class="text-sm text-muted-foreground">days</span>
							</div>
						</div>
					</div>
					<div class="flex items-center gap-3">
						<Button type="submit" size="sm" disabled={manager.saving}>Save</Button>
						{#if manager.seedingGoalError}
							<span class="text-sm text-destructive" role="alert">{manager.seedingGoalError}</span>
						{/if}
					</div>
				</form>
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header>
				<Card.Title>Timeouts</Card.Title>
				<Card.Description>
					A release that hits either limit is removed, blocklisted for 24 hours, and the next best
					release is grabbed.
				</Card.Description>
			</Card.Header>
			<Card.Content>
				<form
					class="grid gap-4"
					onsubmit={(event) => {
						event.preventDefault();
						manager.saveTimeouts();
					}}
				>
					<div class="grid gap-4 sm:grid-cols-2">
						<div class="grid gap-1.5">
							<Label for="metadata-timeout">Magnet metadata timeout</Label>
							<div class="flex items-center gap-2">
								<Input
									id="metadata-timeout"
									type="number"
									class="w-24"
									min={LIMITS.METADATA_TIMEOUT_MINUTES.MIN}
									max={LIMITS.METADATA_TIMEOUT_MINUTES.MAX}
									bind:value={manager.timeouts.metadataTimeoutMinutes}
								/>
								<span class="text-sm text-muted-foreground">minutes</span>
							</div>
						</div>
						<div class="grid gap-1.5">
							<Label for="stalled-limit">Stalled download limit</Label>
							<div class="flex items-center gap-2">
								<Input
									id="stalled-limit"
									type="number"
									class="w-24"
									step={STALLED_LIMIT_STEP_HOURS}
									min={LIMITS.STALLED_LIMIT_HOURS.MIN}
									max={LIMITS.STALLED_LIMIT_HOURS.MAX}
									bind:value={manager.timeouts.stalledLimitHours}
								/>
								<span class="text-sm text-muted-foreground">hours</span>
							</div>
						</div>
					</div>
					<div class="flex items-center gap-3">
						<Button type="submit" size="sm" disabled={manager.saving}>Save</Button>
						{#if manager.timeoutsError}
							<span class="text-sm text-destructive" role="alert">{manager.timeoutsError}</span>
						{/if}
					</div>
				</form>
			</Card.Content>
		</Card.Root>
	{/if}
</div>

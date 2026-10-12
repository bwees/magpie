<script lang="ts">
	import { onMount } from 'svelte';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Switch } from '$lib/components/ui/switch';
	import { LoadStatus } from '$lib/load-status';
	import QuotaBadge from './QuotaBadge.svelte';
	import { SubtitleSettings } from './subtitle-settings.svelte';
	import { LIMITS, WHISPER_MODELS } from './subtitles';

	const settings = new SubtitleSettings();

	onMount(() => {
		void settings.load();
	});
</script>

<Card.Root>
	<Card.Header>
		<div class="flex flex-wrap items-start justify-between gap-2">
			<div class="grid gap-1">
				<Card.Title>Finding subtitles</Card.Title>
				<Card.Description>
					Embedded tracks and release subtitles come first. Missing languages are searched on
					OpenSubtitles with the login under Settings → Connections.
				</Card.Description>
			</div>
			{#if settings.quota}
				<QuotaBadge quota={settings.quota} />
			{/if}
		</div>
	</Card.Header>
	<Card.Content>
		{#if settings.status === LoadStatus.LOADING}
			<Skeleton class="h-48 w-full" aria-label="Loading subtitle settings" />
		{:else if settings.status === LoadStatus.ERROR || !settings.draft}
			<div class="flex flex-wrap items-center gap-3" role="alert">
				<p class="text-sm text-destructive">{settings.error}</p>
				<Button size="sm" variant="outline" onclick={() => settings.load()}>Retry</Button>
			</div>
		{:else}
			<form
				class="grid gap-5"
				onsubmit={(event) => {
					event.preventDefault();
					settings.save();
				}}
			>
				{#if settings.quota?.problem}
					<Alert.Root variant="destructive">
						<TriangleAlert />
						<Alert.Title>OpenSubtitles is paused</Alert.Title>
						<Alert.Description>{settings.quota.problem}</Alert.Description>
					</Alert.Root>
				{/if}

				<div class="flex items-start justify-between gap-4">
					<div class="grid gap-1">
						<Label for="auto-sync">Auto-sync downloaded subtitles</Label>
						<p class="text-xs text-muted-foreground">
							Shifts each OpenSubtitles subtitle to line up with the speech. Whisper output is never
							synced.
						</p>
					</div>
					<Switch id="auto-sync" bind:checked={settings.draft.autoSync} />
				</div>

				<div class="grid gap-3">
					<div class="flex items-start justify-between gap-4">
						<div class="grid gap-1">
							<Label for="whisper">Generate with Whisper when nothing is found</Label>
							<p class="text-xs text-muted-foreground">
								Original language only, on an agent with Whisper.
							</p>
						</div>
						<Switch id="whisper" bind:checked={settings.draft.whisperEnabled} />
					</div>
					<div class="grid gap-1.5">
						<Label for="whisper-model">Whisper model</Label>
						<Select.Root
							type="single"
							bind:value={settings.draft.whisperModel}
							disabled={!settings.draft.whisperEnabled}
						>
							<Select.Trigger id="whisper-model" class="w-full sm:w-56">
								{settings.draft.whisperModel}
							</Select.Trigger>
							<Select.Content>
								{#each WHISPER_MODELS as model (model)}
									<Select.Item value={model} label={model} />
								{/each}
							</Select.Content>
						</Select.Root>
					</div>
				</div>

				<div class="grid gap-4 sm:grid-cols-2">
					<div class="grid gap-1.5">
						<Label for="min-score-episode">Minimum score, episodes</Label>
						<div class="flex items-center gap-2">
							<Input
								id="min-score-episode"
								type="number"
								class="w-24"
								min={LIMITS.MIN_SCORE.MIN}
								max={LIMITS.MIN_SCORE.MAX}
								bind:value={settings.draft.minScoreEpisode}
							/>
							<span class="text-sm text-muted-foreground">%</span>
						</div>
					</div>
					<div class="grid gap-1.5">
						<Label for="min-score-movie">Minimum score, movies</Label>
						<div class="flex items-center gap-2">
							<Input
								id="min-score-movie"
								type="number"
								class="w-24"
								min={LIMITS.MIN_SCORE.MIN}
								max={LIMITS.MIN_SCORE.MAX}
								bind:value={settings.draft.minScoreMovie}
							/>
							<span class="text-sm text-muted-foreground">%</span>
						</div>
					</div>
					<div class="grid gap-1.5">
						<Label for="upgrade-window">Upgrade window</Label>
						<div class="flex items-center gap-2">
							<Input
								id="upgrade-window"
								type="number"
								class="w-24"
								min={LIMITS.UPGRADE_WINDOW_DAYS.MIN}
								max={LIMITS.UPGRADE_WINDOW_DAYS.MAX}
								bind:value={settings.draft.upgradeWindowDays}
							/>
							<span class="text-sm text-muted-foreground">days</span>
						</div>
					</div>
					<div class="grid gap-1.5">
						<Label for="upgrade-check">Look for upgrades every</Label>
						<div class="flex items-center gap-2">
							<Input
								id="upgrade-check"
								type="number"
								class="w-24"
								min={LIMITS.UPGRADE_CHECK_HOURS.MIN}
								max={LIMITS.UPGRADE_CHECK_HOURS.MAX}
								bind:value={settings.draft.upgradeCheckHours}
							/>
							<span class="text-sm text-muted-foreground">hours</span>
						</div>
					</div>
				</div>
				<p class="text-xs text-muted-foreground">
					A subtitle that isn’t a near-perfect match is searched again during its upgrade window and
					replaced only by a better one. Missing languages are retried on the same schedule for a
					week, then daily.
				</p>

				<div class="flex flex-wrap items-center gap-3">
					<Button type="submit" size="sm" disabled={settings.saving || !settings.dirty}>
						{settings.saving ? 'Saving…' : 'Save'}
					</Button>
					{#if settings.dirty}
						<Button
							type="button"
							size="sm"
							variant="ghost"
							disabled={settings.saving}
							onclick={() => settings.discard()}>Discard</Button
						>
					{/if}
					{#if settings.draftError}
						<span class="text-sm text-destructive" role="alert">{settings.draftError}</span>
					{/if}
				</div>
			</form>
		{/if}
	</Card.Content>
</Card.Root>

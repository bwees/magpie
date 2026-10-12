<script lang="ts">
	import { onMount } from 'svelte';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import Lock from '@lucide/svelte/icons/lock';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import * as Collapsible from '$lib/components/ui/collapsible';
	import { Input } from '$lib/components/ui/input';
	import * as Select from '$lib/components/ui/select';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Switch } from '$lib/components/ui/switch';
	import { LoadStatus } from '$lib/load-status';
	import { TranscodeSettingsManager } from './transcode-settings-manager.svelte';
	import {
		BITRATE_CAP_MBPS,
		LOW_BITRATE_PCT,
		MIN_SAVING_PCT,
		PRESET_LABELS,
		Preset,
		RESOLUTION_LABELS,
		ResolutionClass,
		SKIP_LABELS,
		SkipReason,
		TARGET_HINTS,
		TARGET_LABELS,
		TargetCodec
	} from './transcode-settings';

	const SKELETON_CARDS = 3;
	const manager = new TranscodeSettingsManager();

	onMount(() => {
		void manager.load();
	});

	function submit(event: SubmitEvent) {
		event.preventDefault();
		void manager.save();
	}

	const skipped = $derived(
		Object.values(SkipReason).filter((reason) => (manager.backlog.skipped[reason] ?? 0) > 0)
	);
</script>

{#snippet row(id: string, label: string, hint: string)}
	<span class="text-sm">
		<label for={id}>{label}</label>
		<span class="block text-xs text-muted-foreground">{hint}</span>
	</span>
{/snippet}

<div class="flex flex-col gap-4">
	{#if manager.status === LoadStatus.LOADING}
		{#each { length: SKELETON_CARDS }, i (i)}
			<Skeleton class="h-40 rounded-xl" />
		{/each}
	{:else if manager.status === LoadStatus.ERROR}
		<section
			class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center"
			role="alert"
		>
			<p class="text-sm text-muted-foreground">{manager.error}</p>
			<Button variant="outline" size="sm" onclick={() => manager.load()}>Retry</Button>
		</section>
	{:else}
		<form class="flex flex-col gap-4" onsubmit={submit}>
			<Card.Root>
				<Card.Header>
					<Card.Title>Target</Card.Title>
					<Card.Description>
						Files are transcoded once they meet their quality cutoff. Output is always MKV.
					</Card.Description>
				</Card.Header>
				<Card.Content class="flex flex-col gap-4">
					<div class="flex items-center justify-between gap-4">
						{@render row(
							'transcode-enabled',
							'Transcoding',
							'Off holds the queue; running jobs finish'
						)}
						<Switch id="transcode-enabled" bind:checked={manager.enabled} />
					</div>
					<div class="grid gap-1.5 sm:grid-cols-[1fr_14rem] sm:items-center">
						{@render row(
							'transcode-target',
							'Target',
							TARGET_HINTS[manager.targetCodec as TargetCodec] ?? ''
						)}
						<Select.Root type="single" bind:value={manager.targetCodec}>
							<Select.Trigger id="transcode-target" class="w-full">
								{TARGET_LABELS[manager.targetCodec as TargetCodec] ?? manager.targetCodec}
							</Select.Trigger>
							<Select.Content>
								{#each Object.values(TargetCodec) as codec (codec)}
									<Select.Item value={codec} label={TARGET_LABELS[codec]} />
								{/each}
							</Select.Content>
						</Select.Root>
					</div>
					<div class="grid gap-1.5 sm:grid-cols-[1fr_14rem] sm:items-center">
						{@render row(
							'transcode-preset',
							'Quality preset',
							'Archive keeps the most detail; Small saves the most space'
						)}
						<Select.Root type="single" bind:value={manager.preset}>
							<Select.Trigger id="transcode-preset" class="w-full">
								{PRESET_LABELS[manager.preset as Preset] ?? manager.preset}
							</Select.Trigger>
							<Select.Content>
								{#each Object.values(Preset) as preset (preset)}
									<Select.Item value={preset} label={PRESET_LABELS[preset]} />
								{/each}
							</Select.Content>
						</Select.Root>
					</div>
				</Card.Content>
			</Card.Root>

			<Card.Root>
				<Card.Header>
					<Card.Title>Max bitrate</Card.Title>
					<Card.Description>
						The encode never goes above the cap for its resolution ({BITRATE_CAP_MBPS.MIN}–{BITRATE_CAP_MBPS.MAX}
						Mbps). Files already in the target codec but over their cap are transcoded too.
					</Card.Description>
				</Card.Header>
				<Card.Content class="grid grid-cols-2 gap-4 sm:grid-cols-4">
					{#each Object.values(ResolutionClass) as resolution (resolution)}
						<label class="grid gap-1.5">
							<span class="text-sm">{RESOLUTION_LABELS[resolution]}</span>
							<span class="flex items-center gap-2">
								<Input
									type="number"
									min={BITRATE_CAP_MBPS.MIN}
									max={BITRATE_CAP_MBPS.MAX}
									bind:value={manager.caps[resolution]}
									aria-invalid={!manager.capValid(resolution)}
								/>
								<span class="text-xs text-muted-foreground">Mbps</span>
							</span>
						</label>
					{/each}
				</Card.Content>
			</Card.Root>

			<Card.Root>
				<Card.Header>
					<Card.Title>Skip rules</Card.Title>
					<Card.Description>Skipped files keep their original encode.</Card.Description>
				</Card.Header>
				<Card.Content class="flex flex-col gap-4">
					<div class="flex items-center justify-between gap-4">
						{@render row(
							'skip-already-target',
							'Already the target codec and under its cap',
							'Leave these files as they are'
						)}
						<Switch id="skip-already-target" bind:checked={manager.skipAlreadyTarget} />
					</div>
					<div class="flex items-center justify-between gap-4">
						{@render row(
							'skip-low-bitrate',
							'Source bitrate already low',
							'Other codecs below a share of their resolution cap'
						)}
						<Switch id="skip-low-bitrate" bind:checked={manager.skipLowBitrate} />
					</div>
					{#if manager.skipLowBitrate}
						<div class="grid gap-1.5 sm:grid-cols-[1fr_8rem] sm:items-center">
							{@render row(
								'low-bitrate-pct',
								'Low bitrate means below',
								`Percent of the cap (${LOW_BITRATE_PCT.MIN}–${LOW_BITRATE_PCT.MAX})`
							)}
							<span class="flex items-center gap-2">
								<Input
									id="low-bitrate-pct"
									type="number"
									min={LOW_BITRATE_PCT.MIN}
									max={LOW_BITRATE_PCT.MAX}
									bind:value={manager.lowBitratePct}
									aria-invalid={!manager.lowBitrateValid}
								/>
								<span class="text-xs text-muted-foreground">%</span>
							</span>
						</div>
					{/if}
					<div class="grid gap-1.5 sm:grid-cols-[1fr_8rem] sm:items-center">
						{@render row(
							'min-saving-pct',
							'Keep the original unless the result is smaller by',
							`Percent of the source size (${MIN_SAVING_PCT.MIN}–${MIN_SAVING_PCT.MAX})`
						)}
						<span class="flex items-center gap-2">
							<Input
								id="min-saving-pct"
								type="number"
								min={MIN_SAVING_PCT.MIN}
								max={MIN_SAVING_PCT.MAX}
								bind:value={manager.minSavingPct}
								aria-invalid={!manager.minSavingValid}
							/>
							<span class="text-xs text-muted-foreground">%</span>
						</span>
					</div>
					<div class="flex items-center justify-between gap-4">
						<span class="text-sm">
							Dolby Vision and HDR10+
							<span class="block text-xs text-muted-foreground">
								Always kept as they are; no GPU encoder writes their dynamic metadata. HDR10 is
								transcoded with its metadata.
							</span>
						</span>
						<Badge variant="secondary" class="shrink-0"><Lock />Skipped</Badge>
					</div>
				</Card.Content>
			</Card.Root>

			<Card.Root>
				<Collapsible.Root bind:open={manager.advancedOpen}>
					<Card.Header>
						<Card.Title>Encoder quality</Card.Title>
						<Card.Description>
							Each preset maps to a calibrated value per encoder. Override one only to correct a GPU
							that looks worse than the others.
						</Card.Description>
						<Card.Action>
							<Collapsible.Trigger>
								{#snippet child({ props })}
									<Button {...props} variant="ghost" size="sm">
										Advanced
										<ChevronDown class={manager.advancedOpen ? 'rotate-180' : ''} />
									</Button>
								{/snippet}
							</Collapsible.Trigger>
						</Card.Action>
					</Card.Header>
					<Collapsible.Content>
						<Card.Content class="flex flex-col gap-3 pt-4">
							{#each manager.encoders as encoder (encoder.encoder)}
								<div class="grid gap-1.5 sm:grid-cols-[1fr_8rem] sm:items-center">
									{@render row(
										`override-${encoder.encoder}`,
										encoder.encoder,
										`${encoder.qualityArg} ${encoder.min}–${encoder.max}, lower is better`
									)}
									<Input
										id={`override-${encoder.encoder}`}
										type="number"
										min={encoder.min}
										max={encoder.max}
										placeholder={`${manager.calibrated(encoder) ?? ''}`}
										bind:value={manager.overrides[encoder.encoder]}
										aria-invalid={!manager.overrideValid(encoder)}
									/>
								</div>
							{/each}
						</Card.Content>
					</Collapsible.Content>
				</Collapsible.Root>
			</Card.Root>

			<div class="flex justify-end gap-2">
				<Button
					variant="ghost"
					disabled={!manager.dirty || manager.saving}
					onclick={() => manager.discard()}
				>
					Discard
				</Button>
				<Button type="submit" disabled={!manager.canSave}>
					{manager.saving ? 'Saving…' : 'Save'}
				</Button>
			</div>
		</form>

		<Card.Root>
			<Card.Header>
				<Card.Title>Backlog</Card.Title>
				<Card.Description>
					Files from library scans and migrations run only on agent slots downloads don't need.
				</Card.Description>
			</Card.Header>
			<Card.Content class="flex flex-col gap-4">
				<dl class="grid grid-cols-2 gap-4 sm:grid-cols-3">
					<div>
						<dt class="text-xs text-muted-foreground">Queued from the backlog</dt>
						<dd class="text-lg font-semibold tabular-nums">{manager.backlog.backlog}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Not transcoded yet</dt>
						<dd class="text-lg font-semibold tabular-nums">{manager.backlog.eligible}</dd>
					</div>
				</dl>
				{#if skipped.length > 0}
					<div class="flex flex-wrap gap-2" aria-label="Skipped files">
						{#each skipped as reason (reason)}
							<Badge variant="outline" class="tabular-nums">
								{SKIP_LABELS[reason]} · {manager.backlog.skipped[reason]}
							</Badge>
						{/each}
					</div>
				{/if}
				<div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
					<p class="text-xs text-muted-foreground">
						Re-scan checks every file again against the policy above. Saving a change does this on
						its own.
					</p>
					<Button
						variant="outline"
						class="self-start sm:self-center"
						disabled={manager.sweeping}
						onclick={() => manager.rescan()}
					>
						<RefreshCw />
						Re-scan library
					</Button>
				</div>
			</Card.Content>
		</Card.Root>
	{/if}
</div>

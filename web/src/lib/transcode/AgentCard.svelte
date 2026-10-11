<script lang="ts">
	import Check from '@lucide/svelte/icons/check';
	import CirclePause from '@lucide/svelte/icons/circle-pause';
	import CirclePlay from '@lucide/svelte/icons/circle-play';
	import Minus from '@lucide/svelte/icons/minus';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import X from '@lucide/svelte/icons/x';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import * as Select from '$lib/components/ui/select';
	import type { AgentResponse } from '$lib/sdk';
	import { cn } from '$lib/utils';
	import {
		CARD_CAPABILITIES,
		CapabilityStatus,
		formatBytes,
		formatLinkSpeed,
		slotRange,
		summarizeCapability,
		TRANSFER_MODE_LABELS,
		TransferMode
	} from './agents';
	import type { AgentsManager } from './agents-manager.svelte';

	let { agent, manager }: { agent: AgentResponse; manager: AgentsManager } = $props();

	const DEFAULT_SLOTS = 'default';

	const busy = $derived(manager.busy.has(agent.id));
	const slots = $derived(agent.devices.length > 0 ? `${agent.devices[0].slots} per GPU` : '—');
	const slotsValue = $derived(
		agent.slotsOverride === null ? DEFAULT_SLOTS : String(agent.slotsOverride)
	);
	const slotsLabel = $derived(
		agent.slotsOverride === null ? 'Default slots' : `${agent.slotsOverride} per GPU`
	);
	const lastSeen = $derived(new Date(agent.lastSeenAt).toLocaleString());

	function onSlots(value: string) {
		void manager.setSlots(agent, value === DEFAULT_SLOTS ? null : Number(value));
	}
</script>

<Card.Root class="gap-4">
	<Card.Header>
		<Card.Title class="flex min-w-0 items-center gap-2">
			<span
				class={cn(
					'size-2.5 shrink-0 rounded-full',
					agent.online ? 'bg-emerald-500' : 'bg-muted-foreground/50'
				)}
				aria-label={agent.online ? 'Online' : 'Offline'}
				role="img"
			></span>
			<span class="truncate">{agent.hostname}</span>
		</Card.Title>
		<Card.Description class="truncate">
			{#if agent.online}
				Agent {agent.version} · ffmpeg {agent.ffmpegVersion || 'unknown'}
			{:else}
				Offline · last seen {lastSeen}
			{/if}
		</Card.Description>
		<Card.Action class="flex gap-1">
			{#if agent.draining}
				<Badge variant="secondary">Draining</Badge>
			{/if}
		</Card.Action>
	</Card.Header>

	<Card.Content class="flex flex-col gap-4">
		{#if agent.devices.length === 0}
			<p class="text-sm text-muted-foreground">
				No GPU found. Check the device flags on the container.
			</p>
		{:else}
			<ul class="flex flex-col gap-1 text-sm">
				{#each agent.devices as device (device.id)}
					<li class="flex min-w-0 flex-wrap items-center gap-x-2">
						<span class="truncate font-medium">{device.name}</span>
						<span class="truncate text-muted-foreground">{device.driver}</span>
						{#if !device.healthy}
							<Badge variant="destructive">Unhealthy</Badge>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}

		<ul class="flex flex-wrap gap-x-4 gap-y-1 text-sm" aria-label="Tested capabilities">
			{#each CARD_CAPABILITIES as { capability, label } (capability)}
				{@const summary = summarizeCapability(agent, capability)}
				<li class="flex items-center gap-1" title={summary.errors.join('\n') || undefined}>
					{#if summary.status === CapabilityStatus.PASSED}
						<Check class="size-4 text-emerald-500" aria-label="passed" />
					{:else if summary.status === CapabilityStatus.FAILED}
						<X class="size-4 text-destructive" aria-label="failed" />
					{:else}
						<Minus class="size-4 text-muted-foreground" aria-label="not tested" />
					{/if}
					{label}
				</li>
			{/each}
		</ul>

		<dl class="grid grid-cols-2 gap-x-4 gap-y-2 text-sm sm:grid-cols-4">
			<div>
				<dt class="text-muted-foreground">Slots</dt>
				<dd>{slots}</dd>
			</div>
			<div>
				<dt class="text-muted-foreground">Scratch free</dt>
				<dd>{formatBytes(agent.scratchFreeBytes)}</dd>
			</div>
			<div>
				<dt class="text-muted-foreground">Link speed</dt>
				<dd>{formatLinkSpeed(agent.throughputBps)}</dd>
			</div>
			<div>
				<dt class="text-muted-foreground">Saved</dt>
				<dd>{formatBytes(agent.lifetimeSavedBytes)}</dd>
			</div>
		</dl>
	</Card.Content>

	<Card.Footer class="flex flex-wrap items-center gap-2">
		<Select.Root type="single" value={slotsValue} onValueChange={onSlots} disabled={busy}>
			<Select.Trigger size="sm" aria-label="Slots per GPU" class="w-32">{slotsLabel}</Select.Trigger
			>
			<Select.Content>
				<Select.Item value={DEFAULT_SLOTS} label="Default slots" />
				{#each slotRange() as slots (slots)}
					<Select.Item value={String(slots)} label={`${slots} per GPU`} />
				{/each}
			</Select.Content>
		</Select.Root>

		<Select.Root
			type="single"
			value={agent.mode}
			onValueChange={(mode) => void manager.setMode(agent, mode as TransferMode)}
			disabled={busy}
		>
			<Select.Trigger size="sm" aria-label="Transfer mode" class="w-36">
				{TRANSFER_MODE_LABELS[agent.mode as TransferMode] ?? agent.mode}
			</Select.Trigger>
			<Select.Content>
				{#each Object.values(TransferMode) as mode (mode)}
					<Select.Item value={mode} label={TRANSFER_MODE_LABELS[mode]} />
				{/each}
			</Select.Content>
		</Select.Root>

		<div class="ml-auto flex gap-2">
			<Button
				variant="outline"
				size="sm"
				disabled={busy || !agent.online}
				onclick={() => manager.retest(agent)}
			>
				<RefreshCw />
				Re-test
			</Button>
			<Button
				variant="outline"
				size="sm"
				disabled={busy}
				onclick={() => manager.setDraining(agent, !agent.draining)}
			>
				{#if agent.draining}
					<CirclePlay />
					Resume
				{:else}
					<CirclePause />
					Drain
				{/if}
			</Button>
		</div>
	</Card.Footer>
</Card.Root>

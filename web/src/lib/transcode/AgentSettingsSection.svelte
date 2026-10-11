<script lang="ts">
	import { onMount } from 'svelte';
	import Copy from '@lucide/svelte/icons/copy';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { AgentSettingsManager } from './agent-settings-manager.svelte';
	import { DEFAULT_SLOTS_PER_GPU, TRANSFER_CAP } from './agents';
	import { LoadState } from './agents-manager.svelte';

	const manager = new AgentSettingsManager(location.origin);

	onMount(() => {
		void manager.load();
	});

	function submit(event: SubmitEvent) {
		event.preventDefault();
		void manager.save();
	}
</script>

<div class="flex flex-col gap-4">
	{#if manager.loadState === LoadState.LOADING}
		<Skeleton class="h-40 rounded-xl" />
		<Skeleton class="h-48 rounded-xl" />
	{:else if manager.loadState === LoadState.ERROR}
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
				<Card.Title>Shared secret</Card.Title>
				<Card.Description>
					Every agent presents this secret. It comes from <code class="font-mono"
						>MAGPIE_AGENT_SECRET</code
					>; to rotate it, change the variable and restart the server.
				</Card.Description>
			</Card.Header>
			<Card.Content>
				{#if manager.secret.configured}
					<div class="flex items-center gap-2">
						<code class="min-w-0 flex-1 truncate rounded-md bg-muted px-3 py-2 font-mono text-sm">
							{manager.secret.masked}
						</code>
						<Button variant="outline" size="sm" onclick={() => manager.copySecret()}>
							<Copy />
							Copy
						</Button>
					</div>
				{:else}
					<p class="flex items-center gap-2 text-sm text-destructive">
						<TriangleAlert class="size-4 shrink-0" />
						MAGPIE_AGENT_SECRET is not set, so the server refuses every agent.
					</p>
				{/if}
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header>
				<Card.Title>Add an agent</Card.Title>
				<Card.Description>
					Run this on a Linux machine with a GPU, with the secret filled in. Keep <code
						class="font-mono">--gpus</code
					>
					for NVIDIA and <code class="font-mono">--device /dev/dri</code> for Intel or AMD.
				</Card.Description>
			</Card.Header>
			<Card.Content class="flex flex-col gap-2">
				<pre
					class="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">{manager.dockerRun}</pre>
				<Button
					variant="outline"
					size="sm"
					class="self-start"
					onclick={() => manager.copyDockerRun()}
				>
					<Copy />
					Copy command
				</Button>
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header>
				<Card.Title>Limits</Card.Title>
				<Card.Description
					>Per-agent slots and transfer mode are set on Activity → Agents.</Card.Description
				>
			</Card.Header>
			<Card.Content>
				<form class="flex flex-col gap-4" onsubmit={submit}>
					<label class="grid gap-1.5 sm:grid-cols-[1fr_8rem] sm:items-center">
						<span class="text-sm">
							Concurrent transfers
							<span class="block text-xs text-muted-foreground">
								Server-wide source downloads plus uploads ({TRANSFER_CAP.MIN}–{TRANSFER_CAP.MAX})
							</span>
						</span>
						<Input
							type="number"
							min={TRANSFER_CAP.MIN}
							max={TRANSFER_CAP.MAX}
							bind:value={manager.transferCap}
							aria-invalid={!manager.transferCapValid}
						/>
					</label>
					<label class="grid gap-1.5 sm:grid-cols-[1fr_8rem] sm:items-center">
						<span class="text-sm">
							Default jobs per GPU
							<span class="block text-xs text-muted-foreground">
								For agents without their own slots ({DEFAULT_SLOTS_PER_GPU.MIN}–{DEFAULT_SLOTS_PER_GPU.MAX})
							</span>
						</span>
						<Input
							type="number"
							min={DEFAULT_SLOTS_PER_GPU.MIN}
							max={DEFAULT_SLOTS_PER_GPU.MAX}
							bind:value={manager.defaultSlotsPerGpu}
							aria-invalid={!manager.defaultSlotsValid}
						/>
					</label>
					<Button type="submit" class="self-end" disabled={!manager.canSave}>Save</Button>
				</form>
			</Card.Content>
		</Card.Root>
	{/if}
</div>

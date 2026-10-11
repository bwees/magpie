<script lang="ts">
	import Check from '@lucide/svelte/icons/check';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { PRESETS } from './quality';
	import type { QualitySettingsManager } from './quality-manager.svelte';

	let { manager }: { manager: QualitySettingsManager } = $props();
</script>

<div class="grid gap-3 sm:grid-cols-3">
	{#each PRESETS as spec (spec.preset)}
		{@const profile = manager.presetProfile(spec.preset)}
		{@const isDefault = Boolean(profile) && profile?.id === manager.defaultProfileId}
		<Card.Root class={isDefault ? 'border-primary' : ''}>
			<Card.Header>
				<Card.Title class="flex items-center gap-2 text-lg">
					{spec.label}
					{#if isDefault}
						<Badge class="gap-1"><Check class="size-3" />Default</Badge>
					{/if}
				</Card.Title>
				<Card.Description>{spec.description}</Card.Description>
			</Card.Header>
			<Card.Content class="text-xs text-muted-foreground">
				Every source at {spec.label}: WEB-DL, WEBRip, HDTV, Bluray and Remux. The first release at
				this resolution meets the cutoff.
			</Card.Content>
			<Card.Footer>
				{#if !profile}
					<Button
						size="sm"
						variant="outline"
						disabled={Boolean(manager.creatingPreset)}
						onclick={() => manager.createPreset(spec.preset)}
					>
						{manager.creatingPreset === spec.preset ? 'Creating…' : 'Create profile'}
					</Button>
				{:else if !isDefault}
					<Button
						size="sm"
						variant="outline"
						disabled={manager.savingSettings}
						onclick={() => manager.makeDefault(profile.id)}
					>
						Make default
					</Button>
				{:else}
					<span class="text-xs text-muted-foreground">Used for new requests</span>
				{/if}
			</Card.Footer>
		</Card.Root>
	{/each}
</div>
{#if manager.presetError}
	<p class="text-sm text-destructive" role="alert">{manager.presetError}</p>
{/if}

<script lang="ts">
	import Send from '@lucide/svelte/icons/send';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import { formatWhen } from '$lib/activity/format';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { cn } from '$lib/utils';
	import {
		TestOutcome,
		type NotificationSettings,
		type NotificationUrlCard
	} from './notification-settings.svelte';

	let { card, settings }: { card: NotificationUrlCard; settings: NotificationSettings } = $props();

	const ENTER = 'Enter';
	const labelId = $derived(`notification-label-${card.url.id}`);
</script>

<Card.Root>
	<Card.Header>
		<Card.Title class="min-w-0">
			<Label for={labelId} class="sr-only">Label</Label>
			<Input
				id={labelId}
				bind:value={card.label}
				disabled={card.busy}
				class="h-8 max-w-64 font-medium"
				onblur={() => settings.saveLabel(card)}
				onkeydown={(event) => {
					if (event.key === ENTER) settings.saveLabel(card);
				}}
			/>
		</Card.Title>
		<Card.Description class="grid gap-1">
			<code class="truncate font-mono text-xs">{card.url.maskedUrl}</code>
			{#if card.url.lastError}
				<span class="text-xs break-words text-destructive"
					>Last send failed: {card.url.lastError}</span
				>
			{:else if card.url.lastSentAt}
				<span class="text-xs">Last sent {formatWhen(card.url.lastSentAt)}</span>
			{/if}
		</Card.Description>
		<Card.Action class="flex gap-1">
			<Button size="sm" variant="outline" disabled={card.busy} onclick={() => settings.test(card)}>
				<Send />
				{card.testing ? 'Sending…' : 'Send test'}
			</Button>
			<Button
				size="icon"
				variant="ghost"
				class="size-8"
				aria-label="Delete {card.url.label}"
				disabled={card.busy}
				onclick={() => settings.remove(card)}
			>
				<Trash2 />
			</Button>
		</Card.Action>
	</Card.Header>
	<Card.Content class="grid gap-4">
		{#if card.testOutcome}
			<p
				role="status"
				class={cn(
					'text-sm break-words',
					card.testOutcome === TestOutcome.FAILED ? 'text-destructive' : 'text-muted-foreground'
				)}
			>
				{card.testMessage}
			</p>
		{/if}
		{#each settings.groups as group (group.label)}
			<fieldset class="grid gap-2">
				<legend class="mb-2 text-xs font-medium text-muted-foreground">{group.label}</legend>
				<div class="grid gap-2 sm:grid-cols-2">
					{#each group.kinds as kind (kind.kind)}
						{@const id = `${card.url.id}-${kind.kind}`}
						<div class="flex items-center gap-2">
							<Checkbox
								{id}
								checked={card.subscribes(kind.kind)}
								disabled={card.saving}
								onCheckedChange={(checked) => settings.setKind(card, kind.kind, checked)}
							/>
							<Label for={id} class="text-sm font-normal">{kind.label}</Label>
						</div>
					{/each}
				</div>
			</fieldset>
		{/each}
		{#if card.error}
			<p class="text-sm break-words text-destructive" role="alert">{card.error}</p>
		{/if}
	</Card.Content>
</Card.Root>

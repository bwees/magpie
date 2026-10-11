<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import ConnectionFields from './ConnectionFields.svelte';
	import type { ConnectionsState } from './connections-state.svelte';
	import HealthBadge from './HealthBadge.svelte';
	import { HealthStatus, type Service } from './services';

	let { connections, service }: { connections: ConnectionsState; service: Service } = $props();

	const form = $derived(connections.forms[service]);
	const health = $derived(connections.health[service]);
</script>

<Card.Root>
	<Card.Header>
		<Card.Title class="flex flex-wrap items-center gap-2">
			{form.spec.label}
			<HealthBadge {health} />
		</Card.Title>
		<Card.Description>{form.spec.description}</Card.Description>
		<Card.Action>
			<Button
				size="sm"
				variant="outline"
				disabled={form.busy}
				onclick={() => connections.test(service)}
			>
				{form.testing ? 'Testing…' : 'Test'}
			</Button>
		</Card.Action>
	</Card.Header>
	<Card.Content class="grid gap-3">
		<ConnectionFields {form} disabled={form.busy} />
		{#if health.status === HealthStatus.UNHEALTHY && health.error}
			<p class="text-sm break-words text-destructive">{health.error}</p>
		{/if}
		{#if form.dirty}
			<div class="flex justify-end gap-2">
				<Button size="sm" variant="ghost" disabled={form.busy} onclick={() => form.discard()}>
					Cancel
				</Button>
				<Button size="sm" disabled={form.busy} onclick={() => connections.save(service)}>
					{form.saving ? 'Saving…' : 'Save'}
				</Button>
			</div>
		{/if}
	</Card.Content>
</Card.Root>

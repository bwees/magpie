<script lang="ts">
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import CircleDashed from '@lucide/svelte/icons/circle-dashed';
	import CircleX from '@lucide/svelte/icons/circle-x';
	import CircleHelp from '@lucide/svelte/icons/circle-help';
	import { Badge } from '$lib/components/ui/badge';
	import type { ServiceHealthResponse } from '$lib/sdk';
	import { HealthStatus } from './services';

	let { health }: { health: ServiceHealthResponse } = $props();

	const LABELS: Record<HealthStatus, string> = {
		[HealthStatus.HEALTHY]: 'Connected',
		[HealthStatus.UNHEALTHY]: 'Unreachable',
		[HealthStatus.NOT_CONFIGURED]: 'Not set up',
		[HealthStatus.UNKNOWN]: 'Not checked'
	};

	const status = $derived(health.status as HealthStatus);
</script>

{#if status === HealthStatus.HEALTHY}
	<Badge class="gap-1 border-primary/40 bg-primary/15 text-primary" variant="outline">
		<CircleCheck class="size-3" />{LABELS[status]}
	</Badge>
{:else if status === HealthStatus.UNHEALTHY}
	<Badge class="gap-1" variant="destructive" title={health.error}>
		<CircleX class="size-3" />{LABELS[status]}
	</Badge>
{:else if status === HealthStatus.NOT_CONFIGURED}
	<Badge class="gap-1" variant="secondary">
		<CircleDashed class="size-3" />{LABELS[status]}
	</Badge>
{:else}
	<Badge class="gap-1" variant="outline">
		<CircleHelp class="size-3" />{LABELS[HealthStatus.UNKNOWN]}
	</Badge>
{/if}

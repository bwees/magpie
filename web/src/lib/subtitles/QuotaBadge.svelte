<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { formatWhen } from '$lib/activity/format';
	import type { QuotaResponse } from '$lib/sdk';
	import { quotaLabel } from './subtitles';

	let { quota }: { quota: QuotaResponse } = $props();

	const label = $derived(quotaLabel(quota));
	const empty = $derived(quota.remaining === 0);
</script>

{#if quota.problem}
	<Badge variant="destructive" title={quota.problem}>Paused</Badge>
{:else if label}
	<Badge
		variant="outline"
		class={empty ? 'border-amber-500/50 text-amber-500' : 'border-primary/50 text-primary'}
		title={quota.resetAt ? `Resets ${formatWhen(quota.resetAt)}` : undefined}
	>
		{label}
	</Badge>
{:else}
	<Badge variant="secondary">Quota unknown</Badge>
{/if}

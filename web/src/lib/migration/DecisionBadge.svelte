<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';

	/** A settled item's decision, in words; nothing for an open one. */
	let { label }: { label?: string } = $props();
</script>

{#if label}
	<Badge variant="secondary" class="shrink-0">{label}</Badge>
{/if}

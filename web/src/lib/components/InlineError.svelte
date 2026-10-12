<script lang="ts">
	import { Button } from '$lib/components/ui/button';

	/** label names what failed when the page shows several sources. */
	let { label, message, onRetry }: { label?: string; message: string; onRetry: () => void } =
		$props();
</script>

<div
	class="flex flex-wrap items-center gap-3 rounded-lg border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm"
	role="alert"
>
	<span class="text-destructive">
		{#if label}<span class="font-medium">{label}:</span>{/if}
		{message}
	</span>
	<Button size="xs" variant="outline" onclick={onRetry}>Retry</Button>
</div>

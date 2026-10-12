<script lang="ts">
	import { formatPercent } from './downloads';

	let { progress }: { progress: number } = $props();
</script>

<div class="flex items-center gap-2">
	<div
		class="h-1.5 min-w-16 flex-1 overflow-hidden rounded-full bg-muted"
		role="progressbar"
		aria-valuemin={0}
		aria-valuemax={1}
		aria-valuenow={progress}
	>
		<div
			class="h-full rounded-full bg-primary transition-[width]"
			style:width={formatPercent(progress)}
		></div>
	</div>
	<span class="w-9 text-right text-xs tabular-nums text-muted-foreground"
		>{formatPercent(progress)}</span
	>
</div>

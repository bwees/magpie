<script lang="ts">
	import ThinProgress from '$lib/components/ThinProgress.svelte';
	import type { MigrationProgress } from '$lib/sdk';
	import { progressPercent, STEP_LABELS } from './migration';

	let { progress, label }: { progress: MigrationProgress; label: string } = $props();

	const percent = $derived(progressPercent(progress.done, progress.total));
</script>

<div class="grid gap-2 rounded-lg border p-3" aria-live="polite">
	<div class="flex flex-wrap items-baseline justify-between gap-2 text-sm">
		<span class="font-medium">{label}</span>
		<span class="text-muted-foreground tabular-nums">
			{STEP_LABELS[progress.step] ?? 'Starting'}{#if progress.total > 0}
				· {progress.done} of {progress.total}{/if}
		</span>
	</div>
	<ThinProgress class="h-1.5" {percent} />
</div>

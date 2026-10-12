<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { STATE_LABELS, isQuiet } from './downloads';

	let { state }: { state: string } = $props();
</script>

<Badge
	variant={isQuiet(state) ? 'secondary' : 'outline'}
	class={isQuiet(state) ? '' : 'border-primary/50 text-primary'}
>
	{STATE_LABELS[state] ?? state}
</Badge>

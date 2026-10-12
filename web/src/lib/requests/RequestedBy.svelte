<script lang="ts">
	import { TitleRequester } from './title-requester.svelte';

	let { titleId, class: className = '' }: { titleId: string; class?: string } = $props();

	const requester = new TitleRequester();

	$effect(() => {
		void requester.load(titleId);
	});
</script>

<span class={className}>{requester.name ? `Requested by ${requester.name}` : 'Requested'}</span>

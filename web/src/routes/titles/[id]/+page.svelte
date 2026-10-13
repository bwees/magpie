<script lang="ts">
	import { page } from '$app/state';
	import TitleView from '$lib/catalog/TitleView.svelte';
</script>

{#key page.params.id}
	<TitleView id={page.params.id ?? ''} />
{/key}

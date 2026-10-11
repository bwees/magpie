<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import type { ConnectionForm } from './connections-state.svelte';

	let { form, disabled = false }: { form: ConnectionForm; disabled?: boolean } = $props();
</script>

<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
	{#each form.spec.fields as field (field.key)}
		{@const id = `${form.spec.path}-${field.key}`}
		<div class="grid min-w-0 gap-1.5">
			<Label for={id}>{field.label}</Label>
			<Input
				{id}
				type={field.secret ? 'password' : 'text'}
				autocomplete="off"
				placeholder={field.placeholder}
				bind:value={form.values[field.key]}
				{disabled}
			/>
		</div>
	{/each}
</div>

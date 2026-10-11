<script lang="ts">
	import { formatWhen } from '$lib/activity/format';
	import { Badge } from '$lib/components/ui/badge';
	import { SOURCE_LABELS, UserSource, type UserRow } from './users-manager.svelte';

	let { row }: { row: UserRow } = $props();
</script>

<div class="min-w-0 flex-1">
	<div class="flex flex-wrap items-center gap-1.5">
		<span class="truncate font-medium">{row.user.displayName}</span>
		{#if row.isSelf}
			<Badge variant="secondary">You</Badge>
		{/if}
		{#if row.user.disabled}
			<Badge variant="destructive">Disabled</Badge>
		{/if}
	</div>
	<div class="truncate text-xs text-muted-foreground">
		{row.user.username} · {SOURCE_LABELS[row.user.source as UserSource] ?? row.user.source}
	</div>
	<div class="text-xs text-muted-foreground">
		{row.user.lastLoginAt ? `Signed in ${formatWhen(row.user.lastLoginAt)}` : 'Never signed in'}
	</div>
</div>

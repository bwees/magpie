<script lang="ts">
	import NotificationSettings from '$lib/notifications/NotificationSettings.svelte';
	import { getSession } from '$lib/session/session.svelte';

	const session = getSession();
</script>

<NotificationSettings admin={session.isAdmin} />

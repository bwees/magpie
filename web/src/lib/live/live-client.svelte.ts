import { createContext } from 'svelte';
import { LiveConnection, LiveStatus } from './live-connection';

export { LiveStatus } from './live-connection';

/** The live connection with a reactive status for the shell. */
export class LiveClient extends LiveConnection {
	#status = $state(LiveStatus.CONNECTING);

	override get status(): LiveStatus {
		return this.#status;
	}

	protected override set status(status: LiveStatus) {
		this.#status = status;
	}
}

export const [getLive, setLive] = createContext<LiveClient>();

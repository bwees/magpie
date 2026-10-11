import { SvelteSet } from 'svelte/reactivity';
import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	listAgents,
	retestAgent,
	updateAgent,
	type AgentResponse,
	type UpdateAgentRequest
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import { AgentEvent, TransferMode } from './agents';

export enum LoadState {
	LOADING = 'LOADING',
	READY = 'READY',
	ERROR = 'ERROR'
}

/** Scratch space and retest results change without an event, so the list also refreshes on this beat. */
export const REFRESH_INTERVAL_MS = 10_000;

export interface AgentsApi {
	list: () => Promise<AgentResponse[]>;
	update: (id: string, request: UpdateAgentRequest) => Promise<AgentResponse>;
	retest: (id: string) => Promise<unknown>;
}

export interface Notifier {
	success: (message: string) => void;
	error: (message: string) => void;
}

const sdkApi: AgentsApi = {
	list: () => listAgents(),
	update: (id, request) => updateAgent(id, request),
	retest: (id) => retestAgent(id)
};

/** State and actions behind Activity → Agents. */
export class AgentsManager {
	agents = $state<AgentResponse[]>([]);
	loadState = $state(LoadState.LOADING);
	error = $state('');
	/** Agents with an action in flight, so their controls disable. */
	readonly busy = new SvelteSet<string>();

	readonly #api: AgentsApi;
	readonly #notify: Notifier;

	constructor(api: AgentsApi = sdkApi, notify: Notifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	async load(): Promise<void> {
		try {
			this.agents = await this.#api.list();
			this.loadState = LoadState.READY;
			this.error = '';
		} catch (error) {
			if (this.loadState !== LoadState.READY) this.loadState = LoadState.ERROR;
			this.error = apiErrorMessage(error, 'Could not load agents.');
		}
	}

	/** Reloads on agent events, after a missed replay and on the refresh beat. Returns a cleanup. */
	watch(live: LiveConnection): () => void {
		const reload = () => void this.load();
		const stops = [
			live.on(AgentEvent.ONLINE, reload),
			live.on(AgentEvent.OFFLINE, reload),
			live.onReset(reload)
		];
		const timer = setInterval(reload, REFRESH_INTERVAL_MS);
		return () => {
			clearInterval(timer);
			for (const stop of stops) stop();
		};
	}

	retest(agent: AgentResponse): Promise<void> {
		return this.#act(agent, async () => {
			await this.#api.retest(agent.id);
			this.#notify.success(`Re-testing ${agent.hostname}. Results appear when the tests finish.`);
		});
	}

	setDraining(agent: AgentResponse, draining: boolean): Promise<void> {
		return this.#update(agent, { draining });
	}

	setSlots(agent: AgentResponse, slotsOverride: number | null): Promise<void> {
		return this.#update(agent, { slotsOverride });
	}

	setMode(agent: AgentResponse, mode: TransferMode): Promise<void> {
		return this.#update(agent, { mode });
	}

	/** The server stores all three overrides together, so every change sends the current set. */
	#update(agent: AgentResponse, change: Partial<UpdateAgentRequest>): Promise<void> {
		const request: UpdateAgentRequest = {
			slotsOverride: agent.slotsOverride,
			mode: agent.mode as TransferMode,
			draining: agent.draining,
			...change
		};
		return this.#act(agent, async () => {
			const updated = await this.#api.update(agent.id, request);
			this.agents = this.agents.map((a) => (a.id === updated.id ? updated : a));
		});
	}

	async #act(agent: AgentResponse, action: () => Promise<void>): Promise<void> {
		this.busy.add(agent.id);
		try {
			await action();
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, `Could not update ${agent.hostname}.`));
		} finally {
			this.busy.delete(agent.id);
		}
	}
}

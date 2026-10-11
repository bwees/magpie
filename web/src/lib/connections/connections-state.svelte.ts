import { toast } from 'svelte-sonner';
import type { LiveConnection } from '$lib/live/live-connection';
import { LoadStatus } from '$lib/load-status';
import {
	apiErrorMessage,
	getConnections,
	testConnection,
	updateConnections,
	type ConnectionsResponse,
	type ServiceHealthChanged,
	type ServiceHealthResponse
} from '$lib/sdk';
import {
	HealthStatus,
	SERVICE_HEALTH_CHANGED,
	SERVICE_ORDER,
	SERVICES,
	storedValues,
	testSummary,
	unknownHealth,
	updateFor,
	type FieldValues,
	type Service,
	type ServiceSpec
} from './services';

/** One service's editable fields next to what the server has stored. */
export class ConnectionForm {
	readonly spec: ServiceSpec;
	values = $state<FieldValues>({});
	saved = $state<FieldValues>({});
	saving = $state(false);
	testing = $state(false);

	constructor(spec: ServiceSpec) {
		this.spec = spec;
	}

	get dirty(): boolean {
		return this.spec.fields.some(
			(field) => (this.values[field.key] ?? '') !== (this.saved[field.key] ?? '')
		);
	}

	get busy(): boolean {
		return this.saving || this.testing;
	}

	load(values: FieldValues): void {
		this.values = { ...values };
		this.saved = { ...values };
	}

	discard(): void {
		this.values = { ...this.saved };
	}
}

function formsFor(): Record<Service, ConnectionForm> {
	return Object.fromEntries(
		SERVICE_ORDER.map((service) => [service, new ConnectionForm(SERVICES[service])])
	) as Record<Service, ConnectionForm>;
}

function healthFor(): Record<Service, ServiceHealthResponse> {
	return Object.fromEntries(
		SERVICE_ORDER.map((service) => [service, unknownHealth(service)])
	) as Record<Service, ServiceHealthResponse>;
}

/**
 * Every connection's form and health. Saving or testing one service never touches the others' unsaved edits.
 * With a live connection, health badges follow `platform.ServiceHealthChanged`.
 */
export class ConnectionsState {
	status = $state(LoadStatus.LOADING);
	error = $state('');
	readonly forms = formsFor();
	health = $state(healthFor());
	readonly #stopListening: (() => void)[] = [];

	constructor(live?: LiveConnection) {
		if (live) {
			this.#stopListening.push(
				live.on(SERVICE_HEALTH_CHANGED, (message) =>
					this.applyHealthChange(message.data as ServiceHealthChanged)
				),
				live.onReset(() => this.load())
			);
		}
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		try {
			const connections = await getConnections();
			for (const service of SERVICE_ORDER) {
				this.forms[service].load(storedValues(SERVICES[service], connections));
			}
			this.#applyHealth(connections);
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error, 'Could not load connections.');
			this.status = LoadStatus.ERROR;
		}
	}

	/** Stores the service's fields. Returns false and toasts when the server rejects them. */
	async save(service: Service): Promise<boolean> {
		const form = this.forms[service];
		form.saving = true;
		try {
			const connections = await updateConnections(updateFor(form.spec, form.values));
			form.load(storedValues(form.spec, connections));
			this.#applyHealth(connections);
			return true;
		} catch (error) {
			toast.error(`${form.spec.label} not saved`, { description: apiErrorMessage(error) });
			return false;
		} finally {
			form.saving = false;
		}
	}

	/** Saves unsaved edits, probes the service and toasts the outcome with the detected version. */
	async test(service: Service): Promise<ServiceHealthResponse | undefined> {
		const form = this.forms[service];
		if (form.dirty && !(await this.save(service))) return undefined;
		form.testing = true;
		try {
			const result = await testConnection(form.spec.path);
			this.health[service] = result;
			this.#toastResult(form.spec, result);
			return result;
		} catch (error) {
			toast.error(`${form.spec.label} test failed`, { description: apiErrorMessage(error) });
			return undefined;
		} finally {
			form.testing = false;
		}
	}

	applyHealthChange(change: ServiceHealthChanged): void {
		const service = change.service as Service;
		const current = this.health[service];
		if (!current) return;
		this.health[service] = {
			...current,
			status: change.healthy ? HealthStatus.HEALTHY : HealthStatus.UNHEALTHY,
			error: change.error
		};
	}

	destroy(): void {
		for (const stop of this.#stopListening) stop();
	}

	#applyHealth(connections: ConnectionsResponse): void {
		for (const health of connections.health) {
			this.health[health.service as Service] = health;
		}
	}

	#toastResult(spec: ServiceSpec, result: ServiceHealthResponse): void {
		switch (result.status) {
			case HealthStatus.HEALTHY:
				toast.success(`${spec.label} connected`, { description: testSummary(result) });
				return;
			case HealthStatus.NOT_CONFIGURED:
				toast.warning(`${spec.label} is not set up`, { description: 'Fill in every field first.' });
				return;
			default:
				toast.error(`${spec.label} unreachable`, { description: result.error });
		}
	}
}

import type { LiveEvents } from '$lib/activity/activity-source';
import type { LiveHandler, ResetListener } from './live-connection';
import type { LiveMessage } from './protocol';

/** An in-memory live socket for manager tests: emit hands a message to the handlers of its type. */
export class FakeLive implements LiveEvents {
	readonly handlers = new Map<string, Set<LiveHandler>>();
	readonly resets = new Set<ResetListener>();

	on(type: string, handler: LiveHandler): () => void {
		const handlers = this.handlers.get(type) ?? new Set<LiveHandler>();
		handlers.add(handler);
		this.handlers.set(type, handlers);
		return () => handlers.delete(handler);
	}

	onReset(listener: ResetListener): () => void {
		this.resets.add(listener);
		return () => this.resets.delete(listener);
	}

	emit(type: string, data: unknown): void {
		const message: LiveMessage = { seq: 1, epoch: 'E', type, topic: '', data };
		this.handlers.get(type)?.forEach((handler) => handler(message));
	}

	reset(): void {
		this.resets.forEach((listener) => listener());
	}

	get listeners(): number {
		return [...this.handlers.values()].reduce((sum, set) => sum + set.size, 0) + this.resets.size;
	}
}

/** Lets pending promise callbacks run. */
export function settle(): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, 0));
}

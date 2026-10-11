export const INITIAL_RECONNECT_DELAY_MS = 1_000;
export const MAX_RECONNECT_DELAY_MS = 30_000;
const BACKOFF_FACTOR = 2;
const JITTER_SHARE = 0.5;

export function nextReconnectDelay(current: number): number {
	return Math.min(current * BACKOFF_FACTOR, MAX_RECONNECT_DELAY_MS);
}

/** Waits between half and all of delay, so tabs that lost the server together do not reconnect together. */
export function withJitter(delay: number, random: () => number): number {
	return delay * (1 - JITTER_SHARE) + delay * JITTER_SHARE * random();
}

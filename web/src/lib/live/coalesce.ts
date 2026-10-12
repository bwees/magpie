/**
 * Wraps a refetch so a burst of live events runs it at most twice: once now, and once more after it
 * finishes if anything asked again meanwhile.
 */
export function coalesce(run: () => Promise<void>): () => Promise<void> {
	let running: Promise<void> | undefined;
	let again = false;

	async function loop(): Promise<void> {
		do {
			again = false;
			await run();
		} while (again);
	}

	return () => {
		if (running) {
			again = true;
			return running;
		}
		running = loop().finally(() => {
			running = undefined;
		});
		return running;
	};
}

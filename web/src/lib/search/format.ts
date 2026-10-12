const BYTES_PER_MB = 1024 ** 2;
const BYTES_PER_GB = 1024 ** 3;

const GIGABYTES = new Intl.NumberFormat(undefined, {
	maximumFractionDigits: 1,
	minimumFractionDigits: 1
});
const MEGABYTES = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 });

/** Release sizes as the *arr apps show them: GB with one decimal, MB below a gigabyte. */
export function formatSize(bytes: number): string {
	if (bytes >= BYTES_PER_GB) {
		return `${GIGABYTES.format(bytes / BYTES_PER_GB)} GB`;
	}
	return `${MEGABYTES.format(bytes / BYTES_PER_MB)} MB`;
}

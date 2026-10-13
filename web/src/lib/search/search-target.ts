/** What a search looks for: a whole title, or some of its episodes. */
export interface SearchTarget {
	titleId: string;
	episodeIds?: string[];
	/** Names the target in the toast and the search dialog, e.g. "Severance" or "Severance S01E03". */
	label: string;
}

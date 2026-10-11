/** The five main pipeline stages. Subtitles and transcoding are chips beside the bar, not stages. */
export enum PipelineStage {
	REQUESTED = 'REQUESTED',
	SEARCHING = 'SEARCHING',
	DOWNLOADING = 'DOWNLOADING',
	IMPORTING = 'IMPORTING',
	AVAILABLE = 'AVAILABLE'
}

export const PIPELINE_STAGES: { stage: PipelineStage; label: string }[] = [
	{ stage: PipelineStage.REQUESTED, label: 'Requested' },
	{ stage: PipelineStage.SEARCHING, label: 'Searching' },
	{ stage: PipelineStage.DOWNLOADING, label: 'Downloading' },
	{ stage: PipelineStage.IMPORTING, label: 'Importing' },
	{ stage: PipelineStage.AVAILABLE, label: 'Available' }
];

export enum SegmentState {
	DONE = 'DONE',
	CURRENT = 'CURRENT',
	PENDING = 'PENDING'
}

export const NO_ACTIVITY_LABEL = 'No activity yet';

/** How each segment of the bar draws for the current stage; all pending when the stage is unknown. */
export function segmentStates(current: PipelineStage | undefined): SegmentState[] {
	const currentIndex = PIPELINE_STAGES.findIndex((s) => s.stage === current);
	return PIPELINE_STAGES.map((_, index) => {
		if (currentIndex < 0 || index > currentIndex) {
			return SegmentState.PENDING;
		}
		return index === currentIndex ? SegmentState.CURRENT : SegmentState.DONE;
	});
}

/** The phone label: "<Stage> · step N of 5". */
export function stepLabel(current: PipelineStage | undefined): string {
	const index = PIPELINE_STAGES.findIndex((s) => s.stage === current);
	if (index < 0) {
		return NO_ACTIVITY_LABEL;
	}
	return `${PIPELINE_STAGES[index].label} · step ${index + 1} of ${PIPELINE_STAGES.length}`;
}

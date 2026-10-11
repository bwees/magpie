/** The state of a data block: skeleton, content, or inline error with Retry. */
export enum LoadStatus {
	LOADING = 'LOADING',
	READY = 'READY',
	ERROR = 'ERROR'
}

const SAMPLE_SIZE = 1;
const RGB_CHANNELS = 3;

/** The poster's average colour as "r g b", for the hero gradient. Undefined when it can't be read. */
export function posterTint(url: string): Promise<string | undefined> {
	return new Promise((resolve) => {
		const img = new Image();
		img.onload = () => {
			const canvas = document.createElement('canvas');
			canvas.width = SAMPLE_SIZE;
			canvas.height = SAMPLE_SIZE;
			const context = canvas.getContext('2d');
			if (!context) {
				resolve(undefined);
				return;
			}
			context.drawImage(img, 0, 0, SAMPLE_SIZE, SAMPLE_SIZE);
			const pixel = context.getImageData(0, 0, SAMPLE_SIZE, SAMPLE_SIZE).data;
			resolve(Array.from(pixel.slice(0, RGB_CHANNELS)).join(' '));
		};
		img.onerror = () => resolve(undefined);
		img.src = url;
	});
}

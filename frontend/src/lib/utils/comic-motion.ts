// One decelerating glide; browser smooth scrolling restarts with an ease-in.
export function comicGlide(from: number, to: number, releaseVelocity = 0) {
	const distance = to - from;
	const speed = releaseVelocity * Math.sign(distance);
	const duration = Math.min(420, Math.max(140, 3 * Math.abs(distance) / Math.max(0.8, speed)));
	return {
		duration,
		position(elapsed: number) {
			const progress = Math.min(1, Math.max(0, elapsed / duration));
			return from + distance * (1 - (1 - progress) ** 3);
		}
	};
}

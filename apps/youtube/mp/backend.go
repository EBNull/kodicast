package mp

import (
	"time"
)

type Backend interface {
	initialize() chan State
	quit()
	play(string, time.Duration, int)
	pause()
	resume()
	getPosition() (time.Duration)
	setPosition(time.Duration)
	setVolume(int)
	stop()
}

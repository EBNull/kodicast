package mp

import (
	"time"
)

// A generic YouTube media player using a playlist.
type MediaPlayer struct {
	player      Backend
	stateChange chan StateChange

	// A channel to coordinate access to the PlayState.
	// The pointer to the PlayState is used as an access token.
	playstateChan chan PlayState
}

func New(stateChange chan StateChange) *MediaPlayer {
	p := MediaPlayer{}
	p.stateChange = stateChange
	p.playstateChan = make(chan PlayState)

	p.player = &Kodi{}
	playerEventChan := p.player.initialize()

	go p.run(playerEventChan, 100)

	return &p
}

// Quit quits the MediaPlayer.
// No other method may be called upon this object after this function has been
// called.
func (p *MediaPlayer) Quit() {
	p.getPlayState(func(ps *PlayState) {
		p.player.quit()
	})
}

func (p *MediaPlayer) getPosition(ps *PlayState) time.Duration {
	var position time.Duration

	switch ps.State {
	case STATE_STOPPED:
		position = 0
	case STATE_BUFFERING:
		position = ps.bufferingPosition
	case STATE_PLAYING, STATE_PAUSED:
		position = p.player.getPosition()
	default:
		panic("unknown state")
	}

	if position < 0 {
		panic("got position < 0")
	}

	return position
}

// getPlayState gets the play state for use in a callback.
// The *PlayState argument may only be used until the callback exits to prevent
// race conditions.
func (p *MediaPlayer) getPlayState(callback func(*PlayState)) {
	ps, ok := <-p.playstateChan
	if !ok {
		// The player has already stopped. Ignore all function calls.
		return
	}
	callback(&ps)
	p.playstateChan <- ps
}

// SetPlaystate changes the play state to the specified arguments
// This function doesn't block, but changes may not be immediately applied.
func (p *MediaPlayer) SetPlaystate(playlist []string, index int, position time.Duration, listId string) {
	p.getPlayState(func(ps *PlayState) {
		if ps.State == STATE_BUFFERING && ps.bufferingPosition == position && ps.Index < len(ps.Playlist) && playlist[index] == ps.Playlist[ps.Index] {
			// just in case something else has changed, update the playlist
			p.updatePlaylist(ps, playlist)
			return
		}
		ps.Playlist = playlist
		ps.Index = index
		ps.ListId = listId

		if len(ps.Playlist) > 0 {
			p.startPlaying(ps, position)
		} else {
			p.stop(ps)
		}
	})
}

func (p *MediaPlayer) startPlaying(ps *PlayState, position time.Duration) {
	if ps.State == STATE_PLAYING {
		// Pause the currently playing track.
		// This has multiple benefits:
		//  *  When pressing 'play', the user probably expects the next video to
		//     be played immediately, or if that is not possible, expects the
		//     current video to stop playing.
		//  *  On very slow systems, like the Raspberry Pi, downloading the
		//     stream URL for the next video doesn't interrupt the currently
		//     playing video.
		p.player.pause()
	}
	p.setPlayState(ps, STATE_BUFFERING, position)

	videoId := ps.Playlist[ps.Index]

	go func() {
		// Do not use the playstate inside the goroutine to prevent race conditions.
		// A new goroutine loses rights to the PlayState structure, enforce that
		// rule here.
		ps = nil

		// again acquire PlayState access
		p.getPlayState(func(ps *PlayState) {
			// Check whether another video has been queued to be played already:
			// one may be played while the URL for another is still being
			// fetched.
			if ps.Video() != videoId {
				// stale video
				return
			}

			volume := -1
			if ps.newVolume {
				ps.newVolume = false
				volume = ps.Volume
			}

			p.player.play(videoId, position, volume)
		})
	}()
}

func (p *MediaPlayer) nextVideo(ps *PlayState) {
	if ps.Index+1 < len(ps.Playlist) {
		// there are more videos, play the next
		ps.Index++
		p.startPlaying(ps, 0)
	} else {
		// signal that the video has stopped playing
		// this resets the position but keeps the playlist
		// TODO keep the position at the end, not the beginning
		p.setPlayState(ps, STATE_STOPPED, 0)
	}
}

func (p *MediaPlayer) NextVideo() {
	p.getPlayState(func(ps *PlayState) {
		p.nextVideo(ps)
	})
}

func (p *MediaPlayer) previousVideo(ps *PlayState) {
	if ps.Index-1 >= 0 {
		// there are more videos, play the previous
		ps.Index--
		p.startPlaying(ps, 0)
	} else {
		// signal that the video has stopped playing
		// this resets the position but keeps the playlist
		// TODO keep the position at the end, not the beginning
		p.setPlayState(ps, STATE_STOPPED, 0)
	}
}

func (p *MediaPlayer) PreviousVideo() {
	p.getPlayState(func(ps *PlayState) {
		p.previousVideo(ps)
	})
}

// setPlayState updates the PlayState and sends events.
// position may be -1: in that case it will be updated.
func (p *MediaPlayer) setPlayState(ps *PlayState, state State, position time.Duration) {
	if ps.State == STATE_BUFFERING {
		position = ps.bufferingPosition
	}

	ps.previousState = ps.State
	ps.State = state

	if state == STATE_BUFFERING {
		ps.bufferingPosition = position
	} else {
		ps.bufferingPosition = -1
	}

	if position == -1 {
		position = p.getPosition(ps)
	}

	p.stateChange <- StateChange{state, position}
}

func (p *MediaPlayer) UpdatePlaylist(playlist []string, listId string) {
	p.getPlayState(func(ps *PlayState) {
		ps.ListId = listId
		p.updatePlaylist(ps, playlist)
	})
}

func (p *MediaPlayer) updatePlaylist(ps *PlayState, playlist []string) {
	if len(ps.Playlist) == 0 {

		if ps.State == STATE_PLAYING {
			// just to be sure
			panic("empty playlist while playing")
		}
		ps.Playlist = playlist

		if ps.Index >= len(playlist) {
			// this appears to be the normal behavior of YouTube
			ps.Index = len(playlist) - 1
		}

	} else {
		videoId := ps.Video()
		ps.Playlist = playlist
		p.setPlaylistIndex(ps, videoId, ps.Index)
		if ps.Video() != videoId && ps.State != STATE_STOPPED {
			p.player.stop()
		}
	}
}

func (p *MediaPlayer) SetVideo(videoId string, position time.Duration) {
	p.getPlayState(func(ps *PlayState) {
		p.setPlaylistIndex(ps, videoId, ps.Index)
		p.startPlaying(ps, position)
	})
}

func (p *MediaPlayer) setPlaylistIndex(ps *PlayState, videoId string, backupIndex int) {
	newIndex := -1
	for i, v := range ps.Playlist {
		if v == videoId {
			if newIndex >= 0 {
				logger.Warnln("videoId exists twice in playlist")
				break
			}
			newIndex = i
			// no 'break' so duplicate video entries can be checked
		}
	}

	if newIndex == -1 {
		// This may happen when the current and last video is removed from the
		// playlist.
		newIndex = backupIndex
		if newIndex >= len(ps.Playlist) {
			newIndex = len(ps.Playlist) - 1
		}
	}

	ps.Index = newIndex
}

// RequestPlaylist asynchronously gets the playlist state and sends it over the
// channel.
// To make asynchronous requests work, it expects a 1-buffered channel. Before a
// new PlaylistState is sent over the channel, the previous is read if it's
// there. It ensures that only one goroutine does that at one time, so this
// trick should not be used elsewhere on the same channel.
func (p *MediaPlayer) RequestPlaylist(playlistChan chan PlaylistState) {
	go p.getPlayState(func(ps *PlayState) {
		playlist := make([]string, len(ps.Playlist))
		copy(playlist, ps.Playlist)

		// If there is a value in the (buffered) channel, clear it.
		// Only one goroutine at a time can do this, because they're guarded by
		// getPlayState. This makes sure the request can run in a goroutine
		// while no goroutines are being leaked and values always arrive in
		// order.
		select {
		case <-playlistChan:
		default:
		}
		playlistChan <- PlaylistState{playlist, ps.Index, p.getPosition(ps), ps.State, ps.ListId}
	})
}

// Pause pauses the currently playing video
func (p *MediaPlayer) Pause() {
	p.getPlayState(func(ps *PlayState) {
		if ps.State != STATE_PLAYING {
			// This is a Printf and not a Warnf because this occurs often in
			// practice when seeking and is harmless in that case.
			logger.Printf("pause while in state %d - ignoring\n", ps.State)
		} else {
			p.player.pause()
		}
	})
}

// Play resumes playback when it was paused
func (p *MediaPlayer) Play() {
	p.getPlayState(func(ps *PlayState) {
		if ps.State == STATE_STOPPED {
			// Restart from the beginning.
			if ps.Index >= len(ps.Playlist) {
				logger.Warnln("invalid index or empty playlist")
				return
			}
			p.startPlaying(ps, 0)

		} else {
			if ps.State != STATE_PAUSED {
				logger.Warnf("resume while in state %d - ignoring\n", ps.State)
			} else {
				p.player.resume()
			}
		}
	})
}

// Seek jumps to the specified position
func (p *MediaPlayer) Seek(position time.Duration) {
	p.getPlayState(func(ps *PlayState) {
		if ps.State == STATE_STOPPED {
			p.startPlaying(ps, position)
		} else if ps.State == STATE_PAUSED || ps.State == STATE_PLAYING {
			p.player.setPosition(position)
		} else {
			logger.Warnf("state is not paused or playing while seeking (state: %d) - ignoring\n", ps.State)
		}
	})
}

// SetVolume sets the volume of the player to the specified value (0-100).
func (p *MediaPlayer) SetVolume(volume int, volumeChan chan int) {
	p.getPlayState(func(ps *PlayState) {
		ps.Volume = volume
		p.applyVolume(ps, volumeChan)
	})
}

// ChangeVolume increases or decreases the volume by the specified delta.
func (p *MediaPlayer) ChangeVolume(delta int, volumeChan chan int) {
	p.getPlayState(func(ps *PlayState) {
		ps.Volume += delta
		// pressing 'volume up' or 'volume down' keeps sending volume
		// increase/decrease messages. Keep the volume within range 0-100.
		if ps.Volume < 0 {
			ps.Volume = 0
		}
		if ps.Volume > 100 {
			ps.Volume = 100
		}

		p.applyVolume(ps, volumeChan)
	})
}

func (p *MediaPlayer) applyVolume(ps *PlayState, volumeChan chan int) {
	if ps.State == STATE_PLAYING || ps.State == STATE_PAUSED {
		p.player.setVolume(ps.Volume)
	} else {
		ps.newVolume = true
	}
	volumeChan <- ps.Volume
}

// RequestVolume asynchronously gets the volume and sends it over the channel
// volumeChan. See RequestPlaylist for how this works.
func (p *MediaPlayer) RequestVolume(volumeChan chan int) {
	go p.getPlayState(func(ps *PlayState) {

		select {
		case <-volumeChan:
		default:
		}
		volumeChan <- ps.Volume
	})
}

func (p *MediaPlayer) stop(ps *PlayState) {
	ps.Playlist = []string{}
	// Do not set ps.Index to 0, it may be needed for UpdatePlaylist:
	// Stop is called before UpdatePlaylist when removing the currently
	// playing video from the playlist.
	// TODO this is a race condition: it looks like the player is playing with
	// an empty playlist now.
	p.player.stop()
}

// Stop stops the currently playing sound and clears the playlist.
func (p *MediaPlayer) Stop() {
	p.getPlayState(p.stop)
}

// Function run is the mainloop of the player. It mainly handles state change
// events.
func (p *MediaPlayer) run(playerEventChan chan State, initialVolume int) {
	ps := PlayState{}
	ps.Volume = initialVolume
	ps.nextState = -1

	for {
		select {
		case p.playstateChan <- ps:
			// Synchronize access to the PlayState structure.
			// See the documentation of PlayState.
			ps = <-p.playstateChan

		case event, ok := <-playerEventChan:
			if !ok {
				// player has quit, and closed channel
				close(p.stateChange)
				close(p.playstateChan)
				return
			}

			switch event {
			case STATE_PLAYING:
				if ps.newVolume {
					ps.newVolume = false
					p.player.setVolume(ps.Volume)
				}

				p.setPlayState(&ps, STATE_PLAYING, -1)

			case STATE_PAUSED:
				if ps.State == STATE_BUFFERING {
					// The video has been paused while the stream for the next
					// video is being loaded.
					break
				}

				p.setPlayState(&ps, STATE_PAUSED, -1)

			case STATE_STOPPED:
				// There may be more videos.
				p.nextVideo(&ps)
			}
		}
	}
}

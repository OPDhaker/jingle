// Package player finds the OS audio player and starts it fully detached
// (new session, null stdio, released process) so git never waits on playback.
// macOS: afplay. Linux: pw-play, paplay, aplay, ffplay. Windows: PowerShell.
package player

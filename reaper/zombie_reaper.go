// +build !windows

package reaper

// ReapZombie reap the zombie child process
func ReapZombie() {
	go Reap()
}

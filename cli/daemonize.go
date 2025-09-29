package cli

import (
	"log"
	"os"
	"path/filepath"

	"github.com/qjpcpu/supervisord/sys"
	"github.com/qjpcpu/go-daemon"
)

// Daemonize run this process in daemon mode
func Daemonize(proc func()) {
	path, _ := filepath.Abs(sys.Args()[0])
	dir := filepath.Join(filepath.Dir(path), "../pid")
	os.MkdirAll(dir, 0755)
	context := daemon.Context{PidFileName: filepath.Join(dir, "supervisord.pid")}

	child, err := context.Reborn()
	if err != nil {
		log.Fatal("Unable to run", err)
	}
	if child != nil {
		return
	}
	defer context.Release()
	proc()
}

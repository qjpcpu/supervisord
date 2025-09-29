package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qjpcpu/filelog"
)

var logger DaemonLog = new(stdoutDaemonLog)

func initLogger(name string) {
	if logger != nil {
		logger.Close()
	}
	switch name {
	case "/dev/stdout":
	case "/dev/stderr":
	case "/dev/null":
		logger = nullDaemonLog{}
	case "":
	default:
		os.MkdirAll(filepath.Dir(name), 0755)
		w, err := filelog.NewWriter(name, filelog.Keep(1), filelog.RotateBy(filelog.RotateDaily), filelog.KeepMaxSize(1*filelog.G), filelog.CreateShortcut(true), filelog.DisableWatchFile())
		if err == nil {
			logger = &fileDaemonLog{w: w}
		}
	}
}

type DaemonLog interface {
	Log(f string, v ...interface{})
	Close() error
}

type nullDaemonLog struct{}

func (nullDaemonLog) Log(string, ...interface{})  {}
func (nullDaemonLog) Close() error                { return nil }
func (nullDaemonLog) Write(b []byte) (int, error) { return len(b), nil }

type stdoutDaemonLog struct{}

func (self *stdoutDaemonLog) Log(f string, v ...interface{}) {
	f = fmt.Sprintf("[%s] %s\n", time.Now().Format(`2006-01-02 15:04:05`), f)
	fmt.Printf(f, v...)
}

func (*stdoutDaemonLog) Close() error { return nil }

type fileDaemonLog struct{ w filelog.FileLogWriter }

func (self *fileDaemonLog) Log(f string, v ...interface{}) {
	f = fmt.Sprintf("[%s] %s\n", time.Now().Format(`2006-01-02 15:04:05`), f)
	self.w.Write([]byte(fmt.Sprintf(f, v...)))
}

func (self *fileDaemonLog) Close() error {
	return self.w.Close()
}

type loggerToWriter struct{}

func (w *loggerToWriter) Write(b []byte) (int, error) {
	logger.Log(string(b))
	return len(b), nil
}

func (w *loggerToWriter) Close() error {
	return logger.Close()
}

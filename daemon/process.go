package daemon

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/qjpcpu/supervisord/signals"

	"github.com/qjpcpu/filelog"

	"os/exec"

	chans "github.com/qjpcpu/channel"
	"github.com/qjpcpu/fp"

	"github.com/qjpcpu/supervisord/config"
)

// State the state of process
type State int

const (
	WaitSchedule State = iota
	Starting
	Running
	Stopped
)

func (s State) String() string {
	switch s {
	case WaitSchedule:
		return "WaitSchedule"
	case Starting:
		return "Starting"
	case Stopped:
		return "Stopped"
	case Running:
		return "Running"
	default:
		return ""
	}
}

type ProcessExitedCb func(byuser bool)

type Process struct {
	config                          *config.ProcessConfig
	createTime, startTime, stopTime int64
	restartCount                    int64
	cmd                             *exec.Cmd
	state                           State
	stopFlag                        chans.StopChan
	writers                         []io.WriteCloser
	cb                              ProcessExitedCb
	cmdQueue                        chan interface{}
	shutdown                        chans.StopChan
}

type ProcessState struct {
	State                           State
	Restart                         int64
	CreateTime, StartTime, StopTime int64
	Config                          config.ProcessConfig
	PID                             string
}

func NewProcess(cnf *config.ProcessConfig, pe ProcessExitedCb) *Process {
	p := &Process{
		createTime: time.Now().Unix(),
		config:     cnf.Clone(),
		stopFlag:   chans.NewStopChan(),
		cb:         pe,
		state:      WaitSchedule,
		cmdQueue:   make(chan interface{}, 3),
		shutdown:   chans.NewStopChan(),
	}
	go p.processCommand()
	return p
}

func (p *Process) GetState() ProcessState {
	stopTm := p.stopTime
	if stopTm < p.startTime {
		stopTm = 0
	}
	var pid string
	if p.cmd != nil && p.cmd.Process != nil {
		pid = strconv.FormatInt(int64(p.cmd.Process.Pid), 10)
	}
	env := make(map[string]string)
	fp.KVStreamOf(p.config.ENV).
		ZipMap(func(k, v string) string {
			return fmt.Sprintf(`%s=%s`, k, v)
		}).
		Union(fp.StreamOf(os.Environ())).
		UniqBy(func(pair string) string {
			return strings.Split(pair, "=")[0]
		}).
		ToSetBy(func(pair string) (string, string) {
			arr := strings.SplitN(pair, "=", 2)
			return arr[0], arr[1]
		}).
		To(&env)
	ps := ProcessState{
		State:      p.state,
		Restart:    p.restartCount,
		CreateTime: p.createTime,
		StartTime:  p.startTime,
		StopTime:   stopTm,
		Config:     *p.config.Clone(),
		PID:        pid,
	}
	ps.Config.ENV = env
	return ps
}

func (p *Process) Start() error {
	cmd := newStartCmd()
	p.sendCmd(cmd)
	return <-cmd.errCh
}

func (p *Process) Stop() error {
	cmd := newStopCmd()
	p.sendCmd(cmd)
	<-cmd.done
	return nil
}

func (p *Process) Shutdown() error {
	p.Stop()
	p.shutdown.Stop()
	return nil
}

func (p *Process) OmitExitCode() {
	p.config.OmitExitCode = true
}

func (p *Process) runProcess(startCallback func(error)) {
	// On Linux, pdeathsig will kill the child process when the thread dies,
	// not when the process dies. runtime.LockOSThread ensures that as long
	// as this function is executing that OS thread will still be around
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	callbackOnce := new(sync.Once)
	if startCallback == nil {
		startCallback = func(error) {}
	}
	logger.Log("starting process %v", p.config.Name)
	/* reset state and get finalizer */
	flag, releaseFn := p.runProcessPrepare()
	defer releaseFn()
ENTRY:
	/* create command and start */
	if err := p.runProcessStartCommand(flag); err != nil {
		callbackOnce.Do(func() { startCallback(err) })
		return
	}
	/* set state to running and write pid file */
	p.runProcessUpdateState()
	callbackOnce.Do(func() { startCallback(nil) })
	/* wait process exit */
	p.runProcessWait(flag)
	/* clear writer and remove pid file */
	p.releaseProcessResource()
	/* check exit code */
	if shouldRestart := p.runProcessCheckResult(flag); shouldRestart {
		interval := p.restartInterval(p.restartCount)
		p.state = Starting
		logger.Log("will restart after %v, total restart count %v", interval, p.restartCount+1)
		select {
		case <-time.After(interval):
		case <-flag.C():
			return
		}
		p.restartCount++
		p.startTime = time.Now().Unix()
		goto ENTRY
	}
}

func (p *Process) stopProcess() {
	p._stopProcess()
	p.releaseProcessResource()
}

func (p *Process) _stopProcess() {
	sig := p.config.StopSignal
	if sig == "" {
		sig = config.DefaultStopSignal
	}
	ossig, err := signals.ToSignal(sig)
	if err != nil {
		logger.Log("parse signal fail %v", err.Error())
		ossig = syscall.SIGTERM
	}
	if p.cmd != nil && p.cmd.Process != nil {
		logger.Log("send signal %s to process %s", sig, p.config.Name)
		signals.Kill(p.cmd.Process, ossig, true)
	}
	/* wait */
	waitSec := firstPositive(p.config.StopWaitSecs, config.DefaultStopWaitSecs) * 1000
	interval := 100
	for i := 0; i < int(waitSec/interval); i++ {
		if !p.isRunning() {
			logger.Log("process %s is halt after send %s %s later", p.config.Name, sig, time.Millisecond*time.Duration(interval*i))
			return
		}
		time.Sleep(time.Millisecond * time.Duration(interval))
	}
	if p.cmd != nil && p.cmd.Process != nil {
		logger.Log("send signal %s to process %s", `KILL`, p.config.Name)
		signals.Kill(p.cmd.Process, syscall.SIGKILL, true)
	}
}

func (p *Process) releaseProcessResource() {
	for _, w := range p.writers {
		w.Close()
	}
	p.writers = nil
	if p.config.PidFile != "" {
		os.Remove(p.config.PidFile)
	}
}

func (p *Process) isRunning() bool {
	if p.cmd != nil && p.cmd.Process != nil {
		if runtime.GOOS == "windows" {
			proc, err := os.FindProcess(p.cmd.Process.Pid)
			return proc != nil && err == nil
		}
		return signals.Kill(p.cmd.Process, syscall.Signal(0), true) == nil
	}
	return false
}

func (p *Process) createCommand() error {
	cmd := exec.Command(p.config.Command, p.config.Args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
	cmd.Env = fp.KVStreamOf(p.config.ENV).
		ZipMap(func(k, v string) string {
			return fmt.Sprintf(`%s=%s`, k, v)
		}).
		Union(fp.StreamOf(os.Environ())).
		UniqBy(func(pair string) string {
			return strings.Split(pair, "=")[0]
		}).
		Strings()
	cmd.Dir = p.config.CWD
	if p.config.SysUser != "" {
		u, err := user.Lookup(p.config.SysUser)
		if err != nil {
			return err
		}
		uid, err := strconv.ParseUint(u.Uid, 10, 32)
		if err != nil {
			return err
		}
		gid, err := strconv.ParseUint(u.Gid, 10, 32)
		if err != nil && p.config.SysGroup == "" {
			return err
		}
		if p.config.SysGroup != "" {
			g, err := user.LookupGroup(p.config.SysGroup)
			if err != nil {
				return err
			}
			gid, err = strconv.ParseUint(g.Gid, 10, 32)
			if err != nil {
				return err
			}
		}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), NoSetGroups: true}
	}
	/* writer */
	writers := make(map[string]io.WriteCloser)
	fp.StreamOf(p.config.Stderr).
		Union(fp.StreamOf(p.config.Stdout)).
		Uniq().
		ToSetBy(func(w string) (string, io.WriteCloser) {
			switch w {
			case `/dev/null`:
				return w, writeCloser(io.Discard)
			case `/dev/stdout`:
				return w, writeCloser(os.Stdout)
			case `/dev/stderr`:
				return w, writeCloser(os.Stdout)
			default:
				/* file logger */
				os.MkdirAll(filepath.Dir(w), 0755)
				keepCount := 24
				if p.config.StdLogCount > 0 {
					keepCount = p.config.StdLogCount
				}
				if writer, err := filelog.NewWriter(w, filelog.Keep(keepCount), filelog.KeepMaxSize(parseMaxLogSize(p.config.StdLogSize)), filelog.CreateShortcut(true), filelog.RotateBy(filelog.RotateHourly), filelog.DisableWatchFile()); err != nil {
					logger.Log("create logger fail %v", err)
				} else {
					return w, writer
				}
			}
			return w, writeCloser(io.Discard)
		}).
		To(&writers)
	getWriters := func(list []string) io.Writer {
		if len(list) == 0 {
			return io.Discard
		}
		var ws []io.Writer
		fp.StreamOf(list).
			Uniq().
			Map(func(n string) io.Writer {
				return writers[n]
			}).
			ToSlice(&ws)
		return io.MultiWriter(ws...)
	}
	cmd.Stdout = getWriters(p.config.Stdout)
	cmd.Stderr = getWriters(p.config.Stderr)
	p.cmd = cmd
	fp.KVStreamOf(writers).Values().ToSlice(&p.writers)
	return nil
}

func writeCloser(w io.Writer) io.WriteCloser {
	return &wcloser{Writer: w}
}

type wcloser struct {
	io.Writer
}

func (w *wcloser) Close() error { return nil }

func (p *Process) restartInterval(cnt int64) time.Duration {
	switch {
	case cnt == 0:
		return time.Millisecond * 10
	case cnt < 4:
		return time.Second * 1
	case cnt < 10:
		return time.Second * 3
	case cnt < 50:
		return time.Second * 30
	default:
		return time.Second * 60
	}
}

func (p *Process) runProcessPrepare() (chans.StopChan, func()) {
	p.stopFlag = chans.NewStopChan()
	flag := p.stopFlag
	flag.Add(1)

	p.config.OmitExitCode = false
	p.state = Starting
	p.startTime = time.Now().Unix()
	p.restartCount = 0
	p.stopTime = 0
	return flag, func() {
		p.state = Stopped
		p.config.OmitExitCode = false
		p.stopTime = time.Now().Unix()
		flag.Done()
	}
}

func (p *Process) runProcessStartCommand(flag chans.StopChan) error {
	if err := p.createCommand(); err != nil {
		logger.Log("create command fail %v", err)
		return err
	}
	var startErr error
	const maxStartCount = 2
	for i := 0; i < maxStartCount && !flag.IsStopped(); i++ {
		if startErr = p.cmd.Start(); startErr != nil {
			logger.Log("start command fail %v", startErr.Error())
			time.Sleep(5 * time.Second)
		} else {
			return nil
		}
	}
	if startErr != nil {
		return startErr
	}
	logger.Log("abandon start because user request process to halt")
	return errors.New(`abandon start command`)
}

func (p *Process) runProcessUpdateState() {
	p.state = Running
	if p.config.PidFile != "" {
		os.MkdirAll(filepath.Dir(p.config.PidFile), 0755)
		os.WriteFile(p.config.PidFile, []byte(fmt.Sprint(p.cmd.Process.Pid)), 0644)
	}
	logger.Log("process %s started", p.config.Name)
}

func (p *Process) runProcessWait(flag chans.StopChan) {
	if err := p.cmd.Wait(); err != nil {
		if strings.Contains(err.Error(), `killed`) && !flag.IsStopped() {
			logger.Log("process %s terminated with %v, maybe OOM", p.config.Name, err.Error())
		} else {
			logger.Log("process %s terminated with %v", p.config.Name, err.Error())
		}
	}
}

func (p *Process) runProcessCheckResult(flag chans.StopChan) (shouldRestart bool) {
	/* check exit code */
	exitCodes := p.config.ExitCodes
	if len(exitCodes) == 0 {
		exitCodes = []int{config.DefaultSuccessExitCode}
	}
	exitCodeMatch := fp.StreamOf(exitCodes).ContainsBy(func(code int) bool {
		return p.cmd.ProcessState.ExitCode() == code || p.config.OmitExitCode
	})
	switch {
	case exitCodeMatch:
		if flag.IsStopped() {
			logger.Log("process %s exit with code %v, user op.", p.config.Name, p.cmd.ProcessState.ExitCode())
		} else {
			logger.Log("process %s exit with code %v, treat as success", p.config.Name, p.cmd.ProcessState.ExitCode())
		}
		p.cb(flag.IsStopped())
	case !exitCodeMatch && flag.IsStopped():
		logger.Log("process %s exit with code %v", p.config.Name, p.cmd.ProcessState.ExitCode())
	case !exitCodeMatch && !flag.IsStopped():
		logger.Log("process %s UNEXPECTED exit with code %v, will restart", p.config.Name, p.cmd.ProcessState.ExitCode())
		return true
	}
	return false
}

func firstPositive(nums ...int) int {
	return fp.StreamOf(nums).Filter(func(n int) bool { return n > 0 }).First().Int()
}

func parseMaxLogSize(keepSize string) int64 {
	parse := func(unit string) (int64, bool) {
		str := strings.ToUpper(keepSize)
		if strings.HasSuffix(str, unit) {
			v, _ := strconv.ParseInt(strings.TrimSuffix(str, unit), 10, 64)
			return v, v > 0
		}
		return 0, false
	}
	if v, ok := parse("G"); ok {
		return v * filelog.G
	}
	if v, ok := parse("M"); ok {
		return v * filelog.M
	}
	if v, ok := parse("K"); ok {
		return v * filelog.K
	}
	if v, _ := strconv.ParseInt(keepSize, 10, 64); v > 0 {
		return v
	}
	return 1 * filelog.G
}

package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	chans "github.com/qjpcpu/channel"
	"github.com/qjpcpu/fp"
	"github.com/qjpcpu/supervisord/config"
	"github.com/qjpcpu/supervisord/reaper"
)

var (
	singleton *Supervisord
	initOnce  sync.Once
)

type Supervisord struct {
	admin        Server
	stopChan     chans.StopChan
	processMap   map[string]*Process
	processDone  *sync.Map
	processMutex *sync.RWMutex
	processExit  chan bool
}

type StopOption struct {
	ClearLog        bool
	StopImmediately bool
}

func (o StopOption) String() string {
	return fmt.Sprintf("clear_log=%t stop_immediately=%t", o.ClearLog, o.StopImmediately)
}

func (s *Supervisord) Start() error {
	ctx := context.Background()
	cnf := config.Provider().GetConfig()
	initLogger(cnf.Log)
	if cnf.ReapZombie {
		reaper.ReapZombie()
	}
	s.setenv(cnf)
	if err := s.StartAll(ctx, true); err != nil {
		return err
	}
	s.admin.Start()
	<-s.stopChan.C()
	return nil
}

func (s *Supervisord) StartProcess(ctx context.Context, name string) error {
	s.processMutex.RLock()
	defer s.processMutex.RUnlock()
	p, ok := s.processMap[name]
	if !ok {
		return fmt.Errorf("process %s no exist", name)
	}
	if st := p.GetState().State; st == Running || st == Starting {
		return fmt.Errorf("Error: %s is running", name)
	}
	p.Start()
	s.processDone.Delete(name)
	return nil
}

func (s *Supervisord) OmitProcessExitCode(ctx context.Context, name string) error {
	s.processMutex.RLock()
	defer s.processMutex.RUnlock()
	p, ok := s.processMap[name]
	if !ok {
		return fmt.Errorf("process %s no exist", name)
	}
	p.OmitExitCode()
	logger.Log("omit exit code of %v", name)
	return nil
}

func (s *Supervisord) OmitAllProcessExitCode(ctx context.Context) error {
	s.processMutex.RLock()
	defer s.processMutex.RUnlock()
	for _, p := range s.processMap {
		p.OmitExitCode()
	}
	return nil
}

func (s *Supervisord) StopProcess(ctx context.Context, name string) error {
	s.processMutex.RLock()
	defer s.processMutex.RUnlock()
	p, ok := s.processMap[name]
	if !ok {
		return fmt.Errorf("process %s no exist", name)
	}
	p.Stop(false)
	return nil
}

func (s *Supervisord) RestartProcess(ctx context.Context, name string) error {
	s.processMutex.RLock()
	defer s.processMutex.RUnlock()
	p, ok := s.processMap[name]
	if !ok {
		return fmt.Errorf("process %s no exist", name)
	}
	p.Stop(false)
	p.Start()
	s.processDone.Delete(name)
	return nil
}

func (s *Supervisord) IsAllProcessDone(ctx context.Context) bool {
	s.processMutex.RLock()
	defer s.processMutex.RUnlock()
	var count int
	for name := range s.processMap {
		if _, ok := s.processDone.Load(name); ok {
			count++
		}
	}
	return count == len(s.processMap)
}

func (s *Supervisord) StartAll(ctx context.Context, includeFinished bool) error {
	s.processMutex.Lock()
	defer s.processMutex.Unlock()
	return s.startAll(ctx, includeFinished)
}

func (s *Supervisord) StopAll(ctx context.Context) error {
	s.processMutex.Lock()
	defer s.processMutex.Unlock()
	return s.stopAll(ctx, false)
}

func (s *Supervisord) RestartAll(ctx context.Context) error {
	s.processMutex.Lock()
	defer s.processMutex.Unlock()
	s.stopAll(ctx, false)
	s.startAll(ctx, true)
	return nil
}

func (s *Supervisord) Reload() error {
	s.processMutex.Lock()
	defer s.processMutex.Unlock()
	ctx := context.Background()
	doneFlags := new(sync.Map)
	s.processDone.Range(func(key, value interface{}) bool { doneFlags.Store(key, value); return true })
	s.stopAll(ctx, false)

	cnf, err := config.Provider().ReloadConfig()
	if err != nil {
		return err
	}
	s.admin.Reload(cnf.AdminListenAddr())
	s.setenv(cnf)
	s.processDone = doneFlags
	s.startAll(ctx, false)
	return nil
}

func (s *Supervisord) Stop(option StopOption) {
	s.processMutex.Lock()
	defer s.processMutex.Unlock()
	ctx := context.Background()
	logger.Log("terminating all process and supervisord, option %s", option.String())
	s.stopAll(ctx, option.StopImmediately)
	logger.Log("all process terminated")
	s.admin.Stop()
	if option.ClearLog {
		logger.Close()
		s.clearLogs()
	}
	s.stopChan.Stop()
}

func (s *Supervisord) GetProcessList() (list []*Process) {
	s.processMutex.RLock()
	defer s.processMutex.RUnlock()
	for name := range s.processMap {
		list = append(list, s.processMap[name])
	}
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].createTime < list[j].createTime
	})
	return
}

func (s *Supervisord) AddProc(ctx context.Context, addProc *config.AddProcConfig) error {
	s.processMutex.Lock()
	defer s.processMutex.Unlock()
	proc := addProc.ProcessConfig
	if p := s.processMap[proc.Name]; p != nil {
		p.Shutdown(false)
	}

	prov := config.Provider()
	gconf := prov.GetConfig()
	gconf.AddProcessConfig(proc)
	prov.UpdateConfig(gconf)

	s.processMap[proc.Name] = NewProcess(proc, func(byuser bool) {
		s.processDone.Store(proc.Name, struct{}{})
		s.processExit <- byuser
	})
	return s.processMap[proc.Name].Start()
}

func (s *Supervisord) startAll(ctx context.Context, includeFinished bool) error {
	alreadyDone := s.processDone
	s.processDone = new(sync.Map)
	cnf := config.Provider().GetConfig()
	for _, p := range cnf.Process {
		name := p.Name
		if pro := s.processMap[name]; pro != nil {
			if st := pro.GetState().State; st == Running || st == Starting {
				return fmt.Errorf("Error: %s is running", name)
			}
			pro.Shutdown(false)
		}
		s.processMap[name] = NewProcess(p, func(byuser bool) {
			s.processDone.Store(name, struct{}{})
			s.processExit <- byuser
		})
		if _, ok := alreadyDone.Load(name); ok && !includeFinished {
			s.processDone.Store(name, struct{}{})
			continue
		}
		s.processMap[p.Name].Start()
	}
	return nil
}

func (s *Supervisord) stopAll(ctx context.Context, stopImediately bool) error {
	for _, p := range s.processMap {
		p.Shutdown(stopImediately)
	}
	s.processMap = make(map[string]*Process)
	return nil
}

func Get() *Supervisord {
	initOnce.Do(func() {
		cnf := config.Provider().GetConfig()
		s := &Supervisord{
			admin:        newFuncServer(cnf.AdminListenAddr(), startAdminServer),
			stopChan:     chans.NewStopChan(),
			processMap:   make(map[string]*Process),
			processMutex: new(sync.RWMutex),
			processDone:  new(sync.Map),
			processExit:  make(chan bool, 1),
		}
		installSignals(s)
		singleton = s
	})
	return singleton
}

func installSignals(s *Supervisord) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for {
			select {
			case sig := <-sigs:
				logger.Log("receive signal %v", sig)
				s.Stop(StopOption{})
				os.Exit(-1)
			case byuser := <-s.processExit:
				cnf := config.Provider().GetConfig()
				if cnf.ExitWhenAllProcessDone && s.IsAllProcessDone(context.Background()) && !byuser {
					logger.Log("all process exited, supervisord would exit too")
					s.Stop(StopOption{})
					return
				}
			}
		}
	}()
}

func (s *Supervisord) clearLogs() {
	files := s.collectPurgeFiles()
	if len(files) == 0 {
		return
	}
	wg := new(sync.WaitGroup)
	for i := range files {
		file := files[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := exec.Command("bash", "-c", fmt.Sprintf("rm -fr %v", file)).Run(); err != nil {
				fmt.Printf("[%s] remove log %v fail %v\n", time.Now().Format(`2006-01-02 15:04:05`), file, err)
			} else {
				fmt.Printf("[%s] remove log %v success\n", time.Now().Format(`2006-01-02 15:04:05`), file)
			}
		}()
	}
	wg.Wait()
}

func (s *Supervisord) collectPurgeFiles() []string {
	conf := config.Provider().GetConfig()
	appendStar := func(s []string) []string {
		return fp.StreamOf(s).
			Reject(func(v string) bool {
				return v == "" || v == "/"
			}).
			Map(func(v string) string { return v + "*" }).
			Strings()
	}
	rewritePurgeFiles := func(fs []string) (ret []string) {
		for _, item := range fs {
			if strings.HasSuffix(item, "/") {
				ret = append(ret, item+"*")
			} else {
				ret = append(ret, item)
			}
		}
		return
	}

	paths := fp.StreamOf(conf.Process).
		FlatMap(func(c *config.ProcessConfig) []string {
			var list []string
			list = append(list, appendStar(c.Stdout)...)
			list = append(list, appendStar(c.Stderr)...)
			list = append(list, rewritePurgeFiles(c.PurgeFiles)...)
			return list
		}).
		Union(fp.StreamOf(appendStar([]string{conf.Log}))).
		Uniq().
		SortBy(func(a, b string) bool {
			return len(a) > len(b)
		}).
		Strings()
	var ret []string

	for i := 0; i < len(paths); i++ {
		var dup bool
		for j := i + 1; j < len(paths); j++ {
			if filePathContains(paths[j], paths[i]) {
				dup = true
				break
			}
		}
		if !dup {
			ret = append(ret, paths[i])
		}
	}
	return ret
}

func (s *Supervisord) setenv(c *config.SupervisorConfig) {
	os.Setenv("SUPERVISOR_ADDRESS", c.AdminDialAddr())
}

func filePathContains(a, b string) bool {
	if a == b {
		return true
	}
	aStar, bStar := strings.HasSuffix(a, "*"), strings.HasSuffix(b, "*")
	a = strings.TrimSuffix(a, "*")
	b = strings.TrimSuffix(b, "*")
	addSlash := func(s string) string {
		if strings.HasSuffix(s, "/") {
			return s
		}
		return s + "/"
	}
	switch {
	case aStar && bStar:
		return strings.HasPrefix(b, a)
	case !aStar && bStar:
		return strings.HasPrefix(b, addSlash(a))
	case aStar && !bStar:
		return strings.HasPrefix(b, a)
	case !aStar && !bStar:
		return strings.HasPrefix(b, addSlash(a))
	}
	return false
}

package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/qjpcpu/fp"
	"github.com/qjpcpu/supervisord/sys"
)

const (
	DefaultStopWaitSecs    = 15
	DefaultStopSignal      = "TERM"
	DefaultSuccessExitCode = 0
)

type SupervisorConfigInfo struct {
	File   string
	Config *SupervisorConfig
}

type SupervisorConfig struct {
	AdminListen            int              `toml:"admin_listen" param:"adminlisten,supervisord admin listen on"`
	AdminBindIP            string           `toml:"admin_bind_ip" param:"admin_bind_ip,supervisord listen on ip default 0.0.0.0"`
	AdminSock              string           `toml:"admin_sock" param:"admin_sock,supervisord listen on unix socket"`
	Log                    string           `toml:"log,omitempty" param:"log,supervisord log file"`
	Process                []*ProcessConfig `toml:"process" param:"-"`
	ExitWhenAllProcessDone bool             `toml:"exit_when_all_done" param:"exit_when_all_done,exit when all process finished with success"`
	EnableLogFinalizer     bool             `toml:"enable_log_finalizer" param:"enable_log_finalizer,supervisor would not clean log when set false"`
	Daemonize              bool             `toml:"daemonize" param:"daemonize,daemonize supervisord"`
	ReapZombie             bool             `toml:"reap_zombie" param:"reap_zombie,reap zombie process"`
	HideArgs               bool             `toml:"hide_args" param:"hide_args,hide command arguments"`
	DisableRCE             bool             `toml:"disable_rce" param:"disable_rce,disable rce"`
}

type AddProcConfig struct {
	*ProcessConfig
}

type ProcessConfig struct {
	Name         string            `toml:"name" param:"-"`
	Command      string            `toml:"command" param:"-"`
	Args         []string          `toml:"args" param:"-"`
	CWD          string            `toml:"cwd" param:"cwd,process cwd"`
	ENV          map[string]string `toml:"env" param:"env,process extra env vars, e.g. k1=v1,k2=v2"`
	PidFile      string            `toml:"pid_file" param:"pid,process pid file"`
	ExitCodes    []int             `toml:"exit_codes" param:"exitcodes,process exit code, e.g. 0,1,2"`
	StopSignal   string            `toml:"stop_signal" param:"stopsig,process stop signal, default TERM"` // TERM
	StopWaitSecs int               `toml:"stop_wait_secs" param:"stop_wait_secs,process terminating wait seconds, default 15s"`
	Stdout       []string          `toml:"stdout" param:"stdout,process stdout, default /dev/stdout"`
	Stderr       []string          `toml:"stderr" param:"stderr,process stderr, default /dev/stderr"`
	PurgeFiles   []string          `toml:"purge_files" param:"purge_files,purge files when supervisord exiting"`
	StdLogCount  int               `toml:"std_log_count" param:"std_log_count,keep max std log files, default 48"`
	StdLogSize   string            `toml:"std_log_size" param:"std_log_size,keep max log size, default 1G"`
	SysUser      string            `toml:"user,omitempty" param:"user,process user, default current user"`
	SysGroup     string            `toml:"group,omitempty" param:"group,process user group, default current user group"`
	OmitExitCode bool              `toml:"omit_exit_code,omitempty" param:"omit_exit_code,treat all exit code as success, default false"`
}

func (self *ProcessConfig) ParseFlags(flags map[string]string) error {
	if err := parseFlags(self, flags); err != nil {
		return err
	}
	self.FillDefaults()
	return nil
}

func (self *ProcessConfig) FillDefaults() *ProcessConfig {
	if len(self.Stdout) == 0 && len(self.Stderr) == 0 {
		self.Stdout = []string{"/dev/stdout"}
		self.Stderr = []string{"/dev/stderr"}
	} else if len(self.Stdout) == 0 && len(self.Stderr) > 0 {
		self.Stdout = self.Stderr
	} else if len(self.Stdout) > 0 && len(self.Stderr) == 0 {
		self.Stderr = self.Stdout
	}
	if len(self.ExitCodes) == 0 {
		self.ExitCodes = []int{DefaultSuccessExitCode}
	}
	if self.CWD == "" {
		self.CWD = fp.M(os.Getwd()).
			Map(filepath.Abs).
			Val().
			String()
	}
	if self.StopSignal == "" {
		self.StopSignal = DefaultStopSignal
	}
	if self.StopWaitSecs == 0 {
		self.StopWaitSecs = DefaultStopWaitSecs
	}
	if self.StdLogCount == 0 {
		self.StdLogCount = 48
	}
	if self.StdLogSize == "" {
		self.StdLogSize = "1G"
	}
	return self
}

func (self *ProcessConfig) Clone() *ProcessConfig {
	data, _ := json.Marshal(self)
	n := new(ProcessConfig)
	json.Unmarshal(data, n)
	return n
}

func (self *SupervisorConfig) IsBlank() bool {
	bs1, err1 := config_marshal(self)
	bs2, err2 := config_marshal(new(SupervisorConfig))
	return err1 == nil && err2 == nil && bytes.Equal(bs1, bs2)
}

func (self *SupervisorConfig) ExistProcess(name string) bool {
	for _, p := range self.Process {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (self *SupervisorConfig) AddProcessConfig(p *ProcessConfig) {
	for i, proc := range self.Process {
		if proc.Name == p.Name {
			self.Process[i] = p
			return
		}
	}
	self.Process = append(self.Process, p)
}

func (self *SupervisorConfig) AdminListenAddr() string {
	if self.AdminSock != "" {
		return fmt.Sprintf("unix://%s", self.AdminSock)
	}
	if self.AdminBindIP == "" {
		return fmt.Sprintf(":%v", self.AdminListen)
	}
	return fmt.Sprintf("%v:%v", self.AdminBindIP, self.AdminListen)
}

func (self *SupervisorConfig) AdminDialProtocol() string {
	if self.AdminSock != "" {
		return "unix"
	}
	return "tcp"
}

func (self *SupervisorConfig) AdminDialAddr() string {
	if self.AdminSock != "" {
		sock, _ := filepath.Abs(self.AdminSock)
		return sock
	}
	if self.AdminBindIP == "" {
		return fmt.Sprintf("localhost:%v", self.AdminListen)
	}
	return fmt.Sprintf("%v:%v", self.AdminBindIP, self.AdminListen)
}

func supervisordDir() string {
	path, _ := filepath.Abs(sys.Args()[0])
	return filepath.Dir(path)
}

func findSupervisordConf() (string, error) {
	dir := supervisordDir()
	possibleSupervisordConf := []string{
		filepath.Join(dir, `../conf/supervisord.conf`),
		filepath.Join(dir, `supervisord.conf`),
	}

	for _, file := range possibleSupervisordConf {
		if _, err := os.Stat(file); err == nil {
			absFile, err := filepath.Abs(file)
			if err == nil {
				return absFile, nil
			}
			return file, nil
		}
	}

	return "", fmt.Errorf("fail to find supervisord.conf")
}

func (self *SupervisorConfig) ParseFlags(flags map[string]string) error {
	if err := parseFlags(self, flags); err != nil {
		return err
	}
	if self.AdminListen == 0 && self.AdminSock == "" {
		/* acquire admin port */
		adminPort, err := sys.GetFreePort()
		if err != nil {
			return err
		}
		self.AdminListen = adminPort
	}
	return nil
}

func (self *AddProcConfig) ParseFlags(flags map[string]string) error {
	if err := parseFlags(self, flags); err != nil {
		return err
	}
	return nil
}

func parseFlags(objectPtr interface{}, flags map[string]string) error {
	if len(flags) == 0 {
		return nil
	}
	typ := reflect.TypeOf(objectPtr).Elem()
	val := reflect.ValueOf(objectPtr).Elem()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		p := field.Tag.Get(`param`)
		if strings.HasPrefix(p, "-") || p == "" {
			continue
		}
		name := strings.SplitN(p, ",", 2)[0]
		if str, ok := flags[name]; ok {
			switch field.Type.Kind() {
			case reflect.String:
				val.Field(i).SetString(strings.TrimSpace(str))
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				num, err := strconv.Atoi(str)
				if err != nil {
					return err
				}
				val.Field(i).SetInt(int64(num))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				num, err := strconv.ParseUint(str, 10, 64)
				if err != nil {
					return err
				}
				val.Field(i).SetUint(num)
			case reflect.Bool:
				b, err := strconv.ParseBool(str)
				if err != nil {
					return err
				}
				val.Field(i).SetBool(b)
			case reflect.Map:
				/* only map[string]string */
				val.Field(i).Set(reflect.ValueOf(ParseEnv(str).AsMap()))
			case reflect.Slice:
				switch field.Type.Elem().Kind() {
				case reflect.Int:
					list := fp.StreamOf(strings.Split(str, ",")).
						Reject(fp.EmptyString()).
						Map(strconv.Atoi).
						Ints()
					val.Field(i).Set(reflect.ValueOf(list))
				case reflect.Int64:
					list := fp.StreamOf(strings.Split(str, ",")).
						Reject(fp.EmptyString()).
						Map(strconv.Atoi).
						Map(func(i int) int64 { return int64(i) }).
						Ints()
					val.Field(i).Set(reflect.ValueOf(list))
				case reflect.Int32:
					list := fp.StreamOf(strings.Split(str, ",")).
						Reject(fp.EmptyString()).
						Map(strconv.Atoi).
						Map(func(i int) int32 { return int32(i) }).
						Ints()
					val.Field(i).Set(reflect.ValueOf(list))
				case reflect.String:
					list := fp.StreamOf(strings.Split(str, ",")).
						Reject(fp.EmptyString()).
						Strings()
					val.Field(i).Set(reflect.ValueOf(list))
				}
			}
		}
	}
	return nil
}

type Env struct {
	Key, Val string
}

type EnvList []*Env

func (e EnvList) String() string {
	return fp.StreamOf(e).
		Map(func(e *Env) string {
			return e.Key + "=" + e.Val
		}).
		JoinStrings(",")
}

func (e EnvList) AsMap() map[string]string {
	ret := make(map[string]string)
	for _, item := range e {
		ret[item.Key] = item.Val
	}
	return ret
}

func (e EnvList) Drop(keys ...string) (ret EnvList) {
	set := fp.StreamOf(keys).ToSet()
	fp.StreamOf(e).
		Reject(func(item *Env) bool {
			return set.Contains(item.Key)
		}).
		ToSlice(&ret)
	return
}

func ParseEnv(str string) (list EnvList) {
	arr := fp.StreamOf(strings.Split(str, ",")).Reject(fp.EmptyString()).Strings()
	for _, str := range arr {
		if strings.Contains(str, "=") {
			vals := strings.SplitN(str, "=", 2)
			list = append(list, &Env{Key: vals[0], Val: vals[1]})
		} else {
			if len(list) == 0 {
				list = append(list, &Env{Key: str})
			} else {
				i := len(list) - 1
				list[i].Val = strings.TrimPrefix(list[i].Val+","+str, ",")
			}
		}
	}
	return
}

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/qjpcpu/supervisord/ctl"
	"github.com/qjpcpu/supervisord/daemon"
	"github.com/qjpcpu/supervisord/sys"

	"github.com/qjpcpu/supervisord/config"
)

func Run() {
	args := sys.Args()
	if len(args) < 2 {
		showHelp()
		os.Exit(1)
	}
	var err error
	switch args[1] {
	case `service`:
		err = serviceCommand(getArgsFrom(2, args))
	case `start`:
		err = startDaemon(getArgsFrom(2, args))
	case `add-proc`:
		err = addProc(getArgsFrom(2, args))
	case `reload`:
		err = reloadDaemon()
	case `exec`:
		vargs := getArgsFrom(2, args)
		if len(vargs) != 1 {
			showHelp()
			os.Exit(1)
		}
		err = execCommand(vargs[0])
	case `shutdown`:
		vargs := getArgsFrom(2, args)
		force := len(vargs) == 1 && vargs[0] == "-f"
		if !confirmShutdown(force) {
			return
		}
		err = ctl.Shutdown(context.Background(), !force)
	case `help`, `-h`, `--help`, `-help`, `h`:
		showHelp()
	default:
		showHelp()
		err = fmt.Errorf("not supported subcommand %s", args[1])
	}
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func startDaemon(args []string) error {
	if err := ctl.Status(context.Background()); err == nil {
		return errors.New("supervisord is already running")
	}

	// Before starting as a new daemon, the program first acts as a "client"
	// to check if another supervisord instance is already running (via ctl.Status()).
	// This check requires reading the existing configuration using the default "client-mode" provider.
	// Only after confirming that no other instance is active, we switch to the "master-mode"
	// provider to take on the role of the main daemon.
	prov := config.UseProvider(config.NewProvider(true))

	flags, args := extractSupervisorFlags(args)
	if prov.GetConfig().IsBlank() {
		cnf := new(config.SupervisorConfig)
		if err := cnf.ParseFlags(flags); err != nil {
			return err
		}
		if len(args) >= 2 {
			p, err := parseProcessConfig(args)
			if err != nil {
				return err
			}
			cnf.AddProcessConfig(p)
		}
		prov.UpdateConfig(cnf)
	}
	cnf := prov.GetConfig()
	if cnf.Daemonize {
		Daemonize(func() {
			defer prov.Close()
			daemon.Get().Start()
		})
		return nil
	}
	defer prov.Close()
	return daemon.Get().Start()
}

func addProc(args []string) error {
	flags, args := extractSupervisorFlags(args)
	cnf := new(config.AddProcConfig)
	if err := cnf.ParseFlags(flags); err != nil {
		return err
	}
	if len(args) < 2 {
		return errors.New("lost command args")
	}
	p, err := parseProcessConfig(args)
	if err != nil {
		return err
	}
	cnf.ProcessConfig = p
	return ctl.AddProc(context.Background(), cnf)
}

func reloadDaemon() error {
	return ctl.Reload(context.Background())
}

func execCommand(file string) error {
	return ctl.ExecCommand(context.Background(), file)
}

func serviceCommand(args []string) error {
	if len(args) == 0 {
		return errors.New(`no subcommand found`)
	}
	ctx := context.Background()
	switch args[0] {
	case `start`:
		if len(args) > 1 {
			return ctl.StartProcess(ctx, args[1])
		} else {
			return ctl.StartAll(ctx)
		}
	case `stop`:
		if len(args) > 1 {
			return ctl.StopProcess(ctx, args[1])
		} else {
			return ctl.StopAll(ctx)
		}
	case `restart`:
		if len(args) > 1 {
			return ctl.RestartProcess(ctx, args[1])
		} else {
			return ctl.RestartAll(ctx)
		}
	case `status`:
		return ctl.Status(ctx)
	case `env`:
		return ctl.DumpEnv(ctx)
	case `omit-exit-code`:
		if len(args) > 1 {
			return ctl.OmitProcessExitCode(ctx, args[1])
		} else {
			return ctl.OmitAllProcessExitCode(ctx)
		}
	case `help`, `-h`, `--help`, `-help`, `h`:
		fmt.Println(strings.TrimSpace(cmdServiceHelpInfo()))
		return nil
	default:
		fmt.Println(strings.TrimSpace(cmdServiceHelpInfo()))
		return fmt.Errorf(`not supported service subcommand %s`, args[0])
	}
}

func getArgsFrom(i int, args []string) []string {
	if i >= 0 && i < len(args) {
		return args[i:]
	}
	return nil
}

const (
	supervisorFlagPrefix = `-supvr.`
)

func extractSupervisorFlags(args []string) (flags map[string]string, extra []string) {
	flags = make(map[string]string)
	var i int
	for i = 0; i < len(args)-1; i += 2 {
		if strings.HasPrefix(args[i], supervisorFlagPrefix) {
			flags[strings.TrimPrefix(args[i], supervisorFlagPrefix)] = args[i+1]
		} else {
			break
		}
	}
	if i < len(args) && i >= 0 {
		extra = args[i:]
	}
	return
}

// process_name [PROCESS OPTION] cmd args
// len(args) should >= 2
func parseProcessConfig(args []string) (*config.ProcessConfig, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf(`[length less than 2]parse process args fail %#v`, args)
	}
	name := args[0]
	flags, args := extractSupervisorFlags(args[1:])
	/* now args should be cmd [args] */
	if len(args) == 0 {
		return nil, fmt.Errorf(`[no command]parse process args fail %#v`, args)
	}
	p := &config.ProcessConfig{
		Name:    name,
		Command: args[0],
		Args:    getArgsFrom(1, args),
	}
	if err := p.ParseFlags(flags); err != nil {
		return nil, err
	}
	return p, nil
}

func confirmShutdown(force bool) bool {
	if force {
		return true
	}
	prompt := promptui.Prompt{
		Label:     "Stop all process and then exit supervisord",
		IsConfirm: true,
	}
	result, err := prompt.Run()
	if err != nil {
		fmt.Println(time.Now().Format("15:04:05"), "Canceled")
		return false
	}
	doShutdown := result == "y"
	if doShutdown {
		fmt.Println(time.Now().Format("15:04:05"), "Begin to shutdown supervisord")
	} else {
		fmt.Println(time.Now().Format("15:04:05"), "Canceled")
	}
	return doShutdown
}

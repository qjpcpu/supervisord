package cli

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/qjpcpu/fp"
	"github.com/qjpcpu/supervisord/color"
	"github.com/qjpcpu/supervisord/config"
)

func showHelp() {
	text := []string{
		cmdStartHelpInfo(),
		cmdAddProcHelpInfo(),
		cmdReloadHelpInfo(),
		cmdShutdownHelpInfo(),
		cmdServiceHelpInfo(),
		cmdExecHelpInfo(),
		cmdHelpHelpInfo(),
	}
	info := fp.StreamOf(text).
		Map(func(s string) string {
			return strings.TrimSpace(s)
		}).
		JoinStrings("\n")
	fmt.Print(info + "\n")
}

func space(i int) string {
	return strings.Repeat(" ", i)
}

func cmdStartHelpInfo() string {
	getParams := func(typ reflect.Type) []string {
		return fp.Times(typ.NumField()).
			Map(func(i int) string {
				return typ.Field(i).Tag.Get(`param`)
			}).
			Reject(func(tag string) bool {
				return tag == `` || strings.HasPrefix(tag, `-`) || strings.Count(tag, ",") == 0
			}).
			Map(func(tag string) string {
				arr := strings.SplitN(tag, ",", 2)
				return space(8) + fmt.Sprintf(`%s %s`, color.Yellow(supervisorFlagPrefix+arr[0]), arr[1])
			}).
			Strings()
	}
	helpBuf := new(strings.Builder)
	helpBuf.WriteString(fmt.Sprintf("supervisord %s [GLOBAL OPTIONS] [NAME [PROCESS OPTIONS] %s args...]\n", color.Yellow(`start`), color.Yellow(`cmd`)))
	helpBuf.WriteString(space(4) + "GLOBAL OPTIONS:\n")
	typ := reflect.TypeOf(config.SupervisorConfig{})
	helpBuf.WriteString(strings.Join(getParams(typ), "\n") + "\n")
	helpBuf.WriteString(space(4) + "PROCESS OPTIONS:\n")
	typ = reflect.TypeOf(config.ProcessConfig{})
	helpBuf.WriteString(strings.Join(getParams(typ), "\n") + "\n")
	return helpBuf.String()
}

func cmdAddProcHelpInfo() string {
	getParams := func(typ reflect.Type) []string {
		return fp.Times(typ.NumField()).
			Map(func(i int) string {
				return typ.Field(i).Tag.Get(`param`)
			}).
			Reject(func(tag string) bool {
				return tag == `` || strings.HasPrefix(tag, `-`) || strings.Count(tag, ",") == 0
			}).
			Map(func(tag string) string {
				arr := strings.SplitN(tag, ",", 2)
				return space(8) + fmt.Sprintf(`%s %s`, color.Yellow(supervisorFlagPrefix+arr[0]), arr[1])
			}).
			Strings()
	}
	helpBuf := new(strings.Builder)
	helpBuf.WriteString(fmt.Sprintf("supervisord %s NAME [PROCESS OPTIONS] %s args...\n", color.Yellow(`add-proc`), color.Yellow(`cmd`)))
	helpBuf.WriteString(space(4) + "PROCESS OPTIONS:\n")
	typ := reflect.TypeOf(config.ProcessConfig{})
	helpBuf.WriteString(strings.Join(getParams(typ), "\n") + "\n")
	return helpBuf.String()
}

func cmdReloadHelpInfo() string {
	helpBuf := new(strings.Builder)
	helpBuf.WriteString(fmt.Sprintf("supervisord %s\n", color.Yellow(`reload`)))
	helpBuf.WriteString(space(4) + "reload supervisor config from file and restart process\n")
	return helpBuf.String()
}

func cmdExecHelpInfo() string {
	helpBuf := new(strings.Builder)
	helpBuf.WriteString(fmt.Sprintf("supervisord %s\n", color.Yellow(`exec`)))
	helpBuf.WriteString(space(4) + "exec glisp command file\n")
	return helpBuf.String()
}

func cmdShutdownHelpInfo() string {
	helpBuf := new(strings.Builder)
	helpBuf.WriteString(fmt.Sprintf("supervisord %s (-f)\n", color.Red(`shutdown`)))
	helpBuf.WriteString(space(4) + color.Red(`[DANGER]`) + " stop all process by send signal TERM, then exit supervisord\n")
	return helpBuf.String()
}

func cmdServiceHelpInfo() string {
	helpBuf := new(strings.Builder)
	helpBuf.WriteString(fmt.Sprintf("supervisord %s %s\n", color.Yellow(`service`), color.Green(`start`)))
	helpBuf.WriteString(space(4) + "start process\n")

	helpBuf.WriteString(fmt.Sprintf("supervisord %s %s\n", color.Yellow(`service`), color.Green(`stop`)))
	helpBuf.WriteString(space(4) + "stop process\n")

	helpBuf.WriteString(fmt.Sprintf("supervisord %s %s\n", color.Yellow(`service`), color.Green(`restart`)))
	helpBuf.WriteString(space(4) + "restart process\n")

	helpBuf.WriteString(fmt.Sprintf("supervisord %s %s\n", color.Yellow(`service`), color.Green(`status`)))
	helpBuf.WriteString(space(4) + "display process status\n")

	helpBuf.WriteString(fmt.Sprintf("supervisord %s %s\n", color.Yellow(`service`), color.Green(`env`)))
	helpBuf.WriteString(space(4) + "display process env\n")

	return helpBuf.String()
}

func cmdHelpHelpInfo() string {
	helpBuf := new(strings.Builder)
	helpBuf.WriteString(fmt.Sprintf("supervisord %s\n", color.Yellow(`help/-h/-help/--help`)))
	helpBuf.WriteString(space(4) + "show help\n")
	return helpBuf.String()
}

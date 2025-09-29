package sys

import (
	"github.com/erikdubbelboer/gspt"
	"os"
	"strings"
	"sync"
)

var (
	args     []string
	loadOnce sync.Once
	hideArgs bool
)

func Args() []string {
	loadOnce.Do(func() {
		if !shouldHideArgs() {
			args = os.Args
			gspt.SetProcTitle(strings.Join(os.Args, " "))
			return
		}
		args = make([]string, len(os.Args))
		for i := 0; i < len(os.Args); i++ {
			args[i] = copyStr(os.Args[i])
		}
		if len(os.Args) >= 2 && os.Args[1] == "start" {
			shellArgs := []string{os.Args[0], os.Args[1]}
			if name, cmd := findCmd(); name != "" && cmd != "" {
				shellArgs = append(shellArgs, name, cmd)
			}
			gspt.SetProcTitle(strings.Join(shellArgs, " "))
		}
	})
	return args
}

func copyStr(s string) string {
	bytes := []byte(s)
	return string(bytes)
}

func shouldHideArgs() bool {
	for i := 2; i < len(os.Args); {
		if strings.HasPrefix(os.Args[i], "-supvr.") {
			if os.Args[i] == "-supvr.hide_args" && i < len(os.Args)-1 && os.Args[i+1] == "true" {
				return true
			}
			/* skip global options */
			i += 2
		} else {
			break
		}
	}
	return false
}

func findCmd() (string, string) {
	var name string
	for i := 2; i < len(os.Args); {
		if strings.HasPrefix(os.Args[i], "-supvr.") {
			/* skip global options */
			i += 2
		} else {
			name = copyStr(os.Args[i])
			/* skip process name */
			for j := i + 1; j < len(os.Args); {
				/* skip process options */
				if strings.HasPrefix(os.Args[j], "-supvr.") {
					j += 2
				} else {
					return name, copyStr(os.Args[j])
				}
			}
			break
		}
	}
	return "", ""
}

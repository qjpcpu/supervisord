package color

import (
	"github.com/fatih/color"
)

var (
	Green   = color.New(color.FgGreen, color.Bold).SprintFunc()
	Cyan    = color.New(color.FgCyan, color.Bold).SprintFunc()
	Magenta = color.New(color.FgMagenta, color.Bold).SprintFunc()
	Yellow  = color.New(color.FgYellow, color.Bold).SprintFunc()
	Red     = color.New(color.FgRed, color.Bold).SprintFunc()
	Blue    = color.New(color.FgBlue, color.Bold).SprintFunc()
)

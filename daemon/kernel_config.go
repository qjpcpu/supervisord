package daemon

import "github.com/qjpcpu/glisp"

var kernelConfig = new(Kernel)

type Kernel struct {
	AdminModule []func(*glisp.Environment) error
}

func SetKernelConfig(config *Kernel) {
	if config != nil {
		kernelConfig = config
	}
}

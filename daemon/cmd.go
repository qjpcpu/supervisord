package daemon

import (
	"runtime/debug"
	"sync"
)

func newStartCmd() *cmdStart {
	return &cmdStart{errCh: make(chan error, 1)}
}

func newStopCmd(stopImediately bool) *cmdStop {
	return &cmdStop{done: make(chan struct{}, 1), stopImediately: stopImediately}
}

type cmdStart struct {
	errCh chan error
}

func (cmd *cmdStart) SendResult(err error) {
	cmd.errCh <- err
}

type cmdStop struct {
	done           chan struct{}
	stopImediately bool
}

func (p *Process) sendCmd(cmd interface{}) {
	p.cmdQueue <- cmd
}

func (p *Process) processCommand() {
	for {
		select {
		case cmd, ok := <-p.cmdQueue:
			if !ok {
				return
			}
			p.onCommand(cmd)
		case <-p.shutdown.C():
			return
		}
	}
}

func (p *Process) onCommand(cmd interface{}) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log("proccess command %T fail %v %v", cmd, r, string(debug.Stack()))
		}
	}()
	switch msg := cmd.(type) {
	case *cmdStart:
		p.onStartCommand(msg)
	case *cmdStop:
		p.onStopCommand(msg)
	}
}

func (p *Process) onStartCommand(cmd *cmdStart) {
	if p.state == Running || p.state == Starting {
		logger.Log("process %v is already running", p.config.Name)
		cmd.SendResult(nil)
		return
	}
	started := make(chan error, 1)
	callback := func(err error) { started <- err }
	go p.runProcess(callback)
	cmd.SendResult(<-started)
}

func (p *Process) onStopCommand(cmd *cmdStop) {
	defer func() {
		cmd.done <- struct{}{}
	}()
	if p.state == WaitSchedule {
		logger.Log("process %v is not started", p.config.Name)
		return
	}
	if p.state == Stopped {
		logger.Log("process %v is already stopped", p.config.Name)
		return
	}
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()
		p.stopFlag.Stop()
	}()
	go func() {
		defer wg.Done()
		p.stopProcess(cmd.stopImediately)
	}()
	wg.Wait()
}

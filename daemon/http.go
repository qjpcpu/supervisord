package daemon

type Server interface {
	Start()
	Reload(addr string)
	Stop()
}

func newFuncServer(addr string, f func(string) func()) Server {
	return &funcServer{startFn: f, addr: addr}
}

type funcServer struct {
	addr    string
	startFn func(string) func()
	stopFn  func()
}

func (f *funcServer) Reload(addr string) {
	if f.addr != addr {
		f.addr = addr
		f.Stop()
		f.Start()
	}
}

func (f *funcServer) Start() {
	f.stopFn = f.startFn(f.addr)
}

func (f *funcServer) Stop() {
	if fn := f.stopFn; fn != nil {
		f.stopFn = nil
		fn()
	}
}

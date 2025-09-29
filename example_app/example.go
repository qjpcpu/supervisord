package main

import (
	"flag"
	"os/signal"
	"strings"
	"time"

	"os"

	"fmt"
	"github.com/qjpcpu/fp"
	"github.com/qjpcpu/supervisord/signals"
)

var (
	flag_run_timeout     int
	flag_exit_code       int
	flag_swallow_signal  string
	flag_list_env_prefix string
)

func main() {
	flag.StringVar(&flag_swallow_signal, "swallow_signal", "", "")
	flag.IntVar(&flag_run_timeout, "run_timeout", 10, "")
	flag.IntVar(&flag_exit_code, "exit_code", 0, "")
	flag.StringVar(&flag_list_env_prefix, "env_prefix", "TEST", "")
	flag.Parse()

	sigs := make(chan os.Signal, 1)
	if flag_swallow_signal != "" {
		s, err := signals.ToSignal(flag_swallow_signal)
		if err != nil {
			fmt.Println(err)
			return
		}
		signal.Notify(sigs, s)
	}
	fmt.Println(time.Now().Format("2006-01-02 15:04:05"), "example app running")
	fp.StreamOf(os.Environ()).
		Filter(func(e string) bool {
			return strings.HasPrefix(e, flag_list_env_prefix)
		}).
		Foreach(func(kv string) {
			fmt.Printf(time.Now().Format("15:04:05")+" %s\n", kv)
		}).
		Run()
	start := time.Now().Unix()
	for {
		select {
		case s := <-sigs:
			fmt.Printf(time.Now().Format("2006-01-02 15:04:05")+" swallow signal %v\n", s)
		case <-time.After(time.Millisecond * 5):
			now := time.Now().Unix()
			if now-start > int64(flag_run_timeout) {
				fmt.Printf(time.Now().Format("2006-01-02 15:04:05")+" app run timeout %vs\n", now-start)
				fmt.Printf(time.Now().Format("2006-01-02 15:04:05")+" i will exit with %v\n", flag_exit_code)
				os.Exit(flag_exit_code)
			}
		}
	}
}

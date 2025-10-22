package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/qjpcpu/glisp"
	ext "github.com/qjpcpu/glisp/extensions"
	myhttp "github.com/qjpcpu/http"
	"github.com/qjpcpu/supervisord/config"
)

func startAdminServer(addr string) func() {
	s := myhttp.NewServer()
	renderSuccess := func(w http.ResponseWriter, text string) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, text)
	}
	renderObject := func(w http.ResponseWriter, text any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(text)
	}
	renderError := func(w http.ResponseWriter, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"code": -1, "message": err.Error()})
	}

	s.GET("/omit_exit_code", func(w http.ResponseWriter, r *http.Request) {
		logger.Log("[admin-api] omit exit code %s", extractParams(r))
		if all := r.URL.Query().Get("all"); all == "true" {
			if err := Get().OmitAllProcessExitCode(context.Background()); err != nil {
				renderError(w, err)
				return
			}
			renderSuccess(w, "OK")
			return
		}
		if err := Get().OmitProcessExitCode(context.Background(), r.URL.Query().Get("name")); err != nil {
			renderError(w, err)
			return
		}
		renderSuccess(w, "OK")
	})
	s.GET("/start", func(w http.ResponseWriter, r *http.Request) {
		logger.Log("[admin-api] start %s", extractParams(r))
		if all := r.URL.Query().Get("all"); all == "true" {
			if err := Get().StartAll(context.Background(), true); err != nil {
				renderError(w, err)
				return
			}
			renderSuccess(w, "OK")
			return
		}
		name := r.URL.Query().Get("name")
		if err := Get().StartProcess(context.Background(), name); err != nil {
			renderError(w, err)
			return
		}
		renderSuccess(w, "OK")
	})
	s.GET("/stop", func(w http.ResponseWriter, r *http.Request) {
		logger.Log("[admin-api] stop %s", extractParams(r))
		if all := r.URL.Query().Get("all"); all == "true" {
			if err := Get().StopAll(context.Background()); err != nil {
				renderError(w, err)
				return
			}
			renderSuccess(w, "OK")
			return
		}
		name := r.URL.Query().Get("name")
		if err := Get().StopProcess(context.Background(), name); err != nil {
			renderError(w, err)
			return
		}
		renderSuccess(w, "OK")
	})
	s.GET("/restart", func(w http.ResponseWriter, r *http.Request) {
		logger.Log("[admin-api] restart %s", extractParams(r))
		if all := r.URL.Query().Get("all"); all == "true" {
			if err := Get().RestartAll(context.Background()); err != nil {
				renderError(w, err)
				return
			}
			renderSuccess(w, "OK")
			return
		}
		name := r.URL.Query().Get("name")
		if err := Get().RestartProcess(context.Background(), name); err != nil {
			renderError(w, err)
			return
		}
		renderSuccess(w, "OK")
	})
	s.GET("/reload", func(w http.ResponseWriter, r *http.Request) {
		logger.Log("[admin-api] reload %s", extractParams(r))
		if err := config.Provider().CheckConfigFile(); err != nil {
			logger.Log("reload config %v", err)
			renderError(w, err)
			return
		}
		if err := Get().Reload(); err != nil {
			logger.Log("reload config %v", err)
			renderError(w, err)
			return
		}
		renderSuccess(w, "OK")
	})
	s.POST("/add_process", func(w http.ResponseWriter, r *http.Request) {
		logger.Log("[admin-api] add process %s", extractParams(r))
		procConfig := new(config.AddProcConfig)
		if r.Body == nil {
			renderError(w, errors.New("no body found"))
			return
		}
		defer r.Body.Close()
		bs, err := io.ReadAll(r.Body)
		if err != nil {
			renderError(w, err)
			return
		}
		if err := json.Unmarshal(bs, procConfig); err != nil {
			renderError(w, err)
			return
		}
		if procConfig.Name == "" {
			renderError(w, errors.New("bad proc"))
			return
		}
		if err := Get().AddProc(context.Background(), procConfig); err != nil {
			renderError(w, err)
			return
		}
		renderObject(w, map[string]any{"code": 0, "message": "ok"})
	})
	s.POST("/command", func(w http.ResponseWriter, r *http.Request) {
		logger.Log("[admin-api] run command %s", extractParams(r))
		if body := r.Body; body != nil {
			defer body.Close()
			command, err := io.ReadAll(body)
			if err != nil {
				renderError(w, err)
				return
			}
			defer func() {
				if r := recover(); r != nil {
					renderError(w, fmt.Errorf("%v %v", r, string(debug.Stack())))
					return
				}
			}()
			config := config.Provider().GetConfig()
			if config.DisableRCE {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			ret, err := runAdminCommand(config, command, r.URL.Query().Get("capture_stdout") == "true")
			if err != nil {
				renderError(w, err)
				return
			}
			renderSuccess(w, string(ret))
			return
		}
		renderSuccess(w, "")
	})
	s.GET("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		logger.Log("[admin-api] shutdown %s", extractParams(r))
		Get().Stop(StopOption{
			StopImmediately: r.URL.Query().Get("now") == "true",
			ClearLog:        r.URL.Query().Get("clear") == "true",
		})
		renderSuccess(w, "OK")
	})
	s.GET("/status", func(w http.ResponseWriter, r *http.Request) {
		logger.Log("[admin-api] status %s", extractParams(r))
		processList := Get().GetProcessList()
		if r.URL.Query().Get("format") == `json` {
			var states []ProcessState
			for _, p := range processList {
				state := p.GetState()
				states = append(states, state)
			}
			renderObject(w, states)
			return
		}
		var text strings.Builder
		t := table.NewWriter()
		t.SetOutputMirror(&text)
		row := table.Row{"name", "pid", "state", "start-time", "stop-time", "restart"}
		t.AppendHeader(row)
		for _, p := range processList {
			state := p.GetState()
			t.AppendRow([]any{
				state.Config.Name,
				state.PID,
				state.State,
				time.Unix(state.StartTime, 0),
				time.Unix(state.StopTime, 0),
				state.Restart,
			})
		}
		t.SetStyle(table.StyleLight)
		t.Render()
		renderSuccess(w, text.String())
	})
	go func() {
		network := "tcp"
		if sock, ok := strings.CutPrefix(addr, "unix://"); ok {
			network = "unix"
			addr = sock
		}
		if err := s.ListenAndServe(network, addr); err != nil && err != http.ErrServerClosed {
			logger.Log("listen err %v", err)
		}
	}()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if err := s.Close(ctx); err != nil && !strings.Contains(err.Error(), "context deadline exceeded") {
			logger.Log("Server forced to shutdown: %v", err)
		}
	}
}

func runAdminCommand(config *config.SupervisorConfig, cmd []byte, captureStdout bool) ([]byte, error) {
	env := glisp.New()
	output := &scriptWriter{captureStdout: captureStdout}
	modules := []func(*glisp.Environment) error{
		func(e *glisp.Environment) error { return e.ImportEval() },
		ext.ImportAll,
		func(_env *glisp.Environment) error {
			_env.AddNamedFunction("println", ext.GetPrintFunction(output))
			_env.AddNamedFunction("printf", ext.GetPrintFunction(output))
			_env.AddNamedFunction("print", ext.GetPrintFunction(output))
			return nil
		},
	}
	for _, extra := range kernelConfig.AdminModule {
		modules = append(modules, extra)
	}

	for _, f := range modules {
		if err := f(env); err != nil {
			return nil, err
		}
	}
	bs, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	configBytes, err := ext.ParseJSON(bs)
	if err != nil {
		return nil, err
	}
	env.BindGlobal("CONFIG", configBytes)
	ret, err := env.EvalString(string(cmd))
	if err != nil {
		return nil, err
	}
	bytes, err := glisp.Marshal(ret)
	if err != nil {
		return nil, err
	}
	if captureStdout {
		output.buf.Write(bytes)
		bytes = output.buf.Bytes()
	}
	return bytes, nil
}

type scriptWriter struct {
	captureStdout bool
	buf           bytes.Buffer
}

func (self *scriptWriter) Write(p []byte) (n int, err error) {
	logger.Log("%s", string(p))
	if self.captureStdout {
		self.buf.Write(p)
	}
	return len(p), nil
}

func extractParams(r *http.Request) string {
	vals, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return err.Error()
	}
	var str []string
	for k := range vals {
		str = append(str, fmt.Sprintf("%s=%s", k, vals.Get(k)))
	}
	return strings.Join(str, ",")
}

package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/qjpcpu/glisp"
	ext "github.com/qjpcpu/glisp/extensions"
	"github.com/qjpcpu/supervisord/config"
)

func init() {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = nullDaemonLog{}
	gin.DefaultErrorWriter = &loggerToWriter{}
}

func startAdminServer(addr string) func() {
	r := gin.Default()
	r.Use(RecoverMW)
	r.GET("/omit_exit_code/:name", func(c *gin.Context) {
		if err := Get().OmitProcessExitCode(c, c.Param("name")); err != nil {
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		c.String(http.StatusOK, "OK")
	})
	r.GET("/omit_exit_code_all", func(c *gin.Context) {
		if err := Get().OmitAllProcessExitCode(c); err != nil {
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		c.String(http.StatusOK, "OK")
	})
	r.GET("/start/:name", func(c *gin.Context) {
		logger.Log("[admin] start `%v`", c.Param("name"))
		if err := Get().StartProcess(c, c.Param("name")); err != nil {
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		c.String(http.StatusOK, "OK")
	})
	r.GET("/stop/:name", func(c *gin.Context) {
		logger.Log("[admin] stop `%v`", c.Param("name"))
		if err := Get().StopProcess(c, c.Param("name")); err != nil {
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		c.String(http.StatusOK, "OK")
	})
	r.GET("/restart/:name", func(c *gin.Context) {
		logger.Log("[admin] restart `%v`", c.Param("name"))
		if err := Get().RestartProcess(c, c.Param("name")); err != nil {
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		c.String(http.StatusOK, "OK")
	})
	r.GET("/startall", func(c *gin.Context) {
		logger.Log("[admin] startall %v", extractParams(c))
		if err := Get().StartAll(c, true); err != nil {
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		c.String(http.StatusOK, "OK")
	})
	r.GET("/stopall", func(c *gin.Context) {
		logger.Log("[admin] stopall %v", extractParams(c))
		if err := Get().StopAll(c); err != nil {
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		c.String(http.StatusOK, "OK")
	})
	r.GET("/restartall", func(c *gin.Context) {
		logger.Log("[admin] restartall %v", extractParams(c))
		if err := Get().RestartAll(c); err != nil {
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		c.String(http.StatusOK, "OK")
	})
	r.GET("/reload", func(c *gin.Context) {
		logger.Log("[admin] reload %v", extractParams(c))
		if err := config.Provider().CheckConfigFile(); err != nil {
			logger.Log("reload config %v", err)
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		if err := Get().Reload(); err != nil {
			logger.Log("reload config %v", err)
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		c.String(http.StatusOK, "OK")
	})
	r.POST("/add_process", func(c *gin.Context) {
		procConfig := new(config.AddProcConfig)
		if err := c.BindJSON(procConfig); err != nil {
			c.PureJSON(http.StatusBadRequest, gin.H{"code": -1, "message": err.Error()})
			return
		}
		if procConfig.Name == "" {
			c.PureJSON(http.StatusBadRequest, gin.H{"code": -1, "message": "bad proc"})
			return
		}
		if err := Get().AddProc(c, procConfig); err != nil {
			c.PureJSON(http.StatusBadRequest, gin.H{"code": -1, "message": err.Error()})
			return
		}
		c.PureJSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
	})
	r.POST("/command", func(c *gin.Context) {
		if body := c.Request.Body; body != nil {
			defer body.Close()
			command, err := io.ReadAll(body)
			if err != nil {
				c.String(http.StatusBadRequest, err.Error())
				return
			}
			defer func() {
				if r := recover(); r != nil {
					c.String(http.StatusBadRequest, fmt.Sprintf("%v %v", r, string(debug.Stack())))
					return
				}
			}()
			config := config.Provider().GetConfig()
			if config.DisableRCE {
				c.Status(http.StatusUnauthorized)
				return
			}
			ret, err := runAdminCommand(config, command, c.Query("capture_stdout") == "true")
			if err != nil {
				c.String(http.StatusBadRequest, err.Error())
				return
			}
			c.String(http.StatusOK, string(ret))
			return
		}
		c.Status(http.StatusOK)
	})
	r.GET("/shutdown", func(c *gin.Context) {
		logger.Log("[admin] shutdown %v", extractParams(c))
		Get().Stop(StopOption{StopImmediately: c.Query("now") == "true", ClearLog: c.Query("clear") == "true"})
		c.String(http.StatusOK, "OK")
	})
	r.GET("/status", func(c *gin.Context) {
		processList := Get().GetProcessList()
		if c.Query("format") == `json` {
			var states []ProcessState
			for _, p := range processList {
				state := p.GetState()
				states = append(states, state)
			}
			c.PureJSON(http.StatusOK, states)
			return
		}
		var text strings.Builder
		w := table.NewWriter()
		w.SetOutputMirror(&text)
		row := table.Row{"name", "pid", "state", "start-time", "stop-time", "restart"}
		w.AppendHeader(row)
		for _, p := range processList {
			state := p.GetState()
			w.AppendRow([]interface{}{
				state.Config.Name,
				state.PID,
				state.State,
				time.Unix(state.StartTime, 0),
				time.Unix(state.StopTime, 0),
				state.Restart,
			})
		}
		w.SetStyle(table.StyleLight)
		w.Render()
		c.String(http.StatusOK, text.String())
	})
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}
	go func() {
		if strings.HasPrefix(addr, "unix://") {
			sock := strings.TrimPrefix(addr, "unix://")
			sock, _ = filepath.Abs(sock)
			dir := filepath.Dir(sock)
			if _, err := os.Stat(dir); err != nil && os.IsNotExist(err) {
				os.MkdirAll(dir, 0755)
			}
			os.RemoveAll(sock)
			unixAddr, err := net.ResolveUnixAddr("unix", sock)
			if err != nil {
				logger.Log("listen err %v", err)
				return
			}
			ln, err := net.ListenUnix("unix", unixAddr)
			if err != nil {
				logger.Log("listen err %v", err)
				return
			}
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				logger.Log("listen err %v", err)
			}
		} else {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Log("listen err %v", err)
			}
		}
	}()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil && !strings.Contains(err.Error(), "context deadline exceeded") {
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

func extractParams(c *gin.Context) string {
	vals, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		return err.Error()
	}
	var str []string
	for k := range vals {
		str = append(str, fmt.Sprintf("%v=%v", k, vals.Get(k)))
	}
	return strings.Join(str, ",")
}

func RecoverMW(c *gin.Context) {
	defer func() {
		if err := recover(); err != nil {
			var brokenPipe bool
			if ne, ok := err.(*net.OpError); ok {
				if se, ok := ne.Err.(*os.SyscallError); ok {
					if strings.Contains(strings.ToLower(se.Error()), "broken pipe") || strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
						brokenPipe = true
					}
				}
			}
			if brokenPipe {
				logger.Log("%s", err)
			} else {
				logger.Log("[Recovery] panic recovered: err[%s] stack[%s]", err, debug.Stack())
			}
			if brokenPipe {
				c.Error(err.(error)) // nolint: errcheck
				c.Abort()
			} else {
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}
	}()
	c.Next()
}

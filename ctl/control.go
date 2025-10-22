package ctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	chttp "github.com/qjpcpu/http"
	"github.com/qjpcpu/supervisord/daemon"

	"github.com/qjpcpu/fp"
	"github.com/qjpcpu/supervisord/config"
)

const adminBaseURL = "http://admin:80"

func getAdminClient() (chttp.Client, error) {
	cnf := config.Provider().GetConfig()
	if cnf.IsBlank() {
		return nil, errors.New(`no supervisor config found`)
	}
	conn, err := net.Dial(cnf.AdminDialProtocol(), cnf.AdminDialAddr())
	if err != nil {
		return nil, fmt.Errorf("supervisord not running on %v", cnf.AdminDialAddr())
	}
	conn.Close()
	return chttp.NewClient().
		SetTimeout(60 * time.Second).
		WithDialer(func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial(cnf.AdminDialProtocol(), cnf.AdminDialAddr())
		}), nil
}

func StartProcess(ctx context.Context, name string) error {
	return controlProcess(ctx, fmt.Sprintf(`/start?name=%s`, name))
}

func StopProcess(ctx context.Context, name string) error {
	return controlProcess(ctx, fmt.Sprintf(`/stop?name=%s`, name))
}

func RestartProcess(ctx context.Context, name string) error {
	return controlProcess(ctx, fmt.Sprintf(`/restart?name=%s`, name))
}

func OmitProcessExitCode(ctx context.Context, name string) error {
	return controlProcess(ctx, fmt.Sprintf(`/omit_exit_code?name=%s`, name))
}

func OmitAllProcessExitCode(ctx context.Context) error {
	return controlProcess(ctx, `/omit_exit_code?all=true`)
}

func StartAll(ctx context.Context) error {
	return controlProcess(ctx, `/start?all=true`)
}

func StopAll(ctx context.Context) error {
	return controlProcess(ctx, `/stop?all=true`)
}

func RestartAll(ctx context.Context) error {
	return controlProcess(ctx, `/restart?all=true`)
}

func Reload(ctx context.Context) error {
	return controlProcess(ctx, `/reload`)
}

func Status(ctx context.Context) error {
	return controlProcess(ctx, `/status`)
}

func DumpEnv(ctx context.Context) error {
	var states []daemon.ProcessState
	result, _ := requestProcess(ctx, `/status?format=json`)
	json.Unmarshal([]byte(result), &states)
	for _, p := range states {
		fmt.Printf("[%s]\n", p.Config.Name)
		for k, v := range p.Config.ENV {
			fmt.Printf("%v=%v\n", k, v)
		}
	}
	return nil
}

func Shutdown(ctx context.Context, option daemon.StopOption) error {
	result, err := requestProcess(ctx,
		fmt.Sprintf(`/shutdown?now=%s&clear=%s`,
			strconv.FormatBool(option.StopImmediately),
			strconv.FormatBool(option.ClearLog),
		))
	if err != nil && strings.Contains(err.Error(), "EOF") {
		result = "OK"
		err = nil
	}
	if result != "" {
		fmt.Println(result)
	}
	return err
}

func AddProc(ctx context.Context, addProc *config.AddProcConfig) error {
	ret, err := postProcess(ctx, "/add_process", addProc)
	if err != nil {
		return err
	}
	fmt.Println(ret)
	return nil
}

func ExecCommand(ctx context.Context, file string) error {
	var result string
	err := fp.M(getAdminClient()).
		Map(func(client chttp.Client) ([]byte, error) {
			data, err := os.ReadFile(file)
			if err != nil {
				return nil, err
			}
			return client.PostJSON(context.TODO(), fmt.Sprintf(`%s/command?capture_stdout=%v`, adminBaseURL, hasSheBang(data)), bytes.NewBuffer(dropSheBang(data))).GetBody()
		}).
		Map(func(body []byte) string {
			return strings.TrimSpace(string(body))
		}).
		Val().
		To(&result)
	if result != "" {
		fmt.Println(result)
	}
	return err
}

func hasSheBang(data []byte) bool {
	return len(data) > 2 && data[0] == '#' && data[1] == '!'
}

func dropSheBang(data []byte) []byte {
	if len(data) > 2 && data[0] == '#' && data[1] == '!' {
		for i := 2; i < len(data); i++ {
			if data[i] == '\n' {
				return data[i+1:]
			}
		}
	}
	return data
}

func controlProcess(ctx context.Context, path string) error {
	result, err := requestProcess(ctx, path)
	if result != "" {
		fmt.Println(result)
	}
	return err
}

func requestProcess(ctx context.Context, path string) (string, error) {
	path = addQuery(path)
	var result string
	err := fp.M(getAdminClient()).
		Map(func(client chttp.Client) ([]byte, error) {
			return client.Get(context.TODO(), fmt.Sprintf(`%s%s`, adminBaseURL, path)).GetBody()
		}).
		Map(func(body []byte) string {
			return strings.TrimSpace(string(body))
		}).
		Val().
		To(&result)
	return result, err
}

func addQuery(path string) string {
	if strings.Contains(path, "?") {
		return path + "&by=local_cli"
	}
	return path + "?by=local_cli"
}

func postProcess(ctx context.Context, path string, payload interface{}) (string, error) {
	data, _ := json.Marshal(payload)
	var result string
	err := fp.M(getAdminClient()).
		Map(func(client chttp.Client) ([]byte, error) {
			return client.PostJSON(context.TODO(), fmt.Sprintf(`%s%s`, adminBaseURL, path), bytes.NewBuffer(data)).GetBody()
		}).
		Map(func(body []byte) string {
			return strings.TrimSpace(string(body))
		}).
		Val().
		To(&result)
	return result, err
}

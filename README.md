# Go-Supervisord

A lightweight, high-performance process management tool implemented in Go, inspired by Python's `supervisord`. It allows users to monitor and control multiple processes in the background, ensuring they are automatically restarted if they crash unexpectedly.

## ✨ Core Features

- **Daemonization**: Can run as a background daemon, detached from the terminal.
- **Process Management**: Defines and manages a group of child processes via a simple TOML configuration file.
- **Auto-Restart**: Automatically attempts to restart child processes when they exit due to an unexpected error.
- **Flexible Admin Interface**: Supports both TCP and Unix Socket for remote control.
- **Log Management**: Captures `stdout` and `stderr` of child processes, with support for log rotation by size and count.
- **Dynamic Configuration**: Supports dynamically adding new process configurations at runtime using the `add-proc` command.
- **Zombie Process Reaping**: Automatically reaps zombie child processes.
- **Cross-Platform**: Designed for compatibility with both Linux and Windows platforms.
- **Script Execution**: Supports executing `glisp` scripts for advanced management tasks.
- **Disable RCE**: Can disable remote command execution (`exec` command) for enhanced security.
- **Attach Process on Start**: Allows attaching a process directly via command line arguments when starting.

## 🚀 Quick Start

### 1. Build

```bash
./build.sh
```

### 2. Configuration File

Create a `supervisord.conf` file in your project directory. Here is a basic configuration example:

```toml
# Supervisord's management port
admin_listen = 9001
# Or use a Unix socket for enhanced security
# admin_sock = "/var/run/supervisord.sock"

# Supervisord's own log file
log = "/var/log/supervisord/supervisord.log"

# If true, supervisord will exit after all managed processes have exited successfully
exit_when_all_done = false

# If true, supervisord will run in the background as a daemon
daemonize = true

# Enable automatic reaping of zombie processes
reap_zombie = true

# Disable remote command execution for security
disable_rce = false

# Define a process to be managed
[[process]]
name = "my-app"
command = "/path/to/your/app"
args = ["--port", "8080"]

# Working directory for the process
cwd = "/path/to/your"

# stdout log for the process
stdout = ["/var/log/supervisord/my-app.stdout.log"]

# stderr log for the process
stderr = ["/var/log/supervisord/my-app.stderr.log"]

# Keep up to 10 log files, each up to 100MB
std_log_count = 10
std_log_size = "100M"

# Successful exit codes. If the process exits with one of these codes, it's considered a normal exit and won't be restarted.
exit_codes = [0]

# Seconds to wait before sending a KILL signal when stopping the process
stop_wait_secs = 10

# Signal to send when stopping the process (e.g., TERM, HUP, INT)
stop_signal = "TERM"

# Run process as a specific user and group
user = "nobody"
group = "nogroup"

# Set environment variables for the process
[process.env]
  GO_ENV = "production"
  API_KEY = "your-secret-key"
```

### 3. Start Supervisord

Use the `start` command to launch the `supervisord` daemon. It can be started with a configuration file or by specifying a process directly on the command line.

```bash
# Start the daemon using supervisord.conf
./supervisord start

# Alternatively, run in the foreground for debugging
./supervisord start -supvr.daemonize false

# Start and attach a process directly
./supervisord start my-temp-app /usr/bin/python my_script.py
```

## 📖 Command-line Usage

`supervisord` provides a rich command-line interface to interact with the daemon.

### `service` - Manage Processes

This is the most common command, used to manage processes.

```bash
# Check the status of all processes
./supervisord service status

# Start/Stop/Restart all or a single process
./supervisord service start
./supervisord service start my-app
./supervisord service stop
./supervisord service stop my-app
./supervisord service restart
./supervisord service restart my-app

# Display environment variables of a process
./supervisord service env my-app
```

### `reload` - Reload Configuration

After modifying `supervisord.conf`, use the `reload` command to apply the new configuration.

```bash
./supervisord reload
```

### `shutdown` - Shut Down Supervisord

Stops all running child processes and shuts down the `supervisord` daemon.

```bash
# Graceful shutdown
./supervisord shutdown

# Shutdown immediately without waiting for processes
./supervisord shutdown --now

# Shutdown and clear all log files
./supervisord shutdown --clear
```

### `add-proc` - Add a Process Dynamically

Dynamically add a new process while `supervisord` is running.

```bash
# Dynamically add a process named "my-worker" 
./supervisord add-proc my-worker /usr/bin/python worker.py
```

### `exec` - Remote Execution

Execute a script file (e.g., a `glisp` script) on the running daemon. This can be disabled for security.

```bash
# Execute a script file
./supervisord exec /path/to/your/script.glisp
```

## Usage as a Library

Besides being used as a standalone binary, `go-supervisord` can also be integrated into your own Go projects as a library. By calling `cli.Run()`, you can embed its functionality directly into your application.

```go
import "github.com/qjpcpu/supervisord/cli"

func main() {
  // This will start the supervisord client or daemon
  cli.Run()
}
```

## 🆚 Comparison with `github.com/ochinchina/supervisord`

Both `github.com/qjpcpu/supervisord` (this project) and `github.com/ochinchina/supervisord` are excellent process management tools written in Go, inspired by the classic Python `supervisord`. However, they have some significant differences in design philosophy and feature implementation.

| Feature | `qjpcpu/supervisord` (This Project) | `ochinchina/supervisord` |
| :--- | :--- | :--- |
| **Design Philosophy** | A modern, lightweight Go implementation focusing on performance and extensibility with a modern toolchain. | Aims to be a drop-in replacement for the original Python `supervisord`, with high compatibility for its configuration and XML-RPC API. |
| **Configuration Format** | Uses `TOML` format, which is structured, clear, and easy to read/write. | Uses `INI` format, compatible with the original `supervisord`'s configuration files. |
| **Admin Interface** | Provides an **HTTP/JSON** based API over TCP or Unix Socket, offering a simple and modern interface. | Implements an **XML-RPC** interface compatible with the original, allowing the use of the standard `supervisorctl` client. |
| **Command-line Tool** | Includes a feature-rich built-in client with commands like `service status`, `add-proc`, `reload`, etc. | Also provides a built-in `supervisorctl` client and is compatible with the original one. |
| **Extensibility** | Features a built-in **`glisp` script engine**, allowing users to execute scripts via the `exec` command for advanced, custom management tasks. | Extensibility is mainly focused on faithfully implementing the original's features; no built-in script engine. |
| **Dynamic Management** | Supports dynamically adding new processes at runtime via the `add-proc` command without reloading the entire configuration. | Supports updating process groups via `supervisorctl`'s `add` and `remove` commands. |
| **Security Features** | Provides a `disable_rce` option to disable remote script execution with a single switch for enhanced security. | Security relies on the authentication configuration of the XML-RPC interface. |
| **Dependencies & Ecosystem** | Uses a set of the author's own libraries (e.g., `fp`, `glisp`, `http`), forming a unique Go tool ecosystem. | Has fewer dependencies, focusing on standalone operation and compatibility with the original ecosystem. |

### Summary

-   **Choose `qjpcpu/supervisord` (This Project)** if you:
    -   Prefer using `TOML` for configuration.
    -   Want to integrate with a more modern `HTTP/JSON` API.
    -   Need advanced, flexible automation through scripting (`glisp`).
    -   Are looking for a lightweight, high-performance, and modern process management solution for new Go projects.

-   **Choose `ochinchina/supervisord`** if you:
    -   Need a smooth migration path from an existing Python `supervisord` environment.
    -   Want to continue using `.ini` configuration files and the standard `supervisorctl` client.
    -   Have system integrations that rely on the XML-RPC interface.

In conclusion, `qjpcpu/supervisord` is a more Go-idiomatic and modern implementation that offers greater flexibility and extensibility. In contrast, `ochinchina/supervisord` is an excellent compatibility-focused alternative, designed for seamlessly replacing the original Python version.

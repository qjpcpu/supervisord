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
go build -o supervisord ./main.go
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

# Dynamically treat a process's exit code as success
./supervisord service omit-exit-code my-app
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
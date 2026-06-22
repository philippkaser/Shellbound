#!/usr/bin/env sh
# Launcher for the Shellbound server. Lets you pick how to run it.
#
# The default keeps the server alive after you log out WITHOUT installing a
# systemd service: it builds the binary and starts it detached (nohup), writing
# a pidfile and a log. Run with no argument for an interactive menu, or pass a
# subcommand directly.
#
#   ./scripts/run.sh            # interactive menu (default: background)
#   ./scripts/run.sh start      # background, survives logout (no service)
#   ./scripts/run.sh fg         # foreground (dev; stops on logout)
#   ./scripts/run.sh stop       # stop the background server
#   ./scripts/run.sh status     # is it running?
#   ./scripts/run.sh logs       # follow the log
#   ./scripts/run.sh service    # install as a systemd service (needs root)
set -eu

cd "$(dirname "$0")/.."  # repo root, so the DB and host key land there
BIN=./shellbound
LOG=${SHELLBOUND_LOG:-./shellbound.log}
PIDFILE=${SHELLBOUND_PID:-./shellbound.pid}

build() { go build -o "$BIN" ./cmd/shellbound; }

running() { [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; }

start_bg() {
	if running; then
		echo "already running (pid $(cat "$PIDFILE"))"
		return 0
	fi
	build
	# nohup ignores SIGHUP and exec's the binary in place, so $! is the server's
	# own pid and it keeps running once the SSH session (and its SIGHUP) is gone.
	nohup "$BIN" >"$LOG" 2>&1 </dev/null &
	echo $! >"$PIDFILE"
	echo "shellbound started (pid $(cat "$PIDFILE")) — survives logout."
	echo "logs: ./scripts/run.sh logs   stop: ./scripts/run.sh stop"
}

start_fg() {
	build
	exec "$BIN"
}

stop() {
	if running; then
		kill "$(cat "$PIDFILE")" && echo "stopped"
	else
		echo "not running"
	fi
	rm -f "$PIDFILE"
}

status() {
	if running; then echo "running (pid $(cat "$PIDFILE"))"; else echo "not running"; fi
}

logs() {
	[ -f "$LOG" ] || { echo "no log yet ($LOG)"; return 0; }
	tail -f "$LOG"
}

service() { exec make service; }

menu() {
	echo "How should Shellbound run?"
	echo "  1) background    — survives logout, no service   (default)"
	echo "  2) foreground    — dev mode, stops on logout"
	echo "  3) systemd unit  — survives reboot too (needs root)"
	printf "choice [1]: "
	read -r choice || choice=1
	case "${choice:-1}" in
	1 | "") start_bg ;;
	2) start_fg ;;
	3) service ;;
	*) echo "unknown choice: $choice" >&2; exit 1 ;;
	esac
}

case "${1:-}" in
"") if [ -t 0 ]; then menu; else start_bg; fi ;;
start | bg | background) start_bg ;;
fg | foreground | run) start_fg ;;
stop) stop ;;
status) status ;;
logs) logs ;;
service) service ;;
*) echo "usage: $0 [start|fg|stop|status|logs|service]" >&2; exit 1 ;;
esac

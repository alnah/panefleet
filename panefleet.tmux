run-shell -b 'PANEFLEET_ROOT="$(CDPATH= cd -- "$(dirname -- "#{current_file}")" && pwd)"; export PANEFLEET_ROOT; exec "$PANEFLEET_ROOT/scripts/install-tmux-bindings.sh" install'

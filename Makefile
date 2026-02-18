BINARY = herd
GOFLAGS = -trimpath

.PHONY: build clean run kill reload vet

build:
	go build $(GOFLAGS) -o $(BINARY) .

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)

kill:
	tmux -S ~/.herd/tmux.sock kill-server 2>/dev/null || true
	pkill -f "herd --sidebar" 2>/dev/null || true
	rm -rf ~/.herd

reload: build
	tmux -S ~/.herd/tmux.sock respawn-pane -k \
		-t "$$(tmux -S ~/.herd/tmux.sock list-panes -s -t herd-main \
			-F '#{pane_id} #{pane_start_command}' | grep '\-\-sidebar' | awk '{print $$1}')" \
		./$(BINARY) --sidebar

vet:
	go vet ./...

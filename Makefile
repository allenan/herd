BINARY = herd
GOFLAGS = -trimpath
PROFILE ?=

# Resolve paths based on profile
ifdef PROFILE
  HERD_DIR = $(HOME)/.herd/profiles/$(PROFILE)
  PROFILE_FLAG = --profile $(PROFILE)
  SESSION_NAME = herd-$(PROFILE)-main
else
  HERD_DIR = $(HOME)/.herd
  PROFILE_FLAG =
  SESSION_NAME = herd-main
endif

.PHONY: build clean run kill reload vet

build:
	go build $(GOFLAGS) -o $(BINARY) .

run: build
	./$(BINARY) $(PROFILE_FLAG)

clean:
	rm -f $(BINARY)

kill:
	tmux -S $(HERD_DIR)/tmux.sock kill-server 2>/dev/null || true
	pkill -f "herd --sidebar" 2>/dev/null || true
	rm -rf $(HERD_DIR)

reload: build
	tmux -S $(HERD_DIR)/tmux.sock respawn-pane -k \
		-t "$$(tmux -S $(HERD_DIR)/tmux.sock list-panes -s -t $(SESSION_NAME) \
			-F '#{pane_id} #{pane_start_command}' | grep '\-\-sidebar' | awk '{print $$1}')" \
		./$(BINARY) --sidebar $(PROFILE_FLAG)

vet:
	go vet ./...

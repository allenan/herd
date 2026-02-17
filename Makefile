BINARY = herd
GOFLAGS = -trimpath

.PHONY: build clean run kill vet

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

vet:
	go vet ./...

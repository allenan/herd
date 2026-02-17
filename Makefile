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
	tmux -L herd kill-server 2>/dev/null || true

vet:
	go vet ./...

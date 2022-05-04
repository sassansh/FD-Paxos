.PHONY: client tracing clean all

all: server server2 server3 server4 server5 client tracing

server:
	go build -o bin/server ./cmd/server

server2:
	go build -o bin/server2 ./cmd/server2

server3:
	go build -o bin/server3 ./cmd/server3

server4:
	go build -o bin/server4 ./cmd/server4

server5:
	go build -o bin/server5 ./cmd/server5

client:
	go build -o bin/client ./cmd/client

tracing:
	go build -o bin/tracing ./cmd/tracing-server

clean:
	rm -f bin/*



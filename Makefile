
build:
	go build -o server ./server
	go build -o peer/peer1/ ./peer
	go build -o peer/peer2/ ./peer
	go build -o peer/peer3/ ./peer
run_server:
	./server/server

run_peer:
	./peer/peer

clean:
	go clean
	rm ./peer/peer
	rm ./server/server

build:
	go build -o server ./server
	go build -o peer ./peer
run:
	./server/server

clean:
	go clean
	rm peer/peer1/peer
	rm peer/peer2/peer
	rm server/server
	rm brown.txt
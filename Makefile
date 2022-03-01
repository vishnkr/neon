
build:
	go build -o server ./server
	go build -o peer ./peer
run:
	./server/server

clean:
	go clean
	rm peer/peer
	rm server/server
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"strconv"
	"sync"
)

type File struct {
	Filename string
	Length   int
}

/*

Sample Message Format:

Register Request:
REG 127.0.0.1:8001\r\n a.txt 54\r\n

Register Response:
REG OK (maybe a client id for future use here) \r\n

Modify later after multiple peers come into play

Chunk send Request:
REGCH 127.0.0.1:8001 a.txt 54\r\n [payload with size 54]\r\n

Chunk send Response:
REGCH OK \r\n




Old:
Register Request - REG 127.8.0.1:8001 2 a.txt\r\n
filchunk - REGCH a.txt 43\r\n123123123123123123123123123123\r\n
Register Response - REG RS\r\n a.txt SUC <file-id>
Register Response - REG RS\r\n audio.mps FAIL
REGCH <file-id>:<chunk-id>:length of message\r\n file content
*/
type RequestType int

const (
	RegisterRequest RequestType = iota
	FileListRequest
	FileLocationsRequest
	ChunkRegisterRequest
	FileChunkRequest
	NoneType
)

//const maxBufferSize = 2048

type Message struct {
	reqType RequestType
	sender  string
	body    []byte
}
type FileChunk struct {
	content []byte
	fileID  int
	chunkID int
}

type FileInfo struct {
	fileID         int
	fname          string
	sizeInBytes    int
	totalChunks    int               //chunks 0 to totalChunks-1 will be present as keys in chunkLocations
	chunkLocations map[int][]*Client //map chunkid to list of clients containing
}

type Client struct {
	id       int
	conn     net.Conn
	reader   *bufio.Reader
	addr     net.Addr
	register chan<- *Client
}
type Server struct {
	clients   map[int]*Client
	fileMap   map[string]FileInfo
	fileCount int
}

func getNextandRemaining(buffer []byte, sep []byte) ([]byte, []byte) {
	cmd := bytes.TrimSpace(bytes.Split(buffer, sep)[0])
	rem := bytes.TrimSpace(bytes.TrimPrefix(buffer, cmd))
	return cmd, rem
}

func (s *Server) getFileNameFromId(fileId int) string {
	for k, v := range s.fileMap {
		if v.fileID == fileId {
			return k
		}
	}
	return ""
}

func (s *Server) handleConn(conn net.Conn) {
	defer wg.Done()
	client := &Client{id: len(s.clients) + 1, conn: conn}
	r := bufio.NewReader(conn)
	client.reader = r
	s.clients[client.id] = client
	i := 0
	for {
		fmt.Println("Loop:", i)
		if i == 5 {
			break
		}
		header, err := client.reader.ReadBytes('\n')
		fmt.Println("Parsing header", string(header), len(header), string(header))

		cmd, fileId, payloadSize := s.extractHeader(header)
		fmt.Println("got cmd", cmd, "file", s.getFileNameFromId(fileId), payloadSize)
		s.handleMessage(cmd, client, fileId, payloadSize)
		if err != nil {
			fmt.Println("error", err)
			break
		}
		i += 1

	}
}

func (s *Server) extractHeader(header []byte) (RequestType, int, int) {

	headerSplit := bytes.Split(header, []byte(" "))
	fmt.Println("header time cmd:", string(header))
	switch string(bytes.TrimSpace(headerSplit[0])) {
	case "REG":
		//senderAddr:= string(bytes.TrimSpace(headerSplit[1]))
		fname := string(bytes.TrimSpace(headerSplit[2]))
		fsize, _ := strconv.Atoi(string(bytes.TrimSpace(headerSplit[3])))
		fmt.Println("Old:", s.fileMap, s)
		s.fileMap[fname] = FileInfo{fileID: s.fileCount, fname: fname, sizeInBytes: fsize}
		s.fileCount += 1
		fmt.Println("New file", s.fileMap[fname], s.fileCount, s.fileMap)
		return RegisterRequest, 0, 0
	case "REGCH":
		//senderAddr:= string(bytes.TrimSpace(headerSplit[1]))
		fname := string(bytes.TrimSpace(headerSplit[2]))
		chunkSize, _ := strconv.Atoi(string(bytes.TrimSpace(headerSplit[3])))
		fmt.Println("filename", fname, "has chunk size", string(headerSplit[3]))
		return FileChunkRequest, s.fileMap[fname].fileID, chunkSize
	case "FLIST":
		return FileListRequest, 0, 0

	}

	return NoneType, 0, 0
}
func (s Server) sendResponse(clientId int, message []byte) {
	s.clients[clientId].conn.Write(message)
}
func (s Server) handleMessage(cmd RequestType, client *Client, fileId int, payloadSize int) {
	readBuf := make([]byte, payloadSize)
	n, _ := client.reader.Read(readBuf)
	fmt.Printf("--------BUFFER START------\n%d:%s-------BUFFER END------", n, string(readBuf))
	switch cmd {
	case FileChunkRequest:
		/*
			totalRead := 0
			f, err := os.OpenFile(s.getFileNameFromId(fileId),os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				log.Println(err)
			}
			defer f.Close()
			for totalRead<payloadSize{
				content:= readBuf[:n]
				totalRead+=n
				fmt.Println("cont",string(content),s.getFileNameFromId(fileId))
				if _, err := f.Write(content); err != nil {
					log.Println(err)
				}
				fmt.Println("buffer after regch: ",totalRead,payloadSize)
				n, _ = reader.Read(readBuf)
				if n==0{
					break
				}
			}*/
	case FileListRequest:
		response := fmt.Sprintf("FLIST %d\r\n", len(s.fileMap))
		for _, file := range s.fileMap {
			response += fmt.Sprintf("%d:%s:%d ", file.fileID, file.fname, file.sizeInBytes)
		}
		fmt.Print("sending flist:", response)
		s.sendResponse(client.id, []byte(response+"\r\n"))

	}
}

var wg sync.WaitGroup

const (
	connIP   = "127.0.0.1"
	connPort = 8080
	connType = "tcp"
)

func main() {
	fmt.Printf("Welcome! The server is running at port %d\n", connPort)
	var address = fmt.Sprintf("%s:%d", connIP, connPort)
	var server Server = Server{fileMap: make(map[string]FileInfo), fileCount: 0, clients: make(map[int]*Client)}
	listener, _ := net.Listen("tcp", address)
	for {

		conn, err := listener.Accept()
		fmt.Println("new client")
		if err != nil {
			fmt.Println("Error connecting:", err.Error())
		}
		wg.Add(1)
		go server.handleConn(conn)
	}

}

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
)


type File struct{
	Filename string
	Length int
}

/*
Sample Message Format:
Register Request - REG\r\n127.8.0.1:8001 2 a.txt audio.mp3 143 746000
Register Response - REG RS\r\n a.txt SUC <file-id>
Register Response - REG RS\r\n audio.mps FAIL
REGCH <file-id>:<chunk-id>:length of message\r\n file content
File List Request - LST RQ 
File List Response - LST RS\r\n2 a.txt audio.mp3 143 746000
TODO:
LOC RQ/RS
CREG RQ/RS
FCHNK RQ/RS
*/
type RequestType int

const (
	RegisterRequest RequestType = iota 
	FileListRequest
	FileLocationsRequest
	ChunkRegisterRequest
	FileChunkRequest
)
const maxBufferSize = 1024

type FileChunk struct{
	content []byte
	fileID int
	chunkID int
}

type FileInfo struct{
	fileID int
	fname string
	sizeInBytes int
}
type Server struct{
	fileMap map[string]FileInfo
	fileCount int
}


func (s Server) handleConn(conn net.Conn){
	defer wg.Done()
	readBuf := make([]byte, maxBufferSize)
	//f, _ := os.Create("dat2.mp3")
	r := bufio.NewReader(conn)
	for {
		_, err := r.Read(readBuf)
		if err!=nil{
			fmt.Println("error",err)
			break
		}
		fmt.Println("BUFFER START------ ",string(readBuf),"-----BUFFER END")
		cmd := bytes.TrimSpace(bytes.Split(readBuf, []byte(" "))[0])
		rem := bytes.TrimSpace(bytes.TrimPrefix(readBuf, cmd))
		fmt.Println("cmd",string(cmd))
		switch string(cmd) {
		case "REG":
			fmt.Print("args:", string(rem),"\n")
			header:= bytes.Split(rem, []byte(" "))
			fname := string(bytes.TrimSpace(header[0]))
			fmt.Println("size is ",string(header[0]),string(header[1]),"A")
			fsize,err := strconv.Atoi(string(header[1]))
			if err!=nil{
				log.Println(err)
			}
			s.fileMap[fname]=FileInfo{fileID:s.fileCount,fname: fname,sizeInBytes: fsize}
			s.fileCount+=1
			fmt.Println(s.fileMap[fname])
			//send success response with file id
			continue
		case "REGCH":
			fmt.Print("REGCH message:", string(rem),"\n")
			continue
		}
		
	}
}

var wg sync.WaitGroup
const (
    connIP = "127.0.0.1"
    connPort = 8080
    connType = "tcp"
)

func main(){
	fmt.Printf("Welcome! The server is running at port %d\n",connPort)
	var address = fmt.Sprintf("%s:%d",connIP,connPort)
	var server Server = Server{fileMap:make(map[string]FileInfo),fileCount: 0}
	listener, _ := net.Listen("tcp", address)
	
	
	for{
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error connecting:", err.Error())
			return
		}
		wg.Add(1)
		go server.handleConn(conn)
		wg.Wait()
	}
	
	
}
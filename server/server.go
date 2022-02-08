package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
)


type File struct{
	Filename string
	Length int
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
const maxBufferSize = 2048

type Message struct{
	reqType RequestType
	sender	string
	body []byte
}
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

type Client struct{
	id int
	conn net.Conn
	register   chan<- *Client
}
type Server struct{
	clients []*Client
	fileMap map[string]FileInfo
	fileCount int
}

func getNextandRemaining(buffer []byte,sep []byte) ([]byte,[]byte){
	cmd := bytes.TrimSpace(bytes.Split(buffer,sep)[0])
	rem := bytes.TrimSpace(bytes.TrimPrefix(buffer, cmd))
	return cmd,rem
}

func (s Server) getFileName(fileId int )string{
	for k,v:=range s.fileMap{
		if v.fileID==fileId{
			return k
		}
	}
	return ""
}

func (s Server) handleConn(conn net.Conn){
	defer wg.Done()
	
	r := bufio.NewReader(conn)
	i:=0
	for {
		fmt.Println("Loop:",i)
		header, err := r.ReadBytes('\n')
		fmt.Println("Parsing header",string(header),len(header),string(header))
		
		cmd,fileId,payloadSize := s.extractHeader(header)
		fmt.Println("got cmd",cmd,"file",s.getFileName(fileId),payloadSize)
		if payloadSize>0{
			s.handleMessage(cmd,r,fileId,payloadSize)
		}
		if err!=nil{
			fmt.Println("error",err)
			break
		}
		i+=1
		
	}
}

func (s *Server) extractHeader(header []byte) (RequestType,int,int){
	fmt.Println("header time")
	headerSplit:=bytes.Split(header,[]byte(" "))
	switch string(bytes.TrimSpace(headerSplit[0])) {
	case "REG":
		//senderAddr:= string(bytes.TrimSpace(headerSplit[1]))
		fname := string(bytes.TrimSpace(headerSplit[2]))
		fsize,_:= strconv.Atoi(string(bytes.TrimSpace(headerSplit[3])))
		fmt.Println("Old:",s.fileMap,s)
		s.fileMap[fname]=FileInfo{fileID:s.fileCount,fname: fname,sizeInBytes: fsize}
		s.fileCount+=1
		fmt.Println("New file",s.fileMap[fname],s.fileCount,s.fileMap)
		return RegisterRequest,0,0
	case "REGCH":
		//senderAddr:= string(bytes.TrimSpace(headerSplit[1]))
		fname := string(bytes.TrimSpace(headerSplit[2]))
		chunkSize,_:= strconv.Atoi(string(bytes.TrimSpace(headerSplit[3])))
		return ChunkRegisterRequest,s.fileMap[fname].fileID,chunkSize
	}
	return NoneType,0,0
}

func (s Server) handleMessage(cmd RequestType,reader *bufio.Reader,fileId int,payloadSize int){
	
	//cmd,rem:= getNextandRemaining(readBuf,[]byte(" "))
	/*cmd:=bytes.Split(readBuf,[]byte(" "))[0]
	_,rem:=getNextandRemaining(readBuf,[]byte(" "))
	fmt.Println("cmd",string(cmd),len(cmd))*/
	readBuf := make([]byte, payloadSize)
	n, _ := reader.Read(readBuf)
	fmt.Printf("--------BUFFER START------\n%d:%s-------BUFFER END------",n,string(readBuf))
	switch cmd {
	case ChunkRegisterRequest:
		//fmt.Print("REGCH message:", string(rem))
		//header,rem:= getNextandRemaining(rem,[]byte(" "))//bytes.Split(rem, []byte(" "))
		//headerSplit:= bytes.Split(header,[]byte(":"))
		//fname:=string(bytes.TrimSpace(headerSplit[0]))
		/*contentsize,err := strconv.Atoi(string(bytes.TrimSpace(headerSplit[1])))
		if err!=nil{
			log.Println(err)
		}*/
		content:= readBuf[:payloadSize]
		fmt.Println("cont",string(content),s.getFileName(fileId))
		f, err := os.OpenFile(s.getFileName(fileId),os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Println(err)
		}
		defer f.Close()
		if _, err := f.Write(content); err != nil {
			log.Println(err)
		}
		defer f.Close()
		fmt.Println("buffer after regch: ",string(readBuf))
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
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"strconv"
	"sync"
)

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
//const maxBufferSize = 2048

type Message struct {
	reqType ProtocolMessageType
	sender  string
	body    []byte
}
type FileChunk struct {
	content []byte
	fileID  int
	chunkID int
	chunkSize int
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
	chunksStored int
	conn     net.Conn
	reader   *bufio.Reader
	addr     net.Addr
	register chan<- *Client
	d *json.Decoder
}
type Server struct {
	clients   map[int]*Client
	fileMap   map[int]*FileInfo
	fileNametoId map[string]int
	fileCount int
}

type RequestData struct{
	FileId int `json:"file_id,omitempty"`
	Fname string `json:"file_name,omitempty"`
	PayloadSize int `json:"payload_size,omitempty"`
	ChunkId int `json:"chunk_id,omitempty"`
}
type MessageType int
const (
	RequestMessage MessageType = iota
	ResponseMessage
)

//all the properties don't have to be set/ varies between request types
type HeaderData struct{
	mtype MessageType
	cmd ProtocolMessageType
	fileId int 
	fname string
	payloadSize int
	chunkId int
}

func (s *Server) handleConn(conn net.Conn) {
	defer wg.Done()
	client := &Client{id: len(s.clients) + 1, conn: conn, d : json.NewDecoder(conn)}
	r := bufio.NewReader(conn)
	client.reader = r
	s.clients[client.id] = client
	i := 0
	for {
		fmt.Println("Loop:", i)
		header, err := client.reader.ReadBytes('\n')
		fmt.Println("Parsing header", string(header), len(header), string(header))

		headerData := s.extractHeader(header)
		//fmt.Println("got cmd", cmd, "file", s.getFileNameFromId(fileId), payloadSize)
		s.handleMessage(client,headerData)
		if err != nil {
			fmt.Println("error", err)
			break
		}
		i += 1

	}
}

func getPayload(payloadSize int, client *Client)([]byte,error){
	readBuf := make([]byte, payloadSize)
	n, err := client.reader.Read(readBuf)
	fmt.Printf("--------BUFFER START------\n%d:%s-------BUFFER END------\n", n, string(readBuf))
	if err!=nil{
		return nil,err
	}
	return readBuf,nil
}

func (s *Server) extractHeader(header []byte) (HeaderData) {
	headerData := HeaderData{}
	headerSplit := bytes.Split(header, []byte(" "))
	fmt.Println("----Received Header:", string(header))
	mtype,_ :=strconv.Atoi(string(bytes.TrimSpace(headerSplit[1])))
	headerData.mtype = MessageType(mtype)
	switch string(bytes.TrimSpace(headerSplit[0])) {
	case "REG":
		fname := string(bytes.TrimSpace(headerSplit[3]))
		fsize, _ := strconv.Atoi(string(bytes.TrimSpace(headerSplit[4])))
		newFid := s.fileCount
		s.fileMap[newFid] = &FileInfo{fileID:newFid,fname: fname, sizeInBytes: fsize}
		s.fileNametoId[fname]=newFid
		s.fileCount += 1
		fmt.Println("---New fileMap---", s.fileMap[newFid], s.fileCount, s.fileMap)
		headerData.cmd = RegisterRequest
		headerData.fileId = newFid
	case "FCHNK":
		fname := string(bytes.TrimSpace(headerSplit[3]))
		chunkSize, _ := strconv.Atoi(string(bytes.TrimSpace(headerSplit[4])))
		fmt.Println("filename", fname, "has chunk size", string(headerSplit[4]))
		headerData.cmd = FileChunkRequest
		headerData.fileId=s.fileMap[s.fileNametoId[fname]].fileID
		headerData.payloadSize = chunkSize
	case "FLIST":
		headerData.cmd = FileListRequest
	case "CHREG":
		//CHREG fileid chunkid chunksize
		fileId, _ := strconv.Atoi(string(bytes.TrimSpace(headerSplit[2])))
		chunkId, _ := strconv.Atoi(string(bytes.TrimSpace(headerSplit[3])))
		chunkSize, _ := strconv.Atoi(string(bytes.TrimSpace(headerSplit[4])))
		headerData.cmd = ChunkRegisterRequest
		headerData.fileId = fileId
		headerData.chunkId = chunkId
		headerData.payloadSize = chunkSize
	}

	return headerData
}

func (s Server) sendResponse(client *Client, message []byte) {
	client.conn.Write(message)
	fmt.Println("wrote response",message)
}

func (s Server) handleMessage(client *Client, header HeaderData) {
	cmd,payloadSize := header.cmd, header.payloadSize
	readBuf,err:=getPayload(payloadSize,client)
	if err!=nil{
		log.Println(err)
	}
	if header.mtype == ResponseMessage{
		return
	}
	switch cmd {
	case RegisterRequest:
		response:=fmt.Sprintf("REG %d",header.fileId)
		fmt.Println("sending response",response)
		s.sendResponse(client,[]byte(response))

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
		log.Print("sending flist:", response)
		s.sendResponse(client, []byte(response+"\r\n"))
	
	case ChunkRegisterRequest:
		s.handleChunkRegister(header,readBuf)
	}
}

func (s *Server) handleChunkRegister(header HeaderData,payload []byte,){
	fchunk:= FileChunk{
		fileID: header.fileId,
		content: payload,
		chunkID: header.chunkId,
		chunkSize: header.payloadSize,
	}
	client:=s.getClientWithLowestChunks()
	s.sendChunkReceiverData(client,&fchunk)
}

func (s *Server) sendChunkReceiverData(client *Client, fileChunk *FileChunk) {
	//REGGCHNK fileid chunkid chunksize\r\npayload\r\n
	fileId,chunkId:=fileChunk.fileID,fileChunk.chunkID
	message:=fmt.Sprintf("CHREG %d %d %d\r\n",fileId,chunkId,fileChunk.chunkSize)
	s.sendResponse(client,[]byte(message))
	client.chunksStored+=1
	s.fileMap[fileId].chunkLocations[chunkId] = append(s.fileMap[fileId].chunkLocations[chunkId],client)
	s.fileMap[fileId].totalChunks+=1
}

func (s *Server) getClientWithLowestChunks() *Client{
	var minClient *Client
	minChunks := math.MaxInt32
	for _,c := range s.clients{
		if c.chunksStored<minChunks{ 
			minChunks = c.chunksStored
			minClient = c
		}
	}
	return minClient
}

func decodeJson(){

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
	var server Server = Server{fileMap: make(map[int]*FileInfo),fileNametoId: make(map[string]int), fileCount: 0, clients: make(map[int]*Client)}
	listener, err := net.Listen("tcp", address)
	if err!=nil{
		log.Fatal(err)
	}
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

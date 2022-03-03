package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"sync"
)

type RequestType int
const (
	Register RequestType = iota
	FileList
	FileLocations
	ChunkRegister
	FileChunk
)

type FileChunkInfo struct {
	content []byte
	fileID  int
	chunkID int
	chunkSize int
}

type FileInfo struct {
	fileID         int
	fname          string
	sizeInBytes    int
	 //chunks 0 to totalChunks-1 will be present as keys in chunkLocations
	totalChunks    int              
	 //map chunkid to list of clients containing
	chunkLocations map[int][]*Client
	//SHA256 hash of entire file
	fileHash []byte
}

type Client struct {
	id       int
	chunksStored int
	conn     net.Conn
	reader   *bufio.Reader
	addr     net.Addr
	register chan<- *Client
	decoder *json.Decoder
	encoder *json.Encoder
	mu sync.Mutex
}
type Server struct {
	clients   map[int]*Client
	fileMap   map[int]*FileInfo
	fileNametoId map[string]int
	fileCount int
}
type JSONMessage struct{
	Mtype MessageType `json:"message_type"`
	Rtype RequestType `json:"req_type"`
	Addr string `json:"addr"`
	File string `json:"file,omitempty"`
	FileSize int `json:"file_size,omitempty"`
	FileId int `json:"file_id,omitempty"`
	ChunkSize int `json:"chunk_size,omitempty"`
	ChunkId int `json:"chunk_id,omitempty"`
	Body []byte `json:"body,omitempty"`
	Status ResponseStatus `json:"status,omitempty"`
}

type ResponseStatus int 
const (
	Success ResponseStatus = iota
	Err
)

type MessageType string
const (
	RequestMessage MessageType = "Request"
	ResponseMessage MessageType = "Response"
)

type FileLocation struct{
	FileId int `json:"file_id"`
	TotalChunks int `json:"total_chunks"`
	ChunkLocations map[int][]string `json:"chunk_locations"`
	FileHash []byte `json:"file_hash"`
} 

func (s *Server) handleConn(conn net.Conn) {
	client := &Client{
		id: len(s.clients) + 1, 
		conn: conn, 
		addr: conn.RemoteAddr(),
		decoder : json.NewDecoder(conn),
		encoder : json.NewEncoder(conn),
	}
	fmt.Println("Client id:",client.id,"has joined -",client.addr.String())
	r := bufio.NewReader(conn)
	client.reader = r
	s.clients[client.id] = client
	for {
		var jsonReq JSONMessage
		err := client.decoder.Decode(&jsonReq)
		if err != nil {
			return
		}
		s.handleMessage(client,jsonReq)
		if err != nil {
			fmt.Println("error", err)
			break
		}

	}
}

func (s *Server) sendResponse(client *Client, resp JSONMessage) {
	client.encoder.Encode(resp)
	fmt.Println("wrote response",resp.Addr,resp.Mtype,resp.Rtype,resp.ChunkId,resp.ChunkSize,resp.FileId)
}

func (s *Server) handleMessage(client *Client, req JSONMessage) {
	cmd := req.Rtype
	if req.Mtype == ResponseMessage{
		return
	}
	switch cmd {
	case Register:
		newFid := s.fileCount
		s.fileMap[newFid] = &FileInfo{
			fileID:newFid,
			fname: req.File, 
			sizeInBytes: req.FileSize,
			chunkLocations: make(map[int][]*Client),
			totalChunks: 0,
			fileHash: req.Body,
		}
		s.fileNametoId[req.File]=newFid
		s.fileCount += 1
		var response JSONMessage = JSONMessage{Mtype: ResponseMessage, Rtype: Register, FileId: newFid, Status: Success}
		client.encoder.Encode(response)
	case FileList:
		response:=""
		for id, file := range s.fileMap {
			response += fmt.Sprintf("%d:%s:%d ", id, file.fname, file.sizeInBytes)
		}
		var resp JSONMessage = JSONMessage{Mtype: ResponseMessage, Rtype: FileList,Body : []byte(response)}
		client.encoder.Encode(resp)
	
	case ChunkRegister:
		newclient:=s.getClientWithLowestChunks()
		req.Addr = newclient.addr.String()
		s.sendChunkReceiverData(client,newclient,req)

	case FileLocations:
		var response JSONMessage = JSONMessage{
			Mtype: ResponseMessage,
			Rtype: FileLocations,
		}
		fileInfo,ok:= s.fileMap[req.FileId] 
		if !ok{
			response.Status = Err 
			client.encoder.Encode(response)
			return
		}
		fileLocations := FileLocation{
			FileId: req.FileId,
			TotalChunks: fileInfo.totalChunks,
			ChunkLocations: s.getChunkLocationMap(fileInfo),
			FileHash: s.fileMap[req.FileId].fileHash,
		}
		body,err := json.Marshal(fileLocations)
		if err!=nil{
			fmt.Println("Marshal error")
		}
		response.Body = body
		client.encoder.Encode(response)
		
	}
}
//getChunkLocationMap: organizes chunk locations of each file chunk into a map that can be easily encoded into JSON
func (s *Server) getChunkLocationMap(fileInfo *FileInfo) map[int][]string{
	//map stores key value pairs of chunk id and a list of string addresses of peers storing that chunk
	locations:= make(map[int][]string)
	chunkId:=0
	for chunkId < fileInfo.totalChunks{
		chunkLocations:= []string{}
		for _,peer := range(fileInfo.chunkLocations[chunkId]){
			chunkLocations = append(chunkLocations, peer.addr.String())
		}
		locations[chunkId] = chunkLocations
		chunkId+=1
		
	}
	return locations
}

//sendChunkReceiverData: stores chunklocation for the newly registered chunk and responds to client
func (s *Server) sendChunkReceiverData(client *Client,newclient *Client, message JSONMessage){
	client.mu.Lock()
	defer client.mu.Unlock() 
	fileId,chunkId:=message.FileId, message.ChunkId
	newclient.chunksStored+=1
	s.fileMap[fileId].chunkLocations[chunkId] = append(s.fileMap[fileId].chunkLocations[chunkId],newclient)
	s.fileMap[fileId].totalChunks+=1
	message.Mtype=ResponseMessage
	client.encoder.Encode(message)
}

//getClientWithLowestChunks: chunks are sent to clients with lowest chunks so that chunks are distributed equally
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
		if err != nil {
			fmt.Println("Error connecting:", err.Error())
		}
		go server.handleConn(conn)
	}

}

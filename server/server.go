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
	FileContent
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

/*
ChunksLocations: {1:["127.0.9.0:2344","128.0.9.0:1111"],2:["56.13.87.90"]}

*/

type FileLocation struct{
	FileId int `json:"file_id"`
	TotalChunks int `json:"total_chunks"`
	ChunkLocations map[int][]string `json:"chunk_locations"`
} 

func (s *Server) handleConn(conn net.Conn) {
	defer wg.Done()
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
		fmt.Println(jsonReq)
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
	fmt.Println("got cmd",cmd)
	switch cmd {
	case Register:
		newFid := s.fileCount
		s.fileMap[newFid] = &FileInfo{
			fileID:newFid,
			fname: req.File, 
			sizeInBytes: req.FileSize,
			chunkLocations: make(map[int][]*Client),
			totalChunks: 0,
		}
		s.fileNametoId[req.File]=newFid
		s.fileCount += 1
		req.Mtype = ResponseMessage
		req.FileId = newFid
		s.sendResponse(client,req)
	case FileList:
		response:=""
		for id, file := range s.fileMap {
			response += fmt.Sprintf("%d:%s:%d ", id, file.fname, file.sizeInBytes)
		}
		var resp JSONMessage = JSONMessage{Mtype: ResponseMessage, Rtype: FileList,Body : []byte(response)}
		log.Print("sending flist:", resp)
		s.sendResponse(client,resp)
	
	case ChunkRegister:

		newclient:=s.getClientWithLowestChunks()
		fmt.Println("newclient",newclient.conn.RemoteAddr().String())
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
			s.sendResponse(client,response)
			return
		}
		fileLocations := FileLocation{
			FileId: req.FileId,
			TotalChunks: fileInfo.totalChunks,
			ChunkLocations: s.getChunkLocationMap(fileInfo),
		}
		body,err := json.Marshal(fileLocations)
		if err!=nil{
			fmt.Println("Marshal error")
		}
		fmt.Println("file locations body",string(body))
		response.Body = body
		s.sendResponse(client,response)
		
	}
}

func (s *Server) getChunkLocationMap(fileInfo *FileInfo) map[int][]string{
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
	fmt.Println("Final chunk locations for file",locations)
	return locations
}
func (s *Server) sendChunkReceiverData(client *Client,newclient *Client, message JSONMessage){// fileChunk *FileChunkInfo) {
	client.mu.Lock()
	defer client.mu.Unlock() 
	fileId,chunkId:=message.FileId, message.ChunkId
	newclient.chunksStored+=1
	s.fileMap[fileId].chunkLocations[chunkId] = append(s.fileMap[fileId].chunkLocations[chunkId],newclient)
	s.fileMap[fileId].totalChunks+=1
	fmt.Println("chunk send to",newclient.id,newclient.addr.String())
	message.Mtype=ResponseMessage
	s.sendResponse(client,message)
}

func (s *Server) getClientWithLowestChunks() *Client{
	var minClient *Client
	minChunks := math.MaxInt32
	for _,c := range s.clients{
		fmt.Println("client:",c.id,"chunks:",c.chunksStored)
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
		wg.Add(1)
		go server.handleConn(conn)
	}

}

package main

import (
	"bufio"
	"fmt"
	"strconv"

	//"ioutil"

	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
)

type Peer struct {
	peerId     int
	addr       net.Addr
	conn       net.Conn
	chunkStore *ChunkStore
	mu sync.Mutex
	decoder *json.Decoder
	encoder *json.Encoder
}

type ChunkStore struct {
	fileChunks map[int]*FileChunks //map file id to filechunk struct
	chunksStored int //for verification purposes
}

type FileChunks struct{
	chunks map[int]*Chunk //map chunk id to actual chunk
	mu sync.Mutex
}

type Chunk struct {
	fileId  int
	size    int
	chunkId int //used to reconstruct files from a collection of chunks
	data    []byte
}

type FileLocation struct{
	FileId int `json:"file_id"`
	TotalChunks int `json:"total_chunks"`
	ChunkLocations map[int][]string `json:"chunk_locations"`
} 

type MessageType string
const (
	RequestMessage MessageType = "Request"
	ResponseMessage MessageType = "Response"
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

const (
	serverIP      = "127.0.0.1"
	serverPort    = "8080"
	maxBufferSize = 8192//1048576
	maxChunkSize  = 8192//1048576 //1MB
)

var wg sync.WaitGroup

func displayHelpPrompt() {
	fmt.Print("Welcome to Neon Peer CLI!!\nUse the following commands to view/download files:\n\t1) list - list all files available to download \n\t2) download <file-id> - download file from network \n\t3) upload <filepath> - upload file to network\n\t4) progress - view active download progress\n\t5) help - view available commands\n")
}

func printRepl() {
	fmt.Print("neon> ")
}
func printUnknown(txt string) {
	fmt.Println(txt, ": command not found")
}

func stripInput(txt string) string {
	output := strings.TrimSpace(txt)
	output = strings.ToLower(output)
	return output
}

func (p *Peer) SendMessage(message JSONMessage){//rType RequestType, header string, payload []byte) error {
	if err := p.encoder.Encode(message); err != nil {
		fmt.Println("Encode error: ", err)
	}
}

func (p *Peer) ReceiveMessage(response *JSONMessage){
	if err:= p.decoder.Decode(&response); err!=nil{
		fmt.Println("Server response decode error",err)
	}
}

func (p *Peer) readShareFile(filepath string) {
	f, err := os.Open(filepath)
	if err != nil {
		log.Printf("unable to read file: %v", err)
		return
	}
	defer f.Close()
	fstat, _ := f.Stat()
	buf := make([]byte, maxBufferSize)
	message:= JSONMessage{
		Mtype: RequestMessage,
		Rtype: Register,
		File: getFileName(filepath),
		FileSize: int(fstat.Size()),
		Addr: p.addr.String(),
	}
	if err := p.encoder.Encode(message); err != nil {
		fmt.Println("Encode error: ", err)
	}
	var resp JSONMessage
	//fmt.Println("req:",message) 
	p.decoder.Decode(&resp)
	//fmt.Println("resp:",resp)
	if err!=nil{
		panic(err)
	}
	chunkId := 0
	message.FileId = resp.FileId
	for {
		n, err := f.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println(err)
			continue
		}

		if n > 0 {
			message.Rtype = ChunkRegister
			message.ChunkId = chunkId
			message.Body = buf[:n]
			message.ChunkSize = n
			p.SendMessage(message)
			var response JSONMessage
			p.ReceiveMessage(&response)
			//fmt.Println("got response",response)
			if response.Addr==p.addr.String(){
				fmt.Println("storing chunk in self")
				p.storeChunk(message)
			} else{
				p.sendPeerMessage(response,response.Addr)
			}
			chunkId += 1
		} else {
			break
		}
	}

}

func getFileName(path string) string { return filepath.Base(path) }

func NewPeer() (*Peer,error){
	conn, err := net.Dial("tcp", serverIP+":"+serverPort)
	if err != nil {
		fmt.Printf("Server is not online - %v\n", err)
		return nil,err
	}
	fmt.Println("HI MY ADDRESS IS",conn.LocalAddr().String())
	return &Peer{
		addr: conn.LocalAddr(),
		chunkStore: &ChunkStore{fileChunks: make(map[int]*FileChunks),chunksStored: 0},
		conn:conn,
		decoder: json.NewDecoder(conn),
		encoder: json.NewEncoder(conn),
	},nil
}

func (p *Peer) downloadFile(fileId int) {
	//request chunks, peerid to addr
	var downloadWg sync.WaitGroup
	var fileLocReq JSONMessage = JSONMessage{
		Mtype: RequestMessage,
		Rtype: FileLocations,
		FileId: fileId,
	}
	p.SendMessage(fileLocReq)
	var fileLocRes JSONMessage
	var fileLocations FileLocation
	p.ReceiveMessage(&fileLocRes)
	if fileLocRes.Status==Success{
		json.Unmarshal(fileLocRes.Body,&fileLocations)
		fmt.Println("got file locations",fileLocations)
	} else {
		fmt.Println("Couldn't get file location")
		return 
	}
	fileChunks := FileChunks{chunks:make(map[int]*Chunk)}
	// for each chunk spawn a go routine that makes a peer request  with file chunk request
	// once number of received chunks is equal to total chunks for the file, reconstruct the file and save it
	for chunkid:=0;chunkid<fileLocations.TotalChunks;chunkid+=1{
		for _,peerAddr:= range(fileLocations.ChunkLocations[chunkid]){
			//if you hold the required chunk just add it to file chunks otherwise initiate request
			fmt.Println("chunkid 1",chunkid)
			if peerAddr == p.addr.String(){
				fileChunks.chunks[chunkid] = &Chunk{data: p.chunkStore.fileChunks[fileId].chunks[chunkid].data}
			} else {
				downloadWg.Add(1)
				fmt.Println("chunkid 2",chunkid)
				x:=chunkid //x is the new local closed over value
				go func(id int){
					fmt.Println("chunkid 3",chunkid)
					defer downloadWg.Done()
					p.getChunkFromPeer(peerAddr,fileId,x,&fileChunks)
				}(chunkid)
			}
			break
		}
	} 
	downloadWg.Wait()
	fmt.Println(fileChunks.chunks)
	p.reconstructFile(&fileChunks,fileLocations.TotalChunks)
}

func (peer *Peer) reconstructFile(fileChunks *FileChunks,totalChunks int){
	f, err := os.OpenFile("download.txt",os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err!=nil{
		log.Println(err)
	}
	defer f.Close()
	fmt.Println("filechunks",fileChunks.chunks,totalChunks)
	for chunkId:=0;chunkId<totalChunks;chunkId+=1{
		var content []byte = fileChunks.chunks[chunkId].data
		if _, err := f.Write(content); err != nil {
			log.Println("write err",err)
		}
	}
}

func (peer *Peer) getChunkFromPeer(addr string,fileId int,chunkId int,fileChunks *FileChunks)error{//,initiatedDownloads map[string]bool) error{
	var request JSONMessage = JSONMessage{
		Mtype: RequestMessage,
		Rtype: FileChunk,
		FileId: fileId,
		ChunkId: chunkId,
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Printf("Peer is not online - %v\n", err)
		return err
	}
	var response JSONMessage
	//initiatedDownloads[addr]=true
	encoder,decoder := json.NewEncoder(conn), json.NewDecoder(conn)
	if err := encoder.Encode(request); err != nil {
		fmt.Println("Encode error: ", err)
		return err
	}
	if err:= decoder.Decode(&response); err!=nil{
		fmt.Println("Encode error: ", err)
		return err
	}
	fmt.Printf("got chunk from peer: %+v\n",response)
	//initiatedDownloads[addr]=false
	fileChunks.chunks[chunkId]=&Chunk{data:response.Body,fileId: fileId, chunkId: chunkId}
	return nil
}

func (peer *Peer) sendPeerMessage(message JSONMessage,addr string)error{
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Printf("Peer is not online - %v\n", err)
		return err
	}
	encoder:= json.NewEncoder(conn)
	if err := encoder.Encode(message); err != nil {
		fmt.Println("Encode error: ", err)
	}
	return nil
}

func (peer *Peer) storeChunk(message JSONMessage) bool{
	if _,ok:= peer.chunkStore.fileChunks[message.FileId]; !ok{ //peer doesn't have any chunks from this file
		peer.chunkStore.fileChunks[message.FileId] = &FileChunks{chunks:make(map[int]*Chunk)}
	}
	
	
	peer.chunkStore.fileChunks[message.FileId].chunks[message.ChunkId] = &Chunk{
		fileId: message.FileId,
		size: message.ChunkSize,
		chunkId: message.ChunkId,
		data: message.Body,
	}
	peer.chunkStore.chunksStored+=1
	//fmt.Println(string(peer.chunkStore.fileChunks[message.FileId].chunks[message.ChunkId].data))
	fmt.Println("got chunk with size",message.ChunkSize,"id:",message.ChunkId)
	fmt.Println("CHUNKS STORED:",peer.chunkStore.chunksStored)
	printRepl()
	return true
}

func (p *Peer) listFiles() {
	//make a FileList request to server
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)
	req := JSONMessage{
		Mtype: RequestMessage,
		Rtype: FileList,
	}
	p.SendMessage(req)
	message:=&JSONMessage{}
	err := p.decoder.Decode(message)
	if err!=nil{
		fmt.Println("Docoding error",err)
	}
	if string(message.Body) == "" {
		fmt.Println("No files present in the network")
		return
	}
	splitFn := func(c rune) bool {
        return c == ' '
	}
	bodySplit := strings.FieldsFunc(string(message.Body),splitFn)
	fmt.Println("Files present in the network:", len(bodySplit),message)
	fmt.Fprintln(writer, "FileID\tFilename\tSize(in bytes)")
	for _, fileData := range bodySplit {
		split:=strings.Split(fileData,":")
		if len(split) > 0 {
			fileId, fName, size := split[0], split[1], split[2]
			displayStr := fmt.Sprintf("%s\t%s\t%s", string(fileId), string(fName), string(size))
			fmt.Fprintln(writer, displayStr)
		}
	}
	writer.Flush()

}

func (peer *Peer) displayProgress() {
	var wg sync.WaitGroup
	p := mpb.New(mpb.WithWaitGroup(&wg))
	var total int64 = 100
	numBars := 6
	wg.Add(numBars)
	for i := 0; i < numBars; i++ {
		name := fmt.Sprintf("Bar#%d:", i)
		bar := p.AddBar(total,
			mpb.PrependDecorators(
				decor.Name(name),
				decor.OnComplete(decor.EwmaETA(decor.ET_STYLE_GO, 60, decor.WCSyncWidth), "done"),
			),
			mpb.AppendDecorators(decor.Counters(decor.UnitKiB, "% .1f / %d")),
		)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))
			max := 100 * time.Millisecond
			for i := 0; i < int(total); i++ {
				// start variable is solely for EWMA calculation
				// EWMA's unit of measure is an iteration's duration
				start := time.Now()
				time.Sleep(time.Duration(rng.Intn(10)+1) * max / 10)
				bar.Increment()
				// we need to call DecoratorEwmaUpdate to fulfill ewma decorator's contract
				bar.DecoratorEwmaUpdate(time.Since(start))
			}
		}()
	}
	p.Wait()
}

func (peer *Peer) handleChunkRequests(conn net.Conn,message JSONMessage){
	defer wg.Done()
	switch message.Rtype{
	case ChunkRegister:
		success:= peer.storeChunk(message)
		if success{
			response:= &JSONMessage{
				Mtype: ResponseMessage,
				Rtype: ChunkRegister,
				Status:Success,
				FileId: message.FileId,
				ChunkId: message.ChunkId,
			}
			json.NewEncoder(conn).Encode(response)
		}
	case FileChunk:
		response:= &JSONMessage{
			Mtype: ResponseMessage,
			Rtype: FileChunk,
			Status:Success,
		}
		fileid,chunkid:= message.FileId,message.ChunkId
		fileChunks,ok:=peer.chunkStore.fileChunks[fileid]
		if !ok{
			fmt.Println("no file found")
			response.Status = Err
		} else{
			chunk,ok:=fileChunks.chunks[chunkid]
			if !ok{ 
				fmt.Println("no chunk found")
				response.Status = Err
			} else {
				fmt.Println("sending body",string(chunk.data))
				printRepl()
				response.Body = chunk.data
			}
		}
		json.NewEncoder(conn).Encode(response)
	}
}

func (peer *Peer) listenForPeerRequests(){
	listener, err := net.Listen("tcp", peer.addr.String())
	if err!=nil{
		log.Fatalf("Cannot send/receive chunks from this peer\n")
	}
	for {
		conn, err := listener.Accept()
		fmt.Println("new peer")
		if err != nil {
			fmt.Println("Error connecting:", err.Error())
		}
		wg.Add(1)
		decoder := json.NewDecoder(conn)
		var message JSONMessage
		if err := decoder.Decode(&message);err!=nil{
			log.Println("error decoding peer message")
		}
		go peer.handleChunkRequests(conn,message)
	}
}


func main() {
	selfPeer,err := NewPeer() 
	if err!=nil{
		return
	}
	args := os.Args
	if len(args) > 1 {
		i := 1
		for _, file := range args[i:] {
			selfPeer.readShareFile(file)
		}
	}
	reader := bufio.NewScanner(os.Stdin)
	displayHelpPrompt()
	go selfPeer.listenForPeerRequests()
	printRepl()
	for reader.Scan() {
		text := strings.Fields(stripInput(reader.Text()))
		switch text[0] {
		case "help":
			displayHelpPrompt()
		case "download":
			if len(text)>2{ 
				fmt.Println("Cannot download more than 1 file")
				printRepl()
				continue
			}
			file_id,err := strconv.Atoi(text[1])
			if err!=nil{
				fmt.Println("Invalid file ID")
				printRepl()
				continue
			}
			selfPeer.downloadFile(file_id)
		case "list":
			selfPeer.listFiles()
		case "progress":
			selfPeer.displayProgress()
		case "upload":
			selfPeer.readShareFile(text[1])
		default:
			printUnknown(text[0])
		}
		printRepl()
	}
}
package main

import (
	"bufio"
	"bytes"
	"fmt"

	//"ioutil"

	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
)

type Peer struct {
	peerId     int
	addr       net.TCPAddr
	conn       net.Conn
	fileChunks *ChunkStore
	writer          *bufio.Writer
	reader          *bufio.Reader
	sync.Mutex
}

type ChunkStore struct {
	fileToChunkId map[int][]int
}

type Chunk struct {
	fileId  int
	size    int
	chunkId int //used to reconstruct files from a collection of chunks
	data    []byte
}

type MessageType int
const (
	RequestMessage MessageType = iota
	ResponseMessage
)

type RequestType int
const (
	RegisterRequest RequestType = iota
	FileContentRequest
	FileListRequest
	FileLocationsRequest
	ChunkRegisterRequest
	FileChunkRequest
)

const (
	serverIP      = "127.0.0.1"
	serverPort    = "8080"
	maxBufferSize = 1048576
	maxChunkSize  = 1048576 //1MB
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

func (p *Peer) SendMessage(rType RequestType, header string, payload []byte) error {
	switch rType {
	case RegisterRequest:
		header = fmt.Sprintf("REG %d %s %s",RequestMessage, p.addr.String(), header)
		break
	case FileChunkRequest:
		header = fmt.Sprintf("FCHNK %d %s %s", RequestMessage,p.addr.String(), header)
		break
	case FileListRequest:
		header = fmt.Sprintf("FLIST %d", RequestMessage)
		break
	case ChunkRegisterRequest:
		//CHREG fileid chunkid chunksize
		header = fmt.Sprintf("CHREG %d %s",RequestMessage, header)
		break
	}
	message := fmt.Sprintf("%s\r\n%s", header, string(payload))
	fmt.Println("sending", string(message))
	_, err := p.conn.Write([]byte(message))
	if err != nil {
		log.Println(err)
	}
	err = p.writer.Flush()
	if err!=nil{
		log.Println(err)
	}
	return nil
}

func (p *Peer) readShareFile(filepath string) {
	f, err := os.Open(filepath)
	if err != nil {
		log.Printf("unable to read file: %v", err)
		return
	}
	defer f.Close()
	fstat, _ := f.Stat()
	message := fmt.Sprintf("%s %s", getFileName(filepath), strconv.Itoa(int(fstat.Size())))
	buf := make([]byte, maxBufferSize)
	p.SendMessage(RegisterRequest, message, []byte(""))
	fmt.Println("here1")
	//_, _, err = p.reader.ReadLine()
	//body,_, err := p.reader.ReadLine()
	mess, err := bufio.NewReader(p.conn).ReadString('\n')
    fmt.Print("Message from server: "+mess)
	fmt.Println("here2")
	if err!=nil{
		panic(err)
	}
	fileid:=bytes.Split([]byte(mess),[]byte(" "))[1]
	fmt.Println("GOT fileid",fileid)
	i := 0
	for {
		n, err := f.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println(err)
			continue
		}
		println(i, n, buf)
		if n > 0 {
			content := fmt.Sprintf("%d %d %d", fileid, i, n)
			err = p.SendMessage(ChunkRegisterRequest, content, buf[:n])
			if err != nil {
				fmt.Println(err)
				break
			}
			i += 1
		} else {
			break
		}
	}

}

func getFileName(path string) string { return filepath.Base(path) }

func NewPeer(ip string,portStr string) (*Peer,error){
	port, err := strconv.Atoi(portStr)
	if err != nil {
		fmt.Println("Can't use port", port)
		return nil,err
	}
	conn, err := net.Dial("tcp", serverIP+":"+serverPort)
	if err != nil {
		fmt.Printf("Server is not online - %v\n", err)
		return nil,err
	}
	return &Peer{
		addr: net.TCPAddr{
			IP: net.ParseIP(ip),
			Port:port,
		},
		conn:conn,
		writer:bufio.NewWriter(conn),
		reader:bufio.NewReader(conn),
	},nil
}
// ./peer/peer1/peer localhost 8001 peer/peer1/a.txt peer/peer1/sample.txt

func (p *Peer) downloadFile(file string) {
	//request chunks, peerid to addr
}

func sendChunk(peer *Peer){

}

func (p *Peer) listFiles() {
	//make a FileList request to server
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)
	p.SendMessage(FileListRequest, "", []byte(""))
	//read response from buffer terminated by \n into buffered reader
	_, _, err := p.reader.ReadLine()
	body, _, err := p.reader.ReadLine()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("got response body:",string(body))
	bodySplit := bytes.Split(body, []byte(" "))
	bodySplit = bodySplit[:len(bodySplit)-1]

	if len(bodySplit) == 0 {
		fmt.Println("No files present in the network")
		return
	}
	fmt.Println("Files present in the network:", len(bodySplit))
	fmt.Fprintln(writer, "FileID\tFilename\tSize(in bytes)")
	for _, filedata := range bodySplit {
		filedata = bytes.Trim(filedata, "")
		if len(filedata) > 0 {
			fileId, fName, size := bytes.Split(filedata, []byte(":"))[0], bytes.Split(filedata, []byte(":"))[1], bytes.Split(filedata, []byte(":"))[2]
			displayStr := fmt.Sprintf("%s\t%s\t%s", string(fileId), string(fName), string(size))
			fmt.Fprintln(writer, displayStr)
		}
	}
	writer.Flush()

}

func displayProgress() {
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

func (peer *Peer) handleChunkRequests(conn net.Conn){
	defer wg.Done()
}

func (peer *Peer) listenForPeerRequests(){
	listener, err := net.Listen("tcp", peer.addr.String())
	if err!=nil{
		log.Println("Cannot send chunks from this peer")
	}
	for {
		conn, err := listener.Accept()
		fmt.Println("new client")
		if err != nil {
			fmt.Println("Error connecting:", err.Error())
		}
		wg.Add(1)
		go peer.handleChunkRequests(conn)
	}
}


func main() {
	args := os.Args[1:]
	selfPeer,err := NewPeer(args[0],args[1]) 
	if err!=nil{
		return
	}
	if len(args) >= 3 {
		i := 2
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
			selfPeer.downloadFile(text[1])
		case "list":
			selfPeer.listFiles()
		case "progress":
			displayProgress()
		case "upload":
			selfPeer.readShareFile(text[1])
		default:
			printUnknown(text[0])
		}
		printRepl()
	}
}
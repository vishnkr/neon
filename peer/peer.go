package main

import (
	"bufio"
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
	"time"

	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
)

type Peer struct{
	peerId int
	addr net.TCPAddr
	conn net.Conn
	w    *bufio.Writer
    sync.Mutex
}

type RequestType int
const (
	RegisterRequest RequestType = iota
	FileContentRequest
	FileListRequest
	FileLocationsRequest
	ChunkRegisterRequest
	FileChunkRequest
)

func displayHelpPrompt(){
	fmt.Print("Welcome to Neon Peer CLI!!\nUse the following commands to view/download files:\n\t1) list - list all files available to download \n\t2) download <file-id> - download file from network \n\t3) progress - view active download progress\n\t4) help - view available commands\n")
}

func printRepl(){
	fmt.Print("neon> ")
}
func printUnknown(txt string){
	fmt.Println(txt, ": command not found")
}

func stripInput(txt string)string {
	output := strings.TrimSpace(txt)
    output = strings.ToLower(output)
    return output
}

func performCmd(){
	print("Sf\n")
}

const (
	serverIP ="127.0.0.1"
	serverPort = "8080"
	maxBufferSize = 2048
)


func (p *Peer) SendMessage(rType RequestType, header string,payload []byte) error{
	var cmd string
	switch rType{
	case RegisterRequest:
		cmd = "REG"
		header = fmt.Sprintf("%s %s",p.addr.String(),header)
		break
	case FileChunkRequest:
		cmd = "REGCH"
		header = fmt.Sprintf("%s %s",p.addr.String(),header)
		break
	}
	message:=fmt.Sprintf("%s %s\r\n%s",cmd,header,string(payload))
	_,err := p.w.Write([]byte(message))
	if err==nil{
		err = p.w.Flush()
	}
	if err != nil {               
		return err
	}
	return nil
}



func (p *Peer) readShareFile(filepath string){
	f, err := os.Open(filepath)
	if err != nil {
        log.Fatalf("unable to read file: %v", err)
    }
	defer f.Close()
	fstat,_:=f.Stat()
	message:=fmt.Sprintf("%s %s",getFileName(filepath),strconv.Itoa(int(fstat.Size())))
    buf := make([]byte, maxBufferSize)
	p.SendMessage(RegisterRequest,message,[]byte(""))
	i:=0
	for {
		n,err:=f.Read(buf)
		if err == io.EOF {
			break
		}
		if err!=nil{
			fmt.Println(err)
			continue
		}
		println(i,n,buf)
		i+=1
		if n > 0 {
	 		content:= fmt.Sprintf("%s %d",getFileName(filepath),n)
			fmt.Println("sending",content)
			p.SendMessage(FileChunkRequest,content,buf[:n])
		} else{
			break
		}
	}

}

func getFileName(path string) string{ return filepath.Base(path) }
func main(){
	args := os.Args[1:]
	var selfPeer Peer= Peer{}
	selfPeer.addr= net.TCPAddr{IP:net.ParseIP(args[0])}
	port, err := strconv.Atoi(args[1])
	if err!=nil{
		fmt.Println("Can't use port",port)
	}
	selfPeer.addr.Port = port
	conn,err := net.Dial("tcp",serverIP+":"+serverPort)
	if err!=nil{
		fmt.Printf("Server is not online - %v\n",err)
		return
	}
	selfPeer.conn = conn
	selfPeer.w = bufio.NewWriter(conn)
	fmt.Println(selfPeer.addr.IP.String())
	if len(args)>=3{
		i:=2
		
		for _,file:=range(args[i:]){
			fmt.Println("r",args,file)
			selfPeer.readShareFile(file)
		}
	}
	reader := bufio.NewScanner(os.Stdin)
	displayHelpPrompt()
	printRepl()
	for reader.Scan(){
		text := strings.Fields(stripInput(reader.Text()))
		switch text[0]{
		case "help":
			displayHelpPrompt()
		case "download":
			downloadFile(text[1])
		case "list":
			listFiles()
		case "progress":
			displayProgress()
		default:
			printUnknown(text[0])
		}
		
		printRepl()
	}

}

// ./peer/peer1/peer localhost 8001 peer/peer1/a.txt peer/peer1/sample.txt

func downloadFile(file string){}

func listFiles(){
	//make a FileList request to server
}

func displayProgress(){
	var wg sync.WaitGroup
    p := mpb.New(mpb.WithWaitGroup(&wg))
    var total int64 = 100
	numBars :=  6
    wg.Add(numBars)
	for i := 0; i < numBars; i++ {
        name := fmt.Sprintf("Bar#%d:", i)
        bar := p.AddBar(total,
            mpb.PrependDecorators(
                decor.Name(name),
				decor.OnComplete(decor.EwmaETA(decor.ET_STYLE_GO, 60, decor.WCSyncWidth), "done",),
            ),
            mpb.AppendDecorators(decor.Counters(decor.UnitKiB, "% .1f / %d"),),
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
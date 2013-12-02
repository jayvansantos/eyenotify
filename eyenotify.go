package main

import (
	"syscall"
	"bytes"
	"encoding/binary"
	"flag"
	"log"
	"os"
	"os/exec"
	"io/ioutil"
	"strings"
	"time"
)

const (
	EVENT_SIZE = 16
)

type inotifyEvent struct {
	wd int32
	mask int32
	cookie int32
	length int32
	name string
}

type polledFile struct {
	path string
	modTime time.Time
}

var (
	path string
	command string
	ext string
	pid int
	polling bool
	lastEvent *inotifyEvent
	pollList map[string] polledFile
	ignoreDir map[string]bool
)

func init() {
	flag.StringVar(&path, "watch", ".", "path to be watched")
	flag.StringVar(&command, "command", "echo", "path to be watched")
	flag.StringVar(&ext, "ext", "go", "extension to be watched")
	flag.BoolVar(&polling, "polling", false, "use polling")
	flag.BoolVar(&polling, "p", false, "use polling")
	flag.Parse()
	ignoreDir := make(map[string]bool,256)
	ignoreDir[".git"] = true 
}

func intFromByte(byteSlice []byte, data interface{} ) {
	err := binary.Read(bytes.NewBuffer(byteSlice), binary.LittleEndian, data)
	if err != nil {
		log.Fatal("binary.read failed: ", err)
	}
}

func runApp() {
	log.Print("Starting Process...")
	commandArray := strings.Split(command, " ")
	paramArray := commandArray[1:]
	cmd := exec.Command(commandArray[0], paramArray...)
	err := cmd.Start()
	if err != nil {
		log.Fatal(err)
	}
	log.Print("Process Started Successfuly: ",cmd.Process.Pid)
	pid = cmd.Process.Pid
}

func processBuffer(n int, buffer []byte) {
	event := new(inotifyEvent)
	defer func() { lastEvent = event }()
	var  i int32

	for i < int32(n) {
		intFromByte(buffer[i: i + 4], &event.wd)
		intFromByte(buffer[i+ 4: i + 8], &event.mask)
		intFromByte(buffer[i + 8: i + 12], &event.cookie)
		intFromByte(buffer[i + 12: i + 16], &event.length)
		event.name =string(buffer[i + 16: i + 16 + event.length])
		event.name = strings.TrimRight(event.name, "\x00")
		i += EVENT_SIZE + event.length

//		log.Print(event)
//		continue

		if(len(strings.Split(event.name,".")) > 1) {
			eventExt := strings.Split(event.name,".")[1]
			if(ext == eventExt){
				if lastEvent != nil && lastEvent.name == event.name && lastEvent.mask == 0x100 && event.mask == 0x2 {
					log.Print("Skipping as we already processed events for file: ", event.name)
					break
				}
				log.Print("Killing Process:  ",pid)
				if proc, err := os.FindProcess(pid); err != nil {
					log.Print("error: ",err)
					runApp()
				}else{
					err := proc.Kill()
					if err != nil {
						log.Print("error: ", err)
					}
					_, err = proc.Wait()
					if err != nil {
						log.Print("error: ", err)
					}
					runApp()
				}
				break
			}
		}
	}	
}

func runInotify() {
	fd, err := syscall.InotifyInit()
	if err != nil {
		log.Fatal("error initializing Inotify: ", err)
		return
	}
	//_, err = syscall.InotifyAddWatch(fd, path, syscall.IN_MODIFY | syscall.IN_CLOSE_WRITE | syscall.IN_DELETE | syscall.IN_CREATE)
	_, err = syscall.InotifyAddWatch(fd, path, syscall.IN_ALL_EVENTS)
	if err != nil {
		log.Fatal("error adding watch: ", err)
		return
	}

	var buffer []byte = make([]byte, 1024 * EVENT_SIZE)

	for {
		n, err := syscall.Read(fd, buffer)
		if err != nil {
			log.Fatal("Read failed: ", err)
			return
		}
		processBuffer(n, buffer)
	}
}

func addFilesToPoll(filePath string) {
		fileList, err := ioutil.ReadDir(filePath)
		if err != nil {
			log.Fatal("ReadDir failed: ", err)
		}
		for _,file := range fileList {
			newPath := filePath + "/" + file.Name()
			if file.IsDir() && file.Name() != ".git" {
				pollList[newPath] = polledFile{path:newPath, modTime:file.ModTime()}
				addFilesToPoll(newPath)
			} else {
				fileName := file.Name()
				if(len(strings.Split(fileName,".")) > 1) {
					fileExt := strings.Split(fileName,".")[1]
					if(fileExt == ext){
						pollList[newPath] = polledFile{path:newPath, modTime:file.ModTime()}
						log.Print(fileName, " - ", file.ModTime())
					}
				}
			}
		}
}

func runPolling() {
	pollList = make(map[string] polledFile)
	addFilesToPoll(path)
	for{
		for file,modTime := range pollList {
			fileInfo, err := os.Stat(file)
			if err != nil {
				log.Fatal("Stat error: ", err)
			}
			pollList[file] = polledFile{path: file, modTime:fileInfo.ModTime()}
			log.Print(file, " - ",  modTime)
		}
		time.Sleep(500*time.Millisecond)
	}	
}

func main() {
	runApp()
	if polling {
		runPolling()
	} else {
		runInotify()
	}
}
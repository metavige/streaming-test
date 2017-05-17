package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/op/go-logging"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
)

// BUFSIZE 緩存空間大小
const BUFSIZE = 1024 * 8

// LOGGER_NAME logger name
const LOGGER_NAME = "stream-server"

var (
	log      = logging.MustGetLogger(LOGGER_NAME)
	Port     = ":8080"
	MediaDir = os.TempDir()
	router   = mux.NewRouter()
)

// 讀取 Media 檔案
func handlerMedia(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	urlPath := r.URL.Path[1:]
	fileName := vars["file"]

	log.Debugf("path: %v, fileName: %v\n", urlPath, fileName)

	file, err := os.Open(path.Join(MediaDir, fileName))

	if err != nil {
		w.WriteHeader(500)
		return
	}

	defer file.Close()

	fi, err := file.Stat()

	if err != nil {
		w.WriteHeader(500)
		return
	}

	fileSize := int(fi.Size())

	if len(r.Header.Get("Range")) == 0 {

		contentLength := strconv.Itoa(fileSize)
		contentEnd := strconv.Itoa(fileSize - 1)

		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", contentLength)
		w.Header().Set("Content-Range", "bytes 0-"+contentEnd+"/"+contentLength)
		w.WriteHeader(200)

		buffer := make([]byte, BUFSIZE)

		for {
			n, err := file.Read(buffer)

			if n == 0 {
				break
			}

			if err != nil {
				break
			}

			data := buffer[:n]
			w.Write(data)
			w.(http.Flusher).Flush()
		}

	} else {

		rangeParam := strings.Split(r.Header.Get("Range"), "=")[1]
		splitParams := strings.Split(rangeParam, "-")

		// response values

		contentStartValue := 0
		contentStart := strconv.Itoa(contentStartValue)
		contentEndValue := fileSize - 1
		contentEnd := strconv.Itoa(contentEndValue)
		contentSize := strconv.Itoa(fileSize)

		if len(splitParams) > 0 {
			contentStartValue, err = strconv.Atoi(splitParams[0])

			if err != nil {
				contentStartValue = 0
			}

			contentStart = strconv.Itoa(contentStartValue)
		}

		if len(splitParams) > 1 {
			contentEndValue, err = strconv.Atoi(splitParams[1])

			if err != nil {
				contentEndValue = fileSize - 1
			}

			contentEnd = strconv.Itoa(contentEndValue)
		}

		contentLength := strconv.Itoa(contentEndValue - contentStartValue + 1)

		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", contentLength)
		w.Header().Set("Content-Range", "bytes "+contentStart+"-"+contentEnd+"/"+contentSize)
		w.WriteHeader(206)

		buffer := make([]byte, BUFSIZE)

		file.Seek(int64(contentStartValue), 0)

		writeBytes := 0

		for {
			n, err := file.Read(buffer)

			writeBytes += n

			if n == 0 {
				break
			}

			if err != nil {
				break
			}

			if writeBytes >= contentEndValue {
				data := buffer[:BUFSIZE-writeBytes+contentEndValue+1]
				w.Write(data)
				w.(http.Flusher).Flush()
				break
			}

			data := buffer[:n]
			w.Write(data)
			w.(http.Flusher).Flush()
		}
	}
}

// 上傳檔案
func handlerUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(404)
		return
	} else {
		r.ParseMultipartForm(32 << 20)
		file, handler, err := r.FormFile("file")
		if err != nil {
			log.Error(err)
			return
		}
		log.Debugf("Upload: %v", handler.Header)

		defer file.Close()

		mediaURL, _ := router.Get("media").URL("file", handler.Filename)
		w.Header().Add("Location", mediaURL.String())
		w.WriteHeader(201)

		f, err := os.OpenFile(path.Join(MediaDir, handler.Filename),
			os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			log.Error(err)
			return
		}
		defer f.Close()
		io.Copy(f, file)
	}
}

func main() {
	// init logger
	var format = logging.MustStringFormatter("%{level} %{message}")
	logging.SetFormatter(format)
	logging.SetLevel(logging.DEBUG, LOGGER_NAME)

	for _, e := range os.Environ() {
		pair := strings.Split(e, "=")
		if pair[0] == "PORT" {
			Port = fmt.Sprintf(":%v", pair[1])
		}
		if pair[0] == "MEDIA_FOLDER" {
			MediaDir = pair[1]
		}
	}

	if _, err := os.Stat(MediaDir); os.IsNotExist(err) {
		os.Mkdir(MediaDir, 0775)
	}

	var listener net.Listener
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Info("Close Server..... ")
		listener.Close()
		os.Exit(0)
	}()

	// serve static files
	router.PathPrefix("/static/").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir("./wwwroot"))))

	router.HandleFunc("/media/{file}", handlerMedia).Name("media")
	router.HandleFunc("/upload", handlerUpload).Methods("POST")

	var lerr error
	listener, lerr = net.Listen("tcp", Port)
	if lerr != nil {
		log.Fatal(lerr)
		os.Exit(1)
		return
	}

	log.Infof("Start Listen: %v", Port)
	log.Infof("Media Folder: %v", MediaDir)
	http.Serve(listener, router)

	//listener = http.ListenAndServe(":8080", router)
}

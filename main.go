package main

import (
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/teris-io/shortid"
)

var uploadsDir string

func init() {
	workDir, _ := os.Getwd()
	uploadsDir = filepath.Join(workDir, "static")
}

func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	var err error
	var input multipart.File
	var handler *multipart.FileHeader

	r.ParseMultipartForm(32 << 20)
	if input, handler, err = r.FormFile("file"); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	defer input.Close()

	var out *os.File
	var sid string
	log.Println(handler)
	if sid, err = shortid.Generate(); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else if out, err = os.OpenFile("./uploads/"+sid, os.O_WRONLY|os.O_CREATE, 0666); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	defer out.Close()
	if _, err := io.Copy(out, input); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/?token="+sid, http.StatusSeeOther)
}

func handleFileDownload(w http.ResponseWriter, r *http.Request) {
	var token string
	if token = strings.TrimSpace(r.URL.Query().Get("token")); token == "" || strings.ContainsAny(token, "/.") {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	filePath := "./uploads/" + token

	var err error
	var fi os.FileInfo
	if fi, err = os.Stat("./uploads/" + token); os.IsNotExist(err) || fi.IsDir() {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	respFile, err := os.Open(filePath)
	defer respFile.Close()
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=uploaded_file")
	w.Header().Set("Content-Type", r.Header.Get("Content-Type"))

	io.Copy(w, respFile)
}

func init() {
	if fi, err := os.Stat("uploads"); os.IsNotExist(err) {
		os.Mkdir("uploads", os.ModePerm)
	} else if !fi.IsDir() {
		log.Fatal("'uploads' should be a directory")
	}
}

func main() {
	http.HandleFunc("/api/upload", handleFileUpload)
	http.HandleFunc("/api/download", handleFileDownload)
	http.Handle("/", http.FileServer(http.Dir("static")))

	log.Println("Listening...")
	http.ListenAndServe(":3000", nil)
}

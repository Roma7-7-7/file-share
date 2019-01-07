package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/teris-io/shortid"
)

var uploadDetailsBucket = []byte("uploadDetails")

var db *bolt.DB

type uploadDetails struct {
	Name string
}

func (ud *uploadDetails) Marshal() ([]byte, error) {
	return json.Marshal(ud)
}

func (ud *uploadDetails) Unmarshal(data []byte) error {
	if err := json.Unmarshal(data, &ud); err != nil {
		return err
	}

	return nil
}

func saveUploadDetails(id string, handler *multipart.FileHeader) error {
	return db.Update(func(tx *bolt.Tx) error {
		var err error
		var b []byte
		u := uploadDetails{
			Name: handler.Filename,
		}

		if b, err = u.Marshal(); err != nil {
			return err
		}

		return tx.Bucket(uploadDetailsBucket).Put([]byte(id), b)
	})
}

func getUploadDetails(id string) (*uploadDetails, error) {
	var ud *uploadDetails

	if err := db.View(func(tx *bolt.Tx) error {
		var b []byte

		if b = tx.Bucket(uploadDetailsBucket).Get([]byte(id)); b == nil || len(b) == 0 {
			return nil
		}

		ud = &uploadDetails{}
		return ud.Unmarshal(b)
	}); err != nil {
		return nil, err
	}

	return ud, nil
}

func deleteUploadDetails(id string) error {
	return db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(uploadDetailsBucket).Delete([]byte(id))
	})
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
	}

	filePath := "./uploads/" + sid
	if out, err = os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0666); err != nil {
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
	if err := saveUploadDetails(sid, handler); err != nil {
		log.Println(err)
		os.Remove(filePath)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/?token="+sid, http.StatusSeeOther)
}

func removeUploadDetailsAndFile(udID string) {
	if err := os.Remove("./uploads/" + udID); err != nil {
		log.Println(err)
	}

	deleteUploadDetails(udID)
}

func handleFileDownload(w http.ResponseWriter, r *http.Request) {
	var err error
	var token string
	if token = strings.TrimSpace(r.URL.Query().Get("token")); token == "" || strings.ContainsAny(token, "/.") {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var details *uploadDetails
	if details, err = getUploadDetails(token); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else if details == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	filePath := "./uploads/" + token

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

	defer removeUploadDetailsAndFile(token)

	w.Header().Set("Content-Disposition", "attachment; filename="+details.Name)
	w.Header().Set("Content-Type", r.Header.Get("Content-Type"))

	io.Copy(w, respFile)
}

func setupDb() {
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(uploadDetailsBucket); err != nil {
			return fmt.Errorf("Failed to create uploads bucket")
		}

		return nil
	}); err != nil {
		log.Println(err)
		log.Fatal("Failed to initialize db")
	}
}

func init() {
	if fi, err := os.Stat("uploads"); os.IsNotExist(err) {
		os.Mkdir("uploads", os.ModePerm)
	} else if !fi.IsDir() {
		log.Fatal("'uploads' should be a directory")
	}
}

func main() {
	var err error

	log.Println("Initializing datatbase")
	if db, err = bolt.Open("file-share.db", 0600, nil); err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	setupDb()

	log.Println("Initializing web endpoints")
	http.HandleFunc("/api/upload", handleFileUpload)
	http.HandleFunc("/api/download", handleFileDownload)
	http.Handle("/", http.FileServer(http.Dir("static")))

	log.Println("Listening...")
	http.ListenAndServe(":3000", nil)
}

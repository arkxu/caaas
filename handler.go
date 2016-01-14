package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/gocql/gocql"
)

func init() {
	// only support 3 type of images
	image.RegisterFormat("jpeg", "jpeg", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
	image.RegisterFormat("gif", "gif", gif.Decode, gif.DecodeConfig)
}

type ImgHandler struct {
	cluster *gocql.ClusterConfig
}

func (h *ImgHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	session, err := h.cluster.CreateSession()
	if err != nil {
		log.Panic(err)
	}
	defer session.Close()

	switch req.Method {
	case "GET":
		h.get(w, req, session)
	case "POST":
		h.post(w, req, session)
	}
}

func (h *ImgHandler) get(w http.ResponseWriter, req *http.Request, session *gocql.Session) {
	path := strings.TrimPrefix(req.URL.Path, "/")
	if id := h.getUUID(path); id != "" {
		// if file was cached, return directly
		cachedFile, _ := filepath.Abs(filepath.Clean(Config.Image.CacheDir + req.URL.Path))
		cachedData, err := ioutil.ReadFile(cachedFile)
		if err == nil {
			w.Write(cachedData)
			return
		}

		// not cached, going to process image and cache it for later use
		width, mode, height := h.getSizes(path)

		asset, err := new(Asset).Find(session, id)
		if err != nil {
			log.Panic(err)
		}

		buf, err := h.processImage(bytes.NewBuffer(asset.Binary), mode, width, height)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		// create cache image and wrap a multiwriter
		cacheFile, _ := os.Create(cachedFile)
		defer cacheFile.Close()

		multiWriter := io.MultiWriter(w, cacheFile)
		multiWriter.Write(buf.Bytes())
	} else {
		// list image under a path
		assets, err := new(Asset).FindByPath(session, path)
		if err != nil {
			log.Panic(err)
		}
		data, err := json.Marshal(assets)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

func (h *ImgHandler) post(w http.ResponseWriter, req *http.Request, session *gocql.Session) {
	path := req.URL.Path
	if path == "" || path == "/" {
		log.Println("Please specify the path")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("please specify the path"))
		return
	}

	req.ParseMultipartForm(10 << 20) // 10M
	file, fileHeader, err := req.FormFile("file")
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	log.Println(fileHeader.Filename)

	buf, err := h.processImage(file, "z", Config.Image.StoreWidth, Config.Image.StoreHeight)
	if err != nil {
		log.Panic(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	fileName, _ := url.QueryUnescape(fileHeader.Filename)

	log.Println("resized:", fileName)
	asset := &Asset{
		Name:        fileName,
		Path:        strings.Split(strings.TrimPrefix(req.URL.Path, "/"), "/"),
		ContentType: "image/jpeg",
		CreatedAt:   time.Now(),
		Binary:      buf.Bytes(),
	}

	err = asset.Save(session)
	if err != nil {
		log.Panic(err)
	}
	log.Println("saved:", fileName)
	data, err := json.Marshal(asset)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (h *ImgHandler) getUUID(text string) string {
	r := regexp.MustCompile("[a-z0-9]{8}-[a-z0-9]{4}-[1-5][a-z0-9]{3}-[a-z0-9]{4}-[a-z0-9]{12}")
	if r.MatchString(text) {
		return r.FindStringSubmatch(text)[0]
	}
	return ""
}

// get the width, crop mode and height
func (h *ImgHandler) getSizes(path string) (int, string, int) {
	segments := strings.Split(path, "__")

	width := Config.Image.DefaultWidth
	height := Config.Image.DefaultHeight
	mode := "z"

	if len(segments) > 1 {
		reg := regexp.MustCompile(`([0-9]+)([z|x])([0-9]+)`)
		parts := reg.FindStringSubmatch(segments[1])
		widthInt, _ := strconv.Atoi(parts[1])
		mode = parts[2]
		heightInt, _ := strconv.Atoi(parts[3])

		if widthInt != 0 || heightInt != 0 {
			width = widthInt
			height = heightInt

			if widthInt == 0 {
				height = heightInt
				width = height
			}

			if heightInt == 0 {
				width = widthInt
				height = width
			}
		}
	}
	return width, mode, height
}

func (h *ImgHandler) processImage(in io.Reader, mode string, width int, height int) (*bytes.Buffer, error) {
	ImageChannel <- 1
	defer func() {
		<-ImageChannel
	}()

	img, _, err := image.Decode(in)
	if err != nil {
		log.Panic(err)
		return nil, err
	}

	var m *image.NRGBA
	switch mode {
	case "z":
		m = imaging.Fit(img, width, height, imaging.Lanczos)
	case "x":
		m = imaging.Fill(img, width, height, imaging.Center, imaging.Lanczos)
	}

	buf := new(bytes.Buffer)
	jpeg.Encode(buf, m, &jpeg.Options{Config.Image.ReadQuality})
	return buf, nil
}

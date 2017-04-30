package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"path"
)

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func handleThumbnailRequest(w http.ResponseWriter, req *http.Request, root string) {
	videoUri, err := parseUriFromRequest(req)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	if !regexp.MustCompile("^https?://[^/]*pr0gramm.com/.*").MatchString(videoUri) {
		w.WriteHeader(403)
		w.Write([]byte("Uri not allowed"))
		return
	}

	bufferSizes := []int{128, 256, 512, 1024}
	if strings.HasSuffix(videoUri, ".mp4") {
		bufferSizes = []int{256, 512, 1024}
	}

	for _, bufferSize := range bufferSizes {
		log.Printf("Try to get thumbnail for %s with size %dkb\n",
			path.Base(videoUri), bufferSize)

		if err = generateThumbnail(w, videoUri, root, bufferSize*1024); err == nil {
			// success, we don't need to try the next size!
			break
		}
	}

	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
	}
}

func openLastFrame(dir string) (*os.File, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.webp"))
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, errors.New("No resulting images found")
	}

	// open the last frame
	sort.Strings(files)
	frame := files[len(files)-1]
	return os.Open(frame)
}

func bufferVideoUriIfNecessary(videoUri string, temp string, bufferSize int) (string, error) {
	suffix := ".mp4"
	if strings.Contains(videoUri, ".gif") {
		suffix = ".gif"
	}

	// open target file
	target := filepath.Join(temp, "file"+suffix)
	file, err := os.Create(target)
	if err != nil {
		return "", errors.WithMessage(err, "Could not open target file")
	}

	defer file.Close()

	request, err := http.NewRequest("GET", videoUri, nil)
	if err != nil {
		return "", errors.WithMessage(err, "Could not create request")
	}

	// only request the start of the file
	request.Header.Set("Range", fmt.Sprintf("bytes=0-%d", bufferSize))

	// do http request to source file
	resp, err := httpClient.Do(request)
	if err != nil {
		return "", errors.WithMessage(err, "Could not execute request")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return "", errors.New("Status code not 200.")
	}

	// download the gif file
	io.Copy(file, resp.Body)
	return target, nil
}

func generateThumbnail(w http.ResponseWriter, videoUri string, root string, bufferSize int) error {
	temp, err := ioutil.TempDir(root, "thumb")
	if err != nil {
		return errors.WithMessage(err, "Could not create temporary directory")
	}

	videoFile, err := bufferVideoUriIfNecessary(videoUri, temp, bufferSize)
	if err != nil {
		return errors.WithMessage(err, "Could not download video file")
	}

	// remove temp dir at the end
	defer os.RemoveAll(temp)

	var videoInfo VideoInfo
	if !strings.Contains(videoUri, ".gif") {
		// get video info
		videoInfo, err = probeVideoInfo(videoFile)
		if err != nil {
			return errors.WithMessage(err, "Could not get video info")
		}
	}

	timeOffset := math.Min(2.0, videoInfo.Format.Duration/10.0)

	log.Printf("Get thumbnail at time %f\n", timeOffset)

	// execute ffmpeg to create the thumbnail
	argv := []string{
		"-y", "-i", videoFile,
		"-ss", fmt.Sprintf("%f", timeOffset),
		"-vf", "scale='if(gt(iw,1024),1024,iw)':-1,boxblur=1:1",
		"-f", "image2", "-q:v", "20", "-vframes", "1", "out-%04d.webp"}

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	cmd := exec.CommandContext(ctx, "ffmpeg", argv...)
	cmd.Dir = temp

	if err := cmd.Run(); err != nil {
		return errors.WithMessage(err, "FFmpeg stopped with an error.")
	}

	// read the resulting file
	if file, err := openLastFrame(cmd.Dir); err == nil {
		defer file.Close()

		// send to the client
		w.Header().Set("Content-Type", "image/webp")
		io.Copy(w, file)
	}

	return err
}

func parseUriFromRequest(req *http.Request) (string, error) {
	vars := mux.Vars(req)
	encodedVideoUri := vars["url"]
	if encodedVideoUri == "" {
		return "", errors.New("No encoded url found")
	}

	videoUriBytes, err := base64.URLEncoding.DecodeString(encodedVideoUri)
	if err != nil {
		return "", errors.New("Could not decode uri string")
	}

	// normalize
	videoUri := strings.TrimSpace(string(videoUriBytes))
	videoUri = strings.Replace(videoUri, "https://", "http://", -1)
	videoUri = strings.Replace(videoUri, ".mpg", ".mp4", -1)
	return videoUri, nil
}

type VideoInfo struct {
	Format struct {
		Duration float64 `json:"duration,string"`
	}
}

func probeVideoInfo(file string) (VideoInfo, error) {
	var info VideoInfo
	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", file)

	output, err := cmd.Output()
	if err != nil {
		return info, errors.WithMessage(err, "Could not run ffprobe")
	}

	// parse result into json!
	err = json.Unmarshal(output, &info)

	if info.Format.Duration == 0 {
		fmt.Println(string(output))
	}

	return info, errors.WithMessage(err, "Error parsing ffprobe result")
}

func main() {
	args := parseArguments()

	limiter := make(chan int, args.Concurrent)

	router := &mux.Router{}
	router.HandleFunc("/{url}", func(w http.ResponseWriter, req *http.Request) {
		// do request limiting
		limiter <- 1
		defer func() {
			<-limiter
		}()

		handleThumbnailRequest(w, req, args.Path)
	})

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", args.Port),
		handlers.RecoveryHandler()(
			handlers.LoggingHandler(os.Stdout,
				handlers.CORS()(router)))))
}

package main // import "github.com/mopsalarm/go-pr0gramm-thumbnail"

import (
  "fmt"
  "os"
  "log"
  "net/http"
  "github.com/gorilla/handlers"
  "github.com/gorilla/mux"
  "encoding/base64"
  "errors"
  "io"
  "strings"
  "os/exec"
  "io/ioutil"
  "path/filepath"
  "sort"
  "time"
  "regexp"
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

  if ! regexp.MustCompile("^https?://[^/]*pr0gramm.com/.*").MatchString(videoUri) {
    w.WriteHeader(403)
    w.Write([]byte("Uri not allowed"))
    return
  }

  if err = generateThumbnail(w, videoUri, root); err != nil {
    w.WriteHeader(500)
    w.Write([]byte(err.Error()))
    return
  }
}

func openLastFrame(dir string) (*os.File, error) {
  files, err := filepath.Glob(dir + "/*.webp")
  if err != nil {
    return nil, err
  }

  if len(files) == 0 {
    return nil, errors.New("No resulting images found")
  }

  // open the last frame
  sort.Strings(files)
  frame := files[len(files) - 1]
  return os.Open(frame)
}

func bufferVideoUriIfNecessary(videoUri string, temp string) (string, error) {
  suffix := ".webm"
  if ! strings.Contains(videoUri, ".gif") {
    suffix = ".gif"
  }

  // open target file
  target := temp + "/input" + suffix
  file, err := os.Create(target)
  if err != nil {
    return "", err
  }

  defer file.Close()

  // do http request to source file
  resp, err := httpClient.Get(videoUri)
  if err != nil {
    return "", err
  }

  defer resp.Body.Close()

  // download the gif file
  io.Copy(file, resp.Body)
  return target, nil
}

func generateThumbnail(w http.ResponseWriter, videoUri string, root string) error {
  var err error

  temp, err := ioutil.TempDir(root, "thumb")
  if err != nil {
    return err
  }

  videoUri, err = bufferVideoUriIfNecessary(videoUri, temp)
  if err != nil {
    return err
  }

  // remove temp dir at the end
  defer os.RemoveAll(temp)

  // Execute ffmpeg
  argv := []string{"-y", "-i", videoUri,
    "-vf", "scale='if(gt(iw,1024),1024,iw)':-1",
    "-f", "image2", "-t", "3", "-r", "1", "-q:v", "20", "out-%04d.webp"}

  cmd := exec.Command(root + "/ffmpeg", argv...)
  cmd.Dir = temp
  if err = cmd.Start(); err != nil {
    return err
  }

  // set a timeout so the process wont block everything
  var timer *time.Timer
  timer = time.AfterFunc(10 * time.Second, func() {
    timer.Stop()
    cmd.Process.Kill()
  })

  // wait for ffmpeg to finish
  err = cmd.Wait()
  timer.Stop()
  if err != nil {
    return err
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
  videoUri := strings.Replace(string(videoUriBytes), "https://", "http://", -1)
  videoUri = strings.Replace(string(videoUriBytes), ".mpg", ".webm", -1)
  return string(videoUri), nil
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

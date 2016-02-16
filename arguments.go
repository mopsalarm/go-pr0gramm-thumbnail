package main

import (
  "os"
  "github.com/bobziuchkovski/writ"
)

type Args struct {
  HelpFlag   bool   `flag:"help" description:"Display this help message and exit"`
  Port       int    `option:"p, port" default:"8080" description:"The port to open the rest service on"`
  Concurrent int `option:"c, concurrent" default:"16" description:"Maximum number of concurrent processes"`
  Path       string `option:"path" default:"/tmp" description:"Path where to put temp folders and where to find ffmpeg"`
  Datadog    string `option:"datadog" description:"Datadog api key for reporting"`
}

func parseArguments() Args {
  args := Args{}
  cmd := writ.New("thumbnail", &args)

  // Parse command line arguments
  _, _, err := cmd.Decode(os.Args[1:])
  if err != nil || args.HelpFlag {
    cmd.ExitHelp(err)
  }

  return args
}

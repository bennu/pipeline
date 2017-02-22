package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/cncd/pipeline/pipeline"
	"github.com/cncd/pipeline/pipeline/backend"
	"github.com/cncd/pipeline/pipeline/backend/docker"
	"github.com/cncd/pipeline/pipeline/multipart"
	"github.com/urfave/cli"
)

var executeCommand = cli.Command{
	Name:   "exec",
	Usage:  "execute the compiled file",
	Action: executeAction,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "in",
			Value: "pipeline.json",
		},
		cli.DurationFlag{
			Name:   "timeout",
			EnvVar: "CI_TIMEOUT",
			Value:  time.Hour,
		},
	},
}

func executeAction(c *cli.Context) (err error) {
	path := c.Args().First()
	if path == "" {
		path = c.String("in")
	}

	var reader io.ReadCloser
	if path == "-" {
		reader = os.Stdin
	} else {
		reader, err = os.Open(path)
		if err != nil {
			return err
		}
	}
	defer reader.Close()

	config, err := pipeline.Parse(reader)
	if err != nil {
		return err
	}

	engine, err := docker.NewEnv()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.Duration("timeout"))
	defer cancel()
	ctx = WithInterrupt(ctx)

	return pipeline.New(config,
		pipeline.WithContext(ctx),
		pipeline.WithLogger(defaultLogger),
		pipeline.WithTracer(defaultTracer),
		pipeline.WithEngine(engine),
	).Run()
}

var defaultLogger = pipeline.LogFunc(func(proc *backend.Step, rc multipart.Reader) error {
	part, err := rc.NextPart()
	if err != nil {
		return err
	}
	io.Copy(os.Stderr, part)
	return nil
})

var defaultTracer = pipeline.TraceFunc(func(state *pipeline.State) error {
	if state.Process.Exited {
		fmt.Printf("proc %q exited with status %d\n", state.Pipeline.Step.Name, state.Process.ExitCode)
	} else {
		fmt.Printf("proc %q started\n", state.Pipeline.Step.Name)
		state.Pipeline.Step.Environment["CI_BUILD_STATUS"] = "success"
		state.Pipeline.Step.Environment["CI_BUILD_FINISHED"] = strconv.FormatInt(time.Now().Unix(), 10)
		if state.Pipeline.Error != nil {
			state.Pipeline.Step.Environment["CI_BUILD_STATUS"] = "failure"
		}
	}
	return nil
})
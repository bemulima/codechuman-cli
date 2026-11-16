package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/bemulima/codechuman-cli/internal/teacher"
	"io"
	"net/http"
	"os"
	"time"
)

func runTeacher(args []string, h *http.Client) error {
	f := flag.NewFlagSet("teacher-chat", flag.ContinueOnError)
	origin := f.String("api-base", "", "HTTPS Gateway")
	workspace := f.String("workspace", "", "existing checkout")
	stdin := f.Bool("request-stdin", false, "read ephemeral token/message JSON from stdin")
	if e := f.Parse(args); e != nil {
		return e
	}
	if !*stdin || *workspace == "" {
		return errors.New("--workspace and --request-stdin required")
	}
	raw, e := io.ReadAll(io.LimitReader(os.Stdin, 32769))
	if e != nil || len(raw) > 32768 {
		return errors.New("invalid Teacher request")
	}
	var input teacher.Request
	if json.Unmarshal(raw, &input) != nil {
		return errors.New("invalid Teacher request JSON")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	result, e := teacher.Chat(ctx, h, *origin, *workspace, input)
	if e != nil {
		return e
	}
	return writeJSON(result)
}

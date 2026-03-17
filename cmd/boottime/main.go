package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/boreec/boottime/exec"
)

func main() {
	var args Args
	var flags Flags

	fs := flag.NewFlagSet("boottime", flag.ContinueOnError)
	if err := parseArgs(fs, os.Args[1:], &args, &flags); err != nil {
		panic(err.Error())
	}

	if err := runWithArgs(&args, &flags); err != nil {
		panic(err.Error())
	}
}

type Flags struct {
	RunRetrieveBootTime bool
	RunAggregate        bool
	Prettify            bool
}

type Args struct {
	FileName string
}

func parseArgs(fs *flag.FlagSet, argv []string, args *Args, flags *Flags) error {
	fs.BoolVar(&flags.RunRetrieveBootTime, "R", false, "retrieve boot time")
	fs.BoolVar(&flags.RunRetrieveBootTime, "retrieve-boot-time", false, "retrieve boot time")

	fs.BoolVar(&flags.RunAggregate, "A", false, "average boot time records")
	fs.BoolVar(&flags.RunAggregate, "average-boot-records", false, "average boot time records")

	fs.BoolVar(&flags.Prettify, "p", false, "prettify results")
	fs.BoolVar(&flags.Prettify, "prettify", false, "prettify results")

	if err := fs.Parse(argv); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	argsUnparsed := fs.Args()
	if len(argsUnparsed) == 0 {
		return errors.New("expected 1 arg for jsonl file, found 0")
	}
	args.FileName = argsUnparsed[0]

	if !strings.HasSuffix(args.FileName, ".jsonl") {
		return errors.New("argument should be a file name with .jsonl suffix")
	}

	if flags.RunAggregate && flags.RunRetrieveBootTime {
		return errors.New("flags -A and -R are incompatible")
	}

	if !flags.RunAggregate && !flags.RunRetrieveBootTime {
		return errors.New("flags -A or -R required")
	}

	return nil
}

func runWithArgs(args *Args, flags *Flags) error {
	if flags.RunRetrieveBootTime {
		return exec.RetrieveBootTimes(args.FileName)
	}

	if flags.RunAggregate {
		return exec.PrintRecordsAverage(args.FileName, flags.Prettify)
	}

	return nil
}
